package httpapi

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"log/slog"
	"net/http"
	"time"

	"campus-events/internal/event"
	"campus-events/web"

	"github.com/gin-gonic/gin"
)

func NewRouter(service *event.Service, source string, ready func(context.Context) error, log *slog.Logger, full ...*FullServices) *gin.Engine {
	router := gin.New()
	gin.EnableJsonDecoderDisallowUnknownFields()
	router.SetTrustedProxies(nil)
	router.Use(func(c *gin.Context) {
		var requestBytes [16]byte
		if _, err := rand.Read(requestBytes[:]); err != nil {
			c.AbortWithStatus(500)
			return
		}
		requestID := hex.EncodeToString(requestBytes[:])
		c.Set("request_id", requestID)
		c.Header("X-Request-ID", requestID)
		c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 1<<20)
		c.Header("X-Content-Type-Options", "nosniff")
		c.Header("X-Frame-Options", "DENY")
		c.Header("Referrer-Policy", "strict-origin-when-cross-origin")
		c.Header("Content-Security-Policy", "default-src 'self'; script-src 'self'; style-src 'self'; img-src 'self' data:; base-uri 'none'; frame-ancestors 'none'; form-action 'self'")
		c.Header("Cache-Control", "no-store")
		started := time.Now()
		c.Next()
		log.Info("http request", "request_id", requestID, "method", c.Request.Method, "path", c.Request.URL.Path,
			"status", c.Writer.Status(), "duration_ms", time.Since(started).Milliseconds())
	})
	router.Use(gin.CustomRecovery(func(c *gin.Context, recovered any) {
		log.Error("request panicked", "panic", recovered)
		c.Abort()
		event.WriteError(c, http.StatusInternalServerError, "INTERNAL_ERROR", "internal server error")
	}))
	router.GET("/healthz", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "ok", "data_source": source})
	})
	router.GET("/readyz", func(c *gin.Context) {
		ctx, cancel := context.WithTimeout(c.Request.Context(), time.Second)
		defer cancel()
		if err := ready(ctx); err != nil {
			log.Error("readiness check failed", "error", err)
			event.WriteError(c, http.StatusServiceUnavailable, "NOT_READY", "data source is not ready")
			return
		}
		c.JSON(http.StatusOK, gin.H{"status": "ready", "data_source": source})
	})
	handler := event.NewHandler(service, log)
	router.GET("/api/v1/events", handler.List)
	router.GET("/api/v1/events/:id", handler.Get)
	var services *FullServices
	if len(full) > 0 {
		services = full[0]
	}
	registerFull(router, services, log)
	router.GET("/api/v1/meta", func(c *gin.Context) {
		c.JSON(200, gin.H{"name": "Campus Events", "mode": source, "writable": services != nil})
	})
	router.GET("/", func(c *gin.Context) {
		data, _ := web.Files.ReadFile("index.html")
		c.Data(200, "text/html; charset=utf-8", data)
	})
	for _, file := range []struct{ path, mime string }{{"app.js", "text/javascript; charset=utf-8"}, {"style.css", "text/css; charset=utf-8"}} {
		router.GET("/"+file.path, func(c *gin.Context) { data, _ := web.Files.ReadFile(file.path); c.Data(200, file.mime, data) })
	}
	router.NoRoute(func(c *gin.Context) {
		event.WriteError(c, http.StatusNotFound, "ROUTE_NOT_FOUND", "route not found")
	})
	return router
}
