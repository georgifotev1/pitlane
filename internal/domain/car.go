package domain

import "time"

type Car struct {
	ID         string
	TenantID   string
	CustomerID string
	Plate      string
	VIN        string
	Make       string
	Model      string
	Year       int
	Mileage    int
	ArchivedAt *time.Time
	CreatedAt  time.Time
	UpdatedAt  time.Time
}
