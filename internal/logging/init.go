package logging

import (
	"time"

	"LensGateway.com/internal/middleware"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

func init() {
	middleware.Register("logging", func(cfg map[string]any) (gin.HandlerFunc, error) {

		// Init logging service.
		bufferSize := cfg["buffer_size"].(int)
		loggingService := NewService(bufferSize)
		loggingService.Start()

		// Create a Gin middleware that captures request information and hands it to the logging service.
		return func(c *gin.Context) {
			start := time.Now()

			// Adopts UUID for globally unique request identification.
			reqID := uuid.New().String()
			c.Set("request_id", reqID)
			// Set it also in the response header for client troubleshooting.
			c.Header("X-Request-ID", reqID)
			// Go through the rest of the middleware chain and final handlers.
			c.Next()

			// All processing is done, now gather info for logging.
			latency := time.Since(start)
			entry := &Entry{
				Timestamp: start,
				Level:     "info", // 默认为info级别
				ClientIP:  c.ClientIP(),
				Method:    c.Request.Method,
				Path:      c.Request.URL.Path,
				Status:    c.Writer.Status(),
				LatencyMs: latency.Milliseconds(),
				UserAgent: c.Request.UserAgent(),
				RequestID: reqID,
			}
			// Pick strategy info from other middlewares within Gin context.
			if prefix, exists := c.Get("route.prefix"); exists {
				entry.Gateway.RoutePrefix, _ = prefix.(string)
			}
			if upstream, exists := c.Get("upstream.name"); exists {
				entry.Gateway.UpstreamName, _ = upstream.(string)
			}
			if node, exists := c.Get("upstream.host"); exists {
				entry.Gateway.UpstreamNode, _ = node.(string)
			}
			if sub, exists := c.Get("auth.sub"); exists {
				entry.Auth.UserID, _ = sub.(string)
				entry.Auth.Status = "success"
			}

			// Append any errors recorded in Gin context during processing,
			// elevate log level to "error" and record the error details.
			if len(c.Errors) > 0 {
				entry.Level = "error"
				entry.Error = c.Errors.String()
			}

			// hand the filled log entry to the logging service asynchronously.
			loggingService.Log(entry)
		}, nil
	})
}
