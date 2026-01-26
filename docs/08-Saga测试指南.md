# Saga 分布式事务测试指南

## 测试环境准备

### 1. 启动依赖服务

```bash
# 启动 Redis
docker-compose up -d redis

# 启动 MySQL
docker-compose up -d mysql

# 启动其他服务（Cart, Order, Payment, Product）
# 确保所有微服务都已启动
```

### 2. 初始化库存数据

在测试前，需要初始化商品库存：

```go
// 在 checkout service 中调用
service.InitStock(ctx, productID, 100) // 初始化商品库存为 100
```

## 单元测试

### 运行 Saga 单元测试

```bash
cd app/checkout
go test ./test -v
```

### 测试用例说明

1. **TestSagaCoordinator** - 测试 Saga 协调器基本功能
   - 验证步骤正常执行
   - 验证实例状态保存

2. **TestSagaCompensation** - 测试 Saga 补偿功能
   - 验证步骤失败时触发补偿
   - 验证补偿顺序（逆序）

3. **TestCheckoutSagaBuild** - 测试 Checkout Saga 构建
   - 验证步骤数量
   - 验证步骤顺序
   - 验证上下文初始化

4. **TestSagaRetry** - 测试 Saga 重试机制
   - 验证重试次数
   - 验证指数退避

## 集成测试

### 测试正常流程

```bash
# 1. 添加商品到购物车
curl -X POST http://localhost:8080/api/cart/add \
  -H "Content-Type: application/json" \
  -d '{
    "user_id": 1,
    "item": {
      "product_id": 1,
      "quantity": 2
    }
  }'

# 2. 执行 checkout（应该成功）
curl -X POST http://localhost:8080/api/checkout \
  -H "Content-Type: application/json" \
  -d '{
    "user_id": 1,
    "email": "test@example.com",
    "credit_card": {
      "credit_card_number": "1234567890123456",
      "credit_card_expiration_year": 2025,
      "credit_card_expiration_month": 12,
      "credit_card_cvv": 123
    }
  }'
```

### 测试失败场景

#### 场景 1: 库存不足

```bash
# 1. 初始化库存为 1
# 2. 尝试购买 2 件商品
# 预期：失败，触发补偿，库存恢复
```

#### 场景 2: 支付失败

```bash
# 1. 使用无效的信用卡信息
# 预期：支付失败，触发补偿：
#   - 退款（如果已支付）
#   - 恢复购物车
#   - 取消订单
#   - 恢复库存
```

#### 场景 3: 订单创建失败

```bash
# 1. 模拟 Order Service 不可用
# 预期：订单创建失败，触发补偿：
#   - 恢复库存
```

## 验证 Saga 状态

### 查询 Saga 实例

```go
// 在代码中查询
instance, err := sagaStorage.Get(ctx, sagaID)
if err != nil {
    log.Fatal(err)
}

fmt.Printf("Saga ID: %s\n", instance.ID)
fmt.Printf("Status: %s\n", instance.Status)
fmt.Printf("Current Step: %d\n", instance.CurrentStep)
fmt.Printf("Logs: %+v\n", instance.Logs)
```

### 查看 Redis 中的 Saga 数据

```bash
# 连接 Redis
redis-cli

# 查看所有 Saga 实例
KEYS saga:instance:*

# 查看特定 Saga 实例
GET saga:instance:{saga_id}
```

## 监控和日志

### 查看 Saga 日志

Saga 执行过程中的日志会记录：
- 每个步骤的执行状态
- 补偿操作的执行情况
- 错误信息

```bash
# 查看 checkout service 日志
tail -f logs/checkout.log | grep saga
```

### 关键日志信息

- `Starting checkout saga: {saga_id}` - Saga 开始
- `step {step_name} failed: {error}` - 步骤失败
- `compensate step {step_name}` - 执行补偿
- `Checkout saga completed successfully` - Saga 完成
- `Saga execution failed` - Saga 失败

## 性能测试

### 并发测试

```go
// 并发执行多个 checkout
for i := 0; i < 100; i++ {
    go func() {
        // 执行 checkout
    }()
}
```

### 压力测试

使用工具如 `wrk` 或 `ab` 进行压力测试：

```bash
wrk -t12 -c400 -d30s http://localhost:8080/api/checkout
```

## 故障注入测试

### 模拟服务故障

1. **停止 Order Service**
   ```bash
   docker-compose stop order
   ```

2. **停止 Payment Service**
   ```bash
   docker-compose stop payment
   ```

3. **停止 Redis**
   ```bash
   docker-compose stop redis
   ```

### 验证补偿机制

在服务故障时，验证：
1. 已执行的步骤是否正确补偿
2. 补偿顺序是否正确（逆序）
3. 数据一致性是否恢复

## 常见问题

### 1. Saga 实例未找到

**原因**: Redis 连接失败或实例已过期（24小时）

**解决**: 检查 Redis 连接，确认实例未过期

### 2. 补偿失败

**原因**: 补偿操作本身失败

**解决**: 检查补偿逻辑，确保补偿操作是幂等的

### 3. 步骤重试次数过多

**原因**: 步骤持续失败

**解决**: 检查步骤逻辑，调整重试策略

## 测试检查清单

- [ ] 正常流程测试通过
- [ ] 库存不足场景测试通过
- [ ] 支付失败场景测试通过
- [ ] 订单创建失败场景测试通过
- [ ] 补偿机制验证通过
- [ ] 并发测试通过
- [ ] 性能测试满足要求
- [ ] 日志记录完整
- [ ] Saga 状态查询正常
