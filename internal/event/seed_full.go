package event

import (
	"context"
	"fmt"
	"time"

	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"campus-events/internal/account"
)

func SeedFull(ctx context.Context, db *gorm.DB, now time.Time, password string) error {
	if err := Seed(ctx, db, now); err != nil {
		return err
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return err
	}
	return db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		for _, spec := range []struct{ name, role string }{{"organizer_demo", "organizer"}, {"student_demo", "student"}} {
			var user account.User
			if err := tx.Where("username = ?", spec.name).Attrs(account.User{Username: spec.name, Role: spec.role, PasswordHash: string(hash), CreatedAt: now.UTC()}).FirstOrCreate(&user).Error; err != nil {
				return err
			}
			if user.Role != spec.role {
				return fmt.Errorf("reserved demo username %s has a different role", spec.name)
			}
			if user.PasswordHash == "!login-disabled-stage1" {
				if err := tx.Model(&user).Update("password_hash", string(hash)).Error; err != nil {
					return err
				}
			}
		}
		var e Event
		var organizer account.User
		if err := tx.Where("username = ?", "organizer_demo").First(&organizer).Error; err != nil {
			return err
		}
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("organizer_id = ? AND title = ? AND status = ?", organizer.ID, "校园项目交流工作坊", "published").Order("id ASC").First(&e).Error; err != nil {
			return err
		}
		var student account.User
		if err := tx.Where("username = ?", "student_demo").First(&student).Error; err != nil {
			return err
		}
		var count int64
		if err := tx.Table("registrations").Where("event_id = ? AND user_id = ?", e.ID, student.ID).Count(&count).Error; err != nil {
			return err
		}
		if count == 0 && e.RegisteredCount < e.Capacity {
			if err := tx.Exec("INSERT INTO registrations (event_id,user_id,status,created_at,updated_at) VALUES (?,?,'active',?,?)", e.ID, student.ID, now.UTC(), now.UTC()).Error; err != nil {
				return err
			}
			if err := tx.Model(&e).UpdateColumn("registered_count", gorm.Expr("registered_count + 1")).Error; err != nil {
				return err
			}
		}
		return nil
	})
}
