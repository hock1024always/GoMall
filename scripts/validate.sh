#!/bin/bash

set -e

echo "=== Gomall 项目重构验证 ==="

cd /home/hyz/AI/Gomall

echo "1. 检查新文件存在性..."
FILES=(
    "common/permission/manager.go"
    "common/permission/context.go"
    "common/permission/middleware.go"
    "common/permission/init.go"
    "common/permission/conf/model.conf"
    "common/redis/client.go"
    "common/redis/stock.go"
    "common/redis/cart.go"
    "common/clickhouse/client.go"
    "common/events/publisher.go"
    "common/events/subscriber.go"
    "app/order/biz/service/cancel_order.go"
    "app/checkout/biz/service/get_saga_status.go"
    "config/redis/master.conf"
    "config/redis/slave.conf"
    "config/redis/sentinel.conf"
    "config/nginx/nginx.conf"
    "config/nginx/lua/seckill_filter.lua"
    "config/nginx/lua/product_cache.lua"
    "config/clickhouse/init.sql"
    "docs/11-落地工程文档.md"
)

for file in "${FILES[@]}"; do
    if [ -f "$file" ]; then
        echo "✓ $file"
    else
        echo "✗ $file (缺失)"
    fi
done

echo ""
echo "2. 检查修改的文件..."
MODIFIED_FILES=(
    "docker-compose.yaml"
    "db/sql/ini/databases.sql"
    "idl/order.proto"
    "idl/checkout.proto"
    "app/checkout/biz/saga/checkout_saga.go"
    "app/checkout/handler.go"
    "app/order/handler.go"
    "app/auth/main.go"
    "README.md"
)

for file in "${MODIFIED_FILES[@]}"; do
    if [ -f "$file" ]; then
        echo "✓ $file (存在)"
    else
        echo "✗ $file (缺失)"
    fi
done

echo ""
echo "3. 检查Lua脚本语法..."
if command -v luac >/dev/null 2>&1; then
    luac -p config/nginx/lua/seckill_filter.lua && echo "✓ seckill_filter.lua 语法正确"
    luac -p config/nginx/lua/product_cache.lua && echo "✓ product_cache.lua 语法正确"
else
    echo "? 无法验证Lua语法 (luac未安装)"
fi

echo ""
echo "4. 检查配置文件..."
if [ -f "config/nginx/nginx.conf" ]; then
    echo "✓ nginx.conf 存在"
fi

if [ -f "config/redis/master.conf" ]; then
    echo "✓ Redis master.conf 存在"
fi

if [ -f "config/clickhouse/init.sql" ]; then
    echo "✓ ClickHouse init.sql 存在"
fi

echo ""
echo "=== 验证完成 ==="
echo ""
echo "下一步建议:"
echo "1. 运行: docker-compose up -d"
echo "2. 运行: make gen-order gen-checkout"
echo "3. 运行: go test ./..."
