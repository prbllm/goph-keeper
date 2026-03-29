package migrations

import "embed"

// ServerFS contains SQL migration files for the server schema.
//
//go:embed server/*.sql
var ServerFS embed.FS

// ClientFS contains SQL migration files for the client schema.
//
//go:embed client/*.sql
var ClientFS embed.FS
