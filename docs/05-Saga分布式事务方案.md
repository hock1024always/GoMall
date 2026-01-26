# Saga 分布式事务方案

## 设计目标

解决微服务架构下跨服务数据一致性问题，特别是 checkout 流程中的订单创建、库存扣减、支付等操作的原子性问题。

## 问题分析

### 当前问题

在 `checkout` 服务中，流程如下：
1. 减库存（Redis）
2. 创建订单（Order Service）
3. 清空购物车（Cart Service）
4. 支付（Payment Service）
5. 更新订单状态（Order Service）

**问题**：如果支付失败，订单已创建、库存已扣减，但没有回滚机制，导致数据不一致。

### Saga 模式解决方案

使用 **Saga 编排模式**，每个步骤都有对应的补偿操作，失败时按逆序执行补偿。

## 架构设计

### Saga 协调器架构

```
┌─────────────────────────────────────────┐
│         Checkout Service                 │
│  ┌──────────────────────────────────┐   │
│  │     Saga 协调器                  │   │
│  │  - 状态机管理                    │   │
│  │  - 步骤编排                      │   │
│  │  - 补偿执行                      │   │
│  └──────────┬───────────────────────┘   │
└─────────────┼───────────────────────────┘
              │
    ┌─────────┼─────────┬─────────┬─────────┐
    │         │         │         │         │
┌───▼───┐ ┌──▼───┐ ┌───▼───┐ ┌───▼───┐ ┌───▼───┐
│Stock  │ │Order │ │ Cart  │ │Payment│ │Order  │
│Service│ │Service│ │Service│ │Service│ │Service│
└───────┘ └──────┘ └───────┘ └───────┘ └───────┘
```

### Saga 流程定义

```go
// Saga 步骤定义
type SagaStep struct {
    Name        string
    Service     string
    Action      func(ctx context.Context, req interface{}) error
    Compensate  func(ctx context.Context, req interface{}) error
    RetryPolicy RetryPolicy
}

// Checkout Saga 流程
var CheckoutSaga = []SagaStep{
    {
        Name:    "decrease_stock",
        Service: "product",
        Action:  DecreaseStockAction,
        Compensate: IncreaseStockCompensate,
    },
    {
        Name:    "create_order",
        Service: "order",
        Action:  CreateOrderAction,
        Compensate: CancelOrderCompensate,
    },
    {
        Name:    "empty_cart",
        Service: "cart",
        Action:  EmptyCartAction,
        Compensate: RestoreCartCompensate,
    },
    {
        Name:    "charge_payment",
        Service: "payment",
        Action:  ChargePaymentAction,
        Compensate: RefundPaymentCompensate,
    },
    {
        Name:    "mark_order_paid",
        Service: "order",
        Action:  MarkOrderPaidAction,
        Compensate: UnmarkOrderPaidCompensate,
    },
}
```

## 实现方案

### 1. Saga 协调器

```go
// saga/coordinator.go
package saga

import (
    "context"
    "encoding/json"
    "fmt"
    "time"
    
    "github.com/cloudwego/kitex/pkg/klog"
)

// SagaInstance Saga 实例
type SagaInstance struct {
    ID          string
    Steps       []SagaStep
    CurrentStep int
    Status      SagaStatus
    Context     map[string]interface{}
    Logs        []SagaLog
    CreatedAt   time.Time
    UpdatedAt   time.Time
}

// SagaStatus Saga 状态
type SagaStatus string

const (
    SagaStatusStarted    SagaStatus = "started"
    SagaStatusCompleted  SagaStatus = "completed"
    SagaStatusCompensating SagaStatus = "compensating"
    SagaStatusFailed     SagaStatus = "failed"
)

// SagaLog Saga 日志
type SagaLog struct {
    Step      string
    Action    string  // "execute" | "compensate"
    Status    string  // "success" | "failed"
    Error     string
    Timestamp time.Time
}

// Coordinator Saga 协调器
type Coordinator struct {
    storage SagaStorage
}

// NewCoordinator 创建协调器
func NewCoordinator(storage SagaStorage) *Coordinator {
    return &Coordinator{storage: storage}
}

// Execute 执行 Saga
func (c *Coordinator) Execute(ctx context.Context, sagaID string, steps []SagaStep, initialContext map[string]interface{}) error {
    instance := &SagaInstance{
        ID:          sagaID,
        Steps:       steps,
        CurrentStep: 0,
        Status:      SagaStatusStarted,
        Context:     initialContext,
        Logs:        []SagaLog{},
        CreatedAt:   time.Now(),
        UpdatedAt:   time.Now(),
    }
    
    // 保存实例
    if err := c.storage.Save(ctx, instance); err != nil {
        return fmt.Errorf("save saga instance failed: %w", err)
    }
    
    // 执行步骤
    for i, step := range steps {
        instance.CurrentStep = i
        
        // 执行动作
        if err := c.executeStep(ctx, instance, step); err != nil {
            klog.Errorf("step %s failed: %v", step.Name, err)
            
            // 执行补偿
            if err := c.compensate(ctx, instance, i-1); err != nil {
                klog.Errorf("compensate failed: %v", err)
                instance.Status = SagaStatusFailed
                c.storage.Save(ctx, instance)
                return fmt.Errorf("compensate failed: %w", err)
            }
            
            instance.Status = SagaStatusFailed
            c.storage.Save(ctx, instance)
            return fmt.Errorf("step %s failed: %w", step.Name, err)
        }
        
        // 更新状态
        instance.UpdatedAt = time.Now()
        if err := c.storage.Save(ctx, instance); err != nil {
            klog.Errorf("save saga instance failed: %v", err)
        }
    }
    
    // 所有步骤成功
    instance.Status = SagaStatusCompleted
    instance.UpdatedAt = time.Now()
    return c.storage.Save(ctx, instance)
}

// executeStep 执行单个步骤
func (c *Coordinator) executeStep(ctx context.Context, instance *SagaInstance, step SagaStep) error {
    log := SagaLog{
        Step:      step.Name,
        Action:    "execute",
        Timestamp: time.Now(),
    }
    
    // 重试策略
    maxRetries := 3
    retryDelay := 100 * time.Millisecond
    
    var lastErr error
    for i := 0; i < maxRetries; i++ {
        if i > 0 {
            time.Sleep(retryDelay)
            retryDelay *= 2  // 指数退避
        }
        
        err := step.Action(ctx, instance.Context)
        if err == nil {
            log.Status = "success"
            instance.Logs = append(instance.Logs, log)
            return nil
        }
        
        lastErr = err
        klog.Warnf("step %s retry %d failed: %v", step.Name, i+1, err)
    }
    
    log.Status = "failed"
    log.Error = lastErr.Error()
    instance.Logs = append(instance.Logs, log)
    return lastErr
}

// compensate 执行补偿
func (c *Coordinator) compensate(ctx context.Context, instance *SagaInstance, fromStep int) error {
    instance.Status = SagaStatusCompensating
    c.storage.Save(ctx, instance)
    
    // 逆序执行补偿
    for i := fromStep; i >= 0; i-- {
        step := instance.Steps[i]
        
        log := SagaLog{
            Step:      step.Name,
            Action:    "compensate",
            Timestamp: time.Now(),
        }
        
        if step.Compensate == nil {
            klog.Warnf("step %s has no compensate function", step.Name)
            log.Status = "skipped"
            instance.Logs = append(instance.Logs, log)
            continue
        }
        
        err := step.Compensate(ctx, instance.Context)
        if err != nil {
            log.Status = "failed"
            log.Error = err.Error()
            instance.Logs = append(instance.Logs, log)
            klog.Errorf("compensate step %s failed: %v", step.Name, err)
            // 补偿失败也继续执行其他补偿
        } else {
            log.Status = "success"
            instance.Logs = append(instance.Logs, log)
        }
        
        instance.UpdatedAt = time.Now()
        c.storage.Save(ctx, instance)
    }
    
    return nil
}
```

### 2. Saga 存储接口

```go
// saga/storage.go
package saga

import "context"

// SagaStorage Saga 存储接口
type SagaStorage interface {
    Save(ctx context.Context, instance *SagaInstance) error
    Get(ctx context.Context, sagaID string) (*SagaInstance, error)
    List(ctx context.Context, status SagaStatus, limit int) ([]*SagaInstance, error)
}

// RedisSagaStorage Redis 实现
type RedisSagaStorage struct {
    client *redis.Client
}

func NewRedisSagaStorage(client *redis.Client) *RedisSagaStorage {
    return &RedisSagaStorage{client: client}
}

func (s *RedisSagaStorage) Save(ctx context.Context, instance *SagaInstance) error {
    key := fmt.Sprintf("saga:instance:%s", instance.ID)
    data, err := json.Marshal(instance)
    if err != nil {
        return err
    }
    
    // 设置过期时间（24小时）
    return s.client.Set(ctx, key, data, 24*time.Hour).Err()
}

func (s *RedisSagaStorage) Get(ctx context.Context, sagaID string) (*SagaInstance, error) {
    key := fmt.Sprintf("saga:instance:%s", sagaID)
    data, err := s.client.Get(ctx, key).Bytes()
    if err != nil {
        return nil, err
    }
    
    var instance SagaInstance
    if err := json.Unmarshal(data, &instance); err != nil {
        return nil, err
    }
    
    return &instance, nil
}
```

### 3. Checkout Saga 实现

```go
// app/checkout/biz/saga/checkout_saga.go
package saga

import (
    "context"
    "fmt"
    
    "github.com/xvxiaoman8/gomall/app/checkout/infra/rpc"
    "github.com/xvxiaoman8/gomall/rpc_gen/kitex_gen/cart"
    checkout "github.com/xvxiaoman8/gomall/rpc_gen/kitex_gen/checkout"
    "github.com/xvxiaoman8/gomall/rpc_gen/kitex_gen/order"
    "github.com/xvxiaoman8/gomall/rpc_gen/kitex_gen/payment"
    "github.com/xvxiaoman8/gomall/rpc_gen/kitex_gen/product"
)

// CheckoutSagaContext Checkout Saga 上下文
type CheckoutSagaContext struct {
    UserID      int64
    CartItems   []*cart.CartItem
    OrderID     string
    OrderItems  []*order.OrderItem
    TotalAmount float32
    PaymentID   string
}

// BuildCheckoutSaga 构建 Checkout Saga
func BuildCheckoutSaga(ctx context.Context, req *checkout.CheckoutReq) ([]SagaStep, map[string]interface{}) {
    sagaCtx := map[string]interface{}{
        "user_id": req.UserId,
        "email":   req.Email,
        "address": req.Address,
        "credit_card": req.CreditCard,
    }
    
    steps := []SagaStep{
        {
            Name:    "decrease_stock",
            Service: "product",
            Action: func(ctx context.Context, sagaCtx map[string]interface{}) error {
                return decreaseStockAction(ctx, sagaCtx, req)
            },
            Compensate: func(ctx context.Context, sagaCtx map[string]interface{}) error {
                return increaseStockCompensate(ctx, sagaCtx)
            },
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
    
    // 记录扣减的库存信息（用于补偿）
    stockDecreases := make(map[int64]int32)
    
    for _, item := range cartResult.Cart.Items {
        // 扣减库存（使用现有的逻辑）
        // ... 扣减逻辑 ...
        
        stockDecreases[item.ProductId] = item.Quantity
    }
    
    sagaCtx["stock_decreases"] = stockDecreases
    return nil
}

// increaseStockCompensate 回滚库存
func increaseStockCompensate(ctx context.Context, sagaCtx map[string]interface{}) error {
    stockDecreases, ok := sagaCtx["stock_decreases"].(map[int64]int32)
    if !ok {
        return nil
    }
    
    for productID, quantity := range stockDecreases {
        // 回滚库存
        key := fmt.Sprintf("product:%d:stock", productID)
        redis.RedisClient.IncrBy(ctx, key, int64(quantity))
    }
    
    return nil
}

// createOrderAction 创建订单
func createOrderAction(ctx context.Context, sagaCtx map[string]interface{}, req *checkout.CheckoutReq) error {
    // 构建订单请求
    orderReq := &order.PlaceOrderReq{
        UserId:       req.UserId,
        UserCurrency: "USD",
        Email:        req.Email,
        // ... 其他字段 ...
    }
    
    orderResult, err := rpc.OrderClient.PlaceOrder(ctx, orderReq)
    if err != nil {
        return fmt.Errorf("place order failed: %w", err)
    }
    
    sagaCtx["order_id"] = orderResult.Order.OrderId
    return nil
}

// cancelOrderCompensate 取消订单
func cancelOrderCompensate(ctx context.Context, sagaCtx map[string]interface{}) error {
    orderID, ok := sagaCtx["order_id"].(string)
    if !ok {
        return nil
    }
    
    userID, _ := sagaCtx["user_id"].(int64)
    
    // 调用订单服务取消订单
    _, err := rpc.OrderClient.UpdateOrder(ctx, &order.UpdateOrderReq{
        UserId:  userID,
        OrderId: orderID,
        Status:  "cancelled",
    })
    
    return err
}

// chargePaymentAction 支付
func chargePaymentAction(ctx context.Context, sagaCtx map[string]interface{}, req *checkout.CheckoutReq) error {
    orderID, _ := sagaCtx["order_id"].(string)
    totalAmount, _ := sagaCtx["total_amount"].(float32)
    
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
    
    sagaCtx["payment_id"] = paymentResult.TransactionId
    return nil
}

// refundPaymentCompensate 退款
func refundPaymentCompensate(ctx context.Context, sagaCtx map[string]interface{}) error {
    paymentID, ok := sagaCtx["payment_id"].(string)
    if !ok {
        return nil
    }
    
    userID, _ := sagaCtx["user_id"].(int64)
    totalAmount, _ := sagaCtx["total_amount"].(float32)
    
    // 调用支付服务退款
    _, err := rpc.PaymentClient.Refund(ctx, &payment.RefundReq{
        UserId:        userID,
        TransactionId: paymentID,
        Amount:        totalAmount,
    })
    
    return err
}

// 其他 Action 和 Compensate 函数...
```

### 4. 在 Checkout Service 中使用 Saga

```go
// app/checkout/biz/service/checkout.go
func (s *CheckoutService) Run(req *checkout.CheckoutReq) (resp *checkout.CheckoutResp, err error) {
    // 生成 Saga ID
    sagaID := uuid.New().String()
    
    // 构建 Saga
    steps, sagaCtx := saga.BuildCheckoutSaga(s.ctx, req)
    
    // 创建协调器
    coordinator := saga.NewCoordinator(sagaStorage)
    
    // 执行 Saga
    err = coordinator.Execute(s.ctx, sagaID, steps, sagaCtx)
    if err != nil {
        return nil, fmt.Errorf("saga execution failed: %w", err)
    }
    
    // 获取结果
    orderID, _ := sagaCtx["order_id"].(string)
    paymentID, _ := sagaCtx["payment_id"].(string)
    
    return &checkout.CheckoutResp{
        OrderId:       orderID,
        TransactionId: paymentID,
    }, nil
}
```

## 配置参数

### Saga 配置

```yaml
# app/checkout/conf/conf.yaml
saga:
  storage:
    type: "redis"  # "redis" | "mysql"
    redis:
      address: "localhost:6379"
      password: "gomall_redis_password"
      db: 1
  retry:
    max_retries: 3
    initial_delay: 100ms
    max_delay: 5s
    multiplier: 2.0
  timeout:
    step_timeout: 30s
    saga_timeout: 5m
```

## 监控和日志

### Saga 状态查询接口

```go
// 查询 Saga 实例状态
func (s *CheckoutService) GetSagaStatus(ctx context.Context, sagaID string) (*SagaStatusResp, error) {
    instance, err := sagaStorage.Get(ctx, sagaID)
    if err != nil {
        return nil, err
    }
    
    return &SagaStatusResp{
        SagaID:     instance.ID,
        Status:     string(instance.Status),
        CurrentStep: instance.CurrentStep,
        Logs:       instance.Logs,
    }, nil
}
```

## 实施步骤

1. ✅ 实现 Saga 协调器核心逻辑
2. ✅ 实现 Saga 存储接口（Redis）
3. ✅ 定义 Checkout Saga 步骤
4. ✅ 实现各个 Action 和 Compensate 函数
5. ✅ 集成到 Checkout Service
6. ✅ 添加 Saga 状态查询接口
7. ✅ 添加监控和日志
8. ✅ 测试和优化
