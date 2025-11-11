package main

import (
	"context"
	"flag"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"LensGateway.com/internal/config"
	"LensGateway.com/internal/core"
	_ "LensGateway.com/internal/logging"
	"LensGateway.com/internal/middleware"
	"github.com/common-nighthawk/go-figure"
	"github.com/gin-gonic/gin"
)

var (
	confPath string
)

func init() {
	flag.StringVar(&confPath, "conf", "config/gateway.yaml", "gateway config file path")
}

func main() {
	flag.Parse()

	// Load configuration.
	conf, err := config.LoadConfig(confPath)
	if err != nil {
		log.Fatalf("Failed to load config: %v", err)
	}

	// Initialize router using gin.New() for full middleware control.
	router := gin.New()
	// set trusted proxies if configured
	trustedProxies := conf.Global.TrustedProxies
	if len(trustedProxies) == 0 {
		trustedProxies = []string{"127.0.0.1", "::1"}
	}
	err = router.SetTrustedProxies(trustedProxies)
	if err != nil {
		log.Fatalf("Failed to set trusted proxies: %v", err)
	}

	// gateway health check endpoint
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
		// config file mode
		routerManager, err = core.NewRouterManager(conf.Upstreams, conf.ConfigSource)
		if err != nil {
			log.Fatalf("Failed to create router manager: %v", err)
		}
	}

	// register pre-match middleware for route prefix matching
	router.Use(routerManager.PreMatchMiddleware())

	// register other global middlewares
	err = middleware.SetupMiddlewares(router, conf.Middlewares)
	if err != nil {
		log.Fatalf("Failed to setup middlewares: %v", err)
	}
	// capture all routes except configured ones(e.g. /healthz)
	router.NoRoute(routerManager.HandleRequest)

	// Start server with graceful shutdown and hot-reload capabilities.
	srv := &http.Server{
		Addr:    conf.Global.ListenAddr,
		Handler: router,
	}

	// Goroutine for listening for signals (shutdown and reload).
	go func() {
		sig := make(chan os.Signal, 1)
		signal.Notify(sig, syscall.SIGINT, syscall.SIGTERM, syscall.SIGHUP)
		for {
			receivedSignal := <-sig
			switch receivedSignal {
			case syscall.SIGINT, syscall.SIGTERM:
				log.Println("Shutdown signal received, graceful shutdown...")
				ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
				defer cancel()
				if err := srv.Shutdown(ctx); err != nil {
					log.Fatalf("Server forced to shutdown: %v", err)
				}
				return
			case syscall.SIGHUP:
				log.Println("Reload signal (SIGHUP) received, attempting to reload configuration...")
				newConf, err := config.LoadConfig(confPath)
				if err != nil {
					log.Printf("Error reloading config, keeping the old configuration. Error: %v", err)
					continue // keep running with the old config
				}

				// In a file-based config source, we update the upstreams.
				// For etcd, the watch mechanism handles this automatically.
				if conf.ConfigSource.Type == "file" {
					routerManager.UpdateUpstreams(newConf.Upstreams)
					log.Println("Configuration reloaded successfully. Upstreams updated.")
				} else {
					log.Println("Configuration reload via SIGHUP is only supported for 'file' config source. Ignoring.")
				}
			}
		}
	}()

	// start server
	fig := figure.NewFigure("LensGateway", "", true)
	fig.Print()
	log.Printf("Server listening on %s", conf.Global.ListenAddr)
	if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Fatalf("listen: %s\n", err)
	}
	log.Println("Server exiting.")
}
