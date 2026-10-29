package event

import (
	"context"
	"time"

	"gorm.io/gorm"
)

type demoUser struct {
	ID           uint64 `gorm:"primaryKey"`
	Username     string
	PasswordHash string
	Role         string
	CreatedAt    time.Time
}

func (demoUser) TableName() string { return "users" }

// Seed adds examples without deleting or overwriting existing records.
func Seed(ctx context.Context, db *gorm.DB, now time.Time) error {
	return db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		user := demoUser{}
		if err := tx.Where("username = ?", "organizer_demo").Attrs(demoUser{
			Username: "organizer_demo", PasswordHash: "!login-disabled-stage1", Role: "organizer", CreatedAt: now.UTC(),
		}).FirstOrCreate(&user).Error; err != nil {
			return err
		}
		// Lock the organizer so two concurrent seed commands do not create duplicates.
		if err := tx.Exec("SELECT id FROM users WHERE id = ? FOR UPDATE", user.ID).Error; err != nil {
			return err
		}
		for _, sample := range DemoEvents(now, user.ID) {
			sample.ID = 0 // Let MySQL allocate IDs; never overwrite an existing row.
			var existing Event
			if err := tx.Where("organizer_id = ? AND title = ?", user.ID, sample.Title).
				Attrs(sample).FirstOrCreate(&existing).Error; err != nil {
				return err
			}
		}
		return nil
	})
}
