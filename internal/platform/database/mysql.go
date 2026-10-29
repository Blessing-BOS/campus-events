package database

import (
	"context"
	"net"
	"strconv"
	"time"

	"campus-events/internal/platform/config"

	driver "github.com/go-sql-driver/mysql"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func Open(ctx context.Context, c config.MySQL) (*gorm.DB, error) {
	dsn := driver.NewConfig()
	dsn.User = c.User
	dsn.Passwd = c.Password
	dsn.Net = "tcp"
	dsn.Addr = net.JoinHostPort(c.Host, strconv.Itoa(c.Port))
	dsn.DBName = c.Database
	dsn.ParseTime = true
	dsn.Loc = time.UTC
	dsn.Timeout = 3 * time.Second
	dsn.ReadTimeout = 5 * time.Second
	dsn.WriteTimeout = 5 * time.Second
	dsn.Params = map[string]string{"charset": "utf8mb4", "time_zone": "'+00:00'"}

	db, err := gorm.Open(mysql.New(mysql.Config{
		DSN:                       dsn.FormatDSN(),
		SkipInitializeWithVersion: true,
	}), &gorm.Config{DisableAutomaticPing: true, Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		return nil, err
	}
	sqlDB, err := db.DB()
	if err != nil {
		return nil, err
	}
	sqlDB.SetMaxOpenConns(20)
	sqlDB.SetMaxIdleConns(5)
	sqlDB.SetConnMaxLifetime(30 * time.Minute)
	if err := sqlDB.PingContext(ctx); err != nil {
		_ = sqlDB.Close()
		return nil, err
	}
	return db, nil
}
