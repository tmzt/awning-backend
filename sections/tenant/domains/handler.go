package domains

import (
	"log/slog"
	"net/http"
	"time"

	"awning-backend/sections"
	"awning-backend/sections/common/auth"
	"awning-backend/sections/models"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// Handler handles domain-related requests
type Handler struct {
	logger    *slog.Logger
	deps      *sections.Dependencies
	registrar DomainRegistrar
}

// NewHandler creates a new domains handler
func NewHandler(deps *sections.Dependencies, registrar DomainRegistrar) *Handler {
	return &Handler{
		logger:    slog.With("handler", "DomainsHandler"),
		deps:      deps,
		registrar: registrar,
	}
}

// DomainResponse represents a domain response
type DomainResponse struct {
	ID            uint       `json:"id"`
	Domain        string     `json:"domain"`
	DomainType    string     `json:"domainType"`
	Verified      bool       `json:"verified"`
	VerifiedAt    *time.Time `json:"verifiedAt,omitempty"`
	SSLEnabled    bool       `json:"sslEnabled"`
	SSLExpiresAt  *time.Time `json:"sslExpiresAt,omitempty"`
	DNSConfigured bool       `json:"dnsConfigured"`
	Primary       bool       `json:"primary"`
}

// ListDomains retrieves all domains for a tenant
func (h *Handler) ListDomains(c *gin.Context) {
	tenantID, ok := auth.GetTenantIDFromContext(c)
	if !ok {
		c.JSON(http.StatusBadRequest, gin.H{"error": "tenant context required"})
		return
	}

	var domains []models.TenantDomain
	err := h.deps.DB.WithTenant(c.Request.Context(), tenantID, func(tx *gorm.DB) error {
		return tx.Where("tenant_schema = ?", tenantID).Find(&domains).Error
	})

	if err != nil {
		h.logger.Error("Failed to list domains", "error", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to list domains"})
		return
	}

	responses := make([]DomainResponse, len(domains))
	for i, d := range domains {
		responses[i] = h.toResponse(&d)
	}

	c.JSON(http.StatusOK, gin.H{"domains": responses})
}

// GetDomain retrieves a specific domain
func (h *Handler) GetDomain(c *gin.Context) {
	tenantID, ok := auth.GetTenantIDFromContext(c)
	if !ok {
		c.JSON(http.StatusBadRequest, gin.H{"error": "tenant context required"})
		return
	}

	domainName := c.Param("domain")

	var domain models.TenantDomain
	err := h.deps.DB.WithTenant(c.Request.Context(), tenantID, func(tx *gorm.DB) error {
		return tx.Where("tenant_schema = ? AND domain = ?", tenantID, domainName).First(&domain).Error
	})

	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "domain not found"})
		return
	}

	c.JSON(http.StatusOK, h.toResponse(&domain))
}

// AddDomain adds a new domain
func (h *Handler) AddDomain(c *gin.Context) {
	tenantID, ok := auth.GetTenantIDFromContext(c)
	if !ok {
		c.JSON(http.StatusBadRequest, gin.H{"error": "tenant context required"})
		return
	}

	var req struct {
		Domain     string `json:"domain" binding:"required"`
		DomainType string `json:"domainType"` // subdomain, custom, registered
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if req.DomainType == "" {
		req.DomainType = "custom"
	}

	domain := models.TenantDomain{
		TenantSchema: tenantID,
		Domain:       req.Domain,
		DomainType:   req.DomainType,
		Verified:     false,
	}

	err := h.deps.DB.WithTenant(c.Request.Context(), tenantID, func(tx *gorm.DB) error {
		return tx.Create(&domain).Error
	})

	if err != nil {
		h.logger.Error("Failed to add domain", "error", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to add domain"})
		return
	}

	c.JSON(http.StatusCreated, h.toResponse(&domain))
}

// DeleteDomain removes a domain
func (h *Handler) DeleteDomain(c *gin.Context) {
	tenantID, ok := auth.GetTenantIDFromContext(c)
	if !ok {
		c.JSON(http.StatusBadRequest, gin.H{"error": "tenant context required"})
		return
	}

	domainName := c.Param("domain")

	err := h.deps.DB.WithTenant(c.Request.Context(), tenantID, func(tx *gorm.DB) error {
		return tx.Where("tenant_schema = ? AND domain = ?", tenantID, domainName).Delete(&models.TenantDomain{}).Error
	})

	if err != nil {
		h.logger.Error("Failed to delete domain", "error", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to delete domain"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "domain deleted"})
}

// SetPrimaryDomain sets a domain as the primary domain
func (h *Handler) SetPrimaryDomain(c *gin.Context) {
	tenantID, ok := auth.GetTenantIDFromContext(c)
	if !ok {
		c.JSON(http.StatusBadRequest, gin.H{"error": "tenant context required"})
		return
	}

	domainName := c.Param("domain")

	err := h.deps.DB.WithTenant(c.Request.Context(), tenantID, func(tx *gorm.DB) error {
		// Unset all primary domains
		if err := tx.Model(&models.TenantDomain{}).
			Where("tenant_schema = ?", tenantID).
			Update("primary", false).Error; err != nil {
			return err
		}

		// Set the specified domain as primary
		return tx.Model(&models.TenantDomain{}).
			Where("tenant_schema = ? AND domain = ?", tenantID, domainName).
			Update("primary", true).Error
	})

	if err != nil {
		h.logger.Error("Failed to set primary domain", "error", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to set primary domain"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "primary domain set"})
}

// CheckDomainAvailability checks if a domain is available for registration
func (h *Handler) CheckDomainAvailability(c *gin.Context) {
	if h.registrar == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "domain registrar not configured"})
		return
	}

	domain := c.Query("domain")
	if domain == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "domain query parameter is required"})
		return
	}

	result, err := h.registrar.CheckAvailability(c.Request.Context(), domain)
	if err != nil {
		if err == ErrNotImplemented {
			c.JSON(http.StatusNotImplemented, gin.H{"error": "domain availability check not implemented for this registrar"})
			return
		}
		h.logger.Error("Failed to check domain availability", "error", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to check domain availability"})
		return
	}

	c.JSON(http.StatusOK, result)
}

// RegisterDomain registers a new domain
func (h *Handler) RegisterDomain(c *gin.Context) {
	if h.registrar == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "domain registrar not configured"})
		return
	}

	tenantID, ok := auth.GetTenantIDFromContext(c)
	if !ok {
		c.JSON(http.StatusBadRequest, gin.H{"error": "tenant context required"})
		return
	}

	var req struct {
		Domain  string       `json:"domain" binding:"required"`
		Years   int          `json:"years" binding:"min=1,max=10"`
		Contact *ContactInfo `json:"contact" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if req.Years == 0 {
		req.Years = 1
	}

	result, err := h.registrar.Register(c.Request.Context(), req.Domain, req.Years, req.Contact)
	if err != nil {
		if err == ErrNotImplemented {
			c.JSON(http.StatusNotImplemented, gin.H{"error": "domain registration not implemented for this registrar"})
			return
		}
		h.logger.Error("Failed to register domain", "error", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to register domain"})
		return
	}

	// Save domain to database
	registrarName := h.registrar.Name()
	domain := models.TenantDomain{
		TenantSchema:  tenantID,
		Domain:        req.Domain,
		DomainType:    "registered",
		Verified:      true,
		VerifiedAt:    ptrTime(time.Now()),
		RegistrarID:   &result.RegistrarID,
		RegistrarName: &registrarName,
	}

	err = h.deps.DB.WithTenant(c.Request.Context(), tenantID, func(tx *gorm.DB) error {
		return tx.Create(&domain).Error
	})

	if err != nil {
		h.logger.Error("Failed to save registered domain", "error", err)
		// Domain was registered but save failed - return partial success
		c.JSON(http.StatusOK, gin.H{
			"registration": result,
			"warning":      "domain registered but failed to save to database",
		})
		return
	}

	c.JSON(http.StatusCreated, gin.H{
		"registration": result,
		"domain":       h.toResponse(&domain),
	})
}

func ptrTime(t time.Time) *time.Time {
	return &t
}

func (h *Handler) toResponse(domain *models.TenantDomain) DomainResponse {
	return DomainResponse{
		ID:            domain.ID,
		Domain:        domain.Domain,
		DomainType:    domain.DomainType,
		Verified:      domain.Verified,
		VerifiedAt:    domain.VerifiedAt,
		SSLEnabled:    domain.SSLEnabled,
		SSLExpiresAt:  domain.SSLExpiresAt,
		DNSConfigured: domain.DNSConfigured,
		Primary:       domain.Primary,
	}
}

// RegisterRoutes registers domain-related routes
func RegisterRoutes(r *gin.Engine, deps *sections.Dependencies, jwtManager *auth.JWTManager, registrar DomainRegistrar) {
	handler := NewHandler(deps, registrar)

	tenantCfg := auth.DefaultTenantMiddlewareConfig()

	domainRoutes := r.Group("/api/v1/domains")
	domainRoutes.Use(auth.JWTAuthMiddleware(jwtManager))
	domainRoutes.Use(auth.TenantFromHeaderMiddleware(tenantCfg))
	{
		domainRoutes.GET("", handler.ListDomains)
		domainRoutes.GET("/:domain", handler.GetDomain)
		domainRoutes.POST("", handler.AddDomain)
		domainRoutes.DELETE("/:domain", handler.DeleteDomain)
		domainRoutes.POST("/:domain/primary", handler.SetPrimaryDomain)
		domainRoutes.GET("/check", handler.CheckDomainAvailability)
		domainRoutes.POST("/register", handler.RegisterDomain)
	}
}
