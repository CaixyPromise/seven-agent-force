package application

import (
	"context"
	"errors"
	"strings"
	"time"

	"go.uber.org/zap"

	"github.com/CaixyPromise/seven-agent/force/internal/app/workforce/taskdispatch/domain"
)

var (
	ErrNotFound         = errors.New("assignment not found")
	ErrNotClaimable     = errors.New("assignment is not claimable")
	ErrStaleInstance    = errors.New("stale worker instance")
	ErrIdentityConflict = errors.New("execution identity conflict")
)

type Repository interface {
	DispatchOne(context.Context, []int64) (int64, bool, error)
	ReconcileExpired(context.Context, int) (int, error)
	Claim(context.Context, domain.MachineEnvelope, int64, string) (domain.ExecutionContract, error)
	ReleaseAssignment(context.Context, domain.MachineEnvelope, int64, string) error
	AcquireLaunchPermit(context.Context, domain.MachineEnvelope, int64, string) (domain.LaunchPermit, bool, error)
	RecordContainerCreated(context.Context, domain.MachineEnvelope, int64, string, string, string, string) error
	RecordProviderProxyReady(context.Context, domain.MachineEnvelope, int64, string, domain.ProviderProxyReady) error
	RecordSupervisorReady(context.Context, domain.MachineEnvelope, int64, string, domain.SupervisorReady) error
	AcquireRunPermit(context.Context, domain.MachineEnvelope, int64, string) (domain.RunPermit, error)
	RecordHarnessStarted(context.Context, domain.MachineEnvelope, int64, string, domain.HarnessStarted) error
	RenewExecutionLease(context.Context, domain.MachineEnvelope, int64, string, *int64) (string, error)
	AcquireReattachPermit(context.Context, domain.MachineEnvelope, int64, domain.RuntimeEvidence) (domain.ReattachPermit, error)
	CommitReattach(context.Context, domain.MachineEnvelope, int64, string, domain.ReattachAck) (domain.ReattachResult, error)
	ReportRuntimeLoss(context.Context, domain.MachineEnvelope, int64, string, string) (domain.RuntimeLossResult, error)
	ReconcileRuntimeLosses(context.Context, int) (int, error)
	ReconcileTimeouts(context.Context, int) (int, error)
}

func (s *Service) ReleaseAssignment(ctx context.Context, e domain.MachineEnvelope, assignmentID int64, reason string) error {
	if !validEnvelope(e) || assignmentID == 0 || strings.TrimSpace(reason) == "" || len(reason) > 128 {
		return ErrIdentityConflict
	}
	return s.repo.ReleaseAssignment(ctx, e, assignmentID, strings.TrimSpace(reason))
}

type Service struct{ repo Repository }

func New(repo Repository) *Service { return &Service{repo: repo} }
func (s *Service) Claim(ctx context.Context, e domain.MachineEnvelope, assignmentID int64, requirement string) (domain.ExecutionContract, error) {
	if !validEnvelope(e) || assignmentID == 0 || len(requirement) != 64 {
		return domain.ExecutionContract{}, ErrIdentityConflict
	}
	return s.repo.Claim(ctx, e, assignmentID, requirement)
}
func (s *Service) AcquireLaunchPermit(ctx context.Context, e domain.MachineEnvelope, attemptID int64, lease string) (domain.LaunchPermit, bool, error) {
	if !validEnvelope(e) || attemptID == 0 || lease == "" {
		return domain.LaunchPermit{}, false, ErrIdentityConflict
	}
	return s.repo.AcquireLaunchPermit(ctx, e, attemptID, lease)
}
func (s *Service) RecordContainerCreated(ctx context.Context, e domain.MachineEnvelope, attemptID int64, lease string, containerID string, imageDigest string, identity string) error {
	if !validAttemptEnvelope(e) || attemptID == 0 || lease == "" || strings.TrimSpace(containerID) == "" || imageDigest == "" || identity == "" {
		return ErrIdentityConflict
	}
	return s.repo.RecordContainerCreated(ctx, e, attemptID, lease, containerID, imageDigest, identity)
}
func (s *Service) RecordProviderProxyReady(ctx context.Context, e domain.MachineEnvelope, attemptID int64, lease string, ready domain.ProviderProxyReady) error {
	if !validAttemptEnvelope(e) || attemptID == 0 || lease == "" || strings.TrimSpace(ready.ContainerID) == "" || ready.ContainerName == "" || ready.ImageDigest == "" || len(ready.ExecutionIdentityDigest) != 64 || ready.ControlState != "READY" {
		return ErrIdentityConflict
	}
	return s.repo.RecordProviderProxyReady(ctx, e, attemptID, lease, ready)
}
func (s *Service) RecordSupervisorReady(ctx context.Context, e domain.MachineEnvelope, attemptID int64, lease string, ready domain.SupervisorReady) error {
	if !validAttemptEnvelope(e) || ready.ControlProtocol != "seven.supervisor-control/v2" {
		return ErrIdentityConflict
	}
	return s.repo.RecordSupervisorReady(ctx, e, attemptID, lease, ready)
}
func (s *Service) AcquireRunPermit(ctx context.Context, e domain.MachineEnvelope, attemptID int64, lease string) (domain.RunPermit, error) {
	if !validAttemptEnvelope(e) || attemptID == 0 || lease == "" {
		return domain.RunPermit{}, ErrIdentityConflict
	}
	return s.repo.AcquireRunPermit(ctx, e, attemptID, lease)
}
func (s *Service) RecordHarnessStarted(ctx context.Context, e domain.MachineEnvelope, attemptID int64, lease string, started domain.HarnessStarted) error {
	if !validAttemptEnvelope(e) || started.RunPermitID == 0 || len(started.RunPermitDigest) != 64 || len(started.RunNonceDigest) != 64 || started.HarnessPID < 1 {
		return ErrIdentityConflict
	}
	return s.repo.RecordHarnessStarted(ctx, e, attemptID, lease, started)
}
func (s *Service) RenewExecutionLease(ctx context.Context, e domain.MachineEnvelope, attemptID int64, lease string, activeElapsedMillis *int64) (string, error) {
	if !validAttemptEnvelope(e) || attemptID == 0 || lease == "" {
		return "", ErrIdentityConflict
	}
	if activeElapsedMillis != nil && *activeElapsedMillis < 0 {
		return "", ErrIdentityConflict
	}
	return s.repo.RenewExecutionLease(ctx, e, attemptID, lease, activeElapsedMillis)
}

func (s *Service) AcquireReattachPermit(ctx context.Context, e domain.MachineEnvelope, attemptID int64, evidence domain.RuntimeEvidence) (domain.ReattachPermit, error) {
	if !validAttemptEnvelope(e) || attemptID == 0 || evidence.AttemptID != attemptID {
		return domain.ReattachPermit{}, ErrIdentityConflict
	}
	return s.repo.AcquireReattachPermit(ctx, e, attemptID, evidence)
}

func (s *Service) CommitReattach(ctx context.Context, e domain.MachineEnvelope, attemptID int64, lease string, ack domain.ReattachAck) (domain.ReattachResult, error) {
	if !validAttemptEnvelope(e) || attemptID == 0 || lease == "" || ack.PermitID == 0 || len(ack.PermitDigest) != 64 {
		return domain.ReattachResult{}, ErrIdentityConflict
	}
	if ack.ProviderProxy != nil && (ack.ProviderProxy.PermitID != ack.PermitID || len(ack.ProviderProxy.PermitDigest) != 64 || ack.ProviderProxy.ControlState != "READY") {
		return domain.ReattachResult{}, ErrIdentityConflict
	}
	return s.repo.CommitReattach(ctx, e, attemptID, lease, ack)
}

func (s *Service) ReportRuntimeLoss(ctx context.Context, e domain.MachineEnvelope, attemptID int64, reason string, evidenceDigest string) (domain.RuntimeLossResult, error) {
	if !validEnvelope(e) || attemptID == 0 || !DefaultRuntimeLossReason(reason) || (evidenceDigest != "" && len(evidenceDigest) != 64) {
		return domain.RuntimeLossResult{}, ErrIdentityConflict
	}
	return s.repo.ReportRuntimeLoss(ctx, e, attemptID, reason, evidenceDigest)
}

func DefaultRuntimeLossReason(reason string) bool {
	switch strings.TrimSpace(reason) {
	case domain.ReasonHostLost, domain.ReasonRuntimeMissing, domain.ReasonWorkerLostWithoutRuntime, "RUNTIME_IDENTITY_DRIFT", "REATTACH_DEADLINE_EXPIRED":
		return true
	default:
		return false
	}
}
func validEnvelope(e domain.MachineEnvelope) bool {
	return e.SchemaVersion == 1 && e.WorkerID > 0 && strings.TrimSpace(e.InstanceID) != "" && strings.TrimSpace(e.HostBootID) != "" && strings.TrimSpace(e.RequestID) != "" && len(e.RequestID) <= 128 && e.Generation > 0
}
func validAttemptEnvelope(e domain.MachineEnvelope) bool { return validEnvelope(e) && e.OwnerEpoch > 0 }

// DispatchAvailable performs one fairness sweep. A tenant can contribute at
// most one Assignment per sweep even when its selected Task cannot currently
// be placed, so a busy or incompatible tenant cannot block other tenants.
func (s *Service) DispatchAvailable(ctx context.Context, limit int) (int, error) {
	if limit < 1 || limit > 256 {
		limit = 32
	}
	excludedTenants := make([]int64, 0, limit)
	assignedCount := 0
	for i := 0; i < limit; i++ {
		tenantID, assigned, err := s.repo.DispatchOne(ctx, excludedTenants)
		if err != nil {
			return assignedCount, err
		}
		if tenantID == 0 {
			break
		}
		excludedTenants = append(excludedTenants, tenantID)
		if assigned {
			assignedCount++
		}
	}
	return assignedCount, nil
}

func (s *Service) Run(ctx context.Context, logger *zap.Logger) {
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	for {
		if _, err := s.DispatchAvailable(ctx, 32); err != nil && ctx.Err() == nil {
			logger.Warn("dispatch Workforce task", zap.Error(err))
		}
		_, _ = s.repo.ReconcileExpired(ctx, 32)
		_, _ = s.repo.ReconcileRuntimeLosses(ctx, 32)
		_, _ = s.repo.ReconcileTimeouts(ctx, 32)
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}
