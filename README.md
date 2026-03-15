# Gomall - 抖音商城后端项目

字节跳动青训营后端项目，基于云原生架构的电商后端系统。

## 项目特性

### 已实现功能

| 功能模块 | 状态 | 说明 |
|----------|------|------|
| 微服务架构 | ✅ 已完成 | 8个微服务，基于Kitex/Hertz框架 |
| Saga分布式事务 | ✅ 已完成 | 解决跨服务数据一致性，支持补偿回滚 |
| Casbin权限管理 | ✅ 已完成 | RBAC权限模型，支持动态策略 |
| Redis主存储 | ✅ 已完成 | 主从复制+Sentinel高可用+AOF持久化 |
| OpenResty网关 | ✅ 已完成 | 秒杀请求过滤，商品缓存 |
| OLTP/OLAP双架构 | ✅ 已完成 | MySQL+ClickHouse，支持实时分析 |

## 技术栈

| 类型 | 技术 |
|------|------|
| RPC框架 | Kitex |
| Web框架 | Hertz |
| OLTP数据库 | MySQL 8.0 |
| OLAP数据库 | ClickHouse |
| 主存储/缓存 | Redis 7.0 + Sentinel |
| 消息队列 | NATS |
| API网关 | OpenResty |
| 服务发现 | Consul |
| 分布式事务 | Saga模式 |
| 权限管理 | Casbin |
| 链路追踪 | Jaeger |
| 监控面板 | Grafana |

## 架构设计模式

### OLTP/OLAP双架构

项目采用OLTP和OLAP两种架构思维：

**OLTP (在线事务处理)**
- 使用MySQL作为主数据库
- 窄表设计，严格遵循三范式
- 支持高并发写入和修改
- 应用于秒杀、下单等场景

**OLAP (在线分析处理)**
- 使用ClickHouse作为分析数据库
- 宽表设计，以空间换时间
- 支持复杂查询和聚合分析
- 应用于销售趋势、用户行为分析

### Redis主存储优化

在热场景下使用Redis代替MySQL作为主存储：

**优势**
- 解决Cache Aside模式下的一致性问题
- 内存级IO实现性能提升
- 支持高并发秒杀场景

**可靠性保障**
- 主从复制 + Sentinel高可用
- AOF持久化 (appendfsync: everysec)
- 主从秒级切换

### 网关层设计

Nginx + OpenResty + Redis从库部署方案：

```
┌─────────────────────────────────────┐
│         OpenResty Gateway           │
│  ┌─────────────────────────────┐    │
│  │     Lua Script              │    │
│  │  ┌───────────────────────┐  │    │
│  │  │ Redis Slave (本地)     │  │    │
│  │  │ - 商品状态检查         │  │    │
│  │  │ - 库存预检查           │  │    │
│  │  │ - 请求频率限制         │  │    │
│  │  └───────────────────────┘  │    │
│  └─────────────────────────────┘    │
└─────────────────┬───────────────────┘
                  │
                  ▼
           Backend Services
```

## Saga分布式事务

### 执行流程

```
Checkout Saga 执行流程:

步骤1: 扣减库存 → 补偿: 恢复库存
    ↓
步骤2: 创建订单 → 补偿: 取消订单
    ↓
步骤3: 清空购物车 → 补偿: 恢复购物车
    ↓
步骤4: 支付 → 补偿: 退款
    ↓
步骤5: 标记订单已支付 → 补偿: 取消状态
```

### 文件位置

- `common/saga/` - Saga核心模块
  - `coordinator.go` - Saga协调器
  - `storage.go` - 存储接口
  - `storage_redis.go` - Redis存储实现
  - `types.go` - 类型定义
- `app/checkout/biz/saga/checkout_saga.go` - Checkout Saga实现

## Casbin权限管理

### 权限模型

| 角色 | 权限范围 |
|------|----------|
| admin | 所有资源的完全访问权限 |
| seller | 商品管理、订单查看 |
| customer | 商品查看、订单管理、购物车操作 |

### 文件位置

- `common/permission/` - 权限管理公共模块
- `app/auth/biz/middleware/casbin_middleware.go` - Casbin中间件
- `app/auth/conf/casbin/model.conf` - 权限模型配置

## 快速开始

### 环境要求

- Docker 20.10+
- Docker Compose 2.0+
- Go 1.21+

### 启动服务

```bash
# 启动基础设施
docker-compose up -d

# 编译服务
make build

# 启动所有服务
./scripts/start.sh
```

### 验证部署

```bash
# 检查服务状态
curl http://localhost:8500/v1/agent/services

# 测试API
curl http://localhost/api/product/list
```

## 项目结构

```
Gomall/
├── app/                    # 微服务应用
│   ├── auth/               # 认证授权
│   ├── cart/               # 购物车
│   ├── checkout/           # 结账
│   ├── frontend/           # 前端网关
│   ├── order/              # 订单
│   ├── payment/            # 支付
│   ├── product/            # 商品
│   └── user/               # 用户
├── common/                 # 公共模块
│   ├── clickhouse/         # ClickHouse客户端
│   ├── events/             # 事件发布/订阅
│   ├── permission/         # 权限管理
│   ├── redis/              # Redis客户端
│   └── saga/               # Saga分布式事务
├── config/                 # 配置文件
│   ├── clickhouse/         # ClickHouse配置
│   ├── nginx/              # Nginx/OpenResty配置
│   └── redis/              # Redis配置
├── db/                     # 数据库脚本
├── docs/                   # 项目文档
├── idl/                    # IDL定义
└── rpc_gen/                # 生成的RPC代码
```

## 文档目录

- [总体架构设计](docs/01-总体架构设计.md)
- [OLTP/OLAP双架构设计](docs/02-OLTP-OLAP双架构设计.md)
- [Redis主存储优化方案](docs/03-Redis主存储优化方案.md)
- [Nginx+OpenResty网关层设计](docs/04-Nginx-OpenResty网关层设计.md)
- [Saga分布式事务方案](docs/05-Saga分布式事务方案.md)
- [Casbin权限管理方案](docs/06-Casbin权限管理方案.md)
- [实施计划](docs/07-实施计划.md)
- [Saga测试指南](docs/08-Saga测试指南.md)
- [服务器配置评估](docs/09-服务器配置评估.md)
- [四大场景方案对比与面试总结](docs/10-四大场景方案对比与面试总结.md)
- [落地工程文档](docs/11-落地工程文档.md)

## 测试

```bash
# 运行单元测试
go test ./...

# 运行Saga测试
go test ./app/checkout/test -v

# 压力测试
wrk -t12 -c400 -d30s http://localhost/api/product/list
```

## License

Apache License 2.0
