package httpapi_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/gin-gonic/gin"
	"github.com/redis/go-redis/v9"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"

	"campus-events/internal/account"
	"campus-events/internal/event"
	"campus-events/internal/httpapi"
	rediscache "campus-events/internal/platform/cache"
	"campus-events/internal/platform/config"
	"campus-events/internal/platform/database"
	"campus-events/internal/registration"
	"campus-events/migrations"
)

func TestFullMySQLWorkflow(t *testing.T) {
	if os.Getenv("CAMPUS_TEST_MYSQL") != "1" {
		t.Skip("requires dedicated campus_events_*_test MySQL database")
	}
	cfg, err := config.Load()
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(cfg.MySQL.Database, "campus_events_") || !strings.HasSuffix(cfg.MySQL.Database, "_test") {
		t.Fatal("refusing non-test database")
	}
	ctx := context.Background()
	db, err := database.Open(ctx, cfg.MySQL)
	if err != nil {
		t.Fatal(err)
	}
	sqlDB, _ := db.DB()
	defer sqlDB.Close()
	if err := migrations.Apply(ctx, db); err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC().Truncate(time.Millisecond).Add(123 * time.Microsecond)
	clock := func() time.Time { return now }
	secret := "integration-secret-at-least-thirty-two-bytes"
	full := httpapi.NewFullServices(db, secret, clock)
	public := event.NewService(event.NewMySQLRepository(db), clock)
	gin.SetMode(gin.TestMode)
	router := httpapi.NewRouter(public, "mysql", func(context.Context) error { return nil }, slog.New(slog.NewTextHandler(io.Discard, nil)), full)
	call := func(method, path, token string, input any) *httptest.ResponseRecorder {
		var raw []byte
		if input != nil {
			raw, _ = json.Marshal(input)
		}
		req := httptest.NewRequest(method, path, bytes.NewReader(raw))
		if input != nil {
			req.Header.Set("Content-Type", "application/json")
		}
		if token != "" {
			req.Header.Set("Authorization", "Bearer "+token)
		}
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		return w
	}
	var users []uint64
	var events []uint64
	defer func() {
		if len(events) > 0 {
			db.Where("event_id IN ?", events).Delete(&registration.Registration{})
			db.Where("id IN ?", events).Delete(&event.Event{})
		}
		if len(users) > 0 {
			db.Where("user_id IN ?", users).Delete(&account.Session{})
			db.Where("id IN ?", users).Delete(&account.User{})
		}
	}()
	hash, _ := bcrypt.GenerateFromPassword([]byte("StudentTest123!"), bcrypt.DefaultCost)
	prefix := fmt.Sprintf("full_%d_", time.Now().UnixNano())
	makeUser := func(name, role string) (account.User, string) {
		u := account.User{Username: prefix + name, PasswordHash: string(hash), Role: role, CreatedAt: now}
		if err := db.Create(&u).Error; err != nil {
			t.Fatal(err)
		}
		users = append(users, u.ID)
		login, err := full.Account.Login(ctx, u.Username, "StudentTest123!")
		if err != nil {
			t.Fatal(err)
		}
		return u, login.AccessToken
	}
	owner, ownerToken := makeUser("o", "organizer")
	_, otherToken := makeUser("x", "organizer")
	students := make([]account.User, 20)
	tokens := make([]string, 20)
	for i := range students {
		students[i], tokens[i] = makeUser(fmt.Sprintf("s%d", i), "student")
	}
	if w := call("GET", "/api/v1/me", "", nil); w.Code != 401 {
		t.Fatalf("missing authentication: %d", w.Code)
	}
	if w := call("GET", "/api/v1/me", tokens[0]+"tampered", nil); w.Code != 401 {
		t.Fatalf("forged token accepted: %d", w.Code)
	}
	if w := call("POST", "/api/v1/auth/register", "", map[string]any{"username": "bypass_role", "password": "StudentTest123!", "role": "organizer"}); w.Code != 400 {
		t.Fatalf("client role was not rejected: %d %s", w.Code, w.Body.String())
	}
	registered := call("POST", "/api/v1/auth/register", "", map[string]string{"username": prefix + "new", "password": "StudentTest123!"})
	if registered.Code != 201 {
		t.Fatalf("register: %d %s", registered.Code, registered.Body.String())
	}
	var newUser account.User
	json.Unmarshal(registered.Body.Bytes(), &newUser)
	users = append(users, newUser.ID)
	if newUser.Role != "student" || strings.Contains(registered.Body.String(), "password") {
		t.Fatal("registration leaks secrets or grants organizer")
	}
	if w := call("POST", "/api/v1/auth/register", "", map[string]string{"username": prefix + "new", "password": "StudentTest123!"}); w.Code != 409 {
		t.Fatalf("duplicate username: %d", w.Code)
	}
	if w := call("POST", "/api/v1/auth/login", "", map[string]string{"username": prefix + "new", "password": "WrongPassword"}); w.Code != 401 {
		t.Fatalf("wrong password: %d", w.Code)
	}
	input := event.Input{Title: "并发报名集成测试", Location: "测试教室", Description: "验证名额与权限", Capacity: 5, RegistrationStartsAt: now.Add(-time.Hour), RegistrationEndsAt: now.Add(10 * time.Minute), StartsAt: now.Add(20 * time.Minute), EndsAt: now.Add(2 * time.Hour)}
	if w := call("POST", "/api/v1/events", tokens[0], input); w.Code != 403 {
		t.Fatalf("student created event: %d", w.Code)
	}
	created := call("POST", "/api/v1/events", ownerToken, input)
	if created.Code != 201 {
		t.Fatalf("create: %d %s", created.Code, created.Body.String())
	}
	var managed event.ManagedView
	json.Unmarshal(created.Body.Bytes(), &managed)
	id := managed.ID
	events = append(events, id)
	path := fmt.Sprintf("/api/v1/events/%d", id)
	if w := call("GET", path, "", nil); w.Code != 404 {
		t.Fatal("draft exposed")
	}
	if w := call("PATCH", path, otherToken, input); w.Code != 403 {
		t.Fatalf("cross-owner update: %d", w.Code)
	}
	if w := call("POST", path+"/publish", ownerToken, nil); w.Code != 200 {
		t.Fatalf("publish: %d %s", w.Code, w.Body.String())
	}
	if w := call("PATCH", path, ownerToken, input); w.Code != 409 {
		t.Fatalf("published event changed: %d", w.Code)
	}
	if w := call("GET", path+"/registrations", otherToken, nil); w.Code != 403 {
		t.Fatalf("private roster exposed: %d", w.Code)
	}
	if w := call("POST", path+"/registrations", ownerToken, nil); w.Code != 403 {
		t.Fatalf("organizer registered as student: %d", w.Code)
	}
	var wg sync.WaitGroup
	responses := make([]*httptest.ResponseRecorder, 20)
	for i := range tokens {
		wg.Add(1)
		go func(i int) { defer wg.Done(); responses[i] = call("POST", path+"/registrations", tokens[i], nil) }(i)
	}
	wg.Wait()
	winners := make([]int, 0)
	for i, w := range responses {
		if w.Code == 200 {
			winners = append(winners, i)
		} else if w.Code != 409 || !strings.Contains(w.Body.String(), "EVENT_FULL") {
			t.Fatalf("unexpected concurrent response: %d %s", w.Code, w.Body.String())
		}
	}
	if len(winners) != 5 {
		t.Fatalf("20 students / 5 seats: success=%d", len(winners))
	}
	checkCount := func(want int) {
		t.Helper()
		var e event.Event
		db.First(&e, id)
		var count int64
		db.Model(&registration.Registration{}).Where("event_id = ? AND status = ?", id, "active").Count(&count)
		if e.RegisteredCount != want || count != int64(want) {
			t.Fatalf("counter mismatch: stored=%d rows=%d want=%d", e.RegisteredCount, count, want)
		}
	}
	checkCount(5)
	if w := call("GET", path+"/registrations/me", tokens[winners[0]], nil); w.Code != 200 {
		t.Fatalf("own registration status: %d %s", w.Code, w.Body.String())
	}
	winner := winners[0]
	var original registration.Registration
	json.Unmarshal(responses[winner].Body.Bytes(), &original)
	for i := range responses {
		wg.Add(1)
		go func(i int) { defer wg.Done(); responses[i] = call("POST", path+"/registrations", tokens[winner], nil) }(i)
	}
	wg.Wait()
	for _, w := range responses {
		if w.Code != 200 {
			t.Fatalf("duplicate signup: %d", w.Code)
		}
	}
	checkCount(5)
	for i := range responses {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			responses[i] = call("DELETE", path+"/registrations/me", tokens[winner], nil)
		}(i)
	}
	wg.Wait()
	for _, w := range responses {
		if w.Code != 200 {
			t.Fatalf("duplicate cancellation: %d %s", w.Code, w.Body.String())
		}
	}
	checkCount(4)
	reregistered := call("POST", path+"/registrations", tokens[winner], nil)
	var again registration.Registration
	json.Unmarshal(reregistered.Body.Bytes(), &again)
	if reregistered.Code != 200 || again.ID != original.ID {
		t.Fatalf("re-registration did not reuse record: %s", reregistered.Body.String())
	}
	checkCount(5)
	// Actual SQL transaction rollback after INSERT, with an injected counter error.
	rollbackEvent := event.Event{OrganizerID: owner.ID, Title: "回滚验证", Location: "测试", Capacity: 2, Status: "published", RegistrationStartsAt: now.Add(-time.Hour), RegistrationEndsAt: now.Add(time.Hour), StartsAt: now.Add(2 * time.Hour), EndsAt: now.Add(3 * time.Hour), CreatedAt: now}
	db.Create(&rollbackEvent)
	events = append(events, rollbackEvent.ID)
	if err := db.Callback().Update().Before("gorm:update").Register("test:counter_failure", func(tx *gorm.DB) {
		if tx.Statement.Table == "events" {
			tx.AddError(errors.New("injected counter failure"))
		}
	}); err != nil {
		t.Fatal(err)
	}
	_, rollbackErr := full.Registrations.Signup(ctx, rollbackEvent.ID, students[0].ID)
	db.Callback().Update().Remove("test:counter_failure")
	if rollbackErr == nil {
		t.Fatal("injected failure not returned")
	}
	var rolledRows int64
	db.Model(&registration.Registration{}).Where("event_id = ?", rollbackEvent.ID).Count(&rolledRows)
	if rolledRows != 0 {
		t.Fatal("transaction retained registration after counter failure")
	}
	// Redis is only metadata: count is still read from actual MySQL on a cache hit.
	mr := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: mr.Addr(), MaxRetries: -1, DialTimeout: 100 * time.Millisecond, ReadTimeout: 100 * time.Millisecond, ContextTimeoutEnabled: true})
	defer client.Close()
	public.SetCache(rediscache.New(client))
	if _, err := public.Get(ctx, id); err != nil {
		t.Fatal(err)
	}
	if w := call("DELETE", path+"/registrations/me", tokens[winner], nil); w.Code != 200 {
		t.Fatal(w.Body.String())
	}
	detail, err := public.Get(ctx, id)
	if err != nil || detail.RegisteredCount != 4 {
		t.Fatalf("cached count became stale: %+v %v", detail, err)
	}
	if w := call("POST", path+"/registrations", tokens[winner], nil); w.Code != 200 {
		t.Fatal(w.Body.String())
	}
	mr.Close()
	detail, err = public.Get(ctx, id)
	if err != nil || detail.RegisteredCount != 5 {
		t.Fatalf("Redis outage did not fall back: %+v %v", detail, err)
	}
	if w := call("GET", "/api/v1/me/registrations?limit=1", tokens[winner], nil); w.Code != 200 || !strings.Contains(w.Body.String(), "event_title") {
		t.Fatalf("my registrations: %d %s", w.Code, w.Body.String())
	}
	checkPath := fmt.Sprintf("%s/registrations/%d/check-in", path, original.ID)
	if w := call("POST", checkPath, ownerToken, nil); w.Code != 409 {
		t.Fatalf("early check-in accepted: %d", w.Code)
	}
	now = now.Add(21 * time.Minute)
	if w := call("POST", checkPath, otherToken, nil); w.Code != 403 {
		t.Fatalf("cross-owner check-in: %d", w.Code)
	}
	firstCheck := call("POST", checkPath, ownerToken, nil)
	if firstCheck.Code != 200 {
		t.Fatalf("check-in: %d %s", firstCheck.Code, firstCheck.Body.String())
	}
	var checked registration.Registration
	json.Unmarshal(firstCheck.Body.Bytes(), &checked)
	secondCheck := call("POST", checkPath, ownerToken, nil)
	var twice registration.Registration
	json.Unmarshal(secondCheck.Body.Bytes(), &twice)
	if secondCheck.Code != 200 || checked.CheckedInAt == nil || twice.CheckedInAt == nil || !checked.CheckedInAt.Equal(*twice.CheckedInAt) {
		t.Fatal("check-in is not idempotent")
	}
	if w := call("DELETE", path+"/registrations/me", tokens[winner], nil); w.Code != 409 {
		t.Fatalf("late cancellation: %d", w.Code)
	}
	checkCount(5)
	if w := call("POST", "/api/v1/auth/logout", tokens[winner], nil); w.Code != 204 {
		t.Fatalf("logout: %d", w.Code)
	}
	if w := call("GET", "/api/v1/me", tokens[winner], nil); w.Code != 401 {
		t.Fatalf("revoked token still works: %d", w.Code)
	}
	now = now.Add(3 * time.Hour)
	if w := call("GET", "/api/v1/me", ownerToken, nil); w.Code != 401 {
		t.Fatalf("expired token accepted: %d", w.Code)
	}
	t.Log("20 distinct students / 5 seats: exactly 5 successes; duplicate signup/cancel, re-enrollment, rollback, permissions, check-in, session revocation and Redis fallback passed")
}
