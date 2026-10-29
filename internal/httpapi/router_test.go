package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"campus-events/internal/event"

	"github.com/gin-gonic/gin"
)

var testNow = time.Date(2026, 10, 29, 12, 0, 0, 0, time.UTC)

func testRouter(repo event.Repository, ready func(context.Context) error) http.Handler {
	gin.SetMode(gin.TestMode)
	if ready == nil {
		ready = func(context.Context) error { return nil }
	}
	return NewRouter(event.NewService(repo, func() time.Time { return testNow }), "demo", ready,
		slog.New(slog.NewTextHandler(io.Discard, nil)))
}

func request(router http.Handler, path string) *httptest.ResponseRecorder {
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, path, nil))
	return recorder
}

func TestPublicPaginationDoesNotExposeDrafts(t *testing.T) {
	router := testRouter(event.NewDemoRepository(event.DemoEvents(testNow, 1)), nil)
	first := request(router, "/api/v1/events?limit=1")
	if first.Code != http.StatusOK {
		t.Fatalf("first page: status=%d body=%s", first.Code, first.Body.String())
	}
	var page event.ListResult
	if err := json.Unmarshal(first.Body.Bytes(), &page); err != nil {
		t.Fatal(err)
	}
	if len(page.Items) != 1 || page.Items[0].ID != 2 || !page.HasMore || page.NextBeforeID == nil || *page.NextBeforeID != 2 {
		t.Fatalf("unexpected first page: %+v", page)
	}
	second := request(router, "/api/v1/events?limit=1&before_id=2")
	if err := json.Unmarshal(second.Body.Bytes(), &page); err != nil {
		t.Fatal(err)
	}
	if second.Code != http.StatusOK || len(page.Items) != 1 || page.Items[0].ID != 1 || page.HasMore || page.NextBeforeID != nil {
		t.Fatalf("unexpected second page: status=%d page=%+v", second.Code, page)
	}
	empty := request(router, "/api/v1/events?before_id=1")
	if empty.Code != http.StatusOK || !strings.Contains(empty.Body.String(), `"items":[]`) {
		t.Fatalf("empty page must contain an array: %d %s", empty.Code, empty.Body.String())
	}
}

func TestDetailsHideDraftsAndReturnPublicFields(t *testing.T) {
	router := testRouter(event.NewDemoRepository(event.DemoEvents(testNow, 1)), nil)
	for _, id := range []string{"3", "999"} {
		response := request(router, "/api/v1/events/"+id)
		if response.Code != http.StatusNotFound || !strings.Contains(response.Body.String(), "EVENT_NOT_FOUND") {
			t.Fatalf("draft or missing event %s: %d %s", id, response.Code, response.Body.String())
		}
	}
	response := request(router, "/api/v1/events/1")
	var detail event.View
	if err := json.Unmarshal(response.Body.Bytes(), &detail); err != nil {
		t.Fatal(err)
	}
	if response.Code != http.StatusOK || detail.Phase != "open" || detail.RemainingCapacity != 30 {
		t.Fatalf("unexpected detail: status=%d detail=%+v", response.Code, detail)
	}
	if strings.Contains(response.Body.String(), "password") {
		t.Fatal("public response contains private data")
	}
}

func TestInvalidQueryAndID(t *testing.T) {
	router := testRouter(event.NewDemoRepository(nil), nil)
	paths := []string{
		"/api/v1/events?limit=0", "/api/v1/events?limit=101", "/api/v1/events?limit=-1",
		"/api/v1/events?limit=abc", "/api/v1/events?limit=", "/api/v1/events?limit=1&limit=2",
		"/api/v1/events?before_id=0", "/api/v1/events?before_id=-1", "/api/v1/events?before_id=",
		"/api/v1/events?before_id=x", "/api/v1/events?before_id=18446744073709551616",
		"/api/v1/events?before_id=1&before_id=2", "/api/v1/events/0", "/api/v1/events/-1",
		"/api/v1/events/nope", "/api/v1/events/18446744073709551616",
	}
	for _, path := range paths {
		t.Run(path, func(t *testing.T) {
			response := request(router, path)
			if response.Code != http.StatusBadRequest || !strings.Contains(response.Body.String(), "INVALID_ARGUMENT") {
				t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
			}
		})
	}
}

type unavailableRepository struct{}

func (unavailableRepository) ListPublished(context.Context, uint64, int) ([]event.Event, error) {
	return nil, errors.New("SELECT secret_database: internal connection failure")
}

func (unavailableRepository) GetPublished(context.Context, uint64) (event.Event, error) {
	return event.Event{}, errors.New("SELECT secret_database: internal connection failure")
}

func TestDatabaseErrorsAreNotExposed(t *testing.T) {
	router := testRouter(unavailableRepository{}, nil)
	for _, path := range []string{"/api/v1/events", "/api/v1/events/1"} {
		response := request(router, path)
		if response.Code != http.StatusServiceUnavailable || !strings.Contains(response.Body.String(), "SERVICE_UNAVAILABLE") {
			t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
		}
		if strings.Contains(response.Body.String(), "SELECT") || strings.Contains(response.Body.String(), "secret_database") {
			t.Fatal("internal error leaked to HTTP response")
		}
	}
}

func TestLivenessAndReadinessAreDifferent(t *testing.T) {
	router := testRouter(event.NewDemoRepository(nil), func(context.Context) error { return errors.New("database offline") })
	if got := request(router, "/healthz"); got.Code != http.StatusOK {
		t.Fatalf("liveness should not depend on database: %d", got.Code)
	}
	if got := request(router, "/readyz"); got.Code != http.StatusServiceUnavailable {
		t.Fatalf("readiness should fail when database is unavailable: %d", got.Code)
	}
	readyRouter := testRouter(event.NewDemoRepository(nil), nil)
	if got := request(readyRouter, "/readyz"); got.Code != http.StatusOK {
		t.Fatalf("healthy readiness: %d", got.Code)
	}
	if got := request(router, "/unknown"); got.Code != http.StatusNotFound || !strings.Contains(got.Body.String(), "ROUTE_NOT_FOUND") {
		t.Fatalf("unknown route: %d %s", got.Code, got.Body.String())
	}
}
