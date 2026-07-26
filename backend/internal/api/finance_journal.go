// [FINANCE] — the finance module's half of the goods receipt (§8.4 step 6).
//
// The finance module is a stub in the MVP: no screens, no reporting, no period
// close. What it does have is a chart of accounts and a double-entry journal
// that other modules post into — which is the part worth demonstrating, because
// it is the part that has to be atomic with everything else.
//
// One entry, two lines, for the value of what arrived:
//
//	Dr 1300 Inventory                     — the goods are an asset now
//	Cr 2150 Goods received not invoiced   — and owed for, though no invoice exists
//
// THE BALANCE IS ASSERTED HERE AND ENFORCED AT COMMIT. `jel_balanced` is a
// DEFERRABLE INITIALLY DEFERRED constraint trigger, so it fires once at COMMIT
// and aborts the whole transaction — the right outcome, since an unbalanced
// posting means the goods receipt behind it should not stand either. But a
// database error at commit time is hard to attribute to a request, so the
// assertion below fails fast and names the numbers. Belt and braces, with the
// belt doing the user-facing work.
package api

import (
	"fmt"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/DGosal/mini-erp/backend/internal/docnum"
	"github.com/DGosal/mini-erp/backend/internal/httpx"
	"github.com/DGosal/mini-erp/backend/internal/identity"
)

// postReceiptJournal posts the goods receipt's journal entry on the caller's
// transaction and returns what the confirmation panel needs to name it.
func postReceiptJournal(tx *gorm.DB, caller *identity.Identity, grID uuid.UUID, grNumber string) (*receiptFinanceResult, error) {
	// The value of what arrived, summed where both operands are still NUMERIC
	// (I8) and read from the rows that were just written rather than from the
	// request — so the posting cannot value something the receipt does not say
	// arrived.
	var amounts []struct {
		Total httpx.Numeric
	}
	if err := tx.Raw(`
		SELECT COALESCE(SUM(grl.qty_received * pol.unit_cost), 0)::numeric(18,2)::text AS total
		FROM goods_receipt_lines grl
		JOIN purchase_order_lines pol ON pol.id = grl.po_line_id
		WHERE grl.gr_id = ?`, grID).Scan(&amounts).Error; err != nil {
		return nil, err
	}
	if len(amounts) == 0 {
		return nil, fmt.Errorf("finance: goods receipt %s has no lines to value", grID)
	}
	total := amounts[0].Total

	debit, err := accountByCode(tx, accountInventory)
	if err != nil {
		return nil, err
	}
	credit, err := accountByCode(tx, accountGRNI)
	if err != nil {
		return nil, err
	}

	number, err := docnum.Allocate(tx, caller.TenantID, docnum.JE)
	if err != nil {
		return nil, err
	}

	entryID := uuid.New()
	if err := tx.Exec(`
		INSERT INTO journal_entries
		  (id, tenant_id, entry_number, source_type, source_id, description, created_by)
		VALUES (?, ?, ?, 'goods_receipt', ?, ?, ?)`,
		entryID, caller.TenantID, number, grID,
		fmt.Sprintf("Goods receipt %s", grNumber), caller.UserID).Error; err != nil {
		return nil, err
	}

	if err := tx.Exec(`
		INSERT INTO journal_entry_lines
		  (id, tenant_id, journal_entry_id, account_id, debit, credit, memo)
		VALUES (gen_random_uuid(), ?, ?, ?, ?, 0, ?),
		       (gen_random_uuid(), ?, ?, ?, 0, ?, ?)`,
		caller.TenantID, entryID, debit.ID, total, fmt.Sprintf("Stock received on %s", grNumber),
		caller.TenantID, entryID, credit.ID, total, fmt.Sprintf("Not yet invoiced — %s", grNumber),
	).Error; err != nil {
		return nil, err
	}

	if err := assertJournalBalances(tx, entryID); err != nil {
		return nil, err
	}

	return &receiptFinanceResult{
		JournalEntryID:    entryID,
		EntryNumber:       number,
		Amount:            total,
		DebitAccountID:    debit.ID,
		DebitAccountCode:  debit.Code,
		DebitAccountName:  debit.Name,
		CreditAccountID:   credit.ID,
		CreditAccountCode: credit.Code,
		CreditAccountName: credit.Name,
	}, nil
}

// receiptJournalFor reads back the entry a goods receipt posted, or nil if it
// has none. This is what makes an idempotent replay report the *first* call's
// journal entry rather than a fresh one.
func receiptJournalFor(tx *gorm.DB, grID uuid.UUID) (*receiptFinanceResult, error) {
	// Two joins onto the same table rather than an aggregate: this entry has
	// exactly one debit line and exactly one credit line, and saying so in the
	// query is clearer than summing over a shape it does not have.
	var rows []receiptFinanceResult
	if err := tx.Raw(`
		SELECT je.id AS journal_entry_id, je.entry_number,
		       dr.debit::text AS amount,
		       dra.id AS debit_account_id, dra.code AS debit_account_code,
		       dra.name AS debit_account_name,
		       cra.id AS credit_account_id, cra.code AS credit_account_code,
		       cra.name AS credit_account_name
		FROM journal_entries je
		JOIN journal_entry_lines dr  ON dr.journal_entry_id = je.id AND dr.debit  > 0
		JOIN accounts            dra ON dra.id = dr.account_id
		JOIN journal_entry_lines cr  ON cr.journal_entry_id = je.id AND cr.credit > 0
		JOIN accounts            cra ON cra.id = cr.account_id
		WHERE je.source_type = 'goods_receipt' AND je.source_id = ?`, grID).
		Scan(&rows).Error; err != nil {
		return nil, err
	}
	if len(rows) == 0 {
		return nil, nil
	}
	return &rows[0], nil
}

// account is one row of the chart of accounts, resolved by its code.
type account struct {
	ID   uuid.UUID
	Code string
	Name string
}

// accountByCode resolves one of the two accounts a receipt posts against.
//
// A missing account is an internal error rather than a business refusal: the
// chart of accounts is seeded when the workspace is created (§4.2.1) and nothing
// in the MVP lets anybody remove one. If it is gone, something has been done to
// the database by hand, and the honest answer is a 500 with a log line rather
// than a 422 telling a warehouse clerk to go and create an account.
func accountByCode(tx *gorm.DB, code string) (*account, error) {
	var rows []account
	if err := tx.Raw(`
		SELECT id, code, name FROM accounts
		WHERE code = ? AND deleted_at IS NULL`, code).Scan(&rows).Error; err != nil {
		return nil, err
	}
	if len(rows) == 0 {
		return nil, fmt.Errorf(
			"finance: this tenant has no account %s, so a goods receipt cannot be posted", code)
	}
	return &rows[0], nil
}

// assertJournalBalances is §8.4 step 6's "assert debits equal credits before
// insert", run immediately after the lines go in and before the transaction can
// commit.
//
// Both sums are computed by PostgreSQL, where the operands are still NUMERIC.
// Comparing them in Go would need arithmetic on httpx.Numeric, which does not
// have any on purpose — and a float64 comparison here would be a ledger deciding
// whether it balances in a type that cannot represent a tenth.
func assertJournalBalances(tx *gorm.DB, entryID uuid.UUID) error {
	var checks []struct {
		Balanced bool
		Lines    int
		Debits   httpx.Numeric
		Credits  httpx.Numeric
	}
	if err := tx.Raw(`
		SELECT (SUM(debit) = SUM(credit)) AS balanced,
		       count(*)                   AS lines,
		       SUM(debit)::text           AS debits,
		       SUM(credit)::text          AS credits
		FROM journal_entry_lines WHERE journal_entry_id = ?`, entryID).
		Scan(&checks).Error; err != nil {
		return err
	}
	if len(checks) == 0 || checks[0].Lines < 2 {
		return fmt.Errorf("finance: journal entry %s has fewer than two lines", entryID)
	}
	if !checks[0].Balanced {
		return fmt.Errorf("finance: journal entry %s is unbalanced: debits %s, credits %s",
			entryID, checks[0].Debits, checks[0].Credits)
	}
	return nil
}
