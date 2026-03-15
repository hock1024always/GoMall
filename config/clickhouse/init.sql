-- ClickHouse 初始化脚本
-- OLAP 数据库和表结构

-- 创建数据库
CREATE DATABASE IF NOT EXISTS gomall_analytics;

-- 订单分析宽表（OLAP）
CREATE TABLE IF NOT EXISTS gomall_analytics.order_analytics
(
    order_id String,
    user_id UInt64,
    order_state String,
    total_amount Decimal(18, 2),
    item_count UInt32,
    currency String,
    
    -- 时间维度
    created_at DateTime,
    created_date Date,
    created_hour UInt8,
    created_weekday UInt8,
    
    -- 地理维度
    country String,
    state String,
    city String,
    
    -- 商品维度
    product_ids Array(UInt64),
    category_ids Array(UInt32),
    
    -- 支付维度
    payment_method String,
    payment_status String,
    
    -- 元数据
    ingested_at DateTime DEFAULT now()
)
ENGINE = MergeTree()
PARTITION BY toYYYYMM(created_date)
ORDER BY (created_date, order_id)
SETTINGS index_granularity = 8192;

-- 商品销量统计表
CREATE TABLE IF NOT EXISTS gomall_analytics.product_sales
(
    product_id UInt64,
    product_name String,
    category_id UInt32,
    category_name String,
    
    -- 时间维度
    stat_date Date,
    stat_hour UInt8,
    
    -- 统计指标
    order_count UInt64,
    quantity_sold UInt64,
    revenue Decimal(18, 2),
    unique_buyers UInt64,
    
    -- 元数据
    ingested_at DateTime DEFAULT now()
)
ENGINE = MergeTree()
PARTITION BY toYYYYMM(stat_date)
ORDER BY (stat_date, product_id)
SETTINGS index_granularity = 8192;

-- 用户行为分析表
CREATE TABLE IF NOT EXISTS gomall_analytics.user_behavior
(
    user_id UInt64,
    session_id String,
    
    -- 行为类型
    action_type String,  -- view, cart, purchase, etc.
    resource_type String, -- product, order, cart, etc.
    resource_id String,
    
    -- 时间
    action_time DateTime,
    action_date Date,
    
    -- 设备信息
    device_type String,
    platform String,
    
    -- 元数据
    ingested_at DateTime DEFAULT now()
)
ENGINE = MergeTree()
PARTITION BY toYYYYMM(action_date)
ORDER BY (action_date, user_id, action_time)
SETTINGS index_granularity = 8192;

-- 销售趋势表（预聚合）
CREATE TABLE IF NOT EXISTS gomall_analytics.sales_trend
(
    stat_date Date,
    stat_hour UInt8,
    
    -- 汇总指标
    total_orders UInt64,
    total_revenue Decimal(18, 2),
    total_items_sold UInt64,
    unique_customers UInt64,
    avg_order_value Decimal(18, 2),
    
    -- 元数据
    ingested_at DateTime DEFAULT now()
)
ENGINE = MergeTree()
PARTITION BY toYYYYMM(stat_date)
ORDER BY (stat_date, stat_hour)
SETTINGS index_granularity = 8192;

-- 商品库存变化表
CREATE TABLE IF NOT EXISTS gomall_analytics.inventory_changes
(
    product_id UInt64,
    change_type String, -- decrease, increase, reserve, cancel_reserve
    quantity Int32,
    before_stock Int32,
    after_stock Int32,
    
    -- 关联信息
    order_id String,
    user_id UInt64,
    
    -- 时间
    change_time DateTime,
    change_date Date,
    
    -- 元数据
    ingested_at DateTime DEFAULT now()
)
ENGINE = MergeTree()
PARTITION BY toYYYYMM(change_date)
ORDER BY (change_date, product_id, change_time)
SETTINGS index_granularity = 8192;

-- 创建物化视图：实时销售统计
CREATE MATERIALIZED VIEW IF NOT EXISTS gomall_analytics.sales_trend_mv
TO gomall_analytics.sales_trend
AS
SELECT
    created_date AS stat_date,
    toHour(created_at) AS stat_hour,
    count() AS total_orders,
    sum(total_amount) AS total_revenue,
    sum(item_count) AS total_items_sold,
    uniqExact(user_id) AS unique_customers,
    avg(total_amount) AS avg_order_value
FROM gomall_analytics.order_analytics
GROUP BY stat_date, stat_hour;
