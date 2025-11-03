package core

import (
	"crypto/tls"
	"errors"
	"log"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"sort"
	"strings"
	"sync/atomic"
	"time"

	"LensGateway.com/internal/balancer"
	"LensGateway.com/internal/config"
	"LensGateway.com/internal/middleware"
	"github.com/gin-gonic/gin"
)

// routeEntry 路由表项（简单前缀匹配 + 可选重写）
type routeEntry struct {
	balancerIdx int
	prefix      string // 例如 /api/users/
	methods     map[string]struct{}
	rewrite     string // 将 prefix 重写为 rewrite
	middlewares []gin.HandlerFunc
}

// RouterManager 核心路由管理器
type RouterManager struct {
	configSource config.ConfigSource
	table        atomic.Value // stores routingTable
}

type routingTable struct {
	balancers []balancer.Balancer
	routes    []routeEntry
}

// NewRouterManager 根据配置构建路由表与上游节点
func NewRouterManager(upstreams []config.UpstreamConfig, cfgSrc config.ConfigSource) (*RouterManager, error) {
	rm := &RouterManager{configSource: cfgSrc}
	tbl := buildRoutingTable(upstreams)
	rm.table.Store(tbl)
	return rm, nil
}

// PreMatch 根据当前已加载的路由表做一次只读匹配，返回命中的前缀（已按最长优先排好序）
// 该方法不做任何转发，仅用于在中间件链前段标注 route.prefix 以供路由级限流等功能使用。
func (rm *RouterManager) PreMatch(method, path string) (string, bool) {
	tbl, _ := rm.table.Load().(routingTable)
	for _, rt := range tbl.routes {
		if !matchPrefix(path, rt.prefix) {
			continue
		}
		if len(rt.methods) > 0 {
			if _, ok := rt.methods[method]; !ok {
				continue
			}
		}
		return rt.prefix, true
	}
	return "", false
}

// PreMatchMiddleware 在请求进入业务中间件前尝试匹配路由，并把命中的前缀放入上下文。
// 这样像 rate_limiter 这样的前置中间件就可以基于 route.prefix 做路由级限流。
func (rm *RouterManager) PreMatchMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		if prefix, ok := rm.PreMatch(c.Request.Method, c.Request.URL.Path); ok {
			c.Set("route.prefix", prefix)
		}
		c.Next()
	}
}

// HandleRequest 作为 gin.NoRoute 的兜底处理器
func (rm *RouterManager) HandleRequest(c *gin.Context) {
	tbl, _ := rm.table.Load().(routingTable)
	method := c.Request.Method
	path := c.Request.URL.Path

	// 1) 路由匹配（线性查找，数量少时足够；后续可优化为前缀树）
	for _, rt := range tbl.routes {
		if !matchPrefix(path, rt.prefix) {
			continue
		}
		if len(rt.methods) > 0 {
			if _, ok := rt.methods[method]; !ok {
				continue
			}
		}

		// 命中路由，执行该路由专属的中间件链
		c.Set("route.prefix", rt.prefix) // 确保路由级中间件能拿到前缀
		for _, mw := range rt.middlewares {
			mw(c)
			if c.IsAborted() {
				return
			}
		}

		// 命中该路由，选择一个上游节点
		if rt.balancerIdx < 0 || rt.balancerIdx >= len(tbl.balancers) {
			// 检查索引合法
			break
		}
		balancerx := tbl.balancers[rt.balancerIdx]
		ip := c.ClientIP()
		node, err := balancerx.Balance(ip)
		if err != nil {
			log.Printf("failed to balance upstream: %v", err)
			c.AbortWithStatusJSON(http.StatusBadGateway, gin.H{"error": "no healthy upstream node available"})
			return
		}

		// 2) URL 重写（仅基于前缀替换）
		newPath := path
		if rt.rewrite != "" {
			newPath = rewritePathByPrefix(path, rt.prefix, rt.rewrite)
		}

		// 3) 反向代理到目标节点
		proxy := newSingleHostReverseProxy(node.Url)
		// 设置一些上下文信息供日志等中间件采集
		c.Set("upstream.name", balancerx.Name())
		c.Set("upstream.host", node.Url.String())
		// 设置 path（Director 中也会校正）
		c.Request.URL.Path = newPath
		proxy.ServeHTTP(c.Writer, c.Request)
		return
	}

	// 未匹配到任何路由
	c.JSON(http.StatusNotFound, gin.H{"error": "no route matched"})
}

// UpdateUpstreams 用新的上游配置重建表并原子替换
func (rm *RouterManager) UpdateUpstreams(upstreams []config.UpstreamConfig) {
	tbl := buildRoutingTable(upstreams)
	rm.table.Store(tbl)
}

// 将上游配置构建为运行态表
// func buildRoutingTable(upstreams []config.UpstreamConfig) routingTable {
// 	var tbl routingTable
// 	// 1) 解析上游
// 	for _, up := range upstreams {
// 		scheme := up.Scheme
// 		if scheme == "" {
// 			scheme = "http"
// 		}
// 		st := upstreamState{name: up.Name, algo: strings.ToLower(up.LoadBalancing)}
// 		if st.algo == "" {
// 			st.algo = "round_robin"
// 		}
// 		for _, host := range up.Hosts {
// 			var u *url.URL
// 			if strings.HasPrefix(host, "http://") || strings.HasPrefix(host, "https://") {
// 				parsed, err := url.Parse(host)
// 				if err != nil {
// 					log.Printf("skip invalid upstream host %q: %v", host, err)
// 					continue
// 				}
// 				u = parsed
// 			} else {
// 				u = &url.URL{Scheme: scheme, Host: host}
// 			}
// 			st.nodes = append(st.nodes, upstreamNode{url: u})
// 		}
// 		if len(st.nodes) == 0 {
// 			log.Printf("upstream %q has no valid nodes; skipping", up.Name)
// 			continue
// 		}

// 		tbl.upstreams = append(tbl.upstreams, st)

// 		// 2) 解析路由
// 		for _, r := range up.Routes {
// 			prefix := normalizePrefix(r.Path)
// 			methods := make(map[string]struct{})
// 			for _, m := range r.Methods {
// 				methods[strings.ToUpper(m)] = struct{}{}
// 			}
// 			tbl.routes = append(tbl.routes, routeEntry{
// 				upstreamIdx: len(tbl.upstreams) - 1,
// 				prefix:      prefix,
// 				methods:     methods,
// 				rewrite:     r.Rewrite,
// 			})
// 		}
// 	}
// 	// 优先匹配更长的前缀
// 	sort.Slice(tbl.routes, func(i, j int) bool { return len(tbl.routes[i].prefix) > len(tbl.routes[j].prefix) })
// 	return tbl
// }

func buildRoutingTable(upstreams []config.UpstreamConfig) routingTable {
	var tbl routingTable

	for _, up := range upstreams {
		scheme := up.Scheme
		if scheme == "" {
			scheme = "http"
		}

		// 1) 解析上游服务节点
		nodes := []balancer.UpstreamNode{}
		for _, host := range up.Hosts {
			var u *url.URL
			if strings.HasPrefix(host, "http://") || strings.HasPrefix(host, "https://") {
				parsed, err := url.Parse(host)
				if err != nil {
					log.Printf("skip invalid upstream host %q: %v", host, err)
					continue
				}
				u = parsed
			} else {
				u = &url.URL{Scheme: scheme, Host: host}
			}
			nodes = append(nodes, balancer.UpstreamNode{Url: u})
		}
		if len(nodes) == 0 {
			log.Printf("upstream %q has no valid nodes; skipping", up.Name)
			continue
		}

		algo := strings.ToLower(up.LoadBalancing)
		if algo == "" {
			algo = "round_robin"
		}

		balancerx, err := balancer.Build(up.Name, algo, nodes)
		if err != nil {
			log.Printf("failed to build balancer for upstream %q: %v", up.Name, err)
			continue
		}
		tbl.balancers = append(tbl.balancers, balancerx)

		// 2) 解析路由
		for _, r := range up.Routes {
			prefix := normalizePrefix(r.Path)
			methods := make(map[string]struct{})
			for _, m := range r.Methods {
				methods[strings.ToUpper(m)] = struct{}{}
			}

			// 创建路由级中间件
			var routeMiddlewares []gin.HandlerFunc
			for _, mwConf := range r.Middlewares {
				mwName, _ := mwConf["name"].(string)
				if mwName == "" {
					log.Printf("skip middleware with empty name on route %s", prefix)
					continue
				}
				creator, err := middleware.GetCreator(mwName)
				if err != nil {
					log.Printf("skip unregistered middleware %q on route %s", mwName, prefix)
					continue
				}
				mwCfg, _ := mwConf["config"].(map[string]any)
				if mwCfg == nil {
					mwCfg = make(map[string]any)
				}
				handler, err := creator(mwCfg)
				if err != nil {
					log.Printf("failed to create middleware %q on route %s: %v", mwName, prefix, err)
					continue
				}
				routeMiddlewares = append(routeMiddlewares, handler)
			}

			tbl.routes = append(tbl.routes, routeEntry{
				balancerIdx: len(tbl.balancers) - 1,
				prefix:      prefix,
				methods:     methods,
				rewrite:     r.Rewrite,
				middlewares: routeMiddlewares,
			})
		}
	}

	// 启动健康检测
	balancer.HealthCheckAll(tbl.balancers, 30)

	// 优先匹配更长的前缀
	sort.Slice(tbl.routes, func(i, j int) bool { return len(tbl.routes[i].prefix) > len(tbl.routes[j].prefix) })
	return tbl
}

// 将 pattern 转换为标准前缀（去掉 /** 并确保以 / 结尾，便于前缀替换）
func normalizePrefix(p string) string {
	p = strings.TrimSuffix(p, "/**")
	if !strings.HasSuffix(p, "/") && p != "/" {
		p += "/"
	}
	return p
}

// matchPrefix 支持精确匹配基础路径（无尾斜杠）和前缀匹配两种情况
func matchPrefix(path, prefix string) bool {
	if prefix == "/" {
		return true
	}
	if path == strings.TrimSuffix(prefix, "/") { // 精确命中基础路径
		return true
	}
	return strings.HasPrefix(path, prefix)
}

// rewritePathByPrefix 安全地用 rewrite 替换 prefix
func rewritePathByPrefix(path, prefix, rewrite string) string {
	// 处理无尾斜杠的精确命中
	base := strings.TrimSuffix(prefix, "/")
	if path == base {
		// 将精确命中重写为 rewrite 去掉尾部斜杠后的基础路径
		return strings.TrimSuffix(rewrite, "/")
	}
	// 常规前缀替换
	return strings.Replace(path, prefix, rewrite, 1)
}

// 自定义 ReverseProxy，主要设置 Director 以覆盖 scheme/host/path 并补充代理头
func newSingleHostReverseProxy(target *url.URL) *httputil.ReverseProxy {
	director := func(req *http.Request) {
		// X-Forwarded-*
		if req.Header.Get("X-Forwarded-Host") == "" {
			req.Header.Set("X-Forwarded-Host", req.Host)
		}
		if req.Header.Get("X-Forwarded-Proto") == "" {
			if req.TLS != nil {
				req.Header.Set("X-Forwarded-Proto", "https")
			} else {
				req.Header.Set("X-Forwarded-Proto", "http")
			}
		}
		// 追加 X-Forwarded-For
		ip, _, _ := net.SplitHostPort(req.RemoteAddr)
		if ip != "" {
			prior := req.Header.Get("X-Forwarded-For")
			if prior == "" {
				req.Header.Set("X-Forwarded-For", ip)
			} else {
				req.Header.Set("X-Forwarded-For", prior+", "+ip)
			}
		}

		req.URL.Scheme = target.Scheme
		req.URL.Host = target.Host
		if !strings.HasPrefix(req.URL.Path, "/") {
			req.URL.Path = "/" + req.URL.Path
		}
		// 大多数后端希望 Host 为目标主机
		req.Host = target.Host
	}

	// 自定义传输层，设置合理超时，支持 HTTP/2
	transport := &http.Transport{
		Proxy: http.ProxyFromEnvironment,
		DialContext: (&net.Dialer{
			Timeout:   10 * time.Second,
			KeepAlive: 30 * time.Second,
		}).DialContext,
		ForceAttemptHTTP2:     true,
		MaxIdleConns:          100,
		IdleConnTimeout:       90 * time.Second,
		TLSHandshakeTimeout:   10 * time.Second,
		ExpectContinueTimeout: 1 * time.Second,
		TLSClientConfig:       &tls.Config{InsecureSkipVerify: false},
	}

	rp := &httputil.ReverseProxy{
		Director:  director,
		Transport: transport,
		ModifyResponse: func(resp *http.Response) error {
			// 预留：可在此统一注入响应头等
			return nil
		},
		ErrorHandler: func(w http.ResponseWriter, r *http.Request, err error) {
			// 将上游错误映射为 502
			var netErr net.Error
			if errors.As(err, &netErr) && netErr.Timeout() {
				http.Error(w, "upstream timeout", http.StatusGatewayTimeout)
				return
			}
			http.Error(w, "bad gateway", http.StatusBadGateway)
		},
	}
	return rp
}
