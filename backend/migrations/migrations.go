// Package migrations embeds the SQL files next to it, so both `cmd/migrate`
// and the test harness apply exactly the same schema without depending on the
// working directory.
package migrations

import "embed"

// FS holds every .sql file in this directory.
//
//go:embed *.sql
var FS embed.FS

// RolesFile is applied outside the versioned sequence: it runs from
// docker-entrypoint-initdb.d on the container's first boot, before any table
// exists, and again after the migrations so its platform-table grants land.
// It is idempotent by design. See the header comment in the file itself.
const RolesFile = "000_roles.sql"
