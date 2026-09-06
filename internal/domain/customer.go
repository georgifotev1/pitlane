package domain

import "time"

type Customer struct {
	ID         string
	TenantID   string
	Name       string
	Company    string
	Email      string
	Phone      string
	Address    string
	Notes      string
	ArchivedAt *time.Time
	CreatedAt  time.Time
	UpdatedAt  time.Time
}
