package main

import (
	"context"
	"log"
	"time"

	"LensGateway.com/internal/config"
	"LensGateway.com/internal/core"
	_ "LensGateway.com/internal/logging"
	"LensGateway.com/internal/middleware"
	"github.com/common-nighthawk/go-figure"
	"github.com/gin-gonic/gin"
)

func main() {
	// Load configuration.
	conf, err := config.LoadConfig("./config/gateway.yaml")
	if err != nil {
		log.Fatalf("Failed to load config: %v", err)
	}

	// Initialize router using gin.New() for full middleware control.
	router := gin.New()
	// Set trusted proxies if configured
	trustedProxies := conf.Global.TrustedProxies
	if len(trustedProxies) == 0 {
		trustedProxies = []string{"127.0.0.1", "::1"}
	}
	err = router.SetTrustedProxies(trustedProxies)
	if err != nil {
		log.Fatalf("Failed to set trusted proxies: %v", err)
	}

	// Gateway health check endpoint.
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

	// register other global middlewares
	err = middleware.SetupMiddlewares(router, conf.Middlewares)
	if err != nil {
		log.Fatalf("Failed to setup middlewares: %v", err)
	}
	// capture all routes except configured ones(e.g. /healthz)
	router.NoRoute(routerManager.HandleRequest)

	// start server
	fig := figure.NewFigure("LensGateway", "", true)
	fig.Print()
	if err := router.Run(conf.Global.ListenAddr); err != nil {
		log.Fatal(err)
	}
}
