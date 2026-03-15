package permission

import (
	"context"
)

// contextKey 上下文键类型
type contextKey string

const (
	// UserIDKey 用户ID上下文键
	UserIDKey contextKey = "user_id"
	// UserRoleKey 用户角色上下文键
	UserRoleKey contextKey = "user_role"
	// ResourceKey 资源上下文键
	ResourceKey contextKey = "resource"
	// ActionKey 操作上下文键
	ActionKey contextKey = "action"
)

// PermissionInfo 权限信息
type PermissionInfo struct {
	UserID   int64
	Role     string
	Resource string
	Action   string
}

// NewContext 创建带有权限信息的上下文
func NewContext(ctx context.Context, info *PermissionInfo) context.Context {
	ctx = context.WithValue(ctx, UserIDKey, info.UserID)
	ctx = context.WithValue(ctx, UserRoleKey, info.Role)
	ctx = context.WithValue(ctx, ResourceKey, info.Resource)
	ctx = context.WithValue(ctx, ActionKey, info.Action)
	return ctx
}

// GetUserIDFromContext 从上下文获取用户ID
func GetUserIDFromContext(ctx context.Context) int64 {
	if v, ok := ctx.Value(UserIDKey).(int64); ok {
		return v
	}
	return 0
}

// GetRoleFromContext 从上下文获取用户角色
func GetRoleFromContext(ctx context.Context) string {
	if v, ok := ctx.Value(UserRoleKey).(string); ok {
		return v
	}
	return "customer"
}

// GetResourceFromContext 从上下文获取资源
func GetResourceFromContext(ctx context.Context) string {
	if v, ok := ctx.Value(ResourceKey).(string); ok {
		return v
	}
	return ""
}

// GetActionFromContext 从上下文获取操作
func GetActionFromContext(ctx context.Context) string {
	if v, ok := ctx.Value(ActionKey).(string); ok {
		return v
	}
	return ""
}

// GetPermissionInfoFromContext 从上下文获取完整权限信息
func GetPermissionInfoFromContext(ctx context.Context) *PermissionInfo {
	return &PermissionInfo{
		UserID:   GetUserIDFromContext(ctx),
		Role:     GetRoleFromContext(ctx),
		Resource: GetResourceFromContext(ctx),
		Action:   GetActionFromContext(ctx),
	}
}
