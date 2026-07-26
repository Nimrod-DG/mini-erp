// Command seed loads the demo database of §15.
//
// It is what makes the project reviewable cold: two workspaces with different
// entitlements and different timezones, seven people with different levels, sixty
// days of stock movements, and — the part that matters most — two purchase orders
// that have been received in full, so somebody opening the app for the first time
// sees a completed procurement → inventory → finance flow without having to
// perform one first.
//
// FOUR PROPERTIES, IN THE ORDER THEY MATTER.
//
//  1. IT IS IDEMPOTENT. Every row's UUID is derived from what the row is (see
//     ids.go), so re-running writes to the same rows rather than to new ones, and
//     the receipts carry deterministic idempotency keys so §8.6.1 recognises them
//     as replays. "Reseeding twice in a row produces the same database" is the
//     Phase 7 acceptance criterion, and the verification block at the bottom of
//     this file is how it is checked on every run.
//
//  2. THE RECEIPTS GO THROUGH THE APPLICATION'S OWN CODE. api.PostGoodsReceipt,
//     not hand-written ledger and journal rows (§15). The seed therefore cannot
//     drift from the endpoint, and if it ever produced an unbalanced journal the
//     `jel_balanced` trigger would refuse the commit.
//
//  3. IT RUNS AS erp_app, INSIDE db.WithTenant. The seed is held to the same
//     grants and the same RLS as a request (I1): it cannot update the stock
//     ledger, and it cannot write into the wrong tenant. A seed run as the schema
//     owner could produce a database the application could not have produced.
//
//  4. IT CHECKS ITSELF. §15 fixes the document counts and requires at least three
//     products below their reorder point once everything has run. Both are
//     verified at the end rather than assumed, because a quantity in one file and
//     a reorder point in another are exactly the pair that drifts.
//
// Demo accounts go into the dev Firebase project and nowhere else. There is no
// guard against pointing this at production beyond that sentence and the
// `seed-` UID prefix, which makes them identifiable and purgeable.
package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"time"

	"gorm.io/gorm"

	"github.com/DGosal/mini-erp/backend/internal/auth"
	"github.com/DGosal/mini-erp/backend/internal/config"
	"github.com/DGosal/mini-erp/backend/internal/db"
)

func main() {
	skipFirebase := flag.Bool("skip-firebase", false,
		"write the database only, creating no provider accounts. The seeded people "+
			"will then exist in the database and be unable to sign in.")
	flag.Parse()

	cfg, err := config.Load()
	if err != nil {
		log.Fatal(err)
	}

	ctx := context.Background()
	pools := db.NewPools(cfg.DatabaseURL, cfg.AdminDatabaseURL)
	defer func() { _ = pools.Close() }()

	admin, err := pools.Admin()
	if err != nil {
		log.Fatal(err)
	}
	app, err := pools.App()
	if err != nil {
		log.Fatal(err)
	}

	var fb provisioner = noFirebase{}
	if *skipFirebase {
		log.Print("seed: --skip-firebase — no provider accounts will be created, " +
			"and no seeded user will be able to sign in")
	} else {
		firebase, err := auth.NewFirebase(ctx, cfg.FirebaseProjectID)
		if err != nil {
			log.Fatalf("seed: %v\n\nIf you have no service-account key to hand, "+
				"`go run ./cmd/seed --skip-firebase` still loads the database.", err)
		}
		fb = firebase
	}

	// One instant for the whole run. Every date in the seed is an offset from
	// this, so a run that straddles midnight does not put half the history in
	// one day and half in the next.
	now := time.Now().UTC()

	if err := seedSuperadmin(ctx, admin, fb); err != nil {
		log.Fatalf("seed: superadmin: %v", err)
	}
	log.Printf("seed: superadmin %s", superadminEmail)

	for _, spec := range tenants {
		t, err := seedTenantPlatform(ctx, admin, fb, spec)
		if err != nil {
			log.Fatalf("seed: %v", err)
		}

		// One transaction for the whole of this tenant's business data. A seed
		// that half-succeeds leaves documents pointing at master data that is not
		// there, and the next person has to work out which half ran.
		if err := db.WithTenant(ctx, app, t.id, func(tx *gorm.DB) error {
			return seedTenantData(ctx, app, tx, t, now)
		}); err != nil {
			log.Fatalf("seed: %s: %v", spec.Slug, err)
		}

		if err := verifyTenant(ctx, app, t); err != nil {
			log.Fatalf("seed: %s: %v", spec.Slug, err)
		}
		log.Printf("seed: %s (%s, %s) — %d users, %d products, %d suppliers",
			spec.Name, spec.Slug, spec.Timezone,
			len(spec.Users), len(spec.Products), len(spec.Suppliers))
	}

	log.Printf("seed: done. Sign in as %s or any of the seeded addresses, "+
		"password %s", superadminEmail, seedPassword)
}

// --------------------------------------------------------------------------
// Verification.
//
// Not a test — the seed cannot import testsupport, and this has to run against
// the developer's own database rather than a container. It is a set of
// assertions §15 makes that nothing else would catch, checked on every run so a
// broken demo fails at `make seed` rather than in front of a reviewer.
// --------------------------------------------------------------------------

func verifyTenant(ctx context.Context, app *gorm.DB, t *seededTenant) error {
	return db.WithTenant(ctx, app, t.id, func(tx *gorm.DB) error {
		if err := verifyDocumentCounts(tx); err != nil {
			return err
		}
		if err := verifyLowStock(tx); err != nil {
			return err
		}
		return verifyCompletedFlow(tx)
	})
}

// verifyDocumentCounts checks the volumes §15 tabulates. The table in
// documents.go is easy to edit into disagreeing with the specification it
// transcribes, and nothing else in the project would notice.
func verifyDocumentCounts(tx *gorm.DB) error {
	wanted := []struct {
		table  string
		status string
		count  int
		what   string
	}{
		{"purchase_requisitions", "draft", 2, "draft requisitions"},
		{"purchase_requisitions", "submitted", 3, "submitted requisitions"},
		{"purchase_requisitions", "rejected", 1, "rejected requisitions"},
		{"purchase_requisitions", "cancelled", 1, "cancelled requisitions"},
		{"purchase_requisitions", "approved", 6, "approved requisitions"},
		{"purchase_orders", "open", 2, "open orders"},
		{"purchase_orders", "partially_received", 1, "partially received orders"},
		{"purchase_orders", "received", 2, "received orders"},
		{"purchase_orders", "cancelled", 1, "cancelled orders"},
	}
	for _, want := range wanted {
		var got int
		if err := tx.Raw(`SELECT count(*) FROM `+want.table+` WHERE status = ?`,
			want.status).Scan(&got).Error; err != nil {
			return err
		}
		if got != want.count {
			return fmt.Errorf("%d %s, want %d (§15's table and documents.go have drifted)",
				got, want.what, want.count)
		}
	}
	return nil
}

// verifyLowStock is §15's "at least 3 deliberately below reorder point after all
// seeding completes, so the low-stock widget is populated on first load".
//
// It runs the same expression the widget and the list do, which is the point:
// the reorder points live in data.go, the quantities that decide the outcome
// live partly there and partly in documents.go, and nothing but this asks
// PostgreSQL what the three files actually add up to.
func verifyLowStock(tx *gorm.DB) error {
	var low int
	if err := tx.Raw(`
		SELECT count(*)
		FROM products p
		LEFT JOIN (
		  SELECT product_id, SUM(qty_on_hand) AS qty_on_hand
		  FROM stock_balances GROUP BY product_id
		) b ON b.product_id = p.id
		WHERE p.deleted_at IS NULL
		  AND p.reorder_point > 0
		  AND COALESCE(b.qty_on_hand, 0) < p.reorder_point`).Scan(&low).Error; err != nil {
		return err
	}
	if low < 3 {
		return fmt.Errorf("%d products below their reorder point, want at least 3 — "+
			"the openings in data.go no longer clear the reorder points, or an "+
			"adjustment in documents.go rescued one", low)
	}
	return nil
}

// verifyCompletedFlow is the claim §15 says matters most: "a reviewer opening the
// app cold should immediately see a completed procurement → inventory → finance
// flow without performing one first."
//
// So it checks all three ends of it — the receipt, the stock it credited, and the
// balanced journal entry it posted — rather than just counting receipts. A
// receipt with no journal entry behind it would satisfy a count and fail the
// sentence.
func verifyCompletedFlow(tx *gorm.DB) error {
	var flows []struct {
		GRNumber      string
		LedgerEntries int
		EntryNumber   string
		Debits        string
		Credits       string
	}
	if err := tx.Raw(`
		SELECT gr.gr_number,
		       (SELECT count(*) FROM stock_ledger sl
		         WHERE sl.source_type = 'goods_receipt' AND sl.source_id = gr.id)
		         AS ledger_entries,
		       COALESCE(je.entry_number, '') AS entry_number,
		       COALESCE((SELECT SUM(debit)  FROM journal_entry_lines l
		                  WHERE l.journal_entry_id = je.id), 0)::text AS debits,
		       COALESCE((SELECT SUM(credit) FROM journal_entry_lines l
		                  WHERE l.journal_entry_id = je.id), 0)::text AS credits
		FROM goods_receipts gr
		LEFT JOIN journal_entries je
		  ON je.source_type = 'goods_receipt' AND je.source_id = gr.id
		ORDER BY gr.gr_number`).Scan(&flows).Error; err != nil {
		return err
	}
	if len(flows) == 0 {
		return fmt.Errorf("no goods receipts were posted, so there is no completed " +
			"procurement → inventory → finance flow for a reviewer to open")
	}
	for _, flow := range flows {
		if flow.LedgerEntries == 0 {
			return fmt.Errorf("%s credited no stock", flow.GRNumber)
		}
		if flow.EntryNumber == "" {
			return fmt.Errorf("%s posted no journal entry", flow.GRNumber)
		}
		// Compared as the text PostgreSQL produced. Parsing these into float64
		// to compare them would be deciding whether the books balance in a type
		// that cannot represent a tenth (I8) — and `jel_balanced` has already
		// refused anything else at commit, so this is the belt to that brace.
		if flow.Debits != flow.Credits {
			return fmt.Errorf("%s is unbalanced: debits %s, credits %s",
				flow.GRNumber, flow.Debits, flow.Credits)
		}
	}
	return nil
}
