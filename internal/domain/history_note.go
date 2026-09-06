package domain

import "time"

type HistoryNote struct {
	ID          string
	TenantID    string
	CarID       string
	Title       string
	Description string
	RecordedAt  time.Time
	CreatedAt   time.Time
	UpdatedAt   time.Time
}
