package domain

import "time"

type Attachment struct {
	ID          string
	TenantID    string
	CarID       *string
	RepairID    *string
	Name        string
	StorageKey  string
	SizeBytes   int64
	ContentType string
	CreatedAt   time.Time
}
