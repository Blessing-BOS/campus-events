package event_test

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"campus-events/internal/event"
	"campus-events/internal/platform/config"
	"campus-events/internal/platform/database"
	"campus-events/migrations"
)

func TestMySQLRepositoryIntegration(t *testing.T) {
	if os.Getenv("CAMPUS_TEST_MYSQL") != "1" {
		t.Skip("set CAMPUS_TEST_MYSQL=1 and configure a dedicated campus_events_*_test database")
	}
	cfg, err := config.Load()
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(cfg.MySQL.Database, "campus_events_") || !strings.HasSuffix(cfg.MySQL.Database, "_test") {
		t.Fatal("integration tests require a dedicated campus_events_*_test database")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	db, err := database.Open(ctx, cfg.MySQL)
	if err != nil {
		t.Fatal(err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatal(err)
	}
	defer sqlDB.Close()
	for i := 0; i < 2; i++ {
		if err := migrations.ApplyInitial(ctx, db); err != nil {
			t.Fatalf("schema initialization pass %d: %v", i+1, err)
		}
	}
	username := fmt.Sprintf("test_organizer_%d", time.Now().UnixNano())
	if err := db.Exec("INSERT INTO users (username, password_hash, role, created_at) VALUES (?, ?, ?, ?)",
		username, "!test-only", "organizer", time.Now().UTC()).Error; err != nil {
		t.Fatal(err)
	}
	var organizerID uint64
	if err := db.Raw("SELECT id FROM users WHERE username = ?", username).Scan(&organizerID).Error; err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := db.Exec("DELETE FROM events WHERE organizer_id = ?", organizerID).Error; err != nil {
			t.Error(err)
		}
		if err := db.Exec("DELETE FROM users WHERE id = ?", organizerID).Error; err != nil {
			t.Error(err)
		}
	}()
	now := time.Now().UTC().Truncate(time.Second)
	fixtures := event.DemoEvents(now, organizerID)
	for i := range fixtures {
		fixtures[i].ID = 0
		if err := db.Create(&fixtures[i]).Error; err != nil {
			t.Fatal(err)
		}
	}
	repo := event.NewMySQLRepository(db)
	listed, err := repo.ListPublished(ctx, 0, 2)
	if err != nil || len(listed) != 2 || listed[0].ID != fixtures[1].ID || listed[1].ID != fixtures[0].ID {
		t.Fatalf("published ordering/filtering: %+v %v", listed, err)
	}
	next, err := repo.ListPublished(ctx, fixtures[1].ID, 1)
	if err != nil || len(next) != 1 || next[0].ID != fixtures[0].ID {
		t.Fatalf("exclusive cursor: %+v %v", next, err)
	}
	got, err := repo.GetPublished(ctx, fixtures[0].ID)
	if err != nil || got.Title != fixtures[0].Title || !got.StartsAt.Equal(fixtures[0].StartsAt) {
		t.Fatalf("detail and UTC round trip: %+v %v", got, err)
	}
	if _, err := repo.GetPublished(ctx, fixtures[2].ID); !errors.Is(err, event.ErrNotFound) {
		t.Fatalf("draft must not be visible: %v", err)
	}
	cancelledCtx, stop := context.WithCancel(context.Background())
	stop()
	if _, err := repo.ListPublished(cancelledCtx, 0, 2); !errors.Is(err, context.Canceled) {
		t.Fatalf("query must honor cancellation: %v", err)
	}
	// The initial username must be stored, not just used in a WHERE condition.
	if err := event.Seed(ctx, db, now); err != nil {
		t.Fatal(err)
	}
	var demoOrganizerCount int64
	if err := db.Table("users").Where("username = ? AND role = ?", "organizer_demo", "organizer").
		Count(&demoOrganizerCount).Error; err != nil || demoOrganizerCount != 1 {
		t.Fatalf("demo organizer not initialized correctly: count=%d error=%v", demoOrganizerCount, err)
	}
	var countBefore int64
	if err := db.Model(&event.Event{}).Count(&countBefore).Error; err != nil {
		t.Fatal(err)
	}
	if err := event.Seed(ctx, db, now.Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	var countAfter int64
	if err := db.Model(&event.Event{}).Count(&countAfter).Error; err != nil {
		t.Fatal(err)
	}
	if countBefore != countAfter {
		t.Fatalf("seed duplicated events: before=%d after=%d", countBefore, countAfter)
	}
}
