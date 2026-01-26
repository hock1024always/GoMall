# gomall
字节跳动青训营后端项目---抖音商城
# 项目优化点

## 架构设计模式使用

也就是我们尝试使用OLTP OLAP两种架构思维
1.  使用OLTP 维护数据的高一致性 应用于秒杀等场景
2.  使用OLAP 增加关于趋势预测和数据批量计算的新场景

注意：两种设计需要分别在不同的数据库实例里面进行，不得并行对同一个数据库进行操作，否则会适得其反

### 设计思维介绍
#### OLTP
- 在存储上，以窄表为主
    -  严格使用数据库三范式：实现写入快，修改快，少冗余，事务完整的效果
    -  使用行式存储Mysql或者Oracle：方便对于某用户的信息快速索引或者查询

#### OLAP
- 在存储上使，用宽表为主
    - 读取快（覆盖索引），冗余大，以空间换取时间 
    - 使用列式存储ClickHouse等，方便对同类型数据的快速统计

### 两种设计思维的应用场景



## 主存储优化
尝试在热场景下使用Redis代替Mysql作为主存储

优势：

- 解决Cache Aside旁路缓存模式之下的高并发场景下的一致性灾难问题
- 尝试借助内存级IO实现性能的机制提升

### 主要可行性问题
1. 数据丢失风险问题：使用cluster集群+AOF实时备份+多副本
    - 对于Redis被理解的异步刷盘问题，将appendfsync设置成always和everysec
    - 主从秒级切换 借助哨兵机制



2. Redis不支持复杂数据索引查询
    - 借助Redis Stack拓展包的支持性，借助其search模块的倒排索引等功能  



3. 内存成本高

    - 采用冷热分离的方式 借助redis on flash 或者 kvrocks 自动的将冷数据沉降到SSD硬盘上

### 应用场景——秒杀

传统的： 

请求进来之后，全量打入BG Server（后端服务），然后后端服务旁路缓存到Redis 主干持久化存储到Mysql中

新架构：

1. 首先使用Nginx+Redis从库部署到同一单机作为网关层。设置product_1001_status = finish  就可以放置无效请求进入Bg server
    
    - Nginx网关配合OpenResty在Nginx网关中调启Lua脚本，读Redis的标志位

    - 这样的Nginx与Redis从在同一个节点的部署计划，也避免了大量网络IO造成的性能损耗 

2. 后端连Redis主库 主库可以将数据同步给从节点

    - 如果多个Redis从+Nginx单机 可以达到什么效果呢？有意思


## Casbin权限模式的使用

### 实现场景

在 `auth` 服务中实现了基于 Casbin 的 RBAC（基于角色的访问控制）权限管理系统。

**实现内容：**
- 使用 GORM Adapter 将权限策略存储在 MySQL 数据库中，支持动态策略管理
- 实现了 JWT 认证中间件和 Casbin 权限中间件，形成完整的认证授权链
- 支持三种角色：admin（管理员）、seller（商家）、customer（普通用户）
- 支持资源所有者权限检查（动态策略），例如用户只能操作自己的订单

**权限模型：**
- Admin：拥有所有资源的增删改查权限
- Seller：拥有商品管理权限和订单查看权限
- Customer：拥有商品查看、订单管理（自己的订单）、购物车操作权限

**技术实现：**
- 权限策略存储在 `casbin_rule` 表中（通过 GORM Adapter 自动创建）
- 启动时如果数据库为空，自动加载默认策略
- 支持通过 API 动态添加/删除策略和分配角色

**文件位置：**
- `app/auth/biz/middleware/casbin_middleware.go` - Casbin 权限中间件
- `app/auth/biz/middleware/jwt_middleware.go` - JWT 认证中间件
- `app/auth/biz/service/policy_service.go` - 权限策略管理服务
- `app/auth/conf/casbin/model.conf` - Casbin 权限模型配置

## Saga分布式事务的使用

### 实现场景

在 `checkout` 服务中实现了 Saga 分布式事务模式，解决跨服务数据一致性问题。

**问题背景：**
原有的 checkout 流程：减库存 → 创建订单 → 清空购物车 → 支付 → 更新订单状态
如果支付失败，订单已创建、库存已扣减，但没有回滚机制，导致数据不一致。

**Saga 解决方案：**
使用 Saga 编排模式，每个步骤都有对应的补偿操作，失败时按逆序执行补偿。

**实现内容：**
1. **Saga 协调器** (`common/saga/coordinator.go`)
   - 管理 Saga 执行流程
   - 实现步骤执行、补偿执行、重试机制
   - 支持指数退避重试策略

2. **Redis 存储层** (`common/saga/storage_redis.go`)
   - 将 Saga 实例状态存储在 Redis 中
   - 支持 Saga 状态查询和历史日志查看
   - 实例过期时间 24 小时

3. **Checkout Saga 步骤** (`app/checkout/biz/saga/checkout_saga.go`)
   - **步骤 1：扣减库存** → 补偿：恢复库存
   - **步骤 2：创建订单** → 补偿：删除订单
   - **步骤 3：清空购物车** → 补偿：恢复购物车
   - **步骤 4：支付** → 补偿：退款
   - **步骤 5：标记订单已支付** → 补偿：取消支付标记

**执行流程：**
```
用户请求 → Checkout Service → Saga 协调器
  ↓
步骤 1: 扣减库存 (Redis Lua 脚本保证原子性)
  ↓ 成功
步骤 2: 创建订单 (Order Service)
  ↓ 成功
步骤 3: 清空购物车 (Cart Service)
  ↓ 成功
步骤 4: 支付 (Payment Service)
  ↓ 成功
步骤 5: 标记订单已支付 (Order Service)
  ↓
完成
```

**失败补偿流程：**
如果步骤 4（支付）失败：
```
步骤 4 失败
  ↓
触发补偿（逆序执行）：
  ↓
补偿步骤 3: 恢复购物车
  ↓
补偿步骤 2: 删除订单
  ↓
补偿步骤 1: 恢复库存
  ↓
Saga 状态：失败
```

**技术特点：**
- 每个步骤支持自定义重试策略（默认：3次重试，指数退避）
- Saga 实例状态实时保存到 Redis，支持查询和监控
- 详细的执行日志记录，便于问题排查
- 补偿操作设计为幂等操作，支持重复执行

**文件位置：**
- `common/saga/` - Saga 核心模块
  - `coordinator.go` - Saga 协调器
  - `storage.go` - 存储接口
  - `storage_redis.go` - Redis 存储实现
  - `types.go` - 类型定义
  - `errors.go` - 错误定义
- `app/checkout/biz/saga/checkout_saga.go` - Checkout Saga 实现
- `app/checkout/biz/service/checkout.go` - 集成 Saga 的 Checkout Service
- `app/checkout/test/saga_test.go` - Saga 单元测试

**测试：**
- 单元测试：`go test ./app/checkout/test -v`
- 集成测试：参考 `docs/08-Saga测试指南.md`

## DDD领域驱动划分（先不管）