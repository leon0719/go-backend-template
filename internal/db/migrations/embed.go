// Package migrations embeds the goose SQL migration files so they can be
// applied without shipping the .sql files alongside a deployed binary (e.g.
// in the distroless prod image, which has no filesystem access to source).
package migrations

import "embed"

// FS holds the embedded goose migration files.
//
//go:embed *.sql
var FS embed.FS
