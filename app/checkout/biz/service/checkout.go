package service

import (
	"context"
	"fmt"

	"github.com/cloudwego/kitex/pkg/klog"
	"github.com/google/uuid"
	"github.com/xvxiaoman8/gomall/app/checkout/biz/dal/redis"
	checkoutsaga "github.com/xvxiaoman8/gomall/app/checkout/biz/saga"
	"github.com/xvxiaoman8/gomall/common/saga"
	checkout "github.com/xvxiaoman8/gomall/rpc_gen/kitex_gen/checkout"
)

var (
	sagaCoordinator *saga.Coordinator
	sagaStorage     saga.SagaStorage
)

// InitSaga 初始化 Saga
func InitSaga() {
	// 初始化 Saga 存储
	sagaStorage = saga.NewRedisSagaStorage(redis.RedisClient)
	// 创建 Saga 协调器
	sagaCoordinator = saga.NewCoordinator(sagaStorage)
}

type CheckoutService struct {
	ctx context.Context
} // NewCheckoutService new CheckoutService
func NewCheckoutService(ctx context.Context) *CheckoutService {
	return &CheckoutService{ctx: ctx}
}

// Run create note info
func (s *CheckoutService) Run(req *checkout.CheckoutReq) (resp *checkout.CheckoutResp, err error) {
	// 生成 Saga ID
	sagaID := uuid.New().String()
	klog.Infof("Starting checkout saga: %s", sagaID)

	// 构建 Saga
	steps, sagaCtx := checkoutsaga.BuildCheckoutSaga(s.ctx, req)

	// 执行 Saga
	err = sagaCoordinator.Execute(s.ctx, sagaID, steps, sagaCtx)
	if err != nil {
		klog.Errorf("Saga execution failed: %v", err)
		return nil, fmt.Errorf("checkout failed: %w", err)
	}

	// 获取结果
	orderID, _ := sagaCtx["order_id"].(string)
	paymentID, _ := sagaCtx["payment_id"].(string)

	klog.Infof("Checkout saga completed successfully: %s, order: %s, payment: %s", sagaID, orderID, paymentID)

	return &checkout.CheckoutResp{
		OrderId:       orderID,
		TransactionId: paymentID,
		SagaId:        sagaID,
	}, nil
}

// InitStock 初始化库存（保留用于测试）
func InitStock(ctx context.Context, ID int, amount int) error {
	_, err := redis.RedisDo(ctx, "SET", fmt.Sprintf("%d_stock", ID), amount)
	return err
}
