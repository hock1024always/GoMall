package test

import (
	"os"

	"github.com/joho/godotenv"
	"github.com/xvxiaoman8/gomall/app/checkout/biz/dal/redis"
)

func init() {
	// 加载环境变量
	_ = godotenv.Load()
	// 设置测试环境
	if os.Getenv("GO_ENV") == "" {
		os.Setenv("GO_ENV", "test")
	}
	// 初始化 Redis（如果未初始化）
	if redis.RedisClient == nil {
		redis.Init()
	}
}
