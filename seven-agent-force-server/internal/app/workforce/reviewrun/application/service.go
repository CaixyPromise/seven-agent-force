// Package application coordinates the ReviewRun aggregate and the small
// machine-control contract used by Slice 4's isolated Reviewer runtime. It
// owns neither Docker mechanics nor direct Provider credentials.
package application

import (
	"context"
	"strings"

	"github.com/jackc/pgx/v5"

	"github.com/CaixyPromise/seven-agent/force/internal/app/workforce/reviewcontract"
	"github.com/CaixyPromise/seven-agent/force/internal/app/workforce/reviewrun/domain"
	"github.com/CaixyPromise/seven-agent/force/internal/app/workforce/reviewrun/facade"
	"github.com/CaixyPromise/seven-agent/force/internal/app/workforce/reviewrun/infrastructure"
	taskfacade "github.com/CaixyPromise/seven-agent/force/internal/app/workforce/task/facade"
	"github.com/CaixyPromise/seven-agent/force/internal/shared/stepup"
)

type Service struct {
	store      *infrastructure.Store
	taskInputs taskfacade.IndependentReviewInputReader
}

func NewService(store *infrastructure.Store) *Service { return &Service{store: store} }

func (s *Service) FinalizationGate() facade.FinalizationGate { return s }
func (s *Service) TaskLifecycle() facade.TaskLifecycle       { return s }
func (s *Service) TaskSummaryProvider() facade.TaskSummaryProvider {
	return s
}
func (s *Service) TaskReportProvider() facade.TaskReportProvider { return s }
func (s *Service) WorkspaceRetention() facade.WorkspaceRetention { return s }
func (s *Service) MachineControl() facade.MachineControl         { return s }
func (s *Service) Scheduler() facade.Scheduler                   { return s }
func (s *Service) HumanClosure() facade.HumanClosure             { return s }
func (s *Service) TaskReviewReader() facade.TaskReviewReader     { return s }

func (s *Service) Decide(ctx context.Context, request facade.HumanDecisionRequest) (facade.HumanDecisionResult, bool, error) {
	if s == nil || s.store == nil || request.TenantID < 1 || request.ReviewRunID < 1 || request.ActorUserID < 1 || !domain.ValidateHumanAction(request.Action) {
		return facade.HumanDecisionResult{}, false, domain.ErrHumanActionNotAllowed
	}
	reason, err := domain.NormalizeHumanReason(request.Reason)
	if err != nil {
		return facade.HumanDecisionResult{}, false, err
	}
	request.Reason = reason
	guidance, err := domain.NormalizeRevisionGuidance(request.Action, request.RevisionGuidance)
	if err != nil {
		return facade.HumanDecisionResult{}, false, err
	}
	request.RevisionGuidance = guidance
	request.IdempotencyKey = strings.TrimSpace(request.IdempotencyKey)
	if request.IdempotencyKey == "" || len(request.IdempotencyKey) > 128 {
		return facade.HumanDecisionResult{}, false, domain.ErrHumanIdempotency
	}
	binding := domain.HumanDecisionOperationBinding(request.ReviewRunID, request.Action, request.Reason, request.RevisionGuidance, request.IdempotencyKey)
	if err = stepup.Require(request.Proof, domain.BusinessActionHumanDecision, binding); err != nil {
		return facade.HumanDecisionResult{}, false, err
	}
	return s.store.Decide(ctx, request)
}

// BindExecutionInputs connects only stable, read-only cross-module facades.
// The Reviewer never reads Task or Review Policy private stores directly.
func (s *Service) BindExecutionInputs(taskInputs taskfacade.IndependentReviewInputReader) {
	if s != nil {
		s.taskInputs = taskInputs
	}
}

func (s *Service) GateVerifiedFinalization(ctx context.Context, tx pgx.Tx, input facade.FinalizationInput) (facade.FinalizationDecision, error) {
	if s == nil || s.store == nil {
		return facade.FinalizationDecision{}, domain.ErrInvalidReviewInput
	}
	return s.store.GateVerifiedFinalization(ctx, tx, input)
}

func (s *Service) CancelForTask(ctx context.Context, tx pgx.Tx, taskID int64, reason string) error {
	if s == nil || s.store == nil || taskID < 1 || strings.TrimSpace(reason) == "" {
		return domain.ErrInvalidReviewInput
	}
	return s.store.CancelForTask(ctx, tx, taskID, strings.TrimSpace(reason))
}

func (s *Service) SummaryForTask(ctx context.Context, tenantID, taskID int64) (domain.Summary, error) {
	if s == nil || s.store == nil || tenantID < 1 || taskID < 1 {
		return domain.Summary{}, domain.ErrInvalidReviewInput
	}
	return s.store.SummaryForTask(ctx, tenantID, taskID)
}

func (s *Service) ReportForTask(ctx context.Context, tenantID, taskID int64) (domain.TaskReport, error) {
	if s == nil || s.store == nil || tenantID < 1 || taskID < 1 {
		return domain.TaskReport{}, domain.ErrInvalidReviewInput
	}
	return s.store.ReportForTask(ctx, tenantID, taskID)
}

func (s *Service) ListRunsForTask(ctx context.Context, tenantID, taskID, cursor int64, limit int) (domain.RunPage, error) {
	if s == nil || s.store == nil || tenantID < 1 || taskID < 1 || cursor < 0 {
		return domain.RunPage{}, domain.ErrInvalidReviewInput
	}
	if limit <= 0 {
		limit = 20
	}
	if limit > 50 {
		limit = 50
	}
	return s.store.ListRunsForTask(ctx, tenantID, taskID, cursor, limit)
}

func (s *Service) RunDetailForTask(ctx context.Context, tenantID, runID int64) (domain.RunView, error) {
	if s == nil || s.store == nil || tenantID < 1 || runID < 1 {
		return domain.RunView{}, domain.ErrInvalidReviewInput
	}
	item, err := s.store.RunDetailForTask(ctx, tenantID, runID)
	if err != nil {
		return domain.RunView{}, err
	}
	return s.hydrateHistoricalRun(ctx, tenantID, item)
}

func (s *Service) ReviewRunRetention(ctx context.Context, taskID int64) (facade.RetentionDecision, error) {
	if s == nil || s.store == nil || taskID < 1 {
		return facade.RetentionDecision{}, domain.ErrInvalidReviewInput
	}
	return s.store.ReviewRunRetention(ctx, taskID)
}

func (s *Service) AssignAvailable(ctx context.Context, limit int) (int, error) {
	if s == nil || s.store == nil {
		return 0, domain.ErrInvalidReviewInput
	}
	return s.store.AssignAvailable(ctx, limit)
}

func (s *Service) ReconcileExpired(ctx context.Context, limit int) (int, error) {
	if s == nil || s.store == nil {
		return 0, domain.ErrInvalidReviewInput
	}
	return s.store.ReconcileExpired(ctx, limit)
}

func (s *Service) Claim(ctx context.Context, envelope facade.MachineEnvelope, runID int64) (facade.Claim, error) {
	// Assignment messages carry the owner epoch. Requiring it on the first
	// claim prevents a stale delivery from being accepted merely because the
	// same Worker instance was assigned a later ReviewRun generation.
	if !validMachineEnvelope(envelope, true) || runID < 1 {
		return facade.Claim{}, domain.ErrFenceRejected
	}
	claim, err := s.store.Claim(ctx, envelope, runID)
	if err != nil {
		return facade.Claim{}, err
	}
	return s.hydrateClaim(ctx, claim)
}

// Recover transfers a running local container to a newly registered Worker
// instance only after the Server compares all durable container identity
// facts.  The old instance is fenced by the worker-session switch and gets no
// new lease or permit.
func (s *Service) Recover(ctx context.Context, envelope facade.MachineEnvelope, runID int64, container facade.ContainerRegistration) (facade.Claim, error) {
	if !validMachineEnvelope(envelope, true) || runID < 1 || strings.TrimSpace(container.ContainerID) == "" || strings.TrimSpace(container.ContainerName) == "" || strings.TrimSpace(container.ImageDigest) == "" || strings.TrimSpace(container.ExecutionIdentityDigest) == "" {
		return facade.Claim{}, domain.ErrFenceRejected
	}
	claim, err := s.store.Recover(ctx, envelope, runID, container)
	if err != nil {
		return facade.Claim{}, err
	}
	return s.hydrateClaim(ctx, claim)
}

func (s *Service) hydrateClaim(ctx context.Context, claim facade.Claim) (facade.Claim, error) {
	if s.taskInputs == nil {
		return facade.Claim{}, domain.ErrInvalidReviewInput
	}
	input, err := s.taskInputs.IndependentReviewInput(ctx, claim.TenantID, claim.TaskID, claim.TaskTurnID)
	if err != nil || input.TenantID != claim.TenantID || input.TaskID != claim.TaskID || input.TaskTurnID != claim.TaskTurnID {
		return facade.Claim{}, domain.ErrFenceRejected
	}
	if !domain.ValidCompletionCheckDigest(claim.CompletionCheckDigest) || input.CompletionCheckDigest != claim.CompletionCheckDigest {
		return facade.Claim{}, domain.ErrFenceRejected
	}
	claim.Requirements, err = reviewRequirements(input)
	if err != nil {
		return facade.Claim{}, domain.ErrFenceRejected
	}
	claim.ReviewContract = reviewcontract.RuntimeContract()
	return claim, nil
}

func (s *Service) RenewLease(ctx context.Context, envelope facade.MachineEnvelope, runID int64, lease string) (string, error) {
	if !validMachineEnvelope(envelope, true) || runID < 1 || strings.TrimSpace(lease) == "" {
		return "", domain.ErrFenceRejected
	}
	return s.store.RenewLease(ctx, envelope, runID, lease)
}

func (s *Service) AcquireLaunchPermit(ctx context.Context, envelope facade.MachineEnvelope, runID int64, lease string) (facade.LaunchPermit, error) {
	if !validMachineEnvelope(envelope, true) || runID < 1 || strings.TrimSpace(lease) == "" {
		return facade.LaunchPermit{}, domain.ErrFenceRejected
	}
	return s.store.AcquireLaunchPermit(ctx, envelope, runID, lease)
}

func (s *Service) RecordContainer(ctx context.Context, envelope facade.MachineEnvelope, runID int64, container facade.ContainerRegistration, lease string, permitID int64, permit string) error {
	if !validMachineEnvelope(envelope, true) || runID < 1 || strings.TrimSpace(lease) == "" || strings.TrimSpace(permit) == "" || permitID < 1 || strings.TrimSpace(container.ContainerID) == "" || strings.TrimSpace(container.ContainerName) == "" || strings.TrimSpace(container.ImageDigest) == "" || strings.TrimSpace(container.ExecutionIdentityDigest) == "" {
		return domain.ErrFenceRejected
	}
	return s.store.RecordContainer(ctx, envelope, runID, container, lease, permitID, permit)
}

func (s *Service) SubmitReport(ctx context.Context, envelope facade.MachineEnvelope, runID int64, submission facade.ReportSubmission) (facade.ReportResult, error) {
	if !validMachineEnvelope(envelope, true) || runID < 1 || strings.TrimSpace(submission.LeaseToken) == "" || strings.TrimSpace(submission.PermitToken) == "" || submission.PermitID < 1 || len(submission.Report) == 0 || len(submission.Report) > domain.MaxCandidateReportBytes {
		return facade.ReportResult{}, domain.ErrFenceRejected
	}
	return s.store.SubmitReport(ctx, envelope, runID, submission)
}

func (s *Service) RecordTransientFailure(ctx context.Context, envelope facade.MachineEnvelope, runID int64, lease, reason string) error {
	if !validMachineEnvelope(envelope, true) || runID < 1 || strings.TrimSpace(lease) == "" || !safeReason(reason) {
		return domain.ErrFenceRejected
	}
	return s.store.RecordTransientFailure(ctx, envelope, runID, lease, strings.TrimSpace(reason))
}

func validMachineEnvelope(v facade.MachineEnvelope, owner bool) bool {
	if v.SchemaVersion != 1 || v.WorkerID < 1 || strings.TrimSpace(v.InstanceID) == "" || strings.TrimSpace(v.HostBootID) == "" || strings.TrimSpace(v.RequestID) == "" || len(v.RequestID) > 128 || v.Generation < 1 {
		return false
	}
	return !owner || v.OwnerEpoch > 0
}

func safeReason(value string) bool {
	if len(value) == 0 || len(value) > 128 {
		return false
	}
	for _, r := range value {
		if !(r == '_' || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9')) {
			return false
		}
	}
	return true
}
