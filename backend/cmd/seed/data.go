package main

// The demo, as data. §15 fixes the tenants, the seven users, and the document
// volumes exactly, because the acceptance test names these email addresses and
// counts these rows — so this file is the specification transcribed, and
// seed.go is the machinery that lands it.
//
// EVERYTHING HERE IS DETERMINISTIC. No random names, no `time.Now()` in a
// quantity, no shuffled order. A seed that produced different data on each run
// could not satisfy "reseeding twice in a row produces the same database", and
// a reviewer comparing two runs of the demo would be comparing two demos.

// Passwords, and the one place they are written down. §15: every seeded user
// gets the same one, and it goes nowhere near `erp-prod`.
const seedPassword = "password123"

// The platform operator. Belongs to no tenant, can read no tenant business
// data, and is the only account that can register a workspace (§4.2, §5.7).
const (
	superadminEmail = "super@erp.test"
	superadminName  = "Platform Operator"
)

// tenantSpec is one workspace: its identity, what it has bought, and who works
// in it.
type tenantSpec struct {
	Slug     string
	Name     string
	Timezone string
	// Modules is the entitlement set. A row is written to `tenant_modules` for
	// every module in the catalogue whether or not it is enabled, so the matrix
	// is complete from the start — the same shape POST /admin/tenants produces.
	Modules map[string]bool
	Users   []userSpec

	Warehouses []warehouseSpec
	Products   []productSpec
	Suppliers  []supplierSpec
}

// userSpec is one row in `users` plus its per-module levels.
type userSpec struct {
	Slug       string
	Email      string
	FullName   string
	TenantRole string
	// Roles is nil for a tenant admin, deliberately and importantly. §15: "Do
	// not create user_module_roles rows for Rina or Agus — their access is
	// derived from tenant_role." Seeding rows for them would make a later
	// demotion restore levels they never chose (§5.4), and B7 is the test that
	// this shape resolves to full access anyway.
	Roles map[string]string
}

type warehouseSpec struct {
	Code string
	Name string
}

type productSpec struct {
	SKU          string
	Name         string
	UOM          string
	ReorderPoint string
	StandardCost string
	// Opening is the §15 opening adjustment, one entry per warehouse in the
	// tenant's Warehouses list, dated at the start of the 60-day window. An
	// empty entry writes no row: `ledger_qty_nonzero` refuses a movement of
	// zero, and "we hold none of this here" is the absence of a row rather than
	// a row of nothing.
	//
	// It sits HERE, on the same line as ReorderPoint, on purpose. Whether a
	// product ends up in the low-stock widget is the relationship between these
	// two numbers, and §15 requires at least three products to be below after
	// everything else has run. Declaring the quantity in another file is how
	// that relationship stops being readable — and `verifyLowStock` checks the
	// outcome anyway, because a reorder point set here against a quantity set
	// somewhere else is exactly the pair that drifts.
	Opening []string
	// Deleted marks §15's "1 soft-deleted product", so the recycle bin and the
	// restore flow have something to act on.
	Deleted bool
	// Discontinued is `is_active = false`. A different question from Deleted
	// (§6.9.1), and having one of each is what makes the difference visible on
	// the product list.
	Discontinued bool
}

type supplierSpec struct {
	Code         string
	Name         string
	LeadTimeDays int
	PaymentTerms string
	// Inactive is §15's "1 inactive", so the active/inactive distinction is
	// visible without anybody having to deactivate one first.
	Inactive bool
}

// The two workspaces of §15.
//
// The differences are the point and neither is decoration:
//
//   - Bahari has **no Finance entitlement**, which is what makes module gating
//     visible without opening the admin console — and what makes Agus, a tenant
//     admin with no Finance access, demonstrate that the admin shortcut sits
//     below the entitlement ceiling (acceptance step 5).
//   - The timezones differ, so §2.5.3 is exercised: the same instant is a
//     different business date in Jakarta and Makassar, and document numbers are
//     periodised in the tenant's zone.
var tenants = []tenantSpec{
	{
		Slug:     "nusantara",
		Name:     "Nusantara Retail",
		Timezone: "Asia/Jakarta",
		Modules:  map[string]bool{"procurement": true, "inventory": true, "finance": true},
		Users: []userSpec{
			{
				Slug: "rina-nusantara", Email: "rina@nusantara.test",
				FullName: "Rina Wijaya", TenantRole: "admin",
				// No Roles — implicit admin everywhere entitled (§5.4).
			},
			{
				Slug: "budi-nusantara", Email: "budi@nusantara.test",
				FullName: "Budi Santoso", TenantRole: "staff",
				// The approver the acceptance test needs: C2 forbids approving
				// your own requisition for everybody, so a walkthrough with one
				// user cannot perform the approval step at all.
				Roles: map[string]string{"procurement": "approver", "inventory": "viewer"},
			},
			{
				Slug: "sari-nusantara", Email: "sari@nusantara.test",
				FullName: "Sari Dewanti", TenantRole: "staff",
				Roles: map[string]string{"procurement": "user", "inventory": "user"},
			},
			{
				Slug: "dewi-nusantara", Email: "dewi@nusantara.test",
				FullName: "Dewi Lestari", TenantRole: "staff",
				// The mirror image of Budi: reads Procurement, administers
				// Finance, holds nothing in Inventory. Between them the two prove
				// the levels are per module rather than per person.
				Roles: map[string]string{"procurement": "viewer", "finance": "admin"},
			},
		},
		Warehouses: []warehouseSpec{
			{Code: "GP", Name: "Gudang Pusat"},
			{Code: "GC", Name: "Gudang Cabang"},
		},
		Products:  nusantaraProducts,
		Suppliers: nusantaraSuppliers,
	},
	{
		Slug:     "bahari",
		Name:     "Bahari Logistics",
		Timezone: "Asia/Makassar",
		Modules:  map[string]bool{"procurement": true, "inventory": true, "finance": false},
		Users: []userSpec{
			{
				Slug: "agus-bahari", Email: "agus@bahari.test",
				FullName: "Agus Pratama", TenantRole: "admin",
				// The important seed row. A tenant admin with no Finance access,
				// because his tenant has no Finance entitlement — no user_module_roles
				// row can express that, and none is written.
			},
			{
				Slug: "manager-bahari", Email: "manager@bahari.test",
				FullName: "Intan Kusuma", TenantRole: "staff",
				Roles: map[string]string{"procurement": "approver", "inventory": "approver"},
			},
			{
				Slug: "staff-bahari", Email: "staff@bahari.test",
				FullName: "Rudi Hartono", TenantRole: "staff",
				Roles: map[string]string{"procurement": "user", "inventory": "viewer"},
			},
		},
		Warehouses: []warehouseSpec{
			{Code: "PLB", Name: "Pelabuhan Utama"},
			{Code: "TRS", Name: "Gudang Transit"},
		},
		Products:  bahariProducts,
		Suppliers: bahariSuppliers,
	},
}

// Ten products across three loose categories, per §15 — packaging, handling,
// and office — with varied reorder points and standard costs.
//
// Costs are in rupiah and are whole thousands, which is what makes the finance
// numbers on screen look like an Indonesian retailer's rather than like test
// data. They are decimal **strings** all the way to PostgreSQL: no quantity or
// price in this file is ever a Go float (I8).
//
// Which of these end up below their reorder point is NOT decided here. It falls
// out of the opening stock in movements.go, and `verifyLowStock` checks after
// everything else has run that at least three are — because §15 wants the
// low-stock widget populated on first load, and a reorder point set in this file
// against a quantity set in that one is exactly the pair that drifts.
// The three products deliberately left below their reorder point are index 0,
// 2, and 5 in each list — chosen so the widget shows one from each of the three
// categories rather than three of the same kind of thing.
//
// Nothing in the plan afterwards touches those three except one further negative
// adjustment on index 0, which pushes it further below rather than rescuing it.
// The requisitions and receipts all draw on indices 1, 3, 6, and 7, whose
// openings clear their reorder points with room to spare.
var nusantaraProducts = []productSpec{
	{SKU: "PKG-BOX-S", Name: "Kardus kecil 20×20×15", UOM: "pcs", ReorderPoint: "200", StandardCost: "3500",
		Opening: []string{"90", "60"}}, // 150 against 200 — low
	{SKU: "PKG-BOX-L", Name: "Kardus besar 60×40×40", UOM: "pcs", ReorderPoint: "120", StandardCost: "11000",
		Opening: []string{"250", "150"}},
	{SKU: "PKG-TAPE", Name: "Lakban bening 48mm", UOM: "roll", ReorderPoint: "150", StandardCost: "8500",
		Opening: []string{"60", "30"}}, // 90 against 150 — low
	{SKU: "PKG-WRAP", Name: "Plastik wrap 50cm", UOM: "roll", ReorderPoint: "40", StandardCost: "42000",
		Opening: []string{"80", "40"}},
	{SKU: "HND-PALLET", Name: "Palet kayu standar", UOM: "pcs", ReorderPoint: "25", StandardCost: "95000",
		Opening: []string{"40", "20"}},
	{SKU: "HND-TROLLEY", Name: "Trolley lipat 150kg", UOM: "unit", ReorderPoint: "4", StandardCost: "780000",
		Opening: []string{"2", ""}}, // 2 against 4 — low, and only in one warehouse
	{SKU: "HND-GLOVE", Name: "Sarung tangan kerja", UOM: "pair", ReorderPoint: "60", StandardCost: "17500",
		Opening: []string{"120", "80"}},
	{SKU: "OFF-PAPER", Name: "Kertas A4 80gsm", UOM: "ream", ReorderPoint: "30", StandardCost: "54000",
		Opening: []string{"30", "15"}},
	{SKU: "OFF-TONER", Name: "Toner printer mono", UOM: "unit", ReorderPoint: "3", StandardCost: "620000",
		Discontinued: true, Opening: []string{"10", ""}},
	// Deleted, and holding stock on purpose. A deleted product's balance stays
	// visible in the stock grid, marked (Phase 4's decision): the goods do not
	// leave the shelf when somebody tidies the catalogue, and the demo should
	// show that rather than hide it.
	{SKU: "OFF-LABEL", Name: "Label thermal 100×150", UOM: "roll", ReorderPoint: "20", StandardCost: "68000",
		Deleted: true, Opening: []string{"30", ""}},
}

var bahariProducts = []productSpec{
	{SKU: "SEA-STRAP", Name: "Lashing strap 5 ton", UOM: "pcs", ReorderPoint: "40", StandardCost: "135000",
		Opening: []string{"15", "10"}}, // 25 against 40 — low
	{SKU: "SEA-SEAL", Name: "Container seal bolt", UOM: "pcs", ReorderPoint: "300", StandardCost: "6500",
		Opening: []string{"500", "300"}},
	{SKU: "SEA-TARP", Name: "Terpal kontainer 6×8m", UOM: "pcs", ReorderPoint: "15", StandardCost: "425000",
		Opening: []string{"5", "3"}}, // 8 against 15 — low
	{SKU: "SEA-ROPE", Name: "Tali tambang 24mm", UOM: "m", ReorderPoint: "500", StandardCost: "18000",
		Opening: []string{"800", "400"}},
	{SKU: "HND-DOLLY", Name: "Hand dolly 300kg", UOM: "unit", ReorderPoint: "5", StandardCost: "1250000",
		Opening: []string{"8", "4"}},
	{SKU: "HND-VEST", Name: "Rompi safety reflektif", UOM: "pcs", ReorderPoint: "50", StandardCost: "62000",
		Opening: []string{"20", "10"}}, // 30 against 50 — low
	{SKU: "HND-HELMET", Name: "Helm proyek SNI", UOM: "pcs", ReorderPoint: "40", StandardCost: "85000",
		Opening: []string{"100", "50"}},
	{SKU: "OFF-FORM", Name: "Formulir bill of lading", UOM: "pad", ReorderPoint: "25", StandardCost: "32000",
		Opening: []string{"40", "20"}},
	{SKU: "OFF-INK", Name: "Tinta stempel biru", UOM: "btl", ReorderPoint: "10", StandardCost: "24000",
		Discontinued: true, Opening: []string{"20", ""}},
	{SKU: "OFF-FOLDER", Name: "Map arsip gantung", UOM: "pcs", ReorderPoint: "60", StandardCost: "9500",
		Deleted: true, Opening: []string{"100", ""}},
}

// Five suppliers each, with varied lead times and terms — the lead time is not
// decoration, it is what approval adds to today's date in the tenant's timezone
// to set the order's `expected_at` (§8.3).
var nusantaraSuppliers = []supplierSpec{
	{Code: "SUP-KMS", Name: "PT Kemas Sejahtera", LeadTimeDays: 3, PaymentTerms: "NET30"},
	{Code: "SUP-ANP", Name: "CV Aneka Plastik", LeadTimeDays: 7, PaymentTerms: "NET14"},
	{Code: "SUP-LGS", Name: "PT Logistik Sarana", LeadTimeDays: 14, PaymentTerms: "NET45"},
	{Code: "SUP-OFC", Name: "Toko Office Prima", LeadTimeDays: 2, PaymentTerms: "COD"},
	{Code: "SUP-LMA", Name: "UD Lama Tidak Aktif", LeadTimeDays: 21, PaymentTerms: "NET60", Inactive: true},
}

var bahariSuppliers = []supplierSpec{
	{Code: "SUP-MRN", Name: "PT Marine Supply Nusantara", LeadTimeDays: 5, PaymentTerms: "NET30"},
	{Code: "SUP-BJA", Name: "CV Baja Tali Perkasa", LeadTimeDays: 10, PaymentTerms: "NET30"},
	{Code: "SUP-SFT", Name: "PT Safety Gear Indonesia", LeadTimeDays: 4, PaymentTerms: "NET14"},
	{Code: "SUP-PRC", Name: "Toko Percetakan Bahari", LeadTimeDays: 2, PaymentTerms: "COD"},
	{Code: "SUP-OLD", Name: "UD Pemasok Lama", LeadTimeDays: 30, PaymentTerms: "NET60", Inactive: true},
}
