// Group E — document numbering (§12.3, §8.1).
//
// These run against the real database rather than through HTTP, because three of
// the five claims are about transactions and concurrency: that twenty
// simultaneous allocations serialise, that a rollback gives the number back, and
// that the period comes from the tenant's timezone. None of those is visible in a
// response body.
package docnum_test

import (
	"fmt"
	"os"
	"sync"
	"testing"
	"time"
	_ "time/tzdata" // so LoadLocation works on a machine with no zoneinfo

	"gorm.io/gorm"

	"github.com/DGosal/mini-erp/backend/internal/docnum"
	"github.com/DGosal/mini-erp/backend/testsupport"
)

func TestMain(m *testing.M) {
	code := m.Run()
	testsupport.Shutdown()
	os.Exit(code)
}

// allocate runs one allocation in its own tenant-scoped transaction — the same
// shape a handler's does, since document_sequences is RLS-forced like every other
// tenant table (I1).
func allocate(t *testing.T, f *testsupport.TenantFixture, docType string) string {
	t.Helper()
	var number string
	f.Must(t, func(tx *gorm.DB) error {
		var err error
		number, err = docnum.Allocate(tx, f.ID, docType)
		return err
	})
	return number
}

// period is the YYYYMM a document created now belongs to, in the tenant's zone.
// Computed here from time.Now so the expectation does not come from the same
// expression it is checking.
func period(t *testing.T, timezone string) string {
	t.Helper()
	loc, err := time.LoadLocation(timezone)
	if err != nil {
		t.Fatalf("load %s: %v", timezone, err)
	}
	return time.Now().In(loc).Format("200601")
}

// E1 — the sequence increments per tenant independently.
//
// Two tenants, because one tenant cannot show independence: a global counter
// would pass a single-tenant test and hand tenant B the number `0003` on its
// first ever requisition.
func TestE1SequenceIsPerTenant(t *testing.T) {
	d := testsupport.NewTestDB(t)
	a := d.NewTenant(t, "Tenant A")
	b := d.NewTenant(t, "Tenant B")
	month := period(t, a.Timezone)

	first := allocate(t, a, docnum.PR)
	second := allocate(t, a, docnum.PR)
	other := allocate(t, b, docnum.PR)

	if want := fmt.Sprintf("PR-%s-0001", month); first != want {
		t.Errorf("tenant A's first number = %s, want %s", first, want)
	}
	if want := fmt.Sprintf("PR-%s-0002", month); second != want {
		t.Errorf("tenant A's second number = %s, want %s", second, want)
	}
	if want := fmt.Sprintf("PR-%s-0001", month); other != want {
		t.Errorf("tenant B's first number = %s, want %s — tenant B's counter is "+
			"following tenant A's", other, want)
	}

	// Document types are separate counters too: a PO is not "the next document".
	if got, want := allocate(t, a, docnum.PO), fmt.Sprintf("PO-%s-0001", month); got != want {
		t.Errorf("tenant A's first PO number = %s, want %s", got, want)
	}
}

// E2 — the sequence resets at a period boundary.
//
// The boundary is simulated by moving the existing counter row into last month,
// which is indistinguishable from a month having passed: the period is part of
// the primary key, so allocation in the new period has no row to increment.
//
// This is what a Postgres sequence could not do, and the reason §8.1 forbids one.
func TestE2SequenceResetsAtAPeriodBoundary(t *testing.T) {
	d := testsupport.NewTestDB(t)
	f := d.NewTenant(t, "Rollover Ltd")
	month := period(t, f.Timezone)

	for i := 1; i <= 3; i++ {
		allocate(t, f, docnum.PR)
	}

	f.Must(t, func(tx *gorm.DB) error {
		return tx.Exec(`
			UPDATE document_sequences
			SET period = to_char((now() AT TIME ZONE ?) - interval '1 month', 'YYYYMM')
			WHERE tenant_id = ? AND doc_type = 'PR'`, f.Timezone, f.ID).Error
	})

	got := allocate(t, f, docnum.PR)
	if want := fmt.Sprintf("PR-%s-0001", month); got != want {
		t.Errorf("first number of the new period = %s, want %s — the counter "+
			"carried over instead of resetting", got, want)
	}

	// Last month's counter is still there, unchanged. A reset is not a wipe:
	// re-opening last month must not renumber what is already in it.
	var rows int64
	f.Must(t, func(tx *gorm.DB) error {
		return tx.Raw(`
			SELECT count(*) FROM document_sequences
			WHERE tenant_id = ? AND doc_type = 'PR'`, f.ID).Scan(&rows).Error
	})
	if rows != 2 {
		t.Errorf("document_sequences rows for PR = %d, want 2 (last month and this)", rows)
	}
}

// E3 — twenty goroutines allocating simultaneously produce twenty distinct
// numbers with no gaps.
//
// This is the test the `ON CONFLICT … DO UPDATE` exists to pass. The DO UPDATE
// takes a row lock, so the twentieth allocation waits for the nineteenth instead
// of reading the same `last_number` as it did. Read-then-increment in Go would
// produce duplicates here, and duplicates would be caught much later by
// `purchase_requisitions_tenant_id_pr_number_key` — as a 500 on a user's screen.
func TestE3ConcurrentAllocationsAreDistinctAndGapless(t *testing.T) {
	d := testsupport.NewTestDB(t)
	f := d.NewTenant(t, "Busy Ltd")
	month := period(t, f.Timezone)

	const workers = 20
	// A private pool: the shared one is used by every other test in this process,
	// and twenty concurrent transactions on it would be twenty of its connections.
	pool := d.NewAppPool(t, workers)

	numbers := make([]string, workers)
	errs := make([]error, workers)

	var start sync.WaitGroup
	var done sync.WaitGroup
	start.Add(1)
	for i := 0; i < workers; i++ {
		done.Add(1)
		go func(i int) {
			defer done.Done()
			// Every goroutine waits for the same signal, so the allocations
			// genuinely overlap rather than queueing behind goroutine startup.
			start.Wait()
			errs[i] = testsupport.WithTenantOn(pool, f.ID, func(tx *gorm.DB) error {
				number, err := docnum.Allocate(tx, f.ID, docnum.GR)
				numbers[i] = number
				return err
			})
		}(i)
	}
	start.Done()
	done.Wait()

	seen := map[string]bool{}
	for i, err := range errs {
		if err != nil {
			t.Fatalf("worker %d: %v", i, err)
		}
		if seen[numbers[i]] {
			t.Fatalf("number %s was allocated twice", numbers[i])
		}
		seen[numbers[i]] = true
	}

	// No gaps: the twenty numbers are exactly 0001..0020, so nothing was
	// skipped by a failed attempt either.
	for n := 1; n <= workers; n++ {
		want := fmt.Sprintf("GR-%s-%04d", month, n)
		if !seen[want] {
			t.Errorf("%s was never allocated — the sequence has a gap", want)
		}
	}
}

// E4 — a rolled-back transaction does not consume a number.
//
// This is why allocation takes the caller's transaction rather than a pool: the
// counter row is written by the same transaction as the document, so if the
// document does not survive, neither does the number. A sequence would leave a
// permanent gap, and a gap in a document series is the kind of thing an auditor
// asks about.
func TestE4RollbackDoesNotConsumeANumber(t *testing.T) {
	d := testsupport.NewTestDB(t)
	f := d.NewTenant(t, "Rollback Ltd")
	month := period(t, f.Timezone)

	sentinel := fmt.Errorf("the document was refused after its number was allocated")
	var allocated string
	err := f.AsTenant(t, func(tx *gorm.DB) error {
		var err error
		allocated, err = docnum.Allocate(tx, f.ID, docnum.JE)
		if err != nil {
			return err
		}
		return sentinel
	})
	if err == nil {
		t.Fatal("the transaction committed; it was supposed to roll back")
	}
	if want := fmt.Sprintf("JE-%s-0001", month); allocated != want {
		t.Fatalf("allocated %s inside the doomed transaction, want %s", allocated, want)
	}

	// The next allocation gets the same number, because the first one never
	// happened.
	if got, want := allocate(t, f, docnum.JE), fmt.Sprintf("JE-%s-0001", month); got != want {
		t.Errorf("number after the rollback = %s, want %s — the rolled-back "+
			"transaction consumed it", got, want)
	}

	var rows int64
	f.Must(t, func(tx *gorm.DB) error {
		return tx.Raw(`
			SELECT count(*) FROM document_sequences
			WHERE tenant_id = ? AND doc_type = 'JE'`, f.ID).Scan(&rows).Error
	})
	if rows != 1 {
		t.Errorf("document_sequences rows for JE = %d, want 1", rows)
	}
}

// E5 — the period is computed in the TENANT's timezone, not the server's.
//
// Two tenants and one instant: 17:30 UTC on 31 July is 00:30 on 1 August in
// Jakarta. So the same click produces `202608` for the Jakarta tenant and
// `202607` for the UTC one, and a `to_char(now(), 'YYYYMM')` implementation
// gives both of them July.
//
// The instant is passed explicitly because no real timezone can move an ordinary
// mid-month afternoon into another month — a version of this test that used
// `now()` would pass whether the conversion were there or not, on all but two
// days a month.
func TestE5PeriodUsesTheTenantTimezone(t *testing.T) {
	d := testsupport.NewTestDB(t)

	jakarta := d.NewTenantInTZ(t, "Jakarta Ltd", "Asia/Jakarta")
	utc := d.NewTenantInTZ(t, "London Ltd", "UTC")

	// 2026-07-31 17:30 UTC — the §2.5.1 example, from the other side.
	instant := time.Date(2026, 7, 31, 17, 30, 0, 0, time.UTC)

	for _, tc := range []struct {
		name   string
		tenant *testsupport.TenantFixture
		want   string
	}{
		{"tenant in Asia/Jakarta is already in August", jakarta, "PR-202608-0001"},
		{"tenant in UTC is still in July", utc, "PR-202607-0001"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var got string
			tc.tenant.Must(t, func(tx *gorm.DB) error {
				var err error
				got, err = docnum.AllocateAt(tx, tc.tenant.ID, docnum.PR, &instant)
				return err
			})
			if got != tc.want {
				t.Errorf("number = %s, want %s", got, tc.want)
			}
		})
	}
}

// A tenant that does not exist gets an error rather than a plausible-looking
// number. Allocating under a guessed period would file a document in a month
// nobody chose, and the row would then be invisible to every month-end report.
func TestUnknownTenantIsAnError(t *testing.T) {
	d := testsupport.NewTestDB(t)
	f := d.NewTenant(t, "Real Ltd")

	f.Must(t, func(tx *gorm.DB) error {
		number, err := docnum.Allocate(tx, testsupport.NoSuchTenant, docnum.PR)
		if err == nil {
			t.Errorf("allocated %q for a tenant that does not exist", number)
		}
		return nil
	})
}
