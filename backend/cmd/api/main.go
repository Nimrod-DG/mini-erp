// Command api is the mini-erp HTTP server.
package main

import (
	"log"
	"strings"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/cors"
	"github.com/gofiber/fiber/v2/middleware/logger"
	"github.com/gofiber/fiber/v2/middleware/recover"

	"github.com/DGosal/mini-erp/backend/internal/config"
	"github.com/DGosal/mini-erp/backend/internal/db"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		log.Fatal(err)
	}

	pools := db.NewPools(cfg.DatabaseURL, cfg.AdminDatabaseURL)
	defer func() {
		if err := pools.Close(); err != nil {
			log.Printf("closing pools: %v", err)
		}
	}()

	app := fiber.New(fiber.Config{
		AppName:               "mini-erp",
		DisableStartupMessage: true,
	})
	app.Use(recover.New(), logger.New())
	app.Use(cors.New(cors.Config{
		AllowOrigins: strings.Join(cfg.CORSOrigins, ","),
		AllowHeaders: "Origin, Content-Type, Accept, Authorization",
		AllowMethods: "GET, POST, PATCH, DELETE, OPTIONS",
	}))

	// Liveness only: it must not touch the database, or a database blip would
	// make the platform restart a container that is perfectly healthy.
	app.Get("/api/health", func(c *fiber.Ctx) error {
		return c.JSON(fiber.Map{"status": "ok"})
	})

	log.Printf("mini-erp api listening on :%s", cfg.Port)
	if err := app.Listen(":" + cfg.Port); err != nil {
		log.Fatal(err)
	}
}
