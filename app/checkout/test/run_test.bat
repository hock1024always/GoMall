@echo off
REM Saga 测试运行脚本 (Windows)
REM 使用前请确保：
REM 1. Redis 已启动
REM 2. 环境变量已配置（.env 文件）
REM 3. 所有依赖已安装

echo === 运行 Saga 单元测试 ===

REM 设置环境变量
set GO_ENV=test

REM 运行测试
cd /d %~dp0\..
go test ./test -v -run TestSaga

echo === 测试完成 ===
pause
