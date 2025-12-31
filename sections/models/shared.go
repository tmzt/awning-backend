package models

import (
	"time"

	multitenancy "github.com/bartventer/gorm-multitenancy/v8"
	"gorm.io/gorm"
)

// Tenant represents a tenant in the system (public/shared model)
type Tenant struct {
	multitenancy.TenantModel
	gorm.Model
	Name        string `gorm:"size:255;not null" json:"name"`
	DisplayName string `gorm:"size:255" json:"displayName"`
	Active      bool   `gorm:"default:true" json:"active"`
}

// TableName returns the table name with public schema prefix
func (Tenant) TableName() string {
	return "public.tenants"
}

// IsSharedModel indicates this is a shared/public model
func (Tenant) IsSharedModel() bool {
	return true
}

// User represents a user in the system (public/shared model)
type User struct {
	gorm.Model
	Email           string     `gorm:"uniqueIndex;size:255;not null" json:"email"`
	PasswordHash    string     `gorm:"size:255" json:"-"`
	FirstName       string     `gorm:"size:100" json:"firstName"`
	LastName        string     `gorm:"size:100" json:"lastName"`
	EmailVerified   bool       `gorm:"default:false" json:"emailVerified"`
	EmailVerifiedAt *time.Time `json:"emailVerifiedAt,omitempty"`
	LastLoginAt     *time.Time `json:"lastLoginAt,omitempty"`
	Active          bool       `gorm:"default:true" json:"active"`

	// OAuth fields
	GoogleID   *string `gorm:"uniqueIndex;size:255" json:"-"`
	FacebookID *string `gorm:"uniqueIndex;size:255" json:"-"`
	TikTokID   *string `gorm:"uniqueIndex;size:255" json:"-"`

	// Password reset
	PasswordResetToken   *string    `gorm:"size:255" json:"-"`
	PasswordResetExpires *time.Time `json:"-"`
}

// TableName returns the table name with public schema prefix
func (User) TableName() string {
	return "public.users"
}

// IsSharedModel indicates this is a shared/public model
func (User) IsSharedModel() bool {
	return true
}

// UserTenant links users to tenants (public/shared model)
type UserTenant struct {
	gorm.Model
	UserID       uint   `gorm:"not null;index" json:"userId"`
	TenantSchema string `gorm:"size:63;not null;index" json:"tenantSchema"`
	Role         string `gorm:"size:50;default:'member'" json:"role"` // owner, admin, member
	User         User   `gorm:"foreignKey:UserID" json:"-"`
	Tenant       Tenant `gorm:"foreignKey:TenantSchema;references:SchemaName" json:"-"`
}

// TableName returns the table name with public schema prefix
func (UserTenant) TableName() string {
	return "public.user_tenants"
}

// IsSharedModel indicates this is a shared/public model
func (UserTenant) IsSharedModel() bool {
	return true
}
