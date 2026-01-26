# Casbin 权限管理方案

## 设计目标

使用 Casbin 实现细粒度的权限控制，确保用户只能访问和操作自己有权限的资源。

## 当前状态

项目已有基础的 Casbin 实现：
- `app/auth/biz/middleware/casbin_middleware.go` - Casbin 中间件
- `app/auth/conf/casbin/model.conf` - 权限模型
- `app/auth/conf/casbin/policy.csv` - 权限策略

## 权限模型设计

### RBAC 模型（基于角色的访问控制）

```
用户(User) → 角色(Role) → 权限(Permission)
```

### 权限模型配置

```conf
# app/auth/conf/casbin/model.conf

[request_definition]
r = sub, obj, act

[policy_definition]
p = sub, obj, act

[role_definition]
g = _, _  # 角色继承：g, user, role 表示 user 继承 role 的权限

[policy_effect]
e = some(where (p.eft == allow))  # 至少一条策略允许则通过

[matchers]
m = g(r.sub, p.sub) && r.obj == p.obj && r.act == p.act
```

### 角色定义

1. **admin** - 管理员
   - 所有资源的增删改查权限

2. **seller** - 商家
   - 商品管理权限
   - 订单查看权限（自己的订单）

3. **customer** - 普通用户
   - 商品查看权限
   - 订单管理权限（自己的订单）
   - 购物车权限

## 权限策略设计

### 静态策略（policy.csv）

```csv
# 角色权限定义
# p, role, resource, action

# Admin 权限
p, admin, product, create
p, admin, product, read
p, admin, product, update
p, admin, product, delete
p, admin, order, read
p, admin, order, update
p, admin, user, read
p, admin, user, update

# Seller 权限
p, seller, product, create
p, seller, product, read
p, seller, product, update
p, seller, order, read  # 只能查看自己的订单

# Customer 权限
p, customer, product, read
p, customer, order, create
p, customer, order, read
p, customer, order, update
p, customer, cart, create
p, customer, cart, read
p, customer, cart, update
p, customer, cart, delete

# 角色继承
# g, user, role

# 示例用户角色分配
g, alice, admin
g, bob, seller
g, charlie, customer
```

### 动态策略（数据库存储）

对于需要基于资源所有者的权限控制（如用户只能操作自己的订单），使用动态策略：

```go
// 动态策略格式：p, role, resource, action, owner
// 例如：p, customer, order, read, {user_id}
```

## 实现方案

### 1. 增强 Casbin 中间件

```go
// app/auth/biz/middleware/casbin_middleware.go
package middleware

import (
    "context"
    "errors"
    "fmt"
    
    "github.com/casbin/casbin/v2"
    "github.com/cloudwego/kitex/pkg/endpoint"
    "github.com/xvxiaoman8/gomall/rpc_gen/kitex_gen/auth"
)

// CasbinMiddleware Casbin 权限中间件
func CasbinMiddleware(enforcer *casbin.Enforcer) endpoint.Middleware {
    return func(next endpoint.Endpoint) endpoint.Endpoint {
        return func(ctx context.Context, req, resp interface{}) error {
            // 判断是否为分发token请求（登录/注册）
            if _, isDeliver := req.(*auth.DeliverTokenReq); isDeliver {
                return next(ctx, req, resp)
            }
            
            // 从上下文中获取用户信息
            userID, _ := ctx.Value("user_id").(int64)
            userRole, _ := ctx.Value("user_role").(string)
            resource, _ := ctx.Value("resource").(string)
            action, _ := ctx.Value("action").(string)
            
            if userRole == "" {
                userRole = "customer"  // 默认角色
            }
            
            // 构建权限检查请求
            // 格式：sub (用户/角色), obj (资源), act (操作)
            ok, err := enforcer.Enforce(userRole, resource, action)
            if err != nil {
                return fmt.Errorf("casbin enforce error: %w", err)
            }
            
            if !ok {
                // 检查是否是资源所有者权限（动态策略）
                if userID > 0 {
                    // 尝试检查资源所有者权限
                    // 格式：role, resource, action, owner
                    ok, err = enforcer.Enforce(userRole, resource, action, fmt.Sprintf("user:%d", userID))
                    if err == nil && ok {
                        return next(ctx, req, resp)
                    }
                }
                
                return errors.New("access denied: insufficient permissions")
            }
            
            return next(ctx, req, resp)
        }
    }
}
```

### 2. JWT 中间件集成

```go
// app/auth/biz/middleware/jwt_middleware.go
package middleware

import (
    "context"
    "errors"
    
    "github.com/cloudwego/kitex/pkg/endpoint"
    "github.com/xvxiaoman8/gomall/app/auth/biz/service"
)

// JWTMiddleware JWT 认证中间件
func JWTMiddleware() endpoint.Middleware {
    return func(next endpoint.Endpoint) endpoint.Endpoint {
        return func(ctx context.Context, req, resp interface{}) error {
            // 从请求中提取 token
            token := extractTokenFromRequest(req)
            if token == "" {
                return errors.New("token is required")
            }
            
            // 验证 token
            claims, err := service.VerifyToken(token)
            if err != nil {
                return errors.New("invalid token")
            }
            
            // 将用户信息放入上下文
            ctx = context.WithValue(ctx, "user_id", claims.UserID)
            ctx = context.WithValue(ctx, "user_role", claims.Role)
            ctx = context.WithValue(ctx, "user_email", claims.Email)
            
            return next(ctx, req, resp)
        }
    }
}
```

### 3. 资源权限提取中间件

```go
// app/order/biz/middleware/resource_middleware.go
package middleware

import (
    "context"
    
    "github.com/cloudwego/kitex/pkg/endpoint"
    "github.com/xvxiaoman8/gomall/rpc_gen/kitex_gen/order"
)

// OrderResourceMiddleware 订单资源权限中间件
func OrderResourceMiddleware() endpoint.Middleware {
    return func(next endpoint.Endpoint) endpoint.Endpoint {
        return func(ctx context.Context, req, resp interface{}) error {
            // 根据请求类型设置资源和操作
            switch r := req.(type) {
            case *order.PlaceOrderReq:
                ctx = context.WithValue(ctx, "resource", "order")
                ctx = context.WithValue(ctx, "action", "create")
                
            case *order.GetOrderReq:
                ctx = context.WithValue(ctx, "resource", "order")
                ctx = context.WithValue(ctx, "action", "read")
                
            case *order.UpdateOrderReq:
                ctx = context.WithValue(ctx, "resource", "order")
                ctx = context.WithValue(ctx, "action", "update")
                
            case *order.DeleteOrderReq:
                ctx = context.WithValue(ctx, "resource", "order")
                ctx = context.WithValue(ctx, "action", "delete")
            }
            
            return next(ctx, req, resp)
        }
    }
}
```

### 4. 资源所有者权限检查

```go
// app/order/biz/service/get_order.go
func (s *GetOrderService) Run(req *order.GetOrderReq) (resp *order.GetOrderResp, err error) {
    // 获取订单
    order, err := model.GetOrderByID(s.ctx, mysql.DB, req.OrderId)
    if err != nil {
        return nil, err
    }
    
    // 获取当前用户ID
    userID, _ := s.ctx.Value("user_id").(int64)
    userRole, _ := s.ctx.Value("user_role").(string)
    
    // 检查权限：admin 可以查看所有订单，其他角色只能查看自己的订单
    if userRole != "admin" && order.UserID != userID {
        return nil, errors.New("access denied: you can only access your own orders")
    }
    
    // 返回订单信息
    return &order.GetOrderResp{Order: convertOrder(order)}, nil
}
```

### 5. Casbin 初始化

```go
// app/auth/main.go
package main

import (
    "github.com/casbin/casbin/v2"
    gormadapter "github.com/casbin/gorm-adapter/v3"
    "github.com/xvxiaoman8/gomall/app/auth/biz/dal/mysql"
    "github.com/xvxiaoman8/gomall/app/auth/biz/middleware"
)

func initCasbin() (*casbin.Enforcer, error) {
    // 使用 GORM Adapter（支持数据库存储策略）
    adapter, err := gormadapter.NewAdapterByDB(mysql.DB)
    if err != nil {
        return nil, err
    }
    
    // 加载模型文件
    enforcer, err := casbin.NewEnforcer("conf/casbin/model.conf", adapter)
    if err != nil {
        return nil, err
    }
    
    // 加载策略（从数据库）
    err = enforcer.LoadPolicy()
    if err != nil {
        return nil, err
    }
    
    // 如果数据库为空，加载默认策略
    if len(enforcer.GetPolicy()) == 0 {
        loadDefaultPolicies(enforcer)
    }
    
    return enforcer, nil
}

func loadDefaultPolicies(enforcer *casbin.Enforcer) {
    // 加载默认策略
    enforcer.AddPolicy("admin", "product", "create")
    enforcer.AddPolicy("admin", "product", "read")
    enforcer.AddPolicy("admin", "product", "update")
    enforcer.AddPolicy("admin", "product", "delete")
    
    enforcer.AddPolicy("seller", "product", "create")
    enforcer.AddPolicy("seller", "product", "read")
    enforcer.AddPolicy("seller", "product", "update")
    
    enforcer.AddPolicy("customer", "product", "read")
    enforcer.AddPolicy("customer", "order", "create")
    enforcer.AddPolicy("customer", "order", "read")
    enforcer.AddPolicy("customer", "order", "update")
    
    // 保存策略
    enforcer.SavePolicy()
}
```

### 6. 在服务中使用中间件

```go
// app/order/main.go
func main() {
    // 初始化 Casbin
    enforcer, err := initCasbin()
    if err != nil {
        panic(err)
    }
    
    // 创建服务
    svc := order.NewOrderService(...)
    
    // 添加中间件链
    svc.Use(
        middleware.JWTMiddleware(),           // JWT 认证
        middleware.OrderResourceMiddleware(), // 资源提取
        middleware.CasbinMiddleware(enforcer), // 权限检查
    )
    
    // 启动服务
    svc.Run()
}
```

## 权限策略管理 API

### 1. 添加权限策略

```go
// app/auth/biz/service/add_policy.go
func AddPolicy(enforcer *casbin.Enforcer, role, resource, action string) error {
    added, err := enforcer.AddPolicy(role, resource, action)
    if err != nil {
        return err
    }
    
    if added {
        return enforcer.SavePolicy()
    }
    
    return nil
}
```

### 2. 删除权限策略

```go
func RemovePolicy(enforcer *casbin.Enforcer, role, resource, action string) error {
    removed, err := enforcer.RemovePolicy(role, resource, action)
    if err != nil {
        return err
    }
    
    if removed {
        return enforcer.SavePolicy()
    }
    
    return nil
}
```

### 3. 分配用户角色

```go
func AssignRole(enforcer *casbin.Enforcer, userID int64, role string) error {
    added, err := enforcer.AddGroupingPolicy(fmt.Sprintf("user:%d", userID), role)
    if err != nil {
        return err
    }
    
    if added {
        return enforcer.SavePolicy()
    }
    
    return nil
}
```

## 配置参数

### Casbin 配置

```yaml
# app/auth/conf/conf.yaml
casbin:
  model_path: "conf/casbin/model.conf"
  policy_adapter: "database"  # "file" | "database"
  database:
    table_name: "casbin_rule"
    auto_migrate: true
```

## 应用场景

### 场景 1: 订单操作权限

```go
// 用户只能操作自己的订单
// 策略：p, customer, order, read, {user_id}
// 检查：enforcer.Enforce("customer", "order", "read", fmt.Sprintf("user:%d", userID))
```

### 场景 2: 商品管理权限

```go
// Admin 和 Seller 可以管理商品
// 策略：p, admin, product, * 和 p, seller, product, *
// 检查：enforcer.Enforce(role, "product", action)
```

### 场景 3: 支付权限

```go
// 只有订单所有者可以支付
// 策略：p, customer, payment, create, {user_id}
// 检查：验证订单所有者 == 当前用户
```

## 实施步骤

1. ✅ 完善 Casbin 模型配置
2. ✅ 实现数据库策略存储（GORM Adapter）
3. ✅ 增强 Casbin 中间件（支持动态策略）
4. ✅ 在各服务中集成权限中间件
5. ✅ 实现资源所有者权限检查
6. ✅ 实现权限管理 API
7. ✅ 添加权限测试用例
8. ✅ 文档和示例
