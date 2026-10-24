// Package facade exposes the narrow cross-module invalidation port used by
// system configuration and dictionary application services.
package facade

import (
	"context"
	"errors"

	"github.com/CaixyPromise/seven-agent/force/internal/shared/cachepolicy"
)

// Registration is opaque durable invalidation identity. It contains no cache
// key, business target, configuration value, or secret.
type Registration struct {
	EventID   string
	DataClass cachepolicy.DataClass
}

// InvalidationRegistrar must be called inside the mutation transaction.
// AfterCommit is deliberately separate so a rolled-back transaction cannot
// dirty a writer's local cache or emit a broker event.
type InvalidationRegistrar interface {
	Enabled() bool
	Register(ctx context.Context, dataClass cachepolicy.DataClass) (Registration, error)
	AfterCommit(ctx context.Context, registrations ...Registration)
	AcquireMutationFence(ctx context.Context, dataClass cachepolicy.DataClass) (cachepolicy.FreshnessLease, error)
}

// DisabledRegistrar is the explicit no-cache protocol used when DG5 has not
// been opted in. It cannot append an Outbox event or create a local-only path.
type DisabledRegistrar struct{}

func (DisabledRegistrar) Enabled() bool { return false }

func (DisabledRegistrar) Register(_ context.Context, dataClass cachepolicy.DataClass) (Registration, error) {
	return Registration{DataClass: dataClass}, nil
}

func (DisabledRegistrar) AfterCommit(_ context.Context, _ ...Registration) {}

func (DisabledRegistrar) AcquireMutationFence(_ context.Context, _ cachepolicy.DataClass) (cachepolicy.FreshnessLease, error) {
	return nil, errors.New("cache governance is disabled")
}

// TargetedRegistration is an opaque DG6.2 event identity. TargetDigest is an
// irreversible digest, never a raw session identifier.
type TargetedRegistration struct {
	EventID      string
	DataClass    cachepolicy.DataClass
	TargetKind   string
	TargetDigest string
}

// TargetedInvalidationRegistrar is a distinct protocol so existing V1
// config/dict/authorization registrations remain class-wide and compatible.
type TargetedInvalidationRegistrar interface {
	Enabled() bool
	RegisterTarget(ctx context.Context, dataClass cachepolicy.DataClass, targetKind, targetDigest string) (TargetedRegistration, error)
	AfterTargetCommit(ctx context.Context, registrations ...TargetedRegistration)
	AcquireTargetMutationFence(ctx context.Context, dataClass cachepolicy.DataClass, targetKind, targetDigest string) (cachepolicy.FreshnessLease, error)
	BeginTargetMutationFence(ctx context.Context) (cachepolicy.TargetedMutationFence, error)
}

type DisabledTargetedRegistrar struct{}

func (DisabledTargetedRegistrar) Enabled() bool { return false }
func (DisabledTargetedRegistrar) RegisterTarget(_ context.Context, class cachepolicy.DataClass, kind, digest string) (TargetedRegistration, error) {
	return TargetedRegistration{DataClass: class, TargetKind: kind, TargetDigest: digest}, errors.New("targeted cache governance is disabled")
}
func (DisabledTargetedRegistrar) AfterTargetCommit(_ context.Context, _ ...TargetedRegistration) {}
func (DisabledTargetedRegistrar) AcquireTargetMutationFence(_ context.Context, _ cachepolicy.DataClass, _, _ string) (cachepolicy.FreshnessLease, error) {
	return nil, errors.New("targeted cache governance is disabled")
}
func (DisabledTargetedRegistrar) BeginTargetMutationFence(context.Context) (cachepolicy.TargetedMutationFence, error) {
	return nil, errors.New("targeted cache governance is disabled")
}

// RefreshResult intentionally exposes only a safe state for the protected
// system operation. It does not leak a raw cache key, target, broker state, or
// outbox payload.
type RefreshResult struct {
	State string
}

// RefreshFacade is the only cross-layer entry point for a global governed
// cache refresh. HTTP handlers must call this application facade rather than
// infrastructure cache managers or Redis clients.
type RefreshFacade interface {
	Enabled() bool
	Refresh(ctx context.Context) (RefreshResult, error)
}

type DisabledRefreshFacade struct{}

func (DisabledRefreshFacade) Enabled() bool { return false }
func (DisabledRefreshFacade) Refresh(context.Context) (RefreshResult, error) {
	return RefreshResult{}, errors.New("cache governance is disabled")
}
