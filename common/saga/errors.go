package saga

import "errors"

var (
	// ErrStepFailed 步骤执行失败
	ErrStepFailed = errors.New("saga step failed")
	// ErrSagaNotFound Saga 实例未找到
	ErrSagaNotFound = errors.New("saga instance not found")
	// ErrSagaAlreadyCompleted Saga 已完成
	ErrSagaAlreadyCompleted = errors.New("saga already completed")
)
