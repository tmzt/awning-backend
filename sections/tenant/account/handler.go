package account

import (
	"log/slog"
	"net/http"

	"awning-backend/sections"
	"awning-backend/sections/common/auth"
	"awning-backend/sections/models"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// Handler handles account-related requests
type Handler struct {
	logger *slog.Logger
	deps   *sections.Dependencies
}

// NewHandler creates a new account handler
func NewHandler(deps *sections.Dependencies) *Handler {
	return &Handler{
		logger: slog.With("handler", "AccountHandler"),
		deps:   deps,
	}
}

// AccountResponse represents an account response
type AccountResponse struct {
	ID               uint   `json:"id"`
	TenantSchema     string `json:"tenantSchema"`
	PaidAccount      bool   `json:"paidAccount"`
	BasicCredits     int    `json:"basicCredits"`
	PremiumCredits   int    `json:"premiumCredits"`
	SubscriptionPlan string `json:"subscriptionPlan"`
	DomainRegistered bool   `json:"domainRegistered"`
}

// UpdateAccountRequest represents an account update request
type UpdateAccountRequest struct {
	BasicCredits     *int    `json:"basicCredits,omitempty"`
	PremiumCredits   *int    `json:"premiumCredits,omitempty"`
	SubscriptionPlan *string `json:"subscriptionPlan,omitempty"`
}

// GetAccount retrieves the tenant account
func (h *Handler) GetAccount(c *gin.Context) {
	tenantID, ok := auth.GetTenantIDFromContext(c)
	if !ok {
		c.JSON(http.StatusBadRequest, gin.H{"error": "tenant context required"})
		return
	}

	var account models.TenantAccount
	err := h.deps.DB.WithTenant(c.Request.Context(), tenantID, func(tx *gorm.DB) error {
		return tx.Where("tenant_schema = ?", tenantID).First(&account).Error
	})

	if err != nil {
		// If not found, return default account
		c.JSON(http.StatusOK, AccountResponse{
			TenantSchema:     tenantID,
			PaidAccount:      false,
			BasicCredits:     0,
			PremiumCredits:   0,
			SubscriptionPlan: "free",
			DomainRegistered: false,
		})
		return
	}

	c.JSON(http.StatusOK, h.toResponse(&account))
}

// UpdateAccount updates the tenant account
func (h *Handler) UpdateAccount(c *gin.Context) {
	tenantID, ok := auth.GetTenantIDFromContext(c)
	if !ok {
		c.JSON(http.StatusBadRequest, gin.H{"error": "tenant context required"})
		return
	}

	var req UpdateAccountRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	var account models.TenantAccount
	err := h.deps.DB.WithTenant(c.Request.Context(), tenantID, func(tx *gorm.DB) error {
		err := tx.Where("tenant_schema = ?", tenantID).First(&account).Error
		if err != nil {
			account = models.TenantAccount{
				TenantSchema:     tenantID,
				SubscriptionPlan: "free",
			}
		}

		if req.BasicCredits != nil {
			account.BasicCredits = *req.BasicCredits
		}
		if req.PremiumCredits != nil {
			account.PremiumCredits = *req.PremiumCredits
		}
		if req.SubscriptionPlan != nil {
			account.SubscriptionPlan = *req.SubscriptionPlan
			account.PaidAccount = *req.SubscriptionPlan != "free"
		}

		return tx.Save(&account).Error
	})

	if err != nil {
		h.logger.Error("Failed to update account", "error", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to update account"})
		return
	}

	c.JSON(http.StatusOK, h.toResponse(&account))
}

// AddCredits adds credits to the tenant account
func (h *Handler) AddCredits(c *gin.Context) {
	tenantID, ok := auth.GetTenantIDFromContext(c)
	if !ok {
		c.JSON(http.StatusBadRequest, gin.H{"error": "tenant context required"})
		return
	}

	var req struct {
		Type   string `json:"type" binding:"required,oneof=basic premium"`
		Amount int    `json:"amount" binding:"required,min=1"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	var account models.TenantAccount
	err := h.deps.DB.WithTenant(c.Request.Context(), tenantID, func(tx *gorm.DB) error {
		err := tx.Where("tenant_schema = ?", tenantID).First(&account).Error
		if err != nil {
			account = models.TenantAccount{
				TenantSchema:     tenantID,
				SubscriptionPlan: "free",
			}
		}

		if req.Type == "basic" {
			account.BasicCredits += req.Amount
		} else {
			account.PremiumCredits += req.Amount
		}

		return tx.Save(&account).Error
	})

	if err != nil {
		h.logger.Error("Failed to add credits", "error", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to add credits"})
		return
	}

	c.JSON(http.StatusOK, h.toResponse(&account))
}

// UseCredits deducts credits from the tenant account
func (h *Handler) UseCredits(c *gin.Context) {
	tenantID, ok := auth.GetTenantIDFromContext(c)
	if !ok {
		c.JSON(http.StatusBadRequest, gin.H{"error": "tenant context required"})
		return
	}

	var req struct {
		Type   string `json:"type" binding:"required,oneof=basic premium"`
		Amount int    `json:"amount" binding:"required,min=1"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	var account models.TenantAccount
	err := h.deps.DB.WithTenant(c.Request.Context(), tenantID, func(tx *gorm.DB) error {
		err := tx.Where("tenant_schema = ?", tenantID).First(&account).Error
		if err != nil {
			return err
		}

		if req.Type == "basic" {
			if account.BasicCredits < req.Amount {
				return ErrInsufficientCredits
			}
			account.BasicCredits -= req.Amount
		} else {
			if account.PremiumCredits < req.Amount {
				return ErrInsufficientCredits
			}
			account.PremiumCredits -= req.Amount
		}

		return tx.Save(&account).Error
	})

	if err == ErrInsufficientCredits {
		c.JSON(http.StatusBadRequest, gin.H{"error": "insufficient credits"})
		return
	}
	if err != nil {
		h.logger.Error("Failed to use credits", "error", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to use credits"})
		return
	}

	c.JSON(http.StatusOK, h.toResponse(&account))
}

var ErrInsufficientCredits = &AccountError{Message: "insufficient credits"}

type AccountError struct {
	Message string
}

func (e *AccountError) Error() string {
	return e.Message
}

func (h *Handler) toResponse(account *models.TenantAccount) AccountResponse {
	return AccountResponse{
		ID:               account.ID,
		TenantSchema:     account.TenantSchema,
		PaidAccount:      account.PaidAccount,
		BasicCredits:     account.BasicCredits,
		PremiumCredits:   account.PremiumCredits,
		SubscriptionPlan: account.SubscriptionPlan,
		DomainRegistered: account.DomainRegistered,
	}
}

// RegisterRoutes registers account-related routes
func RegisterRoutes(r *gin.RouterGroup, deps *sections.Dependencies, jwtManager *auth.JWTManager) {
	handler := NewHandler(deps)

	tenantCfg := auth.DefaultTenantMiddlewareConfig()

	accountRoutes := r.Group("/api/v1/account")
	accountRoutes.Use(auth.JWTAuthMiddleware(jwtManager))
	accountRoutes.Use(auth.TenantFromHeaderMiddleware(tenantCfg))
	{
		accountRoutes.GET("", handler.GetAccount)
		accountRoutes.PUT("", handler.UpdateAccount)
		accountRoutes.POST("/credits/add", handler.AddCredits)
		accountRoutes.POST("/credits/use", handler.UseCredits)
	}
}
