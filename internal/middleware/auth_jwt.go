package middleware

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"

	"LensGateway.com/internal/util"
	"github.com/gin-gonic/gin"
)

// Lightweight JWT (HS256) verification without external dependency.
// Only validates signature + exp if present. For more algs/claims, swap to a full JWT lib.

func init() {
	Register("auth_jwt", func(cfg map[string]any) (gin.HandlerFunc, error) {
		secret := util.StrOr(cfg["secret_key"], "")
		if secret == "" {
			return nil, errors.New("auth_jwt.secret_key required")
		}
		lookup := util.StrOr(cfg["token_lookup"], "header:Authorization")
		skip := util.ToStringSlice(cfg["skip_paths"]) // paths to bypass

		return func(c *gin.Context) {
			// Skip paths
			for _, p := range skip {
				if p != "" && strings.HasPrefix(c.Request.URL.Path, p) {
					c.Next()
					return
				}
			}

			token := extractToken(c, lookup)
			if token == "" {
				c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "missing token"})
				return
			}
			if err := verifyHS256(token, secret); err != nil {
				c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "invalid token"})
				return
			}
			// Minimal claims: if exp present, ensure not expired
			if claims, _ := parseClaims(token); claims != nil {
				if exp, ok := claims["exp"].(float64); ok {
					if time.Now().Unix() > int64(exp) {
						c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "token expired"})
						return
					}
				}
				// Optional: set sub to context
				if sub, ok := claims["sub"].(string); ok {
					c.Set("auth.sub", sub)
				}
			}
			c.Next()
		}, nil
	})
}

func extractToken(c *gin.Context, lookup string) string {
	// formats: "header:Authorization", "query:token", "cookie:jwt"
	parts := strings.SplitN(lookup, ":", 2)
	source := "header"
	key := "Authorization"
	if len(parts) == 2 {
		source = parts[0]
		key = parts[1]
	}
	switch strings.ToLower(source) {
	case "header":
		v := c.Request.Header.Get(key)
		if strings.HasPrefix(strings.ToLower(v), "bearer ") {
			return strings.TrimSpace(v[7:])
		}
		return strings.TrimSpace(v)
	case "query":
		return c.Query(key)
	case "cookie":
		if ck, err := c.Request.Cookie(key); err == nil {
			return ck.Value
		}
	}
	return ""
}

func verifyHS256(token, secret string) error {
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		return errors.New("invalid token format")
	}
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(parts[0] + "." + parts[1]))
	sig := mac.Sum(nil)
	want := base64.RawURLEncoding.EncodeToString(sig)
	if !hmac.Equal([]byte(want), []byte(parts[2])) {
		return errors.New("bad signature")
	}
	return nil
}

func parseClaims(token string) (map[string]interface{}, error) {
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		return nil, errors.New("invalid token format")
	}
	payload, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return nil, err
	}
	var m map[string]interface{}
	if err := json.Unmarshal(payload, &m); err != nil {
		return nil, err
	}
	return m, nil
}
