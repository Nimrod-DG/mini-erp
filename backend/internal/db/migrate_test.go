// The migration runner and the pool wiring, on the paths that are not the happy
// one.
//
// WHY. Phase 8's coverage pass put `internal/db` at 71.9% against §12.6's 90%, and
// the gap was almost entirely `Apply`, `ExecFile`, `applyOne` and the two lazy pool
// openers — every one of them exercised only by the harness taking the successful
// route through it on the way to testing something else.
//
// The two assertions here that are about behaviour rather than about a percentage:
//
//   - **A migration that fails is not recorded.** `applyOne` wraps each file in its
//     own transaction, so a syntax error must leave `schema_migrations` untouched.
//     Recorded-but-not-applied is the worst possible state for a migration runner:
//     every subsequent run skips the file, and the schema is permanently one step
//     behind what the table claims.
//   - **`Apply` is idempotent**, which is what makes `make migrate` safe to run
//     twice — and it reports what it did, so the second run reporting nothing is
//     the observable form of that.
package db_test

import (
	"context"
	"database/sql"
	"errors"
	"io/fs"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/jackc/pgx/v5/pgconn"

	"github.com/DGosal/mini-erp/backend/internal/db"
	"github.com/DGosal/mini-erp/backend/testsupport"
)

// A scratch schema per test.
//
// The package shares one migrated container, and `migrationsTable` is a constant —
// so running `Apply` with a MapFS against the public schema would put junk tables
// beside the fourteen real ones, where A5 and I8 both enumerate what they find.
// Isolation here is a schema, not a database, because starting a second container
// for four tests is the load that made this suite flaky in the first place.
type scratch struct {
	ctx    context.Context
	db     *sql.DB
	schema string
}

func openScratch(t *testing.T, schema string) *scratch {
	t.Helper()
	d := testsupport.NewTestDB(t)
	ctx := context.Background()

	// The schema has to exist before a handle whose search_path names it can
	// create anything in it, so it is made over an ordinary connection first.
	setup, err := db.OpenSQL(d.OwnerURL)
	if err != nil {
		t.Fatalf("OpenSQL: %v", err)
	}
	defer func() { _ = setup.Close() }()
	if _, err := setup.ExecContext(ctx, `DROP SCHEMA IF EXISTS `+schema+` CASCADE`); err != nil {
		t.Fatalf("drop schema %s: %v", schema, err)
	}
	if _, err := setup.ExecContext(ctx, `CREATE SCHEMA `+schema); err != nil {
		t.Fatalf("create schema %s: %v", schema, err)
	}

	sqlDB, err := db.OpenSQL(d.OwnerURL + "&search_path=" + schema)
	if err != nil {
		t.Fatalf("OpenSQL scratch: %v", err)
	}
	t.Cleanup(func() {
		_ = sqlDB.Close()
		cleanup, err := db.OpenSQL(d.OwnerURL)
		if err != nil {
			return
		}
		defer func() { _ = cleanup.Close() }()
		_, _ = cleanup.ExecContext(context.Background(),
			`DROP SCHEMA IF EXISTS `+schema+` CASCADE`)
	})

	return &scratch{ctx: ctx, db: sqlDB, schema: schema}
}

// exists asks whether an unqualified table name resolves, which is the scratch
// schema's own search_path answering.
func exists(t *testing.T, s *scratch, table string) bool {
	t.Helper()
	var name *string
	if err := s.db.QueryRowContext(s.ctx, `SELECT to_regclass($1)::text`, table).
		Scan(&name); err != nil {
		t.Fatalf("to_regclass(%s): %v", table, err)
	}
	return name != nil
}

func TestApplyRunsPendingMigrationsOnceAndReportsThem(t *testing.T) {
	s := openScratch(t, "mig_ok")

	// 000_* is skipped: 000_roles.sql is applied outside the versioned sequence,
	// before the tables it grants on exist. A file numbered 000 here proves the
	// skip rather than assuming it.
	fsys := fstest.MapFS{
		"000_roles.sql":  {Data: []byte(`CREATE TABLE should_not_exist (x int)`)},
		"001_first.sql":  {Data: []byte(`CREATE TABLE first (x int)`)},
		"002_second.sql": {Data: []byte(`CREATE TABLE second (x int)`)},
		"notes.md":       {Data: []byte(`not a migration`)},
	}

	applied, err := db.Apply(s.ctx, s.db, fsys)
	if err != nil {
		t.Fatalf("Apply: %v", err)
	}
	// In filename order, which is the only ordering guarantee migrations have.
	if len(applied) != 2 || applied[0] != "001_first.sql" || applied[1] != "002_second.sql" {
		t.Fatalf("applied = %v, want the two versioned files in order", applied)
	}

	var skipped int
	if err := s.db.QueryRowContext(s.ctx,
		`SELECT count(*) FROM information_schema.tables
		  WHERE table_schema = $1 AND table_name = 'should_not_exist'`,
		s.schema).Scan(&skipped); err != nil {
		t.Fatal(err)
	}
	if skipped != 0 {
		t.Error("a 000_ file was applied by the versioned sequence")
	}

	// The second run is the one that makes `make migrate` safe to repeat.
	again, err := db.Apply(s.ctx, s.db, fsys)
	if err != nil {
		t.Fatalf("second Apply: %v", err)
	}
	if len(again) != 0 {
		t.Errorf("second Apply reported %v, want nothing pending", again)
	}
}

// A migration that fails must not be recorded. Recorded-but-not-applied means
// every later run skips it and the schema is permanently behind the table.
func TestApplyLeavesAFailedMigrationUnrecorded(t *testing.T) {
	s := openScratch(t, "mig_fail")

	fsys := fstest.MapFS{
		"001_good.sql":  {Data: []byte(`CREATE TABLE good (x int)`)},
		"002_bad.sql":   {Data: []byte(`CREATE TABL typo (x int)`)},
		"003_never.sql": {Data: []byte(`CREATE TABLE never (x int)`)},
	}

	applied, err := db.Apply(s.ctx, s.db, fsys)
	if err == nil {
		t.Fatal("Apply returned no error for a migration with a syntax error")
	}
	if !strings.Contains(err.Error(), "002_bad.sql") {
		t.Errorf("error = %v, want the failing file named", err)
	}
	// It reports what it *did* manage, so the operator knows where they are.
	if len(applied) != 1 || applied[0] != "001_good.sql" {
		t.Errorf("applied = %v, want only the file that succeeded", applied)
	}

	// And it stops: 003 is not attempted after 002 failed, because migrations are
	// ordered for a reason.
	for table, want := range map[string]bool{"good": true, "typo": false, "never": false} {
		if exists(t, s, table) != want {
			t.Errorf("table %q exists = %v, want %v", table, !want, want)
		}
	}

	var recorded int
	if err := s.db.QueryRowContext(s.ctx,
		`SELECT count(*) FROM schema_migrations WHERE version = '002_bad.sql'`).Scan(&recorded); err != nil {
		t.Fatal(err)
	}
	if recorded != 0 {
		t.Error("a migration that failed was recorded as applied")
	}
}

func TestApplyFailsWhenTheDatabaseIsUnreachable(t *testing.T) {
	s := openScratch(t, "mig_closed")
	if err := s.db.Close(); err != nil {
		t.Fatal(err)
	}

	// The very first statement is CREATE TABLE IF NOT EXISTS schema_migrations, so
	// a closed handle is refused before anything is read.
	if _, err := db.Apply(s.ctx, s.db, fstest.MapFS{}); err == nil {
		t.Fatal("Apply on a closed database returned no error")
	} else if !strings.Contains(err.Error(), "schema_migrations") {
		t.Errorf("error = %v, want it to name what it was doing", err)
	}
}

// A `schema_migrations` that exists with the wrong shape. `CREATE TABLE IF NOT
// EXISTS` is satisfied by it, so the failure lands on the *read* — which is the one
// way `appliedVersions` fails in practice, and the reason its error names the table
// rather than just propagating a column error nobody can place.
func TestApplyReportsAnUnreadableMigrationsTable(t *testing.T) {
	s := openScratch(t, "mig_wrong_shape")

	if _, err := s.db.ExecContext(s.ctx,
		`CREATE TABLE schema_migrations (something_else int)`); err != nil {
		t.Fatal(err)
	}

	_, err := db.Apply(s.ctx, s.db, fstest.MapFS{
		"001_first.sql": {Data: []byte(`CREATE TABLE first (x int)`)},
	})
	if err == nil {
		t.Fatal("Apply returned no error against a schema_migrations with no version column")
	}
	if !strings.Contains(err.Error(), "schema_migrations") {
		t.Errorf("error = %v, want the table named", err)
	}
	if exists(t, s, "first") {
		t.Error("a migration was applied despite the ledger being unreadable")
	}
}

// listedButUnreadableFS reports a migration and then refuses to hand it over.
//
// Contrived, and the branch is worth having: `migrations.Files` is an embed.FS in
// production, where a listed name always reads. A future `os.DirFS` over a mounted
// volume is not, and the failure there — a file that vanishes between the glob and
// the read — has to name the file rather than surface as a bare io error.
type listedButUnreadableFS struct{ name string }

func (f listedButUnreadableFS) Open(name string) (fs.File, error) {
	if name == f.name {
		return nil, fs.ErrPermission
	}
	return nil, fs.ErrNotExist
}

func (f listedButUnreadableFS) Glob(string) ([]string, error) { return []string{f.name}, nil }

func TestApplyNamesAMigrationItCannotRead(t *testing.T) {
	s := openScratch(t, "mig_unreadable")

	applied, err := db.Apply(s.ctx, s.db, listedButUnreadableFS{name: "001_unreadable.sql"})
	if err == nil {
		t.Fatal("Apply returned no error for a migration it could not read")
	}
	if !strings.Contains(err.Error(), "001_unreadable.sql") {
		t.Errorf("error = %v, want the unreadable file named", err)
	}
	if len(applied) != 0 {
		t.Errorf("applied = %v, want nothing", applied)
	}
}

func TestExecFileRunsAFileEveryTimeAndNamesWhatItCannotDo(t *testing.T) {
	s := openScratch(t, "exec_file")

	// 000_roles.sql is applied this way because it has to run once before the
	// tables exist and again afterwards, so it is written to be idempotent — and
	// ExecFile has no ledger of its own.
	fsys := fstest.MapFS{
		"roles.sql":  {Data: []byte(`CREATE TABLE IF NOT EXISTS roles_marker (x int)`)},
		"broken.sql": {Data: []byte(`GRANT nonsense TO nobody`)},
	}

	for range 2 {
		if err := db.ExecFile(s.ctx, s.db, fsys, "roles.sql"); err != nil {
			t.Fatalf("ExecFile: %v", err)
		}
	}
	if !exists(t, s, "roles_marker") {
		t.Error("ExecFile did not run the file")
	}

	// A file that is not there, and a file whose SQL is not valid, are different
	// mistakes and both name the file.
	missing := db.ExecFile(s.ctx, s.db, fsys, "absent.sql")
	if missing == nil || !strings.Contains(missing.Error(), "absent.sql") {
		t.Errorf("ExecFile on a missing file: %v", missing)
	}
	broken := db.ExecFile(s.ctx, s.db, fsys, "broken.sql")
	if broken == nil || !strings.Contains(broken.Error(), "broken.sql") {
		t.Errorf("ExecFile on invalid SQL: %v", broken)
	}
}

// --------------------------------------------------------------------------
// Pools. Both are opened lazily, which is a Cloud Run decision: a cold start
// that dials the database before it has a request to serve pays the latency for
// nothing, and a database that is briefly unreachable should not stop the
// container from booting and answering /api/health.
// --------------------------------------------------------------------------

func TestPoolsOpenLazilyAndCacheTheirError(t *testing.T) {
	pools := db.NewPools("postgres://nobody@127.0.0.1:1/nowhere", "postgres://nobody@127.0.0.1:1/nowhere")
	t.Cleanup(func() { _ = pools.Close() })

	first, err := pools.App()
	if err == nil {
		t.Fatal("App() on an unreachable database returned no error")
	}
	if first != nil {
		t.Error("App() returned a handle alongside its error")
	}
	// The error names which pool, because "connection refused" on its own does
	// not say whether the app or the admin URL is wrong.
	if !strings.Contains(err.Error(), "app pool") {
		t.Errorf("error = %v, want the app pool named", err)
	}

	// sync.Once: the second call returns the cached failure rather than dialling
	// again on every request.
	if _, second := pools.App(); second == nil || second.Error() != err.Error() {
		t.Errorf("second App() = %v, want the same cached error", second)
	}

	if _, err := pools.Admin(); err == nil {
		t.Fatal("Admin() on an unreachable database returned no error")
	} else if !strings.Contains(err.Error(), "admin pool") {
		t.Errorf("error = %v, want the admin pool named", err)
	}
}

// Close releases whichever pools were actually opened — which, for a container
// that never served a request, is none of them.
func TestCloseOnUnopenedPoolsIsNotAnError(t *testing.T) {
	if err := db.NewPools("ignored", "ignored").Close(); err != nil {
		t.Errorf("Close on unopened pools = %v, want nil", err)
	}
}

func TestPoolsOpenAndCloseARealDatabase(t *testing.T) {
	d := testsupport.NewTestDB(t)
	pools := db.NewPools(d.AppURL, d.AdminURL)

	app, err := pools.App()
	if err != nil {
		t.Fatalf("App: %v", err)
	}
	admin, err := pools.Admin()
	if err != nil {
		t.Fatalf("Admin: %v", err)
	}
	// Two different roles, which is the whole reason there are two pools: RLS
	// applies to one and the other cannot reach the tenant tables at all.
	var appRole, adminRole string
	if err := app.Raw(`SELECT current_user`).Scan(&appRole).Error; err != nil {
		t.Fatal(err)
	}
	if err := admin.Raw(`SELECT current_user`).Scan(&adminRole).Error; err != nil {
		t.Fatal(err)
	}
	if appRole == adminRole {
		t.Errorf("both pools connected as %q", appRole)
	}

	if err := pools.Close(); err != nil {
		t.Errorf("Close: %v", err)
	}
}

// --------------------------------------------------------------------------
// pgerr. The mapping from a constraint to a business outcome.
// --------------------------------------------------------------------------

func TestSQLStateAndConstraintNameIgnoreNonPostgresErrors(t *testing.T) {
	// The guard that keeps a `nil` or a plain error from being read as a
	// constraint violation — which would turn a programming mistake into a 409.
	for name, err := range map[string]error{
		"nil":   nil,
		"plain": errors.New("something else went wrong"),
	} {
		if got := db.SQLState(err); got != "" {
			t.Errorf("SQLState(%s) = %q, want empty", name, got)
		}
		if got := db.ConstraintName(err); got != "" {
			t.Errorf("ConstraintName(%s) = %q, want empty", name, got)
		}
		if db.IsUniqueViolation(err) {
			t.Errorf("IsUniqueViolation(%s) = true", name)
		}
	}
}

func TestConstraintNameSurvivesWrapping(t *testing.T) {
	// It has to work through a wrap, because that is how it arrives: GORM wraps,
	// and the handler wraps again on the way out. One 23505 becoming two
	// different messages depends entirely on this.
	inner := &pgconn.PgError{Code: "23505", ConstraintName: "tenants_slug_key"}
	wrapped := errors.Join(errors.New("gorm: create"), inner)

	if got := db.SQLState(wrapped); got != db.SQLStateUniqueViolation {
		t.Errorf("SQLState = %q, want %q", got, db.SQLStateUniqueViolation)
	}
	if got := db.ConstraintName(wrapped); got != "tenants_slug_key" {
		t.Errorf("ConstraintName = %q, want tenants_slug_key", got)
	}
	if !db.IsUniqueViolation(wrapped) {
		t.Error("IsUniqueViolation = false for a wrapped 23505")
	}
}
