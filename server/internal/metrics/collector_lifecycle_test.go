package metrics

import (
	"context"
	"testing"
	"time"

	"backupx/server/internal/model"
	"backupx/server/internal/repository"
)

type lifecycleSampleSource struct{}

func (lifecycleSampleSource) ListStorageTargets(context.Context) ([]model.StorageTarget, error) {
	return nil, nil
}

func (lifecycleSampleSource) StorageUsage(context.Context) ([]repository.BackupStorageUsageItem, error) {
	return nil, nil
}

func (lifecycleSampleSource) ListNodes(context.Context) ([]model.Node, error) {
	return nil, nil
}

func (lifecycleSampleSource) AgentQueueSummaries(context.Context) (map[uint]repository.AgentCommandQueueSummary, error) {
	return nil, nil
}

func (lifecycleSampleSource) CountSLABreach(context.Context) (int, error) {
	return 0, nil
}

type capturingCollectorRunner struct {
	task func(context.Context)
}

func (r *capturingCollectorRunner) Go(task func(context.Context)) bool {
	r.task = task
	return true
}

func TestCollectorUsesConfiguredBackgroundRunner(t *testing.T) {
	runner := &capturingCollectorRunner{}
	collector := NewCollector(New("test"), lifecycleSampleSource{}, time.Hour)
	collector.SetBackgroundRunner(runner)
	collector.Start(context.Background())
	if runner.task == nil {
		t.Fatal("collector did not register with the background runner")
	}

	runCtx, cancel := context.WithCancel(context.Background())
	cancel()
	done := make(chan struct{})
	go func() {
		runner.task(runCtx)
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("collector did not stop when background runner context was canceled")
	}
}
