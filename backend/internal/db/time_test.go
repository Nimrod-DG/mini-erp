// Group J — time and timezone. A mismatch between a laptop in Asia/Jakarta and
// a container in UTC produces wrong data, not just odd-looking timestamps: the
// damage happens wherever an instant becomes a date or a month.
package db_test

import (
	"testing"
	"time"
	_ "time/tzdata" // so LoadLocation works on a machine with no zoneinfo

	"gorm.io/gorm"

	"github.com/DGosal/mini-erp/backend/internal/config"
	"github.com/DGosal/mini-erp/backend/testsupport"
)

// J1 — the database session timezone is UTC, for every role.
//
// Pinned twice: TZ/PGTZ on the container, and ALTER ROLE ... SET timezone in
// 000_roles.sql. The role-level setting is the one that survives a managed host
// whose containers you do not control, so it is what this asserts.
func TestJ1_DatabaseSessionTimezoneIsUTC(t *testing.T) {
	d := testsupport.NewTestDB(t)

	for name, conn := range map[string]*gorm.DB{
		"erp_migrate": d.Owner, "erp_app": d.App, "erp_admin": d.Admin,
	} {
		var tz string
		if err := conn.Raw(`SHOW timezone`).Scan(&tz).Error; err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if tz != "UTC" {
			t.Errorf("%s session timezone is %q, want UTC", name, tz)
		}
	}
}

// J2 — the API process runs in UTC. config.Load pins it, so a developer's
// laptop cannot disagree with the deployment about which month a document
// belongs to.
func TestJ2_ProcessLocalTimeIsUTC(t *testing.T) {
	t.Setenv("TZ", "Asia/Jakarta")

	// Missing DATABASE_URL and friends make Load return an error; the pin
	// happens first and is what is under test here.
	_, _ = config.Load()

	if time.Local != time.UTC {
		t.Fatalf("time.Local is %v, want UTC", time.Local)
	}
	if _, offset := time.Now().Zone(); offset != 0 {
		t.Fatalf("local zone offset is %ds, want 0", offset)
	}
}

// J3 — a TIMESTAMPTZ round-trips the same instant whatever zone the client is
// in. TIMESTAMPTZ stores an absolute instant; the session zone only decides how
// it is rendered.
func TestJ3_TimestampsRoundTripRegardlessOfClientZone(t *testing.T) {
	d := testsupport.NewTestDB(t)
	f := d.NewTenant(t, "Tenant A")

	jakarta, err := time.LoadLocation("Asia/Jakarta")
	if err != nil {
		t.Fatal(err)
	}
	// The same instant, expressed two ways. Written as Jakarta wall-clock.
	written := time.Date(2026, 7, 31, 23, 30, 0, 0, jakarta)

	f.Must(t, func(tx *gorm.DB) error {
		return tx.Exec(`
			INSERT INTO stock_ledger
			  (tenant_id, product_id, warehouse_id, entry_type, qty_delta,
			   source_type, occurred_at, created_by)
			VALUES (?, ?, ?, 'adjustment', 1, 'manual_adjustment', ?, ?)`,
			f.ID, f.ProductID, f.WarehouseID, written, f.User.ID).Error
	})

	for _, sessionZone := range []string{"UTC", "Asia/Jakarta", "America/New_York"} {
		var readBack time.Time
		var rendered string
		f.Must(t, func(tx *gorm.DB) error {
			if err := tx.Exec(`SET LOCAL TIME ZONE ` + quoteLiteral(sessionZone)).Error; err != nil {
				return err
			}
			if err := tx.Raw(`SELECT occurred_at FROM stock_ledger`).Scan(&readBack).Error; err != nil {
				return err
			}
			return tx.Raw(`SELECT to_char(occurred_at AT TIME ZONE 'UTC',
			                              'YYYY-MM-DD HH24:MI:SS') FROM stock_ledger`).
				Scan(&rendered).Error
		})
		if !readBack.Equal(written) {
			t.Errorf("session %s: read back %v, want the instant %v", sessionZone, readBack, written)
		}
		if want := "2026-07-31 16:30:00"; rendered != want {
			t.Errorf("session %s: stored UTC value is %q, want %q", sessionZone, rendered, want)
		}
	}
}

// J4 — a document's period is computed in the TENANT's timezone, not the
// server's. §2.5.1's example: 00:30 on 1 August in Jakarta is 17:30 on 31 July
// UTC, so a UTC period would file it under the previous month and two
// environments would allocate from different counters for the same instant.
func TestJ4_DocumentPeriodUsesTheTenantTimezone(t *testing.T) {
	d := testsupport.NewTestDB(t)
	f := d.NewTenantInTZ(t, "Jakarta tenant", "Asia/Jakarta")

	jakarta, err := time.LoadLocation("Asia/Jakarta")
	if err != nil {
		t.Fatal(err)
	}

	for _, tc := range []struct {
		name             string
		instant          time.Time
		wantTenantPeriod string
		wantUTCPeriod    string
	}{
		{
			// Late on the last day of the month: both agree.
			name:             "23:30 on the last day of July",
			instant:          time.Date(2026, 7, 31, 23, 30, 0, 0, jakarta),
			wantTenantPeriod: "202607",
			wantUTCPeriod:    "202607",
		},
		{
			// Just after midnight: only the tenant's zone gets it right.
			name:             "00:30 on the first day of August",
			instant:          time.Date(2026, 8, 1, 0, 30, 0, 0, jakarta),
			wantTenantPeriod: "202608",
			wantUTCPeriod:    "202607",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var tenantPeriod, utcPeriod string
			f.Must(t, func(tx *gorm.DB) error {
				return tx.Raw(`
					SELECT to_char(?::timestamptz AT TIME ZONE t.timezone, 'YYYYMM'),
					       to_char(?::timestamptz AT TIME ZONE 'UTC',       'YYYYMM')
					FROM tenants t WHERE t.id = ?`,
					tc.instant, tc.instant, f.ID).Row().Scan(&tenantPeriod, &utcPeriod)
			})
			if tenantPeriod != tc.wantTenantPeriod {
				t.Errorf("tenant period = %s, want %s", tenantPeriod, tc.wantTenantPeriod)
			}
			if utcPeriod != tc.wantUTCPeriod {
				t.Errorf("utc period = %s, want %s", utcPeriod, tc.wantUTCPeriod)
			}
		})
	}
}

// quoteLiteral wraps a zone name for SET TIME ZONE, which takes no parameters.
// The inputs here are constants in this file.
func quoteLiteral(s string) string { return "'" + s + "'" }
