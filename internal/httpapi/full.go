package httpapi

import (
	"errors"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"campus-events/internal/account"
	"campus-events/internal/apierror"
	"campus-events/internal/event"
	"campus-events/internal/registration"
)

type FullServices struct {
	Account       *account.Service
	Events        *event.AdminService
	Registrations *registration.Service
}

func NewFullServices(db *gorm.DB, secret string, now func() time.Time) *FullServices {
	return &FullServices{account.NewService(db, secret, now), event.NewAdminService(db, now), registration.NewService(db, now)}
}
func respondError(c *gin.Context, err error, log *slog.Logger) {
	var business *apierror.Error
	if errors.As(err, &business) {
		event.WriteError(c, business.Status, business.Code, business.Message)
		return
	}
	log.Error("operation failed", "request_id", c.GetString("request_id"), "path", c.Request.URL.Path, "error", err)
	event.WriteError(c, 503, "SERVICE_UNAVAILABLE", "服务暂时不可用，请稍后重试")
}
func body(c *gin.Context, v any) bool {
	if err := c.ShouldBindJSON(v); err != nil {
		event.WriteError(c, 400, "INVALID_ARGUMENT", "请求内容格式不正确，请检查字段与时间格式")
		return false
	}
	return true
}
func principal(c *gin.Context) account.Principal { return c.MustGet("principal").(account.Principal) }
func positiveID(c *gin.Context, key string) (uint64, bool) {
	id, err := strconv.ParseUint(c.Param(key), 10, 64)
	if err != nil || id == 0 {
		event.WriteError(c, 400, "INVALID_ARGUMENT", "ID 必须为正整数")
		return 0, false
	}
	return id, true
}
func pagination(c *gin.Context) (uint64, int, bool) {
	limit := 20
	var before uint64
	query := c.Request.URL.Query()
	if v, ok := query["limit"]; ok {
		if len(v) != 1 {
			event.WriteError(c, 400, "INVALID_ARGUMENT", "limit 只能出现一次")
			return 0, 0, false
		}
		n, e := strconv.Atoi(v[0])
		if e != nil || n < 1 || n > 100 {
			event.WriteError(c, 400, "INVALID_ARGUMENT", "limit 范围为 1～100")
			return 0, 0, false
		}
		limit = n
	}
	if v, ok := query["before_id"]; ok {
		if len(v) != 1 {
			event.WriteError(c, 400, "INVALID_ARGUMENT", "before_id 只能出现一次")
			return 0, 0, false
		}
		n, e := strconv.ParseUint(v[0], 10, 64)
		if e != nil || n == 0 {
			event.WriteError(c, 400, "INVALID_ARGUMENT", "before_id 必须为正整数")
			return 0, 0, false
		}
		before = n
	}
	return before, limit, true
}
func authenticate(s *account.Service, log *slog.Logger, role string) gin.HandlerFunc {
	return func(c *gin.Context) {
		parts := strings.Fields(c.GetHeader("Authorization"))
		if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") {
			c.Abort()
			respondError(c, apierror.Unauthorized, log)
			return
		}
		p, err := s.Authenticate(c.Request.Context(), parts[1])
		if err != nil {
			c.Abort()
			respondError(c, err, log)
			return
		}
		if role != "" && p.User.Role != role {
			c.Abort()
			respondError(c, apierror.Forbidden, log)
			return
		}
		c.Set("principal", p)
		c.Next()
	}
}

type rateEntry struct {
	count int
	until time.Time
}

func loginLimit() gin.HandlerFunc {
	var mu sync.Mutex
	entries := map[string]rateEntry{}
	return func(c *gin.Context) {
		mu.Lock()
		now := time.Now()
		key := c.ClientIP()
		entry := entries[key]
		if !now.Before(entry.until) {
			entry = rateEntry{until: now.Add(time.Minute)}
		}
		if len(entries) >= 4096 {
			for k, v := range entries {
				if !now.Before(v.until) {
					delete(entries, k)
				}
			}
		}
		if entry.count >= 30 || (len(entries) >= 4096 && entries[key].until.IsZero()) {
			mu.Unlock()
			c.Header("Retry-After", "60")
			event.WriteError(c, 429, "RATE_LIMITED", "请求过于频繁，请稍后再试")
			c.Abort()
			return
		}
		entry.count++
		entries[key] = entry
		mu.Unlock()
		c.Next()
	}
}
func registerFull(router *gin.Engine, s *FullServices, log *slog.Logger) {
	if s == nil {
		readOnly := func(c *gin.Context) {
			event.WriteError(c, 503, "READ_ONLY_MODE", "当前为只读预览，请启动完整 MySQL 模式")
		}
		for _, path := range []string{"/auth/register", "/auth/login", "/auth/logout", "/events", "/events/:id/publish", "/events/:id/registrations", "/events/:id/registrations/:registration_id/check-in"} {
			router.POST("/api/v1"+path, readOnly)
		}
		for _, path := range []string{"/me", "/me/registrations", "/organizer/events", "/events/:id/registrations"} {
			router.GET("/api/v1"+path, readOnly)
		}
		router.PATCH("/api/v1/events/:id", readOnly)
		router.DELETE("/api/v1/events/:id/registrations/me", readOnly)
		return
	}
	limiter := loginLimit()
	for _, action := range []string{"register", "login"} {
		router.POST("/api/v1/auth/"+action, limiter, func(c *gin.Context) {
			var input struct {
				Username string `json:"username"`
				Password string `json:"password"`
			}
			if !body(c, &input) {
				return
			}
			if action == "register" {
				u, err := s.Account.Register(c.Request.Context(), input.Username, input.Password)
				if err != nil {
					respondError(c, err, log)
					return
				}
				c.JSON(201, u)
				return
			}
			result, err := s.Account.Login(c.Request.Context(), input.Username, input.Password)
			if err != nil {
				respondError(c, err, log)
				return
			}
			c.JSON(200, result)
		})
	}
	logged := router.Group("/api/v1", authenticate(s.Account, log, ""))
	logged.GET("/me", func(c *gin.Context) { c.JSON(200, principal(c).User) })
	logged.POST("/auth/logout", func(c *gin.Context) {
		if err := s.Account.Logout(c.Request.Context(), principal(c)); err != nil {
			respondError(c, err, log)
			return
		}
		c.Status(204)
	})
	student := router.Group("/api/v1", authenticate(s.Account, log, "student"))
	student.GET("/events/:id/registrations/me", func(c *gin.Context) {
		id, ok := positiveID(c, "id")
		if !ok {
			return
		}
		result, err := s.Registrations.GetMine(c.Request.Context(), id, principal(c).User.ID)
		if err != nil {
			respondError(c, err, log)
			return
		}
		c.JSON(200, result)
	})
	student.POST("/events/:id/registrations", func(c *gin.Context) {
		id, ok := positiveID(c, "id")
		if !ok {
			return
		}
		result, err := s.Registrations.Signup(c.Request.Context(), id, principal(c).User.ID)
		if err != nil {
			respondError(c, err, log)
			return
		}
		c.JSON(200, result)
	})
	student.DELETE("/events/:id/registrations/me", func(c *gin.Context) {
		id, ok := positiveID(c, "id")
		if !ok {
			return
		}
		result, err := s.Registrations.Cancel(c.Request.Context(), id, principal(c).User.ID)
		if err != nil {
			respondError(c, err, log)
			return
		}
		c.JSON(200, result)
	})
	student.GET("/me/registrations", func(c *gin.Context) {
		before, limit, ok := pagination(c)
		if !ok {
			return
		}
		items, err := s.Registrations.Mine(c.Request.Context(), principal(c).User.ID, before, limit+1)
		if err != nil {
			respondError(c, err, log)
			return
		}
		more := len(items) > limit
		var next *uint64
		if more {
			items = items[:limit]
			id := items[len(items)-1].ID
			next = &id
		}
		c.JSON(200, gin.H{"items": items, "has_more": more, "next_before_id": next})
	})
	organizer := router.Group("/api/v1", authenticate(s.Account, log, "organizer"))
	organizer.GET("/organizer/events", func(c *gin.Context) {
		before, limit, ok := pagination(c)
		if !ok {
			return
		}
		items, err := s.Events.List(c.Request.Context(), principal(c).User.ID, before, limit+1)
		if err != nil {
			respondError(c, err, log)
			return
		}
		more := len(items) > limit
		var next *uint64
		if more {
			items = items[:limit]
			id := items[len(items)-1].ID
			next = &id
		}
		c.JSON(200, gin.H{"items": items, "has_more": more, "next_before_id": next})
	})
	organizer.POST("/events", func(c *gin.Context) {
		var input event.Input
		if !body(c, &input) {
			return
		}
		result, err := s.Events.Create(c.Request.Context(), principal(c).User.ID, input)
		if err != nil {
			respondError(c, err, log)
			return
		}
		c.JSON(201, result)
	})
	organizer.PATCH("/events/:id", func(c *gin.Context) {
		id, ok := positiveID(c, "id")
		if !ok {
			return
		}
		var input event.Input
		if !body(c, &input) {
			return
		}
		result, err := s.Events.Update(c.Request.Context(), principal(c).User.ID, id, input)
		if err != nil {
			respondError(c, err, log)
			return
		}
		c.JSON(200, result)
	})
	organizer.POST("/events/:id/publish", func(c *gin.Context) {
		id, ok := positiveID(c, "id")
		if !ok {
			return
		}
		result, err := s.Events.Publish(c.Request.Context(), principal(c).User.ID, id)
		if err != nil {
			respondError(c, err, log)
			return
		}
		c.JSON(200, result)
	})
	organizer.GET("/events/:id/registrations", func(c *gin.Context) {
		id, ok := positiveID(c, "id")
		if !ok {
			return
		}
		before, limit, ok := pagination(c)
		if !ok {
			return
		}
		items, err := s.Registrations.Roster(c.Request.Context(), id, principal(c).User.ID, before, limit+1)
		if err != nil {
			respondError(c, err, log)
			return
		}
		more := len(items) > limit
		var next *uint64
		if more {
			items = items[:limit]
			cursor := items[len(items)-1].ID
			next = &cursor
		}
		c.JSON(200, gin.H{"items": items, "has_more": more, "next_before_id": next})
	})
	organizer.POST("/events/:id/registrations/:registration_id/check-in", func(c *gin.Context) {
		id, ok := positiveID(c, "id")
		if !ok {
			return
		}
		rid, ok := positiveID(c, "registration_id")
		if !ok {
			return
		}
		result, err := s.Registrations.CheckIn(c.Request.Context(), id, rid, principal(c).User.ID)
		if err != nil {
			respondError(c, err, log)
			return
		}
		c.JSON(http.StatusOK, result)
	})
}
