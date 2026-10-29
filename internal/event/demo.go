package event

import (
	"context"
	"sort"
	"time"
)

// DemoRepository is immutable after construction and does not persist writes.
type DemoRepository struct{ events []Event }

func NewDemoRepository(events []Event) *DemoRepository {
	copyOfEvents := append([]Event(nil), events...)
	sort.Slice(copyOfEvents, func(i, j int) bool { return copyOfEvents[i].ID > copyOfEvents[j].ID })
	return &DemoRepository{events: copyOfEvents}
}

func (r *DemoRepository) ListPublished(ctx context.Context, beforeID uint64, limit int) ([]Event, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	result := make([]Event, 0, limit)
	for _, e := range r.events {
		if e.Status == "published" && (beforeID == 0 || e.ID < beforeID) {
			result = append(result, e)
			if len(result) == limit {
				break
			}
		}
	}
	return result, nil
}

func (r *DemoRepository) GetPublished(ctx context.Context, id uint64) (Event, error) {
	if err := ctx.Err(); err != nil {
		return Event{}, err
	}
	for _, e := range r.events {
		if e.ID == id && e.Status == "published" {
			return e, nil
		}
	}
	return Event{}, ErrNotFound
}

// DemoEvents uses relative UTC times so a fresh demo always has useful examples.
func DemoEvents(now time.Time, organizerID uint64) []Event {
	now = now.UTC().Truncate(time.Second)
	return []Event{
		{
			ID: 1, OrganizerID: organizerID, Title: "Go 后端入门分享会",
			Description: "一起了解 HTTP 接口、数据库查询和后端开发的基本流程。",
			Location:    "教学楼 A101", Capacity: 30, Status: "published",
			RegistrationStartsAt: now.Add(-24 * time.Hour), RegistrationEndsAt: now.Add(24 * time.Hour),
			StartsAt: now.Add(48 * time.Hour), EndsAt: now.Add(50 * time.Hour), CreatedAt: now,
		},
		{
			ID: 2, OrganizerID: organizerID, Title: "校园项目交流工作坊",
			Description: "讨论自己的项目设计，练习解释一次请求如何到达数据库。",
			Location:    "图书馆研讨室", Capacity: 20, Status: "published",
			RegistrationStartsAt: now.Add(-48 * time.Hour), RegistrationEndsAt: now.Add(-2 * time.Hour),
			StartsAt: now.Add(-time.Hour), EndsAt: now.Add(2 * time.Hour), CreatedAt: now,
		},
		{
			ID: 3, OrganizerID: organizerID, Title: "尚未发布的活动草稿",
			Description: "用于验证公开接口不会暴露草稿。", Location: "待定",
			Capacity: 10, Status: "draft", RegistrationStartsAt: now.Add(time.Hour),
			RegistrationEndsAt: now.Add(24 * time.Hour), StartsAt: now.Add(48 * time.Hour),
			EndsAt: now.Add(50 * time.Hour), CreatedAt: now,
		},
	}
}
