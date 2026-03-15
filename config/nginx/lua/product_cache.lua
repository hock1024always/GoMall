-- product_cache.lua
-- 商品缓存脚本
-- 在网关层缓存商品信息，减少后端压力

local redis = require "resty.redis"
local cjson = require "cjson"

-- 从URI中提取商品ID
local function get_product_id()
    local uri = ngx.var.uri
    local matches = ngx.re.match(uri, "/api/product/(\\d+)")
    if matches then
        return matches[1]
    end
    return ngx.var.arg_id
end

-- 连接Redis
local function get_redis_connection()
    local red = redis:new()
    red:set_timeout(1000)
    
    local ok, err = red:connect("redis-slave", 6379)
    if not ok then
        ngx.log(ngx.ERR, "failed to connect to redis: ", err)
        return nil, err
    end
    return red, nil
end

-- 从Redis获取商品缓存
local function get_product_cache(red, product_id)
    local cache_key = "cache:product:" .. product_id
    local cache, err = red:get(cache_key)
    
    if err then
        ngx.log(ngx.WARN, "failed to get product cache: ", err)
        return nil, err
    end
    
    if cache ~= ngx.null then
        return cache
    end
    
    return nil
end

-- 设置响应头
local function set_cache_headers(ttl)
    ngx.header["X-Cache"] = "HIT"
    ngx.header["Cache-Control"] = "public, max-age=" .. ttl
end

-- 主逻辑
local function main()
    -- 只缓存GET请求
    if ngx.req.get_method() ~= "GET" then
        return
    end
    
    -- 获取商品ID
    local product_id = get_product_id()
    if not product_id then
        return
    end
    
    -- 连接Redis
    local red, err = get_redis_connection()
    if not red then
        return
    end
    
    -- 尝试获取缓存
    local cache = get_product_cache(red, product_id)
    if cache then
        set_cache_headers(60) -- 缓存60秒
        ngx.header["Content-Type"] = "application/json"
        ngx.say(cache)
        red:set_keepalive(10000, 100)
        return ngx.exit(200)
    end
    
    -- 缓存未命中，放行请求
    ngx.header["X-Cache"] = "MISS"
    red:set_keepalive(10000, 100)
end

-- 执行
local ok, err = pcall(main)
if not ok then
    ngx.log(ngx.ERR, "product cache error: ", err)
end
