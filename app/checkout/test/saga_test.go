package test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/xvxiaoman8/gomall/app/checkout/biz/dal/redis"
	checkoutsaga "github.com/xvxiaoman8/gomall/app/checkout/biz/saga"
	"github.com/xvxiaoman8/gomall/common/saga"
	checkout "github.com/xvxiaoman8/gomall/rpc_gen/kitex_gen/checkout"
)

// TestSagaCoordinator 测试 Saga 协调器基本功能
func TestSagaCoordinator(t *testing.T) {
	if redis.RedisClient == nil {
		t.Skip("Redis not initialized, skipping test")
	}
	
	ctx := context.Background()
	
	// 创建存储
	storage := saga.NewRedisSagaStorage(redis.RedisClient)
	coordinator := saga.NewCoordinator(storage)

	// 创建测试步骤
	steps := []saga.SagaStep{
		{
			Name:    "step1",
			Service: "test",
			Action: func(ctx context.Context, sagaCtx map[string]interface{}) error {
				sagaCtx["step1"] = "completed"
				return nil
			},
			Compensate: func(ctx context.Context, sagaCtx map[string]interface{}) error {
				delete(sagaCtx, "step1")
				return nil
			},
		},
		{
			Name:    "step2",
			Service: "test",
			Action: func(ctx context.Context, sagaCtx map[string]interface{}) error {
				sagaCtx["step2"] = "completed"
				return nil
			},
			Compensate: func(ctx context.Context, sagaCtx map[string]interface{}) error {
				delete(sagaCtx, "step2")
				return nil
			},
		},
	}

	sagaCtx := map[string]interface{}{
		"test": "value",
	}

	// 执行 Saga
	sagaID := uuid.New().String()
	err := coordinator.Execute(ctx, sagaID, steps, sagaCtx)
	if err != nil {
		t.Fatalf("Saga execution failed: %v", err)
	}

	// 验证结果
	if sagaCtx["step1"] != "completed" {
		t.Error("step1 not completed")
	}
	if sagaCtx["step2"] != "completed" {
		t.Error("step2 not completed")
	}

	// 验证实例已保存
	instance, err := storage.Get(ctx, sagaID)
	if err != nil {
		t.Fatalf("Failed to get saga instance: %v", err)
	}

	if instance.Status != saga.SagaStatusCompleted {
		t.Errorf("Expected status completed, got %s", instance.Status)
	}

	t.Logf("Saga test passed: %s", sagaID)
}

// TestSagaCompensation 测试 Saga 补偿功能
func TestSagaCompensation(t *testing.T) {
	ctx := context.Background()
	
	// 初始化 Redis 连接
	redis.Init()
	
	storage := saga.NewRedisSagaStorage(redis.RedisClient)
	coordinator := saga.NewCoordinator(storage)

	// 创建会失败的步骤
	steps := []saga.SagaStep{
		{
			Name:    "step1",
			Service: "test",
			Action: func(ctx context.Context, sagaCtx map[string]interface{}) error {
				sagaCtx["step1"] = "completed"
				return nil
			},
			Compensate: func(ctx context.Context, sagaCtx map[string]interface{}) error {
				delete(sagaCtx, "step1")
				return nil
			},
		},
		{
			Name:    "step2_fail",
			Service: "test",
			Action: func(ctx context.Context, sagaCtx map[string]interface{}) error {
				return errors.New("step failed") // 模拟失败
			},
			Compensate: func(ctx context.Context, sagaCtx map[string]interface{}) error {
				// 补偿逻辑
				return nil
			},
		},
	}

	sagaCtx := map[string]interface{}{}

	// 执行 Saga（应该失败并触发补偿）
	sagaID := uuid.New().String()
	err := coordinator.Execute(ctx, sagaID, steps, sagaCtx)
	if err == nil {
		t.Fatal("Expected saga to fail")
	}

	// 验证 step1 已被补偿（从上下文中删除）
	if _, ok := sagaCtx["step1"]; ok {
		t.Error("step1 should be compensated (removed from context)")
	}

	// 验证实例状态
	instance, err := storage.Get(ctx, sagaID)
	if err != nil {
		t.Fatalf("Failed to get saga instance: %v", err)
	}

	if instance.Status != saga.SagaStatusFailed {
		t.Errorf("Expected status failed, got %s", instance.Status)
	}

	t.Logf("Saga compensation test passed: %s", sagaID)
}

// TestCheckoutSagaBuild 测试 Checkout Saga 构建
func TestCheckoutSagaBuild(t *testing.T) {
	ctx := context.Background()

	req := &checkout.CheckoutReq{
		UserId: 1,
		Email:   "test@example.com",
		CreditCard: &checkout.CreditCardInfo{
			CreditCardNumber:          "1234567890123456",
			CreditCardExpirationYear:  2025,
			CreditCardExpirationMonth: 12,
			CreditCardCvv:             123,
		},
	}

	steps, sagaCtx := checkoutsaga.BuildCheckoutSaga(ctx, req)

	if len(steps) == 0 {
		t.Fatal("No steps created")
	}

	if len(steps) != 5 {
		t.Errorf("Expected 5 steps, got %d", len(steps))
	}

	// 验证步骤名称
	expectedSteps := []string{"decrease_stock", "create_order", "empty_cart", "charge_payment", "mark_order_paid"}
	for i, step := range steps {
		if step.Name != expectedSteps[i] {
			t.Errorf("Step %d: expected %s, got %s", i, expectedSteps[i], step.Name)
		}
	}

	// 验证上下文
	if sagaCtx["user_id"] != req.UserId {
		t.Error("user_id not set in context")
	}

	t.Log("Checkout Saga build test passed")
}

// TestSagaRetry 测试 Saga 重试机制
func TestSagaRetry(t *testing.T) {
	if redis.RedisClient == nil {
		t.Skip("Redis not initialized, skipping test")
	}
	
	ctx := context.Background()
	
	storage := saga.NewRedisSagaStorage(redis.RedisClient)
	coordinator := saga.NewCoordinator(storage)

	retryCount := 0
	maxRetries := 3

	steps := []saga.SagaStep{
		{
			Name:    "retry_step",
			Service: "test",
			Action: func(ctx context.Context, sagaCtx map[string]interface{}) error {
				retryCount++
				if retryCount < maxRetries {
					return errors.New("retry needed") // 前两次失败
				}
				return nil // 第三次成功
			},
			Compensate: nil,
			RetryPolicy: saga.RetryPolicy{
				MaxRetries:  maxRetries,
				InitialDelay: 10 * time.Millisecond,
				MaxDelay:     100 * time.Millisecond,
				Multiplier:   2.0,
			},
		},
	}

	sagaCtx := map[string]interface{}{}
	sagaID := uuid.New().String()

	err := coordinator.Execute(ctx, sagaID, steps, sagaCtx)
	if err != nil {
		t.Fatalf("Saga execution failed: %v", err)
	}

	if retryCount != maxRetries {
		t.Errorf("Expected %d retries, got %d", maxRetries, retryCount)
	}

	t.Logf("Saga retry test passed: retried %d times", retryCount)
}
