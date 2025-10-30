package main

import (
	"context"
	"log"
	"time"

	"LensGateway.com/internal/config"
	"LensGateway.com/internal/core"
	"LensGateway.com/internal/middleware"
	"github.com/gin-gonic/gin"
)

func main() {
	// 加载配置
	conf, err := config.LoadConfig("./config/gateway.yaml")
	if err != nil {
		log.Fatalf("Failed to load config: %v", err)
	}

	// 初始化路由
	router := gin.Default()

	// 健康检查
	router.GET("/healthz", func(c *gin.Context) { c.JSON(200, gin.H{"status": "ok"}) })

	// 设置动态路由和反向代理（支持 etcd 动态上游）
	var routerManager *core.RouterManager
	if conf.ConfigSource.Type == "etcd" && len(conf.ConfigSource.Etcd.Endpoints) > 0 {
		etcdCli, err := config.NewEtcdClient(conf.ConfigSource.Etcd.Endpoints)
		if err != nil {
			log.Fatalf("Failed to connect etcd: %v", err)
		}
		// 初次拉取
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		ups, err := etcdCli.FetchUpstreams(ctx, conf.ConfigSource.Etcd.Key)
		cancel()
		if err != nil {
			log.Fatalf("Failed to fetch upstreams from etcd: %v", err)
		}
		routerManager, err = core.NewRouterManager(ups, conf.ConfigSource)
		if err != nil {
			log.Fatalf("Failed to create router manager: %v", err)
		}
		// 监听变更
		if conf.ConfigSource.Etcd.Watch {
			go func() {
				log.Printf("[gateway] watching etcd key %s for upstream updates", conf.ConfigSource.Etcd.Key)
				if err := etcdCli.WatchUpstreams(context.Background(), conf.ConfigSource.Etcd.Key, func(newUps []config.UpstreamConfig) {
					routerManager.UpdateUpstreams(newUps)
					log.Printf("[gateway] upstreams updated: %d", len(newUps))
				}); err != nil {
					log.Printf("[gateway] etcd watch stopped: %v", err)
				}
			}()
		}
	} else {
		// 文件模式
		routerManager, err = core.NewRouterManager(conf.Upstreams, conf.ConfigSource)
		if err != nil {
			log.Fatalf("Failed to create router manager: %v", err)
		}
	}

	// 在注册用户中间件之前，先挂载路由预匹配中间件，便于后续限流按 route.prefix 生效
	router.Use(routerManager.PreMatchMiddleware())

	// 注册中间件（注意：gin.Default() 已含默认日志/恢复中间件，可能与自定义日志重复）
	err = middleware.SetupMiddlewares(router, conf.Middlewares)
	if err != nil {
		log.Fatalf("Failed to setup middlewares: %v", err)
	}
	// 设置一个通用路由，所有请求都由RouterManager处理
	router.NoRoute(routerManager.HandleRequest)

	// 启动服务器
	log.Printf("Lens Gateway starting on %s", conf.Global.ListenAddr)
	if err := router.Run(conf.Global.ListenAddr); err != nil {
		log.Fatal(err)
	}
}
