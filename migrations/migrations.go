package migrations

import (
	"context"
	_ "embed"
	"fmt"
	"strings"

	"gorm.io/gorm"
)

//go:embed 001_init.sql
var initialSchema string

//go:embed 002_registration.sql
var registrationSchema string

func Apply(ctx context.Context, db *gorm.DB) error {
	if err := ApplyInitial(ctx, db); err != nil {
		return err
	}
	for _, statement := range strings.Split(registrationSchema, ";") {
		if strings.TrimSpace(statement) == "" {
			continue
		}
		if err := db.WithContext(ctx).Exec(statement).Error; err != nil {
			return fmt.Errorf("apply 002_registration.sql: %w", err)
		}
	}
	return nil
}

// ApplyInitial is deliberately limited to phase 1's CREATE TABLE IF NOT EXISTS.
// The SQL file contains no procedures or semicolons inside string literals.
func ApplyInitial(ctx context.Context, db *gorm.DB) error {
	for _, statement := range strings.Split(initialSchema, ";") {
		if strings.TrimSpace(statement) == "" {
			continue
		}
		if err := db.WithContext(ctx).Exec(statement).Error; err != nil {
			return fmt.Errorf("apply 001_init.sql: %w", err)
		}
	}
	return nil
}
