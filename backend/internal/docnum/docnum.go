// Package docnum allocates document numbers — `<PREFIX>-<YYYYMM>-<SEQ4>` (§8.1).
//
// THREE THINGS MAKE THIS CORRECT, AND ALL THREE ARE EASY TO GET WRONG.
//
//  1. The period is computed in the **tenant's** timezone, read from `tenants`.
//     `to_char(now(), 'YYYYMM')` evaluates in the session zone, which is UTC
//     everywhere (§2.5.2) — so a requisition created at 00:30 on 1 August in
//     Jakarta is 17:30 on 31 July UTC and would be filed under the previous
//     month. Two environments would then allocate from different counters for
//     the same instant (E5, J4).
//
//  2. Allocation is a locking upsert, not a sequence. A PostgreSQL sequence is
//     not tenant-aware and does not roll back, so it would leave gaps and share
//     one counter between tenants. The `DO UPDATE` takes a row lock, which is
//     what makes twenty concurrent allocations produce twenty distinct numbers
//     rather than colliding (E3).
//
//  3. It runs in the **caller's** transaction — the one the document insert is
//     in. That is what makes a rolled-back document not consume a number (E4),
//     and it is why this takes a *gorm.DB rather than a pool.
//
// This package is deliberately separate from internal/api: four document types
// across two modules allocate from it, and its own tests can then drive it
// directly against a real database rather than through HTTP.
package docnum

import (
	"fmt"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// The four document types (§3 naming contract). Same four strings the
// doc_seq_type_valid CHECK constraint permits.
const (
	PR = "PR"
	PO = "PO"
	GR = "GR"
	JE = "JE"
)

// Allocate reserves the next number for this tenant, document type, and current
// period, inside the caller's transaction.
//
// The transaction must already have tenant context set (I1):
// `document_sequences` is RLS-forced like every other tenant table.
func Allocate(tx *gorm.DB, tenantID uuid.UUID, docType string) (string, error) {
	return AllocateAt(tx, tenantID, docType, nil)
}

// AllocateAt dates the allocation at an explicit instant instead of now.
//
// Every caller in the application uses Allocate. This exists because the claim
// that the period is the *tenant's* month and not the server's cannot be made to
// fail on an ordinary mid-month afternoon — no real timezone moves 26 July into
// another month — so E5 needs a controllable clock. The seam is one COALESCE
// inside the same expression the production path evaluates, so what E5 exercises
// is the shipped conversion.
//
// A backdated document is post-MVP; when it arrives, this is where the date
// comes in.
func AllocateAt(tx *gorm.DB, tenantID uuid.UUID, docType string, at *time.Time) (string, error) {
	// `tenants` carries no RLS — it is read before tenant context exists during
	// identity resolution (§6.8) — so the tenant filter here is written by hand.
	var period string
	if err := tx.Raw(`
		SELECT to_char(COALESCE(?::timestamptz, now()) AT TIME ZONE t.timezone, 'YYYYMM')
		FROM tenants t WHERE t.id = ?`, at, tenantID).Scan(&period).Error; err != nil {
		return "", fmt.Errorf("docnum: read period for tenant %s: %w", tenantID, err)
	}
	if period == "" {
		// No such tenant, or a tenant with no timezone. Either way, allocating a
		// number under a guessed period would file the document in a month
		// nobody chose.
		return "", fmt.Errorf("docnum: tenant %s has no period (missing tenant or timezone)", tenantID)
	}

	var last int
	if err := tx.Raw(`
		INSERT INTO document_sequences (tenant_id, doc_type, period, last_number)
		VALUES (?, ?, ?, 1)
		ON CONFLICT (tenant_id, doc_type, period)
		DO UPDATE SET last_number = document_sequences.last_number + 1
		RETURNING last_number`, tenantID, docType, period).Scan(&last).Error; err != nil {
		return "", fmt.Errorf("docnum: allocate %s for tenant %s: %w", docType, tenantID, err)
	}
	if last < 1 {
		// Unreachable: the upsert either inserts 1 or increments. A zero here
		// would mean RETURNING gave nothing, and emitting `PR-202607-0000` would
		// hide that behind a plausible-looking number.
		return "", fmt.Errorf("docnum: allocate %s returned no number", docType)
	}

	// %04d is a minimum width, not a limit: the 10000th document of a month
	// becomes PR-202607-10000 rather than silently wrapping to 0000.
	return fmt.Sprintf("%s-%s-%04d", docType, period, last), nil
}
