package test

import (
	"bytes"
	"io"
	"net/http/httptest"
	"os"
	"time"

	"LensGateway.com/internal/config"
	"LensGateway.com/internal/core"
	_ "LensGateway.com/internal/logging"
	"LensGateway.com/internal/middleware"
	"github.com/gin-gonic/gin"
)

// TODO 分解setupGateway函数


// Create and start a gateway instance backed by an httptest server.
// It returns the server URL, a stop function (to close servers and cleanup),
// a bytes.Buffer that receives captured stdout (where logging writes), and an error.
func setupGateway() (url string, stop func(), logs *bytes.Buffer, err error) {
	// Create a dummy backend that simply responds 200 OK.
	backend := httptest.NewServer(gin.New())
	// Ensure it has a simple handler.
	backend.Config.Handler.(*gin.Engine).GET("/", func(c *gin.Context) { c.String(200, "ok") })

	// Prepare gateway config that uses the backend as upstream
	conf := &config.GatewayConfig{
		Global: config.GlobalConfig{ListenAddr: ""},
		Middlewares: map[string]config.MiddlewareConfig{
			"logging": {Enabled: true, Order: 10, Config: map[string]any{"buffer_size": 100}},
		},
		Upstreams: []config.UpstreamConfig{
			{
				Name:          "test-upstream",
				Hosts:         []string{backend.URL},
				Routes:        []config.RouteConfig{{Path: "/", Methods: []string{}}},
				LoadBalancing: "round-robin",
			},
		},
		ConfigSource: config.ConfigSource{Type: "file"},
	}

	// build gin router
	router := gin.New()
	router.GET("/healthz", func(c *gin.Context) { c.JSON(200, gin.H{"status": "ok"}) })

	// build router manager
	routerManager, err := core.NewRouterManager(conf.Upstreams, conf.ConfigSource)
	if err != nil {
		backend.Close()
		return "", nil, nil, err
	}
	router.Use(routerManager.PreMatchMiddleware())

	// Capture stdout so that logging.NewService which writes to os.Stdout will be captured.
	oldStdout := os.Stdout
	r, w, pipeErr := os.Pipe()
	if pipeErr != nil {
		backend.Close()
		return "", nil, nil, pipeErr
	}
	os.Stdout = w

	// Setup middlewares (this will create the logging service which writes to current os.Stdout)
	if err := middleware.SetupMiddlewares(router, conf.Middlewares); err != nil {
		// restore and cleanup
		w.Close()
		os.Stdout = oldStdout
		backend.Close()
		return "", nil, nil, err
	}

	// restore os.Stdout to original for the rest of the test process; the logging service
	// keeps a handle to the writer we set (w) so logs will continue to flow into our pipe.
	os.Stdout = oldStdout

	router.NoRoute(routerManager.HandleRequest)

	// start gateway server
	srv := httptest.NewServer(router)

	// Start goroutine to copy from pipe reader into a buffer.
	buf := &bytes.Buffer{}
	done := make(chan struct{})
	go func() {
		_, _ = io.Copy(buf, r)
		close(done)
	}()

	stopFunc := func() {
		// Close server and backend
		srv.Close()
		backend.Close()
		// Close writer to unblock reader
		w.Close()
		// Wait a short time for copy to finish
		select {
		case <-done:
		case <-time.After(200 * time.Millisecond):
		}
	}

	return srv.URL, stopFunc, buf, nil
}
