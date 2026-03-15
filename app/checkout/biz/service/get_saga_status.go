package service

import (
	"context"
	"fmt"

	"github.com/xvxiaoman8/gomall/common/saga"
	checkout "github.com/xvxiaoman8/gomall/rpc_gen/kitex_gen/checkout"
)

type GetSagaStatusService struct {
	ctx context.Context
}

// NewGetSagaStatusService new GetSagaStatusService
func NewGetSagaStatusService(ctx context.Context) *GetSagaStatusService {
	return &GetSagaStatusService{ctx: ctx}
}

// Run returns the status of a Saga instance
func (s *GetSagaStatusService) Run(req *checkout.GetSagaStatusReq) (resp *checkout.GetSagaStatusResp, err error) {
	if req.SagaId == "" {
		return nil, fmt.Errorf("saga_id cannot be empty")
	}

	if sagaStorage == nil {
		return nil, fmt.Errorf("saga storage not initialized")
	}

	instance, err := sagaStorage.Get(s.ctx, req.SagaId)
	if err != nil {
		return nil, fmt.Errorf("failed to get saga instance: %w", err)
	}

	// 转换日志
	var logs []*checkout.SagaLog
	for _, log := range instance.Logs {
		logs = append(logs, &checkout.SagaLog{
			Step:      log.Step,
			Action:    log.Action,
			Status:    log.Status,
			Error:     log.Error,
			Timestamp: log.Timestamp.Format("2006-01-02 15:04:05"),
		})
	}

	return &checkout.GetSagaStatusResp{
		SagaId:       instance.ID,
		Status:       string(instance.Status),
		CurrentStep:  int32(instance.CurrentStep),
		Logs:         logs,
		CreatedAt:    instance.CreatedAt.Format("2006-01-02 15:04:05"),
		UpdatedAt:    instance.UpdatedAt.Format("2006-01-02 15:04:05"),
	}, nil
}

// ListSagaStatus lists saga instances by status
func ListSagaStatus(ctx context.Context, status saga.SagaStatus, limit int) ([]*checkout.GetSagaStatusResp, error) {
	if sagaStorage == nil {
		return nil, fmt.Errorf("saga storage not initialized")
	}

	instances, err := sagaStorage.List(ctx, status, limit)
	if err != nil {
		return nil, fmt.Errorf("failed to list saga instances: %w", err)
	}

	var results []*checkout.GetSagaStatusResp
	for _, instance := range instances {
		var logs []*checkout.SagaLog
		for _, log := range instance.Logs {
			logs = append(logs, &checkout.SagaLog{
				Step:      log.Step,
				Action:    log.Action,
				Status:    log.Status,
				Error:     log.Error,
				Timestamp: log.Timestamp.Format("2006-01-02 15:04:05"),
			})
		}

		results = append(results, &checkout.GetSagaStatusResp{
			SagaId:      instance.ID,
			Status:      string(instance.Status),
			CurrentStep: int32(instance.CurrentStep),
			Logs:        logs,
			CreatedAt:   instance.CreatedAt.Format("2006-01-02 15:04:05"),
			UpdatedAt:   instance.UpdatedAt.Format("2006-01-02 15:04:05"),
		})
	}

	return results, nil
}
