package models

import (
	"time"

	"gorm.io/gorm"
)

// TenantProfile stores tenant-specific profile data (tenant-scoped model)
type TenantProfile struct {
	gorm.Model
	TenantSchema string `gorm:"size:63;not null;index" json:"tenantSchema"`
	BusinessName string `gorm:"size:255" json:"businessName"`
	Description  string `gorm:"type:text" json:"description"`
	LogoURL      string `gorm:"size:512" json:"logoUrl"`
	Website      string `gorm:"size:255" json:"website"`
	Phone        string `gorm:"size:50" json:"phone"`
	Email        string `gorm:"size:255" json:"email"`
	Address      string `gorm:"type:text" json:"address"`
	Timezone     string `gorm:"size:50;default:'UTC'" json:"timezone"`
	Locale       string `gorm:"size:10;default:'en-US'" json:"locale"`
	Metadata     string `gorm:"type:jsonb" json:"metadata"` // Additional JSON metadata
}

// TableName returns the table name (no prefix for tenant-scoped)
func (TenantProfile) TableName() string {
	return "profiles"
}

// IsSharedModel indicates this is a tenant-specific model
func (TenantProfile) IsSharedModel() bool {
	return false
}

// TenantAccount stores account/billing information (tenant-scoped model)
type TenantAccount struct {
	gorm.Model
	TenantSchema      string     `gorm:"size:63;not null;index" json:"tenantSchema"`
	PaidAccount       bool       `gorm:"default:false" json:"paidAccount"`
	BasicCredits      int        `gorm:"default:0" json:"basicCredits"`
	PremiumCredits    int        `gorm:"default:0" json:"premiumCredits"`
	SubscriptionPlan  string     `gorm:"size:50" json:"subscriptionPlan"` // free, basic, premium, enterprise
	SubscriptionStart *time.Time `json:"subscriptionStart,omitempty"`
	SubscriptionEnd   *time.Time `json:"subscriptionEnd,omitempty"`
	DomainRegistered  bool       `gorm:"default:false" json:"domainRegistered"`
	StripeCustomerID  *string    `gorm:"size:255" json:"-"`
}

// TableName returns the table name (no prefix for tenant-scoped)
func (TenantAccount) TableName() string {
	return "accounts"
}

// IsSharedModel indicates this is a tenant-specific model
func (TenantAccount) IsSharedModel() bool {
	return false
}

// TenantDomain stores domain configuration (tenant-scoped model)
type TenantDomain struct {
	gorm.Model
	TenantSchema  string     `gorm:"size:63;not null;index" json:"tenantSchema"`
	Domain        string     `gorm:"size:255;not null;uniqueIndex" json:"domain"`
	DomainType    string     `gorm:"size:50;default:'subdomain'" json:"domainType"` // subdomain, custom, registered
	Verified      bool       `gorm:"default:false" json:"verified"`
	VerifiedAt    *time.Time `json:"verifiedAt,omitempty"`
	SSLEnabled    bool       `gorm:"default:false" json:"sslEnabled"`
	SSLExpiresAt  *time.Time `json:"sslExpiresAt,omitempty"`
	RegistrarID   *string    `gorm:"size:255" json:"-"` // External registrar reference ID
	RegistrarName *string    `gorm:"size:100" json:"-"` // namecheap, cloudflare, opensrs
	DNSConfigured bool       `gorm:"default:false" json:"dnsConfigured"`
	Primary       bool       `gorm:"default:false" json:"primary"`
}

// TableName returns the table name (no prefix for tenant-scoped)
func (TenantDomain) TableName() string {
	return "domains"
}

// IsSharedModel indicates this is a tenant-specific model
func (TenantDomain) IsSharedModel() bool {
	return false
}

// TenantFilesystem stores JSON blobs (tenant-scoped model)
type TenantFilesystem struct {
	gorm.Model
	TenantSchema string `gorm:"size:63;not null;index" json:"tenantSchema"`
	Key          string `gorm:"size:255;not null;index" json:"key"` // Path-like key
	Data         string `gorm:"type:jsonb;not null" json:"data"`
	ContentType  string `gorm:"size:100;default:'application/json'" json:"contentType"`
	Size         int64  `gorm:"default:0" json:"size"`
	Checksum     string `gorm:"size:64" json:"checksum"` // SHA256 hash
}

// TableName returns the table name (no prefix for tenant-scoped)
func (TenantFilesystem) TableName() string {
	return "filesystem"
}

// IsSharedModel indicates this is a tenant-specific model
func (TenantFilesystem) IsSharedModel() bool {
	return false
}

// TenantChat stores chat sessions (tenant-scoped model)
type TenantChat struct {
	gorm.Model
	TenantSchema string `gorm:"size:63;not null;index" json:"tenantSchema"`
	ChatID       string `gorm:"size:36;not null;uniqueIndex" json:"chatId"` // UUID
	Messages     string `gorm:"type:jsonb" json:"messages"`                 // JSON array of messages
	ChatStage    string `gorm:"size:50" json:"chatStage"`
	LastRole     string `gorm:"size:20" json:"lastRole"`
}

// TableName returns the table name (no prefix for tenant-scoped)
func (TenantChat) TableName() string {
	return "chats"
}

// IsSharedModel indicates this is a tenant-specific model
func (TenantChat) IsSharedModel() bool {
	return false
}
