# Redis 主存储优化方案

## 设计目标

在热场景下使用 Redis 代替 MySQL 作为主存储，解决 Cache Aside 模式下的高并发一致性问题，提升性能。

## 适用场景

### 热数据场景（Redis 主存储）

1. **秒杀商品库存**
   - 高频读写
   - 需要原子操作
   - 实时性要求高

2. **购物车数据**
   - 频繁更新
   - 用户会话相关
   - 可接受数据丢失（可重建）

3. **热点商品信息**
   - 高访问量
   - 读多写少
   - 需要快速响应

### 冷数据场景（MySQL 主存储）

1. **历史订单**
2. **用户基本信息**
3. **支付记录**

## 架构设计

### Redis 主存储架构

```
┌─────────────────────────────────────────┐
│        应用服务层                        │
│  - Product Service                      │
│  - Cart Service                          │
│  - Checkout Service                      │
└──────────────┬──────────────────────────┘
               │
┌──────────────▼──────────────────────────┐
│      Redis 主存储层                      │
│  ┌──────────┐  ┌──────────┐            │
│  │  Master  │──│  Slave   │            │
│  └────┬─────┘  └────┬─────┘            │
│       │             │                   │
│       └──────┬──────┘                   │
│              │                           │
│       ┌──────▼──────┐                    │
│       │  Sentinel   │                    │
│       │  (哨兵)     │                    │
│       └─────────────┘                    │
└──────────────┬──────────────────────────┘
               │
┌──────────────▼──────────────────────────┐
│      MySQL 备份层（异步）                 │
│  - 定期快照                              │
│  - 关键数据双写                          │
└─────────────────────────────────────────┘
```

## Redis 配置（生产级参数）

### Redis Master 配置

```conf
# redis-master.conf
# 网络配置
bind 0.0.0.0
port 6379
protected-mode yes
requirepass gomall_redis_password

# 内存配置
maxmemory 4gb
maxmemory-policy allkeys-lru

# 持久化配置（关键）
# AOF 持久化（实时备份）
appendonly yes
appendfilename "appendonly.aof"
appendfsync everysec  # 每秒同步（平衡性能和数据安全）
auto-aof-rewrite-percentage 100
auto-aof-rewrite-min-size 64mb

# RDB 快照（定期备份）
save 900 1      # 900秒内至少1个key变化
save 300 10     # 300秒内至少10个key变化
save 60 10000   # 60秒内至少10000个key变化

# 主从复制
replica-serve-stale-data yes
replica-read-only yes

# 性能优化
tcp-backlog 511
timeout 0
tcp-keepalive 300
```

### Redis Slave 配置

```conf
# redis-slave.conf
bind 0.0.0.0
port 6380
protected-mode yes
requirepass gomall_redis_password

# 主从复制配置
replicaof redis-master 6379
masterauth gomall_redis_password
replica-serve-stale-data yes
replica-read-only yes

# 持久化配置（与 Master 相同）
appendonly yes
appendfsync everysec
```

### Redis Sentinel 配置

```conf
# sentinel.conf
port 26379
sentinel monitor mymaster redis-master 6379 2
sentinel auth-pass mymaster gomall_redis_password
sentinel down-after-milliseconds mymaster 5000
sentinel parallel-syncs mymaster 1
sentinel failover-timeout mymaster 10000
```

### Docker Compose 配置

```yaml
services:
  redis-master:
    image: redis:7.2-alpine
    ports:
      - "6379:6379"
    volumes:
      - ./redis/master.conf:/usr/local/etc/redis/redis.conf
      - redis-master-data:/data
    command: redis-server /usr/local/etc/redis/redis.conf
    networks:
      - redis-net

  redis-slave:
    image: redis:7.2-alpine
    ports:
      - "6380:6379"
    volumes:
      - ./redis/slave.conf:/usr/local/etc/redis/redis.conf
      - redis-slave-data:/data
    command: redis-server /usr/local/etc/redis/redis.conf
    depends_on:
      - redis-master
    networks:
      - redis-net

  redis-sentinel:
    image: redis:7.2-alpine
    ports:
      - "26379:26379"
    volumes:
      - ./redis/sentinel.conf:/usr/local/etc/redis/sentinel.conf
    command: redis-sentinel /usr/local/etc/redis/sentinel.conf
    depends_on:
      - redis-master
      - redis-slave
    networks:
      - redis-net

volumes:
  redis-master-data:
  redis-slave-data:

networks:
  redis-net:
    driver: bridge
```

## Redis Stack 集成（复杂查询支持）

### 安装 Redis Stack

```yaml
redis-stack:
  image: redis/redis-stack-server:latest
  ports:
    - "6379:6379"
    - "8001:8001"  # RedisInsight 管理界面
  environment:
    - REDIS_ARGS=--requirepass gomall_redis_password
  volumes:
    - redis-stack-data:/data
```

### RediSearch 使用示例

```go
// 创建商品搜索索引
func createProductIndex(client *redis.Client) error {
    // 创建索引定义
    schema := &redisearch.Schema{
        Fields: []*redisearch.Field{
            redisearch.NewTextField("name"),
            redisearch.NewTextFieldOptions("description", redisearch.TextFieldOptions{Weight: 0.5}),
            redisearch.NewNumericField("price"),
            redisearch.NewNumericField("stock"),
            redisearch.NewTagField("category"),
        },
    }
    
    // 创建索引
    index := redisearch.NewClient("product_idx", client)
    return index.CreateIndex(schema)
}

// 搜索商品
func searchProducts(client *redis.Client, query string) ([]Product, error) {
    index := redisearch.NewClient("product_idx", client)
    
    // 执行搜索
    docs, _, err := index.Search(redisearch.NewQuery(query).
        SetSortBy("price", true).
        Limit(0, 20))
    
    if err != nil {
        return nil, err
    }
    
    // 解析结果
    var products []Product
    for _, doc := range docs {
        product := parseProductFromDoc(doc)
        products = append(products, product)
    }
    
    return products, nil
}
```

## 冷热分离方案

### 方案 1: Redis on Flash（推荐）

使用 Redis Enterprise 的 Redis on Flash 功能，自动将冷数据沉降到 SSD。

### 方案 2: KeyDB（开源替代）

```yaml
keydb:
  image: eqalpha/keydb:latest
  ports:
    - "6379:6379"
  volumes:
    - keydb-data:/data
  command:
    - --server-threads 4
    - --storage-provider flash
    - --storage-path /data/flash
```

### 方案 3: 手动冷热分离

```go
// 判断数据热度
func isHotData(key string) bool {
    accessCount := redisClient.Get(ctx, "access:"+key).Int()
    return accessCount > 100  // 访问次数阈值
}

// 冷数据迁移到 MySQL
func migrateColdData(key string) {
    data := redisClient.Get(ctx, key).Val()
    // 写入 MySQL
    mysqlDB.Save(key, data)
    // 从 Redis 删除
    redisClient.Del(ctx, key)
}
```

## 数据安全方案

### 1. 定期快照到 MySQL

```go
// 定时任务：每小时快照一次
func snapshotToMySQL() {
    ticker := time.NewTicker(1 * time.Hour)
    for range ticker.C {
        // 获取所有热数据 key
        keys := redisClient.Keys(ctx, "hot:*").Val()
        
        for _, key := range keys {
            data := redisClient.Get(ctx, key).Val()
            // 写入 MySQL 备份表
            mysqlDB.Exec("INSERT INTO redis_backup (key, value, updated_at) VALUES (?, ?, NOW()) ON DUPLICATE KEY UPDATE value=?, updated_at=NOW()", 
                key, data, data)
        }
    }
}
```

### 2. 关键数据双写

```go
// 关键数据同时写入 Redis 和 MySQL
func writeCriticalData(key string, value interface{}) error {
    // 1. 写入 Redis（主存储）
    err1 := redisClient.Set(ctx, key, value, 0).Err()
    
    // 2. 异步写入 MySQL（备份）
    go func() {
        mysqlDB.Save(key, value)
    }()
    
    return err1
}
```

### 3. AOF 实时备份

配置 `appendfsync everysec`，确保每秒同步到磁盘，最多丢失 1 秒数据。

## 应用场景实现

### 场景 1: 秒杀商品库存（Redis 主存储）

```go
// 库存数据结构
// Key: product:{product_id}:stock
// Value: 库存数量（整数）

// 初始化库存
func InitStock(ctx context.Context, productID int, amount int) error {
    key := fmt.Sprintf("product:%d:stock", productID)
    return redisClient.Set(ctx, key, amount, 0).Err()
}

// 扣减库存（原子操作）
func DecreaseStock(ctx context.Context, productID int, quantity int) (bool, error) {
    key := fmt.Sprintf("product:%d:stock", productID)
    
    // 使用 Lua 脚本保证原子性
    script := `
        local stock = tonumber(redis.call('GET', KEYS[1]) or 0)
        if stock >= tonumber(ARGV[1]) then
            redis.call('DECRBY', KEYS[1], ARGV[1])
            return 1
        else
            return 0
        end
    `
    
    result, err := redisClient.Eval(ctx, script, []string{key}, quantity).Int()
    if err != nil {
        return false, err
    }
    
    return result == 1, nil
}
```

### 场景 2: 购物车（Redis 主存储）

```go
// 购物车数据结构
// Key: cart:{user_id}
// Value: Hash {product_id: quantity}

// 添加商品到购物车
func AddToCart(ctx context.Context, userID int64, productID int64, quantity int) error {
    key := fmt.Sprintf("cart:%d", userID)
    return redisClient.HIncrBy(ctx, key, fmt.Sprintf("%d", productID), int64(quantity)).Err()
}

// 获取购物车
func GetCart(ctx context.Context, userID int64) (map[int64]int, error) {
    key := fmt.Sprintf("cart:%d", userID)
    result := redisClient.HGetAll(ctx, key).Val()
    
    cart := make(map[int64]int)
    for k, v := range result {
        productID, _ := strconv.ParseInt(k, 10, 64)
        quantity, _ := strconv.Atoi(v)
        cart[productID] = quantity
    }
    
    return cart, nil
}
```

## 监控和告警

### 监控指标

1. **Redis 性能指标**
   - 内存使用率
   - 连接数
   - 命令执行延迟
   - 主从同步延迟

2. **数据安全指标**
   - AOF 文件大小
   - 最后同步时间
   - 备份任务执行状态

### 告警规则

```yaml
# Prometheus 告警规则示例
groups:
  - name: redis_alerts
    rules:
      - alert: RedisMemoryHigh
        expr: redis_memory_used_bytes / redis_memory_max_bytes > 0.9
        for: 5m
        annotations:
          summary: "Redis 内存使用率过高"
      
      - alert: RedisMasterDown
        expr: redis_up{role="master"} == 0
        for: 1m
        annotations:
          summary: "Redis Master 节点宕机"
```

## 实施步骤

1. ✅ 部署 Redis 主从集群 + Sentinel
2. ✅ 配置 AOF 持久化
3. ✅ 实现库存管理（Redis 主存储）
4. ✅ 实现购物车管理（Redis 主存储）
5. ✅ 集成 Redis Stack（RediSearch）
6. ✅ 实现数据备份机制（MySQL）
7. ✅ 实现冷热分离
8. ✅ 添加监控和告警
