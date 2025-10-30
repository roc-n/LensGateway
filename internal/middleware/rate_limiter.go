package middleware

import (
	"net/http"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"LensGateway.com/internal/util"
	"github.com/gin-gonic/gin"
)

// A simple, fast, lock-light token bucket.
type bucket struct {
	rate   float64      // tokens per second
	burst  int64        // max tokens
	tokens int64        // current tokens (atomic)
	last   atomic.Int64 // unix nano of last refill
}

func newBucket(rate float64, burst int64) *bucket {
	b := &bucket{rate: rate, burst: burst, tokens: burst}
	b.last.Store(time.Now().UnixNano())
	return b
}

func (b *bucket) allow(n int64) bool {
	now := time.Now().UnixNano()
	last := b.last.Load()
	dt := float64(now-last) / 1e9
	if dt > 0 {
		add := int64(dt * b.rate)
		if add > 0 {
			// try to advance last time
			if b.last.CompareAndSwap(last, now) {
				// refill
				for {
					cur := atomic.LoadInt64(&b.tokens)
					nt := min(cur+add, b.burst)
					if atomic.CompareAndSwapInt64(&b.tokens, cur, nt) {
						break
					}
				}
			}
		}
	}
	for {
		cur := atomic.LoadInt64(&b.tokens)
		if cur < n {
			return false
		}
		if atomic.CompareAndSwapInt64(&b.tokens, cur, cur-n) {
			return true
		}
	}
}

// rate limiter registry state
type rlState struct {
	mu sync.Mutex
	m  sync.Map // key -> *bucket
	// default params
	rate  float64
	burst int64
}

func (s *rlState) get(key string) *bucket {
	if v, ok := s.m.Load(key); ok {
		return v.(*bucket)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if v, ok := s.m.Load(key); ok {
		return v.(*bucket)
	}
	b := newBucket(s.rate, s.burst)
	s.m.Store(key, b)
	return b
}

func init() {
	Register("rate_limiter", func(cfg map[string]any) (gin.HandlerFunc, error) {
		strategy := strings.ToLower(util.StrOr(cfg["strategy"], "ip")) // ip|route|combined
		rate := parseFloat(cfg["requests_per_second"], 100.0)
		burst := parseInt(cfg["burst"], 50)

		// allow nested like config.global/ per_ip; prefer flat minimal config
		if g, ok := cfg["global"].(map[string]any); ok {
			rate = parseFloat(g["requests_per_second"], rate)
			burst = parseInt(g["burst"], int(burst))
		}

		state := &rlState{rate: rate, burst: int64(burst)}

		keyFn := func(c *gin.Context) string {
			ip := util.ClientIP(c.Request)
			route, _ := c.Get("route.prefix")
			switch strategy {
			case "route":
				return "route:" + toString(route)
			case "combined":
				return "comb:" + toString(route) + ":" + ip
			default:
				return "ip:" + ip
			}
		}

		return func(c *gin.Context) {
			k := keyFn(c)
			b := state.get(k)
			if !b.allow(1) {
				c.AbortWithStatusJSON(http.StatusTooManyRequests, gin.H{"error": "rate limit exceeded"})
				return
			}
			c.Next()
		}, nil
	})
}

func parseFloat(v interface{}, def float64) float64 {
	switch t := v.(type) {
	case float64:
		return t
	case string:
		if f, err := strconv.ParseFloat(t, 64); err == nil {
			return f
		}
	}
	return def
}

func parseInt(v interface{}, def int) int {
	switch t := v.(type) {
	case int:
		return t
	case int64:
		return int(t)
	case float64:
		return int(t)
	case string:
		if n, err := strconv.Atoi(t); err == nil {
			return n
		}
	}
	return def
}

func toString(v any) string {
	if v == nil {
		return ""
	}
	if s, ok := v.(string); ok {
		return s
	}
	return ""
}
