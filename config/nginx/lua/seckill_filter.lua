-- seckill_filter.lua
-- 秒杀请求过滤脚本
-- 在网关层过滤无效请求，减轻后端压力

local redis = require "resty.redis"
local cjson = require "cjson"

-- 从URI中提取商品ID
local function get_product_id()
    local uri = ngx.var.uri
    local product_id = ngx.var.arg_product_id
    if not product_id then
        -- 尝试从请求体解析
        ngx.req.read_body()
        local body = ngx.req.get_body_data()
        if body then
            local data = cjson.decode(body)
            if data and data.product_id then
                return tostring(data.product_id)
            end
        end
    end
    return product_id
end

-- 连接Redis
local function get_redis_connection()
    local red = redis:new()
    red:set_timeout(1000) -- 1秒超时

    -- 连接Redis从库（本地部署）
    local ok, err = red:connect("redis-slave", 6379)
    if not ok then
        ngx.log(ngx.ERR, "failed to connect to redis: ", err)
        return nil, err
    end
    return red, nil
end

-- 检查商品状态
local function check_product_status(red, product_id)
    local status_key = "stock:status:product:" .. product_id
    local status, err = red:get(status_key)
    
    if err then
        ngx.log(ngx.WARN, "failed to get product status: ", err)
        return true -- 如果Redis出错，放行请求
    end
    
    if status == ngx.null then
        return true -- 状态不存在，默认放行
    end
    
    if status == "finished" then
        return false, "秒杀已结束"
    elseif status == "paused" then
        return false, "秒杀暂停中"
    end
    
    return true
end

-- 检查库存
local function check_stock(red, product_id)
    local stock_key = "stock:product:" .. product_id
    local stock, err = red:get(stock_key)
    
    if err then
        ngx.log(ngx.WARN, "failed to get stock: ", err)
        return true
    end
    
    if stock == ngx.null then
        return true -- 库存信息不存在，放行让后端处理
    end
    
    local stock_num = tonumber(stock)
    if stock_num and stock_num <= 0 then
        return false, "商品已售罄"
    end
    
    return true
end

-- 请求频率限制（单IP）
local function check_rate_limit(red, client_ip, product_id)
    local rate_key = "seckill:rate:" .. product_id .. ":" .. client_ip
    local count, err = red:incr(rate_key)
    
    if err then
        ngx.log(ngx.WARN, "failed to incr rate: ", err)
        return true
    end
    
    if count == 1 then
        red:expire(rate_key, 1) -- 1秒窗口
    end
    
    if count > 5 then -- 每秒最多5次请求
        return false, "请求过于频繁"
    end
    
    return true
end

-- 主逻辑
local function main()
    -- 获取商品ID
    local product_id = get_product_id()
    if not product_id then
        ngx.status = 400
        ngx.say(cjson.encode({code = 400, message = "缺少商品ID"}))
        return ngx.exit(400)
    end
    
    -- 获取客户端IP
    local client_ip = ngx.var.remote_addr
    
    -- 连接Redis
    local red, err = get_redis_connection()
    if not red then
        -- Redis连接失败，放行请求（降级策略）
        ngx.log(ngx.WARN, "redis connection failed, passing request")
        return
    end
    
    -- 检查请求频率
    local ok, msg = check_rate_limit(red, client_ip, product_id)
    if not ok then
        ngx.status = 429
        ngx.say(cjson.encode({code = 429, message = msg}))
        return ngx.exit(429)
    end
    
    -- 检查商品状态
    ok, msg = check_product_status(red, product_id)
    if not ok then
        ngx.status = 400
        ngx.say(cjson.encode({code = 400, message = msg}))
        return ngx.exit(400)
    end
    
    -- 检查库存
    ok, msg = check_stock(red, product_id)
    if not ok then
        ngx.status = 400
        ngx.say(cjson.encode({code = 400, message = msg}))
        return ngx.exit(400)
    end
    
    -- 放行请求
    -- 将连接放回连接池
    red:set_keepalive(10000, 100)
end

-- 执行
local ok, err = pcall(main)
if not ok then
    ngx.log(ngx.ERR, "seckill filter error: ", err)
    -- 出错时放行请求
end
