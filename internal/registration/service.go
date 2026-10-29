package registration

import (
	"context"
	"errors"
	"time"

	"github.com/go-sql-driver/mysql"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"campus-events/internal/apierror"
	"campus-events/internal/event"
)

type Registration struct {
	ID          uint64     `json:"id" gorm:"primaryKey"`
	EventID     uint64     `json:"event_id"`
	UserID      uint64     `json:"user_id"`
	Status      string     `json:"status"`
	CheckedInAt *time.Time `json:"checked_in_at"`
	CreatedAt   time.Time  `json:"created_at"`
	UpdatedAt   time.Time  `json:"updated_at"`
}
type View struct {
	Registration
	Username           string    `json:"username"`
	EventTitle         string    `json:"event_title"`
	Location           string    `json:"location"`
	StartsAt           time.Time `json:"starts_at"`
	RegistrationEndsAt time.Time `json:"registration_ends_at"`
}
type Service struct {
	db  *gorm.DB
	now func() time.Time
}

func NewService(db *gorm.DB, now func() time.Time) *Service { return &Service{db, now} }

func (s *Service) GetMine(ctx context.Context, eventID, userID uint64) (Registration, error) {
	var result Registration
	err := s.db.WithContext(ctx).Where("event_id = ? AND user_id = ?", eventID, userID).First(&result).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return result, apierror.NotFound
	}
	return result, err
}
func (s *Service) transaction(ctx context.Context, fn func(*gorm.DB) error) error {
	err := s.db.WithContext(ctx).Transaction(fn)
	var me *mysql.MySQLError
	if errors.As(err, &me) && (me.Number == 1205 || me.Number == 1213) {
		return apierror.New(503, "RETRY_LATER", "当前报名较多，请稍后重试")
	}
	return err
}
func lockEvent(tx *gorm.DB, id uint64) (event.Event, error) {
	var e event.Event
	err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&e, id).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return e, apierror.NotFound
	}
	return e, err
}
func registrationWindow(e event.Event, now time.Time) error {
	if e.Status != "published" {
		return apierror.NotFound
	}
	if now.Before(e.RegistrationStartsAt) || !now.Before(e.RegistrationEndsAt) {
		return apierror.New(409, "REGISTRATION_CLOSED", "当前不在报名时间内")
	}
	return nil
}
func (s *Service) Signup(ctx context.Context, eventID, userID uint64) (Registration, error) {
	var result Registration
	err := s.transaction(ctx, func(tx *gorm.DB) error {
		e, err := lockEvent(tx, eventID)
		if err != nil {
			return err
		}
		now := s.now().UTC().Truncate(time.Millisecond)
		if err := registrationWindow(e, now); err != nil {
			return err
		}
		err = tx.Where("event_id = ? AND user_id = ?", eventID, userID).First(&result).Error
		if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}
		if err == nil && result.Status == "active" {
			return nil
		}
		if e.RegisteredCount >= e.Capacity {
			return apierror.New(409, "EVENT_FULL", "报名名额已满")
		}
		if errors.Is(err, gorm.ErrRecordNotFound) {
			result = Registration{EventID: eventID, UserID: userID, Status: "active", CreatedAt: now, UpdatedAt: now}
			if err := tx.Create(&result).Error; err != nil {
				return err
			}
		} else {
			result.Status = "active"
			result.CheckedInAt = nil
			result.UpdatedAt = now
			if err := tx.Save(&result).Error; err != nil {
				return err
			}
		}
		updated := tx.Model(&event.Event{}).Where("id = ? AND registered_count < capacity", eventID).UpdateColumn("registered_count", gorm.Expr("registered_count + 1"))
		if updated.Error != nil {
			return updated.Error
		}
		if updated.RowsAffected != 1 {
			return apierror.New(409, "EVENT_FULL", "报名名额已满")
		}
		return nil
	})
	return result, err
}
func (s *Service) Cancel(ctx context.Context, eventID, userID uint64) (Registration, error) {
	var result Registration
	err := s.transaction(ctx, func(tx *gorm.DB) error {
		e, err := lockEvent(tx, eventID)
		if err != nil {
			return err
		}
		if err := registrationWindow(e, s.now()); err != nil {
			return err
		}
		if err := tx.Where("event_id = ? AND user_id = ?", eventID, userID).First(&result).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return apierror.NotFound
			}
			return err
		}
		if result.Status == "cancelled" {
			return nil
		}
		if result.CheckedInAt != nil {
			return apierror.New(409, "ALREADY_CHECKED_IN", "已签到的报名不能取消")
		}
		result.Status = "cancelled"
		result.UpdatedAt = s.now().UTC().Truncate(time.Millisecond)
		if err := tx.Save(&result).Error; err != nil {
			return err
		}
		updated := tx.Model(&event.Event{}).Where("id = ? AND registered_count > 0", eventID).UpdateColumn("registered_count", gorm.Expr("registered_count - 1"))
		if updated.Error != nil {
			return updated.Error
		}
		if updated.RowsAffected != 1 {
			return errors.New("registration counter invariant violated")
		}
		return nil
	})
	return result, err
}
func (s *Service) CheckIn(ctx context.Context, eventID, registrationID, ownerID uint64) (Registration, error) {
	var result Registration
	err := s.transaction(ctx, func(tx *gorm.DB) error {
		e, err := lockEvent(tx, eventID)
		if err != nil {
			return err
		}
		if e.OrganizerID != ownerID {
			return apierror.Forbidden
		}
		now := s.now().UTC().Truncate(time.Millisecond)
		if e.Status != "published" || now.Before(e.StartsAt) || !now.Before(e.EndsAt) {
			return apierror.New(409, "CHECK_IN_CLOSED", "当前不在活动签到时间内")
		}
		if err := tx.Where("id = ? AND event_id = ?", registrationID, eventID).First(&result).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return apierror.NotFound
			}
			return err
		}
		if result.Status != "active" {
			return apierror.New(409, "REGISTRATION_CANCELLED", "报名已取消，不能签到")
		}
		if result.CheckedInAt != nil {
			return nil
		}
		result.CheckedInAt = &now
		result.UpdatedAt = now
		return tx.Save(&result).Error
	})
	return result, err
}
func (s *Service) Mine(ctx context.Context, userID, before uint64, limit int) ([]View, error) {
	return s.list(ctx, s.db.Where("registrations.user_id = ?", userID), before, limit)
}
func (s *Service) Roster(ctx context.Context, eventID, ownerID, before uint64, limit int) ([]View, error) {
	var e event.Event
	if err := s.db.WithContext(ctx).First(&e, eventID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, apierror.NotFound
		}
		return nil, err
	}
	if e.OrganizerID != ownerID {
		return nil, apierror.Forbidden
	}
	return s.list(ctx, s.db.Where("registrations.event_id = ?", eventID), before, limit)
}
func (s *Service) list(ctx context.Context, q *gorm.DB, before uint64, limit int) ([]View, error) {
	if before > 0 {
		q = q.Where("registrations.id < ?", before)
	}
	result := make([]View, 0)
	err := q.WithContext(ctx).Model(&Registration{}).
		Select("registrations.*, users.username, events.title AS event_title, events.location, events.starts_at, events.registration_ends_at").
		Joins("JOIN users ON users.id = registrations.user_id").Joins("JOIN events ON events.id = registrations.event_id").Order("registrations.id DESC").Limit(limit).Scan(&result).Error
	return result, err
}
