// Package jobs wires the River durable job queue (ADR decision 17). Workers
// and periodic jobs arrive in Phase 7; Phase 1 starts the client with a
// single no-op placeholder so the operational path — migrate, start, drain
// on shutdown — is exercised from day one.
package jobs

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/riverqueue/river"
	"github.com/riverqueue/river/riverdriver/riverpgxv5"
)

// noopArgs is a placeholder job kind. River refuses to start with zero
// registered workers, so the bundle gets this single no-op until Phase 7 adds
// the real SendOfferEmail job. Nothing ever enqueues it.
type noopArgs struct{}

func (noopArgs) Kind() string { return "noop" }

type noopWorker struct {
	river.WorkerDefaults[noopArgs]
}

func (w *noopWorker) Work(_ context.Context, _ *river.Job[noopArgs]) error { return nil }

// NewClient builds a River client on the default queue, backed by the app's
// Postgres pool. River requires its schema to be migrated before Start
// (see "api migrate up").
func NewClient(pool *pgxpool.Pool) (*river.Client[pgx.Tx], error) {
	workers := river.NewWorkers()
	river.AddWorker(workers, &noopWorker{})

	client, err := river.NewClient(riverpgxv5.New(pool), &river.Config{
		Queues:  map[string]river.QueueConfig{river.QueueDefault: {MaxWorkers: 100}},
		Workers: workers,
	})
	if err != nil {
		return nil, fmt.Errorf("river client: %w", err)
	}
	return client, nil
}
