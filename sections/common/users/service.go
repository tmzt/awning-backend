package users

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"awning-backend/sections"
	"awning-backend/sections/models"

	"gorm.io/gorm"
)

// UserService handles user and tenant creation logic
type UserService struct {
	logger *slog.Logger
	deps   *sections.Dependencies
}

// NewUserService creates a new user service
func NewUserService(deps *sections.Dependencies) *UserService {
	return &UserService{
		logger: slog.With("service", "UserService"),
		deps:   deps,
	}
}

// CreateUserWithTenantParams holds parameters for creating a user with tenant
type CreateUserWithTenantParams struct {
	User         models.User
	TenantName   string // Optional: if not provided, uses email domain
	TenantSchema string // Optional: if not provided, auto-generated
}

// CreateUserWithTenant creates a user and associated tenant, making the user admin if they're the first
func (s *UserService) CreateUserWithTenant(ctx context.Context, params CreateUserWithTenantParams) (*models.User, *models.Tenant, error) {
	// Generate tenant name if not provided
	tenantName := params.TenantName
	if tenantName == "" {
		tenantName = s.generateTenantNameFromEmail(params.User.Email)
	}

	// Generate tenant schema if not provided
	tenantSchema := params.TenantSchema
	if tenantSchema == "" {
		tenantSchema = s.generateTenantSchema(params.User.Email)
	}

	// Start transaction
	tx := s.deps.DB.DB.Begin()
	if tx.Error != nil {
		return nil, nil, fmt.Errorf("failed to start transaction: %w", tx.Error)
	}
	defer func() {
		if r := recover(); r != nil {
			tx.Rollback()
		}
	}()

	// Create user
	if err := tx.Create(&params.User).Error; err != nil {
		tx.Rollback()
		return nil, nil, fmt.Errorf("failed to create user: %w", err)
	}

	s.logger.Info("User created", "user_id", params.User.ID, "email", params.User.Email)

	// Check if tenant exists
	var tenant models.Tenant
	err := tx.Where("schema_name = ?", tenantSchema).First(&tenant).Error
	tenantExists := err == nil

	if !tenantExists {
		// Create new tenant
		tenant = models.Tenant{
			Name:        tenantName,
			DisplayName: tenantName,
			Active:      true,
		}
		tenant.SchemaName = tenantSchema
		tenant.DomainURL = tenantSchema

		if err := tx.Create(&tenant).Error; err != nil {
			tx.Rollback()
			return nil, nil, fmt.Errorf("failed to create tenant: %w", err)
		}

		s.logger.Info("Tenant created", "tenant_schema", tenantSchema, "tenant_name", tenantName)

		// Migrate tenant schema
		if err := s.deps.DB.CreateTenantSchema(ctx, tenantSchema); err != nil {
			tx.Rollback()
			return nil, nil, fmt.Errorf("failed to migrate tenant schema: %w", err)
		}

		s.logger.Info("Tenant schema migrated", "tenant_schema", tenantSchema)
	} else {
		s.logger.Info("Using existing tenant", "tenant_schema", tenantSchema)
	}

	// Check if this is the first user for the tenant
	var userTenantCount int64
	if err := tx.Model(&models.UserTenant{}).
		Where("tenant_schema = ?", tenantSchema).
		Count(&userTenantCount).Error; err != nil {
		tx.Rollback()
		return nil, nil, fmt.Errorf("failed to count tenant users: %w", err)
	}

	// Determine role: first user is admin, others are members
	role := "member"
	if userTenantCount == 0 {
		role = "admin"
	}

	// Link user to tenant
	userTenant := models.UserTenant{
		UserID:        params.User.ID,
		TenantSchema:  tenantSchema,
		Role:          role,
		PrimaryTenant: true, // First tenant is always primary
	}

	if err := tx.Create(&userTenant).Error; err != nil {
		tx.Rollback()
		return nil, nil, fmt.Errorf("failed to link user to tenant: %w", err)
	}

	s.logger.Info("User linked to tenant",
		"user_id", params.User.ID,
		"tenant_schema", tenantSchema,
		"role", role,
		"is_first_user", role == "admin")

	// Commit transaction
	if err := tx.Commit().Error; err != nil {
		return nil, nil, fmt.Errorf("failed to commit transaction: %w", err)
	}

	return &params.User, &tenant, nil
}

// FindOrCreateUserWithOAuth finds existing user or creates new one with tenant for OAuth login
func (s *UserService) FindOrCreateUserWithOAuth(ctx context.Context, user models.User, oauthProvider string, oauthID string) (*models.User, error) {
	// Try to find by OAuth ID
	var existingUser models.User
	var err error

	switch oauthProvider {
	case "google":
		err = s.deps.DB.DB.Where("google_id = ?", oauthID).First(&existingUser).Error
	case "facebook":
		err = s.deps.DB.DB.Where("facebook_id = ?", oauthID).First(&existingUser).Error
	case "tiktok":
		err = s.deps.DB.DB.Where("tiktok_id = ?", oauthID).First(&existingUser).Error
	default:
		return nil, fmt.Errorf("unsupported OAuth provider: %s", oauthProvider)
	}

	if err == nil {
		// User found, update last login
		now := time.Now()
		s.deps.DB.DB.Model(&existingUser).Update("last_login_at", now)
		return &existingUser, nil
	}

	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, fmt.Errorf("failed to query user: %w", err)
	}

	// Try to find by email and link OAuth account
	if user.Email != "" {
		err = s.deps.DB.DB.Where("email = ?", user.Email).First(&existingUser).Error
		if err == nil {
			// Link OAuth account to existing user
			updates := map[string]interface{}{
				"last_login_at": time.Now(),
			}
			switch oauthProvider {
			case "google":
				updates["google_id"] = oauthID
			case "facebook":
				updates["facebook_id"] = oauthID
			case "tiktok":
				updates["tiktok_id"] = oauthID
			}

			if err := s.deps.DB.DB.Model(&existingUser).Updates(updates).Error; err != nil {
				return nil, fmt.Errorf("failed to link OAuth account: %w", err)
			}

			s.logger.Info("OAuth account linked to existing user",
				"user_id", existingUser.ID,
				"provider", oauthProvider,
				"email", user.Email)

			return &existingUser, nil
		}

		if !errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, fmt.Errorf("failed to query user by email: %w", err)
		}
	}

	// User doesn't exist, create with tenant
	createdUser, tenant, err := s.CreateUserWithTenant(ctx, CreateUserWithTenantParams{
		User: user,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create user with tenant: %w", err)
	}

	s.logger.Info("New user created via OAuth",
		"user_id", createdUser.ID,
		"provider", oauthProvider,
		"email", user.Email,
		"tenant_schema", tenant.SchemaName)

	return createdUser, nil
}

// GetPrimaryTenantSchema returns the primary tenant schema for a user
func (s *UserService) GetPrimaryTenantSchema(ctx context.Context, userID uint) (string, error) {
	var userTenant models.UserTenant
	err := s.deps.DB.DB.Where("user_id = ? AND primary_tenant = ?", userID, true).First(&userTenant).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			// Fallback: get the first tenant if no primary is set
			err = s.deps.DB.DB.Where("user_id = ?", userID).Order("created_at ASC").First(&userTenant).Error
			if err != nil {
				return "", fmt.Errorf("no tenant found for user: %w", err)
			}
			s.logger.Warn("No primary tenant found, using first tenant",
				"user_id", userID,
				"tenant_schema", userTenant.TenantSchema)
			return userTenant.TenantSchema, nil
		}
		return "", fmt.Errorf("failed to query primary tenant: %w", err)
	}
	return userTenant.TenantSchema, nil
}

// generateTenantNameFromEmail generates a tenant name from email
func (s *UserService) generateTenantNameFromEmail(email string) string {
	parts := strings.Split(email, "@")
	if len(parts) > 0 {
		username := parts[0]
		// Capitalize first letter
		if len(username) > 0 {
			return strings.ToUpper(username[:1]) + username[1:] + "'s Workspace"
		}
	}
	return "My Workspace"
}

// generateTenantSchema generates a unique tenant schema name
func (s *UserService) generateTenantSchema(email string) string {
	parts := strings.Split(email, "@")
	if len(parts) > 0 {
		username := parts[0]
		// Remove non-alphanumeric characters and convert to lowercase
		schema := strings.ToLower(username)
		schema = strings.Map(func(r rune) rune {
			if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
				return r
			}
			return '_'
		}, schema)

		// Ensure it starts with a letter
		if len(schema) > 0 && schema[0] >= '0' && schema[0] <= '9' {
			schema = "t_" + schema
		}

		// Check if schema exists, append number if needed
		var tenant models.Tenant
		baseSchema := schema
		counter := 1
		for {
			err := s.deps.DB.DB.Where("schema_name = ?", schema).First(&tenant).Error
			if errors.Is(err, gorm.ErrRecordNotFound) {
				break
			}
			schema = fmt.Sprintf("%s_%d", baseSchema, counter)
			counter++
		}

		return schema
	}
	return "tenant_" + fmt.Sprintf("%d", time.Now().Unix())
}
