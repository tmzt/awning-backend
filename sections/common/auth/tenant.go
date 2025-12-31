package auth

import (
	"log/slog"
	"net/http"

	ginmw "github.com/bartventer/gorm-multitenancy/middleware/gin/v8"
	"github.com/gin-gonic/gin"
)

// TenantMiddlewareConfig holds configuration for tenant resolution
type TenantMiddlewareConfig struct {
	// HeaderName is the HTTP header to extract tenant from (e.g., "X-Tenant-ID")
	HeaderName string
	// SkipPaths are paths that don't require tenant context
	SkipPaths []string
}

// DefaultTenantMiddlewareConfig returns the default configuration
func DefaultTenantMiddlewareConfig() *TenantMiddlewareConfig {
	return &TenantMiddlewareConfig{
		HeaderName: "X-Tenant-ID",
		SkipPaths: []string{
			"/api/v1/auth/",
			"/api/v1/users/",
			"/health",
		},
	}
}

// TenantFromHeaderMiddleware extracts tenant ID from a header
func TenantFromHeaderMiddleware(cfg *TenantMiddlewareConfig) gin.HandlerFunc {
	return func(c *gin.Context) {
		// Skip tenant resolution for certain paths
		for _, skipPath := range cfg.SkipPaths {
			if len(c.Request.URL.Path) >= len(skipPath) && c.Request.URL.Path[:len(skipPath)] == skipPath {
				c.Next()
				return
			}
		}

		tenantID := c.GetHeader(cfg.HeaderName)
		if tenantID == "" {
			// Also check query param as fallback
			tenantID = c.Query("tenant")
		}

		if tenantID == "" {
			// Try to get from JWT claims
			if schema, ok := GetTenantSchemaFromContext(c); ok && schema != "" {
				tenantID = schema
			}
		}

		if tenantID == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "tenant ID is required"})
			c.Abort()
			return
		}

		// Validate tenant ID format
		if err := validateTenantID(tenantID); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			c.Abort()
			return
		}

		slog.Debug("Tenant context set", "tenant", tenantID)
		c.Set("tenantID", tenantID)
		c.Next()
	}
}

// GetTenantIDFromContext retrieves the tenant ID from the Gin context
func GetTenantIDFromContext(c *gin.Context) (string, bool) {
	tenantID, exists := c.Get("tenantID")
	if !exists {
		return "", false
	}
	return tenantID.(string), true
}

// validateTenantID validates the tenant ID format
func validateTenantID(tenantID string) error {
	if len(tenantID) < 3 {
		return ErrInvalidTenantID
	}
	if len(tenantID) > 63 {
		return ErrInvalidTenantID
	}
	// Basic alphanumeric + underscore check
	for _, r := range tenantID {
		if !isValidTenantChar(r) {
			return ErrInvalidTenantID
		}
	}
	return nil
}

func isValidTenantChar(r rune) bool {
	return (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '_'
}

var ErrInvalidTenantID = &TenantError{Message: "invalid tenant ID format"}

type TenantError struct {
	Message string
}

func (e *TenantError) Error() string {
	return e.Message
}

// GormMultitenancyMiddleware returns the gorm-multitenancy middleware for Gin
// This uses the default subdomain-based tenant extraction from gorm-multitenancy
func GormMultitenancyMiddleware() gin.HandlerFunc {
	return ginmw.WithTenant(ginmw.DefaultWithTenantConfig)
}
