package test

import (
	"fmt"
	"log"
	"net"
	"net/http/httptest"

	"LensGateway.com/internal/config"
	"LensGateway.com/internal/core"
	_ "LensGateway.com/internal/logging"
	"LensGateway.com/internal/middleware"
	"github.com/gin-gonic/gin"
)

const confPath = "gateway_test.yaml"

func createTestBackend(addr ...string) *httptest.Server {
	// create a simple backend server that echoes requests
	engine := gin.New()
	engine.Any("/*any", func(c *gin.Context) {
		method := c.Request.Method
		path := c.Param("any")
		c.String(200, fmt.Sprintf("ok %s %s", method, path))
	})

	// use a specific address
	if len(addr) > 0 && addr[0] != "" {
		l, err := net.Listen("tcp", addr[0])
		if err != nil {
			panic(fmt.Sprintf("failed to listen on %s: %v", addr[0], err))
		}
		server := httptest.NewUnstartedServer(engine)
		server.Listener = l
		server.Start()
		return server
	}

	// default use a random port
	return httptest.NewServer(engine)
}

func setupGatewayCore(exfn func(fn func() error) (map[string]any, error)) (gatewaySrv, backendSrv *httptest.Server, vals map[string]any, err error) {
	// create backend server & load gateway config
	backend := createTestBackend("localhost:8081")
	conf, _ := config.LoadConfig(confPath)

	// build gin router
	router := gin.New()
	router.GET("/healthz", func(c *gin.Context) { c.JSON(200, gin.H{"status": "ok"}) })
	routerManager, err := core.NewRouterManager(conf.Upstreams, conf.ConfigSource)
	if err != nil {
		backend.Close()
		return nil, nil, nil, err
	}
	router.Use(routerManager.PreMatchMiddleware())

	// execute external operations for specific test cases
	if exfn != nil {
		vals, err = exfn(func() error {
			return middleware.SetupMiddlewares(router, conf.Middlewares)
		})
	} else {
		err = middleware.SetupMiddlewares(router, conf.Middlewares)
	}
	if err != nil {
		backend.Close()
		log.Fatalf("failed to setup middlewares: %v", err)
		return nil, nil, nil, err
	}

	router.NoRoute(routerManager.HandleRequest)
	gatewaySrv = httptest.NewServer(router)

	return gatewaySrv, backend, vals, nil
}
