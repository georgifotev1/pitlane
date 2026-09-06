package store

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
)

type documentType string

const (
	documentTypeOffer  documentType = "offer"
	documentTypeRepair documentType = "repair"
)

func nextDocumentNumber(ctx context.Context, tx pgx.Tx, tenantID string, kind documentType) (string, error) {
	prefix := ""
	switch kind {
	case documentTypeOffer:
		prefix = "OF"
	case documentTypeRepair:
		prefix = "RP"
	default:
		return "", fmt.Errorf("unknown document type %q", kind)
	}

	var year int
	var sequence int64
	if err := tx.QueryRow(ctx, `
		INSERT INTO document_counters (tenant_id, document_type, year, last_number)
		VALUES ($1, $2, EXTRACT(YEAR FROM CURRENT_DATE)::integer, 1)
		ON CONFLICT (tenant_id, document_type, year) DO UPDATE
		SET last_number = document_counters.last_number + 1
		RETURNING year, last_number
	`, tenantID, string(kind)).Scan(&year, &sequence); err != nil {
		return "", fmt.Errorf("allocate %s document number: %w", kind, err)
	}

	return fmt.Sprintf("%s-%04d-%06d", prefix, year, sequence), nil
}
