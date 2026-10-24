package control

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	artifactdomain "github.com/CaixyPromise/seven-agent/force/internal/app/workforce/artifact/domain"
	continuationapp "github.com/CaixyPromise/seven-agent/force/internal/app/workforce/continuation/application"
	continuationdomain "github.com/CaixyPromise/seven-agent/force/internal/app/workforce/continuation/domain"
	deliveryapp "github.com/CaixyPromise/seven-agent/force/internal/app/workforce/delivery/application"
	deliverydomain "github.com/CaixyPromise/seven-agent/force/internal/app/workforce/delivery/domain"
	processorfacade "github.com/CaixyPromise/seven-agent/force/internal/app/workforce/resultprocessor/facade"
	reviewrundomain "github.com/CaixyPromise/seven-agent/force/internal/app/workforce/reviewrun/domain"
	"github.com/CaixyPromise/seven-agent/force/internal/app/workforce/shared/ids"
	taskdomain "github.com/CaixyPromise/seven-agent/force/internal/app/workforce/task/domain"
	taskinfra "github.com/CaixyPromise/seven-agent/force/internal/app/workforce/task/infrastructure"
	dispatchdomain "github.com/CaixyPromise/seven-agent/force/internal/app/workforce/taskdispatch/domain"
	registrydomain "github.com/CaixyPromise/seven-agent/force/internal/app/workforce/workerregistry/domain"
	workspaceinputapp "github.com/CaixyPromise/seven-agent/force/internal/app/workforce/workspaceinput/application"
	workspaceinputdomain "github.com/CaixyPromise/seven-agent/force/internal/app/workforce/workspaceinput/domain"
	commonjson "github.com/CaixyPromise/seven-agent/force/internal/common/json"
)

const maxResponseBytes = 2 << 20

type Client struct {
	baseURL  string
	token    string
	workerID int64
	instance string
	hostBoot string
	http     *http.Client
}

func New(baseURL, token string, workerID int64, instanceID, hostBootID string) (*Client, error) {
	parsed, err := url.Parse(strings.TrimRight(strings.TrimSpace(baseURL), "/"))
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return nil, errors.New("worker serverBaseURL is invalid")
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return nil, errors.New("worker serverBaseURL must use http or https")
	}
	if workerID <= 0 || instanceID == "" || hostBootID == "" || len(token) < 32 {
		return nil, errors.New("worker machine identity is incomplete")
	}
	return &Client{baseURL: strings.TrimRight(parsed.String(), "/"), token: token, workerID: workerID, instance: instanceID, hostBoot: hostBootID, http: &http.Client{Timeout: 30 * time.Second}}, nil
}

type RegisterInput struct {
	StartedAt          string          `json:"startedAt"`
	Version            string          `json:"version"`
	Revision           string          `json:"revision"`
	BuildDate          string          `json:"buildDate"`
	MaxActiveAttempts  int             `json:"maxActiveAttempts"`
	MaxActiveReviews   int             `json:"maxActiveReviews"`
	MaxTotalContainers int             `json:"maxTotalContainers"`
	Capabilities       json.RawMessage `json:"capabilities"`
}

type RegisterResult struct {
	RegistrationID           int64                         `json:"registrationId,string"`
	WorkerID                 int64                         `json:"workerId,string"`
	InstanceID               string                        `json:"instanceId"`
	DesiredState             string                        `json:"desiredState"`
	HeartbeatIntervalSeconds int                           `json:"heartbeatIntervalSeconds"`
	StaleAfterSeconds        int                           `json:"staleAfterSeconds"`
	AssignmentLeaseSeconds   int                           `json:"assignmentLeaseSeconds"`
	ReconcileRequired        bool                          `json:"reconcileRequired"`
	WorkerState              string                        `json:"workerState"`
	Reconciliation           registrydomain.Reconciliation `json:"reconciliation"`
	InventoryVersion         int                           `json:"inventoryVersion"`
	Queue                    registrydomain.QueueContract  `json:"queue"`
}

func (c *Client) Reconciliation(ctx context.Context) (registrydomain.Reconciliation, error) {
	var result registrydomain.Reconciliation
	headers := map[string]string{
		"X-Seven-Worker-Instance": c.instance,
		"X-Seven-Host-Boot-ID":    c.hostBoot,
		"X-Seven-Request-ID":      ids.New("req"),
	}
	err := c.call(ctx, http.MethodGet, "/api/internal/workforce/v1/workers/"+idText(c.workerID)+"/reconciliation", nil, "reconciliation-"+c.instance, headers, &result)
	return result, err
}

func (c *Client) Register(ctx context.Context, input RegisterInput) (RegisterResult, error) {
	body := merge(c.envelope(0), input)
	var result RegisterResult
	err := c.call(ctx, http.MethodPost, "/api/internal/workforce/v1/workers/"+idText(c.workerID)+"/register", body, ids.New("req"), nil, &result)
	return result, err
}

type HeartbeatInput struct {
	Sequence          int64           `json:"sequence,string"`
	ConsumerState     string          `json:"consumerState"`
	DockerReady       bool            `json:"dockerReady"`
	ActiveAttempts    int             `json:"activeAttempts"`
	MaxActiveAttempts int             `json:"maxActiveAttempts"`
	Hostname          string          `json:"hostname"`
	ReportedIP        string          `json:"reportedIp"`
	PrimaryInterface  string          `json:"primaryInterface"`
	PrimaryMAC        string          `json:"primaryMac"`
	Interfaces        json.RawMessage `json:"interfaces"`
	ResourceSnapshot  json.RawMessage `json:"resourceSnapshot"`
}

type HeartbeatResult struct {
	DesiredState               string `json:"desiredState"`
	DerivedState               string `json:"derivedState"`
	ConsumerDirective          string `json:"consumerDirective"`
	RefreshInventoryGeneration int    `json:"refreshInventoryGeneration"`
}

// WorkspaceGCEligibility is a Server-derived retention decision. The Worker
// deliberately receives no task body, credentials or provider data.
type WorkspaceGCEligibility struct {
	TaskID                 int64     `json:"taskId,string"`
	Terminal               bool      `json:"terminal"`
	TerminalAt             time.Time `json:"terminalAt,omitempty"`
	ActiveReferences       bool      `json:"activeReferences"`
	PortableStateAvailable bool      `json:"portableStateAvailable"`
	ReasonCode             string    `json:"reasonCode"`
}

type ReviewClaim struct {
	ReviewRunID                int64           `json:"reviewRunId,string"`
	ReviewRunAttemptID         int64           `json:"reviewRunAttemptId,string"`
	TenantID                   int64           `json:"tenantId,string"`
	TaskID                     int64           `json:"taskId,string"`
	TaskTurnID                 int64           `json:"taskTurnId,string"`
	FinalizationRunID          int64           `json:"finalizationRunId,string"`
	ResultCommit               string          `json:"resultCommit"`
	ResultTree                 string          `json:"resultTree"`
	ReviewerImage              string          `json:"reviewerImageDigest"`
	ModelCredentialID          int64           `json:"modelCredentialId,string"`
	CredentialVersion          int64           `json:"credentialVersion,string"`
	ModelProtocol              string          `json:"modelProtocol"`
	PrimaryModel               string          `json:"primaryModel"`
	ProviderStructuredContract string          `json:"providerStructuredOutputContract"`
	ProviderProxyImageDigest   string          `json:"providerProxyImageDigest,omitempty"`
	SystemContract             string          `json:"systemContract"`
	ReportSchemaVersion        string          `json:"reportSchemaVersion"`
	HardTimeoutSeconds         int             `json:"hardTimeoutSeconds"`
	ReviewContract             json.RawMessage `json:"reviewContract"`
	Requirements               json.RawMessage `json:"requirements"`
	OwnerEpoch                 int64           `json:"ownerEpoch,string"`
	LeaseToken                 string          `json:"reviewLeaseToken"`
	LeaseExpiresAt             string          `json:"reviewLeaseExpiresAt"`
	ExecutionDeadlineAt        string          `json:"executionDeadlineAt"`
}

type ReviewLaunchPermit struct {
	PermitID    int64  `json:"permitId,string"`
	PermitToken string `json:"permitToken"`
	ExpiresAt   string `json:"expiresAt"`
	OwnerEpoch  int64  `json:"ownerEpoch,string"`
}

type ReviewReportResult struct {
	Status       string `json:"status"`
	Verdict      string `json:"verdict"`
	ReportDigest string `json:"reportDigest"`
}

func (c *Client) ReviewClaim(ctx context.Context, runID, ownerEpoch int64) (ReviewClaim, error) {
	body := c.reviewEnvelope(ownerEpoch)
	var result struct {
		Claim ReviewClaim `json:"claim"`
	}
	err := c.call(ctx, http.MethodPost, "/api/internal/workforce/v1/review-runs/"+idText(runID)+"/claims", body, scopedIdempotencyKey("review-claim", c.instance, idText(runID), strconv.FormatInt(ownerEpoch, 10)), nil, &result)
	return result.Claim, err
}

func (c *Client) RecoverReviewRun(ctx context.Context, runID, ownerEpoch int64, containerID, containerName, imageDigest, identity string) (ReviewClaim, error) {
	body := merge(c.reviewEnvelope(ownerEpoch), map[string]any{"containerId": containerID, "containerName": containerName, "imageDigest": imageDigest, "executionIdentityDigest": identity})
	var result struct {
		Claim ReviewClaim `json:"claim"`
	}
	err := c.call(ctx, http.MethodPost, "/api/internal/workforce/v1/review-runs/"+idText(runID)+"/recover", body, scopedIdempotencyKey("review-recover", c.instance, idText(runID), containerID), nil, &result)
	return result.Claim, err
}

func (c *Client) RenewReviewLease(ctx context.Context, runID, ownerEpoch int64, lease string) (LeaseRenewal, error) {
	body := merge(c.reviewEnvelope(ownerEpoch), map[string]any{"reviewLeaseToken": lease})
	var result LeaseRenewal
	err := c.call(ctx, http.MethodPut, "/api/internal/workforce/v1/review-runs/"+idText(runID)+"/lease", body, scopedIdempotencyKey("review-lease", c.instance, idText(runID), strconv.FormatInt(ownerEpoch, 10), ids.New("renew")), nil, &result)
	return result, err
}

func (c *Client) ReviewLaunchPermit(ctx context.Context, runID, ownerEpoch int64, lease string) (ReviewLaunchPermit, error) {
	body := merge(c.reviewEnvelope(ownerEpoch), map[string]any{"reviewLeaseToken": lease})
	var result struct {
		Permit ReviewLaunchPermit `json:"permit"`
	}
	err := c.call(ctx, http.MethodPost, "/api/internal/workforce/v1/review-runs/"+idText(runID)+"/launch-permits", body, scopedIdempotencyKey("review-launch", c.instance, idText(runID), strconv.FormatInt(ownerEpoch, 10)), nil, &result)
	return result.Permit, err
}

func (c *Client) ReviewContainerCreated(ctx context.Context, runID, ownerEpoch int64, lease string, permit ReviewLaunchPermit, containerID, containerName, imageDigest, identity string) error {
	body := merge(c.reviewEnvelope(ownerEpoch), map[string]any{"reviewLeaseToken": lease, "permitId": idText(permit.PermitID), "permitToken": permit.PermitToken, "containerId": containerID, "containerName": containerName, "imageDigest": imageDigest, "executionIdentityDigest": identity})
	return c.call(ctx, http.MethodPost, "/api/internal/workforce/v1/review-runs/"+idText(runID)+"/containers", body, scopedIdempotencyKey("review-container", c.instance, idText(runID), containerID), nil, nil)
}

func (c *Client) SubmitReviewReport(ctx context.Context, runID, ownerEpoch int64, lease string, permit ReviewLaunchPermit, report []byte, usage reviewrundomain.UsageFacts) (ReviewReportResult, error) {
	body := merge(c.reviewEnvelope(ownerEpoch), map[string]any{"reviewLeaseToken": lease, "permitId": idText(permit.PermitID), "permitToken": permit.PermitToken, "report": json.RawMessage(report), "usage": usage})
	var result struct {
		Result ReviewReportResult `json:"result"`
	}
	err := c.call(ctx, http.MethodPost, "/api/internal/workforce/v1/review-runs/"+idText(runID)+"/reports", body, scopedIdempotencyKey("review-report", c.instance, idText(runID), strconv.FormatInt(ownerEpoch, 10), sha256Text(report)), nil, &result)
	return result.Result, err
}

func (c *Client) ReviewFailure(ctx context.Context, runID, ownerEpoch int64, lease, reason string) error {
	body := merge(c.reviewEnvelope(ownerEpoch), map[string]any{"reviewLeaseToken": lease, "reasonCode": reason})
	return c.call(ctx, http.MethodPost, "/api/internal/workforce/v1/review-runs/"+idText(runID)+"/failures", body, scopedIdempotencyKey("review-failure", c.instance, idText(runID), strconv.FormatInt(ownerEpoch, 10), reason), nil, nil)
}

func (c *Client) WorkspaceGCEligibility(ctx context.Context, taskID int64) (WorkspaceGCEligibility, error) {
	if taskID < 1 {
		return WorkspaceGCEligibility{}, errors.New("workspace retention task ID is invalid")
	}
	var result WorkspaceGCEligibility
	err := c.call(ctx, http.MethodPost, "/api/internal/workforce/v1/tasks/"+idText(taskID)+"/workspace-gc-eligibility", c.envelope(0), scopedIdempotencyKey("workspace-gc", c.instance, idText(taskID)), nil, &result)
	return result, err
}

func (c *Client) Heartbeat(ctx context.Context, input HeartbeatInput) (HeartbeatResult, error) {
	var result HeartbeatResult
	err := c.call(ctx, http.MethodPost, "/api/internal/workforce/v1/workers/"+idText(c.workerID)+"/heartbeat", merge(c.envelope(0), input), ids.New("req"), nil, &result)
	return result, err
}

func (c *Client) PublishInventory(ctx context.Context, version int64, digest string, images []registrydomain.Image) error {
	body := merge(c.envelope(0), map[string]any{"inventoryVersion": idText(version), "inventoryDigest": digest, "images": images})
	return c.call(ctx, http.MethodPut, "/api/internal/workforce/v1/workers/"+idText(c.workerID)+"/image-inventory", body, scopedIdempotencyKey("inventory", c.instance, strconv.FormatInt(version, 10), digest), nil, nil)
}

func (c *Client) Deregister(ctx context.Context, reason string) error {
	return c.call(ctx, http.MethodPost, "/api/internal/workforce/v1/workers/"+idText(c.workerID)+"/deregister", merge(c.envelope(0), map[string]any{"reason": reason}), ids.New("req"), nil, nil)
}

func (c *Client) Claim(ctx context.Context, message dispatchdomain.AssignmentMessage) (dispatchdomain.ExecutionContract, error) {
	body := merge(c.envelope(message.Generation), map[string]any{"executionRequirementDigest": message.ExecutionRequirementDigest})
	var result struct {
		Contract dispatchdomain.ExecutionContract `json:"contract"`
	}
	// A Delivery retry within one Worker process must replay the same Claim,
	// while a replacement instance is a distinct authenticated owner and has a
	// different Machine envelope. Scope the idempotency key to that instance so
	// a stale terminal notification can be acknowledged after a Worker restart
	// instead of conflicting with the old instance's completed response record.
	err := c.call(ctx, http.MethodPost, "/api/internal/workforce/v1/assignments/"+idText(message.AssignmentID)+"/claim", body, claimIdempotencyKey(c.instance, message.AssignmentID, message.Generation), nil, &result)
	return result.Contract, err
}

func claimIdempotencyKey(instanceID string, assignmentID, generation int64) string {
	return scopedIdempotencyKey("claim", instanceID, idText(assignmentID), strconv.FormatInt(generation, 10))
}

func (c *Client) ReleaseAssignment(ctx context.Context, message dispatchdomain.AssignmentMessage, reason string) error {
	body := merge(c.envelope(message.Generation), map[string]any{"reasonCode": reason})
	return c.call(ctx, http.MethodPost, "/api/internal/workforce/v1/assignments/"+idText(message.AssignmentID)+"/release", body, "release-"+idText(message.MessageID), nil, nil)
}

func (c *Client) LaunchPermit(ctx context.Context, attemptID, generation, ownerEpoch int64, lease string) (dispatchdomain.LaunchPermit, bool, error) {
	body := merge(c.attemptEnvelope(generation, ownerEpoch), map[string]any{"attemptLeaseToken": lease})
	var result struct {
		Permit   dispatchdomain.LaunchPermit `json:"permit"`
		Replayed bool                        `json:"replayed"`
	}
	err := c.call(ctx, http.MethodPost, "/api/internal/workforce/v1/attempts/"+idText(attemptID)+"/launch-permits", body, "launch-"+idText(attemptID), nil, &result)
	return result.Permit, result.Replayed, err
}

func (c *Client) ContainerCreated(ctx context.Context, attempt taskdomain.Attempt, containerID, imageDigest, identity string) error {
	body := merge(c.attemptEnvelope(attempt.Generation, attempt.OwnerEpoch), map[string]any{
		"attemptLeaseToken": attempt.LeaseToken, "containerId": containerID,
		"imageDigest": imageDigest, "executionIdentityDigest": identity,
	})
	return c.call(ctx, http.MethodPost, c.attemptPath(attempt.ID, "/containers"), body, "container-"+idText(attempt.ID)+"-"+containerID, nil, nil)
}

func (c *Client) ProviderProxyReady(ctx context.Context, attempt taskdomain.Attempt, ready dispatchdomain.ProviderProxyReady) error {
	body := merge(c.attemptEnvelope(attempt.Generation, attempt.OwnerEpoch), map[string]any{"attemptLeaseToken": attempt.LeaseToken, "ready": ready})
	return c.call(ctx, http.MethodPost, c.attemptPath(attempt.ID, "/provider-proxy-ready"), body, "provider-proxy-ready-"+idText(attempt.ID)+"-"+ready.ContainerID, nil, nil)
}

func (c *Client) SupervisorReady(ctx context.Context, attempt taskdomain.Attempt, ready dispatchdomain.SupervisorReady) error {
	body := merge(c.attemptEnvelope(attempt.Generation, attempt.OwnerEpoch), map[string]any{"attemptLeaseToken": attempt.LeaseToken, "ready": ready})
	return c.call(ctx, http.MethodPost, c.attemptPath(attempt.ID, "/supervisor-ready"), body, "supervisor-ready-"+idText(attempt.ID), nil, nil)
}

func (c *Client) RunPermit(ctx context.Context, attempt taskdomain.Attempt) (dispatchdomain.RunPermit, error) {
	body := merge(c.attemptEnvelope(attempt.Generation, attempt.OwnerEpoch), map[string]any{"attemptLeaseToken": attempt.LeaseToken})
	var result struct {
		Permit dispatchdomain.RunPermit `json:"permit"`
	}
	err := c.call(ctx, http.MethodPost, c.attemptPath(attempt.ID, "/run-permits"), body, "run-permit-"+idText(attempt.ID), nil, &result)
	return result.Permit, err
}

func (c *Client) HarnessStarted(ctx context.Context, attempt taskdomain.Attempt, started dispatchdomain.HarnessStarted) error {
	body := merge(c.attemptEnvelope(attempt.Generation, attempt.OwnerEpoch), map[string]any{"attemptLeaseToken": attempt.LeaseToken, "started": started})
	return c.call(ctx, http.MethodPost, c.attemptPath(attempt.ID, "/harness-started"), body, "harness-started-"+idText(attempt.ID), nil, nil)
}

type LeaseRenewal struct {
	ExpiresAt     string `json:"expiresAt"`
	ReceiptDigest string `json:"receiptDigest"`
}

func (c *Client) RenewLease(ctx context.Context, attempt taskdomain.Attempt, activeElapsedMillis *int64) (LeaseRenewal, error) {
	body := merge(c.attemptEnvelope(attempt.Generation, attempt.OwnerEpoch), map[string]any{"attemptLeaseToken": attempt.LeaseToken})
	if activeElapsedMillis != nil {
		body["activeElapsedMillis"] = strconv.FormatInt(*activeElapsedMillis, 10)
	}
	var result LeaseRenewal
	err := c.call(ctx, http.MethodPut, c.attemptPath(attempt.ID, "/lease"), body, "lease-"+idText(attempt.ID)+"-"+ids.New("renew"), nil, &result)
	return result, err
}

func (c *Client) AcquireReattachPermit(ctx context.Context, runtime registrydomain.RecoverableRuntime, evidence dispatchdomain.RuntimeEvidence) (dispatchdomain.ReattachPermit, error) {
	body := merge(c.attemptEnvelope(runtime.Generation, runtime.OwnerEpoch), map[string]any{"evidence": evidence})
	var result struct {
		Permit dispatchdomain.ReattachPermit `json:"permit"`
	}
	err := c.call(ctx, http.MethodPost, c.attemptPath(runtime.AttemptID, "/reattach-permits"), body, "reattach-"+idText(runtime.AttemptID)+"-"+strconv.FormatInt(runtime.OwnerEpoch+1, 10), nil, &result)
	return result.Permit, err
}

func (c *Client) CommitReattach(ctx context.Context, runtime registrydomain.RecoverableRuntime, permit dispatchdomain.ReattachPermit, ack dispatchdomain.ReattachAck) (dispatchdomain.ReattachResult, error) {
	body := merge(c.attemptEnvelope(runtime.Generation, permit.NewOwnerEpoch), map[string]any{"attemptLeaseToken": permit.LeaseToken, "ack": ack})
	var result dispatchdomain.ReattachResult
	err := c.call(ctx, http.MethodPost, c.attemptPath(runtime.AttemptID, "/reattach-acks"), body, "reattach-ack-"+idText(runtime.AttemptID)+"-"+strconv.FormatInt(permit.NewOwnerEpoch, 10), nil, &result)
	return result, err
}

func (c *Client) ReportRuntimeLoss(ctx context.Context, runtime registrydomain.RecoverableRuntime, reason, evidenceDigest string) (dispatchdomain.RuntimeLossResult, error) {
	body := merge(c.attemptEnvelope(runtime.Generation, runtime.OwnerEpoch), map[string]any{"reasonCode": reason, "evidenceDigest": evidenceDigest})
	var result dispatchdomain.RuntimeLossResult
	// call derives requestId from the final replay key. Exclude the provisional
	// random envelope ID so an identical observation hashes identically.
	delete(body, "requestId")
	encoded, err := json.Marshal(body)
	if err != nil {
		return result, err
	}
	// A new reporting instance/boot/owner or evidence is a new observation,
	// while byte-identical retries retain the same transport replay identity.
	key := scopedIdempotencyKey("runtime-loss", idText(runtime.AttemptID), sha256Text(encoded))
	err = c.call(ctx, http.MethodPost, c.attemptPath(runtime.AttemptID, "/runtime-losses"), body, key, nil, &result)
	return result, err
}

func (c *Client) AddEvent(ctx context.Context, attempt taskdomain.Attempt, eventType string, payload any, sequence int64) error {
	raw, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	digest, err := taskinfra.RequestHash(json.RawMessage(raw))
	if err != nil {
		return err
	}
	fact := dispatchdomain.Fact{FactID: ids.New("fac"), ProducerSequence: sequence, EventType: eventType, Payload: raw, PayloadDigest: digest}
	body := merge(c.attemptEnvelope(attempt.Generation, attempt.OwnerEpoch), map[string]any{"attemptLeaseToken": attempt.LeaseToken, "facts": []dispatchdomain.Fact{fact}})
	return c.call(ctx, http.MethodPost, c.attemptPath(attempt.ID, "/events/batch"), body, fact.FactID, nil, nil)
}

func (c *Client) ReplayJournalFacts(ctx context.Context, attempt taskdomain.Attempt, facts []dispatchdomain.Fact) error {
	if len(facts) == 0 {
		return nil
	}
	digestInput, err := json.Marshal(facts)
	if err != nil {
		return err
	}
	sum := sha256.Sum256(digestInput)
	key := "journal:" + idText(attempt.ID) + ":" + strconv.FormatInt(attempt.OwnerEpoch, 10) + ":" + strconv.FormatInt(facts[0].ProducerSequence, 10) + ":" + strconv.FormatInt(facts[len(facts)-1].ProducerSequence, 10) + ":" + fmt.Sprintf("%x", sum[:8])
	// Journal digests bind the original payload bytes, not a re-encoded JSON
	// object. merge decodes RawMessage into maps and reorders their keys.
	body := c.attemptEnvelope(attempt.Generation, attempt.OwnerEpoch)
	body["attemptLeaseToken"] = attempt.LeaseToken
	body["facts"] = facts
	return c.call(ctx, http.MethodPost, c.attemptPath(attempt.ID, "/journal-facts/batch"), body, key, nil, nil)
}

func (c *Client) Completion(ctx context.Context, attempt taskdomain.Attempt, completion json.RawMessage) error {
	body := merge(c.attemptEnvelope(attempt.Generation, attempt.OwnerEpoch), map[string]any{"attemptLeaseToken": attempt.LeaseToken, "completion": completion})
	return c.call(ctx, http.MethodPost, c.attemptPath(attempt.ID, "/completion-proposals"), body, "completion-"+idText(attempt.ID), nil, nil)
}

// ProcessorMaterialization carries a single EvaluationCycle's CONFIG/SECRET
// values. The caller must send it only to the Supervisor private control path;
// it is intentionally never copied into the Docker command, Attempt spec, or
// Worker event stream.
func (c *Client) ProcessorMaterialization(ctx context.Context, attempt taskdomain.Attempt, cycleID int64, cycleNumber int) (processorfacade.Materialization, error) {
	var result processorfacade.Materialization
	body := merge(c.attemptEnvelope(attempt.Generation, attempt.OwnerEpoch), map[string]any{"attemptLeaseToken": attempt.LeaseToken, "cycleId": idText(cycleID), "cycleNumber": cycleNumber})
	// A materialization can be replayed after a lost response by the same
	// owner, but a reattached Worker has a different lease and must not collide
	// with the prior owner's encrypted response record.  Scope the key to the
	// current instance/owner authority rather than weakening the Machine API's
	// request-digest conflict protection.
	err := c.call(ctx, http.MethodPost, c.attemptPath(attempt.ID, "/processor-materializations"), body, c.attemptIdempotencyKey("processor-materialize", attempt, strconv.Itoa(cycleNumber), idText(cycleID)), nil, &result)
	return result, err
}

// ResultEvaluation is the durable Server acknowledgement for a protected
// process(context) candidate. A response retry reuses the cycle/contract
// identity, so it never causes a second resume or CompletionProposal.
func (c *Client) ResultEvaluation(ctx context.Context, attempt taskdomain.Attempt, candidate processorfacade.EvaluationCandidate) (processorfacade.EvaluationDirective, error) {
	var result processorfacade.EvaluationDirective
	body := merge(c.attemptEnvelope(attempt.Generation, attempt.OwnerEpoch), map[string]any{"attemptLeaseToken": attempt.LeaseToken, "candidate": candidate})
	// The durable EvaluationCycle itself is still the primary business
	// idempotency boundary.  This transport key intentionally includes the
	// owner authority so a replacement Worker can recover a candidate left in
	// EVALUATION_PENDING_ACK without conflicting with the old lease's cached
	// response.
	key := c.attemptIdempotencyKey("evaluation", attempt, idText(candidate.CycleID), candidate.ContractDigest)
	err := c.call(ctx, http.MethodPost, c.attemptPath(attempt.ID, "/result-evaluations"), body, key, nil, &result)
	return result, err
}

// ContinuationCheckpoint persists a checkpoint after the Worker has uploaded
// and byte-for-byte read back every private checkpoint object.  Its stable key
// makes a lost response safe without allowing an arbitrary new object key.
func (c *Client) ContinuationCheckpoint(ctx context.Context, attempt taskdomain.Attempt, candidate processorfacade.ContinuationCheckpointCandidate) (processorfacade.ContinuationCheckpointView, error) {
	var result processorfacade.ContinuationCheckpointView
	body := merge(c.attemptEnvelope(attempt.Generation, attempt.OwnerEpoch), map[string]any{"attemptLeaseToken": attempt.LeaseToken, "checkpoint": candidate})
	key := c.attemptIdempotencyKey("checkpoint", attempt, candidate.CheckpointIdentity)
	err := c.call(ctx, http.MethodPost, c.attemptPath(attempt.ID, "/continuation-checkpoints"), body, key, nil, &result)
	return result, err
}

func (c *Client) ContinuationCheckpointKey(ctx context.Context, attempt taskdomain.Attempt, checkpointIdentity string) (continuationapp.CheckpointKeyMaterialization, error) {
	var result continuationapp.CheckpointKeyMaterialization
	body := merge(c.attemptEnvelope(attempt.Generation, attempt.OwnerEpoch), map[string]any{"attemptLeaseToken": attempt.LeaseToken, "checkpointIdentity": checkpointIdentity})
	err := c.call(ctx, http.MethodPost, c.attemptPath(attempt.ID, "/continuation-checkpoint-keys"), body, c.attemptIdempotencyKey("checkpoint-key", attempt, checkpointIdentity), nil, &result)
	return result, err
}

func (c *Client) ContinuationRestoreKey(ctx context.Context, attempt taskdomain.Attempt, restoreRunID int64) (continuationapp.CheckpointKeyMaterialization, error) {
	var result continuationapp.CheckpointKeyMaterialization
	body := merge(c.attemptEnvelope(attempt.Generation, attempt.OwnerEpoch), map[string]any{"attemptId": idText(attempt.ID), "attemptLeaseToken": attempt.LeaseToken})
	err := c.call(ctx, http.MethodPost, "/api/internal/workforce/v1/continuation-restores/"+idText(restoreRunID)+"/key", body, c.attemptIdempotencyKey("continuation-restore-key", attempt, idText(restoreRunID)), nil, &result)
	return result, err
}

func (c *Client) CompleteContinuationRestore(ctx context.Context, attempt taskdomain.Attempt, restoreRunID int64, resultDigest, baseCommit, resultCommit, resultTree string) error {
	body := merge(c.attemptEnvelope(attempt.Generation, attempt.OwnerEpoch), map[string]any{"attemptId": idText(attempt.ID), "attemptLeaseToken": attempt.LeaseToken, "restoreRunId": idText(restoreRunID), "resultDigest": resultDigest, "baseCommit": baseCommit, "resultCommit": resultCommit, "resultTree": resultTree})
	// restoreRunID is the stable identity for the one permitted result. The
	// Machine idempotency layer binds the complete request-body digest to this
	// key, so a different result fails closed without exceeding its 128-byte
	// header bound.
	return c.call(ctx, http.MethodPost, "/api/internal/workforce/v1/continuation-restores/"+idText(restoreRunID)+"/results", body, "continuation-restore-result:"+idText(restoreRunID), nil, nil)
}

func (c *Client) ClaimProcessorFixture(ctx context.Context, runID, generation int64) (processorfacade.FixtureExecution, error) {
	var result processorfacade.FixtureExecution
	err := c.call(ctx, http.MethodPost, "/api/internal/workforce/v1/result-processor-fixture-runs/"+idText(runID)+"/claims", c.envelope(generation), "fixture-claim:"+idText(runID)+":"+strconv.FormatInt(generation, 10), nil, &result)
	return result, err
}

func (c *Client) CompleteProcessorFixture(ctx context.Context, runID, generation int64, result processorfacade.FixtureResult) error {
	body := merge(c.envelope(generation), map[string]any{"result": result})
	key := "fixture-result:" + idText(runID) + ":" + result.ContractDigest
	if result.ContractDigest == "" {
		key += ":" + result.ReasonCode
	}
	return c.call(ctx, http.MethodPost, "/api/internal/workforce/v1/result-processor-fixture-runs/"+idText(runID)+"/results", body, key, nil, nil)
}

type CheckInput struct {
	Ordinal  int    `json:"ordinal"`
	Name     string `json:"name"`
	Command  string `json:"command"`
	ExitCode int    `json:"exitCode"`
	Output   string `json:"output"`
	Status   string `json:"status"`
}

func (c *Client) Checks(ctx context.Context, attempt taskdomain.Attempt, checks []CheckInput) error {
	body := merge(c.attemptEnvelope(attempt.Generation, attempt.OwnerEpoch), map[string]any{"attemptLeaseToken": attempt.LeaseToken, "checks": checks})
	return c.call(ctx, http.MethodPost, c.attemptPath(attempt.ID, "/checks/batch"), body, "checks-"+idText(attempt.ID)+"-"+strconv.Itoa(len(checks)), nil, nil)
}

func (c *Client) Evidence(ctx context.Context, attempt taskdomain.Attempt) ([]taskinfra.CheckEvidence, []taskdomain.Event, error) {
	headers := c.attemptHeaders(attempt)
	var result struct {
		Checks []taskinfra.CheckEvidence `json:"checks"`
		Events []taskdomain.Event        `json:"events"`
	}
	err := c.call(ctx, http.MethodGet, c.attemptPath(attempt.ID, "/evidence"), nil, ids.New("req"), headers, &result)
	return result.Checks, result.Events, err
}

func (c *Client) Control(ctx context.Context, attempt taskdomain.Attempt) (bool, error) {
	var result struct {
		Cancelling bool `json:"cancelling"`
	}
	err := c.call(ctx, http.MethodGet, c.attemptPath(attempt.ID, "/control"), nil, ids.New("req"), c.attemptHeaders(attempt), &result)
	return result.Cancelling, err
}

func (c *Client) RecordArtifact(ctx context.Context, attempt taskdomain.Attempt, artifact taskdomain.Artifact) error {
	metadata := map[string]any{
		"id": idText(artifact.ID), "type": artifact.Type, "name": artifact.Name,
		"objectKey": artifact.ObjectKey, "sha256": artifact.SHA256,
		"sizeBytes": artifact.SizeBytes, "mediaType": artifact.MediaType,
		"status": artifact.Status, "metadata": artifact.Metadata,
	}
	body := merge(c.attemptEnvelope(attempt.Generation, attempt.OwnerEpoch), map[string]any{"attemptLeaseToken": attempt.LeaseToken, "artifacts": []any{metadata}})
	return c.call(ctx, http.MethodPost, c.attemptPath(attempt.ID, "/artifacts/batch"), body, "artifact-"+idText(artifact.ID), nil, nil)
}

type Outcome struct {
	Outcome      string `json:"outcome"`
	ReasonCode   string `json:"reasonCode"`
	Message      string `json:"message"`
	ResultCommit string `json:"resultCommit"`
	ResultTree   string `json:"resultTree"`
	Summary      string `json:"summary"`
}

func (c *Client) Outcome(ctx context.Context, attempt taskdomain.Attempt, outcome Outcome) error {
	return c.call(ctx, http.MethodPost, c.attemptPath(attempt.ID, "/outcome"), merge(c.attemptEnvelope(attempt.Generation, attempt.OwnerEpoch), merge(map[string]any{"attemptLeaseToken": attempt.LeaseToken}, outcome)), "outcome-"+idText(attempt.ID)+"-"+outcome.Outcome, nil, nil)
}

func (c *Client) ClaimFinalization(ctx context.Context, message artifactdomain.WorkMessage) (artifactdomain.FinalizationClaim, error) {
	var result artifactdomain.FinalizationClaim
	body := c.envelope(message.Generation)
	// A Delivery retry reuses the same message ID and must replay the original
	// Lease. A new FinalizationRun retry has a new message ID and must claim a
	// fresh run/Lease instead of replaying the prior run's secret response.
	err := c.call(ctx, http.MethodPost, "/api/internal/workforce/v1/finalization-runs/"+idText(message.ResourceID)+"/claims", body, scopedIdempotencyKey("fn-claim", c.instance, idText(message.ResourceID), idText(message.MessageID)), nil, &result)
	return result, err
}

func (c *Client) FreezeFinalization(ctx context.Context, message artifactdomain.WorkMessage, lease string, result artifactdomain.FreezeResult) error {
	body := merge(c.envelope(message.Generation), map[string]any{"leaseToken": lease, "result": result})
	// A new retry message has a new Lease even when it deterministically freezes
	// the same manifest. Bind the replay scope to the work message so transport
	// retries replay the same response while a later FinalizationRun retry is a
	// distinct mutation.
	return c.call(ctx, http.MethodPost, "/api/internal/workforce/v1/finalization-runs/"+idText(message.ResourceID)+"/freeze", body, scopedIdempotencyKey("fn-freeze", c.instance, idText(message.ResourceID), idText(message.MessageID), result.ManifestDigest), nil, nil)
}

func (c *Client) FailFinalization(ctx context.Context, message artifactdomain.WorkMessage, lease, reason string) error {
	body := merge(c.envelope(message.Generation), map[string]any{"leaseToken": lease, "reasonCode": reason})
	return c.call(ctx, http.MethodPost, "/api/internal/workforce/v1/finalization-runs/"+idText(message.ResourceID)+"/failures", body, "finalization-failure:"+idText(message.ResourceID)+":"+idText(message.MessageID), nil, nil)
}

func (c *Client) ClaimUpload(ctx context.Context, message artifactdomain.WorkMessage) (artifactdomain.UploadClaim, error) {
	var result artifactdomain.UploadClaim
	body := c.envelope(message.Generation)
	err := c.call(ctx, http.MethodPost, "/api/internal/workforce/v1/artifact-upload-attempts/"+idText(message.ResourceID)+"/claims", body, scopedIdempotencyKey("up-claim", c.instance, idText(message.ResourceID), idText(message.MessageID)), nil, &result)
	return result, err
}

func (c *Client) VerifyUpload(ctx context.Context, message artifactdomain.WorkMessage, lease string) error {
	body := merge(c.envelope(message.Generation), map[string]any{"leaseToken": lease})
	return c.call(ctx, http.MethodPost, "/api/internal/workforce/v1/artifact-upload-attempts/"+idText(message.ResourceID)+"/verifications", body, scopedIdempotencyKey("up-verify", c.instance, idText(message.ResourceID), idText(message.MessageID)), nil, nil)
}

func (c *Client) FailUpload(ctx context.Context, message artifactdomain.WorkMessage, lease, reason string) error {
	body := merge(c.envelope(message.Generation), map[string]any{"leaseToken": lease, "reasonCode": reason})
	return c.call(ctx, http.MethodPost, "/api/internal/workforce/v1/artifact-upload-attempts/"+idText(message.ResourceID)+"/failures", body, "upload-failure:"+idText(message.ResourceID)+":"+idText(message.MessageID), nil, nil)
}

func (c *Client) ClaimMaterialization(ctx context.Context, message continuationdomain.MaterializationMessage) (continuationdomain.MaterializationClaim, error) {
	var result continuationdomain.MaterializationClaim
	err := c.call(ctx, http.MethodPost, "/api/internal/workforce/v1/result-materialization-runs/"+idText(message.ResourceID)+"/claims", c.envelope(message.Generation), scopedIdempotencyKey("mt-claim", c.instance, idText(message.ResourceID), idText(message.MessageID)), nil, &result)
	return result, err
}

func (c *Client) FreezeMaterialization(ctx context.Context, message continuationdomain.MaterializationMessage, lease string, result continuationdomain.MaterializationFreeze) error {
	body := merge(c.envelope(message.Generation), map[string]any{"leaseToken": lease, "result": result})
	return c.call(ctx, http.MethodPost, "/api/internal/workforce/v1/result-materialization-runs/"+idText(message.ResourceID)+"/freeze", body, scopedIdempotencyKey("mt-freeze", c.instance, idText(message.ResourceID), idText(message.MessageID), result.SHA256), nil, nil)
}

func (c *Client) FailMaterialization(ctx context.Context, message continuationdomain.MaterializationMessage, lease, reason string) error {
	body := merge(c.envelope(message.Generation), map[string]any{"leaseToken": lease, "reasonCode": reason})
	return c.call(ctx, http.MethodPost, "/api/internal/workforce/v1/result-materialization-runs/"+idText(message.ResourceID)+"/failures", body, scopedIdempotencyKey("mt-fail", c.instance, idText(message.ResourceID), idText(message.MessageID), reason), nil, nil)
}

func (c *Client) ClaimDelivery(ctx context.Context, message deliverydomain.Message) (deliveryapp.DeliveryClaim, error) {
	var result deliveryapp.DeliveryClaim
	err := c.call(ctx, http.MethodPost, "/api/internal/workforce/v1/delivery-attempts/"+idText(message.ResourceID)+"/claims", c.envelope(message.Generation), scopedIdempotencyKey("delivery-claim", c.instance, idText(message.ResourceID), idText(message.MessageID)), nil, &result)
	return result, err
}

func (c *Client) PrepareDelivery(ctx context.Context, message deliverydomain.Message, lease, commit string) (deliveryapp.PrepareResult, error) {
	var result deliveryapp.PrepareResult
	body := merge(c.envelope(message.Generation), map[string]any{"leaseToken": lease, "deliveryCommit": commit})
	err := c.call(ctx, http.MethodPost, "/api/internal/workforce/v1/delivery-attempts/"+idText(message.ResourceID)+"/observations", body, scopedIdempotencyKey("delivery-prepare", c.instance, idText(message.ResourceID), idText(message.MessageID), commit), nil, &result)
	return result, err
}

func (c *Client) MaterializeDeliveryCredential(ctx context.Context, message deliverydomain.Message, lease string) (deliveryapp.DeliveryCredential, error) {
	var result deliveryapp.DeliveryCredential
	body := merge(c.envelope(message.Generation), map[string]any{"leaseToken": lease})
	err := c.call(ctx, http.MethodPost, "/api/internal/workforce/v1/delivery-attempts/"+idText(message.ResourceID)+"/credential-materializations", body, scopedIdempotencyKey("delivery-credential", c.instance, idText(message.ResourceID), idText(message.MessageID)), nil, &result)
	return result, err
}

func (c *Client) CompleteDelivery(ctx context.Context, message deliverydomain.Message, lease, authorization string, result deliveryapp.DeliveryResult) error {
	body := merge(c.envelope(message.Generation), map[string]any{"leaseToken": lease, "pushAuthorization": authorization, "result": result})
	identity := result.RemoteAfter
	if identity == "" {
		identity = "unreadable"
	}
	return c.call(ctx, http.MethodPost, "/api/internal/workforce/v1/delivery-attempts/"+idText(message.ResourceID)+"/results", body, scopedIdempotencyKey("delivery-result", c.instance, idText(message.ResourceID), idText(message.MessageID), identity), nil, nil)
}

func (c *Client) ReconcileDelivery(ctx context.Context, message deliverydomain.Message, lease string, result deliveryapp.DeliveryResult) error {
	body := merge(c.envelope(message.Generation), map[string]any{"leaseToken": lease, "result": result})
	identity := result.RemoteAfter
	if identity == "" {
		identity = "unreadable"
	}
	return c.call(ctx, http.MethodPost, "/api/internal/workforce/v1/delivery-attempts/"+idText(message.ResourceID)+"/reconciliations", body, scopedIdempotencyKey("delivery-reconcile", c.instance, idText(message.ResourceID), idText(message.MessageID), identity), nil, nil)
}

func (c *Client) FailDelivery(ctx context.Context, message deliverydomain.Message, lease, reason string) error {
	body := merge(c.envelope(message.Generation), map[string]any{"leaseToken": lease, "reasonCode": reason})
	return c.call(ctx, http.MethodPost, "/api/internal/workforce/v1/delivery-attempts/"+idText(message.ResourceID)+"/failures", body, scopedIdempotencyKey("delivery-failure", c.instance, idText(message.ResourceID), idText(message.MessageID), reason), nil, nil)
}

func (c *Client) ClaimSourceIngestion(ctx context.Context, message workspaceinputdomain.SourceIngestionMessage) (workspaceinputapp.SourceIngestionClaim, error) {
	var result workspaceinputapp.SourceIngestionClaim
	err := c.call(ctx, http.MethodPost, "/api/internal/workforce/v1/source-ingestion-runs/"+idText(message.ResourceID)+"/claims", c.envelope(message.Generation), scopedIdempotencyKey("source-claim", c.instance, idText(message.ResourceID), idText(message.MessageID)), nil, &result)
	return result, err
}
func (c *Client) MaterializeSourceCredential(ctx context.Context, message workspaceinputdomain.SourceIngestionMessage, lease string) (workspaceinputapp.SourceCredential, error) {
	var result workspaceinputapp.SourceCredential
	body := merge(c.envelope(message.Generation), map[string]any{"leaseToken": lease})
	err := c.call(ctx, http.MethodPost, "/api/internal/workforce/v1/source-ingestion-runs/"+idText(message.ResourceID)+"/credential-materializations", body, scopedIdempotencyKey("source-credential", c.instance, idText(message.ResourceID), idText(message.MessageID)), nil, &result)
	return result, err
}
func (c *Client) ObserveSourceCommit(ctx context.Context, message workspaceinputdomain.SourceIngestionMessage, lease, commit string) error {
	body := merge(c.envelope(message.Generation), map[string]any{"leaseToken": lease, "resolvedCommit": commit})
	return c.call(ctx, http.MethodPost, "/api/internal/workforce/v1/source-ingestion-runs/"+idText(message.ResourceID)+"/observations", body, scopedIdempotencyKey("source-observation", c.instance, idText(message.ResourceID), idText(message.MessageID), commit), nil, nil)
}
func (c *Client) CompleteSourceIngestion(ctx context.Context, message workspaceinputdomain.SourceIngestionMessage, lease string, result workspaceinputapp.SourceIngestionResult) error {
	body := merge(c.envelope(message.Generation), map[string]any{"leaseToken": lease, "result": result})
	return c.call(ctx, http.MethodPost, "/api/internal/workforce/v1/source-ingestion-runs/"+idText(message.ResourceID)+"/results", body, scopedIdempotencyKey("source-result", c.instance, idText(message.ResourceID), idText(message.MessageID), result.RepositoryBundleContentSHA256), nil, nil)
}
func (c *Client) FailSourceIngestion(ctx context.Context, message workspaceinputdomain.SourceIngestionMessage, lease, reason string) error {
	body := merge(c.envelope(message.Generation), map[string]any{"leaseToken": lease, "reasonCode": reason})
	return c.call(ctx, http.MethodPost, "/api/internal/workforce/v1/source-ingestion-runs/"+idText(message.ResourceID)+"/failures", body, scopedIdempotencyKey("source-failure", c.instance, idText(message.ResourceID), idText(message.MessageID), reason), nil, nil)
}

func (c *Client) envelope(generation int64) map[string]any {
	return map[string]any{"schemaVersion": 1, "workerId": idText(c.workerID), "instanceId": c.instance, "hostBootId": c.hostBoot, "requestId": ids.New("req"), "generation": idText(generation)}
}
func (c *Client) attemptEnvelope(generation, ownerEpoch int64) map[string]any {
	body := c.envelope(generation)
	body["ownerEpoch"] = idText(ownerEpoch)
	return body
}
func (c *Client) reviewEnvelope(ownerEpoch int64) map[string]any {
	// ReviewRun ownership uses ownerEpoch as its generation fence. Its broker
	// assignment does not carry an Agent-assignment generation.
	body := c.envelope(1)
	body["ownerEpoch"] = idText(ownerEpoch)
	return body
}
func (c *Client) attemptPath(id int64, suffix string) string {
	return "/api/internal/workforce/v1/attempts/" + idText(id) + suffix
}
func (c *Client) attemptHeaders(attempt taskdomain.Attempt) map[string]string {
	return map[string]string{"X-Seven-Worker-ID": idText(c.workerID), "X-Seven-Worker-Instance": c.instance, "X-Seven-Host-Boot-ID": c.hostBoot, "X-Seven-Request-ID": ids.New("req"), "X-Seven-Generation": strconv.FormatInt(attempt.Generation, 10), "X-Seven-Owner-Epoch": strconv.FormatInt(attempt.OwnerEpoch, 10), "X-Seven-Attempt-Lease": attempt.LeaseToken}
}

type resultEnvelope struct {
	Code    int             `json:"code"`
	Data    json.RawMessage `json:"data"`
	Message string          `json:"message"`
}

func (c *Client) call(ctx context.Context, method, path string, body any, idempotency string, headers map[string]string, target any) error {
	if envelope, ok := body.(map[string]any); ok && idempotency != "" {
		// The business Idempotency-Key is stable across transport retries and
		// Worker restarts. Bind requestId to it as well so the canonical request
		// digest cannot drift merely because an envelope was rebuilt.
		digest := sha256.Sum256([]byte(idText(c.workerID) + "|" + method + "|" + path + "|" + idempotency))
		envelope["requestId"] = fmt.Sprintf("req_%x", digest[:16])
	}
	var encoded []byte
	if body != nil {
		var err error
		encoded, err = json.Marshal(body)
		if err != nil {
			return err
		}
	}
	var resp *http.Response
	var err error
	for attempt := 0; attempt < 3; attempt++ {
		var reader io.Reader
		if encoded != nil {
			reader = bytes.NewReader(encoded)
		}
		req, requestErr := http.NewRequestWithContext(ctx, method, c.baseURL+path, reader)
		if requestErr != nil {
			return requestErr
		}
		req.Header.Set("Authorization", "Bearer "+c.token)
		req.Header.Set("Accept", "application/json")
		if body != nil {
			req.Header.Set("Content-Type", "application/json")
		}
		if idempotency != "" {
			req.Header.Set("Idempotency-Key", idempotency)
		}
		for key, value := range headers {
			req.Header.Set(key, value)
		}
		resp, err = c.http.Do(req)
		if err == nil {
			break
		}
		if ctx.Err() != nil || idempotency == "" || attempt == 2 {
			return fmt.Errorf("machine API %s: %w", path, err)
		}
		timer := time.NewTimer(time.Duration(attempt+1) * 100 * time.Millisecond)
		select {
		case <-ctx.Done():
			timer.Stop()
			return ctx.Err()
		case <-timer.C:
		}
	}
	if resp == nil {
		return fmt.Errorf("machine API %s: %w", path, err)
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, maxResponseBytes+1))
	if err != nil {
		return err
	}
	if len(raw) > maxResponseBytes {
		return errors.New("machine API response exceeds limit")
	}
	var envelope resultEnvelope
	if err := json.Unmarshal(raw, &envelope); err != nil {
		return fmt.Errorf("machine API returned invalid JSON: %w", err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 || envelope.Code != 0 {
		var details struct {
			ReasonCode string `json:"reasonCode"`
		}
		_ = json.Unmarshal(envelope.Data, &details)
		return &APIError{StatusCode: resp.StatusCode, ReasonCode: details.ReasonCode, Message: envelope.Message}
	}
	if target != nil && len(envelope.Data) > 0 && string(envelope.Data) != "null" {
		return commonjson.Unmarshal(envelope.Data, target)
	}
	return nil
}

type APIError struct {
	StatusCode int
	ReasonCode string
	Message    string
}

func (e *APIError) Error() string {
	if e.ReasonCode != "" {
		return e.ReasonCode + ": " + e.Message
	}
	return e.Message
}

func merge(left any, right any) map[string]any {
	result := map[string]any{}
	for _, source := range []any{left, right} {
		body, _ := json.Marshal(source)
		decoder := json.NewDecoder(bytes.NewReader(body))
		decoder.UseNumber()
		_ = decoder.Decode(&result)
	}
	return result
}

func scopedIdempotencyKey(prefix string, parts ...string) string {
	sum := sha256.Sum256([]byte(strings.Join(parts, "\x00")))
	return prefix + ":" + fmt.Sprintf("%x", sum[:16])
}

func sha256Text(value []byte) string {
	sum := sha256.Sum256(value)
	return fmt.Sprintf("%x", sum[:])
}

func idText(id int64) string {
	return strconv.FormatInt(id, 10)
}

// attemptIdempotencyKey scopes a transport replay record to the one Worker
// instance and owner epoch that owns the current Execution Lease.  Server-side
// aggregates still enforce Attempt/generation/cycle uniqueness, so including
// the authority here never permits a second Result Evaluation or Checkpoint;
// it simply lets a newly reattached owner make its own correctly fenced
// request instead of inheriting an old instance's cached secret response.
func (c *Client) attemptIdempotencyKey(prefix string, attempt taskdomain.Attempt, parts ...string) string {
	scope := []string{c.instance, idText(attempt.ID), strconv.FormatInt(attempt.Generation, 10), strconv.FormatInt(attempt.OwnerEpoch, 10)}
	scope = append(scope, parts...)
	return scopedIdempotencyKey(prefix, scope...)
}
