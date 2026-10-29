package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/redis/go-redis/v9"

	"campus-events/internal/event"
	"campus-events/internal/httpapi"
	rediscache "campus-events/internal/platform/cache"
	"campus-events/internal/platform/config"
	"campus-events/internal/platform/database"

	"github.com/gin-gonic/gin"
)

func main() {
	log := slog.New(slog.NewTextHandler(os.Stdout, nil))
	if err := run(log); err != nil {
		log.Error("server stopped", "error", err)
		os.Exit(1)
	}
}

func run(log *slog.Logger) error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	var repo event.Repository
	var full *httpapi.FullServices
	ready := func(ctx context.Context) error { return ctx.Err() }
	if cfg.DataSource == "mysql" {
		if len(cfg.JWTSecret) < 32 {
			return fmt.Errorf("JWT_SECRET must contain at least 32 bytes in MySQL mode")
		}
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		db, err := database.Open(ctx, cfg.MySQL)
		cancel()
		if err != nil {
			return fmt.Errorf("MySQL connection failed (no automatic demo fallback): %w", err)
		}
		sqlDB, err := db.DB()
		if err != nil {
			return err
		}
		defer sqlDB.Close()
		repo = event.NewMySQLRepository(db)
		full = httpapi.NewFullServices(db, cfg.JWTSecret, time.Now)
		ready = func(ctx context.Context) error {
			// A ping alone would miss an uninitialized events table.
			for _, table := range []string{"users", "events", "registrations", "auth_sessions"} {
				if err := db.WithContext(ctx).Exec("SELECT id FROM " + table + " LIMIT 1").Error; err != nil {
					return err
				}
			}
			return nil
		}
	} else {
		repo = event.NewDemoRepository(event.DemoEvents(time.Now(), 1))
		log.Info("read-only demo mode: built-in examples, no database or persisted writes")
	}
	gin.SetMode(gin.ReleaseMode)
	service := event.NewService(repo, time.Now)
	if cfg.RedisAddr != "" && cfg.DataSource == "mysql" {
		client := redis.NewClient(&redis.Options{Addr: cfg.RedisAddr, Password: cfg.RedisPassword, DialTimeout: 200 * time.Millisecond, ReadTimeout: 200 * time.Millisecond, WriteTimeout: 200 * time.Millisecond, MaxRetries: -1, ContextTimeoutEnabled: true})
		defer client.Close()
		service.SetCache(rediscache.New(client))
		log.Info("optional metadata cache configured")
	}
	router := httpapi.NewRouter(service, cfg.DataSource, ready, log, full)
	server := &http.Server{
		Addr: cfg.Addr, Handler: router,
		ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 10 * time.Second,
		WriteTimeout: 10 * time.Second, IdleTimeout: 60 * time.Second,
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	serverError := make(chan error, 1)
	go func() { serverError <- server.ListenAndServe() }()
	log.Info("server starting", "address", cfg.Addr, "data_source", cfg.DataSource)
	select {
	case err := <-serverError:
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return err
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := server.Shutdown(shutdownCtx); err != nil {
			_ = server.Close()
			return err
		}
		return nil
	}
}
