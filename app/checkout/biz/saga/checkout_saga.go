package saga

import (
	"context"
	"fmt"
	"strconv"

	"github.com/cloudwego/kitex/pkg/klog"
	"github.com/xvxiaoman8/gomall/app/checkout/biz/dal/redis"
	"github.com/xvxiaoman8/gomall/app/checkout/infra/rpc"
	"github.com/xvxiaoman8/gomall/common/saga"
	"github.com/xvxiaoman8/gomall/rpc_gen/kitex_gen/cart"
	checkout "github.com/xvxiaoman8/gomall/rpc_gen/kitex_gen/checkout"
	"github.com/xvxiaoman8/gomall/rpc_gen/kitex_gen/order"
	"github.com/xvxiaoman8/gomall/rpc_gen/kitex_gen/payment"
	"github.com/xvxiaoman8/gomall/rpc_gen/kitex_gen/product"
)

// BuildCheckoutSaga 构建 Checkout Saga
func BuildCheckoutSaga(ctx context.Context, req *checkout.CheckoutReq) ([]saga.SagaStep, map[string]interface{}) {
	sagaCtx := map[string]interface{}{
		"user_id":     req.UserId,
		"email":       req.Email,
		"address":     req.Address,
		"credit_card": req.CreditCard,
	}

	steps := []saga.SagaStep{
		{
			Name:    "decrease_stock",
			Service: "product",
			Action: func(ctx context.Context, sagaCtx map[string]interface{}) error {
				return decreaseStockAction(ctx, sagaCtx, req)
			},
			Compensate: func(ctx context.Context, sagaCtx map[string]interface{}) error {
				return increaseStockCompensate(ctx, sagaCtx)
			},
			RetryPolicy: saga.DefaultRetryPolicy(),
		},
		{
			Name:    "create_order",
			Service: "order",
			Action: func(ctx context.Context, sagaCtx map[string]interface{}) error {
				return createOrderAction(ctx, sagaCtx, req)
			},
			Compensate: func(ctx context.Context, sagaCtx map[string]interface{}) error {
				return cancelOrderCompensate(ctx, sagaCtx)
			},
			RetryPolicy: saga.DefaultRetryPolicy(),
		},
		{
			Name:    "empty_cart",
			Service: "cart",
			Action: func(ctx context.Context, sagaCtx map[string]interface{}) error {
				return emptyCartAction(ctx, sagaCtx)
			},
			Compensate: func(ctx context.Context, sagaCtx map[string]interface{}) error {
				return restoreCartCompensate(ctx, sagaCtx)
			},
			RetryPolicy: saga.DefaultRetryPolicy(),
		},
		{
			Name:    "charge_payment",
			Service: "payment",
			Action: func(ctx context.Context, sagaCtx map[string]interface{}) error {
				return chargePaymentAction(ctx, sagaCtx, req)
			},
			Compensate: func(ctx context.Context, sagaCtx map[string]interface{}) error {
				return refundPaymentCompensate(ctx, sagaCtx)
			},
			RetryPolicy: saga.DefaultRetryPolicy(),
		},
		{
			Name:    "mark_order_paid",
			Service: "order",
			Action: func(ctx context.Context, sagaCtx map[string]interface{}) error {
				return markOrderPaidAction(ctx, sagaCtx)
			},
			Compensate: func(ctx context.Context, sagaCtx map[string]interface{}) error {
				return unmarkOrderPaidCompensate(ctx, sagaCtx)
			},
			RetryPolicy: saga.DefaultRetryPolicy(),
		},
	}

	return steps, sagaCtx
}

// decreaseStockAction 扣减库存
func decreaseStockAction(ctx context.Context, sagaCtx map[string]interface{}, req *checkout.CheckoutReq) error {
	// 获取购物车
	cartResult, err := rpc.CartClient.GetCart(ctx, &cart.GetCartReq{UserId: req.UserId})
	if err != nil {
		return fmt.Errorf("get cart failed: %w", err)
	}

	if cartResult == nil || cartResult.Cart == nil || len(cartResult.Cart.Items) == 0 {
		return fmt.Errorf("cart is empty")
	}

	// 记录扣减的库存信息（用于补偿）
	stockDecreases := make(map[int64]int32)

	for _, cartItem := range cartResult.Cart.Items {
		productResp, err := rpc.ProductClient.GetProduct(ctx, &product.GetProductReq{Id: cartItem.ProductId})
		if err != nil {
			return fmt.Errorf("get product failed: %w", err)
		}

		if productResp.Product == nil {
			continue
		}

		p := productResp.Product
		productID := int64(p.Id)

		// 检查库存
		stockKey := strconv.Itoa(int(p.Id)) + "_stock"
		stock, err := redis.RedisDo(ctx, "GET", stockKey)
		if err != nil {
			return fmt.Errorf("get stock failed: %w", err)
		}

		stockStr, ok := stock.(string)
		if !ok {
			return fmt.Errorf("stock is not a string")
		}

		stockInt, err := strconv.Atoi(stockStr)
		if err != nil {
			return fmt.Errorf("parse stock failed: %w", err)
		}

		if int32(stockInt) < cartItem.Quantity {
			return fmt.Errorf("stock is not enough for product %d", productID)
		}

		// 扣减库存（使用 Lua 脚本保证原子性）
		script := `
			local stock = tonumber(redis.call('GET', KEYS[1]) or 0)
			if stock >= tonumber(ARGV[1]) then
				redis.call('DECRBY', KEYS[1], ARGV[1])
				return 1
			else
				return 0
			end
		`
		result, err := redis.RedisClient.Eval(ctx, script, []string{stockKey}, cartItem.Quantity).Int()
		if err != nil {
			return fmt.Errorf("decrease stock failed: %w", err)
		}

		if result == 0 {
			return fmt.Errorf("stock is not enough for product %d", productID)
		}

		// 记录扣减的库存
		stockDecreases[productID] = cartItem.Quantity
	}

	// 保存到上下文用于补偿
	sagaCtx["stock_decreases"] = stockDecreases
	sagaCtx["cart_items"] = cartResult.Cart.Items

	return nil
}

// increaseStockCompensate 回滚库存
func increaseStockCompensate(ctx context.Context, sagaCtx map[string]interface{}) error {
	stockDecreases, ok := sagaCtx["stock_decreases"].(map[int64]int32)
	if !ok || stockDecreases == nil {
		klog.Warn("no stock decreases to compensate")
		return nil
	}

	for productID, quantity := range stockDecreases {
		stockKey := strconv.FormatInt(productID, 10) + "_stock"
		_, err := redis.RedisDo(ctx, "INCRBY", stockKey, quantity)
		if err != nil {
			klog.Errorf("failed to restore stock for product %d: %v", productID, err)
			// 继续执行其他补偿
		}
	}

	return nil
}

// createOrderAction 创建订单
func createOrderAction(ctx context.Context, sagaCtx map[string]interface{}, req *checkout.CheckoutReq) error {
	// 从上下文获取订单项
	cartItems, ok := sagaCtx["cart_items"].([]*cart.CartItem)
	if !ok {
		return fmt.Errorf("cart items not found in context")
	}

	// 计算总金额和订单项
	var oi []*order.OrderItem
	var total float32

	for _, cartItem := range cartItems {
		productResp, err := rpc.ProductClient.GetProduct(ctx, &product.GetProductReq{Id: cartItem.ProductId})
		if err != nil {
			return fmt.Errorf("get product failed: %w", err)
		}

		if productResp.Product == nil {
			continue
		}

		p := productResp.Product
		cost := p.Price * float32(cartItem.Quantity)
		total += cost

		oi = append(oi, &order.OrderItem{
			Item: &cart.CartItem{ProductId: cartItem.ProductId, Quantity: cartItem.Quantity},
			Cost: cost,
		})
	}

	// 构建订单请求
	orderReq := &order.PlaceOrderReq{
		UserId:       req.UserId,
		UserCurrency: "USD",
		OrderItems:   oi,
		Email:        req.Email,
	}

	if req.Address != nil {
		addr := req.Address
		zipCodeInt, _ := strconv.Atoi(addr.ZipCode)
		orderReq.Address = &order.Address{
			StreetAddress: addr.StreetAddress,
			City:          addr.City,
			Country:       addr.Country,
			State:         addr.State,
			ZipCode:       int32(zipCodeInt),
		}
	}

	orderResult, err := rpc.OrderClient.PlaceOrder(ctx, orderReq)
	if err != nil {
		return fmt.Errorf("place order failed: %w", err)
	}

	if orderResult == nil || orderResult.Order == nil {
		return fmt.Errorf("order result is nil")
	}

	// 保存到上下文
	sagaCtx["order_id"] = orderResult.Order.OrderId
	sagaCtx["total_amount"] = total

	return nil
}

// cancelOrderCompensate 取消订单
func cancelOrderCompensate(ctx context.Context, sagaCtx map[string]interface{}) error {
	orderID, ok := sagaCtx["order_id"].(string)
	if !ok || orderID == "" {
		klog.Warn("no order id to cancel")
		return nil
	}

	userID, _ := sagaCtx["user_id"].(int64)

	// 调用订单服务取消订单（作为补偿）
	_, err := rpc.OrderClient.CancelOrder(ctx, &order.CancelOrderReq{
		UserId:  uint32(userID),
		OrderId: orderID,
	})

	if err != nil {
		klog.Errorf("failed to cancel order %s: %v", orderID, err)
		return err
	}

	return nil
}

// emptyCartAction 清空购物车
func emptyCartAction(ctx context.Context, sagaCtx map[string]interface{}) error {
	userID, ok := sagaCtx["user_id"].(int64)
	if !ok {
		return fmt.Errorf("user_id not found in context")
	}

	// 先获取购物车内容（用于补偿）
	cartResult, err := rpc.CartClient.GetCart(ctx, &cart.GetCartReq{UserId: userID})
	if err != nil {
		return fmt.Errorf("get cart failed: %w", err)
	}

	// 保存购物车内容到上下文
	if cartResult != nil && cartResult.Cart != nil {
		sagaCtx["original_cart_items"] = cartResult.Cart.Items
	}

	// 清空购物车
	_, err = rpc.CartClient.EmptyCart(ctx, &cart.EmptyCartReq{UserId: userID})
	if err != nil {
		return fmt.Errorf("empty cart failed: %w", err)
	}

	return nil
}

// restoreCartCompensate 恢复购物车
func restoreCartCompensate(ctx context.Context, sagaCtx map[string]interface{}) error {
	originalItems, ok := sagaCtx["original_cart_items"].([]*cart.CartItem)
	if !ok || originalItems == nil {
		klog.Warn("no original cart items to restore")
		return nil
	}

	userID, _ := sagaCtx["user_id"].(int64)

	// 恢复购物车项
	for _, item := range originalItems {
		_, err := rpc.CartClient.AddItem(ctx, &cart.AddItemReq{
			UserId: userID,
			Item: &cart.CartItem{
				ProductId: item.ProductId,
				Quantity:  item.Quantity,
			},
		})
		if err != nil {
			klog.Errorf("failed to restore cart item product %d: %v", item.ProductId, err)
			// 继续执行其他恢复
		}
	}

	return nil
}

// chargePaymentAction 支付
func chargePaymentAction(ctx context.Context, sagaCtx map[string]interface{}, req *checkout.CheckoutReq) error {
	orderID, ok := sagaCtx["order_id"].(string)
	if !ok || orderID == "" {
		return fmt.Errorf("order_id not found in context")
	}

	totalAmount, ok := sagaCtx["total_amount"].(float32)
	if !ok {
		return fmt.Errorf("total_amount not found in context")
	}

	payReq := &payment.ChargeReq{
		UserId:  req.UserId,
		OrderId: orderID,
		Amount:  totalAmount,
		CreditCard: req.CreditCard,
	}

	paymentResult, err := rpc.PaymentClient.Charge(ctx, payReq)
	if err != nil {
		return fmt.Errorf("charge failed: %w", err)
	}

	if paymentResult == nil {
		return fmt.Errorf("payment result is nil")
	}

	// 保存到上下文
	sagaCtx["payment_id"] = paymentResult.TransactionId

	return nil
}

// refundPaymentCompensate 退款
func refundPaymentCompensate(ctx context.Context, sagaCtx map[string]interface{}) error {
	paymentID, ok := sagaCtx["payment_id"].(string)
	if !ok || paymentID == "" {
		klog.Warn("no payment id to refund")
		return nil
	}

	userID, _ := sagaCtx["user_id"].(int64)
	totalAmount, _ := sagaCtx["total_amount"].(float32)
	creditCard, _ := sagaCtx["credit_card"].(*payment.CreditCardInfo)

	// 调用支付服务退款
	_, err := rpc.PaymentClient.Refund(ctx, &payment.RefundReq{
		UserId:        userID,
		TransactionId: paymentID,
		Amount:        totalAmount,
		CreditCard:    creditCard,
	})

	if err != nil {
		klog.Errorf("failed to refund payment %s: %v", paymentID, err)
		return err
	}

	return nil
}

// markOrderPaidAction 标记订单已支付
func markOrderPaidAction(ctx context.Context, sagaCtx map[string]interface{}) error {
	orderID, ok := sagaCtx["order_id"].(string)
	if !ok || orderID == "" {
		return fmt.Errorf("order_id not found in context")
	}

	userID, _ := sagaCtx["user_id"].(int64)

	_, err := rpc.OrderClient.MarkOrderPaid(ctx, &order.MarkOrderPaidReq{
		UserId:  userID,
		OrderId: orderID,
	})

	if err != nil {
		return fmt.Errorf("mark order paid failed: %w", err)
	}

	return nil
}

// unmarkOrderPaidCompensate 取消订单已支付标记
// 通过将订单状态改为canceled来实现补偿
func unmarkOrderPaidCompensate(ctx context.Context, sagaCtx map[string]interface{}) error {
	orderID, ok := sagaCtx["order_id"].(string)
	if !ok || orderID == "" {
		klog.Warn("no order id to unmark paid")
		return nil
	}

	userID, _ := sagaCtx["user_id"].(int64)

	// 调用订单服务取消订单
	_, err := rpc.OrderClient.CancelOrder(ctx, &order.CancelOrderReq{
		UserId:  uint32(userID),
		OrderId: orderID,
	})

	if err != nil {
		klog.Errorf("failed to cancel order %s in unmark paid compensate: %v", orderID, err)
		return err
	}

	return nil
}
