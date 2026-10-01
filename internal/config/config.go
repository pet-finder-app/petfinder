package config

import (
	"errors"
	"fmt"
	"os"
	"strconv"
	"time"
)

type Config struct {
	Environment     string
	HTTPAddress     string
	DatabaseURL     string
	JWTSecret       string
	JWTIssuer       string
	JWTAudience     string
	AccessTokenTTL  time.Duration
	RefreshTokenTTL time.Duration
	ShutdownTimeout time.Duration
}

func Load() (Config, error) {
	cfg := Config{
		Environment:     value("APP_ENV", "development"),
		HTTPAddress:     value("HTTP_ADDR", ":8080"),
		DatabaseURL:     os.Getenv("DATABASE_URL"),
		JWTSecret:       os.Getenv("JWT_SECRET"),
		JWTIssuer:       value("JWT_ISSUER", "petfinder-api"),
		JWTAudience:     value("JWT_AUDIENCE", "petfinder"),
		AccessTokenTTL:  duration("ACCESS_TOKEN_TTL", 15*time.Minute),
		RefreshTokenTTL: duration("REFRESH_TOKEN_TTL", 30*24*time.Hour),
		ShutdownTimeout: duration("SHUTDOWN_TIMEOUT", 10*time.Second),
	}

	var missing []string
	if cfg.DatabaseURL == "" {
		missing = append(missing, "DATABASE_URL")
	}
	if len(cfg.JWTSecret) < 32 {
		missing = append(missing, "JWT_SECRET (at least 32 bytes)")
	}
	if len(missing) > 0 {
		return Config{}, fmt.Errorf("invalid configuration: %v", missing)
	}
	return cfg, nil
}

func value(name, fallback string) string {
	if v := os.Getenv(name); v != "" {
		return v
	}
	return fallback
}

func duration(name string, fallback time.Duration) time.Duration {
	v := os.Getenv(name)
	if v == "" {
		return fallback
	}
	parsed, err := time.ParseDuration(v)
	if err != nil || parsed <= 0 {
		return fallback
	}
	return parsed
}

func Bool(name string, fallback bool) (bool, error) {
	v := os.Getenv(name)
	if v == "" {
		return fallback, nil
	}
	parsed, err := strconv.ParseBool(v)
	if err != nil {
		return false, errors.New(name + " must be a boolean")
	}
	return parsed, nil
}
