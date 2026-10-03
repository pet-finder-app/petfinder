package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/pet-finder-app/petfinder-api/internal/api"
	"github.com/pet-finder-app/petfinder-api/internal/auth"
	"github.com/pet-finder-app/petfinder-api/internal/config"
)

func main() {
	if err := run(); err != nil {
		slog.Error("api stopped", "error", err)
		os.Exit(1)
	}
}

func run() error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))
	slog.SetDefault(logger)

	rootCtx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	poolConfig, err := pgxpool.ParseConfig(cfg.DatabaseURL)
	if err != nil {
		return err
	}
	poolConfig.MaxConns = 20
	poolConfig.MinConns = 2
	poolConfig.MaxConnLifetime = time.Hour
	pool, err := pgxpool.NewWithConfig(rootCtx, poolConfig)
	if err != nil {
		return err
	}
	defer pool.Close()
	pingCtx, cancelPing := context.WithTimeout(rootCtx, 5*time.Second)
	defer cancelPing()
	if err := pool.Ping(pingCtx); err != nil {
		return err
	}

	tokens, err := auth.NewTokenManager(auth.TokenConfig{
		Secret: []byte(cfg.JWTSecret), Issuer: cfg.JWTIssuer, Audience: cfg.JWTAudience, AccessTTL: cfg.AccessTokenTTL,
	})
	if err != nil {
		return err
	}
	handler := api.NewServer(pool, tokens, cfg.AccessTokenTTL, cfg.RefreshTokenTTL).Handler()
	server := &http.Server{
		Addr: cfg.HTTPAddress, Handler: handler, ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout: 15 * time.Second, WriteTimeout: 30 * time.Second, IdleTimeout: 90 * time.Second,
	}

	serverErrors := make(chan error, 1)
	go func() {
		slog.Info("api listening", "address", cfg.HTTPAddress, "environment", cfg.Environment)
		serverErrors <- server.ListenAndServe()
	}()
	select {
	case err := <-serverErrors:
		if !errors.Is(err, http.ErrServerClosed) {
			return err
		}
	case <-rootCtx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), cfg.ShutdownTimeout)
		defer cancel()
		if err := server.Shutdown(shutdownCtx); err != nil {
			return err
		}
	}
	return nil
}
