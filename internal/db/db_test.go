package db_test

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgconn"

	"github.com/HuyDoan-2112/handle-massive-requests/internal/db"
)

func TestNewPoolRejectsInvalidDSN(t *testing.T) {
	pool, err := db.NewPool(t.Context(), "postgres://%zz")
	if pool != nil {
		t.Cleanup(pool.Close)
		t.Error("NewPool() returned a pool for an invalid DSN")
	}
	var parseErr *pgconn.ParseConfigError
	if !errors.As(err, &parseErr) {
		t.Errorf("NewPool() error = %v, want *pgconn.ParseConfigError", err)
	}
}

func TestNewPoolRejectsCanceledContext(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	pool, err := db.NewPool(ctx, "postgres://localhost/test")
	if pool != nil {
		t.Cleanup(pool.Close)
		t.Error("NewPool() returned a pool for a canceled context")
	}
	if !errors.Is(err, context.Canceled) {
		t.Errorf("NewPool() error = %v, want context.Canceled", err)
	}
}

func TestNewPoolLifecycle(t *testing.T) {
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("set TEST_DATABASE_URL to run PostgreSQL integration tests")
	}
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()

	pool, err := db.NewPool(ctx, dsn)
	if err != nil {
		t.Fatalf("NewPool() error = %v", err)
	}
	t.Cleanup(pool.Close)

	var got int
	if err := pool.QueryRow(ctx, "SELECT 1").Scan(&got); err != nil {
		t.Fatalf("query through pool: %v", err)
	}
	if got != 1 {
		t.Errorf("SELECT 1 = %d, want 1", got)
	}

	pool.Close()
	if err := pool.Ping(ctx); err == nil {
		t.Error("closed pool accepted a ping")
	}
}
