package httpapi

import (
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestAuthenticationRateLimit(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.GET("/limited", loginLimit(), func(c *gin.Context) { c.Status(200) })
	for i := 0; i < 31; i++ {
		req := httptest.NewRequest("GET", "/limited", nil)
		req.RemoteAddr = "192.0.2.10:12345"
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		want := 200
		if i == 30 {
			want = 429
		}
		if w.Code != want {
			t.Fatalf("request %d: status=%d want=%d", i+1, w.Code, want)
		}
	}
	req := httptest.NewRequest("GET", "/limited", nil)
	req.RemoteAddr = "192.0.2.11:12345"
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != 200 {
		t.Fatalf("different client incorrectly limited: %d", w.Code)
	}
}
