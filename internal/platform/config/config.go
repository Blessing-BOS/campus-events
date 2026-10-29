package config

import (
	"fmt"
	"net"
	"os"
	"strconv"
)

type Config struct {
	Addr          string
	DataSource    string
	MySQL         MySQL
	JWTSecret     string
	RedisAddr     string
	RedisPassword string
}

type MySQL struct {
	Host     string
	Port     int
	User     string
	Password string
	Database string
}

func Load() (Config, error) {
	c := Config{
		Addr:       env("APP_ADDR", "127.0.0.1:8080"),
		DataSource: env("APP_DATA_SOURCE", "demo"),
		JWTSecret:  os.Getenv("JWT_SECRET"), RedisAddr: os.Getenv("REDIS_ADDR"), RedisPassword: os.Getenv("REDIS_PASSWORD"),
		MySQL: MySQL{
			Host:     env("MYSQL_HOST", "127.0.0.1"),
			User:     env("MYSQL_USER", "campus"),
			Password: os.Getenv("MYSQL_PASSWORD"),
			Database: env("MYSQL_DATABASE", "campus_events_dev"),
		},
	}
	if c.DataSource != "demo" && c.DataSource != "mysql" {
		return Config{}, fmt.Errorf("APP_DATA_SOURCE must be demo or mysql")
	}
	if _, _, err := net.SplitHostPort(c.Addr); err != nil {
		return Config{}, fmt.Errorf("APP_ADDR must be host:port: %w", err)
	}
	port, err := strconv.Atoi(env("MYSQL_PORT", "3306"))
	if err != nil || port < 1 || port > 65535 {
		return Config{}, fmt.Errorf("MYSQL_PORT must be between 1 and 65535")
	}
	c.MySQL.Port = port
	return c, nil
}

func env(name, fallback string) string {
	if value := os.Getenv(name); value != "" {
		return value
	}
	return fallback
}
