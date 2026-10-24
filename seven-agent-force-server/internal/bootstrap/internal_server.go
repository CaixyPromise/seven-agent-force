package bootstrap

import (
	"context"
	"errors"
	"fmt"
	"net"
	"reflect"
	"sync"
	"time"

	bootstrapruntime "github.com/CaixyPromise/seven-agent/force/internal/bootstrap/runtime"
	apperrors "github.com/CaixyPromise/seven-agent/force/internal/common/errors"
	"github.com/CaixyPromise/seven-agent/force/internal/common/httpx"
	"github.com/CaixyPromise/seven-agent/force/internal/common/response"
	protocolhttp "github.com/CaixyPromise/seven-agent/force/internal/infrastructure/protocol/http"
	"github.com/CaixyPromise/seven-agent/force/internal/shared/config"
	"github.com/cloudwego/hertz/pkg/app"
	"github.com/cloudwego/hertz/pkg/app/server"
	"github.com/cloudwego/hertz/pkg/network/standard"
	"go.uber.org/zap"
)

const internalServerStartTimeout = time.Second

// internalServer hosts Node-only routes on an optional dedicated listener.
// Configuration chooses whether to create it; this component only owns lifecycle.
type internalServer struct {
	engine       *server.Hertz
	listener     net.Listener
	once         sync.Once
	shutdownMu   sync.Mutex
	stateMu      sync.RWMutex
	listenerOnce sync.Once
	startTimeout time.Duration
	run          func() error
	isRunning    func() bool
	completion   chan error
	stopping     bool
}

func newInternalServer(listener net.Listener, mounters ...bootstrapruntime.InternalRouteMounter) (*internalServer, error) {
	return newInternalServerWithMiddleware(listener, protocolhttp.StandardMiddlewareChain(config.Config{}, zap.NewNop()), mounters...)
}

func newInternalServerWithMiddleware(listener net.Listener, middlewares []app.HandlerFunc, mounters ...bootstrapruntime.InternalRouteMounter) (*internalServer, error) {
	if listener == nil {
		return nil, errors.New("internal listener is required")
	}
	usableMounters := make([]bootstrapruntime.InternalRouteMounter, 0, len(mounters))
	for _, mounter := range mounters {
		if !isNilInternalRouteMounter(mounter) {
			usableMounters = append(usableMounters, mounter)
		}
	}
	if len(usableMounters) == 0 {
		return nil, errors.New("at least one internal route mounter is required")
	}
	engine := server.New(
		server.WithBindConfig(httpx.NewBindConfig()),
		server.WithListener(listener),
		server.WithTransport(standard.NewTransporter),
		server.WithHandleMethodNotAllowed(true),
	)
	engine.Use(middlewares...)
	engine.NoRoute(func(ctx context.Context, reqCtx *app.RequestContext) {
		response.Error(reqCtx, apperrors.NotFound("请求路径不存在"))
	})
	engine.NoMethod(func(ctx context.Context, reqCtx *app.RequestContext) {
		response.Error(reqCtx, apperrors.MethodNotAllowed("请求方法不被允许"))
	})
	for _, mounter := range usableMounters {
		mounter.MountInternal(engine.Engine)
	}
	return &internalServer{
		engine:       engine,
		listener:     listener,
		startTimeout: internalServerStartTimeout,
		run:          engine.Run,
		isRunning:    engine.IsRunning,
		completion:   make(chan error, 1),
	}, nil
}

func (s *internalServer) Start() error {
	if s == nil || s.run == nil || s.isRunning == nil {
		return errors.New("internal server is not configured")
	}
	completion := s.completionChannel()
	started := false
	s.once.Do(func() {
		started = true
		go func() { completion <- s.run() }()
	})
	if !started {
		return errors.New("internal server already started")
	}

	deadline := time.NewTimer(s.startTimeout)
	defer deadline.Stop()
	for {
		if s.isRunning() {
			return nil
		}
		select {
		case err := <-completion:
			s.abort()
			if err == nil {
				return errors.New("internal server stopped before readiness")
			}
			return fmt.Errorf("start internal server: %w", err)
		case <-deadline.C:
			s.abort()
			return errors.New("internal server did not become ready")
		case <-time.After(time.Millisecond):
		}
	}
}

// abort releases the bound port before a failed start can leave a late Run alive.
func (s *internalServer) abort() {
	if s == nil {
		return
	}
	s.shutdownMu.Lock()
	defer s.shutdownMu.Unlock()
	s.markStopping()
	s.closeListener()
	if s.engine != nil && s.engine.IsRunning() {
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		_ = s.engine.Shutdown(shutdownCtx)
	}
}

func (s *internalServer) Shutdown(ctx context.Context) error {
	if s == nil {
		return nil
	}
	s.shutdownMu.Lock()
	defer s.shutdownMu.Unlock()
	s.markStopping()
	if s.engine != nil && s.engine.IsRunning() {
		if err := s.engine.Shutdown(ctx); err != nil {
			s.closeListener()
			return fmt.Errorf("shutdown internal server: %w", err)
		}
	}
	s.closeListener()
	return nil
}

// Completion receives the internal Run result exactly once after Start.
func (s *internalServer) Completion() <-chan error {
	if s == nil {
		return nil
	}
	return s.completionChannel()
}

func (s *internalServer) Stopping() bool {
	if s == nil {
		return true
	}
	s.stateMu.RLock()
	defer s.stateMu.RUnlock()
	return s.stopping
}

func (s *internalServer) completionChannel() chan error {
	s.stateMu.Lock()
	defer s.stateMu.Unlock()
	if s.completion == nil {
		s.completion = make(chan error, 1)
	}
	return s.completion
}

func (s *internalServer) markStopping() {
	s.stateMu.Lock()
	defer s.stateMu.Unlock()
	s.stopping = true
}

func (s *internalServer) closeListener() {
	s.listenerOnce.Do(func() {
		if s.listener != nil {
			_ = s.listener.Close()
		}
	})
}

func isNilInternalRouteMounter(mounter bootstrapruntime.InternalRouteMounter) bool {
	if mounter == nil {
		return true
	}
	value := reflect.ValueOf(mounter)
	switch value.Kind() {
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Pointer, reflect.Slice:
		return value.IsNil()
	default:
		return false
	}
}
