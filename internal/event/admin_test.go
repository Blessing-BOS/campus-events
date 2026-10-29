package event

import (
	"testing"
	"time"
)

func TestEventTimeValidationUsesDatabasePrecision(t *testing.T) {
	now := time.Date(2026, 10, 29, 12, 0, 0, 0, time.UTC)
	input := Input{Title: "时间边界", Location: "教室", Capacity: 5, RegistrationStartsAt: now.Add(time.Hour), RegistrationEndsAt: now.Add(time.Hour + 100*time.Microsecond), StartsAt: now.Add(2 * time.Hour), EndsAt: now.Add(3 * time.Hour)}
	if input.Validate(now) == nil {
		t.Fatal("sub-millisecond registration window collapses in DATETIME(3) and must be rejected")
	}
	input.RegistrationEndsAt = now.Add(time.Hour + time.Second)
	if err := input.Validate(now); err != nil {
		t.Fatal(err)
	}
}
