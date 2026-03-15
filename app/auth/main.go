package main

import (
	"fmt"
	"net"
	"time"

	"github.com/cloudwego/kitex/pkg/klog"
	"github.com/cloudwego/kitex/pkg/rpcinfo"
	"github.com/cloudwego/kitex/server"
	kitexlogrus "github.com/kitex-contrib/obs-opentelemetry/logging/logrus"
	"github.com/v2pro/plz/countlog/output/lumberjack"
	"github.com/xvxiaoman8/gomall/app/auth/biz/middleware"
	userMysql "github.com/xvxiaoman8/gomall/app/user/biz/dal/mysql"
	"github.com/xvxiaoman8/gomall/app/user/conf"
	"github.com/xvxiaoman8/gomall/common/permission"
	"github.com/xvxiaoman8/gomall/rpc_gen/kitex_gen/auth/authservice"
	"go.uber.org/zap/zapcore"
)

var (
	permissionManager *permission.PermissionManager
)

func main() {
	// 初始化数据库（通过 user 服务）
	userMysql.Init()

	opts := kitexInit()

	// 初始化权限管理器
	var err error
	permissionManager, err = permission.InitPermission(userMysql.DB, "conf/casbin")
	if err != nil {
		panic(fmt.Errorf("failed to init permission: %w", err))
	}

	// 创建中间件链
	opts = append(opts, server.WithMiddleware(
		middleware.ChainMiddleware(
			middleware.JWTAuthMiddleware,
			middleware.CasbinMiddleware(permissionManager.GetEnforcer()),
		),
	))

	svr := authservice.NewServer(new(AuthServiceImpl), opts...)

	err = svr.Run()
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
