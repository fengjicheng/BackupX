package agent

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"go.uber.org/zap"
)

func TestSubmitCommandResultRetriesWithCanceledCommandContext(t *testing.T) {
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		if requests.Add(1) < 3 {
			http.Error(w, "temporarily unavailable", http.StatusServiceUnavailable)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	agent := &Agent{
		client: NewMasterClient(server.URL, "token", false),
		logger: zap.NewNop(),
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	agent.submitCommandResult(ctx, 17, false, "backup failed", nil)

	if got := requests.Load(); got != 3 {
		t.Fatalf("submit requests = %d, want 3", got)
	}
}
