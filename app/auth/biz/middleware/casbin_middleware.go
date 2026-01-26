package middleware

import (
	"context"
	"errors"
	"fmt"

	"github.com/casbin/casbin/v2"
	"github.com/cloudwego/kitex/pkg/endpoint"
	"github.com/cloudwego/kitex/pkg/klog"
	"github.com/xvxiaoman8/gomall/rpc_gen/kitex_gen/auth"
)

// CasbinMiddleware Casbin 权限中间件
func CasbinMiddleware(enforcer *casbin.Enforcer) endpoint.Middleware {
	return func(next endpoint.Endpoint) endpoint.Endpoint {
		return func(ctx context.Context, req, resp interface{}) error {
			// 判断是否为分发token请求（登录/注册）
			if _, isDeliver := req.(*auth.DeliverTokenReq); isDeliver {
				return next(ctx, req, resp)
			}

			// 从上下文中获取用户信息
			userID, _ := ctx.Value("user_id").(int64)
			userRole, _ := ctx.Value("user_role").(string)
			resource, _ := ctx.Value("resource").(string)
			action, _ := ctx.Value("action").(string)

			// 如果 resource 或 action 为空，尝试从 resourse 获取（兼容旧代码）
			if resource == "" {
				resource, _ = ctx.Value("resourse").(string)
			}

			if userRole == "" {
				userRole = "customer" // 默认角色
			}

			// 构建权限检查请求
			// 格式：sub (用户/角色), obj (资源), act (操作)
			ok, err := enforcer.Enforce(userRole, resource, action)
			if err != nil {
				klog.Errorf("casbin enforce error: %v", err)
				return fmt.Errorf("casbin enforce error: %w", err)
			}

			if !ok {
				// 检查是否是资源所有者权限（动态策略）
				if userID > 0 {
					// 尝试检查资源所有者权限
					// 格式：role, resource, action, owner
					ok, err = enforcer.Enforce(userRole, resource, action, fmt.Sprintf("user:%d", userID))
					if err == nil && ok {
						return next(ctx, req, resp)
					}
				}

				klog.Warnf("access denied: user_role=%s, resource=%s, action=%s, user_id=%d", userRole, resource, action, userID)
				return errors.New("access denied: insufficient permissions")
			}

			return next(ctx, req, resp)
		}
	}
}
