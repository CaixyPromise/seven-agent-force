package microservice

import (
	"context"
	"sync"
	"time"
)

type ManagerOptions struct {
	RetryDelays       []time.Duration
	DeregisterTimeout time.Duration
}

type Manager struct {
	registrar Registrar
	options   ManagerOptions

	mu           sync.Mutex
	registration ServiceRegistration
	cancel       context.CancelFunc
	workers      sync.WaitGroup
	started      bool
	shuttingDown bool
	shutdownDone chan struct{}
	shutdownErr  error
}

func NewManager(registrar Registrar, options ManagerOptions) *Manager {
	if len(options.RetryDelays) == 0 {
		options.RetryDelays = []time.Duration{time.Second, 2 * time.Second, 5 * time.Second, 10 * time.Second, 30 * time.Second}
	} else {
		options.RetryDelays = append([]time.Duration(nil), options.RetryDelays...)
		for index, delay := range options.RetryDelays {
			if delay <= 0 {
				options.RetryDelays[index] = time.Second
			}
		}
	}
	if options.DeregisterTimeout <= 0 {
		options.DeregisterTimeout = 3 * time.Second
	}
	return &Manager{registrar: registrar, options: options, shutdownDone: make(chan struct{})}
}

func (m *Manager) Start(ctx context.Context, registration ServiceRegistration, registrationRequired bool) error {
	if m == nil || isNilDependency(m.registrar) {
		return ErrInvalidDependency
	}
	if ctx == nil {
		return ErrInvalidContext
	}
	m.mu.Lock()
	if m.shuttingDown {
		m.mu.Unlock()
		return ErrManagerShutdown
	}
	if m.started {
		m.mu.Unlock()
		return ErrManagerStarted
	}
	m.started = true
	m.registration = registration
	lifecycleCtx, cancel := context.WithCancel(context.Background())
	m.cancel = cancel
	m.workers.Add(1)
	m.mu.Unlock()

	initialCtx, cancelInitial := context.WithCancel(ctx)
	stopLifecycleCancel := context.AfterFunc(lifecycleCtx, cancelInitial)
	err := m.registrar.Register(initialCtx, registration)
	stopLifecycleCancel()
	cancelInitial()

	m.mu.Lock()
	launchRetry := err != nil && !registrationRequired && !m.shuttingDown
	if launchRetry {
		m.workers.Add(1)
	}
	m.mu.Unlock()
	m.workers.Done()
	if launchRetry {
		go func() {
			defer m.workers.Done()
			m.retryRegistration(lifecycleCtx, registration)
		}()
	}
	if err == nil {
		return nil
	}
	if registrationRequired {
		return err
	}
	return nil
}

func (m *Manager) Shutdown(ctx context.Context) error {
	if m == nil || isNilDependency(m.registrar) {
		return ErrInvalidDependency
	}
	if ctx == nil {
		return ErrInvalidContext
	}
	m.mu.Lock()
	if m.shuttingDown {
		done := m.shutdownDone
		m.mu.Unlock()
		select {
		case <-done:
			m.mu.Lock()
			err := m.shutdownErr
			m.mu.Unlock()
			return err
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	m.shuttingDown = true
	registration := m.registration
	cancel := m.cancel
	m.mu.Unlock()
	if cancel != nil {
		cancel()
	}
	m.workers.Wait()

	var err error
	if registration.ID == "" {
		err = nil
	} else {
		deregisterCtx, cancelDeregister := context.WithTimeout(ctx, m.options.DeregisterTimeout)
		err = m.registrar.Deregister(deregisterCtx, registration.ID)
		cancelDeregister()
	}
	m.mu.Lock()
	m.shutdownErr = err
	close(m.shutdownDone)
	m.mu.Unlock()
	return err
}

func (m *Manager) retryRegistration(ctx context.Context, registration ServiceRegistration) {
	for attempt := 0; ; attempt++ {
		delay := m.options.RetryDelays[0]
		if attempt >= len(m.options.RetryDelays)-1 {
			delay = m.options.RetryDelays[len(m.options.RetryDelays)-1]
		} else {
			delay = m.options.RetryDelays[attempt]
		}
		timer := time.NewTimer(delay)
		select {
		case <-ctx.Done():
			timer.Stop()
			return
		case <-timer.C:
		}
		m.mu.Lock()
		shuttingDown := m.shuttingDown
		m.mu.Unlock()
		if shuttingDown || ctx.Err() != nil {
			return
		}
		if m.registrar.Register(ctx, registration) == nil {
			return
		}
	}
}
