package saga

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
)

// RedisSagaStorage Redis 存储实现
type RedisSagaStorage struct {
	client *redis.Client
	prefix string
}

// NewRedisSagaStorage 创建 Redis 存储实例
func NewRedisSagaStorage(client *redis.Client) *RedisSagaStorage {
	return &RedisSagaStorage{
		client: client,
		prefix: "saga:instance",
	}
}

// Save 保存 Saga 实例
func (s *RedisSagaStorage) Save(ctx context.Context, instance *SagaInstance) error {
	key := fmt.Sprintf("%s:%s", s.prefix, instance.ID)
	
	// 序列化实例（排除 Steps，因为函数无法序列化）
	instanceCopy := *instance
	instanceCopy.Steps = nil // Steps 不序列化，因为包含函数
	
	data, err := json.Marshal(&instanceCopy)
	if err != nil {
		return fmt.Errorf("marshal saga instance failed: %w", err)
	}
	
	// 设置过期时间（24小时）
	return s.client.Set(ctx, key, data, 24*time.Hour).Err()
}

// Get 获取 Saga 实例
func (s *RedisSagaStorage) Get(ctx context.Context, sagaID string) (*SagaInstance, error) {
	key := fmt.Sprintf("%s:%s", s.prefix, sagaID)
	
	data, err := s.client.Get(ctx, key).Bytes()
	if err != nil {
		if err == redis.Nil {
			return nil, fmt.Errorf("saga instance not found: %s", sagaID)
		}
		return nil, fmt.Errorf("get saga instance failed: %w", err)
	}
	
	var instance SagaInstance
	if err := json.Unmarshal(data, &instance); err != nil {
		return nil, fmt.Errorf("unmarshal saga instance failed: %w", err)
	}
	
	return &instance, nil
}

// List 列出 Saga 实例
func (s *RedisSagaStorage) List(ctx context.Context, status SagaStatus, limit int) ([]*SagaInstance, error) {
	// 使用 SCAN 遍历所有匹配的 key
	pattern := fmt.Sprintf("%s:*", s.prefix)
	var instances []*SagaInstance
	
	iter := s.client.Scan(ctx, 0, pattern, int64(limit)).Iterator()
	for iter.Next(ctx) {
		key := iter.Val()
		sagaID := key[len(s.prefix)+1:] // 移除前缀
		
		instance, err := s.Get(ctx, sagaID)
		if err != nil {
			continue
		}
		
		if status == "" || instance.Status == status {
			instances = append(instances, instance)
		}
		
		if len(instances) >= limit {
			break
		}
	}
	
	if err := iter.Err(); err != nil {
		return nil, fmt.Errorf("scan saga instances failed: %w", err)
	}
	
	return instances, nil
}
