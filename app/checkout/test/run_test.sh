#!/bin/bash

# Saga 测试运行脚本
# 使用前请确保：
# 1. Redis 已启动
# 2. 环境变量已配置（.env 文件）
# 3. 所有依赖已安装

echo "=== 运行 Saga 单元测试 ==="

# 设置环境变量
export GO_ENV=test

# 运行测试
cd "$(dirname "$0")/.."
go test ./test -v -run TestSaga

echo "=== 测试完成 ==="
