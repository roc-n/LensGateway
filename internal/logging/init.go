package logging

import (
	"fmt"
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

			// Determine level based on errors, upstream status and latency (configurable).
			entry.Level = determineLevel(c, entry, cfg)

			// hand the filled log entry to the logging service asynchronously.
			loggingService.Log(entry)
		}, nil
	})
}

// determineLevel decides the log level for a request based on context and config.
// It may also set entry.Error when upstream or context errors are present.
func determineLevel(c *gin.Context, entry *Entry, cfg map[string]any) string {
	// 1) If Gin recorded errors in context, treat as error.
	if len(c.Errors) > 0 {
		entry.Error = c.Errors.String()
		return "error"
	}

	// 2) If proxy/transport set an upstream error into context, treat as error
	if ue, ok := c.Get("upstream.error"); ok {
		entry.Error = fmt.Sprint(ue)
		return "error"
	}

	// 3) If upstream status present, prefer it for severity
	if us, ok := c.Get("upstream.status"); ok {
		switch v := us.(type) {
		case int:
			if v >= 500 {
				return "error"
			}
			if v >= 400 {
				// treat 4xx as warn by default if configured, else info
				if cfg != nil {
					if cv, ok := cfg["client_error_as_warn"]; ok {
						if bv, ok := cv.(bool); ok && bv {
							return "warn"
						}
					}
				}
				return "info"
			}
		case int64:
			if v >= 500 {
				return "error"
			}
			if v >= 400 {
				if cfg != nil {
					if cv, ok := cfg["client_error_as_warn"]; ok {
						if bv, ok := cv.(bool); ok && bv {
							return "warn"
						}
					}
				}
				return "info"
			}
		case float64:
			iv := int(v)
			if iv >= 500 {
				return "error"
			}
			if iv >= 400 {
				if cfg != nil {
					if cv, ok := cfg["client_error_as_warn"]; ok {
						if bv, ok := cv.(bool); ok && bv {
							return "warn"
						}
					}
				}
				return "info"
			}
		case string:
			// try parse
			var iv int
			_, err := fmt.Sscanf(v, "%d", &iv)
			if err == nil {
				if iv >= 500 {
					return "error"
				}
				if iv >= 400 {
					if cfg != nil {
						if cv, ok := cfg["client_error_as_warn"]; ok {
							if bv, ok := cv.(bool); ok && bv {
								return "warn"
							}
						}
					}
					return "info"
				}
			}
		}
	}

	// 4) Latency-based warning
	var warnMs int64 = 1000
	if cfg != nil {
		// support multiple possible keys
		if v, ok := cfg["latency_warn_ms"]; ok {
			switch x := v.(type) {
			case int:
				warnMs = int64(x)
			case int64:
				warnMs = x
			case float64:
				warnMs = int64(x)
			}
		} else if v, ok := cfg["warn_latency_ms"]; ok {
			switch x := v.(type) {
			case int:
				warnMs = int64(x)
			case int64:
				warnMs = x
			case float64:
				warnMs = int64(x)
			}
		}
	}
	if entry.LatencyMs >= warnMs {
		return "warn"
	}

	// 5) Default: info
	return "info"
}
