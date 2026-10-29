package event

import (
	"context"
	"errors"

	"gorm.io/gorm"
)

var ErrNotFound = errors.New("event not found")

// Repository has two implementations: read-only demo data and actual MySQL.
type Repository interface {
	ListPublished(ctx context.Context, beforeID uint64, limit int) ([]Event, error)
	GetPublished(ctx context.Context, id uint64) (Event, error)
}

type MySQLRepository struct{ db *gorm.DB }

func NewMySQLRepository(db *gorm.DB) *MySQLRepository { return &MySQLRepository{db: db} }

func (r *MySQLRepository) GetAvailability(ctx context.Context, id uint64) (Event, error) {
	var e Event
	err := r.db.WithContext(ctx).Select("id", "capacity", "registered_count").Where("id = ? AND status = ?", id, "published").First(&e).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return e, ErrNotFound
	}
	return e, err
}

func (r *MySQLRepository) ListPublished(ctx context.Context, beforeID uint64, limit int) ([]Event, error) {
	query := r.db.WithContext(ctx).Where("status = ?", "published").Order("id DESC").Limit(limit)
	if beforeID > 0 {
		query = query.Where("id < ?", beforeID)
	}
	var events []Event
	err := query.Find(&events).Error
	return events, err
}

func (r *MySQLRepository) GetPublished(ctx context.Context, id uint64) (Event, error) {
	var e Event
	err := r.db.WithContext(ctx).Where("id = ? AND status = ?", id, "published").First(&e).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return Event{}, ErrNotFound
	}
	return e, err
}
