// Package db owns the database connections and the one helper that every
// tenant-scoped query goes through.
package db

import (
	"fmt"
	"sync"

	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

// Pools holds the two connections the API uses at runtime. The migrate URL is
// deliberately absent: migrations are a separate process, never the API's job.
//
// Both are opened lazily. Cloud Run scales to zero, so a cold start that dials
// the database before it has a request to serve pays the latency for nothing —
// and a database that is briefly unreachable should not stop the container
// from booting and answering /api/health.
type Pools struct {
	appURL   string
	adminURL string

	appOnce sync.Once
	app     *gorm.DB
	appErr  error

	adminOnce sync.Once
	admin     *gorm.DB
	adminErr  error
}

func NewPools(appURL, adminURL string) *Pools {
	return &Pools{appURL: appURL, adminURL: adminURL}
}

// App returns the erp_app connection. RLS applies to it; every tenant-scoped
// query on it must go through WithTenant.
func (p *Pools) App() (*gorm.DB, error) {
	p.appOnce.Do(func() {
		p.app, p.appErr = open(p.appURL)
		if p.appErr != nil {
			p.appErr = fmt.Errorf("db: open app pool: %w", p.appErr)
		}
	})
	return p.app, p.appErr
}

// Admin returns the erp_admin connection, for platform administration only.
func (p *Pools) Admin() (*gorm.DB, error) {
	p.adminOnce.Do(func() {
		p.admin, p.adminErr = open(p.adminURL)
		if p.adminErr != nil {
			p.adminErr = fmt.Errorf("db: open admin pool: %w", p.adminErr)
		}
	})
	return p.admin, p.adminErr
}

// Close releases whichever pools were actually opened.
func (p *Pools) Close() error {
	var firstErr error
	for _, g := range []*gorm.DB{p.app, p.admin} {
		if g == nil {
			continue
		}
		sqlDB, err := g.DB()
		if err != nil {
			if firstErr == nil {
				firstErr = err
			}
			continue
		}
		if err := sqlDB.Close(); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	return firstErr
}

func open(url string) (*gorm.DB, error) {
	g, err := gorm.Open(postgres.Open(url), &gorm.Config{})
	if err != nil {
		return nil, err
	}
	sqlDB, err := g.DB()
	if err != nil {
		return nil, err
	}
	sqlDB.SetMaxOpenConns(10)
	sqlDB.SetMaxIdleConns(5)
	return g, nil
}
