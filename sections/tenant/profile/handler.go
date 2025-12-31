package profile

import (
	"log/slog"
	"net/http"

	"awning-backend/sections"
	"awning-backend/sections/common/auth"
	"awning-backend/sections/models"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// Handler handles profile-related requests
type Handler struct {
	logger *slog.Logger
	deps   *sections.Dependencies
}

// NewHandler creates a new profile handler
func NewHandler(deps *sections.Dependencies) *Handler {
	return &Handler{
		logger: slog.With("handler", "ProfileHandler"),
		deps:   deps,
	}
}

// ProfileRequest represents a profile update request
type ProfileRequest struct {
	BusinessName string `json:"businessName"`
	Description  string `json:"description"`
	LogoURL      string `json:"logoUrl"`
	Website      string `json:"website"`
	Phone        string `json:"phone"`
	Email        string `json:"email"`
	Address      string `json:"address"`
	Timezone     string `json:"timezone"`
	Locale       string `json:"locale"`
	Metadata     string `json:"metadata"`
}

// ProfileResponse represents a profile response
type ProfileResponse struct {
	ID           uint   `json:"id"`
	TenantSchema string `json:"tenantSchema"`
	BusinessName string `json:"businessName"`
	Description  string `json:"description"`
	LogoURL      string `json:"logoUrl"`
	Website      string `json:"website"`
	Phone        string `json:"phone"`
	Email        string `json:"email"`
	Address      string `json:"address"`
	Timezone     string `json:"timezone"`
	Locale       string `json:"locale"`
	Metadata     string `json:"metadata"`
}

// GetProfile retrieves the tenant profile
func (h *Handler) GetProfile(c *gin.Context) {
	tenantID, ok := auth.GetTenantIDFromContext(c)
	if !ok {
		c.JSON(http.StatusBadRequest, gin.H{"error": "tenant context required"})
		return
	}

	var profile models.TenantProfile
	err := h.deps.DB.WithTenant(c.Request.Context(), tenantID, func(tx *gorm.DB) error {
		// Use the DB within tenant context
		return tx.Where("tenant_schema = ?", tenantID).First(&profile).Error
	})

	if err != nil {
		// If not found, return empty profile
		c.JSON(http.StatusOK, ProfileResponse{
			TenantSchema: tenantID,
			Timezone:     "UTC",
			Locale:       "en-US",
		})
		return
	}

	c.JSON(http.StatusOK, h.toResponse(&profile))
}

// UpdateProfile updates or creates the tenant profile
func (h *Handler) UpdateProfile(c *gin.Context) {
	tenantID, ok := auth.GetTenantIDFromContext(c)
	if !ok {
		c.JSON(http.StatusBadRequest, gin.H{"error": "tenant context required"})
		return
	}

	var req ProfileRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	var profile models.TenantProfile
	err := h.deps.DB.WithTenant(c.Request.Context(), tenantID, func(tx *gorm.DB) error {
		// Try to find existing profile
		err := tx.Where("tenant_schema = ?", tenantID).First(&profile).Error
		if err != nil {
			// Create new profile
			profile = models.TenantProfile{
				TenantSchema: tenantID,
			}
		}

		// Update fields
		profile.BusinessName = req.BusinessName
		profile.Description = req.Description
		profile.LogoURL = req.LogoURL
		profile.Website = req.Website
		profile.Phone = req.Phone
		profile.Email = req.Email
		profile.Address = req.Address
		if req.Timezone != "" {
			profile.Timezone = req.Timezone
		}
		if req.Locale != "" {
			profile.Locale = req.Locale
		}
		profile.Metadata = req.Metadata

		return tx.Save(&profile).Error
	})

	if err != nil {
		h.logger.Error("Failed to update profile", "error", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to update profile"})
		return
	}

	c.JSON(http.StatusOK, h.toResponse(&profile))
}

func (h *Handler) toResponse(profile *models.TenantProfile) ProfileResponse {
	return ProfileResponse{
		ID:           profile.ID,
		TenantSchema: profile.TenantSchema,
		BusinessName: profile.BusinessName,
		Description:  profile.Description,
		LogoURL:      profile.LogoURL,
		Website:      profile.Website,
		Phone:        profile.Phone,
		Email:        profile.Email,
		Address:      profile.Address,
		Timezone:     profile.Timezone,
		Locale:       profile.Locale,
		Metadata:     profile.Metadata,
	}
}

// RegisterRoutes registers profile-related routes
func RegisterRoutes(r *gin.Engine, deps *sections.Dependencies, jwtManager *auth.JWTManager) {
	handler := NewHandler(deps)

	tenantCfg := auth.DefaultTenantMiddlewareConfig()

	profileRoutes := r.Group("/api/v1/profile")
	profileRoutes.Use(auth.JWTAuthMiddleware(jwtManager))
	profileRoutes.Use(auth.TenantFromHeaderMiddleware(tenantCfg))
	{
		profileRoutes.GET("", handler.GetProfile)
		profileRoutes.PUT("", handler.UpdateProfile)
	}
}
