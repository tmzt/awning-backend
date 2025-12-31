package domains

import (
	"context"
	"errors"
	"fmt"
)

// DomainRegistrar defines the interface for domain registrar implementations
type DomainRegistrar interface {
	// Name returns the registrar name
	Name() string

	// CheckAvailability checks if a domain is available for registration
	CheckAvailability(ctx context.Context, domain string) (*AvailabilityResult, error)

	// Register registers a domain
	Register(ctx context.Context, domain string, years int, contact *ContactInfo) (*RegistrationResult, error)

	// GetDNSRecords retrieves DNS records for a domain
	GetDNSRecords(ctx context.Context, domain string) ([]DNSRecord, error)

	// SetDNSRecords sets DNS records for a domain
	SetDNSRecords(ctx context.Context, domain string, records []DNSRecord) error

	// RenewDomain renews a domain registration
	RenewDomain(ctx context.Context, domain string, years int) (*RenewalResult, error)

	// GetDomainInfo retrieves information about a registered domain
	GetDomainInfo(ctx context.Context, domain string) (*DomainInfo, error)
}

// AvailabilityResult represents domain availability check result
type AvailabilityResult struct {
	Domain    string  `json:"domain"`
	Available bool    `json:"available"`
	Premium   bool    `json:"premium"`
	Price     float64 `json:"price,omitempty"`
	Currency  string  `json:"currency,omitempty"`
}

// RegistrationResult represents domain registration result
type RegistrationResult struct {
	Domain           string `json:"domain"`
	RegistrarID      string `json:"registrarId"`
	ExpiresAt        string `json:"expiresAt"`
	RegistrationDate string `json:"registrationDate"`
}

// RenewalResult represents domain renewal result
type RenewalResult struct {
	Domain       string `json:"domain"`
	NewExpiresAt string `json:"newExpiresAt"`
}

// DomainInfo represents information about a registered domain
type DomainInfo struct {
	Domain      string   `json:"domain"`
	RegistrarID string   `json:"registrarId"`
	CreatedAt   string   `json:"createdAt"`
	ExpiresAt   string   `json:"expiresAt"`
	Status      string   `json:"status"`
	NameServers []string `json:"nameServers"`
	AutoRenew   bool     `json:"autoRenew"`
}

// ContactInfo represents contact information for domain registration
type ContactInfo struct {
	FirstName    string `json:"firstName"`
	LastName     string `json:"lastName"`
	Organization string `json:"organization,omitempty"`
	Email        string `json:"email"`
	Phone        string `json:"phone"`
	Address1     string `json:"address1"`
	Address2     string `json:"address2,omitempty"`
	City         string `json:"city"`
	State        string `json:"state"`
	PostalCode   string `json:"postalCode"`
	Country      string `json:"country"` // ISO 3166-1 alpha-2
}

// DNSRecord represents a DNS record
type DNSRecord struct {
	Type     string `json:"type"` // A, AAAA, CNAME, MX, TXT, NS, etc.
	Name     string `json:"name"`
	Value    string `json:"value"`
	TTL      int    `json:"ttl"`
	Priority int    `json:"priority,omitempty"` // For MX records
}

// RegistrarConfig holds registrar configuration
type RegistrarConfig struct {
	Provider  string // namecheap, cloudflare, opensrs
	APIKey    string
	APISecret string
	Username  string
	Sandbox   bool
	BaseURL   string
}

// RegistrarFactory creates registrar instances
type RegistrarFactory struct {
	registrars map[string]func(*RegistrarConfig) (DomainRegistrar, error)
}

// NewRegistrarFactory creates a new registrar factory
func NewRegistrarFactory() *RegistrarFactory {
	factory := &RegistrarFactory{
		registrars: make(map[string]func(*RegistrarConfig) (DomainRegistrar, error)),
	}

	// Register built-in registrars
	factory.Register("namecheap", NewNamecheapRegistrar)
	factory.Register("cloudflare", NewCloudflareRegistrar)
	factory.Register("opensrs", NewOpenSRSRegistrar)
	factory.Register("mock", NewMockRegistrar)

	return factory
}

// Register adds a registrar creator function
func (f *RegistrarFactory) Register(name string, creator func(*RegistrarConfig) (DomainRegistrar, error)) {
	f.registrars[name] = creator
}

// Create creates a registrar instance
func (f *RegistrarFactory) Create(cfg *RegistrarConfig) (DomainRegistrar, error) {
	creator, ok := f.registrars[cfg.Provider]
	if !ok {
		return nil, fmt.Errorf("unknown registrar provider: %s", cfg.Provider)
	}
	return creator(cfg)
}

// GetAvailableProviders returns list of available providers
func (f *RegistrarFactory) GetAvailableProviders() []string {
	providers := make([]string, 0, len(f.registrars))
	for name := range f.registrars {
		providers = append(providers, name)
	}
	return providers
}

var ErrNotImplemented = errors.New("not implemented")
