package middleware

import (
	"net/http"
	"strings"

	"cs485/internal/services"

	"github.com/gin-gonic/gin"
)

const UserIDKey = "userID"
const UserEmailKey = "userEmail"

// Auth returns a Gin middleware that validates Bearer JWT tokens.
// On success it injects the userID and email into the request context.
// On failure it aborts with 401.
func Auth(authSvc *services.AuthService) gin.HandlerFunc {
	return func(c *gin.Context) {
		header := c.GetHeader("Authorization")
		if !strings.HasPrefix(header, "Bearer ") {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "missing bearer token"})
			return
		}
		token := strings.TrimPrefix(header, "Bearer ")
		claims, err := authSvc.ValidateToken(token)
		if err != nil {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "invalid token"})
			return
		}
		c.Set(UserIDKey, claims.UserID)
		c.Set(UserEmailKey, claims.Email)
		c.Next()
	}
}

// OptionalAuth sets userID/email if a valid Bearer token is present,
// but does not abort if it is missing.
func OptionalAuth(authSvc *services.AuthService) gin.HandlerFunc {
	return func(c *gin.Context) {
		header := c.GetHeader("Authorization")
		if strings.HasPrefix(header, "Bearer ") {
			token := strings.TrimPrefix(header, "Bearer ")
			if claims, err := authSvc.ValidateToken(token); err == nil {
				c.Set(UserIDKey, claims.UserID)
				c.Set(UserEmailKey, claims.Email)
			}
		}
		c.Next()
	}
}

// MustGetUserID retrieves the authenticated user ID from the Gin context,
// panicking if it is absent (programmer error — always guard with Auth middleware).
func MustGetUserID(c *gin.Context) string {
	v, _ := c.Get(UserIDKey)
	id, _ := v.(string)
	return id
}
