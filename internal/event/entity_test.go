package event

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestPhaseBoundaries(t *testing.T) {
	now := time.Date(2026, 10, 29, 12, 0, 0, 0, time.UTC)
	e := DemoEvents(now, 1)[0]
	cases := []struct {
		name string
		at   time.Time
		want string
	}{
		{"before registration", e.RegistrationStartsAt.Add(-time.Nanosecond), "not_open"},
		{"registration starts", e.RegistrationStartsAt, "open"},
		{"registration ends", e.RegistrationEndsAt, "closed"},
		{"event starts", e.StartsAt, "ongoing"},
		{"event ends", e.EndsAt, "ended"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := e.PublicView(tc.at).Phase; got != tc.want {
				t.Fatalf("phase=%s, want %s", got, tc.want)
			}
		})
	}
}

func TestDemoRepositoryHonorsCancellationAndCopiesInput(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	repo := NewDemoRepository(nil)
	if _, err := repo.GetPublished(ctx, 1); !errors.Is(err, context.Canceled) {
		t.Fatalf("get ignored cancellation: %v", err)
	}
	if _, err := repo.ListPublished(ctx, 0, 20); !errors.Is(err, context.Canceled) {
		t.Fatalf("list ignored cancellation: %v", err)
	}
	input := DemoEvents(time.Now(), 1)
	repo = NewDemoRepository(input)
	input[0].Title = "mutated"
	stored, err := repo.GetPublished(context.Background(), 1)
	if err != nil || stored.Title == "mutated" {
		t.Fatalf("demo repository should own a copy: %+v %v", stored, err)
	}
}
