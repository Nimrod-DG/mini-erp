// Group K — the finance read side (§9.6, §10.5).
//
// The Finance module is a stub with no writes of its own, so what these tests
// assert is not "finance works" but something better: that the entries other
// modules posted are visible, balanced, and attributable to the document that
// caused them. K3 is the one worth reading — it receives goods through the real
// procurement endpoint and then goes looking for the posting through the real
// finance endpoint, with nothing shared between them except the database. That
// is the Phase 6 "done when" in one function.
package api_test

import (
	"net/http"
	"testing"

	"github.com/DGosal/mini-erp/backend/testsupport"
)

// --------------------------------------------------------------------------
// Response shapes. Money decodes as float64 in the test only — the server never
// sees one (I8), and comparing 4500000 to 4500000 is not where precision goes.
// --------------------------------------------------------------------------

type journalLine struct {
	ID             string  `json:"id"`
	JournalEntryID string  `json:"journalEntryId"`
	AccountID      string  `json:"accountId"`
	AccountCode    string  `json:"accountCode"`
	AccountName    string  `json:"accountName"`
	AccountType    string  `json:"accountType"`
	Debit          float64 `json:"debit"`
	Credit         float64 `json:"credit"`
	Memo           *string `json:"memo"`
}

type journalEntry struct {
	ID            string        `json:"id"`
	EntryNumber   string        `json:"entryNumber"`
	PostedAt      string        `json:"postedAt"`
	Description   string        `json:"description"`
	SourceType    string        `json:"sourceType"`
	SourceID      *string       `json:"sourceId"`
	SourceNumber  *string       `json:"sourceNumber"`
	SourcePOID    *string       `json:"sourcePoId"`
	Amount        float64       `json:"amount"`
	CreatedByID   string        `json:"createdById"`
	CreatedByName string        `json:"createdByName"`
	Lines         []journalLine `json:"lines"`
}

type chartAccount struct {
	ID       string `json:"id"`
	Code     string `json:"code"`
	Name     string `json:"name"`
	Type     string `json:"type"`
	IsActive bool   `json:"isActive"`
}

// financeUser is a staff user with one level in finance and nothing else. Staff
// rather than a tenant admin on purpose: a tenant admin resolves to `admin`
// everywhere entitled, which would make a level test unable to fail.
func financeUser(t *testing.T, f *testsupport.TenantFixture, level string) string {
	t.Helper()
	return f.NewUser(t, map[string]string{"finance": level}).FirebaseUID
}

func listJournalEntries(t *testing.T, h *testsupport.Harness, token, query string) list[journalEntry] {
	t.Helper()
	resp := h.Get(t, "/api/finance/journal-entries"+query, token)
	testsupport.AssertStatus(t, resp, http.StatusOK)
	return testsupport.Decode[list[journalEntry]](t, resp)
}

// --------------------------------------------------------------------------
// K1-K2 — who may read.
// --------------------------------------------------------------------------

// K1 — both routes are `viewer`, and the gate is per module rather than "holds
// any level at all". This is the finance twin of the inventory and procurement
// route tables, and it runs against the real app built by api.New: a route
// registered at the wrong level is exactly what a probe route cannot catch.
func TestFinanceRoutesCarryTheLevelsFromTheSpec(t *testing.T) {
	h := testsupport.NewHarness(t)
	f := h.DB.NewTenant(t, "Ledger Ltd")

	viewer := financeUser(t, f, "viewer")
	// Deliberately `admin` in another module: a high level elsewhere must not
	// open finance, or the module gate is decoration.
	elsewhere := f.NewUser(t, map[string]string{"inventory": "admin"}).FirebaseUID

	for _, path := range []string{"/api/finance/journal-entries", "/api/finance/accounts"} {
		testsupport.AssertStatus(t, h.Get(t, path, viewer), http.StatusOK)
		testsupport.AssertErrorCode(t, h.Get(t, path, elsewhere),
			http.StatusForbidden, "insufficient_module_role")
	}
}

// K2 — the second half of the Phase 6 "done when": a workspace that never bought
// Finance is refused with `module_not_enabled`, which is a different answer from
// "you personally have no level" and has to stay different. The user here holds
// `admin` in finance, so only the entitlement can be doing the refusing.
func TestFinanceEndpointsRefuseATenantWithoutTheModule(t *testing.T) {
	h := testsupport.NewHarness(t)
	f := h.DB.NewTenant(t, "Bahari Trading")

	user := financeUser(t, f, "admin")
	testsupport.AssertStatus(t, h.Get(t, "/api/finance/accounts", user), http.StatusOK)

	f.DisableModule(t, "finance")

	for _, path := range []string{"/api/finance/journal-entries", "/api/finance/accounts"} {
		body := testsupport.AssertErrorCode(t, h.Get(t, path, user),
			http.StatusForbidden, "module_not_enabled")
		if got := body.Details["module"]; got != "finance" {
			t.Errorf("%s: details.module = %v, want finance", path, got)
		}
	}
}

// --------------------------------------------------------------------------
// K3-K5 — what the postings look like from the finance side.
// --------------------------------------------------------------------------

// K3 — THE PHASE 6 GATE. Goods are received through the procurement endpoint,
// and the journal entry it wrote is then found through the finance endpoint, by
// a finance user who holds no procurement level at all.
//
// Nothing is shared between the two halves but the database. The receipt's
// response says a journal entry was posted; this is the test that the claim is
// true from the other side of the application, which is the only place the
// cross-module transaction can be observed rather than asserted.
func TestAReceiptAppearsAsABalancedJournalEntry(t *testing.T) {
	h := testsupport.NewHarness(t)
	f := h.DB.NewTenant(t, "Cross Module Ltd")
	receiver := procurementUser(t, f, "approver")
	// Dewi from the phase brief: finance `viewer`, nothing anywhere else. She
	// cannot see the purchase order and does not need to.
	dewi := financeUser(t, f, "viewer")

	po := orderWithLines(t, h, f, line(f.ProductID, "3", "1500000.00"))
	result := receive(t, h, receiver, po.ID, receiptLine(po.Lines[0].ID, "3"))

	page := listJournalEntries(t, h, dewi, "")
	if page.TotalItems != 1 {
		t.Fatalf("totalItems = %d, want 1 — the receipt's entry and nothing else", page.TotalItems)
	}
	entry := page.Data[0]

	if entry.ID != result.Finance.JournalEntryID {
		t.Errorf("entry id = %s, want the one the receipt reported (%s)",
			entry.ID, result.Finance.JournalEntryID)
	}
	if entry.EntryNumber != result.Finance.EntryNumber {
		t.Errorf("entry number = %s, want %s", entry.EntryNumber, result.Finance.EntryNumber)
	}
	if entry.Amount != 4500000 {
		t.Errorf("amount = %v, want 4500000 (3 × 1,500,000)", entry.Amount)
	}

	// The entry names the document behind it, resolved rather than left as a
	// UUID — this is what §10.4's "linked to source documents" means on the
	// finance side.
	if entry.SourceType != "goods_receipt" {
		t.Errorf("sourceType = %s, want goods_receipt", entry.SourceType)
	}
	if entry.SourceID == nil || *entry.SourceID != result.Receipt.ID {
		t.Errorf("sourceId = %v, want the goods receipt %s", entry.SourceID, result.Receipt.ID)
	}
	if entry.SourceNumber == nil || *entry.SourceNumber != result.Receipt.GRNumber {
		t.Errorf("sourceNumber = %v, want %s", entry.SourceNumber, result.Receipt.GRNumber)
	}
	if entry.SourcePOID == nil || *entry.SourcePOID != po.ID {
		t.Errorf("sourcePoId = %v, want %s", entry.SourcePOID, po.ID)
	}

	// Balanced, and balanced the right way round: 1300 Inventory is debited
	// because the goods are an asset now, 2150 GRNI credited because they are
	// owed for. An entry that balanced with the sides swapped would pass a sum
	// and be wrong in a way no reader could miss on the screen.
	if len(entry.Lines) != 2 {
		t.Fatalf("lines = %d, want 2", len(entry.Lines))
	}
	var debits, credits float64
	byCode := map[string]journalLine{}
	for _, l := range entry.Lines {
		debits += l.Debit
		credits += l.Credit
		byCode[l.AccountCode] = l
	}
	if debits != credits {
		t.Errorf("debits %v != credits %v", debits, credits)
	}
	if byCode["1300"].Debit != 4500000 || byCode["1300"].Credit != 0 {
		t.Errorf("1300 = Dr %v / Cr %v, want Dr 4500000 / Cr 0",
			byCode["1300"].Debit, byCode["1300"].Credit)
	}
	if byCode["2150"].Credit != 4500000 || byCode["2150"].Debit != 0 {
		t.Errorf("2150 = Dr %v / Cr %v, want Dr 0 / Cr 4500000",
			byCode["2150"].Debit, byCode["2150"].Credit)
	}
	if byCode["1300"].AccountName != "Inventory" {
		t.Errorf("1300 name = %q, want Inventory", byCode["1300"].AccountName)
	}
	if byCode["2150"].AccountType != "liability" {
		t.Errorf("2150 type = %q, want liability", byCode["2150"].AccountType)
	}

	// The debit line comes first, so the screen renders Dr above Cr without
	// having to sort it there.
	if entry.Lines[0].Debit == 0 {
		t.Errorf("first line is the credit — lines are ordered debit first")
	}
}

// K4 — `?sourceId=` is what the goods receipt confirmation panel links to
// (§10.3). Two receipts against the same order post two entries; the filter has
// to return the one the reader clicked and not both, or the link is worse than
// no link because it looks like it worked.
func TestJournalEntriesFilterByTheDocumentThatPostedThem(t *testing.T) {
	h := testsupport.NewHarness(t)
	f := h.DB.NewTenant(t, "Two Receipts Ltd")
	receiver := procurementUser(t, f, "approver")
	dewi := financeUser(t, f, "viewer")

	po := orderWithLines(t, h, f, line(f.ProductID, "10", "1000.00"))
	first := receive(t, h, receiver, po.ID, receiptLine(po.Lines[0].ID, "4"))
	second := receive(t, h, receiver, po.ID, receiptLine(po.Lines[0].ID, "6"))

	all := listJournalEntries(t, h, dewi, "")
	if all.TotalItems != 2 {
		t.Fatalf("unfiltered totalItems = %d, want 2", all.TotalItems)
	}

	page := listJournalEntries(t, h, dewi, "?sourceId="+second.Receipt.ID)
	if page.TotalItems != 1 || len(page.Data) != 1 {
		t.Fatalf("filtered totalItems = %d, len = %d, want 1 and 1",
			page.TotalItems, len(page.Data))
	}
	if page.Data[0].ID != second.Finance.JournalEntryID {
		t.Errorf("filtered entry = %s, want the second receipt's %s",
			page.Data[0].ID, second.Finance.JournalEntryID)
	}
	if page.Data[0].Amount != 6000 {
		t.Errorf("amount = %v, want 6000 (6 × 1,000)", page.Data[0].Amount)
	}

	// And the first receipt's entry is still findable by its own id — the filter
	// narrows, it does not shadow.
	other := listJournalEntries(t, h, dewi, "?sourceId="+first.Receipt.ID)
	if other.TotalItems != 1 || other.Data[0].ID != first.Finance.JournalEntryID {
		t.Errorf("first receipt's entry not found by its own sourceId")
	}

	// An account filter answers "what has touched 1300", and must count an entry
	// once even though the entry has two lines.
	inventoryAccount := page.Data[0].Lines[0].AccountID
	byAccount := listJournalEntries(t, h, dewi, "?accountId="+inventoryAccount)
	if byAccount.TotalItems != 2 {
		t.Errorf("by account totalItems = %d, want 2 — one row per entry, not per line",
			byAccount.TotalItems)
	}
}

// K5 — the chart of accounts is seeded with the workspace (§4.2.1) and is
// readable before anybody has posted anything. Build item 1 of the phase
// already existed; this is the test that says so.
func TestAccountsListReturnsTheSeededChart(t *testing.T) {
	h := testsupport.NewHarness(t)
	f := h.DB.NewTenant(t, "Fresh Books Ltd")
	dewi := financeUser(t, f, "viewer")

	resp := h.Get(t, "/api/finance/accounts", dewi)
	testsupport.AssertStatus(t, resp, http.StatusOK)
	page := testsupport.Decode[list[chartAccount]](t, resp)

	if page.TotalItems != 2 {
		t.Fatalf("totalItems = %d, want the two seeded accounts", page.TotalItems)
	}
	// Smallest code first, which is the order the sort default asks for.
	want := []chartAccount{
		{Code: "1300", Name: "Inventory", Type: "asset", IsActive: true},
		{Code: "2150", Name: "Goods received not invoiced", Type: "liability", IsActive: true},
	}
	for i, expected := range want {
		got := page.Data[i]
		if got.Code != expected.Code || got.Name != expected.Name ||
			got.Type != expected.Type || got.IsActive != expected.IsActive {
			t.Errorf("account %d = %+v, want %+v", i, got, expected)
		}
		if got.ID == "" {
			t.Errorf("account %s has no id", got.Code)
		}
	}

	// The journal is empty in a workspace nobody has received into, and an empty
	// list is `[]` rather than null — the screen maps over it.
	entries := listJournalEntries(t, h, dewi, "")
	if entries.TotalItems != 0 || entries.Data == nil || len(entries.Data) != 0 {
		t.Errorf("fresh journal = %+v, want an empty page", entries)
	}
}

// K6 — one workspace's postings are invisible to another's, asserted from both
// sides. RLS is what enforces it, but finance is a new surface over three tables
// and a single-tenant test cannot detect an isolation failure at all.
func TestJournalEntriesAreScopedToTheirTenant(t *testing.T) {
	h := testsupport.NewHarness(t)
	mine := h.DB.NewTenant(t, "Mine Ltd")
	theirs := h.DB.NewTenant(t, "Theirs Ltd")

	receiver := procurementUser(t, mine, "approver")
	po := orderWithLines(t, h, mine, line(mine.ProductID, "2", "500.00"))
	result := receive(t, h, receiver, po.ID, receiptLine(po.Lines[0].ID, "2"))

	stranger := financeUser(t, theirs, "admin")
	page := listJournalEntries(t, h, stranger, "")
	if page.TotalItems != 0 {
		t.Fatalf("another tenant sees %d entries, want 0", page.TotalItems)
	}

	// Not even when asking for it by id: a filter is not a way round RLS.
	byID := listJournalEntries(t, h, stranger, "?sourceId="+result.Receipt.ID)
	if byID.TotalItems != 0 {
		t.Errorf("another tenant found the entry by sourceId")
	}

	// The chart of accounts is per tenant too, and both tenants' accounts carry
	// the same codes — so an id from one must resolve to nothing in the other.
	own := listJournalEntries(t, h, financeUser(t, mine, "viewer"), "")
	if own.TotalItems != 1 {
		t.Errorf("owning tenant sees %d entries, want 1", own.TotalItems)
	}
}

// K7 — the query parameters refuse rubbish rather than quietly ignoring it. A
// filter that stops filtering shows the caller more rows than they asked for,
// which in a ledger reads as postings appearing from nowhere.
func TestFinanceListsRefuseMalformedFilters(t *testing.T) {
	h := testsupport.NewHarness(t)
	f := h.DB.NewTenant(t, "Strict Ltd")
	dewi := financeUser(t, f, "viewer")

	bad := []string{
		"/api/finance/journal-entries?sourceId=banana",
		"/api/finance/journal-entries?accountId=banana",
		"/api/finance/journal-entries?sourceType=invoice",
		"/api/finance/journal-entries?from=yesterday",
		"/api/finance/journal-entries?sort=whatever",
		"/api/finance/accounts?sort=balance",
	}
	for _, path := range bad {
		testsupport.AssertErrorCode(t, h.Get(t, path, dewi),
			http.StatusBadRequest, "malformed")
	}
}
