package main

import (
	"errors"
	"testing"

	"github.com/golang-migrate/migrate/v4"
	"go.uber.org/zap"
	"go.uber.org/zap/zaptest/observer"
)

func TestHandleMigrateUpResult_NoChange(t *testing.T) {
	core, logs := observer.New(zap.InfoLevel)
	logger := zap.New(core)

	noChange, err := handleMigrateUpResult(logger, migrate.ErrNoChange)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if !noChange {
		t.Fatalf("expected noChange=true")
	}

	entries := logs.All()
	if len(entries) != 1 {
		t.Fatalf("expected 1 log entry, got %d", len(entries))
	}
	if entries[0].Message != "migrations: no changes required" {
		t.Fatalf("unexpected log message: %q", entries[0].Message)
	}
}

func TestHandleMigrateUpResult_Dirty(t *testing.T) {
	core, logs := observer.New(zap.ErrorLevel)
	logger := zap.New(core)

	noChange, err := handleMigrateUpResult(logger, migrate.ErrDirty{Version: 7})
	if err == nil {
		t.Fatalf("expected error")
	}
	if noChange {
		t.Fatalf("expected noChange=false")
	}
	if !errors.Is(err, migrate.ErrDirty{Version: 7}) {
		t.Fatalf("expected wrapped dirty error, got %v", err)
	}

	entries := logs.All()
	if len(entries) != 1 {
		t.Fatalf("expected 1 log entry, got %d", len(entries))
	}
	if entries[0].Message != "migrations failed: dirty state" {
		t.Fatalf("unexpected log message: %q", entries[0].Message)
	}
}

func TestHandleMigrateUpResult_GenericError(t *testing.T) {
	core, logs := observer.New(zap.ErrorLevel)
	logger := zap.New(core)
	upErr := errors.New("boom")

	noChange, err := handleMigrateUpResult(logger, upErr)
	if err == nil {
		t.Fatalf("expected error")
	}
	if noChange {
		t.Fatalf("expected noChange=false")
	}
	if !errors.Is(err, upErr) {
		t.Fatalf("expected wrapped original error, got %v", err)
	}

	entries := logs.All()
	if len(entries) != 1 {
		t.Fatalf("expected 1 log entry, got %d", len(entries))
	}
	if entries[0].Message != "migrations failed: up" {
		t.Fatalf("unexpected log message: %q", entries[0].Message)
	}
}
