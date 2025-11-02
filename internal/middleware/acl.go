package middleware

import (
	"net"
	"net/http"

	"LensGateway.com/util"
	"github.com/gin-gonic/gin"
)

// Simple IP-based ACL. Config:
//
//	whitelist: ["127.0.0.1/32", "10.0.0.0/8"]
//	blacklist: ["192.168.1.100/32"]
//
// If whitelist not empty, only allow those; blacklist always deny.
func init() {
	Register("acl", func(cfg map[string]any) (gin.HandlerFunc, error) {
		whitelist := util.ParseCIDRs(util.ToStringSlice(cfg["whitelist"]))
		blacklist := util.ParseCIDRs(util.ToStringSlice(cfg["blacklist"]))

		return func(c *gin.Context) {
			ipStr := util.ClientIP(c.Request)
			ip := net.ParseIP(ipStr)
			if ip == nil {
				c.AbortWithStatusJSON(http.StatusForbidden, gin.H{"error": "forbidden"})
				return
			}
			// deny if in blacklist
			for _, n := range blacklist {
				if n.Contains(ip) {
					c.AbortWithStatusJSON(http.StatusForbidden, gin.H{"error": "blocked"})
					return
				}
			}
			// if whitelist present, must be contained
			if len(whitelist) > 0 {
				allowed := false
				for _, n := range whitelist {
					if n.Contains(ip) {
						allowed = true
						break
					}
				}
				if !allowed {
					c.AbortWithStatusJSON(http.StatusForbidden, gin.H{"error": "not allowed"})
					return
				}
			}
			c.Next()
		}, nil
	})
}
