package domain

import "time"

type Tenant struct {
	ID             string
	Name           string
	Address        string
	VATNumber      string
	LogoKey        string
	Currency       string
	Locale         string
	DefaultTaxRate int32
	Settings       map[string]any
	// IsDemo marks the shared demonstration garage, which visitors may browse
	// but never change.
	IsDemo    bool
	CreatedAt time.Time
	UpdatedAt time.Time
}
