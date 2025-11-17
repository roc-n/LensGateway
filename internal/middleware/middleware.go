package middleware

import (
	"fmt"
	"log"
	"regexp"
	"sort"
	"strings"
	"time"

	"LensGateway.com/internal/config"
	"LensGateway.com/util"
	"github.com/gin-gonic/gin"
)

// MiddlewareCreator 是一个函数类型，它接收一个配置map，返回一个gin.HandlerFunc
// 作为中间件可配置的核心
type MiddlewareCreator func(config map[string]any) (gin.HandlerFunc, error)

// 中间件注册表
var registry = make(map[string]MiddlewareCreator)

// 注册一个中间件创建器
func Register(name string, creator MiddlewareCreator) {
	if _, exists := registry[name]; exists {
		panic(fmt.Sprintf("Middleware %s is already registered", name))
	}
	registry[name] = creator
}

// 获取一个已注册的创建器
func GetCreator(name string) (MiddlewareCreator, error) {
	creator, exists := registry[name]
	if !exists {
		return nil, fmt.Errorf("middleware %s not registered", name)
	}
	return creator, nil
}

// 根据配置，动态创建和排序中间件链
func SetupMiddlewares(router *gin.Engine, middlewareConfigs map[string]config.MiddlewareConfig) error {
	type middlewareItem struct {
		order   int
		handler gin.HandlerFunc
	}

	var middlewares []middlewareItem

	// 1. 遍历所有配置的中间件
	for name, mwConf := range middlewareConfigs {
		if !mwConf.Enabled {
			continue // 跳过未启用的中间件
		}

		// 2. 从注册表中找到对应的创建函数（未注册的中间件跳过并告警）
		creator, exists := registry[name]
		if !exists {
			log.Printf("[gateway] middleware %q not registered, skipping", name)
			continue
		}

		// 3. 调用创建函数，传入该中间件的具体配置，实例化一个中间件Handler
		handler, err := creator(mwConf.Config)
		if err != nil {
			return fmt.Errorf("failed to create middleware %s: %v", name, err)
		}

		// 4. 收集到列表中，稍后排序
		middlewares = append(middlewares, middlewareItem{
			order:   mwConf.Order,
			handler: handler,
		})
	}

	// 5. 按order排序
	sort.Slice(middlewares, func(i, j int) bool {
		return middlewares[i].order < middlewares[j].order
	})

	// 6. 将排序好的中间件添加到Gin的全局使用列表中
	for _, mw := range middlewares {
		router.Use(mw.handler)
	}

	return nil
}

// --- Built-in lightweight middlewares ---

func init() {
	// request_logger
	Register("request_logger", func(cfg map[string]interface{}) (gin.HandlerFunc, error) {
		level := util.StrOr(cfg["level"], "info")
		return func(c *gin.Context) {
			start := time.Now()
			c.Next()
			latency := time.Since(start)
			status := c.Writer.Status()
			upName, _ := c.Get("upstream.name")
			upTarget, _ := c.Get("upstream.target")
			log.Printf("[req] level=%s method=%s path=%s status=%d latency=%s upstream=%v target=%v ip=%s ua=%s",
				level, c.Request.Method, c.Request.URL.Path, status, latency, upName, upTarget, c.ClientIP(), c.Request.UserAgent())
		}, nil
	})

	// cors
	Register("cors", func(cfg map[string]any) (gin.HandlerFunc, error) {
		allowOrigin := util.StrOr(cfg["allow_origin"], "*")
		allowMethods := util.StrOr(cfg["allow_methods"], "GET,POST,PUT,DELETE,OPTIONS")
		allowHeaders := util.StrOr(cfg["allow_headers"], "") // if empty, echo request's Access-Control-Request-Headers
		exposeHeaders := util.StrOr(cfg["expose_headers"], "")
		allowCredentials := strings.EqualFold(util.StrOr(cfg["allow_credentials"], "false"), "true")
		maxAge := util.StrOr(cfg["max_age"], "600")

		// helpers
		parseList := func(s string) []string {
			if s == "" {
				return nil
			}
			parts := strings.Split(s, ",")
			out := make([]string, 0, len(parts))
			for _, p := range parts {
				p = strings.TrimSpace(p)
				if p != "" {
					out = append(out, p)
				}
			}
			return out
		}

		allowedOrigins := parseList(allowOrigin)

		// match allowed origin (supports exact, '*' and wildcard patterns like https://*.example.com)
		allowedOriginMatch := func(origin string) bool {
			if len(allowedOrigins) == 0 {
				return false
			}
			for _, a := range allowedOrigins {
				if a == "*" {
					return true
				}
				if a == origin {
					return true
				}
				if strings.Contains(a, "*") {
					esc := regexp.QuoteMeta(a)
					pattern := "^" + strings.ReplaceAll(esc, "\\*", ".*") + "$"
					re, err := regexp.Compile(pattern)
					if err != nil {
						continue
					}
					if re.MatchString(origin) {
						return true
					}
				}
			}
			return false
		}

		globalStar := false
		if allowOrigin == "*" || len(allowedOrigins) == 0 {
			globalStar = true
		}

		return func(c *gin.Context) {
			origin := c.Request.Header.Get("Origin")
			if origin == "" {
				// not a CORS request
				c.Next()
				return
			}

			// decide Access-Control-Allow-Origin
			setOrigin := ""
			if globalStar {
				if allowCredentials {
					// cannot use '*' when credentials are allowed; echo back request origin
					setOrigin = origin
				} else {
					setOrigin = "*"
				}
			} else {
				if allowedOriginMatch(origin) {
					setOrigin = origin
				} else {
					// origin not allowed -> continue without CORS headers
					c.Next()
					return
				}
			}

			// Common headers
			c.Writer.Header().Set("Access-Control-Allow-Origin", setOrigin)
			if allowCredentials {
				c.Writer.Header().Set("Access-Control-Allow-Credentials", "true")
			}
			// inform caches/proxies that response varies by Origin
			c.Writer.Header().Add("Vary", "Origin")

			if exposeHeaders != "" {
				c.Writer.Header().Set("Access-Control-Expose-Headers", exposeHeaders)
			}

			// Preflight
			if c.Request.Method == httpMethodOptions {
				// Methods
				c.Writer.Header().Set("Access-Control-Allow-Methods", allowMethods)

				// Headers: configured or echo request's desired headers
				acrh := c.Request.Header.Get("Access-Control-Request-Headers")
				if allowHeaders != "" {
					c.Writer.Header().Set("Access-Control-Allow-Headers", allowHeaders)
				} else if acrh != "" {
					c.Writer.Header().Set("Access-Control-Allow-Headers", acrh)
				}

				// Max-Age
				if maxAge != "" {
					c.Writer.Header().Set("Access-Control-Max-Age", maxAge)
				}

				c.AbortWithStatus(204)
				return
			}

			// Simple request -> continue
			c.Next()
		}, nil
	})
}

const httpMethodOptions = "OPTIONS"
