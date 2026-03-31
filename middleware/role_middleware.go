package middleware

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
)

func normalizeRole(role string) string {
	value := strings.ToLower(strings.TrimSpace(role))
	if value == "receptioniest" || value == "recertioniest" || value == "receiptionist" {
		return "receptionist"
	}
	return value
}

func RoleMiddleware(allowedRoles ...string) gin.HandlerFunc {
	return func(c *gin.Context) {

		role, exists := c.Get("role")
		if !exists {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "Role not found"})
			c.Abort()
			return
		}

		userRole := normalizeRole(role.(string))

		// Check allowed roles
		for _, r := range allowedRoles {
			r = normalizeRole(r)
			if r == userRole {
				c.Next()
				return
			}
		}

		c.JSON(http.StatusForbidden, gin.H{
			"error": "Access denied",
		})
		c.Abort()
	}
}
