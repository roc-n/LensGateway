package test

import (
	"bytes"
	"fmt"
	"io"
	"log"
	"net"
	"net/http/httptest"
	"os"
	"time"

	"LensGateway.com/internal/config"
	"LensGateway.com/internal/core"
	_ "LensGateway.com/internal/logging"
	"LensGateway.com/internal/middleware"
	"github.com/gin-gonic/gin"
)

const confPath = "test/gateway_test.yaml"

func createTestBackend(addr ...string) *httptest.Server {
	engine := gin.New()
	engine.GET("/", func(c *gin.Context) { c.String(200, "ok") })

	if len(addr) > 0 && addr[0] != "" {
		// use a specific address
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

// withStdoutCapture captures stdout for the duration of a function call.
func withStdoutCapture(fn func() error) (vals map[string]any, err error) {
	// Before calling the function, redirect stdout.
	oldStdout := os.Stdout
	r, w, pipeErr := os.Pipe()
	if pipeErr != nil {
		return nil, fmt.Errorf("failed to create pipe: %w", pipeErr)
	}
	os.Stdout = w

	// Execute the function and capture its error.
	err = fn()

	// After the function call, restore stdout.
	os.Stdout = oldStdout
	if err != nil {
		r.Close()
		w.Close()
		return nil, err
	}

	vals["r"] = r
	vals["w"] = w

	return vals, nil
}

func setupGatewayCore(exfn func(fn func() error) (map[string]any, error)) (gatewaySrv, backendSrv *httptest.Server, vals map[string]any, err error) {
	// create backend server & load gateway config
	backend := createTestBackend("127.0.0.1:8081")
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
	srv := httptest.NewServer(router)

	return srv, backend, vals, nil
}

// Create and start a gateway instance backed by an httptest server.
// It returns the server URL, a stop function (to close servers and cleanup),
// a bytes.Buffer that receives captured stdout (where logging writes), and an error.
// func setupGateway() (url string, stop func(), logs *bytes.Buffer, err error) {
// 	// create backend server & load gateway config
// 	backend := createTestBackend("127.0.0.1:8081")
// 	conf, _ := config.LoadConfig(confPath)

// 	// build gin router
// 	router := gin.New()
// 	router.GET("/healthz", func(c *gin.Context) { c.JSON(200, gin.H{"status": "ok"}) })

// 	// build router manager
// 	routerManager, err := core.NewRouterManager(conf.Upstreams, conf.ConfigSource)
// 	if err != nil {
// 		backend.Close()
// 		return "", nil, nil, err
// 	}
// 	router.Use(routerManager.PreMatchMiddleware())

// 	// 1. Capture stdout so that logging.NewService which writes to os.Stdout will be captured.
// 	r, w, err := withStdoutCapture(func() error {
// 		return middleware.SetupMiddlewares(router, conf.Middlewares)
// 	})

// 	if err != nil {
// 		// restore and cleanup
// 		r.Close()
// 		w.Close()
// 		backend.Close()
// 		return "", nil, nil, err
// 	}

// 	router.NoRoute(routerManager.HandleRequest)
// 	srv := httptest.NewServer(router)

// 	// Start goroutine to copy from pipe reader into a buffer.
// 	buf := &bytes.Buffer{}
// 	done := make(chan struct{})
// 	go func() {
// 		_, _ = io.Copy(buf, r)
// 		close(done)
// 	}()

// 	stopFunc := func() {
// 		// Close server and backend
// 		srv.Close()
// 		backend.Close()
// 		// Close writer to unblock reader
// 		w.Close()
// 		// Wait a short time for copy to finish
// 		select {
// 		case <-done:
// 		case <-time.After(200 * time.Millisecond):
// 		}
// 	}

// 	return srv.URL, stopFunc, buf, nil
// }

func setupForLoggingTest() (url string, stop func(), logs *bytes.Buffer, err error) {
	gateSrv, backendSrv, vals, err := setupGatewayCore(withStdoutCapture)

	r, w := vals["r"].(*os.File), vals["w"].(*os.File)

	// Start goroutine to copy from pipe reader into a buffer.
	buf := &bytes.Buffer{}
	done := make(chan struct{})
	go func() {
		_, _ = io.Copy(buf, r)
		close(done)
	}()

	stopFunc := func() {
		gateSrv.Close()
		backendSrv.Close()
		// close writer to unblock reader
		w.Close()
		// wait a short time for copy to finish
		select {
		case <-done:
		case <-time.After(200 * time.Millisecond):
		}
	}
	return gateSrv.URL, stopFunc, buf, err
}
