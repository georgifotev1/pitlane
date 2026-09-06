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
	CreatedAt      time.Time
	UpdatedAt      time.Time
}
