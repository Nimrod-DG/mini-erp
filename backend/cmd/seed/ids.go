package main

import (
	"strings"

	"github.com/google/uuid"
)

// Deterministic identity, which is the whole of the seed's idempotency story.
//
// EVERY ROW THE SEED WRITES HAS AN ID DERIVED FROM WHAT IT IS, not from
// gen_random_uuid(). Nusantara's PKG-TAPE has the same UUID on the first run and
// the tenth, so a re-run is `ON CONFLICT (id) DO NOTHING` — no existence checks
// scattered through the code, no "have I seeded this already?" flag table, and
// "reseeding twice in a row produces the same database" is true by construction
// rather than by care.
//
// It also makes the demo's URLs stable, which matters more than it sounds:
// /inventory/products/<uuid> in a screenshot or a bookmark still resolves after
// the database has been rebuilt.
//
// This is emphatically a *seed* technique. Nothing in the application derives an
// id from its content — a production row's identity must not change when
// somebody corrects a typo in its name.

// seedNamespace is an arbitrary fixed UUID. Its only job is to keep these names
// from colliding with anything else that hashes strings into UUIDs. It must
// never change: changing it renames every row in the demo database, and a
// reseed would then write a second copy of everything alongside the first.
var seedNamespace = uuid.MustParse("9f2b6c14-0a3d-4e57-9b21-6d8c5f0e7a44")

// seedID is a version-5 UUID over the slash-joined parts.
//
// The parts are a path, so the caller reads as what the row is:
// seedID("product", "nusantara", "PKG-TAPE"). Every call site includes the
// tenant slug, because two tenants have products with the same SKU and they are
// not the same product.
func seedID(parts ...string) uuid.UUID {
	return uuid.NewSHA1(seedNamespace, []byte(strings.Join(parts, "/")))
}
