package middleware

import (
	"errors"
	"fmt"
	"net/http"
	"strings"

	"LensGateway.com/util"
	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
)

func init() {
	Register("auth_jwt", func(cfg map[string]any) (gin.HandlerFunc, error) {
		secret := util.StrOr(cfg["secret_key"], "")
		if secret == "" {
			// For route-level middleware, a global secret might not be required.
			// We can allow it to be passed per-route in the future.
		}
		lookup := util.StrOr(cfg["token_lookup"], "header:Authorization")

		return func(c *gin.Context) {
			// If secret is not configured for this middleware instance, skip.
			routeSecret := util.StrOr(cfg["secret_key"], secret)
			if routeSecret == "" {
				c.Next()
				return
			}

			tokenStr := extractToken(c, lookup)
			if tokenStr == "" {
				c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "missing token"})
				return
			}

			// Parse and validate the token using the library
			token, err := jwt.Parse(tokenStr, func(token *jwt.Token) (interface{}, error) {
				// Validate the alg is what we expect (HS256)
				if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
					return nil, fmt.Errorf("unexpected signing method: %v", token.Header["alg"])
				}
				return []byte(routeSecret), nil
			})

			if err != nil {
				// The library automatically handles expiration checks.
				if errors.Is(err, jwt.ErrTokenExpired) {
					c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "token expired"})
				} else {
					c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "invalid token"})
				}
				return
			}

			if claims, ok := token.Claims.(jwt.MapClaims); ok && token.Valid {
				// Optional: set claims to context for downstream use
				c.Set("jwt_claims", claims)
				if sub, err := claims.GetSubject(); err == nil {
					c.Set("auth.sub", sub)
				}
			} else {
				c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "invalid token"})
				return
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
		// Standard: "Bearer <token>"
		if strings.HasPrefix(strings.ToLower(v), "bearer ") {
			return strings.TrimSpace(v[7:])
		}
		// Also support just the token
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
