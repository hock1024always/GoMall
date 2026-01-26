package saga

import "context"

// SagaStorage Saga 存储接口
type SagaStorage interface {
	Save(ctx context.Context, instance *SagaInstance) error
	Get(ctx context.Context, sagaID string) (*SagaInstance, error)
	List(ctx context.Context, status SagaStatus, limit int) ([]*SagaInstance, error)
}
