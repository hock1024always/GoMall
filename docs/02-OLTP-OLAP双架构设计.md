# OLTP/OLAP 双架构设计

## 设计目标

将事务型操作（OLTP）与分析型操作（OLAP）分离，使用不同的数据库实例和存储策略，避免相互影响。

## 架构设计

### OLTP 数据库（MySQL）

**用途**: 处理实时事务型操作

**特点**:
- 行式存储
- 严格遵循数据库三范式
- 窄表设计（减少冗余）
- 支持 ACID 事务
- 快速写入和单条记录查询

**应用场景**:
- 用户注册/登录
- 订单创建/更新
- 支付处理
- 购物车操作（非热点数据）

**表设计原则**:
```sql
-- 示例：订单表（窄表设计）
CREATE TABLE orders (
    id BIGINT PRIMARY KEY AUTO_INCREMENT,
    user_id BIGINT NOT NULL,
    order_id VARCHAR(64) UNIQUE NOT NULL,
    status VARCHAR(32) NOT NULL,
    total_amount DECIMAL(10,2) NOT NULL,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    INDEX idx_user_id (user_id),
    INDEX idx_order_id (order_id),
    INDEX idx_status (status)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

-- 订单项表（遵循三范式）
CREATE TABLE order_items (
    id BIGINT PRIMARY KEY AUTO_INCREMENT,
    order_id VARCHAR(64) NOT NULL,
    product_id BIGINT NOT NULL,
    quantity INT NOT NULL,
    price DECIMAL(10,2) NOT NULL,
    FOREIGN KEY (order_id) REFERENCES orders(order_id),
    INDEX idx_order_id (order_id),
    INDEX idx_product_id (product_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
```

### OLAP 数据库（ClickHouse）

**用途**: 处理分析型查询和批量计算

**特点**:
- 列式存储
- 宽表设计（冗余数据，空间换时间）
- 不支持事务（最终一致性）
- 批量写入，快速聚合查询
- 适合趋势分析和报表生成

**应用场景**:
- 商品销量趋势分析
- 用户行为分析
- 库存预测
- 销售报表
- 实时大屏数据

**表设计原则**:
```sql
-- 示例：订单分析宽表（冗余设计）
CREATE TABLE order_analytics (
    order_id String,
    user_id UInt64,
    user_email String,
    product_id UInt64,
    product_name String,
    product_category String,
    quantity UInt32,
    price Decimal(10,2),
    total_amount Decimal(10,2),
    order_status String,
    order_date Date,
    order_hour UInt8,
    payment_method String,
    shipping_address String,
    created_at DateTime
) ENGINE = MergeTree()
PARTITION BY toYYYYMM(order_date)
ORDER BY (order_date, product_category, product_id)
SETTINGS index_granularity = 8192;

-- 商品销量统计表
CREATE TABLE product_sales_stats (
    product_id UInt64,
    product_name String,
    category String,
    total_sales UInt64,
    total_revenue Decimal(12,2),
    avg_price Decimal(10,2),
    sales_date Date,
    sales_hour UInt8
) ENGINE = SummingMergeTree(total_sales, total_revenue)
PARTITION BY toYYYYMM(sales_date)
ORDER BY (sales_date, product_id);
```

## 数据同步方案

### 同步策略

使用 **NATS 消息队列** 进行异步数据同步，避免影响 OLTP 性能。

### 同步流程

```
MySQL (OLTP) → NATS 消息队列 → ClickHouse (OLAP)
```

### 实现步骤

1. **在 MySQL 操作后发送消息到 NATS**
   ```go
   // 示例：订单创建后发送分析事件
   func PlaceOrder(ctx context.Context, req *order.PlaceOrderReq) {
       // 1. 创建订单（MySQL）
       order := createOrderInMySQL(req)
       
       // 2. 发送分析事件到 NATS
       event := &OrderCreatedEvent{
           OrderID: order.OrderID,
           UserID: order.UserID,
           Items: order.Items,
           CreatedAt: time.Now(),
       }
       natsClient.Publish("order.created", event)
   }
   ```

2. **消费 NATS 消息并写入 ClickHouse**
   ```go
   // 独立的消费者服务
   func consumeOrderEvents() {
       natsClient.Subscribe("order.created", func(msg *nats.Msg) {
           event := parseOrderEvent(msg.Data)
           // 转换为宽表格式并写入 ClickHouse
           insertIntoClickHouse(event)
       })
   }
   ```

### 同步延迟

- **目标延迟**: < 5 秒
- **可接受延迟**: < 30 秒（最终一致性）

## 配置参数

### MySQL 配置（OLTP）

```yaml
# docker-compose.yaml 或 my.cnf
mysql:
  image: mysql:8.0
  environment:
    - MYSQL_ROOT_PASSWORD=gomall
    - MYSQL_DATABASE=gomall_oltp
  command:
    - --innodb_buffer_pool_size=2G
    - --innodb_log_file_size=512M
    - --max_connections=1000
    - --innodb_flush_log_at_trx_commit=1  # 严格一致性
    - --sync_binlog=1
    - --binlog_format=ROW
```

### ClickHouse 配置（OLAP）

```yaml
clickhouse:
  image: clickhouse/clickhouse-server:latest
  environment:
    - CLICKHOUSE_DB=gomall_olap
    - CLICKHOUSE_USER=default
    - CLICKHOUSE_PASSWORD=gomall
  ulimits:
    nofile:
      soft: 262144
      hard: 262144
```

### NATS 配置

```yaml
nats:
  image: nats:latest
  ports:
    - "4222:4222"  # 客户端端口
    - "8222:8222"  # 监控端口
  command:
    - "-js"  # 启用 JetStream（持久化消息）
    - "-sd"  # 数据目录
    - "/data"
```

## 应用场景示例

### 场景 1: 商品销量趋势分析

**OLTP 操作**: 用户下单（MySQL）
**OLAP 查询**: 查询最近 7 天商品销量趋势（ClickHouse）

```sql
-- ClickHouse 查询
SELECT 
    toDate(order_date) as date,
    product_name,
    sum(quantity) as total_sales
FROM order_analytics
WHERE order_date >= today() - 7
GROUP BY date, product_name
ORDER BY date DESC, total_sales DESC;
```

### 场景 2: 用户行为分析

**OLTP 操作**: 用户浏览商品（MySQL 记录）
**OLAP 查询**: 分析用户购买偏好（ClickHouse）

```sql
-- ClickHouse 查询
SELECT 
    user_id,
    product_category,
    count(*) as view_count,
    sum(quantity) as purchase_count
FROM user_behavior_analytics
WHERE user_id = ?
GROUP BY user_id, product_category;
```

## 注意事项

1. **数据一致性**: OLAP 数据是最终一致的，不用于实时查询
2. **数据同步失败**: 需要实现重试机制和死信队列
3. **数据量控制**: 定期清理历史数据，避免 ClickHouse 数据过大
4. **查询分离**: 确保分析查询不会影响 OLTP 性能

## 实施步骤

1. ✅ 部署 ClickHouse 实例
2. ✅ 配置 NATS JetStream
3. ✅ 实现消息发布（在订单/支付等服务中）
4. ✅ 实现消息消费（独立的消费者服务）
5. ✅ 设计 ClickHouse 表结构
6. ✅ 实现数据转换和写入逻辑
7. ✅ 添加监控和告警
