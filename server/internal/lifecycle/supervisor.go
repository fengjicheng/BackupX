package lifecycle

import (
	"context"
	"sync"
)

// Supervisor owns application background tasks. It rejects new work once
// shutdown starts, cancels the shared task context, and waits for accepted work.
type Supervisor struct {
	ctx    context.Context
	cancel context.CancelFunc

	mu       sync.Mutex
	stopping bool
	wg       sync.WaitGroup
	done     chan struct{}
	stopOnce sync.Once
}

func NewSupervisor(parent context.Context) *Supervisor {
	if parent == nil {
		parent = context.Background()
	}
	ctx, cancel := context.WithCancel(parent)
	return &Supervisor{
		ctx:    ctx,
		cancel: cancel,
		done:   make(chan struct{}),
	}
}

// Context is the root context passed to every accepted task.
func (s *Supervisor) Context() context.Context {
	return s.ctx
}

// Go starts task unless shutdown has begun or the root context is already
// canceled. The lock makes Add and the transition to Wait mutually exclusive.
func (s *Supervisor) Go(task func(context.Context)) bool {
	if s == nil || task == nil {
		return false
	}
	s.mu.Lock()
	if s.stopping || s.ctx.Err() != nil {
		s.mu.Unlock()
		return false
	}
	s.wg.Add(1)
	s.mu.Unlock()

	go func() {
		defer s.wg.Done()
		task(s.ctx)
	}()
	return true
}

// Shutdown is idempotent. Cancellation always happens, even when waitCtx has
// already expired; callers may call Shutdown again to wait for eventual exit.
func (s *Supervisor) Shutdown(waitCtx context.Context) error {
	if s == nil {
		return nil
	}
	if waitCtx == nil {
		waitCtx = context.Background()
	}
	s.stopOnce.Do(func() {
		s.mu.Lock()
		s.stopping = true
		s.cancel()
		s.mu.Unlock()

		go func() {
			s.wg.Wait()
			close(s.done)
		}()
	})

	select {
	case <-s.done:
		return nil
	case <-waitCtx.Done():
		return waitCtx.Err()
	}
}
