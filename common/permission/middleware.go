package permission

import (
	"context"

	"github.com/cloudwego/kitex/pkg/endpoint"
	"github.com/cloudwego/kitex/pkg/klog"
)

// Middleware 权限检查中间件
func Middleware(pm *PermissionManager) endpoint.Middleware {
	return func(next endpoint.Endpoint) endpoint.Endpoint {
		return func(ctx context.Context, req, resp interface{}) error {
			// 获取权限信息
			info := GetPermissionInfoFromContext(ctx)

			// 如果没有设置资源或操作，跳过权限检查
			if info.Resource == "" || info.Action == "" {
				return next(ctx, req, resp)
			}

			// 检查权限
			if err := pm.CheckPermission(ctx, info.UserID, info.Role, info.Resource, info.Action); err != nil {
				return err
			}

			return next(ctx, req, resp)
		}
	}
}

// ResourceAction 设置资源和操作的辅助函数
func ResourceAction(resource, action string) endpoint.Middleware {
	return func(next endpoint.Endpoint) endpoint.Endpoint {
		return func(ctx context.Context, req, resp interface{}) error {
			// 设置资源和操作到上下文
			ctx = context.WithValue(ctx, ResourceKey, resource)
			ctx = context.WithValue(ctx, ActionKey, action)
			return next(ctx, req, resp)
		}
	}
}

// ChainMiddleware 链式中间件
func ChainMiddleware(middlewares ...endpoint.Middleware) endpoint.Middleware {
	return func(final endpoint.Endpoint) endpoint.Endpoint {
		for i := len(middlewares) - 1; i >= 0; i-- {
			final = middlewares[i](final)
		}
		return final
	}
}

// LogMiddleware 日志中间件
func LogMiddleware() endpoint.Middleware {
	return func(next endpoint.Endpoint) endpoint.Endpoint {
		return func(ctx context.Context, req, resp interface{}) error {
			info := GetPermissionInfoFromContext(ctx)
			klog.Infof("Permission check: user_id=%d, role=%s, resource=%s, action=%s",
				info.UserID, info.Role, info.Resource, info.Action)
			return next(ctx, req, resp)
		}
	}
}
