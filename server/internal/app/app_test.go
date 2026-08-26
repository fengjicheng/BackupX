package app

import (
	"context"
	"errors"
	"path/filepath"
	"testing"

	"backupx/server/internal/lifecycle"
	"github.com/glebarez/sqlite"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

func TestApplicationCloseStopsBackgroundAndClosesDatabaseOnce(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "app.db")), &gorm.Config{})
	if err != nil {
		t.Fatalf("open database: %v", err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatalf("get database handle: %v", err)
	}

	background := lifecycle.NewSupervisor(context.Background())
	started := make(chan struct{})
	finished := make(chan struct{})
	if !background.Go(func(ctx context.Context) {
		close(started)
		<-ctx.Done()
		close(finished)
	}) {
		t.Fatal("expected background task to be accepted")
	}
	<-started

	application := &Application{db: db, logger: zap.NewNop(), background: background}
	application.Close()
	application.Close()

	select {
	case <-finished:
	default:
		t.Fatal("Close returned before the background task exited")
	}
	if err := sqlDB.Ping(); err == nil {
		t.Fatal("database remained open after Close")
	}
}

func TestApplicationShutdownCanRetryAfterWaitTimeout(t *testing.T) {
	background := lifecycle.NewSupervisor(context.Background())
	release := make(chan struct{})
	if !background.Go(func(context.Context) { <-release }) {
		t.Fatal("expected background task to be accepted")
	}
	application := &Application{logger: zap.NewNop(), background: background}

	waitCtx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := application.Shutdown(waitCtx); !errors.Is(err, context.Canceled) {
		t.Fatalf("first Shutdown error = %v, want context.Canceled", err)
	}
	close(release)
	if err := application.Shutdown(context.Background()); err != nil {
		t.Fatalf("second Shutdown returned error: %v", err)
	}
}
