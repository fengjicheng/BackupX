package service

import (
	"context"
	"net/http"
	"time"

	"backupx/server/internal/apperror"
)

// BackgroundRunner is the narrow lifecycle dependency used by asynchronous
// services. lifecycle.Supervisor implements it at the application boundary.
type BackgroundRunner interface {
	Go(func(context.Context)) bool
}

func runDetached(task func(context.Context)) bool {
	if task == nil {
		return false
	}
	go task(context.Background())
	return true
}

// startBackgroundMonitor keeps the legacy caller-owned context when no
// lifecycle runner is configured, while allowing the application supervisor
// to own and wait for long-running monitors in production.
func startBackgroundMonitor(runner BackgroundRunner, fallbackCtx context.Context, task func(context.Context)) bool {
	if task == nil {
		return false
	}
	if runner != nil {
		return runner.Go(task)
	}
	if fallbackCtx == nil {
		fallbackCtx = context.Background()
	}
	go task(fallbackCtx)
	return true
}

func backgroundTaskUnavailable(code string) *apperror.AppError {
	return apperror.New(http.StatusServiceUnavailable, code, "服务正在关闭，无法启动新的后台任务", context.Canceled)
}

// finalizationContext lets a canceled task persist its terminal state. It is
// intentionally short-lived so shutdown cannot wait forever on cleanup I/O.
func finalizationContext(ctx context.Context) (context.Context, context.CancelFunc) {
	if ctx == nil {
		ctx = context.Background()
	}
	return context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
}

func acquireBackgroundSlot(ctx context.Context, semaphore chan struct{}) bool {
	select {
	case semaphore <- struct{}{}:
		return true
	case <-ctx.Done():
		return false
	}
}
