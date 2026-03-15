package permission

import (
	"context"
	"errors"
	"fmt"

	"github.com/casbin/casbin/v2"
	"github.com/cloudwego/kitex/pkg/klog"
)

// PermissionManager 权限管理器
type PermissionManager struct {
	enforcer *casbin.Enforcer
}

// NewPermissionManager 创建权限管理器
func NewPermissionManager(enforcer *casbin.Enforcer) *PermissionManager {
	return &PermissionManager{enforcer: enforcer}
}

// CheckPermission 检查权限
func (pm *PermissionManager) CheckPermission(ctx context.Context, userID int64, role, resource, action string) error {
	if role == "" {
		role = "customer"
	}

	// 检查基本权限
	ok, err := pm.enforcer.Enforce(role, resource, action)
	if err != nil {
		klog.Errorf("casbin enforce error: %v", err)
		return fmt.Errorf("permission check failed: %w", err)
	}

	if ok {
		return nil
	}

	// 检查资源所有者权限（动态策略）
	if userID > 0 {
		ok, err = pm.enforcer.Enforce(role, resource, action, fmt.Sprintf("user:%d", userID))
		if err == nil && ok {
			return nil
		}
	}

	klog.Warnf("access denied: user_id=%d, role=%s, resource=%s, action=%s", userID, role, resource, action)
	return errors.New("access denied: insufficient permissions")
}

// AddRoleForUser 为用户添加角色
func (pm *PermissionManager) AddRoleForUser(userID int64, role string) error {
	_, err := pm.enforcer.AddRoleForUser(fmt.Sprintf("user:%d", userID), role)
	if err != nil {
		return fmt.Errorf("failed to add role: %w", err)
	}
	return pm.enforcer.SavePolicy()
}

// DeleteRoleForUser 移除用户角色
func (pm *PermissionManager) DeleteRoleForUser(userID int64, role string) error {
	_, err := pm.enforcer.DeleteRoleForUser(fmt.Sprintf("user:%d", userID), role)
	if err != nil {
		return fmt.Errorf("failed to delete role: %w", err)
	}
	return pm.enforcer.SavePolicy()
}

// GetRolesForUser 获取用户角色
func (pm *PermissionManager) GetRolesForUser(userID int64) ([]string, error) {
	return pm.enforcer.GetRolesForUser(fmt.Sprintf("user:%d", userID))
}

// AddPolicy 添加策略
func (pm *PermissionManager) AddPolicy(role, resource, action string) error {
	_, err := pm.enforcer.AddPolicy(role, resource, action)
	if err != nil {
		return fmt.Errorf("failed to add policy: %w", err)
	}
	return pm.enforcer.SavePolicy()
}

// RemovePolicy 移除策略
func (pm *PermissionManager) RemovePolicy(role, resource, action string) error {
	_, err := pm.enforcer.RemovePolicy(role, resource, action)
	if err != nil {
		return fmt.Errorf("failed to remove policy: %w", err)
	}
	return pm.enforcer.SavePolicy()
}

// GetEnforcer 获取enforcer实例
func (pm *PermissionManager) GetEnforcer() *casbin.Enforcer {
	return pm.enforcer
}
