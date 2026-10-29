package cache

import (
	"context"
	"encoding/json"
	"strconv"
	"time"

	"github.com/redis/go-redis/v9"

	"campus-events/internal/event"
)

type Redis struct{ client *redis.Client }

func New(client *redis.Client) *Redis { return &Redis{client} }
func key(id uint64) string            { return "campus:events:meta:" + strconv.FormatUint(id, 10) }
func (r *Redis) Get(ctx context.Context, id uint64) (event.Event, bool) {
	ctx, cancel := context.WithTimeout(ctx, 200*time.Millisecond)
	defer cancel()
	data, err := r.client.Get(ctx, key(id)).Bytes()
	if err != nil {
		return event.Event{}, false
	}
	var e event.Event
	if json.Unmarshal(data, &e) != nil || e.ID != id || e.Status != "published" {
		return event.Event{}, false
	}
	return e, true
}
func (r *Redis) Put(ctx context.Context, e event.Event) {
	data, err := json.Marshal(e)
	if err != nil {
		return
	}
	ctx, cancel := context.WithTimeout(ctx, 200*time.Millisecond)
	defer cancel()
	_ = r.client.Set(ctx, key(e.ID), data, 60*time.Second).Err()
}
