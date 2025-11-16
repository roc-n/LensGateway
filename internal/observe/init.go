package observe

import (
	"log"
	"strconv"
	"time"

	"LensGateway.com/internal/middleware"
	"github.com/gin-gonic/gin"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

func init() {
	middleware.Register("observe", func(cfg map[string]any) (gin.HandlerFunc, error) {
		return func(c *gin.Context) {
			start := time.Now()

			c.Next()

			duration := time.Since(start).Seconds()
			path := c.FullPath() // 使用 c.FullPath() 可以聚合动态路由，如 /users/:id
			if path == "" {
				path = c.Request.URL.Path // 如果路由未匹配，则使用原始路径
			}
			method := c.Request.Method
			status := strconv.Itoa(c.Writer.Status())

			HttpRequestsTotal.WithLabelValues(method, path, status).Inc()
			HttpRequestDurationSeconds.WithLabelValues(method, path).Observe(duration)

			log.Println("xxxxx")
			log.Println(HttpRequestsTotal.GetMetricWithLabelValues(method, path, status))

		}, nil
	})
}

// MetricsHandler 返回一个可以暴露 Prometheus 指标的 http.Handler
func MetricsHandler() gin.HandlerFunc {
	h := promhttp.Handler()
	return func(c *gin.Context) {
		h.ServeHTTP(c.Writer, c.Request)
	}
}
