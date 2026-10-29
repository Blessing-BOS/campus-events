package event

import (
	"context"
	"errors"
	"strings"
	"time"
	"unicode/utf8"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"campus-events/internal/apierror"
)

type Input struct {
	Title                string    `json:"title"`
	Description          string    `json:"description"`
	Location             string    `json:"location"`
	Capacity             int       `json:"capacity"`
	RegistrationStartsAt time.Time `json:"registration_starts_at"`
	RegistrationEndsAt   time.Time `json:"registration_ends_at"`
	StartsAt             time.Time `json:"starts_at"`
	EndsAt               time.Time `json:"ends_at"`
}

func (i *Input) Validate(now time.Time) error {
	i.RegistrationStartsAt = i.RegistrationStartsAt.UTC().Truncate(time.Millisecond)
	i.RegistrationEndsAt = i.RegistrationEndsAt.UTC().Truncate(time.Millisecond)
	i.StartsAt = i.StartsAt.UTC().Truncate(time.Millisecond)
	i.EndsAt = i.EndsAt.UTC().Truncate(time.Millisecond)
	i.Title = strings.TrimSpace(i.Title)
	i.Description = strings.TrimSpace(i.Description)
	i.Location = strings.TrimSpace(i.Location)
	if utf8.RuneCountInString(i.Title) < 2 || utf8.RuneCountInString(i.Title) > 120 || utf8.RuneCountInString(i.Description) > 5000 || i.Location == "" || utf8.RuneCountInString(i.Location) > 200 {
		return apierror.New(400, "INVALID_ARGUMENT", "标题需为 2～120 字，地点不能为空，介绍最多 5000 字")
	}
	if i.Capacity < 1 || i.Capacity > 10000 {
		return apierror.New(400, "INVALID_ARGUMENT", "名额需为 1～10000")
	}
	if i.RegistrationStartsAt.IsZero() || !i.RegistrationStartsAt.Before(i.RegistrationEndsAt) || i.RegistrationEndsAt.After(i.StartsAt) || !i.StartsAt.Before(i.EndsAt) || !now.Before(i.RegistrationEndsAt) {
		return apierror.New(400, "INVALID_TIME_RANGE", "请检查报名时间和活动时间，报名截止需晚于当前时间")
	}
	return nil
}

type ManagedView struct {
	View
	Status string `json:"status"`
}
type AdminService struct {
	db  *gorm.DB
	now func() time.Time
}

func NewAdminService(db *gorm.DB, now func() time.Time) *AdminService { return &AdminService{db, now} }
func (a *AdminService) List(ctx context.Context, owner, before uint64, limit int) ([]ManagedView, error) {
	q := a.db.WithContext(ctx).Where("organizer_id = ?", owner).Order("id DESC").Limit(limit)
	if before > 0 {
		q = q.Where("id < ?", before)
	}
	var records []Event
	if err := q.Find(&records).Error; err != nil {
		return nil, err
	}
	out := make([]ManagedView, 0, len(records))
	for _, e := range records {
		out = append(out, ManagedView{e.PublicView(a.now()), e.Status})
	}
	return out, nil
}
func (a *AdminService) Create(ctx context.Context, owner uint64, input Input) (ManagedView, error) {
	if err := input.Validate(a.now()); err != nil {
		return ManagedView{}, err
	}
	e := Event{OrganizerID: owner, Status: "draft", CreatedAt: a.now().UTC()}
	applyInput(&e, input)
	if err := a.db.WithContext(ctx).Create(&e).Error; err != nil {
		return ManagedView{}, err
	}
	return ManagedView{e.PublicView(a.now()), e.Status}, nil
}
func (a *AdminService) Update(ctx context.Context, owner, id uint64, input Input) (ManagedView, error) {
	if err := input.Validate(a.now()); err != nil {
		return ManagedView{}, err
	}
	var e Event
	err := a.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&e, id).Error; err != nil {
			return eventError(err)
		}
		if e.OrganizerID != owner {
			return apierror.Forbidden
		}
		if e.Status != "draft" {
			return apierror.New(409, "EVENT_ALREADY_PUBLISHED", "发布后的活动不能编辑")
		}
		applyInput(&e, input)
		return tx.Save(&e).Error
	})
	return ManagedView{e.PublicView(a.now()), e.Status}, err
}
func (a *AdminService) Publish(ctx context.Context, owner, id uint64) (ManagedView, error) {
	var e Event
	err := a.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&e, id).Error; err != nil {
			return eventError(err)
		}
		if e.OrganizerID != owner {
			return apierror.Forbidden
		}
		if e.Status == "published" {
			return nil
		}
		input := Input{e.Title, e.Description, e.Location, e.Capacity, e.RegistrationStartsAt, e.RegistrationEndsAt, e.StartsAt, e.EndsAt}
		if err := input.Validate(a.now()); err != nil {
			return err
		}
		e.Status = "published"
		return tx.Model(&e).Update("status", "published").Error
	})
	return ManagedView{e.PublicView(a.now()), e.Status}, err
}
func eventError(err error) error {
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return apierror.NotFound
	}
	return err
}
func applyInput(e *Event, i Input) {
	e.Title = i.Title
	e.Description = i.Description
	e.Location = i.Location
	e.Capacity = i.Capacity
	e.RegistrationStartsAt = i.RegistrationStartsAt
	e.RegistrationEndsAt = i.RegistrationEndsAt
	e.StartsAt = i.StartsAt
	e.EndsAt = i.EndsAt
}
