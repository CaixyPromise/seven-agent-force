package application

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	continuation "github.com/CaixyPromise/seven-agent/force/internal/app/workforce/continuation/application"
	continuationfacade "github.com/CaixyPromise/seven-agent/force/internal/app/workforce/continuation/facade"
	"github.com/CaixyPromise/seven-agent/force/internal/app/workforce/executionaccess/domain"
	"github.com/CaixyPromise/seven-agent/force/internal/app/workforce/executionaccess/facade"
)

type Service struct {
	pool                             *pgxpool.Pool
	ids                              facade.IDGenerator
	repo                             Repository
	users                            facade.UserAuthority
	machines                         facade.MachineAuthority
	cipher                           facade.Cipher
	projector                        facade.Projector
	now                              func() time.Time
	operationTTL, grantTTL, leaseTTL time.Duration
	resumeFinder                     continuationfacade.OperationResumeFinder
	resumer                          continuationfacade.OperationResumer
}

func New(pool *pgxpool.Pool, ids facade.IDGenerator, repo Repository, users facade.UserAuthority, machines facade.MachineAuthority, cipher facade.Cipher, projector facade.Projector) *Service {
	return &Service{pool: pool, ids: ids, repo: repo, users: users, machines: machines, cipher: cipher, projector: projector, now: func() time.Time { return time.Now().UTC() }, operationTTL: 24 * time.Hour, grantTTL: 24 * time.Hour, leaseTTL: 5 * time.Minute}
}

func (s *Service) WithContinuation(finder continuationfacade.OperationResumeFinder, resumer continuationfacade.OperationResumer) *Service {
	s.resumeFinder, s.resumer = finder, resumer
	return s
}

var _ facade.PublicService = (*Service)(nil)
var _ facade.MachineService = (*Service)(nil)
var _ facade.Lifecycle = (*Service)(nil)
var _ continuation.OperationSuspendOccurrenceGate = (*Service)(nil)

func (s *Service) configured() bool {
	return s != nil && s.pool != nil && s.ids != nil && s.repo != nil && s.users != nil && s.machines != nil && s.cipher != nil && s.projector != nil
}

func (s *Service) transaction(ctx context.Context, fn func(pgx.Tx) error) error {
	if !s.configured() {
		return domain.ErrUnavailable
	}
	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return domain.ErrUnavailable
	}
	defer tx.Rollback(ctx)
	if err = fn(tx); err != nil {
		return safe(err)
	}
	return safe(tx.Commit(ctx))
}

func safe(err error) error {
	if err == nil {
		return nil
	}
	for _, known := range []error{domain.ErrInvalid, domain.ErrNotFound, domain.ErrStale, domain.ErrActionNotAllowed, domain.ErrIdempotencyConflict, domain.ErrTargetDenied, domain.ErrOutcomeUnknown, domain.ErrGrantRevoked, domain.ErrGrantExpired, domain.ErrFenced, domain.ErrUnavailable} {
		if errors.Is(err, known) {
			return known
		}
	}
	return domain.ErrUnavailable
}

func (s *Service) user(ctx context.Context, tx pgx.Tx, u facade.User, action string) (facade.Target, error) {
	if u.TenantID <= 0 || u.TaskID <= 0 || u.UserID <= 0 {
		return facade.Target{}, domain.ErrInvalid
	}
	if action != facade.ActionRead && !u.CanContinue {
		return facade.Target{}, domain.ErrActionNotAllowed
	}
	t, err := s.users.AuthorizeUser(ctx, tx, u, action)
	if err != nil {
		return t, err
	}
	if t.TenantID != u.TenantID || t.TaskID != u.TaskID || (action != facade.ActionRead && t.PrincipalID <= 0) {
		return t, domain.ErrFenced
	}
	if action != facade.ActionRead && (t.TaskTerminal || t.TaskCancelled) {
		return t, domain.ErrActionNotAllowed
	}
	if _, err = tx.Exec(ctx, `SELECT pg_advisory_xact_lock($1)`, t.TaskID); err != nil {
		return t, err
	}
	return t, nil
}

func (s *Service) machine(ctx context.Context, tx pgx.Tx, m facade.Machine, action string) (facade.Target, error) {
	if m.WorkerID <= 0 || m.AttemptID <= 0 || m.Generation <= 0 || m.OwnerEpoch <= 0 || m.AdapterSessionGeneration <= 0 || m.ColdResumeOperationID < 0 || strings.TrimSpace(m.InstanceID) == "" || strings.TrimSpace(m.HostBootID) == "" || strings.TrimSpace(m.ExecutionLease) == "" {
		return facade.Target{}, domain.ErrInvalid
	}
	t, err := s.machines.ValidateMachine(ctx, tx, m, action)
	if err != nil {
		return t, err
	}
	if t.TenantID <= 0 || t.TaskID <= 0 || t.TaskTurnID <= 0 || t.AttemptID != m.AttemptID || t.AdapterSessionGeneration != m.AdapterSessionGeneration || t.WorkerID != m.WorkerID || t.OwnerEpoch != m.OwnerEpoch || t.TaskTerminal || t.TaskCancelled || (m.ColdResumeOperationID > 0 && t.SourceOperationID != m.ColdResumeOperationID) || (m.ColdResumeOperationID == 0 && t.SourceOperationID != 0) {
		return t, domain.ErrFenced
	}
	if m.TenantID != 0 && m.TenantID != t.TenantID || m.TaskID != 0 && m.TaskID != t.TaskID || m.TaskTurnID != 0 && m.TaskTurnID != t.TaskTurnID {
		return t, domain.ErrFenced
	}
	if _, err = tx.Exec(ctx, `SELECT pg_advisory_xact_lock($1)`, t.TaskID); err != nil {
		return t, err
	}
	return t, nil
}

func operationMatchesTarget(o domain.Operation, t facade.Target) bool {
	if t.SourceOperationID > 0 {
		return o.ID == t.SourceOperationID && o.AdapterSessionGeneration == t.AdapterSessionGeneration
	}
	return o.AttemptID == t.AttemptID && o.AdapterSessionGeneration == t.AdapterSessionGeneration
}

func capable(t facade.Target, capability string) bool {
	for _, c := range t.Capabilities {
		if c == capability {
			return true
		}
	}
	return false
}

func publishedOccurrenceMatches(o domain.Operation, target facade.Target, request facade.PublishRequest, descriptorDigest string) bool {
	return o.TenantID == target.TenantID && o.TaskID == target.TaskID &&
		o.RoundKey == request.RoundKey && o.LocalToolCallKey == request.LocalToolCallKey &&
		o.NativeToolCallID == request.NativeToolCallID && o.Kind == domain.KindNetworkCommand &&
		o.DescriptorDigest == descriptorDigest
}

func (s *Service) AuthorizeOperationSuspendTx(ctx context.Context, tx pgx.Tx, occurrence continuation.OperationSuspendOccurrence) error {
	if s == nil || s.repo == nil || tx == nil || occurrence.TenantID <= 0 || occurrence.TaskID <= 0 || occurrence.TaskTurnID <= 0 || occurrence.AttemptID <= 0 || occurrence.OperationID <= 0 || occurrence.AdapterSessionGeneration <= 0 {
		return domain.ErrFenced
	}
	o, err := s.repo.GetOperation(ctx, tx, occurrence.TenantID, occurrence.TaskID, occurrence.OperationID, true)
	if err != nil || o.TaskTurnID != occurrence.TaskTurnID || o.AttemptID != occurrence.AttemptID || o.AdapterSessionGeneration != occurrence.AdapterSessionGeneration || o.Status != domain.StatusWaitingApproval {
		return domain.ErrFenced
	}
	return nil
}

func (s *Service) PendingSuspendOperations(ctx context.Context, m facade.Machine) (out []facade.PendingSuspendOperation, err error) {
	err = s.transaction(ctx, func(tx pgx.Tx) error {
		t, e := s.machine(ctx, tx, m, facade.ActionRead)
		if e != nil {
			return e
		}
		items, e := s.repo.PendingSuspendOperations(ctx, tx, t.TenantID, t.TaskID, t.AttemptID, t.AdapterSessionGeneration)
		if e != nil {
			return e
		}
		for _, item := range items {
			out = append(out, facade.PendingSuspendOperation{OperationID: item.ID, RoundKey: item.RoundKey, LocalToolCallKey: item.LocalToolCallKey, NativeToolCallID: item.NativeToolCallID})
		}
		return nil
	})
	return out, err
}

func (s *Service) project(ctx context.Context, tx pgx.Tx, o domain.Operation, attention bool) error {
	return s.projector.ProjectExecutionOperation(ctx, tx, facade.Projection{TenantID: o.TenantID, TaskID: o.TaskID, TaskTurnID: o.TaskTurnID, AttemptID: o.AttemptID, OperationID: o.ID, Version: o.Version, Status: o.Status, Purpose: o.Purpose, Targets: append([]domain.Target(nil), o.Targets...), ResultDeliveryStatus: o.ResultDeliveryStatus, ResultSummary: o.ResultSummary, ExitCode: o.ExitCode, AgentContinued: o.AgentContinued, AttentionRequired: attention, OccurredAt: o.UpdatedAt})
}

func (s *Service) Publish(ctx context.Context, m facade.Machine, req facade.PublishRequest) (out facade.Receipt, err error) {
	if !s.configured() {
		return out, domain.ErrUnavailable
	}
	descriptor, err := domain.NormalizeDescriptor(req.Arguments)
	if err != nil || req.ToolName != facade.NetworkCommandToolName || !domain.ValidKey(req.LocalToolCallKey) || !domain.ValidNativeToolCallID(req.NativeToolCallID) || !domain.ValidKey(req.RoundKey) {
		return out, domain.ErrInvalid
	}
	digest := domain.Digest(descriptor)
	if req.ArgumentsDigest != digest {
		return out, domain.ErrInvalid
	}
	raw, err := json.Marshal(descriptor)
	if err != nil {
		return out, domain.ErrInvalid
	}
	encrypted, err := s.cipher.Encrypt(ctx, raw)
	if err != nil || encrypted.Ciphertext == "" || encrypted.EDEK == "" || encrypted.KeyRef == "" {
		return out, domain.ErrUnavailable
	}
	err = s.transaction(ctx, func(tx pgx.Tx) error {
		t, e := s.machine(ctx, tx, m, facade.ActionPublish)
		if e != nil {
			return e
		}
		if !capable(t, "tool.request") || !capable(t, "tool.result") {
			return domain.ErrActionNotAllowed
		}
		old, e := s.repo.FindOccurrence(ctx, tx, t.AttemptID, t.AdapterSessionGeneration, req.LocalToolCallKey)
		if e != nil {
			return e
		}
		if old != nil {
			if !publishedOccurrenceMatches(*old, t, req, digest) {
				return domain.ErrIdempotencyConflict
			}
			out = receipt(*old, 0, 0, true)
			return nil
		}
		now := s.now()
		o := domain.Operation{ID: s.ids.NextID(), TenantID: t.TenantID, TaskID: t.TaskID, TaskTurnID: t.TaskTurnID, AttemptID: t.AttemptID, AdapterSessionGeneration: t.AdapterSessionGeneration, LocalToolCallKey: req.LocalToolCallKey, NativeToolCallID: req.NativeToolCallID, RoundKey: req.RoundKey, Kind: domain.KindNetworkCommand, DescriptorDigest: digest, Descriptor: encrypted, Purpose: descriptor.Purpose, Targets: descriptor.Targets, Status: domain.StatusWaitingApproval, Version: 1, ExpiresAt: now.Add(s.operationTTL), CreatedAt: now, UpdatedAt: now}
		grants, e := s.repo.ActiveGrants(ctx, tx, t.TenantID, t.TaskID, true)
		if e != nil {
			return e
		}
		for i := range grants {
			if grants[i].Covers(o.Targets, now) {
				o.Status = domain.StatusReady
				o.AuthorizationGrantID = grants[i].ID
				break
			}
		}
		if e = s.repo.InsertOperation(ctx, tx, o); e != nil {
			return e
		}
		if e = s.project(ctx, tx, o, o.Status == domain.StatusWaitingApproval); e != nil {
			return e
		}
		out = receipt(o, 0, 0, false)
		return nil
	})
	return out, err
}

func (s *Service) Get(ctx context.Context, u facade.User, id int64) (out facade.OperationView, err error) {
	if id <= 0 {
		return out, domain.ErrInvalid
	}
	err = s.transaction(ctx, func(tx pgx.Tx) error {
		if _, e := s.user(ctx, tx, u, facade.ActionRead); e != nil {
			return e
		}
		o, e := s.repo.GetOperation(ctx, tx, u.TenantID, u.TaskID, id, false)
		if e != nil {
			return e
		}
		return s.view(ctx, o, true, &out)
	})
	return out, err
}

func (s *Service) Decide(ctx context.Context, u facade.User, id int64, req facade.DecisionRequest) (out facade.Receipt, err error) {
	reason, e := domain.NormalizeReason(req.Reason)
	if id <= 0 || req.ExpectedVersion <= 0 || !domain.ValidateDecision(req.Action) || !domain.ValidKey(req.IdempotencyKey) || e != nil {
		return out, domain.ErrInvalid
	}
	req.Reason = reason
	requestDigest := domain.Digest(struct {
		OperationID int64
		Request     facade.DecisionRequest
	}{id, req})
	err = s.transaction(ctx, func(tx pgx.Tx) error {
		t, e := s.user(ctx, tx, u, facade.ActionDecide)
		if e != nil {
			return e
		}
		oldDecision, e := s.repo.FindDecision(ctx, tx, u.TenantID, u.TaskID, t.PrincipalID, req.IdempotencyKey)
		if e != nil {
			return e
		}
		if oldDecision != nil {
			if oldDecision.OperationID != id || oldDecision.RequestDigest != requestDigest {
				return domain.ErrIdempotencyConflict
			}
			o, e := s.repo.GetOperation(ctx, tx, u.TenantID, u.TaskID, id, false)
			if e != nil {
				return e
			}
			out = receipt(o, 0, 0, true)
			if s.resumeFinder != nil {
				resume, findErr := s.resumeFinder.FindOperationResumeTx(ctx, tx, continuationfacade.OperationResumeLookup{TenantID: u.TenantID, TaskID: u.TaskID, AttemptID: o.AttemptID, OperationID: o.ID})
				if findErr == nil && resume != nil {
					out.TaskTurnID = resume.ResumedTaskTurnID
				}
			}
			return nil
		}
		o, e := s.repo.GetOperation(ctx, tx, u.TenantID, u.TaskID, id, true)
		if e != nil {
			return e
		}
		if o.AttemptID != t.AttemptID || o.TaskTurnID != t.TaskTurnID || o.AdapterSessionGeneration != t.AdapterSessionGeneration {
			return domain.ErrFenced
		}
		now := s.now()
		if e = o.ApplyDecision(req.ExpectedVersion, req.Action, now); e != nil {
			return e
		}
		decision := domain.Decision{ID: s.ids.NextID(), TenantID: u.TenantID, TaskID: u.TaskID, OperationID: o.ID, ActorPrincipalID: t.PrincipalID, Action: req.Action, IdempotencyKey: req.IdempotencyKey, RequestDigest: requestDigest, Reason: reason, Status: "APPLIED", CreatedAt: now}
		if e = s.repo.InsertDecision(ctx, tx, decision); e != nil {
			return e
		}
		var grantID int64
		if req.Action == domain.DecisionAllowForTask {
			grant := domain.Grant{ID: s.ids.NextID(), TenantID: u.TenantID, TaskID: u.TaskID, SourceDecisionID: decision.ID, ActorPrincipalID: t.PrincipalID, Targets: append([]domain.Target(nil), o.Targets...), TargetsDigest: domain.Digest(o.Targets), Status: domain.GrantActive, Version: 1, ExpiresAt: now.Add(s.grantTTL), CreatedAt: now, UpdatedAt: now}
			if e = s.repo.InsertGrant(ctx, tx, grant); e != nil {
				return e
			}
			grantID = grant.ID
			o.AuthorizationGrantID = grant.ID
		}
		if e = s.repo.UpdateOperation(ctx, tx, o); e != nil {
			return e
		}
		if e = s.project(ctx, tx, o, false); e != nil {
			return e
		}
		var resume *continuationfacade.OperationResumeTarget
		if s.resumeFinder != nil {
			resume, e = s.resumeFinder.FindOperationResumeTx(ctx, tx, continuationfacade.OperationResumeLookup{TenantID: u.TenantID, TaskID: u.TaskID, AttemptID: o.AttemptID, OperationID: o.ID})
			if e != nil {
				if errors.Is(e, continuationfacade.ErrInteractionSuspendInProgress) {
					return e
				}
				return domain.ErrActionNotAllowed
			}
		}
		var resumedTurnID int64
		if resume != nil {
			resumedTurnID = resume.ResumedTaskTurnID
		}
		if resume != nil && resume.ResumedTaskTurnID == 0 {
			if s.resumer == nil {
				return domain.ErrUnavailable
			}
			resumed, resumeErr := s.resumer.ResumeOperationTx(ctx, tx, continuation.OperationResumeRequest{
				TenantID: u.TenantID, TaskID: u.TaskID, SourceTaskTurnID: resume.SourceTaskTurnID,
				SourceAttemptID: o.AttemptID, OperationID: o.ID, CheckpointID: resume.CheckpointID,
				DecisionID: decision.ID, CreatorUserID: u.UserID, Action: req.Action,
			})
			if resumeErr != nil || resumed.TaskTurnID <= 0 {
				return domain.ErrActionNotAllowed
			}
			resumedTurnID = resumed.TaskTurnID
		}
		out = receipt(o, 0, grantID, false)
		out.TaskTurnID = resumedTurnID
		return nil
	})
	return out, err
}

func (s *Service) ListGrants(ctx context.Context, u facade.User) (out []facade.GrantView, err error) {
	err = s.transaction(ctx, func(tx pgx.Tx) error {
		if _, e := s.user(ctx, tx, u, facade.ActionRead); e != nil {
			return e
		}
		items, e := s.repo.GrantAudits(ctx, tx, u.TenantID, u.TaskID)
		if e != nil {
			return e
		}
		for _, g := range items {
			out = append(out, grantView(g))
		}
		return nil
	})
	return out, err
}

func (s *Service) RevokeGrant(ctx context.Context, u facade.User, id int64, req facade.RevokeRequest) (out facade.Receipt, err error) {
	if id <= 0 || req.ExpectedVersion <= 0 || !domain.ValidKey(req.IdempotencyKey) {
		return out, domain.ErrInvalid
	}
	requestDigest := domain.Digest(struct {
		GrantID int64
		ActorID int64
		Request facade.RevokeRequest
	}{id, u.UserID, req})
	err = s.transaction(ctx, func(tx pgx.Tx) error {
		if _, e := s.user(ctx, tx, u, facade.ActionRevoke); e != nil {
			return e
		}
		g, e := s.repo.GetGrant(ctx, tx, u.TenantID, u.TaskID, id, true)
		if e != nil {
			return e
		}
		if g.RevokeIdempotencyKey != "" {
			if g.RevokeIdempotencyKey != req.IdempotencyKey || g.RevokeRequestDigest != requestDigest {
				return domain.ErrIdempotencyConflict
			}
			out = facade.Receipt{GrantID: g.ID, Version: g.Version, Status: g.Status, Replayed: true}
			return nil
		}
		if e = g.Revoke(req.ExpectedVersion, s.now()); e != nil {
			return e
		}
		g.RevokeIdempotencyKey, g.RevokeRequestDigest = req.IdempotencyKey, requestDigest
		if e = s.repo.UpdateGrant(ctx, tx, g); e != nil {
			return e
		}
		operations, e := s.repo.OperationsForGrant(ctx, tx, u.TenantID, u.TaskID, g.ID)
		if e != nil {
			return e
		}
		for _, operation := range operations {
			before := operation.Version
			if e = operation.Close(false, s.now()); e != nil {
				return e
			}
			if operation.Version == before {
				continue
			}
			if e = s.repo.UpdateOperation(ctx, tx, operation); e != nil {
				return e
			}
			if e = s.repo.CancelOperationAttempts(ctx, tx, operation.ID, s.now()); e != nil {
				return e
			}
			if e = s.project(ctx, tx, operation, false); e != nil {
				return e
			}
		}
		out = facade.Receipt{GrantID: g.ID, Version: g.Version, Status: g.Status}
		return nil
	})
	return out, err
}

func (s *Service) Claim(ctx context.Context, m facade.Machine, req facade.ClaimRequest) (out facade.Claim, err error) {
	if req.OperationID <= 0 {
		return out, domain.ErrInvalid
	}
	err = s.transaction(ctx, func(tx pgx.Tx) error {
		t, e := s.machine(ctx, tx, m, facade.ActionClaim)
		if e != nil {
			return e
		}
		o, e := s.repo.GetOperation(ctx, tx, t.TenantID, t.TaskID, req.OperationID, true)
		if e != nil {
			return e
		}
		out, e = s.claimLocked(ctx, tx, m, t, o)
		return e
	})
	return out, err
}

func (s *Service) ClaimNext(ctx context.Context, m facade.Machine) (out facade.Claim, err error) {
	err = s.transaction(ctx, func(tx pgx.Tx) error {
		t, e := s.machine(ctx, tx, m, facade.ActionClaim)
		if e != nil {
			return e
		}
		recoverable, e := s.repo.RecoverableClaim(ctx, tx, t.TenantID, t.TaskID, t.AttemptID, t.AdapterSessionGeneration, t.WorkerID, t.OwnerEpoch, m.InstanceID, m.HostBootID, domain.Digest(m.ExecutionLease))
		if e != nil {
			return e
		}
		if recoverable != nil {
			out, e = s.claimLocked(ctx, tx, m, t, *recoverable)
			return e
		}
		o, e := s.repo.NextReadyOperation(ctx, tx, t.TenantID, t.TaskID, t.AttemptID, t.AdapterSessionGeneration)
		if e != nil {
			return e
		}
		if o == nil {
			return domain.ErrNotFound
		}
		out, e = s.claimLocked(ctx, tx, m, t, *o)
		return e
	})
	return out, err
}

func (s *Service) Renew(ctx context.Context, m facade.Machine, req facade.RenewRequest) (out facade.Receipt, err error) {
	if req.OperationID <= 0 || req.AttemptID <= 0 {
		return out, domain.ErrInvalid
	}
	err = s.transaction(ctx, func(tx pgx.Tx) error {
		t, e := s.machine(ctx, tx, m, facade.ActionRenew)
		if e != nil {
			return e
		}
		o, a, e := s.lockOperationAttempt(ctx, tx, m, t, req.OperationID, req.AttemptID)
		if e != nil {
			return e
		}
		if a.Status != domain.AttemptDispatched && a.Status != domain.AttemptRunning {
			return domain.ErrStale
		}
		if o.Status != domain.StatusDispatched && o.Status != domain.StatusRunning {
			return domain.ErrFenced
		}
		a.LeaseExpiresAt, a.UpdatedAt = s.now().Add(s.leaseTTL), s.now()
		if e = s.repo.UpdateAttempt(ctx, tx, a); e != nil {
			return e
		}
		out = receipt(o, a.ID, 0, false)
		return nil
	})
	return out, err
}

func (s *Service) claimLocked(ctx context.Context, tx pgx.Tx, m facade.Machine, t facade.Target, o domain.Operation) (facade.Claim, error) {
	if !operationMatchesTarget(o, t) {
		return facade.Claim{}, domain.ErrFenced
	}
	if existing, e := s.repo.AttemptForOperation(ctx, tx, o.ID, true); e != nil {
		return facade.Claim{}, e
	} else if existing != nil {
		if !s.attemptAuthorityMatches(m, t, *existing) ||
			(existing.Status != domain.AttemptDispatched && existing.Status != domain.AttemptRunning) ||
			(o.Status != domain.StatusDispatched && o.Status != domain.StatusRunning) || !s.now().Before(existing.LeaseExpiresAt) {
			return facade.Claim{}, domain.ErrFenced
		}
		var view facade.OperationView
		if e = s.view(ctx, o, true, &view); e != nil {
			return facade.Claim{}, e
		}
		return facade.Claim{Operation: view, AttemptID: existing.ID, LeaseExpiresAt: existing.LeaseExpiresAt}, nil
	}
	if o.AuthorizationGrantID > 0 {
		g, e := s.repo.GetGrant(ctx, tx, t.TenantID, t.TaskID, o.AuthorizationGrantID, true)
		if e != nil {
			return facade.Claim{}, domain.ErrGrantRevoked
		}
		if !g.Covers(o.Targets, s.now()) {
			if g.Status == domain.GrantActive {
				return facade.Claim{}, domain.ErrGrantExpired
			}
			return facade.Claim{}, domain.ErrGrantRevoked
		}
	}
	if e := o.Dispatch(s.now()); e != nil {
		return facade.Claim{}, e
	}
	now := s.now()
	attempt := domain.Attempt{ID: s.ids.NextID(), TenantID: t.TenantID, TaskID: t.TaskID, OperationID: o.ID, WorkerID: m.WorkerID, OwnerEpoch: m.OwnerEpoch, AttemptNumber: 1, WorkerInstanceID: m.InstanceID, HostBootID: m.HostBootID, LeaseDigest: domain.Digest(m.ExecutionLease), LeaseExpiresAt: now.Add(s.leaseTTL), Status: domain.AttemptDispatched, CreatedAt: now, UpdatedAt: now}
	if e := s.repo.InsertAttempt(ctx, tx, attempt); e != nil {
		return facade.Claim{}, e
	}
	if e := s.repo.UpdateOperation(ctx, tx, o); e != nil {
		return facade.Claim{}, e
	}
	var view facade.OperationView
	if e := s.view(ctx, o, true, &view); e != nil {
		return facade.Claim{}, e
	}
	return facade.Claim{Operation: view, AttemptID: attempt.ID, LeaseExpiresAt: attempt.LeaseExpiresAt}, nil
}

func (s *Service) Start(ctx context.Context, m facade.Machine, req facade.StartRequest) (out facade.Receipt, err error) {
	if req.OperationID <= 0 || req.AttemptID <= 0 || strings.TrimSpace(req.ContainerID) == "" || strings.TrimSpace(req.RelayIdentity) == "" {
		return out, domain.ErrInvalid
	}
	err = s.transaction(ctx, func(tx pgx.Tx) error {
		t, e := s.machine(ctx, tx, m, facade.ActionStart)
		if e != nil {
			return e
		}
		o, a, e := s.lockOperationAttempt(ctx, tx, m, t, req.OperationID, req.AttemptID)
		if e != nil {
			return e
		}
		if a.Status == domain.AttemptRunning && o.Status == domain.StatusRunning {
			if a.ContainerID != req.ContainerID || a.RelayIdentity != req.RelayIdentity {
				return domain.ErrIdempotencyConflict
			}
			out = receipt(o, a.ID, 0, true)
			return nil
		}
		if a.Status != domain.AttemptDispatched {
			return domain.ErrStale
		}
		if e = o.Start(s.now()); e != nil {
			return e
		}
		now := s.now()
		a.Status, a.ContainerID, a.RelayIdentity, a.StartedAt, a.UpdatedAt = domain.AttemptRunning, req.ContainerID, req.RelayIdentity, &now, now
		if e = s.repo.UpdateAttempt(ctx, tx, a); e != nil {
			return e
		}
		if e = s.repo.UpdateOperation(ctx, tx, o); e != nil {
			return e
		}
		if e = s.project(ctx, tx, o, false); e != nil {
			return e
		}
		out = receipt(o, a.ID, 0, false)
		return nil
	})
	return out, err
}

func (s *Service) SubmitResult(ctx context.Context, m facade.Machine, req facade.ResultRequest) (out facade.Receipt, err error) {
	if req.OperationID <= 0 || req.AttemptID <= 0 {
		return out, domain.ErrInvalid
	}
	if !s.configured() {
		return out, domain.ErrUnavailable
	}
	output, e := domain.NormalizeOutput(req.Output)
	if e != nil {
		return out, e
	}
	req.Summary = strings.TrimSpace(req.Summary)
	if req.ResultDigest != domain.ResultDigest(req.Status, req.ExitCode, req.Summary, output) {
		return out, domain.ErrInvalid
	}
	encryptedOutput, e := s.cipher.Encrypt(ctx, []byte(output))
	if e != nil || encryptedOutput.Ciphertext == "" || encryptedOutput.EDEK == "" || encryptedOutput.KeyRef == "" {
		return out, domain.ErrUnavailable
	}
	err = s.transaction(ctx, func(tx pgx.Tx) error {
		t, e := s.machine(ctx, tx, m, facade.ActionResult)
		if e != nil {
			return e
		}
		o, a, e := s.lockOperationAttempt(ctx, tx, m, t, req.OperationID, req.AttemptID)
		if e != nil {
			return e
		}
		if a.ResultDigest != "" {
			if a.ResultDigest != req.ResultDigest || o.Status != req.Status {
				return domain.ErrIdempotencyConflict
			}
			out = receipt(o, a.ID, 0, true)
			return nil
		}
		if e = o.Finish(req.Status, req.ResultDigest, req.Summary, req.ExitCode, s.now()); e != nil {
			return e
		}
		o.ResultOutput = encryptedOutput
		now := s.now()
		a.Status, a.ResultDigest, a.ExitCode, a.FinishedAt, a.UpdatedAt = attemptStatus(req.Status), req.ResultDigest, req.ExitCode, &now, now
		if e = s.repo.UpdateAttempt(ctx, tx, a); e != nil {
			return e
		}
		if e = s.repo.UpdateOperation(ctx, tx, o); e != nil {
			return e
		}
		if e = s.project(ctx, tx, o, false); e != nil {
			return e
		}
		out = receipt(o, a.ID, 0, false)
		return nil
	})
	return out, err
}

func (s *Service) PullResults(ctx context.Context, m facade.Machine, limit int) (out []facade.ToolResult, err error) {
	if limit == 0 {
		limit = 20
	}
	if limit < 1 || limit > 50 {
		return nil, domain.ErrInvalid
	}
	err = s.transaction(ctx, func(tx pgx.Tx) error {
		t, e := s.machine(ctx, tx, m, facade.ActionPull)
		if e != nil {
			return e
		}
		items, e := s.repo.PendingResults(ctx, tx, t.TenantID, t.TaskID, t.AttemptID, t.AdapterSessionGeneration, limit)
		if e != nil {
			return e
		}
		for _, o := range items {
			result, e := s.toolResult(ctx, o)
			if e != nil {
				return e
			}
			out = append(out, result)
		}
		return nil
	})
	return out, err
}

func (s *Service) CloseToolCall(ctx context.Context, m facade.Machine, req facade.CloseRequest) (out facade.Receipt, err error) {
	if !domain.ValidKey(req.RoundKey) || !domain.ValidKey(req.LocalToolCallKey) {
		return out, domain.ErrInvalid
	}
	err = s.transaction(ctx, func(tx pgx.Tx) error {
		t, e := s.machine(ctx, tx, m, facade.ActionClose)
		if e != nil {
			return e
		}
		o, e := s.repo.FindOccurrence(ctx, tx, t.AttemptID, t.AdapterSessionGeneration, req.LocalToolCallKey)
		if e != nil {
			return e
		}
		if o == nil || o.TenantID != t.TenantID || o.TaskID != t.TaskID {
			return domain.ErrNotFound
		}
		if o.RoundKey != req.RoundKey {
			return domain.ErrIdempotencyConflict
		}
		before := o.Version
		if e = o.Close(true, s.now()); e != nil {
			return e
		}
		if o.Version != before {
			if e = s.repo.UpdateOperation(ctx, tx, *o); e != nil {
				return e
			}
			if e = s.repo.CancelOperationAttempts(ctx, tx, o.ID, s.now()); e != nil {
				return e
			}
			if e = s.project(ctx, tx, *o, false); e != nil {
				return e
			}
		}
		out = receipt(*o, 0, 0, o.Version == before)
		return nil
	})
	return out, err
}

func (s *Service) AckResult(ctx context.Context, m facade.Machine, req facade.ResultAckRequest) (out facade.Receipt, err error) {
	if req.OperationID <= 0 {
		return out, domain.ErrInvalid
	}
	err = s.transaction(ctx, func(tx pgx.Tx) error {
		t, e := s.machine(ctx, tx, m, facade.ActionAck)
		if e != nil {
			return e
		}
		o, e := s.repo.GetOperation(ctx, tx, t.TenantID, t.TaskID, req.OperationID, true)
		if e != nil {
			return e
		}
		if o.AttemptID != t.AttemptID || o.AdapterSessionGeneration != t.AdapterSessionGeneration {
			return domain.ErrFenced
		}
		before := o.Version
		if e = o.MarkDelivered(req.NativeResolved, req.AgentContinued, s.now()); e != nil {
			return e
		}
		if o.Version != before {
			if e = s.repo.UpdateOperation(ctx, tx, o); e != nil {
				return e
			}
			if e = s.project(ctx, tx, o, false); e != nil {
				return e
			}
		}
		out = receipt(o, 0, 0, o.Version == before)
		return nil
	})
	return out, err
}

func (s *Service) lockOperationAttempt(ctx context.Context, tx pgx.Tx, m facade.Machine, t facade.Target, operationID, attemptID int64) (domain.Operation, domain.Attempt, error) {
	o, e := s.repo.GetOperation(ctx, tx, t.TenantID, t.TaskID, operationID, true)
	if e != nil {
		return o, domain.Attempt{}, e
	}
	if !operationMatchesTarget(o, t) {
		return o, domain.Attempt{}, domain.ErrFenced
	}
	a, e := s.repo.GetAttempt(ctx, tx, operationID, attemptID, true)
	if e != nil {
		return o, a, e
	}
	if !s.attemptAuthorityMatches(m, t, a) {
		return o, a, domain.ErrFenced
	}
	return o, a, nil
}

func (s *Service) attemptAuthorityMatches(m facade.Machine, t facade.Target, a domain.Attempt) bool {
	return a.TenantID == t.TenantID && a.TaskID == t.TaskID &&
		a.WorkerID == m.WorkerID && a.WorkerID == t.WorkerID &&
		a.WorkerInstanceID == m.InstanceID && a.HostBootID == m.HostBootID &&
		a.OwnerEpoch == m.OwnerEpoch && a.OwnerEpoch == t.OwnerEpoch &&
		a.LeaseDigest == domain.Digest(m.ExecutionLease) && s.now().Before(a.LeaseExpiresAt)
}

// ReadResultReceipt confirms an exact terminal result after SubmitResult may
// have committed while its HTTP response was lost. It intentionally does not
// decrypt output, claim work, extend a lease, or mutate either record.
func (s *Service) ReadResultReceipt(ctx context.Context, m facade.Machine, req facade.ResultReceiptRequest) (out facade.Receipt, err error) {
	if req.OperationID <= 0 || req.AttemptID <= 0 || !domain.ValidDigest(req.ResultDigest) {
		return out, domain.ErrInvalid
	}
	err = s.transaction(ctx, func(tx pgx.Tx) error {
		t, e := s.machine(ctx, tx, m, facade.ActionReceipt)
		if e != nil {
			return e
		}
		o, a, e := s.lockOperationAttempt(ctx, tx, m, t, req.OperationID, req.AttemptID)
		if e != nil {
			return e
		}
		if a.ResultDigest == "" || o.ResultDigest == "" {
			return domain.ErrStale
		}
		if a.ResultDigest != req.ResultDigest || o.ResultDigest != req.ResultDigest {
			return domain.ErrIdempotencyConflict
		}
		if a.Status != attemptStatus(o.Status) || (o.Status != domain.StatusSucceeded && o.Status != domain.StatusFailed && o.Status != domain.StatusOutcomeUnknown) {
			return domain.ErrFenced
		}
		out = receipt(o, a.ID, 0, true)
		return nil
	})
	return out, err
}

// ReadResult returns the already persisted result for one exactly fenced
// Operation. It is used by cold resume after a deny or after the runner result
// has been committed; it never claims or executes work.
func (s *Service) ReadResult(ctx context.Context, m facade.Machine, operationID int64) (out facade.ToolResult, err error) {
	if operationID <= 0 {
		return out, domain.ErrInvalid
	}
	err = s.transaction(ctx, func(tx pgx.Tx) error {
		t, e := s.machine(ctx, tx, m, facade.ActionPull)
		if e != nil {
			return e
		}
		o, e := s.repo.GetOperation(ctx, tx, t.TenantID, t.TaskID, operationID, false)
		if e != nil {
			return e
		}
		if !operationMatchesTarget(o, t) || o.ResultDeliveryStatus == "" {
			return domain.ErrStale
		}
		out, e = s.toolResult(ctx, o)
		return e
	})
	return out, err
}

func (s *Service) CancelTaskTx(ctx context.Context, tx pgx.Tx, tenant, task int64, now time.Time) error {
	if s == nil || s.repo == nil || s.projector == nil || tenant <= 0 || task <= 0 {
		return domain.ErrInvalid
	}
	operations, err := s.repo.CancellableOperations(ctx, tx, tenant, task)
	if err != nil {
		return safe(err)
	}
	for _, o := range operations {
		o.Status, o.ResultDeliveryStatus, o.UpdatedAt, o.FinishedAt = domain.StatusCancelled, domain.DeliveryNativeResolved, now, &now
		o.Version++
		if err = s.repo.UpdateOperation(ctx, tx, o); err != nil {
			return safe(err)
		}
		if err = s.project(ctx, tx, o, false); err != nil {
			return safe(err)
		}
	}
	grants, err := s.repo.TaskGrants(ctx, tx, tenant, task)
	if err != nil {
		return safe(err)
	}
	for _, g := range grants {
		if g.Status != domain.GrantActive {
			continue
		}
		g.Status, g.UpdatedAt, g.RevokedAt = domain.GrantRevoked, now, &now
		g.Version++
		if err = s.repo.UpdateGrant(ctx, tx, g); err != nil {
			return safe(err)
		}
	}
	if err = s.repo.CancelActiveAttempts(ctx, tx, tenant, task, now); err != nil {
		return safe(err)
	}
	return nil
}

// ReconcileExpired is a bounded background entry point. It never executes an
// operation: an expired active lease becomes OUTCOME_UNKNOWN, while approval
// and grant expiry close work that never acquired durable authority.
func (s *Service) ReconcileExpired(ctx context.Context, limit int) (int, error) {
	if limit < 1 || limit > 200 {
		return 0, domain.ErrInvalid
	}
	count := 0
	err := s.transaction(ctx, func(tx pgx.Tx) error {
		now := s.now()
		uncertain, err := s.repo.ExpiredClaimOperations(ctx, tx, limit, now)
		if err != nil {
			return err
		}
		for _, o := range uncertain {
			summary := "Operation lease expired before result confirmation."
			o.Status, o.ResultDeliveryStatus, o.ResultSummary = domain.StatusOutcomeUnknown, domain.DeliverySaved, summary
			o.ResultDigest, o.ExitCode, o.UpdatedAt, o.FinishedAt = domain.ResultDigest(domain.StatusOutcomeUnknown, nil, summary, ""), nil, now, &now
			o.Version++
			if err = s.repo.UpdateOperation(ctx, tx, o); err != nil {
				return err
			}
			if err = s.repo.CancelOperationAttempts(ctx, tx, o.ID, now); err != nil {
				return err
			}
			if err = s.project(ctx, tx, o, false); err != nil {
				return err
			}
			count++
		}
		operations, err := s.repo.ExpiredOperations(ctx, tx, limit, now)
		if err != nil {
			return err
		}
		for _, o := range operations {
			o.Status, o.ResultDeliveryStatus, o.UpdatedAt, o.FinishedAt = domain.StatusExpired, domain.DeliverySaved, now, &now
			o.Version++
			if err = s.repo.UpdateOperation(ctx, tx, o); err != nil {
				return err
			}
			if err = s.project(ctx, tx, o, false); err != nil {
				return err
			}
			count++
		}
		grants, err := s.repo.ExpiredGrants(ctx, tx, limit, now)
		if err != nil {
			return err
		}
		for _, g := range grants {
			g.Refresh(now)
			if err = s.repo.UpdateGrant(ctx, tx, g); err != nil {
				return err
			}
			operations, operationErr := s.repo.OperationsForGrant(ctx, tx, g.TenantID, g.TaskID, g.ID)
			if operationErr != nil {
				return operationErr
			}
			for _, operation := range operations {
				operation.Status, operation.ResultDeliveryStatus, operation.UpdatedAt, operation.FinishedAt = domain.StatusExpired, domain.DeliverySaved, now, &now
				operation.Version++
				if err = s.repo.UpdateOperation(ctx, tx, operation); err != nil {
					return err
				}
				if err = s.repo.CancelOperationAttempts(ctx, tx, operation.ID, now); err != nil {
					return err
				}
				if err = s.project(ctx, tx, operation, false); err != nil {
					return err
				}
			}
			count++
		}
		return nil
	})
	return count, err
}

func (s *Service) view(ctx context.Context, o domain.Operation, includeDescriptor bool, out *facade.OperationView) error {
	*out = facade.OperationView{ID: o.ID, TaskID: o.TaskID, TaskTurnID: o.TaskTurnID, AttemptID: o.AttemptID, Version: o.Version, Kind: o.Kind, Status: o.Status, Purpose: o.Purpose, Targets: append([]domain.Target(nil), o.Targets...), DescriptorDigest: o.DescriptorDigest, ResultDeliveryStatus: o.ResultDeliveryStatus, AgentContinued: o.AgentContinued, ResultDigest: o.ResultDigest, ResultSummary: o.ResultSummary, ExitCode: o.ExitCode, ExpiresAt: o.ExpiresAt, CreatedAt: o.CreatedAt, UpdatedAt: o.UpdatedAt}
	if !includeDescriptor {
		return nil
	}
	raw, err := s.cipher.Decrypt(ctx, o.Descriptor)
	if err != nil {
		return domain.ErrUnavailable
	}
	var descriptor domain.Descriptor
	if json.Unmarshal(raw, &descriptor) != nil {
		return domain.ErrUnavailable
	}
	normalized, err := domain.NormalizeDescriptor(descriptor)
	if err != nil || domain.Digest(normalized) != o.DescriptorDigest {
		return domain.ErrUnavailable
	}
	out.Command, out.Cwd = normalized.Command, normalized.Cwd
	return nil
}

func receipt(o domain.Operation, attemptID, grantID int64, replay bool) facade.Receipt {
	return facade.Receipt{OperationID: o.ID, AttemptID: attemptID, GrantID: grantID, Version: o.Version, Status: o.Status, ResultDigest: o.ResultDigest, Replayed: replay}
}
func grantView(a domain.GrantAudit) facade.GrantView {
	g := a.Grant
	out := facade.GrantView{ID: g.ID, TaskID: g.TaskID, SourceDecisionID: g.SourceDecisionID, Version: g.Version, Targets: append([]domain.Target(nil), g.Targets...), Status: g.Status, ActorUserID: a.ActorUserID, Purpose: a.Purpose, ExpiresAt: g.ExpiresAt, CreatedAt: g.CreatedAt, UpdatedAt: g.UpdatedAt}
	if a.LastOperation != nil {
		out.LastOperation = &facade.GrantOperationView{Status: a.LastOperation.Status, ResultSummary: a.LastOperation.ResultSummary, ResultDeliveryStatus: a.LastOperation.ResultDeliveryStatus, ExitCode: a.LastOperation.ExitCode, AgentContinued: a.LastOperation.AgentContinued, FinishedAt: a.LastOperation.FinishedAt}
	}
	return out
}
func attemptStatus(status string) string {
	if status == domain.StatusSucceeded {
		return domain.AttemptSucceeded
	}
	if status == domain.StatusFailed {
		return domain.AttemptFailed
	}
	return domain.AttemptUnknown
}
func (s *Service) toolResult(ctx context.Context, o domain.Operation) (facade.ToolResult, error) {
	r := facade.ToolResult{OperationID: o.ID, RoundKey: o.RoundKey, LocalToolCallKey: o.LocalToolCallKey, NativeToolCallID: o.NativeToolCallID, AdapterSessionGeneration: o.AdapterSessionGeneration, Status: o.Status, ExitCode: o.ExitCode, Summary: o.ResultSummary, ResultDigest: o.ResultDigest}
	if o.ResultOutput.Ciphertext != "" {
		output, err := s.cipher.Decrypt(ctx, o.ResultOutput)
		if err != nil {
			return facade.ToolResult{}, domain.ErrUnavailable
		}
		r.Output = string(output)
	}
	if o.Status == domain.StatusDenied {
		r.ReasonCode = "USER_DECLINED_NETWORK_ACCESS"
	}
	if o.Status == domain.StatusOutcomeUnknown {
		r.ReasonCode = "OPERATION_RESULT_NOT_RECOVERABLE"
	}
	if o.Status == domain.StatusCancelled {
		r.ReasonCode = "EXECUTION_OPERATION_CANCELLED"
	}
	if o.Status == domain.StatusExpired {
		r.ReasonCode = "EXECUTION_OPERATION_EXPIRED"
	}
	return r, nil
}
