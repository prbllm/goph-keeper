package postgres_test

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/prbllm/goph-keeper/internal/server/storage/postgres"
)

func TestOpenPing_unreachable(t *testing.T) {
	ctx := context.Background()
	dsn := "postgres://goph:goph@127.0.0.1:1/postgres?sslmode=disable&connect_timeout=1"

	start := time.Now()
	_, err := postgres.OpenPing(ctx, dsn)
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "ping") {
		t.Errorf("error should mention ping, got: %v", err)
	}
	if time.Since(start) > 5*time.Second {
		t.Errorf("expected fast failure, took %s", time.Since(start))
	}
}

func TestOpenPing_integration(t *testing.T) {
	dsn := os.Getenv("GOPHKEEPER_DATABASE_URL")
	if dsn == "" {
		t.Skip("GOPHKEEPER_DATABASE_URL not set, skipping integration test")
	}

	ctx := context.Background()
	pool, err := postgres.OpenPing(ctx, dsn)
	if err != nil {
		t.Fatalf("OpenPing: %v", err)
	}
	t.Cleanup(func() { _ = pool.Close() })

	if err := pool.PingContext(ctx); err != nil {
		t.Fatalf("PingContext: %v", err)
	}
}
