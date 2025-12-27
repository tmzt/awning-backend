package middleware

// Expects X-Awning-Key header containing the frontend API key
import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"
)

var ErrMissingFrontendAPIKey = errors.New("missing or invalid frontend API key")

func APIFrontendKeyAuthMiddleware(expectedKey string) gin.HandlerFunc {
	return func(c *gin.Context) {
		providedKey := c.GetHeader("X-Awning-Frontend-Key")
		if providedKey == "" {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "Frontend API key is required"})
			return
		}

		if providedKey != expectedKey {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "Invalid frontend API key"})
			return
		}

		c.Next()
	}
}
