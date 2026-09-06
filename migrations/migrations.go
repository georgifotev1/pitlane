package migrations

import (
	"embed"
	"io/fs"
)

//go:embed all:sql
var FS embed.FS

func Sub() (fs.FS, error) {
	return fs.Sub(FS, "sql")
}
