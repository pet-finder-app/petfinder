package config

import (
	"strings"
	"testing"
	"time"
)

func TestLoad(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://test")
	t.Setenv("JWT_SECRET", strings.Repeat("s", 32))
	t.Setenv("ACCESS_TOKEN_TTL", "20m")

	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.AccessTokenTTL != 20*time.Minute {
		t.Fatalf("got %s", cfg.AccessTokenTTL)
	}
	if cfg.HTTPAddress != ":8080" {
		t.Fatalf("got %q", cfg.HTTPAddress)
	}
}

func TestLoadRejectsMissingSecrets(t *testing.T) {
	t.Setenv("DATABASE_URL", "")
	t.Setenv("JWT_SECRET", "short")
	if _, err := Load(); err == nil {
		t.Fatal("expected an error")
	}
}
