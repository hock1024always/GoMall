package saga

import (
	"context"
	"time"
)

// SagaStatus Saga 状态
type SagaStatus string

const (
	SagaStatusStarted     SagaStatus = "started"
	SagaStatusCompleted   SagaStatus = "completed"
	SagaStatusCompensating SagaStatus = "compensating"
	SagaStatusFailed      SagaStatus = "failed"
)

// SagaLog Saga 日志
type SagaLog struct {
	Step      string    `json:"step"`
	Action    string    `json:"action"` // "execute" | "compensate"
	Status    string    `json:"status"` // "success" | "failed"
	Error     string    `json:"error,omitempty"`
	Timestamp time.Time `json:"timestamp"`
}

// SagaInstance Saga 实例
type SagaInstance struct {
	ID          string                 `json:"id"`
	Steps       []SagaStep             `json:"-"`
	CurrentStep int                    `json:"current_step"`
	Status      SagaStatus             `json:"status"`
	Context     map[string]interface{} `json:"context"`
	Logs        []SagaLog             `json:"logs"`
	CreatedAt   time.Time              `json:"created_at"`
	UpdatedAt   time.Time              `json:"updated_at"`
}

// RetryPolicy 重试策略
type RetryPolicy struct {
	MaxRetries  int           `json:"max_retries"`
	InitialDelay time.Duration `json:"initial_delay"`
	MaxDelay     time.Duration `json:"max_delay"`
	Multiplier   float64      `json:"multiplier"`
}

// DefaultRetryPolicy 默认重试策略
func DefaultRetryPolicy() RetryPolicy {
	return RetryPolicy{
		MaxRetries:  3,
		InitialDelay: 100 * time.Millisecond,
		MaxDelay:     5 * time.Second,
		Multiplier:   2.0,
	}
}

// SagaStep Saga 步骤
type SagaStep struct {
	Name        string
	Service     string
	Action      func(ctx context.Context, sagaCtx map[string]interface{}) error
	Compensate  func(ctx context.Context, sagaCtx map[string]interface{}) error
	RetryPolicy RetryPolicy
}
