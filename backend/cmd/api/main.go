// Command api is the mini-erp HTTP server.
package main

import (
	"context"
	"log"

	"github.com/DGosal/mini-erp/backend/internal/api"
	"github.com/DGosal/mini-erp/backend/internal/auth"
	"github.com/DGosal/mini-erp/backend/internal/config"
	"github.com/DGosal/mini-erp/backend/internal/db"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		log.Fatal(err)
	}

	// Built at boot, not per request: it fails loudly here if the service
	// account key is missing, rather than turning every request into a 500.
	firebaseAuth, err := auth.NewFirebase(context.Background(), cfg.FirebaseProjectID)
	if err != nil {
		log.Fatal(err)
	}

	pools := db.NewPools(cfg.DatabaseURL, cfg.AdminDatabaseURL)
	defer func() {
		if err := pools.Close(); err != nil {
			log.Printf("closing pools: %v", err)
		}
	}()

	// One Admin SDK client, handed over as two narrow interfaces: the request
	// chain gets a Verifier that can only return a UID, and the two
	// provisioning endpoints get a UserManager (§3.4).
	app := api.New(api.Deps{
		Pools:       pools,
		Verifier:    firebaseAuth,
		Users:       firebaseAuth,
		CORSOrigins: cfg.CORSOrigins,
	})

	log.Printf("mini-erp api listening on :%s", cfg.Port)
	if err := app.Listen(":" + cfg.Port); err != nil {
		log.Fatal(err)
	}
}
