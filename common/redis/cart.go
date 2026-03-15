package cart

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/cloudwego/kitex/pkg/klog"
	"github.com/xvxiaoman8/gomall/common/redis"
)

// CartItem 购物车项
type CartItem struct {
	ProductID int64 `json:"product_id"`
	Quantity  int32 `json:"quantity"`
}

// CartManager 购物车管理器
type CartManager struct {
	redis *redis.RedisClient
}

// NewCartManager 创建购物车管理器
func NewCartManager(client *redis.RedisClient) *CartManager {
	return &CartManager{redis: client}
}

// CartKey 生成购物车键
func CartKey(userID int64) string {
	return fmt.Sprintf("cart:user:%d", userID)
}

// AddItem 添加商品到购物车
func (m *CartManager) AddItem(ctx context.Context, userID int64, item *CartItem) error {
	key := CartKey(userID)

	// 使用Hash存储购物车项
	itemJSON, err := json.Marshal(item)
	if err != nil {
		return fmt.Errorf("failed to marshal cart item: %w", err)
	}

	if err := m.redis.HSet(ctx, key, fmt.Sprintf("%d", item.ProductID), itemJSON); err != nil {
		return fmt.Errorf("failed to add cart item: %w", err)
	}

	// 设置购物车过期时间（7天）
	m.redis.Expire(ctx, key, 7*24*time.Hour)

	return nil
}

// GetCart 获取购物车
func (m *CartManager) GetCart(ctx context.Context, userID int64) ([]*CartItem, error) {
	key := CartKey(userID)

	items, err := m.redis.HGetAll(ctx, key)
	if err != nil {
		return nil, fmt.Errorf("failed to get cart: %w", err)
	}

	var cartItems []*CartItem
	for _, itemJSON := range items {
		var item CartItem
		if err := json.Unmarshal([]byte(itemJSON), &item); err != nil {
			klog.Warnf("failed to unmarshal cart item: %v", err)
			continue
		}
		cartItems = append(cartItems, &item)
	}

	return cartItems, nil
}

// UpdateItemQuantity 更新购物车项数量
func (m *CartManager) UpdateItemQuantity(ctx context.Context, userID int64, productID int64, quantity int32) error {
	key := CartKey(userID)

	if quantity <= 0 {
		// 数量为0时删除
		return m.RemoveItem(ctx, userID, productID)
	}

	item := &CartItem{
		ProductID: productID,
		Quantity:  quantity,
	}

	return m.AddItem(ctx, userID, item)
}

// RemoveItem 从购物车移除商品
func (m *CartManager) RemoveItem(ctx context.Context, userID int64, productID int64) error {
	key := CartKey(userID)

	// 使用Lua脚本删除Hash字段
	script := `
		redis.call('HDEL', KEYS[1], ARGV[1])
		return 1
	`

	_, err := m.redis.Eval(ctx, script, []string{key}, fmt.Sprintf("%d", productID)).Result()
	if err != nil {
		return fmt.Errorf("failed to remove cart item: %w", err)
	}

	return nil
}

// EmptyCart 清空购物车
func (m *CartManager) EmptyCart(ctx context.Context, userID int64) error {
	key := CartKey(userID)
	return m.redis.Del(ctx, key)
}

// GetItemCount 获取购物车商品数量
func (m *CartManager) GetItemCount(ctx context.Context, userID int64) (int64, error) {
	key := CartKey(userID)

	items, err := m.redis.HGetAll(ctx, key)
	if err != nil {
		return 0, fmt.Errorf("failed to get cart: %w", err)
	}

	var count int64
	for _, itemJSON := range items {
		var item CartItem
		if err := json.Unmarshal([]byte(itemJSON), &item); err != nil {
			continue
		}
		count += int64(item.Quantity)
	}

	return count, nil
}

// BackupCart 备份购物车（用于Saga补偿）
func (m *CartManager) BackupCart(ctx context.Context, userID int64) (string, error) {
	key := CartKey(userID)
	backupKey := fmt.Sprintf("cart:backup:user:%d:%d", userID, time.Now().UnixNano())

	// 获取购物车内容
	items, err := m.redis.HGetAll(ctx, key)
	if err != nil {
		return "", fmt.Errorf("failed to get cart for backup: %w", err)
	}

	// 保存备份
	backupJSON, err := json.Marshal(items)
	if err != nil {
		return "", fmt.Errorf("failed to marshal cart backup: %w", err)
	}

	if err := m.redis.Set(ctx, backupKey, backupJSON, 24*time.Hour); err != nil {
		return "", fmt.Errorf("failed to save cart backup: %w", err)
	}

	return backupKey, nil
}

// RestoreCart 恢复购物车（用于Saga补偿）
func (m *CartManager) RestoreCart(ctx context.Context, userID int64, backupKey string) error {
	// 获取备份
	backupJSON, err := m.redis.Get(ctx, backupKey)
	if err != nil {
		return fmt.Errorf("failed to get cart backup: %w", err)
	}

	var items map[string]string
	if err := json.Unmarshal([]byte(backupJSON), &items); err != nil {
		return fmt.Errorf("failed to unmarshal cart backup: %w", err)
	}

	// 恢复购物车
	key := CartKey(userID)
	for productID, itemJSON := range items {
		if err := m.redis.HSet(ctx, key, productID, itemJSON); err != nil {
			klog.Warnf("failed to restore cart item %s: %v", productID, err)
		}
	}

	// 删除备份
	m.redis.Del(ctx, backupKey)

	return nil
}

// IsCartEmpty 检查购物车是否为空
func (m *CartManager) IsCartEmpty(ctx context.Context, userID int64) (bool, error) {
	items, err := m.GetCart(ctx, userID)
	if err != nil {
		return true, err
	}
	return len(items) == 0, nil
}

// MergeCart 合并购物车（用于登录后合并匿名购物车）
func (m *CartManager) MergeCart(ctx context.Context, userID int64, anonymousCartItems []*CartItem) error {
	for _, item := range anonymousCartItems {
		if err := m.AddItem(ctx, userID, item); err != nil {
			klog.Warnf("failed to merge cart item %d: %v", item.ProductID, err)
		}
	}
	return nil
}
