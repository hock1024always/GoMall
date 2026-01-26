package main

import (
	"fmt"
	"net"
	"path/filepath"
	"time"

	"github.com/casbin/casbin/v2"
	gormadapter "github.com/casbin/gorm-adapter/v3"
	"github.com/cloudwego/kitex/pkg/klog"
	"github.com/cloudwego/kitex/pkg/rpcinfo"
	"github.com/cloudwego/kitex/server"
	kitexlogrus "github.com/kitex-contrib/obs-opentelemetry/logging/logrus"
	"github.com/v2pro/plz/countlog/output/lumberjack"
	"github.com/xvxiaoman8/gomall/app/auth/biz/middleware"
	userMysql "github.com/xvxiaoman8/gomall/app/user/biz/dal/mysql"
	"github.com/xvxiaoman8/gomall/app/user/conf"
	"github.com/xvxiaoman8/gomall/rpc_gen/kitex_gen/auth/authservice"
	"go.uber.org/zap/zapcore"
)

func main() {
	// 初始化数据库（通过 user 服务）
	userMysql.Init()

	opts := kitexInit()

	// 初始化 Casbin
	enforcer := initCasbin()

	// 创建中间件链
	opts = append(opts, server.WithMiddleware(
		middleware.ChainMiddleware(
			middleware.JWTAuthMiddleware,
			middleware.CasbinMiddleware(enforcer),
		),
	))

	svr := authservice.NewServer(new(AuthServiceImpl), opts...)

	err := svr.Run()
	if err != nil {
		klog.Error(err.Error())
	}
}

func kitexInit() (opts []server.Option) {
	// address
	addr, err := net.ResolveTCPAddr("tcp", conf.GetConf().Kitex.Address)
	if err != nil {
		panic(err)
	}
	opts = append(opts, server.WithServiceAddr(addr))

	// service info
	opts = append(opts, server.WithServerBasicInfo(&rpcinfo.EndpointBasicInfo{
		ServiceName: conf.GetConf().Kitex.Service,
	}))

	// klog
	logger := kitexlogrus.NewLogger()
	klog.SetLogger(logger)
	klog.SetLevel(conf.LogLevel())
	asyncWriter := &zapcore.BufferedWriteSyncer{
		WS: zapcore.AddSync(&lumberjack.Logger{
			Filename:   conf.GetConf().Kitex.LogFileName,
			MaxSize:    conf.GetConf().Kitex.LogMaxSize,
			MaxBackups: conf.GetConf().Kitex.LogMaxBackups,
			MaxAge:     conf.GetConf().Kitex.LogMaxAge,
		}),
		FlushInterval: time.Minute,
	}
	klog.SetOutput(asyncWriter)
	server.RegisterShutdownHook(func() {
		asyncWriter.Sync()
	})
	return
}

func initCasbin() *casbin.Enforcer {
	modelPath := filepath.Join("conf/casbin", "model.conf")
	
	// 使用 GORM Adapter 从数据库加载策略
	adapter, err := gormadapter.NewAdapterByDB(userMysql.DB)
	if err != nil {
		panic(fmt.Errorf("failed to create casbin adapter: %w", err))
	}

	enforcer, err := casbin.NewEnforcer(modelPath, adapter)
	if err != nil {
		panic(fmt.Errorf("failed to create casbin enforcer: %w", err))
	}

	// 加载策略
	err = enforcer.LoadPolicy()
	if err != nil {
		panic(fmt.Errorf("failed to load policy: %w", err))
	}

	// 如果数据库为空，加载默认策略
	if len(enforcer.GetPolicy()) == 0 {
		loadDefaultPolicies(enforcer)
	}

	return enforcer
}

func loadDefaultPolicies(enforcer *casbin.Enforcer) {
	// Admin 权限
	enforcer.AddPolicy("admin", "product", "create")
	enforcer.AddPolicy("admin", "product", "read")
	enforcer.AddPolicy("admin", "product", "update")
	enforcer.AddPolicy("admin", "product", "delete")
	enforcer.AddPolicy("admin", "order", "read")
	enforcer.AddPolicy("admin", "order", "update")
	enforcer.AddPolicy("admin", "user", "read")
	enforcer.AddPolicy("admin", "user", "update")

	// Seller 权限
	enforcer.AddPolicy("seller", "product", "create")
	enforcer.AddPolicy("seller", "product", "read")
	enforcer.AddPolicy("seller", "product", "update")
	enforcer.AddPolicy("seller", "order", "read")

	// Customer 权限
	enforcer.AddPolicy("customer", "product", "read")
	enforcer.AddPolicy("customer", "order", "create")
	enforcer.AddPolicy("customer", "order", "read")
	enforcer.AddPolicy("customer", "order", "update")
	enforcer.AddPolicy("customer", "cart", "create")
	enforcer.AddPolicy("customer", "cart", "read")
	enforcer.AddPolicy("customer", "cart", "update")
	enforcer.AddPolicy("customer", "cart", "delete")

	// 保存策略到数据库
	err := enforcer.SavePolicy()
	if err != nil {
		klog.Errorf("failed to save default policies: %v", err)
	}
}
