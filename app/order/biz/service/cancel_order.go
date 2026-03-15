// Copyright 2024 CloudWeGo Authors
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package service

import (
	"context"
	"fmt"

	"github.com/cloudwego/kitex/pkg/klog"
	"github.com/xvxiaoman8/gomall/app/order/biz/dal/mysql"
	"github.com/xvxiaoman8/gomall/app/order/biz/model"
	order "github.com/xvxiaoman8/gomall/rpc_gen/kitex_gen/order"
)

type CancelOrderService struct {
	ctx context.Context
}

// NewCancelOrderService new CancelOrderService
func NewCancelOrderService(ctx context.Context) *CancelOrderService {
	return &CancelOrderService{ctx: ctx}
}

// Run cancels an order (used for Saga compensation)
func (s *CancelOrderService) Run(req *order.CancelOrderReq) (resp *order.CancelOrderResp, err error) {
	if req.UserId == 0 || req.OrderId == "" {
		return nil, fmt.Errorf("user_id or order_id cannot be empty")
	}

	// 获取订单
	o, err := model.GetOrder(mysql.DB, s.ctx, req.UserId, req.OrderId)
	if err != nil {
		klog.Errorf("failed to get order: %v", err)
		return nil, fmt.Errorf("order not found: %w", err)
	}

	// 检查订单状态，只有placed或paid状态可以取消
	if o.OrderState != model.OrderStatePlaced && o.OrderState != model.OrderStatePaid {
		return &order.CancelOrderResp{
			Success: false,
			Message: fmt.Sprintf("order state is %s, cannot cancel", o.OrderState),
		}, nil
	}

	// 更新订单状态为canceled
	err = model.UpdateOrder(mysql.DB, s.ctx, req.UserId, req.OrderId, map[string]interface{}{
		"order_state": model.OrderStateCanceled,
	})
	if err != nil {
		klog.Errorf("failed to cancel order: %v", err)
		return nil, fmt.Errorf("failed to cancel order: %w", err)
	}

	return &order.CancelOrderResp{
		Success: true,
		Message: "order canceled successfully",
	}, nil
}
