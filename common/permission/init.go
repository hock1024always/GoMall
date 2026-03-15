package permission

import (
	"fmt"
	"path/filepath"

	"github.com/casbin/casbin/v2"
	gormadapter "github.com/casbin/gorm-adapter/v3"
	"github.com/cloudwego/kitex/pkg/klog"
	"gorm.io/gorm"
)

// InitPermission 初始化权限管理器
func InitPermission(db *gorm.DB, configPath string) (*PermissionManager, error) {
	// 获取模型文件路径
	modelPath := filepath.Join(configPath, "model.conf")

	// 创建 GORM Adapter
	adapter, err := gormadapter.NewAdapterByDB(db)
	if err != nil {
		return nil, fmt.Errorf("failed to create casbin adapter: %w", err)
	}

	// 创建 Enforcer
	enforcer, err := casbin.NewEnforcer(modelPath, adapter)
	if err != nil {
		return nil, fmt.Errorf("failed to create casbin enforcer: %w", err)
	}

	// 加载策略
	if err := enforcer.LoadPolicy(); err != nil {
		return nil, fmt.Errorf("failed to load policy: %w", err)
	}

	// 如果数据库为空，加载默认策略
	if len(enforcer.GetPolicy()) == 0 {
		loadDefaultPolicies(enforcer)
	}

	return NewPermissionManager(enforcer), nil
}

// loadDefaultPolicies 加载默认策略
func loadDefaultPolicies(enforcer *casbin.Enforcer) {
	// Admin 权限 - 拥有所有资源的完全访问权限
	adminPolicies := [][]string{
		{"admin", "product", "create"},
		{"admin", "product", "read"},
		{"admin", "product", "update"},
		{"admin", "product", "delete"},
		{"admin", "order", "create"},
		{"admin", "order", "read"},
		{"admin", "order", "update"},
		{"admin", "order", "delete"},
		{"admin", "user", "read"},
		{"admin", "user", "update"},
		{"admin", "cart", "create"},
		{"admin", "cart", "read"},
		{"admin", "cart", "update"},
		{"admin", "cart", "delete"},
		{"admin", "payment", "create"},
		{"admin", "payment", "read"},
		{"admin", "payment", "refund"},
	}
	for _, p := range adminPolicies {
		enforcer.AddPolicy(p)
	}

	// Seller 权限 - 商品管理和订单查看
	sellerPolicies := [][]string{
		{"seller", "product", "create"},
		{"seller", "product", "read"},
		{"seller", "product", "update"},
		{"seller", "order", "read"},
		{"seller", "cart", "read"},
	}
	for _, p := range sellerPolicies {
		enforcer.AddPolicy(p)
	}

	// Customer 权限 - 基本购物功能
	customerPolicies := [][]string{
		{"customer", "product", "read"},
		{"customer", "order", "create"},
		{"customer", "order", "read"},
		{"customer", "order", "update"},
		{"customer", "cart", "create"},
		{"customer", "cart", "read"},
		{"customer", "cart", "update"},
		{"customer", "cart", "delete"},
		{"customer", "payment", "create"},
	}
	for _, p := range customerPolicies {
		enforcer.AddPolicy(p)
	}

	// 保存策略
	if err := enforcer.SavePolicy(); err != nil {
		klog.Errorf("failed to save default policies: %v", err)
	} else {
		klog.Info("default policies loaded successfully")
	}
}
