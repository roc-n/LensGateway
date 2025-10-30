package util

import (
	"net"
	"net/http"
	"strconv"
	"strings"
)

// ClientIP returns the client IP, honoring X-Forwarded-For when present.
// Falls back to the host part of RemoteAddr.
func ClientIP(r *http.Request) string {
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		parts := strings.Split(xff, ",")
		if len(parts) > 0 {
			return strings.TrimSpace(parts[0])
		}
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err == nil {
		return host
	}
	return r.RemoteAddr
}

func ParseCIDRs(cidrs []string) []*net.IPNet {
	var out []*net.IPNet
	for _, c := range cidrs {
		c = strings.TrimSpace(c)
		if c == "" {
			continue
		}
		if !strings.Contains(c, "/") { // allow bare IP -> /32 or /128
			if ip := net.ParseIP(c); ip != nil {
				mask := 32
				if ip.To4() == nil {
					mask = 128
				}
				c = c + "/" + strconv.Itoa(mask)
			}
		}
		if _, n, err := net.ParseCIDR(c); err == nil {
			out = append(out, n)
		}
	}
	return out
}
