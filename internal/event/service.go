package event

import (
	"context"
	"fmt"
	"time"
)

type Service struct {
	repo  Repository
	now   func() time.Time
	cache EntityCache
}

type EntityCache interface {
	Get(context.Context, uint64) (Event, bool)
	Put(context.Context, Event)
}

func (s *Service) SetCache(cache EntityCache) { s.cache = cache }

func NewService(repo Repository, now func() time.Time) *Service {
	return &Service{repo: repo, now: now}
}

type ListResult struct {
	Items        []View  `json:"items"`
	HasMore      bool    `json:"has_more"`
	NextBeforeID *uint64 `json:"next_before_id"`
}

func (s *Service) List(ctx context.Context, beforeID uint64, limit int) (ListResult, error) {
	if limit < 1 || limit > 100 {
		return ListResult{}, fmt.Errorf("limit must be between 1 and 100")
	}
	// Fetch one extra record to decide whether there is another page.
	events, err := s.repo.ListPublished(ctx, beforeID, limit+1)
	if err != nil {
		return ListResult{}, err
	}
	result := ListResult{Items: make([]View, 0, limit), HasMore: len(events) > limit}
	if result.HasMore {
		events = events[:limit]
		cursor := events[len(events)-1].ID
		result.NextBeforeID = &cursor
	}
	now := s.now()
	for _, e := range events {
		result.Items = append(result.Items, e.PublicView(now))
	}
	return result, nil
}

func (s *Service) Get(ctx context.Context, id uint64) (View, error) {
	if s.cache != nil {
		if cached, ok := s.cache.Get(ctx, id); ok {
			if reader, ok := s.repo.(interface {
				GetAvailability(context.Context, uint64) (Event, error)
			}); ok {
				fresh, err := reader.GetAvailability(ctx, id)
				if err != nil {
					return View{}, err
				}
				cached.Capacity = fresh.Capacity
				cached.RegisteredCount = fresh.RegisteredCount
				return cached.PublicView(s.now()), nil
			}
		}
	}
	e, err := s.repo.GetPublished(ctx, id)
	if err != nil {
		return View{}, err
	}
	if s.cache != nil {
		s.cache.Put(ctx, e)
	}
	return e.PublicView(s.now()), nil
}
