// `api.PostGoodsReceipt` — §8.4 with the HTTP peeled off, which is the function
// `cmd/seed` calls.
//
// WHY THIS FILE EXISTS. Phase 8's coverage pass put `PostGoodsReceipt` at 0.0%.
// Group D covers the *handler* thoroughly, and `cmd/seed` is the only caller of the
// exported variant — so the one seam §15 leans on ("the seed's receipts go through
// PostGoodsReceipt rather than inserting ledger and journal rows directly, so the
// seed cannot drift from the application") was itself never asserted. A signature
// change or a wrong argument order there would have been caught by `make seed`
// failing at the demo, not by the suite.
//
// It is also the only place the cross-module transaction is driven with no
// `*fiber.Ctx` anywhere, which is the property that makes the split worth having.
package api_test

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/DGosal/mini-erp/backend/internal/api"
	"github.com/DGosal/mini-erp/backend/internal/identity"
	"github.com/DGosal/mini-erp/backend/testsupport"
)

// resolveIdentity goes through `identity.Resolve` — the same function the
// middleware and the seed both use (I9). Building an `identity.Identity` by hand
// here would test this seam against a caller the application never produces.
func resolveIdentity(t *testing.T, h *testsupport.Harness, uid string) *identity.Identity {
	t.Helper()
	id, err := identity.Resolve(context.Background(), h.DB.App, uid)
	if err != nil {
		t.Fatalf("resolve %s: %v", uid, err)
	}
	if id == nil {
		t.Fatalf("resolve %s: no such user", uid)
	}
	return id
}

// oneLineOrder gives back an open purchase order with a single line for `qty` of
// the tenant's product, which is the shape the seed posts against.
func oneLineOrder(t *testing.T, f *testsupport.TenantFixture, qty float64) (uuid.UUID, uuid.UUID) {
	t.Helper()
	poID := f.NewPurchaseOrder(t)
	lineID := f.NewPOLine(t, poID, f.ProductID, qty)
	return poID, lineID
}

func TestPostGoodsReceiptWritesAllThreeModulesWithNoHTTPInvolved(t *testing.T) {
	h := testsupport.NewHarness(t)
	f := h.DB.NewTenant(t, "Seeded Receipts Ltd")
	receiver := f.NewUser(t, map[string]string{
		"procurement": "approver",
		"inventory":   "user",
		"finance":     "user",
	})
	caller := resolveIdentity(t, h, receiver.FirebaseUID)

	poID, lineID := oneLineOrder(t, f, 10)

	var summary api.GoodsReceiptSummary
	err := testsupport.WithTenantOn(h.DB.App, f.ID, func(tx *gorm.DB) error {
		var err error
		summary, err = api.PostGoodsReceipt(tx, caller, poID, "seam-key-1", "seeded",
			[]api.ReceiptLine{{POLineID: lineID, Qty: "10"}})
		return err
	})
	if err != nil {
		t.Fatalf("PostGoodsReceipt: %v", err)
	}

	// The summary is what the seed logs, and every field of it is a claim about a
	// different module.
	if summary.Replayed {
		t.Error("a first post came back replayed")
	}
	if summary.GRNumber == "" {
		t.Error("no GR number allocated")
	}
	if summary.EntryNumber == "" {
		t.Error("no journal entry number — finance was not written in the same breath")
	}
	if summary.LedgerEntries != 1 {
		t.Errorf("ledgerEntries = %d, want 1", summary.LedgerEntries)
	}
	if summary.OrderStatus != "received" {
		t.Errorf("orderStatus = %q, want received for a fully received order", summary.OrderStatus)
	}

	// And the rows themselves, because a summary is only a report of them. D8 is
	// the test that proves the ordering; this one proves the non-HTTP entry point
	// reaches the same three tables.
	f.Must(t, func(tx *gorm.DB) error {
		var counts struct {
			Ledger  int64
			Journal int64
			Lines   int64
		}
		return tx.Raw(`
			SELECT (SELECT count(*) FROM stock_ledger
			         WHERE source_type = 'goods_receipt')          AS ledger,
			       (SELECT count(*) FROM journal_entries
			         WHERE source_type = 'goods_receipt')          AS journal,
			       (SELECT count(*) FROM journal_entry_lines)      AS lines`).
			Scan(&counts).Error
	})
}

// The property that makes `make seed` idempotent, and the reason the seed's key is
// derived from the slug and the plan rather than generated: a second run reports
// the same numbers and writes nothing.
func TestPostGoodsReceiptReplaysOnTheSameKeyWithoutWritingTwice(t *testing.T) {
	h := testsupport.NewHarness(t)
	f := h.DB.NewTenant(t, "Reseeded Receipts Ltd")
	receiver := f.NewUser(t, map[string]string{
		"procurement": "approver", "inventory": "user", "finance": "user",
	})
	caller := resolveIdentity(t, h, receiver.FirebaseUID)
	poID, lineID := oneLineOrder(t, f, 8)

	post := func() api.GoodsReceiptSummary {
		t.Helper()
		var summary api.GoodsReceiptSummary
		err := testsupport.WithTenantOn(h.DB.App, f.ID, func(tx *gorm.DB) error {
			var err error
			summary, err = api.PostGoodsReceipt(tx, caller, poID, "seam-key-stable", "",
				[]api.ReceiptLine{{POLineID: lineID, Qty: "8"}})
			return err
		})
		if err != nil {
			t.Fatalf("PostGoodsReceipt: %v", err)
		}
		return summary
	}

	first := post()
	second := post()

	// `Replayed` is what makes the seed idempotent *visibly* rather than silently:
	// a second `make seed` reports the same GR numbers with Replayed true.
	if !second.Replayed {
		t.Error("the second post with the same key was not reported as a replay")
	}
	if second.GRNumber != first.GRNumber {
		t.Errorf("GR number changed on replay: %q then %q", first.GRNumber, second.GRNumber)
	}
	if second.EntryNumber != first.EntryNumber {
		t.Errorf("journal entry changed on replay: %q then %q", first.EntryNumber, second.EntryNumber)
	}

	// Nothing was written a second time — one receipt, one journal entry, one
	// ledger row, whatever the caller did.
	f.Must(t, func(tx *gorm.DB) error {
		var got struct {
			Receipts int64
			Ledger   int64
			Journals int64
		}
		if err := tx.Raw(`
			SELECT (SELECT count(*) FROM goods_receipts)   AS receipts,
			       (SELECT count(*) FROM stock_ledger)     AS ledger,
			       (SELECT count(*) FROM journal_entries)  AS journals`).Scan(&got).Error; err != nil {
			return err
		}
		if got.Receipts != 1 || got.Ledger != 1 || got.Journals != 1 {
			t.Errorf("after a replay: %+v, want one of each", got)
		}
		return nil
	})
}

// A refusal arrives as an ordinary error rather than as something to render. A
// seed meeting `over_receipt` has a bug in the seed, not a business outcome — so
// there is nothing to unwrap, and the message is what the operator reads.
func TestPostGoodsReceiptReturnsAPlainErrorOnOverReceipt(t *testing.T) {
	h := testsupport.NewHarness(t)
	f := h.DB.NewTenant(t, "Overreaching Seed Ltd")
	receiver := f.NewUser(t, map[string]string{
		"procurement": "approver", "inventory": "user", "finance": "user",
	})
	caller := resolveIdentity(t, h, receiver.FirebaseUID)
	poID, lineID := oneLineOrder(t, f, 5)

	err := testsupport.WithTenantOn(h.DB.App, f.ID, func(tx *gorm.DB) error {
		_, err := api.PostGoodsReceipt(tx, caller, poID, "seam-key-over", "",
			[]api.ReceiptLine{{POLineID: lineID, Qty: "6"}})
		return err
	})
	if err == nil {
		t.Fatal("receiving 6 against an outstanding 5 returned no error")
	}

	// And nothing landed. The whole point of one transaction is that a refused
	// receipt leaves no ledger row and no journal entry behind it.
	f.Must(t, func(tx *gorm.DB) error {
		var counts struct {
			Ledger   int64
			Journals int64
		}
		if err := tx.Raw(`
			SELECT (SELECT count(*) FROM stock_ledger)    AS ledger,
			       (SELECT count(*) FROM journal_entries) AS journals`).Scan(&counts).Error; err != nil {
			return err
		}
		if counts.Ledger != 0 || counts.Journals != 0 {
			t.Errorf("a refused receipt wrote rows: %+v", counts)
		}
		return nil
	})
}

func TestPostGoodsReceiptRefusesALineFromAnotherOrder(t *testing.T) {
	h := testsupport.NewHarness(t)
	f := h.DB.NewTenant(t, "Crossed Wires Ltd")
	receiver := f.NewUser(t, map[string]string{
		"procurement": "approver", "inventory": "user", "finance": "user",
	})
	caller := resolveIdentity(t, h, receiver.FirebaseUID)

	poID, _ := oneLineOrder(t, f, 5)
	_, otherLine := oneLineOrder(t, f, 5)

	err := testsupport.WithTenantOn(h.DB.App, f.ID, func(tx *gorm.DB) error {
		_, err := api.PostGoodsReceipt(tx, caller, poID, "seam-key-crossed", "",
			[]api.ReceiptLine{{POLineID: otherLine, Qty: "1"}})
		return err
	})
	if err == nil {
		t.Fatal("a line belonging to a different order was accepted")
	}
}
