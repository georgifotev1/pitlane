// Package migrations embeds the goose SQL migration files so the api binary
// carries its schema with it (ADR decision 7). Plain .sql files live in this
// directory; the migrate subcommand in cmd/api applies them.
package migrations

import (
	"embed"
	"io/fs"
)

// FS holds the goose SQL migrations under sql/. The all: prefix keeps the
// placeholder .gitkeep visible to embed so this package compiles before the
// first real migration lands (Phase 2 adds 0001).
//
//go:embed all:sql
var FS embed.FS

// Sub returns the sql subdirectory as an fs.FS for goose.
func Sub() (fs.FS, error) {
	return fs.Sub(FS, "sql")
}
