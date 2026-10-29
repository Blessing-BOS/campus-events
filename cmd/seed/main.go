package main

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"campus-events/internal/event"
	"campus-events/internal/platform/config"
	"campus-events/internal/platform/database"
	"campus-events/migrations"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "seed failed:", err)
		os.Exit(1)
	}
}

func run() error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	if cfg.DataSource != "mysql" {
		return fmt.Errorf("set APP_DATA_SOURCE=mysql to initialize the database; demo mode is already populated")
	}
	if !strings.HasPrefix(cfg.MySQL.Database, "campus_events_") {
		return fmt.Errorf("seed requires a dedicated database named campus_events_* to avoid modifying unrelated data")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	db, err := database.Open(ctx, cfg.MySQL)
	if err != nil {
		return err
	}
	sqlDB, err := db.DB()
	if err != nil {
		return err
	}
	defer sqlDB.Close()
	if err := migrations.Apply(ctx, db); err != nil {
		return err
	}
	password := os.Getenv("SEED_PASSWORD")
	if len(password) < 8 || len(password) > 72 {
		return fmt.Errorf("set SEED_PASSWORD (8-72 bytes) to initialize demo accounts")
	}
	if err := event.SeedFull(ctx, db, time.Now(), password); err != nil {
		return err
	}
	fmt.Println("Schema and demo data ready. Accounts: organizer_demo / student_demo. Password: SEED_PASSWORD. Existing account passwords were preserved.")
	return nil
}
