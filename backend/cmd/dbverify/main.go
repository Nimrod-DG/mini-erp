// Command dbverify asserts the invariants that live in the database against a
// database you cannot start a container for — a deployed one.
//
// Phase 9 makes two checks a condition of being done: test A10 (no application
// role holds BYPASSRLS or SUPERUSER) and test J1 (every role's session
// timezone is UTC) run against the production database. Both tests exist in
// the Go suite, and both run against a testcontainer, which proves nothing
// about the database the deployed service actually talks to. A role
// provisioned by hand through a managed provider's console is exactly the
// failure they exist to catch, and it can only be caught here.
//
// It reads the same environment as the API, and takes an optional env file so
// that checking a deployed database does not mean exporting variables by hand:
//
//	cd backend && go run ./cmd/dbverify                  # backend/.env
//	cd backend && go run ./cmd/dbverify .env.production  # the deployed one
//
// DATABASE_URL and ADMIN_DATABASE_URL are required; MIGRATE_DATABASE_URL is
// optional and adds the owner's own checks when present. Exit status is 0 only
// if every check passed — a warning is not a failure, and there is exactly one
// of those.
package main

import (
	"database/sql"
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/DGosal/mini-erp/backend/internal/config"
	"github.com/DGosal/mini-erp/backend/internal/db"
)

// The fourteen tenant-scoped tables, as 005_rls_grants.up.sql lists them. A
// second copy of a canonical list is a liability, so this one is checked
// against the database rather than trusted: rlsForcedOnEveryTenantTable also
// fails if a table here has vanished.
var tenantTables = []string{
	"warehouses", "products", "stock_ledger",
	"suppliers", "purchase_requisitions", "purchase_requisition_lines",
	"purchase_orders", "purchase_order_lines",
	"goods_receipts", "goods_receipt_lines",
	"accounts", "journal_entries", "journal_entry_lines",
	"document_sequences",
}

type result struct {
	name   string
	detail string
	err    error
	// warn downgrades err from a failure to a remark. Exactly one check uses
	// it, and the reason is in ownerIsNotElevated.
	warn bool
}

func main() {
	cfg, err := config.LoadCLI()
	if err != nil {
		fmt.Fprintf(os.Stderr, "dbverify: %v\n", err)
		os.Exit(2)
	}

	app, err := db.OpenSQL(cfg.DatabaseURL)
	if err != nil {
		fmt.Fprintf(os.Stderr, "dbverify: open DATABASE_URL: %v\n", err)
		os.Exit(2)
	}
	defer func() { _ = app.Close() }()

	admin, err := db.OpenSQL(cfg.AdminDatabaseURL)
	if err != nil {
		fmt.Fprintf(os.Stderr, "dbverify: open ADMIN_DATABASE_URL: %v\n", err)
		os.Exit(2)
	}
	defer func() { _ = admin.Close() }()

	// The owner's connection string is not attached to the API service (see
	// config.RequireMigrateURL), so it is checked only when whoever runs this
	// has it to hand.
	var (
		owner      *sql.DB
		ownerCheck []result
	)
	if cfg.MigrateDatabaseURL == "" {
		fmt.Println("note: MIGRATE_DATABASE_URL unset — the owner's timezone (J1) was not checked")
	} else if owner, err = db.OpenSQL(cfg.MigrateDatabaseURL); err != nil {
		ownerCheck = append(ownerCheck, result{name: "erp_migrate connects", err: err})
		owner = nil
	} else {
		defer func() { _ = owner.Close() }()
		ownerCheck = append(ownerCheck, j1SessionTimezoneIsUTC("J1  erp_migrate session timezone", owner))
	}

	results := append([]result{
		connects("erp_app connects", app),
		connects("erp_admin connects", admin),
		a10NoElevatedRoles(app),
		ownerIsNotElevated(app),
		j1SessionTimezoneIsUTC("J1  erp_app session timezone", app),
		j1SessionTimezoneIsUTC("J1  erp_admin session timezone", admin),
		i4ViewsAreSecurityInvoker(app),
		rlsForcedOnEveryTenantTable(app),
		noTenantContextMeansNoRows(app),
		schemaIsUpToDate(app, owner),
	}, ownerCheck...)

	failed := 0
	for _, r := range results {
		switch {
		case r.err != nil && r.warn:
			fmt.Printf("warn  %-44s %v\n", r.name, r.err)
		case r.err != nil:
			failed++
			fmt.Printf("FAIL  %-44s %v\n", r.name, r.err)
		default:
			fmt.Printf("ok    %-44s %s\n", r.name, r.detail)
		}
	}

	fmt.Println()
	if failed > 0 {
		fmt.Printf("%d of %d checks failed\n", failed, len(results))
		os.Exit(1)
	}
	fmt.Printf("all %d checks passed\n", len(results))
}

func connects(name string, conn *sql.DB) result {
	var who, database string
	if err := conn.QueryRow(`SELECT current_user, current_database()`).Scan(&who, &database); err != nil {
		return result{name: name, err: err}
	}
	return result{name: name, detail: fmt.Sprintf("%s@%s", who, database)}
}

// A10 — neither application role may hold BYPASSRLS or SUPERUSER (I3).
func a10NoElevatedRoles(conn *sql.DB) result {
	const name = "A10 erp_app/erp_admin not elevated"

	rows, err := conn.Query(`
		SELECT rolname, rolsuper, rolbypassrls
		FROM pg_roles
		WHERE rolname IN ('erp_app', 'erp_admin')
		ORDER BY rolname`)
	if err != nil {
		return result{name: name, err: err}
	}
	defer func() { _ = rows.Close() }()

	var (
		seen     []string
		elevated []string
	)
	for rows.Next() {
		var (
			role                string
			isSuper, isBypassed bool
		)
		if err := rows.Scan(&role, &isSuper, &isBypassed); err != nil {
			return result{name: name, err: err}
		}
		seen = append(seen, role)
		if isSuper || isBypassed {
			elevated = append(elevated, fmt.Sprintf("%s(super=%t bypassrls=%t)", role, isSuper, isBypassed))
		}
	}
	if err := rows.Err(); err != nil {
		return result{name: name, err: err}
	}

	if len(elevated) > 0 {
		return result{name: name, err: fmt.Errorf(
			"%s — every RLS policy in this schema is decorative; the role was almost certainly created "+
				"through the provider's console instead of deploy/neon-bootstrap.sql",
			strings.Join(elevated, ", "))}
	}
	if len(seen) < 2 {
		return result{name: name, err: fmt.Errorf("expected erp_app and erp_admin, found %v", seen)}
	}
	return result{name: name, detail: strings.Join(seen, ", ")}
}

// The schema owner, reported rather than asserted — and this is the one place
// I3's wording has to be pinned down, which Phase 8 wrote down and left open.
//
// Locally erp_migrate IS the container's superuser: docker-compose.yml boots
// Postgres as it, and nothing else could create the schema. Failing on that
// would fail every local run, and a check that always fails is a check nobody
// reads. On a managed host it is an ordinary role created by
// deploy/neon-bootstrap.sql, and 005's FORCE ROW LEVEL SECURITY is precisely
// what lets the owner be unprivileged — so there is no excuse for it to be
// elevated in a deployment.
//
// I3 therefore means: erp_app and erp_admin, never, asserted; erp_migrate,
// wherever the host allows it, reported. A warning here on a deployed database
// is a finding, not noise.
func ownerIsNotElevated(conn *sql.DB) result {
	const name = "erp_migrate not elevated"

	var isSuper, isBypassed bool
	err := conn.QueryRow(`
		SELECT rolsuper, rolbypassrls FROM pg_roles WHERE rolname = 'erp_migrate'`).Scan(&isSuper, &isBypassed)
	if errors.Is(err, sql.ErrNoRows) {
		return result{name: name, detail: "role does not exist on this host"}
	}
	if err != nil {
		return result{name: name, err: err}
	}
	if isSuper || isBypassed {
		return result{name: name, warn: true, err: fmt.Errorf(
			"super=%t bypassrls=%t — expected locally (it is the container's superuser); "+
				"on a deployed database it means the owner was not created by deploy/neon-bootstrap.sql",
			isSuper, isBypassed)}
	}
	return result{name: name, detail: "super=false bypassrls=false"}
}

// J1 — the session timezone is UTC, per role. Pinned by ALTER ROLE … SET
// timezone, which is the half that survives a host whose containers you do not
// control.
func j1SessionTimezoneIsUTC(name string, conn *sql.DB) result {
	var tz string
	if err := conn.QueryRow(`SHOW timezone`).Scan(&tz); err != nil {
		return result{name: name, err: err}
	}
	if tz != "UTC" {
		return result{name: name, err: fmt.Errorf("timezone is %q, want UTC — run the ALTER ROLE … SET timezone lines from deploy/neon-bootstrap.sql", tz)}
	}
	return result{name: name, detail: tz}
}

// I4 — both views are security_invoker. Without it a view runs with its
// owner's privileges and returns every tenant's rows to everybody.
func i4ViewsAreSecurityInvoker(conn *sql.DB) result {
	const name = "I4 views are security_invoker"

	for _, view := range []string{"stock_balances", "po_line_status"} {
		var invoker sql.NullBool
		err := conn.QueryRow(`
			SELECT c.reloptions @> ARRAY['security_invoker=true']
			FROM pg_class c
			JOIN pg_namespace n ON n.oid = c.relnamespace
			WHERE n.nspname = 'public' AND c.relname = $1 AND c.relkind = 'v'`, view).Scan(&invoker)
		if errors.Is(err, sql.ErrNoRows) {
			return result{name: name, err: fmt.Errorf("view %s does not exist", view)}
		}
		if err != nil {
			return result{name: name, err: err}
		}
		if !invoker.Valid || !invoker.Bool {
			return result{name: name, err: fmt.Errorf("%s is not security_invoker — it leaks every tenant", view)}
		}
	}
	return result{name: name, detail: "stock_balances, po_line_status"}
}

// Every tenant table has RLS enabled AND forced, and carries the isolation
// policy. ENABLE without FORCE lets the owner read everything, which is the
// failure that looks fine in development.
func rlsForcedOnEveryTenantTable(conn *sql.DB) result {
	const name = "RLS enabled+forced on 14 tenant tables"

	for _, table := range tenantTables {
		var enabled, forced bool
		err := conn.QueryRow(`
			SELECT c.relrowsecurity, c.relforcerowsecurity
			FROM pg_class c
			JOIN pg_namespace n ON n.oid = c.relnamespace
			WHERE n.nspname = 'public' AND c.relname = $1`, table).Scan(&enabled, &forced)
		if errors.Is(err, sql.ErrNoRows) {
			return result{name: name, err: fmt.Errorf("table %s does not exist", table)}
		}
		if err != nil {
			return result{name: name, err: err}
		}
		if !enabled || !forced {
			return result{name: name, err: fmt.Errorf("%s: enabled=%t forced=%t, want both true", table, enabled, forced)}
		}

		var policies int
		if err := conn.QueryRow(`
			SELECT count(*) FROM pg_policies
			WHERE schemaname = 'public' AND tablename = $1 AND policyname = 'tenant_isolation'`,
			table).Scan(&policies); err != nil {
			return result{name: name, err: err}
		}
		if policies != 1 {
			return result{name: name, err: fmt.Errorf("%s has %d tenant_isolation policies, want 1", table, policies)}
		}
	}
	return result{name: name, detail: fmt.Sprintf("%d tables", len(tenantTables))}
}

// The end-to-end version of A10: a query with no app.current_tenant set must
// return nothing. If BYPASSRLS were on anywhere, this is where it shows up as
// data rather than as a catalogue flag.
//
// On an empty database this proves less than it looks — zero rows is also what
// an empty table returns. So when the owner's connection is available the row
// is counted again without RLS in the way, and the check reports "0 of N"
// rather than a pass it did not earn. erp_app cannot answer that question about
// itself, by design.
func noTenantContextMeansNoRows(conn *sql.DB) result {
	const name = "no tenant context returns no rows"

	var visible int
	if err := conn.QueryRow(`SELECT count(*) FROM products`).Scan(&visible); err != nil {
		return result{name: name, err: err}
	}
	if visible != 0 {
		return result{name: name, err: fmt.Errorf(
			"%d products are visible without app.current_tenant — tenant isolation is not applying", visible)}
	}

	// Now the same query with the context set, which is the half that tells
	// "isolation works" apart from "there is nothing here".
	//
	// Asked on this same erp_app connection rather than the owner's: FORCE ROW
	// LEVEL SECURITY constrains the owner too, so on a managed host — where
	// erp_migrate is not a superuser — it would answer 0 as well and the check
	// would report an empty database that is not empty.
	var tenantID string
	switch err := conn.QueryRow(`SELECT id::text FROM tenants LIMIT 1`).Scan(&tenantID); {
	case errors.Is(err, sql.ErrNoRows):
		return result{name: name, detail: "0 rows visible, but no tenants exist yet — seed to make this conclusive"}
	case err != nil:
		return result{name: name, err: err}
	}

	tx, err := conn.Begin()
	if err != nil {
		return result{name: name, err: err}
	}
	defer func() { _ = tx.Rollback() }()

	// set_config(..., true) is SET LOCAL: it dies with the transaction, which
	// is the whole of I2. A plain SET here would leak this tenant onto a
	// pooled connection.
	if _, err := tx.Exec(`SELECT set_config('app.current_tenant', $1, true)`, tenantID); err != nil {
		return result{name: name, err: err}
	}
	var scoped int
	if err := tx.QueryRow(`SELECT count(*) FROM products`).Scan(&scoped); err != nil {
		return result{name: name, err: err}
	}
	if scoped == 0 {
		return result{name: name, detail: "0 rows visible, and 0 with the context set — seed to make this conclusive"}
	}
	return result{name: name, detail: fmt.Sprintf("0 without tenant context, %d with it", scoped)}
}

// The migrations have run at all. Deliberately asked of the catalogue rather
// than of the table: schema_migrations is the owner's, and erp_app holds no
// grant on it — by design, since a request has no business reading it. The row
// count is reported only when the owner's connection is to hand.
func schemaIsUpToDate(conn *sql.DB, owner *sql.DB) result {
	const name = "migrations have been applied"

	var exists bool
	if err := conn.QueryRow(`SELECT to_regclass('public.schema_migrations') IS NOT NULL`).Scan(&exists); err != nil {
		return result{name: name, err: err}
	}
	if !exists {
		return result{name: name, err: errors.New("no schema_migrations table — run `go run ./cmd/migrate` against this database first")}
	}

	if owner == nil {
		return result{name: name, detail: "schema_migrations exists (set MIGRATE_DATABASE_URL to count them)"}
	}

	var applied int
	if err := owner.QueryRow(`SELECT count(*) FROM schema_migrations`).Scan(&applied); err != nil {
		return result{name: name, err: err}
	}
	if applied == 0 {
		return result{name: name, err: errors.New("no migrations recorded — run `go run ./cmd/migrate` first")}
	}
	return result{name: name, detail: fmt.Sprintf("%d applied", applied)}
}
