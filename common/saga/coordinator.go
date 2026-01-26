package saga

import (
	"context"
	"fmt"
	"time"

	"github.com/cloudwego/kitex/pkg/klog"
)

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
		instance.UpdatedAt = time.Now()
		
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
	
	// 使用步骤的重试策略，如果没有则使用默认策略
	retryPolicy := step.RetryPolicy
	if retryPolicy.MaxRetries == 0 {
		retryPolicy = DefaultRetryPolicy()
	}
	
	var lastErr error
	for i := 0; i < retryPolicy.MaxRetries; i++ {
		if i > 0 {
			// 计算延迟时间（指数退避）
			delay := time.Duration(float64(retryPolicy.InitialDelay) * retryPolicy.Multiplier * float64(i))
			if delay > retryPolicy.MaxDelay {
				delay = retryPolicy.MaxDelay
			}
			time.Sleep(delay)
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
		if i >= len(instance.Steps) {
			continue
		}
		
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
