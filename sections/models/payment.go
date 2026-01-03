package models

import (
	"time"

	"gorm.io/gorm"
)

// Payment represents a one-time payment transaction
type Payment struct {
	gorm.Model
	TenantSchema string `gorm:"size:63;not null;index" json:"tenantSchema"`
	UserID       uint   `gorm:"not null;index" json:"userId"`

	// Stripe fields
	StripePaymentIntentID string `gorm:"uniqueIndex;size:255;not null" json:"stripePaymentIntentId"`
	StripeCustomerID      string `gorm:"size:255;index" json:"stripeCustomerId"`
	Amount                int64  `gorm:"not null" json:"amount"` // Amount in cents
	Currency              string `gorm:"size:3;not null;default:'usd'" json:"currency"`
	Status                string `gorm:"size:50;not null;default:'pending'" json:"status"` // pending, succeeded, failed, canceled
	Description           string `gorm:"size:500" json:"description"`
	Metadata              string `gorm:"type:jsonb" json:"metadata,omitempty"` // JSON string for additional data

	// Payment details
	PaymentMethod string     `gorm:"size:50" json:"paymentMethod"` // card, etc.
	PaidAt        *time.Time `json:"paidAt,omitempty"`

	// Relations
	User   User   `gorm:"foreignKey:UserID" json:"-"`
	Tenant Tenant `gorm:"foreignKey:TenantSchema;references:SchemaName" json:"-"`
}

// TableName returns the table name with public schema prefix
func (Payment) TableName() string {
	return "public.payments"
}

// IsSharedModel indicates this is a shared/public model
func (Payment) IsSharedModel() bool {
	return true
}

// Subscription represents a recurring subscription
type Subscription struct {
	gorm.Model
	TenantSchema string `gorm:"size:63;not null;index" json:"tenantSchema"`
	UserID       uint   `gorm:"not null;index" json:"userId"`

	// Stripe fields
	StripeSubscriptionID string `gorm:"uniqueIndex;size:255;not null" json:"stripeSubscriptionId"`
	StripeCustomerID     string `gorm:"size:255;index;not null" json:"stripeCustomerId"`
	StripePriceID        string `gorm:"size:255;not null" json:"stripePriceId"`
	StripeProductID      string `gorm:"size:255" json:"stripeProductId"`

	// Subscription details
	Status        string `gorm:"size:50;not null;default:'active'" json:"status"` // active, canceled, past_due, incomplete, trialing, unpaid
	PlanName      string `gorm:"size:100;not null" json:"planName"`
	Amount        int64  `gorm:"not null" json:"amount"` // Amount in cents
	Currency      string `gorm:"size:3;not null;default:'usd'" json:"currency"`
	Interval      string `gorm:"size:20;not null" json:"interval"` // month, year
	IntervalCount int    `gorm:"not null;default:1" json:"intervalCount"`

	// Lifecycle dates
	CurrentPeriodStart time.Time  `gorm:"not null" json:"currentPeriodStart"`
	CurrentPeriodEnd   time.Time  `gorm:"not null" json:"currentPeriodEnd"`
	CanceledAt         *time.Time `json:"canceledAt,omitempty"`
	CancelAtPeriodEnd  bool       `gorm:"default:false" json:"cancelAtPeriodEnd"`
	TrialStart         *time.Time `json:"trialStart,omitempty"`
	TrialEnd           *time.Time `json:"trialEnd,omitempty"`

	Metadata string `gorm:"type:jsonb" json:"metadata,omitempty"` // JSON string for additional data

	// Relations
	User   User   `gorm:"foreignKey:UserID" json:"-"`
	Tenant Tenant `gorm:"foreignKey:TenantSchema;references:SchemaName" json:"-"`
}

// TableName returns the table name with public schema prefix
func (Subscription) TableName() string {
	return "public.subscriptions"
}

// IsSharedModel indicates this is a shared/public model
func (Subscription) IsSharedModel() bool {
	return true
}
