package service

import (
	"fmt"

	"github.com/casbin/casbin/v2"
	"github.com/cloudwego/kitex/pkg/klog"
)

// AddPolicy 添加权限策略
func AddPolicy(enforcer *casbin.Enforcer, role, resource, action string) error {
	added, err := enforcer.AddPolicy(role, resource, action)
	if err != nil {
		return fmt.Errorf("failed to add policy: %w", err)
	}

	if added {
		err = enforcer.SavePolicy()
		if err != nil {
			return fmt.Errorf("failed to save policy: %w", err)
		}
		klog.Infof("Added policy: role=%s, resource=%s, action=%s", role, resource, action)
	}

	return nil
}

// RemovePolicy 删除权限策略
func RemovePolicy(enforcer *casbin.Enforcer, role, resource, action string) error {
	removed, err := enforcer.RemovePolicy(role, resource, action)
	if err != nil {
		return fmt.Errorf("failed to remove policy: %w", err)
	}

	if removed {
		err = enforcer.SavePolicy()
		if err != nil {
			return fmt.Errorf("failed to save policy: %w", err)
		}
		klog.Infof("Removed policy: role=%s, resource=%s, action=%s", role, resource, action)
	}

	return nil
}

// AssignRole 分配用户角色
func AssignRole(enforcer *casbin.Enforcer, userID int64, role string) error {
	added, err := enforcer.AddGroupingPolicy(fmt.Sprintf("user:%d", userID), role)
	if err != nil {
		return fmt.Errorf("failed to assign role: %w", err)
	}

	if added {
		err = enforcer.SavePolicy()
		if err != nil {
			return fmt.Errorf("failed to save policy: %w", err)
		}
		klog.Infof("Assigned role: user_id=%d, role=%s", userID, role)
	}

	return nil
}

// RemoveRole 移除用户角色
func RemoveRole(enforcer *casbin.Enforcer, userID int64, role string) error {
	removed, err := enforcer.RemoveGroupingPolicy(fmt.Sprintf("user:%d", userID), role)
	if err != nil {
		return fmt.Errorf("failed to remove role: %w", err)
	}

	if removed {
		err = enforcer.SavePolicy()
		if err != nil {
			return fmt.Errorf("failed to save policy: %w", err)
		}
		klog.Infof("Removed role: user_id=%d, role=%s", userID, role)
	}

	return nil
}

// GetUserRoles 获取用户角色
func GetUserRoles(enforcer *casbin.Enforcer, userID int64) ([]string, error) {
	roles, err := enforcer.GetRolesForUser(fmt.Sprintf("user:%d", userID))
	if err != nil {
		return nil, fmt.Errorf("failed to get user roles: %w", err)
	}
	return roles, nil
}

// CheckPermission 检查权限
func CheckPermission(enforcer *casbin.Enforcer, userRole, resource, action string) (bool, error) {
	return enforcer.Enforce(userRole, resource, action)
}
