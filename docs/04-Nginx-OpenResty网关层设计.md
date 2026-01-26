# Nginx + OpenResty 网关层设计

## 设计目标

在网关层使用 Nginx + OpenResty 配合 Lua 脚本，通过读取 Redis 标志位来过滤无效请求，减少后端服务压力，提升秒杀场景性能。

## 架构设计

```
┌─────────────────────────────────────────┐
│          用户请求                        │
└──────────────┬──────────────────────────┘
               │
┌──────────────▼──────────────────────────┐
│      Nginx + OpenResty 网关层            │
│  ┌──────────────────────────────────┐   │
│  │  Lua 脚本                        │   │
│  │  - 读取 Redis 标志位             │   │
│  │  - 检查秒杀状态                 │   │
│  │  - 过滤无效请求                 │   │
│  └──────────┬───────────────────────┘   │
│             │                            │
│  ┌──────────▼───────────────────────┐   │
│  │  Redis 从库（同机部署）            │   │
│  │  - product:{id}:status           │   │
│  │  - product:{id}:stock            │   │
│  └──────────────────────────────────┘   │
└──────────────┬──────────────────────────┘
               │
        ┌──────▼──────┐
        │  有效请求    │
        └──────┬──────┘
               │
┌──────────────▼──────────────────────────┐
│          后端服务层                       │
│  - Product Service                      │
│  - Checkout Service                     │
└─────────────────────────────────────────┘
```

## 部署方案

### 同机部署优势

- **减少网络 IO**：Nginx 和 Redis 在同一台机器，使用本地 socket 或 loopback 接口
- **降低延迟**：本地访问延迟 < 1ms
- **简化部署**：减少网络配置复杂度

### 部署架构

```
┌─────────────────────────────────────────┐
│         网关服务器（单机）                │
│  ┌──────────────┐  ┌──────────────┐    │
│  │   Nginx      │  │  Redis       │    │
│  │  OpenResty   │  │  (Slave)     │    │
│  │   :80        │  │  :6379       │    │
│  └──────────────┘  └──────────────┘    │
│         │                │              │
│         └───────┬────────┘              │
│                 │                        │
│         本地通信（低延迟）                 │
└─────────────────────────────────────────┘
```

## Nginx 配置

### 主配置文件

```nginx
# nginx.conf
user nginx;
worker_processes auto;
error_log /var/log/nginx/error.log warn;
pid /var/run/nginx.pid;

events {
    worker_connections 10240;
    use epoll;
}

http {
    include /etc/nginx/mime.types;
    default_type application/octet-stream;
    
    log_format main '$remote_addr - $remote_user [$time_local] "$request" '
                    '$status $body_bytes_sent "$http_referer" '
                    '"$http_user_agent" "$http_x_forwarded_for"';
    
    access_log /var/log/nginx/access.log main;
    
    sendfile on;
    tcp_nopush on;
    tcp_nodelay on;
    keepalive_timeout 65;
    types_hash_max_size 2048;
    
    # Lua 模块配置
    lua_package_path "/etc/nginx/lua/?.lua;;";
    lua_package_cpath "/usr/local/lib/lua/5.1/?.so;;";
    
    # Redis 连接池配置
    lua_shared_dict redis_pool 10m;
    
    # 限流配置
    limit_req_zone $binary_remote_addr zone=api_limit:10m rate=100r/s;
    limit_conn_zone $binary_remote_addr zone=conn_limit:10m;
    
    upstream backend {
        least_conn;
        server backend1:8080 weight=1 max_fails=3 fail_timeout=30s;
        server backend2:8080 weight=1 max_fails=3 fail_timeout=30s;
        keepalive 32;
    }
    
    server {
        listen 80;
        server_name gomall.example.com;
        
        # 限流
        limit_req zone=api_limit burst=50 nodelay;
        limit_conn conn_limit 10;
        
        # 秒杀接口 - 使用 Lua 脚本过滤
        location /api/seckill {
            access_by_lua_block {
                local seckill = require "seckill_filter"
                seckill.filter()
            }
            
            proxy_pass http://backend;
            proxy_set_header Host $host;
            proxy_set_header X-Real-IP $remote_addr;
            proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
            proxy_connect_timeout 5s;
            proxy_send_timeout 5s;
            proxy_read_timeout 5s;
        }
        
        # 商品查询接口 - 检查商品状态
        location ~ ^/api/product/(\d+) {
            set $product_id $1;
            access_by_lua_block {
                local product_filter = require "product_filter"
                product_filter.check_status(ngx.var.product_id)
            }
            
            proxy_pass http://backend;
            proxy_set_header Host $host;
            proxy_set_header X-Real-IP $remote_addr;
        }
        
        # 其他接口直接转发
        location /api/ {
            proxy_pass http://backend;
            proxy_set_header Host $host;
            proxy_set_header X-Real-IP $remote_addr;
        }
        
        # 健康检查
        location /health {
            access_log off;
            return 200 "healthy\n";
            add_header Content-Type text/plain;
        }
    }
}
```

## Lua 脚本实现

### 1. Redis 连接模块

```lua
-- lua/redis_client.lua
local redis = require "resty.redis"
local cjson = require "cjson"

local _M = {}

-- Redis 连接配置（本地连接）
local redis_config = {
    host = "127.0.0.1",  -- 本地 Redis
    port = 6379,
    password = "gomall_redis_password",
    timeout = 1000,  -- 1秒超时
    pool_size = 100,
    pool_idle_timeout = 10000,
}

-- 获取 Redis 连接
function _M.get_connection()
    local red = redis:new()
    red:set_timeout(redis_config.timeout)
    
    local ok, err = red:connect(redis_config.host, redis_config.port)
    if not ok then
        ngx.log(ngx.ERR, "failed to connect to redis: ", err)
        return nil, err
    end
    
    -- 认证
    if redis_config.password then
        local res, err = red:auth(redis_config.password)
        if not res then
            ngx.log(ngx.ERR, "failed to authenticate: ", err)
            return nil, err
        end
    end
    
    return red, nil
end

-- 执行 Redis 命令
function _M.execute(command, ...)
    local red, err = _M.get_connection()
    if not red then
        return nil, err
    end
    
    local res, err = red[command](red, ...)
    
    -- 归还连接到连接池
    red:set_keepalive(redis_config.pool_idle_timeout, redis_config.pool_size)
    
    return res, err
end

return _M
```

### 2. 秒杀过滤脚本

```lua
-- lua/seckill_filter.lua
local redis_client = require "redis_client"
local cjson = require "cjson"

local _M = {}

-- 秒杀过滤逻辑
function _M.filter()
    -- 从请求参数中获取商品ID
    local args = ngx.req.get_uri_args()
    local product_id = args["product_id"]
    
    if not product_id then
        ngx.status = 400
        ngx.say(cjson.encode({error = "product_id is required"}))
        ngx.exit(400)
    end
    
    -- 检查秒杀状态标志位
    local status_key = "product:" .. product_id .. ":status"
    local status, err = redis_client.execute("get", status_key)
    
    if err then
        ngx.log(ngx.ERR, "redis error: ", err)
        -- Redis 错误时放行，由后端处理
        return
    end
    
    -- 如果秒杀已结束，直接返回
    if status == "finished" then
        ngx.status = 200
        ngx.header.content_type = "application/json"
        ngx.say(cjson.encode({
            code = 40001,
            message = "秒杀已结束",
            data = nil
        }))
        ngx.exit(200)
    end
    
    -- 如果秒杀未开始
    if status == "not_started" then
        ngx.status = 200
        ngx.header.content_type = "application/json"
        ngx.say(cjson.encode({
            code = 40002,
            message = "秒杀未开始",
            data = nil
        }))
        ngx.exit(200)
    end
    
    -- 检查库存（快速预检）
    local stock_key = "product:" .. product_id .. ":stock"
    local stock, err = redis_client.execute("get", stock_key)
    
    if err then
        ngx.log(ngx.ERR, "redis error: ", err)
        return
    end
    
    local stock_num = tonumber(stock)
    if not stock_num or stock_num <= 0 then
        ngx.status = 200
        ngx.header.content_type = "application/json"
        ngx.say(cjson.encode({
            code = 40003,
            message = "商品已售罄",
            data = nil
        }))
        ngx.exit(200)
    end
    
    -- 通过过滤，继续转发到后端
end

return _M
```

### 3. 商品状态检查脚本

```lua
-- lua/product_filter.lua
local redis_client = require "redis_client"
local cjson = require "cjson"

local _M = {}

-- 检查商品状态
function _M.check_status(product_id)
    if not product_id then
        return
    end
    
    local status_key = "product:" .. product_id .. ":status"
    local status, err = redis_client.execute("get", status_key)
    
    if err then
        ngx.log(ngx.ERR, "redis error: ", err)
        return
    end
    
    -- 如果商品已下架，直接返回
    if status == "offline" then
        ngx.status = 200
        ngx.header.content_type = "application/json"
        ngx.say(cjson.encode({
            code = 40004,
            message = "商品已下架",
            data = nil
        }))
        ngx.exit(200)
    end
end

return _M
```

## Docker Compose 配置

```yaml
services:
  nginx-gateway:
    image: openresty/openresty:alpine
    ports:
      - "80:80"
    volumes:
      - ./nginx/nginx.conf:/etc/nginx/nginx.conf:ro
      - ./nginx/lua:/etc/nginx/lua:ro
    depends_on:
      - redis-slave-gateway
    networks:
      - gateway-net

  redis-slave-gateway:
    image: redis:7.2-alpine
    ports:
      - "6379:6379"
    volumes:
      - ./redis/slave-gateway.conf:/usr/local/etc/redis/redis.conf
      - redis-gateway-data:/data
    command: redis-server /usr/local/etc/redis/redis.conf
    environment:
      - REDIS_REPLICATION_MODE=slave
      - REDIS_MASTER_HOST=redis-master
      - REDIS_MASTER_PORT=6379
      - REDIS_MASTER_PASSWORD=gomall_redis_password
    networks:
      - gateway-net

volumes:
  redis-gateway-data:

networks:
  gateway-net:
    driver: bridge
```

## Redis 标志位设计

### 商品状态标志位

```redis
# 商品秒杀状态
product:{product_id}:status
# 值: "not_started" | "ongoing" | "finished"

# 商品库存
product:{product_id}:stock
# 值: 整数（库存数量）

# 商品基本信息（缓存）
product:{product_id}:info
# 值: JSON 字符串
```

### 标志位更新时机

```go
// 秒杀开始
func StartSeckill(productID int) {
    redisClient.Set(ctx, fmt.Sprintf("product:%d:status", productID), "ongoing", 0)
}

// 秒杀结束
func FinishSeckill(productID int) {
    redisClient.Set(ctx, fmt.Sprintf("product:%d:status", productID), "finished", 0)
}

// 库存变化时更新（在扣减库存的 Lua 脚本中自动更新）
```

## 性能优化

### 1. Lua 脚本缓存

```nginx
# 在 http 块中配置
lua_code_cache on;  # 生产环境开启缓存
```

### 2. Redis 连接池

```lua
-- 使用连接池复用连接
local red = redis:new()
red:set_keepalive(10000, 100)  -- 10秒空闲时间，100个连接池大小
```

### 3. 本地 Redis 优化

```conf
# redis-slave-gateway.conf
# 禁用持久化（只读从库）
save ""
appendonly no

# 优化内存
maxmemory 2gb
maxmemory-policy allkeys-lru

# 禁用不必要的命令
rename-command FLUSHDB ""
rename-command FLUSHALL ""
rename-command CONFIG ""
```

## 监控指标

### Nginx 监控

```nginx
# 在 server 块中添加
location /nginx_status {
    stub_status on;
    access_log off;
    allow 127.0.0.1;
    deny all;
}
```

### Lua 脚本性能监控

```lua
-- 记录执行时间
local start_time = ngx.now()
-- ... 执行逻辑 ...
local elapsed = ngx.now() - start_time
ngx.log(ngx.INFO, "lua script elapsed: ", elapsed)
```

## 实施步骤

1. ✅ 部署 OpenResty
2. ✅ 配置 Nginx + Lua 模块
3. ✅ 部署 Redis 从库（网关层）
4. ✅ 实现 Redis 连接模块
5. ✅ 实现秒杀过滤脚本
6. ✅ 实现商品状态检查脚本
7. ✅ 配置负载均衡
8. ✅ 添加监控和日志
9. ✅ 性能测试和优化

## 预期效果

- **请求过滤率**: 80%+ 无效请求在网关层被过滤
- **后端压力降低**: 减少 80%+ 无效请求到后端
- **响应时间**: 网关层过滤响应时间 < 5ms
- **吞吐量**: 支持 10,000+ QPS（网关层）
