package migrations

import "embed"

// ServerFS contains SQL migration files for the server schema.
//
//go:embed server/*.sql
var ServerFS embed.FS
