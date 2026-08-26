package service

import (
	"context"
	"testing"
	"time"
)

type capturingMonitorRunner struct {
	tasks []func(context.Context)
}

func (r *capturingMonitorRunner) Go(task func(context.Context)) bool {
	r.tasks = append(r.tasks, task)
	return true
}

func TestLongRunningMonitorsUseConfiguredBackgroundRunner(t *testing.T) {
	runner := &capturingMonitorRunner{}

	nodes := NewNodeService(nil, "test")
	nodes.SetBackgroundRunner(runner)
	nodes.StartOfflineMonitor(context.Background(), time.Hour)

	installTokens := NewInstallTokenService(nil, nil)
	installTokens.SetBackgroundRunner(runner)
	installTokens.StartGC(context.Background(), time.Hour)

	dashboard := NewDashboardService(nil, nil, nil)
	dashboard.SetBackgroundRunner(runner)
	dashboard.StartSLAMonitor(context.Background(), nil, time.Hour, time.Hour)

	versions := NewClusterVersionMonitor(nil, "test")
	versions.SetBackgroundRunner(runner)
	versions.Start(context.Background(), time.Hour, time.Hour)

	storageTargets := NewStorageTargetService(nil, nil, nil, nil)
	storageTargets.SetBackgroundRunner(runner)
	storageTargets.StartHealthMonitor(context.Background(), nil, time.Hour)

	if len(runner.tasks) != 5 {
		t.Fatalf("background runner received %d tasks, want 5", len(runner.tasks))
	}

	// The monitor must listen to the supervisor-provided context, not retain
	// the context passed to Start. Running one captured task is sufficient to
	// lock this ownership contract for the shared helper.
	runCtx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		runner.tasks[0](runCtx)
		close(done)
	}()
	cancel()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("monitor did not stop when background runner context was canceled")
	}
}

func TestBackgroundMonitorFallbackUsesCallerContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	started := make(chan struct{})
	done := make(chan struct{})
	if !startBackgroundMonitor(nil, ctx, func(runCtx context.Context) {
		close(started)
		<-runCtx.Done()
		close(done)
	}) {
		t.Fatal("fallback monitor was rejected")
	}
	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("fallback monitor did not start")
	}
	cancel()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("fallback monitor did not stop when caller context was canceled")
	}
}
