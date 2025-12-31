package db

import (
	"context"
	"fmt"
	"log/slog"
	"os"

	_ "github.com/bartventer/gorm-multitenancy/postgres/v8"
	multitenancy "github.com/bartventer/gorm-multitenancy/v8"
	"github.com/bartventer/gorm-multitenancy/v8/pkg/driver"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// DB wraps the multitenancy database instance
type DB struct {
	*multitenancy.DB
}

// Config holds database configuration
type Config struct {
	DatabaseURL string
	Debug       bool
}

// NewConfig creates a database config from environment variables
func NewConfig() *Config {
	return &Config{
		DatabaseURL: os.Getenv("DATABASE_URL"),
		Debug:       os.Getenv("DB_DEBUG") == "true",
	}
}

// Connect establishes a connection to the PostgreSQL database with multitenancy support
// This is a simplified version that takes just a database URL
func Connect(databaseURL string) (*DB, error) {
	ctx := context.Background()
	cfg := &Config{
		DatabaseURL: databaseURL,
		Debug:       os.Getenv("DB_DEBUG") == "true",
	}
	return ConnectWithConfig(ctx, cfg)
}

// ConnectWithConfig establishes a connection to the PostgreSQL database with multitenancy support
func ConnectWithConfig(ctx context.Context, cfg *Config) (*DB, error) {
	if cfg.DatabaseURL == "" {
		return nil, fmt.Errorf("DATABASE_URL environment variable is required")
	}

	gormConfig := &gorm.Config{}
	if cfg.Debug {
		gormConfig.Logger = logger.Default.LogMode(logger.Info)
	}

	db, err := multitenancy.OpenDB(ctx, cfg.DatabaseURL, gormConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to database: %w", err)
	}

	slog.Info("Database connection established")
	return &DB{DB: db}, nil
}

// RegisterModels registers all models with the multitenancy database
func (db *DB) RegisterModels(ctx context.Context, models ...driver.TenantTabler) error {
	if err := db.DB.RegisterModels(ctx, models...); err != nil {
		return fmt.Errorf("failed to register models: %w", err)
	}
	slog.Info("Models registered", "count", len(models))
	return nil
}

// MigrateSharedModels migrates all shared/public models
func (db *DB) MigrateSharedModels(ctx context.Context) error {
	if err := db.DB.MigrateSharedModels(ctx); err != nil {
		return fmt.Errorf("failed to migrate shared models: %w", err)
	}
	slog.Info("Shared models migrated")
	return nil
}

// MigrateTenantModels migrates all tenant-specific models for a given tenant
func (db *DB) MigrateTenantModels(ctx context.Context, tenantID string) error {
	if err := db.DB.MigrateTenantModels(ctx, tenantID); err != nil {
		return fmt.Errorf("failed to migrate tenant models for %s: %w", tenantID, err)
	}
	slog.Info("Tenant models migrated", "tenant", tenantID)
	return nil
}

// CreateTenantSchema creates a new tenant schema and migrates tenant models
func (db *DB) CreateTenantSchema(ctx context.Context, tenantID string) error {
	if err := db.MigrateTenantModels(ctx, tenantID); err != nil {
		return fmt.Errorf("failed to create tenant schema: %w", err)
	}
	return nil
}

// DeleteTenantSchema removes a tenant schema and all associated data
func (db *DB) DeleteTenantSchema(ctx context.Context, tenantID string) error {
	if err := db.DB.OffboardTenant(ctx, tenantID); err != nil {
		return fmt.Errorf("failed to delete tenant schema: %w", err)
	}
	slog.Info("Tenant schema deleted", "tenant", tenantID)
	return nil
}

// WithTenant executes a function within a tenant's context
func (db *DB) WithTenant(ctx context.Context, tenantID string, fn func(tx *gorm.DB) error) error {
	return db.DB.WithTenant(ctx, tenantID, func(tx *multitenancy.DB) error {
		return fn(tx.DB)
	})
}

// UseTenant sets the database context to a specific tenant and returns a reset function
func (db *DB) UseTenant(ctx context.Context, tenantID string) (reset func() error, err error) {
	return db.DB.UseTenant(ctx, tenantID)
}

// Close closes the database connection
func (db *DB) Close() error {
	sqlDB, err := db.DB.DB.DB()
	if err != nil {
		return fmt.Errorf("failed to get underlying DB: %w", err)
	}
	return sqlDB.Close()
}
