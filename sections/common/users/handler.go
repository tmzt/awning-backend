package users

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"log/slog"
	"net/http"
	"time"

	"awning-backend/sections"
	"awning-backend/sections/common/auth"
	"awning-backend/sections/models"

	"github.com/gin-gonic/gin"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"
)

var (
	ErrUserNotFound       = errors.New("user not found")
	ErrInvalidCredentials = errors.New("invalid credentials")
	ErrUserExists         = errors.New("user already exists")
	ErrInvalidResetToken  = errors.New("invalid or expired reset token")
)

// Handler handles user-related requests
type Handler struct {
	logger     *slog.Logger
	deps       *sections.Dependencies
	jwtManager *auth.JWTManager
}

// NewHandler creates a new users handler
func NewHandler(deps *sections.Dependencies, jwtManager *auth.JWTManager) *Handler {
	return &Handler{
		logger:     slog.With("handler", "UsersHandler"),
		deps:       deps,
		jwtManager: jwtManager,
	}
}

// RegisterRequest represents a user registration request
type RegisterRequest struct {
	Email     string `json:"email" binding:"required,email"`
	Password  string `json:"password" binding:"required,min=8"`
	FirstName string `json:"firstName"`
	LastName  string `json:"lastName"`
}

// LoginRequest represents a login request
type LoginRequest struct {
	Email    string `json:"email" binding:"required,email"`
	Password string `json:"password" binding:"required"`
}

// PasswordResetRequest represents a password reset initiation request
type PasswordResetRequest struct {
	Email string `json:"email" binding:"required,email"`
}

// PasswordResetConfirmRequest represents a password reset confirmation request
type PasswordResetConfirmRequest struct {
	Token       string `json:"token" binding:"required"`
	NewPassword string `json:"newPassword" binding:"required,min=8"`
}

// AuthResponse represents an authentication response
type AuthResponse struct {
	Token string       `json:"token"`
	User  UserResponse `json:"user"`
}

// UserResponse represents a user in API responses
type UserResponse struct {
	ID            uint       `json:"id"`
	Email         string     `json:"email"`
	FirstName     string     `json:"firstName"`
	LastName      string     `json:"lastName"`
	EmailVerified bool       `json:"emailVerified"`
	LastLoginAt   *time.Time `json:"lastLoginAt,omitempty"`
}

// Register handles user registration
func (h *Handler) Register(c *gin.Context) {
	var req RegisterRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Check if user already exists
	var existingUser models.User
	if err := h.deps.DB.DB.Where("email = ?", req.Email).First(&existingUser).Error; err == nil {
		c.JSON(http.StatusConflict, gin.H{"error": "user with this email already exists"})
		return
	}

	// Hash password
	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(req.Password), bcrypt.DefaultCost)
	if err != nil {
		h.logger.Error("Failed to hash password", "error", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to create user"})
		return
	}

	// Create user
	user := models.User{
		Email:        req.Email,
		PasswordHash: string(hashedPassword),
		FirstName:    req.FirstName,
		LastName:     req.LastName,
		Active:       true,
	}

	if err := h.deps.DB.DB.Create(&user).Error; err != nil {
		h.logger.Error("Failed to create user", "error", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to create user"})
		return
	}

	// Generate JWT token
	token, err := h.jwtManager.GenerateToken(user.ID, user.Email, "")
	if err != nil {
		h.logger.Error("Failed to generate token", "error", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to generate token"})
		return
	}

	h.logger.Info("User registered", "userId", user.ID, "email", user.Email)

	c.JSON(http.StatusCreated, AuthResponse{
		Token: token,
		User:  h.toUserResponse(&user),
	})
}

// Login handles user login
func (h *Handler) Login(c *gin.Context) {
	var req LoginRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Find user
	var user models.User
	if err := h.deps.DB.DB.Where("email = ?", req.Email).First(&user).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid credentials"})
			return
		}
		h.logger.Error("Failed to find user", "error", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "login failed"})
		return
	}

	// Check if user is active
	if !user.Active {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "account is disabled"})
		return
	}

	// Verify password
	if err := bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(req.Password)); err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid credentials"})
		return
	}

	// Update last login time
	now := time.Now()
	h.deps.DB.DB.Model(&user).Update("last_login_at", now)
	user.LastLoginAt = &now

	// Get default tenant for user (if any)
	var userTenant models.UserTenant
	tenantSchema := ""
	if err := h.deps.DB.DB.Where("user_id = ?", user.ID).Order("created_at ASC").First(&userTenant).Error; err == nil {
		tenantSchema = userTenant.TenantSchema
	}

	// Generate JWT token
	token, err := h.jwtManager.GenerateToken(user.ID, user.Email, tenantSchema)
	if err != nil {
		h.logger.Error("Failed to generate token", "error", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to generate token"})
		return
	}

	h.logger.Info("User logged in", "userId", user.ID, "email", user.Email)

	c.JSON(http.StatusOK, AuthResponse{
		Token: token,
		User:  h.toUserResponse(&user),
	})
}

// RequestPasswordReset initiates a password reset
func (h *Handler) RequestPasswordReset(c *gin.Context) {
	var req PasswordResetRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Find user
	var user models.User
	if err := h.deps.DB.DB.Where("email = ?", req.Email).First(&user).Error; err != nil {
		// Don't reveal if user exists
		c.JSON(http.StatusOK, gin.H{"message": "if an account exists with this email, a reset link will be sent"})
		return
	}

	// Generate reset token
	tokenBytes := make([]byte, 32)
	if _, err := rand.Read(tokenBytes); err != nil {
		h.logger.Error("Failed to generate reset token", "error", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to initiate password reset"})
		return
	}
	token := hex.EncodeToString(tokenBytes)

	// Save token with expiry
	expires := time.Now().Add(1 * time.Hour)
	h.deps.DB.DB.Model(&user).Updates(map[string]interface{}{
		"password_reset_token":   token,
		"password_reset_expires": expires,
	})

	// TODO: Send email with reset link
	h.logger.Info("Password reset requested", "userId", user.ID, "email", user.Email)

	c.JSON(http.StatusOK, gin.H{"message": "if an account exists with this email, a reset link will be sent"})
}

// ConfirmPasswordReset completes a password reset
func (h *Handler) ConfirmPasswordReset(c *gin.Context) {
	var req PasswordResetConfirmRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Find user by reset token
	var user models.User
	if err := h.deps.DB.DB.Where("password_reset_token = ?", req.Token).First(&user).Error; err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid or expired reset token"})
		return
	}

	// Check if token is expired
	if user.PasswordResetExpires == nil || user.PasswordResetExpires.Before(time.Now()) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid or expired reset token"})
		return
	}

	// Hash new password
	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(req.NewPassword), bcrypt.DefaultCost)
	if err != nil {
		h.logger.Error("Failed to hash password", "error", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to reset password"})
		return
	}

	// Update password and clear reset token
	h.deps.DB.DB.Model(&user).Updates(map[string]interface{}{
		"password_hash":          string(hashedPassword),
		"password_reset_token":   nil,
		"password_reset_expires": nil,
	})

	h.logger.Info("Password reset completed", "userId", user.ID)

	c.JSON(http.StatusOK, gin.H{"message": "password has been reset successfully"})
}

// GetProfile returns the current user's profile
func (h *Handler) GetProfile(c *gin.Context) {
	userID, ok := auth.GetUserIDFromContext(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}

	var user models.User
	if err := h.deps.DB.DB.First(&user, userID).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "user not found"})
		return
	}

	c.JSON(http.StatusOK, h.toUserResponse(&user))
}

// UpdateProfile updates the current user's profile
func (h *Handler) UpdateProfile(c *gin.Context) {
	userID, ok := auth.GetUserIDFromContext(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}

	var req struct {
		FirstName string `json:"firstName"`
		LastName  string `json:"lastName"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	var user models.User
	if err := h.deps.DB.DB.First(&user, userID).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "user not found"})
		return
	}

	user.FirstName = req.FirstName
	user.LastName = req.LastName
	h.deps.DB.DB.Save(&user)

	c.JSON(http.StatusOK, h.toUserResponse(&user))
}

// GetTenants returns the tenants the user belongs to
func (h *Handler) GetTenants(c *gin.Context) {
	userID, ok := auth.GetUserIDFromContext(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}

	var userTenants []models.UserTenant
	if err := h.deps.DB.DB.Preload("Tenant").Where("user_id = ?", userID).Find(&userTenants).Error; err != nil {
		h.logger.Error("Failed to get user tenants", "error", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to get tenants"})
		return
	}

	type TenantResponse struct {
		SchemaName  string `json:"schemaName"`
		DomainURL   string `json:"domainUrl"`
		Name        string `json:"name"`
		DisplayName string `json:"displayName"`
		Role        string `json:"role"`
	}

	tenants := make([]TenantResponse, len(userTenants))
	for i, ut := range userTenants {
		tenants[i] = TenantResponse{
			SchemaName:  ut.Tenant.SchemaName,
			DomainURL:   ut.Tenant.DomainURL,
			Name:        ut.Tenant.Name,
			DisplayName: ut.Tenant.DisplayName,
			Role:        ut.Role,
		}
	}

	c.JSON(http.StatusOK, gin.H{"tenants": tenants})
}

func (h *Handler) toUserResponse(user *models.User) UserResponse {
	return UserResponse{
		ID:            user.ID,
		Email:         user.Email,
		FirstName:     user.FirstName,
		LastName:      user.LastName,
		EmailVerified: user.EmailVerified,
		LastLoginAt:   user.LastLoginAt,
	}
}

// RegisterRoutes registers all user-related routes
func RegisterRoutes(r *gin.Engine, deps *sections.Dependencies, jwtManager *auth.JWTManager) {
	handler := NewHandler(deps, jwtManager)

	// Public routes (no auth required)
	public := r.Group("/api/v1/auth")
	{
		public.POST("/register", handler.Register)
		public.POST("/login", handler.Login)
		public.POST("/password-reset/request", handler.RequestPasswordReset)
		public.POST("/password-reset/confirm", handler.ConfirmPasswordReset)
	}

	// Protected routes (auth required)
	protected := r.Group("/api/v1/users")
	protected.Use(auth.JWTAuthMiddleware(jwtManager))
	{
		protected.GET("/me", handler.GetProfile)
		protected.PUT("/me", handler.UpdateProfile)
		protected.GET("/me/tenants", handler.GetTenants)
	}
}
