// Finance — the read side (§9.6, §10.5).
//
// The module is a stub, and this file is deliberately the whole of it: two list
// endpoints, both `viewer`, no writes. The chart of accounts is seeded when the
// workspace is created (§4.2.1, `seed_tenant_accounts`) and nothing in the MVP
// edits it, so there is no create, no patch, and no delete here — an editable
// chart needs rules about accounts that have postings, and those belong with the
// real Finance module rather than with a page that says "coming soon".
//
// What the endpoints are FOR is proving the cross-module write. Every journal
// entry in this tenant was written by `postReceiptJournal` inside a goods
// receipt's transaction, so a receipt that succeeded and an entry that is
// missing here cannot both be true. `?sourceId=<receipt id>` is the filter that
// turns the confirmation panel's "journal entry JE-… posted" from a claim into
// something the reader can open — the exact counterpart of the stock ledger's
// own `sourceId`.
package api

import (
	"fmt"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/DGosal/mini-erp/backend/internal/httpx"
)

// ModuleFinance is the module code from the naming contract.
const ModuleFinance = "finance"

// journalSourceTypes is the naming contract and the same pair
// `je_source_type_valid` enforces. `manual` has no endpoint that writes it in
// the MVP — manual journal entry is explicitly not built — but it is a legal
// value in the column, so a filter that could not name it would be lying about
// what the ledger can hold.
var journalSourceTypes = []string{"goods_receipt", "manual"}

var journalEntrySortable = map[string]string{
	"postedAt":    "je.posted_at",
	"entryNumber": "je.entry_number",
	"description": "je.description",
	"amount":      "COALESCE(t.total_debit, 0)",
}

var accountSortable = map[string]string{
	"code": "a.code",
	"name": "a.name",
	"type": "a.type",
}

// journalEntryRow is one posting, as it comes out of the database.
//
// The lines live on journalEntryDetail rather than here, for the same reason
// goodsReceiptRow and goodsReceiptDetail are two types: GORM's Scan cannot fill
// a slice-of-struct field and fails the whole query if it meets one.
//
// Amount is the total of the debit side. For a two-line entry that is also the
// credit side by construction — `jel_balanced` refuses to commit anything else —
// so naming one of them "the amount" is honest rather than a simplification.
type journalEntryRow struct {
	ID          uuid.UUID  `json:"id"`
	EntryNumber string     `json:"entryNumber"`
	PostedAt    time.Time  `json:"postedAt"`
	Description string     `json:"description"`
	SourceType  string     `json:"sourceType"`
	SourceID    *uuid.UUID `json:"sourceId"`
	// SourceNumber and SourcePOID resolve a `goods_receipt` entry's document, so
	// the screen can link back to the receipt's order rather than print a UUID.
	// Null for anything else, which in the MVP means nothing.
	SourceNumber  *string       `json:"sourceNumber"`
	SourcePOID    *uuid.UUID    `json:"sourcePoId"`
	Amount        httpx.Numeric `json:"amount"`
	CreatedByID   uuid.UUID     `json:"createdById"`
	CreatedByName string        `json:"createdByName"`
}

// journalEntryDetail is an entry with the lines that make it up. The list
// returns these: an entry without its lines is a number and a description, and
// the thing worth showing is Dr 1300 against Cr 2150.
type journalEntryDetail struct {
	journalEntryRow
	Lines []journalEntryLine `json:"lines"`
}

type journalEntryLine struct {
	ID             uuid.UUID     `json:"id"`
	JournalEntryID uuid.UUID     `json:"journalEntryId"`
	AccountID      uuid.UUID     `json:"accountId"`
	AccountCode    string        `json:"accountCode"`
	AccountName    string        `json:"accountName"`
	AccountType    string        `json:"accountType"`
	Debit          httpx.Numeric `json:"debit"`
	Credit         httpx.Numeric `json:"credit"`
	Memo           *string       `json:"memo"`
}

type accountRow struct {
	ID       uuid.UUID `json:"id"`
	Code     string    `json:"code"`
	Name     string    `json:"name"`
	Type     string    `json:"type"`
	IsActive bool      `json:"isActive"`
}

// listJournalEntries is GET /api/finance/journal-entries — level `viewer`.
//
// Entries come back newest first with their lines attached, because an entry
// without its lines is a number and a description: the thing worth showing is
// Dr 1300 against Cr 2150, which is what makes the double entry visible.
func (s *server) listJournalEntries(c *fiber.Ctx) error {
	_, tx, err := tenantScope(c)
	if err != nil {
		return err
	}
	params, err := httpx.ParseList(c, journalEntrySortable, "-postedAt")
	if err != nil {
		return malformed(c, "%s", err)
	}

	sourceType := trimmed(c.Query("sourceType"))
	if sourceType != "" && !contains(journalSourceTypes, sourceType) {
		return malformed(c, "sourceType must be one of %s.",
			strings.Join(journalSourceTypes, ", "))
	}
	// The entry one document posted. This is what the goods receipt confirmation
	// panel links to (§10.3): "journal entry JE-… posted" has to be followed by
	// the entry itself, or the reader has to take the sentence on trust.
	sourceID, ok := optionalUUID(c, "sourceId")
	if !ok {
		return malformed(c, "sourceId is not a valid id.")
	}
	accountID, ok := optionalUUID(c, "accountId")
	if !ok {
		return malformed(c, "accountId is not a valid id.")
	}
	from, ok := optionalTime(c, "from")
	if !ok {
		return malformed(c, "from must be an RFC 3339 timestamp.")
	}
	to, ok := optionalTime(c, "to")
	if !ok {
		return malformed(c, "to must be an RFC 3339 timestamp.")
	}

	// One filter expression, shared by the count and the page, so a total can
	// never disagree with the rows underneath it.
	//
	// The account filter is an EXISTS rather than a join: an entry touching the
	// same account on both sides would otherwise come back twice and be counted
	// twice, and "which entries touched 1300" is a question about entries.
	where := `
		FROM journal_entries je
		JOIN users u ON u.id = je.created_by
		` + journalSourceJoin + `
		` + journalTotalsJoin + `
		WHERE (? = '' OR je.source_type = ?)
		  AND (?::uuid IS NULL OR je.source_id = ?)
		  AND (?::uuid IS NULL OR EXISTS (
		        SELECT 1 FROM journal_entry_lines jl
		        WHERE jl.journal_entry_id = je.id AND jl.account_id = ?))
		  AND (?::timestamptz IS NULL OR je.posted_at >= ?)
		  AND (?::timestamptz IS NULL OR je.posted_at <= ?)
		  AND (je.entry_number ILIKE ? OR je.description ILIKE ?)`
	args := []any{
		sourceType, sourceType,
		sourceID, sourceID,
		accountID, accountID,
		from, from,
		to, to,
		params.Like(), params.Like(),
	}

	var total int64
	if err := tx.Raw(`SELECT count(*) `+where, args...).Scan(&total).Error; err != nil {
		return err
	}

	var rows []journalEntryRow
	page := append(append([]any{}, args...), params.PageSize, params.Offset())
	if err := tx.Raw(`
		SELECT je.id, je.entry_number, je.posted_at, je.description,
		       je.source_type, je.source_id,`+journalSourceColumns+`
		       COALESCE(t.total_debit, 0)::text AS amount,
		       je.created_by AS created_by_id, u.full_name AS created_by_name
		`+where+fmt.Sprintf(`
		ORDER BY %s
		LIMIT ? OFFSET ?`, params.OrderBy("je.id")), page...).Scan(&rows).Error; err != nil {
		return err
	}

	entries := make([]journalEntryDetail, len(rows))
	for i, row := range rows {
		entries[i] = journalEntryDetail{journalEntryRow: row}
	}
	if err := attachJournalLines(tx, entries); err != nil {
		return err
	}
	return c.JSON(httpx.NewListResponse(entries, params, total))
}

// listAccounts is GET /api/finance/accounts — level `viewer`.
//
// The whole chart, smallest code first, which for a two-account chart is the
// order anybody would read it in. No pagination games: the MVP seeds two rows
// and the §9.0 envelope is still what comes back, because every list in this
// application returns the same shape and the frontend has one hook for it.
func (s *server) listAccounts(c *fiber.Ctx) error {
	_, tx, err := tenantScope(c)
	if err != nil {
		return err
	}
	params, err := httpx.ParseList(c, accountSortable, "code")
	if err != nil {
		return malformed(c, "%s", err)
	}

	// No `?includeDeleted=true` here, unlike the master-data lists. Nothing in
	// the MVP can delete an account, so a recycle bin would be a view onto a
	// state no code path produces.
	where := `
		FROM accounts a
		WHERE a.deleted_at IS NULL
		  AND (a.code ILIKE ? OR a.name ILIKE ?)`
	args := []any{params.Like(), params.Like()}

	var total int64
	if err := tx.Raw(`SELECT count(*) `+where, args...).Scan(&total).Error; err != nil {
		return err
	}

	var rows []accountRow
	page := append(append([]any{}, args...), params.PageSize, params.Offset())
	if err := tx.Raw(`
		SELECT a.id, a.code, a.name, a.type, a.is_active
		`+where+fmt.Sprintf(`
		ORDER BY %s
		LIMIT ? OFFSET ?`, params.OrderBy("a.id")), page...).Scan(&rows).Error; err != nil {
		return err
	}

	return c.JSON(httpx.NewListResponse(rows, params, total))
}

// journalSourceJoin resolves the document behind a `goods_receipt` entry, the
// same shape and for the same reason as the stock ledger's ledgerSourceJoin: a
// LEFT JOIN, with the source type on the join rather than in the WHERE, so a
// future source type whose UUID happens to match a receipt cannot pick it up.
const journalSourceJoin = `
		LEFT JOIN goods_receipts gr
		  ON gr.id = je.source_id AND je.source_type = 'goods_receipt'`

const journalSourceColumns = `
		       gr.gr_number AS source_number, gr.po_id AS source_po_id,`

// journalTotalsJoin is the entry's value, summed where both operands are still
// NUMERIC (I8). Debits only: the credit side is equal by construction, and
// summing both to present one number would invite somebody to compare them here
// rather than in SQL where assertJournalBalances does it.
const journalTotalsJoin = `
		LEFT JOIN (
		  SELECT journal_entry_id, SUM(debit) AS total_debit
		  FROM journal_entry_lines GROUP BY journal_entry_id
		) t ON t.journal_entry_id = je.id`

// attachJournalLines fills in the lines for one page of entries, in one query.
//
// A LEFT JOIN on the page query would multiply the entries by their lines and
// break both the page size and the total. A query per entry would be 25 round
// trips for a screen. This is one round trip for the page, and the accounts are
// joined WITHOUT a deleted filter — an entry that posted against an account
// someone has since retired still posted against it (§6.9.1, Trap 3).
func attachJournalLines(tx *gorm.DB, entries []journalEntryDetail) error {
	if len(entries) == 0 {
		return nil
	}
	ids := make([]uuid.UUID, 0, len(entries))
	for _, entry := range entries {
		ids = append(ids, entry.ID)
	}

	var lines []journalEntryLine
	if err := tx.Raw(`
		SELECT jel.id, jel.journal_entry_id,
		       jel.account_id, a.code AS account_code, a.name AS account_name,
		       a.type AS account_type,
		       jel.debit::text  AS debit,
		       jel.credit::text AS credit,
		       jel.memo
		FROM journal_entry_lines jel
		JOIN accounts a ON a.id = jel.account_id
		WHERE jel.journal_entry_id IN ?
		ORDER BY jel.debit DESC, a.code ASC`, ids).Scan(&lines).Error; err != nil {
		return err
	}

	byEntry := make(map[uuid.UUID][]journalEntryLine, len(entries))
	for _, line := range lines {
		byEntry[line.JournalEntryID] = append(byEntry[line.JournalEntryID], line)
	}
	for i := range entries {
		// `[]` rather than null: the frontend maps over this, and an entry with
		// no lines cannot exist anyway — assertJournalBalances refuses fewer
		// than two before the transaction that wrote it can commit.
		entries[i].Lines = byEntry[entries[i].ID]
		if entries[i].Lines == nil {
			entries[i].Lines = []journalEntryLine{}
		}
	}
	return nil
}
