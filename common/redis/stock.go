package stock

import (
	"context"
	"fmt"
	"strconv"
	"time"

	"github.com/cloudwego/kitex/pkg/klog"
	"github.com/xvxiaoman8/gomall/common/redis"
)

// StockManager 库存管理器
type StockManager struct {
	redis *redis.RedisClient
}

// NewStockManager 创建库存管理器
func NewStockManager(client *redis.RedisClient) *StockManager {
	return &StockManager{redis: client}
}

// StockKey 生成库存键
func StockKey(productID int64) string {
	return fmt.Sprintf("stock:product:%d", productID)
}

// StockStatusKey 生成库存状态键（用于秒杀）
func StockStatusKey(productID int64) string {
	return fmt.Sprintf("stock:status:product:%d", productID)
}

// InitStock 初始化库存
func (m *StockManager) InitStock(ctx context.Context, productID int64, quantity int32) error {
	key := StockKey(productID)
	return m.redis.Set(ctx, key, quantity, 0)
}

// GetStock 获取库存
func (m *StockManager) GetStock(ctx context.Context, productID int64) (int32, error) {
	key := StockKey(productID)
	result, err := m.redis.Get(ctx, key)
	if err != nil {
		return 0, fmt.Errorf("failed to get stock: %w", err)
	}

	stock, err := strconv.Atoi(result)
	if err != nil {
		return 0, fmt.Errorf("failed to parse stock: %w", err)
	}

	return int32(stock), nil
}

// DecreaseStock 扣减库存（原子操作，使用Lua脚本）
func (m *StockManager) DecreaseStock(ctx context.Context, productID int64, quantity int32) (bool, error) {
	key := StockKey(productID)

	// Lua脚本：原子性检查并扣减库存
	script := `
		local stock = tonumber(redis.call('GET', KEYS[1]) or 0)
		if stock >= tonumber(ARGV[1]) then
			redis.call('DECRBY', KEYS[1], ARGV[1])
			return 1
		else
			return 0
		end
	`

	result, err := m.redis.Eval(ctx, script, []string{key}, quantity).Int()
	if err != nil {
		return false, fmt.Errorf("failed to decrease stock: %w", err)
	}

	return result == 1, nil
}

// IncreaseStock 恢复库存（用于补偿）
func (m *StockManager) IncreaseStock(ctx context.Context, productID int64, quantity int32) error {
	key := StockKey(productID)
	_, err := m.redis.IncrBy(ctx, key, int64(quantity))
	if err != nil {
		return fmt.Errorf("failed to increase stock: %w", err)
	}
	return nil
}

// SetStockStatus 设置库存状态（用于秒杀场景）
// status: "active", "finished", "paused"
func (m *StockManager) SetStockStatus(ctx context.Context, productID int64, status string) error {
	key := StockStatusKey(productID)
	return m.redis.Set(ctx, key, status, 0)
}

// GetStockStatus 获取库存状态
func (m *StockManager) GetStockStatus(ctx context.Context, productID int64) (string, error) {
	key := StockStatusKey(productID)
	return m.redis.Get(ctx, key)
}

// IsStockActive 检查库存是否可用
func (m *StockManager) IsStockActive(ctx context.Context, productID int64) (bool, error) {
	status, err := m.GetStockStatus(ctx, productID)
	if err != nil {
		// 如果状态不存在，默认为可用
		return true, nil
	}
	return status == "active" || status == "", nil
}

// BatchDecreaseStock 批量扣减库存
func (m *StockManager) BatchDecreaseStock(ctx context.Context, items map[int64]int32) (map[int64]bool, error) {
	results := make(map[int64]bool)

	for productID, quantity := range items {
		success, err := m.DecreaseStock(ctx, productID, quantity)
		if err != nil {
			klog.Errorf("failed to decrease stock for product %d: %v", productID, err)
			results[productID] = false
			continue
		}
		results[productID] = success
	}

	return results, nil
}

// BatchIncreaseStock 批量恢复库存
func (m *StockManager) BatchIncreaseStock(ctx context.Context, items map[int64]int32) error {
	for productID, quantity := range items {
		if err := m.IncreaseStock(ctx, productID, quantity); err != nil {
			klog.Errorf("failed to increase stock for product %d: %v", productID, err)
			// 继续执行其他恢复
		}
	}
	return nil
}

// ReserveStock 预留库存（用于预扣减场景）
func (m *StockManager) ReserveStock(ctx context.Context, productID int64, quantity int32, ttl time.Duration) (bool, error) {
	stockKey := StockKey(productID)
	reserveKey := fmt.Sprintf("stock:reserve:product:%d:%d", productID, time.Now().UnixNano())

	// Lua脚本：原子性预扣减
	script := `
		local stock = tonumber(redis.call('GET', KEYS[1]) or 0)
		if stock >= tonumber(ARGV[1]) then
			redis.call('DECRBY', KEYS[1], ARGV[1])
			redis.call('SET', KEYS[2], ARGV[1], 'EX', ARGV[2])
			return 1
		else
			return 0
		end
	`

	result, err := m.redis.Eval(ctx, script, []string{stockKey, reserveKey}, quantity, int(ttl.Seconds())).Int()
	if err != nil {
		return false, fmt.Errorf("failed to reserve stock: %w", err)
	}

	return result == 1, nil
}

// ConfirmReserve 确认预留（真正扣减）
func (m *StockManager) ConfirmReserve(ctx context.Context, productID int64, reserveKey string) error {
	return m.redis.Del(ctx, reserveKey)
}

// CancelReserve 取消预留（恢复库存）
func (m *StockManager) CancelReserve(ctx context.Context, productID int64, reserveKey string) error {
	// 获取预留数量
	result, err := m.redis.Get(ctx, reserveKey)
	if err != nil {
		return fmt.Errorf("failed to get reserve: %w", err)
	}

	quantity, err := strconv.Atoi(result)
	if err != nil {
		return fmt.Errorf("failed to parse reserve quantity: %w", err)
	}

	// 恢复库存
	if err := m.IncreaseStock(ctx, productID, int32(quantity)); err != nil {
		return fmt.Errorf("failed to restore stock: %w", err)
	}

	// 删除预留记录
	return m.redis.Del(ctx, reserveKey)
}
