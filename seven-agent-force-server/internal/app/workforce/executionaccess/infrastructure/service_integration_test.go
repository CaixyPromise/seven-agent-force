package infrastructure_test

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"os"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/CaixyPromise/seven-agent/force/internal/app/workforce/executionaccess/application"
	"github.com/CaixyPromise/seven-agent/force/internal/app/workforce/executionaccess/domain"
	"github.com/CaixyPromise/seven-agent/force/internal/app/workforce/executionaccess/facade"
	"github.com/CaixyPromise/seven-agent/force/internal/app/workforce/executionaccess/infrastructure"
)

const executionAccessTestDSNEnv = "EXECUTION_ACCESS_TEST_POSTGRES_DSN"

type integrationIDs struct{ next atomic.Int64 }

func (g *integrationIDs) NextID() int64 { return g.next.Add(1) }

type integrationCipher struct{}

func (integrationCipher) Encrypt(_ context.Context, raw []byte) (domain.EncryptedDescriptor, error) {
	return domain.EncryptedDescriptor{Ciphertext: base64.StdEncoding.EncodeToString(raw), EDEK: base64.StdEncoding.EncodeToString([]byte("test-edek")), KeyRef: "test-key"}, nil
}
func (integrationCipher) Decrypt(_ context.Context, value domain.EncryptedDescriptor) ([]byte, error) {
	return base64.StdEncoding.DecodeString(value.Ciphertext)
}

type integrationProjector struct{}

func (integrationProjector) ProjectExecutionOperation(context.Context, pgx.Tx, facade.Projection) error {
	return nil
}

type integrationUserAuthority struct{ target facade.Target }

func (a integrationUserAuthority) AuthorizeUser(_ context.Context, _ pgx.Tx, _ facade.User, _ string) (facade.Target, error) {
	return a.target, nil
}

type integrationMachineAuthority struct{ target facade.Target }

func (a integrationMachineAuthority) ValidateMachine(_ context.Context, _ pgx.Tx, _ facade.Machine, _ string) (facade.Target, error) {
	return a.target, nil
}

func TestExecutionAccessAuthorityDecisionsAndTerminalReceiptPostgres(t *testing.T) {
	dsn := strings.TrimSpace(os.Getenv(executionAccessTestDSNEnv))
	if dsn == "" {
		t.Skip("set EXECUTION_ACCESS_TEST_POSTGRES_DSN to an isolated PostgreSQL database")
	}
	ctx := context.Background()
	admin, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer admin.Close()
	schemaName := fmt.Sprintf("execution_access_%d", time.Now().UnixNano())
	if _, err = admin.Exec(ctx, `CREATE SCHEMA `+schemaName); err != nil {
		t.Fatal(err)
	}
	defer admin.Exec(context.Background(), `DROP SCHEMA IF EXISTS `+schemaName+` CASCADE`)

	config, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		t.Fatal(err)
	}
	config.ConnConfig.RuntimeParams["search_path"] = schemaName
	pool, err := pgxpool.NewWithConfig(ctx, config)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	if _, err = pool.Exec(ctx, infrastructure.Schema); err != nil {
		t.Fatal(err)
	}
	// Grant audit joins the migration-owned structured principal table.
	// Copy its definition only; fixture identities stay in the disposable schema.
	if _, err = pool.Exec(ctx, `CREATE TABLE workforce_principals (LIKE public.workforce_principals INCLUDING ALL);
		INSERT INTO workforce_principals (id,principal_type,user_id) VALUES (31,'USER',41)`); err != nil {
		t.Fatal(err)
	}

	const tenantID, taskID, taskTurnID, agentAttemptID, adapterGeneration = int64(11), int64(12), int64(13), int64(14), int64(15)
	const workerID, ownerEpoch = int64(21), int64(22)
	target := facade.Target{TenantID: tenantID, TaskID: taskID, TaskTurnID: taskTurnID, AttemptID: agentAttemptID, AdapterSessionGeneration: adapterGeneration, PrincipalID: 31, WorkerID: workerID, OwnerEpoch: ownerEpoch, Capabilities: []string{"tool.request", "tool.result"}}
	ids := &integrationIDs{}
	ids.next.Store(1000)
	service := application.New(pool, ids, infrastructure.New(), integrationUserAuthority{target}, integrationMachineAuthority{target}, integrationCipher{}, integrationProjector{})
	machine := facade.Machine{TenantID: tenantID, TaskID: taskID, TaskTurnID: taskTurnID, WorkerID: workerID, InstanceID: "instance-current", HostBootID: "boot-current", AttemptID: agentAttemptID, Generation: 1, OwnerEpoch: ownerEpoch, AdapterSessionGeneration: adapterGeneration, ExecutionLease: "lease-current"}
	user := facade.User{TenantID: tenantID, TaskID: taskID, UserID: 41, CanContinue: true}
	targets := []domain.Target{{Host: "registry.npmjs.org", Protocol: "https", Port: 443}}

	publish := func(key string) facade.Receipt {
		t.Helper()
		descriptor := domain.Descriptor{Purpose: "install locked dependencies", Command: "pnpm install --frozen-lockfile", Cwd: ".", Targets: targets}
		receipt, publishErr := service.Publish(ctx, machine, facade.PublishRequest{RoundKey: "round-1", LocalToolCallKey: key, NativeToolCallID: "native-" + key, ToolName: facade.NetworkCommandToolName, ArgumentsDigest: domain.Digest(descriptor), Arguments: descriptor})
		if publishErr != nil {
			t.Fatal(publishErr)
		}
		return receipt
	}

	one := publish("once")
	replayed := publish("once")
	if !replayed.Replayed || replayed.OperationID != one.OperationID {
		t.Fatalf("occurrence did not replay: %#v", replayed)
	}
	conflictingDescriptor := domain.Descriptor{Purpose: "different", Command: "pnpm install --frozen-lockfile", Cwd: ".", Targets: targets}
	_, err = service.Publish(ctx, machine, facade.PublishRequest{RoundKey: "round-1", LocalToolCallKey: "once", NativeToolCallID: "native-once", ToolName: facade.NetworkCommandToolName, ArgumentsDigest: domain.Digest(conflictingDescriptor), Arguments: conflictingDescriptor})
	if !errors.Is(err, domain.ErrIdempotencyConflict) {
		t.Fatalf("expected occurrence conflict, got %v", err)
	}
	if _, err = service.Decide(ctx, facade.User{TenantID: tenantID, TaskID: taskID, UserID: 41}, one.OperationID, facade.DecisionRequest{ExpectedVersion: 1, Action: domain.DecisionAllowOnce, IdempotencyKey: "permission-denied"}); !errors.Is(err, domain.ErrActionNotAllowed) {
		t.Fatalf("missing continuation permission was accepted: %v", err)
	}
	allowed, err := service.Decide(ctx, user, one.OperationID, facade.DecisionRequest{ExpectedVersion: 1, Action: domain.DecisionAllowOnce, IdempotencyKey: "allow-once"})
	if err != nil || allowed.Status != domain.StatusReady {
		t.Fatalf("ALLOW_ONCE failed: %#v %v", allowed, err)
	}
	if _, err = service.Decide(ctx, user, one.OperationID, facade.DecisionRequest{ExpectedVersion: 1, Action: domain.DecisionDeny, IdempotencyKey: "allow-once"}); !errors.Is(err, domain.ErrIdempotencyConflict) {
		t.Fatalf("same idempotency key with different decision was accepted: %v", err)
	}
	if _, err = service.Get(ctx, facade.User{TenantID: tenantID + 1, TaskID: taskID, UserID: user.UserID}, one.OperationID); !errors.Is(err, domain.ErrFenced) {
		t.Fatalf("cross-tenant read was accepted: %v", err)
	}

	grantOperation := publish("task-grant")
	grantDecision, err := service.Decide(ctx, user, grantOperation.OperationID, facade.DecisionRequest{ExpectedVersion: 1, Action: domain.DecisionAllowForTask, IdempotencyKey: "allow-task"})
	if err != nil || grantDecision.GrantID <= 0 {
		t.Fatalf("ALLOW_FOR_TASK failed: %#v %v", grantDecision, err)
	}
	auto := publish("covered-by-task-grant")
	if auto.Status != domain.StatusReady {
		t.Fatalf("active task grant did not cover target: %#v", auto)
	}
	grants, err := service.ListGrants(ctx, user)
	if err != nil || len(grants) != 1 {
		t.Fatalf("task grant audit view unavailable: %#v %v", grants, err)
	}
	if grants[0].ActorUserID != user.UserID || grants[0].Purpose != "install locked dependencies" || grants[0].LastOperation == nil || grants[0].LastOperation.Status != domain.StatusReady {
		t.Fatalf("task grant audit view lost decision context: %#v", grants[0])
	}
	if _, err = service.RevokeGrant(ctx, user, grantDecision.GrantID, facade.RevokeRequest{ExpectedVersion: 1, IdempotencyKey: "revoke-task-grant"}); err != nil {
		t.Fatal(err)
	}
	var grantStatus string
	if err = pool.QueryRow(ctx, `SELECT status FROM workforce_task_network_grants WHERE id=$1`, grantDecision.GrantID).Scan(&grantStatus); err != nil || grantStatus != domain.GrantRevoked {
		t.Fatalf("grant was not revoked: %q %v", grantStatus, err)
	}

	deniedOperation := publish("deny")
	denied, err := service.Decide(ctx, user, deniedOperation.OperationID, facade.DecisionRequest{ExpectedVersion: 1, Action: domain.DecisionDeny, IdempotencyKey: "deny-operation"})
	if err != nil || denied.Status != domain.StatusDenied {
		t.Fatalf("DENY failed: %#v %v", denied, err)
	}

	claim, err := service.Claim(ctx, machine, facade.ClaimRequest{OperationID: one.OperationID})
	if err != nil {
		t.Fatal(err)
	}
	assertOperationIdentityFenced(t, machine, func(stale facade.Machine) error {
		_, claimErr := service.Claim(ctx, stale, facade.ClaimRequest{OperationID: one.OperationID})
		return claimErr
	})
	assertExpiredOperationAttemptFenced(t, ctx, pool, claim.AttemptID, func() error {
		_, claimErr := service.Claim(ctx, machine, facade.ClaimRequest{OperationID: one.OperationID})
		return claimErr
	})
	startRequest := facade.StartRequest{OperationID: one.OperationID, AttemptID: claim.AttemptID, ContainerID: "runner-id", RelayIdentity: "relay-digest"}
	assertOperationIdentityFenced(t, machine, func(stale facade.Machine) error {
		_, startErr := service.Start(ctx, stale, startRequest)
		return startErr
	})
	assertExpiredOperationAttemptFenced(t, ctx, pool, claim.AttemptID, func() error {
		_, startErr := service.Start(ctx, machine, startRequest)
		return startErr
	})
	if _, err = service.Start(ctx, machine, startRequest); err != nil {
		t.Fatal(err)
	}
	renewRequest := facade.RenewRequest{OperationID: one.OperationID, AttemptID: claim.AttemptID}
	assertOperationIdentityFenced(t, machine, func(stale facade.Machine) error {
		_, renewErr := service.Renew(ctx, stale, renewRequest)
		return renewErr
	})
	assertExpiredOperationAttemptFenced(t, ctx, pool, claim.AttemptID, func() error {
		_, renewErr := service.Renew(ctx, machine, renewRequest)
		return renewErr
	})
	if _, err = service.Renew(ctx, machine, renewRequest); err != nil {
		t.Fatal(err)
	}
	exitCode := 0
	result := facade.ResultRequest{OperationID: one.OperationID, AttemptID: claim.AttemptID, Status: domain.StatusSucceeded, ExitCode: &exitCode, Summary: "dependencies installed", Output: "done"}
	result.ResultDigest = domain.ResultDigest(result.Status, result.ExitCode, result.Summary, result.Output)
	if _, err = service.ReadResultReceipt(ctx, machine, facade.ResultReceiptRequest{OperationID: one.OperationID, AttemptID: claim.AttemptID, ResultDigest: result.ResultDigest}); !errors.Is(err, domain.ErrStale) {
		t.Fatalf("uncommitted result produced a terminal receipt: %v", err)
	}
	assertOperationIdentityFenced(t, machine, func(stale facade.Machine) error {
		_, resultErr := service.SubmitResult(ctx, stale, result)
		return resultErr
	})
	assertExpiredOperationAttemptFenced(t, ctx, pool, claim.AttemptID, func() error {
		_, resultErr := service.SubmitResult(ctx, machine, result)
		return resultErr
	})
	if _, err = service.SubmitResult(ctx, machine, result); err != nil {
		t.Fatal(err)
	}
	receipt, err := service.ReadResultReceipt(ctx, machine, facade.ResultReceiptRequest{OperationID: one.OperationID, AttemptID: claim.AttemptID, ResultDigest: result.ResultDigest})
	if err != nil || !receipt.Replayed || receipt.Status != domain.StatusSucceeded || receipt.ResultDigest != result.ResultDigest {
		t.Fatalf("terminal receipt failed: %#v %v", receipt, err)
	}
	wrongDigest := strings.Repeat("f", 64)
	if _, err = service.ReadResultReceipt(ctx, machine, facade.ResultReceiptRequest{OperationID: one.OperationID, AttemptID: claim.AttemptID, ResultDigest: wrongDigest}); !errors.Is(err, domain.ErrIdempotencyConflict) {
		t.Fatalf("different terminal digest was not rejected: %v", err)
	}

	assertOperationIdentityFenced(t, machine, func(stale facade.Machine) error {
		_, receiptErr := service.ReadResultReceipt(ctx, stale, facade.ResultReceiptRequest{OperationID: one.OperationID, AttemptID: claim.AttemptID, ResultDigest: result.ResultDigest})
		return receiptErr
	})
	if _, err = service.ClaimNext(ctx, machine); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("terminal operation became claimable after response loss: %v", err)
	}
	if _, err = pool.Exec(ctx, `UPDATE workforce_execution_operation_attempts SET lease_expires_at=CURRENT_TIMESTAMP-INTERVAL '1 second' WHERE id=$1`, claim.AttemptID); err != nil {
		t.Fatal(err)
	}
	if _, err = service.ReadResultReceipt(ctx, machine, facade.ResultReceiptRequest{OperationID: one.OperationID, AttemptID: claim.AttemptID, ResultDigest: result.ResultDigest}); !errors.Is(err, domain.ErrFenced) {
		t.Fatalf("expired receipt lease accepted: %v", err)
	}

	expiringOperation := publish("expiring-task-grant")
	expiringDecision, err := service.Decide(ctx, user, expiringOperation.OperationID, facade.DecisionRequest{ExpectedVersion: 1, Action: domain.DecisionAllowForTask, IdempotencyKey: "allow-expiring-task"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = pool.Exec(ctx, `UPDATE workforce_task_network_grants SET created_at=CURRENT_TIMESTAMP-INTERVAL '2 hours',expires_at=CURRENT_TIMESTAMP-INTERVAL '1 hour' WHERE id=$1`, expiringDecision.GrantID); err != nil {
		t.Fatal(err)
	}
	if _, err = service.ReconcileExpired(ctx, 20); err != nil {
		t.Fatal(err)
	}
	if err = pool.QueryRow(ctx, `SELECT status FROM workforce_task_network_grants WHERE id=$1`, expiringDecision.GrantID).Scan(&grantStatus); err != nil || grantStatus != domain.GrantExpired {
		t.Fatalf("grant expiry did not converge: %q %v", grantStatus, err)
	}

	cancelOperation := publish("cancel")
	cancelTx, beginErr := pool.Begin(ctx)
	if beginErr != nil {
		t.Fatal(beginErr)
	}
	if err = service.CancelTaskTx(ctx, cancelTx, tenantID, taskID, time.Now().UTC()); err != nil {
		_ = cancelTx.Rollback(ctx)
		t.Fatal(err)
	}
	if err = cancelTx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	var cancelStatus string
	if err = pool.QueryRow(ctx, `SELECT status FROM workforce_execution_operations WHERE id=$1`, cancelOperation.OperationID).Scan(&cancelStatus); err != nil || cancelStatus != domain.StatusCancelled {
		t.Fatalf("task cancel did not fence operation: %q %v", cancelStatus, err)
	}
}

func assertOperationIdentityFenced(t *testing.T, current facade.Machine, call func(facade.Machine) error) {
	t.Helper()
	for name, mutate := range map[string]func(*facade.Machine){
		"old worker":   func(m *facade.Machine) { m.WorkerID++ },
		"old instance": func(m *facade.Machine) { m.InstanceID = "instance-old" },
		"old boot":     func(m *facade.Machine) { m.HostBootID = "boot-old" },
		"old owner":    func(m *facade.Machine) { m.OwnerEpoch-- },
		"old lease":    func(m *facade.Machine) { m.ExecutionLease = "lease-old" },
	} {
		t.Run(name, func(t *testing.T) {
			stale := current
			mutate(&stale)
			if err := call(stale); !errors.Is(err, domain.ErrFenced) {
				t.Fatalf("stale operation authority returned %v, want fenced", err)
			}
		})
	}
}

func assertExpiredOperationAttemptFenced(t *testing.T, ctx context.Context, pool *pgxpool.Pool, attemptID int64, call func() error) {
	t.Helper()
	if _, err := pool.Exec(ctx, `UPDATE workforce_execution_operation_attempts SET lease_expires_at=CURRENT_TIMESTAMP-INTERVAL '1 second' WHERE id=$1`, attemptID); err != nil {
		t.Fatal(err)
	}
	if err := call(); !errors.Is(err, domain.ErrFenced) {
		t.Fatalf("expired operation lease returned %v, want fenced", err)
	}
	if _, err := pool.Exec(ctx, `UPDATE workforce_execution_operation_attempts SET lease_expires_at=CURRENT_TIMESTAMP+INTERVAL '5 minutes' WHERE id=$1`, attemptID); err != nil {
		t.Fatal(err)
	}
}
