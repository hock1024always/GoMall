package redis

import (
	"context"
	"fmt"
	"time"

	"github.com/cloudwego/kitex/pkg/klog"
	"github.com/redis/go-redis/v9"
)

// Config Redis配置
type Config struct {
	// Master-Slave 模式
	MasterAddr string
	SlaveAddr  string
	Password   string
	DB         int

	// Sentinel 模式
	SentinelAddrs []string
	MasterName    string

	// 通用配置
	PoolSize     int
	MinIdleConns int
	MaxRetries   int
	DialTimeout  time.Duration
	ReadTimeout  time.Duration
	WriteTimeout time.Duration
}

// DefaultConfig 默认配置
func DefaultConfig() *Config {
	return &Config{
		MasterName:    "mymaster",
		SentinelAddrs: []string{"localhost:26379", "localhost:26380", "localhost:26381"},
		PoolSize:      100,
		MinIdleConns:  10,
		MaxRetries:    3,
		DialTimeout:   5 * time.Second,
		ReadTimeout:   3 * time.Second,
		WriteTimeout:  3 * time.Second,
	}
}

// RedisClient Redis客户端封装
type RedisClient struct {
	master *redis.Client
	slave  *redis.Client
}

// NewRedisClient 创建Redis客户端（主从模式）
func NewRedisClient(cfg *Config) (*RedisClient, error) {
	master := redis.NewClient(&redis.Options{
		Addr:         cfg.MasterAddr,
		Password:     cfg.Password,
		DB:           cfg.DB,
		PoolSize:     cfg.PoolSize,
		MinIdleConns: cfg.MinIdleConns,
		MaxRetries:   cfg.MaxRetries,
		DialTimeout:  cfg.DialTimeout,
		ReadTimeout:  cfg.ReadTimeout,
		WriteTimeout: cfg.WriteTimeout,
	})

	// 测试连接
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := master.Ping(ctx).Err(); err != nil {
		return nil, fmt.Errorf("failed to connect to redis master: %w", err)
	}

	var slave *redis.Client
	if cfg.SlaveAddr != "" {
		slave = redis.NewClient(&redis.Options{
			Addr:         cfg.SlaveAddr,
			Password:     cfg.Password,
			DB:           cfg.DB,
			PoolSize:     cfg.PoolSize,
			MinIdleConns: cfg.MinIdleConns,
			MaxRetries:   cfg.MaxRetries,
			DialTimeout:  cfg.DialTimeout,
			ReadTimeout:  cfg.ReadTimeout,
			WriteTimeout: cfg.WriteTimeout,
			ReadOnly:     true,
		})
	}

	klog.Info("Redis client initialized successfully")
	return &RedisClient{master: master, slave: slave}, nil
}

// NewRedisClientWithSentinel 创建Redis客户端（Sentinel模式）
func NewRedisClientWithSentinel(cfg *Config) (*RedisClient, error) {
	master := redis.NewFailoverClient(&redis.FailoverOptions{
		MasterName:    cfg.MasterName,
		SentinelAddrs: cfg.SentinelAddrs,
		Password:      cfg.Password,
		DB:            cfg.DB,
		PoolSize:      cfg.PoolSize,
		MinIdleConns:  cfg.MinIdleConns,
		MaxRetries:    cfg.MaxRetries,
		DialTimeout:   cfg.DialTimeout,
		ReadTimeout:   cfg.ReadTimeout,
		WriteTimeout:  cfg.WriteTimeout,
	})

	// 测试连接
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := master.Ping(ctx).Err(); err != nil {
		return nil, fmt.Errorf("failed to connect to redis via sentinel: %w", err)
	}

	klog.Info("Redis client with Sentinel initialized successfully")
	return &RedisClient{master: master}, nil
}

// Master 获取主库客户端（用于写操作）
func (c *RedisClient) Master() *redis.Client {
	return c.master
}

// Slave 获取从库客户端（用于读操作）
func (c *RedisClient) Slave() *redis.Client {
	if c.slave != nil {
		return c.slave
	}
	return c.master
}

// Set 设置键值
func (c *RedisClient) Set(ctx context.Context, key string, value interface{}, expiration time.Duration) error {
	return c.master.Set(ctx, key, value, expiration).Err()
}

// Get 获取值
func (c *RedisClient) Get(ctx context.Context, key string) (string, error) {
	return c.Slave().Get(ctx, key).Result()
}

// Del 删除键
func (c *RedisClient) Del(ctx context.Context, keys ...string) error {
	return c.master.Del(ctx, keys...).Err()
}

// Exists 检查键是否存在
func (c *RedisClient) Exists(ctx context.Context, keys ...string) (int64, error) {
	return c.Slave().Exists(ctx, keys...).Result()
}

// Incr 自增
func (c *RedisClient) Incr(ctx context.Context, key string) (int64, error) {
	return c.master.Incr(ctx, key).Result()
}

// IncrBy 自增指定值
func (c *RedisClient) IncrBy(ctx context.Context, key string, value int64) (int64, error) {
	return c.master.IncrBy(ctx, key, value).Result()
}

// DecrBy 自减指定值
func (c *RedisClient) DecrBy(ctx context.Context, key string, value int64) (int64, error) {
	return c.master.DecrBy(ctx, key, value).Result()
}

// HSet 设置哈希字段
func (c *RedisClient) HSet(ctx context.Context, key string, field string, value interface{}) error {
	return c.master.HSet(ctx, key, field, value).Err()
}

// HGet 获取哈希字段
func (c *RedisClient) HGet(ctx context.Context, key, field string) (string, error) {
	return c.Slave().HGet(ctx, key, field).Result()
}

// HGetAll 获取所有哈希字段
func (c *RedisClient) HGetAll(ctx context.Context, key string) (map[string]string, error) {
	return c.Slave().HGetAll(ctx, key).Result()
}

// LPush 列表左推入
func (c *RedisClient) LPush(ctx context.Context, key string, values ...interface{}) error {
	return c.master.LPush(ctx, key, values...).Err()
}

// RPush 列表右推入
func (c *RedisClient) RPush(ctx context.Context, key string, values ...interface{}) error {
	return c.master.RPush(ctx, key, values...).Err()
}

// LRange 获取列表范围
func (c *RedisClient) LRange(ctx context.Context, key string, start, stop int64) ([]string, error) {
	return c.Slave().LRange(ctx, key, start, stop).Result()
}

// SAdd 集合添加成员
func (c *RedisClient) SAdd(ctx context.Context, key string, members ...interface{}) error {
	return c.master.SAdd(ctx, key, members...).Err()
}

// SMembers 获取集合所有成员
func (c *RedisClient) SMembers(ctx context.Context, key string) ([]string, error) {
	return c.Slave().SMembers(ctx, key).Result()
}

// ZAdd 有序集合添加成员
func (c *RedisClient) ZAdd(ctx context.Context, key string, score float64, member string) error {
	return c.master.ZAdd(ctx, key, redis.Z{Score: score, Member: member}).Err()
}

// ZRange 获取有序集合范围
func (c *RedisClient) ZRange(ctx context.Context, key string, start, stop int64) ([]string, error) {
	return c.Slave().ZRange(ctx, key, start, stop).Result()
}

// Expire 设置过期时间
func (c *RedisClient) Expire(ctx context.Context, key string, expiration time.Duration) error {
	return c.master.Expire(ctx, key, expiration).Err()
}

// TTL 获取剩余过期时间
func (c *RedisClient) TTL(ctx context.Context, key string) (time.Duration, error) {
	return c.Slave().TTL(ctx, key).Result()
}

// Eval 执行Lua脚本
func (c *RedisClient) Eval(ctx context.Context, script string, keys []string, args ...interface{}) *redis.Cmd {
	return c.master.Eval(ctx, script, keys, args...)
}

// Close 关闭连接
func (c *RedisClient) Close() error {
	var err error
	if c.master != nil {
		if e := c.master.Close(); e != nil {
			err = e
		}
	}
	if c.slave != nil {
		if e := c.slave.Close(); e != nil && err == nil {
			err = e
		}
	}
	return err
}
