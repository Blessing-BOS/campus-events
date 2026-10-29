package event

import "time"

// Event is the stored business record. Public responses use View below.
type Event struct {
	ID                   uint64 `gorm:"primaryKey;autoIncrement"`
	OrganizerID          uint64
	Title                string
	Description          string
	Location             string
	Capacity             int
	RegisteredCount      int
	Status               string
	RegistrationStartsAt time.Time
	RegistrationEndsAt   time.Time
	StartsAt             time.Time
	EndsAt               time.Time
	CreatedAt            time.Time
}

type View struct {
	ID                   uint64    `json:"id"`
	OrganizerID          uint64    `json:"organizer_id"`
	Title                string    `json:"title"`
	Description          string    `json:"description"`
	Location             string    `json:"location"`
	Capacity             int       `json:"capacity"`
	RegisteredCount      int       `json:"registered_count"`
	RemainingCapacity    int       `json:"remaining_capacity"`
	Phase                string    `json:"phase"`
	RegistrationStartsAt time.Time `json:"registration_starts_at"`
	RegistrationEndsAt   time.Time `json:"registration_ends_at"`
	StartsAt             time.Time `json:"starts_at"`
	EndsAt               time.Time `json:"ends_at"`
	CreatedAt            time.Time `json:"created_at"`
}

func (e Event) PublicView(now time.Time) View {
	return View{
		ID: e.ID, OrganizerID: e.OrganizerID, Title: e.Title,
		Description: e.Description, Location: e.Location,
		Capacity: e.Capacity, RegisteredCount: e.RegisteredCount,
		RemainingCapacity: e.Capacity - e.RegisteredCount,
		Phase:             e.phase(now), RegistrationStartsAt: e.RegistrationStartsAt,
		RegistrationEndsAt: e.RegistrationEndsAt, StartsAt: e.StartsAt,
		EndsAt: e.EndsAt, CreatedAt: e.CreatedAt,
	}
}

func (e Event) phase(now time.Time) string {
	switch {
	case !now.Before(e.EndsAt):
		return "ended"
	case !now.Before(e.StartsAt):
		return "ongoing"
	case !now.Before(e.RegistrationEndsAt):
		return "closed"
	case !now.Before(e.RegistrationStartsAt):
		return "open"
	default:
		return "not_open"
	}
}
