package middleware

import (
	"context"
	"errors"
	"fmt"
	"net/http"

	"github.com/gin-gonic/gin"
)

var ErrMissingAPICredentials = errors.New("missing or invalid API credentials")

func APIKeyAuthMiddleware(validateFunc func(ctx context.Context, apiKey, apiSecret string) (context.Context, error)) gin.HandlerFunc {
	return func(c *gin.Context) {
		// apiKey := c.GetHeader("X-API-KEY")
		// apiSecret := c.GetHeader("X-API-SECRET")

		authHeader := c.GetHeader("Authorization")

		var apiKey, apiSecret string
		if len(authHeader) > 7 && authHeader[:7] == "ApiKey " {
			// Expected format: "ApiKey key:secret"
			credentials := authHeader[7:]
			parts := make([]string, 2)
			n, _ := fmt.Sscanf(credentials, "%[^:]:%s", &parts[0], &parts[1])
			if n == 2 {
				apiKey = parts[0]
				apiSecret = parts[1]
			}
		}

		if apiKey == "" || apiSecret == "" {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "API key and secret are required"})
			return
		}

		ctx, err := validateFunc(c.Request.Context(), apiKey, apiSecret)
		if err != nil {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "Invalid API key or secret"})
			return
		}

		c.Request = c.Request.WithContext(ctx)
		c.Next()
	}
}
