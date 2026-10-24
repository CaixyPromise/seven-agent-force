package microservice

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

type registrarFunc struct {
	register   func(context.Context, ServiceRegistration) error
	deregister func(context.Context, string) error
}

type panicRegistrar struct{}

func (*panicRegistrar) Register(context.Context, ServiceRegistration) error {
	panic("typed-nil registrar invoked")
}

func (*panicRegistrar) Deregister(context.Context, string) error {
	panic("typed-nil registrar invoked")
}

func TestManagerStartIsOneShotUnderConcurrency(t *testing.T) {
	entered := make(chan struct{})
	release := make(chan struct{})
	var calls atomic.Int32
	manager := NewManager(registrarFunc{
		register: func(context.Context, ServiceRegistration) error {
			if calls.Add(1) == 1 {
				close(entered)
			}
			<-release
			return nil
		},
		deregister: func(context.Context, string) error { return nil },
	}, ManagerOptions{})

	firstDone := make(chan error, 1)
	go func() {
		firstDone <- manager.Start(context.Background(), ServiceRegistration{ID: "hub-a"}, false)
	}()
	<-entered
	if err := manager.Start(context.Background(), ServiceRegistration{ID: "hub-b"}, false); !errors.Is(err, ErrManagerStarted) {
		t.Fatalf("second Start() error = %v, want ErrManagerStarted", err)
	}
	close(release)
	if err := <-firstDone; err != nil {
		t.Fatalf("first Start() error = %v", err)
	}
	if got := calls.Load(); got != 1 {
		t.Fatalf("registration calls = %d, want 1", got)
	}
}

func TestManagerRequiredRegistrationFailureDoesNotRetry(t *testing.T) {
	var calls atomic.Int32
	manager := NewManager(registrarFunc{
		register: func(context.Context, ServiceRegistration) error {
			calls.Add(1)
			return errors.New("registration failed")
		},
		deregister: func(context.Context, string) error { return nil },
	}, ManagerOptions{RetryDelays: []time.Duration{time.Millisecond}})

	if err := manager.Start(context.Background(), ServiceRegistration{ID: "hub-a"}, true); err == nil {
		t.Fatal("Start() succeeded, want registration error")
	}
	time.Sleep(20 * time.Millisecond)
	if got := calls.Load(); got != 1 {
		t.Fatalf("registration calls = %d, want 1", got)
	}
}

func TestManagerShutdownJoinsRetryAndIsIdempotent(t *testing.T) {
	retryEntered := make(chan struct{})
	var registerCalls atomic.Int32
	var retryExited atomic.Bool
	var deregisterCalls atomic.Int32
	manager := NewManager(registrarFunc{
		register: func(ctx context.Context, _ ServiceRegistration) error {
			if registerCalls.Add(1) == 1 {
				return errors.New("initial failure")
			}
			close(retryEntered)
			<-ctx.Done()
			retryExited.Store(true)
			return ctx.Err()
		},
		deregister: func(context.Context, string) error {
			if !retryExited.Load() {
				t.Error("deregister ran before retry worker exited")
			}
			deregisterCalls.Add(1)
			return nil
		},
	}, ManagerOptions{RetryDelays: []time.Duration{time.Millisecond}, DeregisterTimeout: time.Second})
	if err := manager.Start(context.Background(), ServiceRegistration{ID: "hub-a"}, false); err != nil {
		t.Fatal(err)
	}
	<-retryEntered

	const shutdowns = 8
	errs := make(chan error, shutdowns)
	var wg sync.WaitGroup
	for range shutdowns {
		wg.Add(1)
		go func() {
			defer wg.Done()
			errs <- manager.Shutdown(context.Background())
		}()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatalf("Shutdown() error = %v", err)
		}
	}
	if got := deregisterCalls.Load(); got != 1 {
		t.Fatalf("deregister calls = %d, want 1", got)
	}
	if got := registerCalls.Load(); got != 2 {
		t.Fatalf("register calls = %d, want 2", got)
	}
	if err := manager.Start(context.Background(), ServiceRegistration{ID: "hub-a"}, false); !errors.Is(err, ErrManagerShutdown) {
		t.Fatalf("post-shutdown Start() error = %v, want ErrManagerShutdown", err)
	}
}

func TestManagerNormalizesUnsafeOptionsAndRejectsNilRegistrar(t *testing.T) {
	manager := NewManager(nil, ManagerOptions{RetryDelays: []time.Duration{0, -time.Second}, DeregisterTimeout: -time.Second})
	for _, delay := range manager.options.RetryDelays {
		if delay <= 0 {
			t.Fatalf("retry delay = %s, want positive", delay)
		}
	}
	if manager.options.DeregisterTimeout <= 0 {
		t.Fatalf("deregister timeout = %s, want positive", manager.options.DeregisterTimeout)
	}
	if err := manager.Start(context.Background(), ServiceRegistration{ID: "hub-a"}, false); !errors.Is(err, ErrInvalidDependency) {
		t.Fatalf("Start() error = %v, want ErrInvalidDependency", err)
	}
}

func TestManagerRejectsTypedNilRegistrar(t *testing.T) {
	var registrar *panicRegistrar
	manager := NewManager(registrar, ManagerOptions{})
	if err := manager.Start(context.Background(), ServiceRegistration{ID: "hub-a"}, false); !errors.Is(err, ErrInvalidDependency) {
		t.Fatalf("Start() error = %v, want ErrInvalidDependency", err)
	}
}

func (r registrarFunc) Register(ctx context.Context, registration ServiceRegistration) error {
	return r.register(ctx, registration)
}

func (r registrarFunc) Deregister(ctx context.Context, id string) error {
	return r.deregister(ctx, id)
}

func TestManagerBestEffortReregistersAndBoundsShutdown(t *testing.T) {
	var registrations atomic.Int32
	registered := make(chan struct{})
	registrar := registrarFunc{
		register: func(context.Context, ServiceRegistration) error {
			if registrations.Add(1) == 2 {
				close(registered)
				return nil
			}
			return errors.New("consul unavailable")
		},
		deregister: func(ctx context.Context, _ string) error {
			<-ctx.Done()
			return ctx.Err()
		},
	}
	manager := NewManager(registrar, ManagerOptions{RetryDelays: []time.Duration{time.Millisecond}, DeregisterTimeout: 10 * time.Millisecond})
	if err := manager.Start(context.Background(), ServiceRegistration{ID: "hub-a"}, false); err != nil {
		t.Fatalf("best effort Start() error = %v", err)
	}
	select {
	case <-registered:
	case <-time.After(time.Second):
		t.Fatal("registration retry did not succeed")
	}
	started := time.Now()
	if err := manager.Shutdown(context.Background()); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("Shutdown() error = %v", err)
	}
	if elapsed := time.Since(started); elapsed > 100*time.Millisecond {
		t.Fatalf("Shutdown() took %v", elapsed)
	}
}
