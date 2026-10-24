package infrastructure

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"github.com/CaixyPromise/seven-agent/force/internal/app/workforce/continuation/application"
	continuationdomain "github.com/CaixyPromise/seven-agent/force/internal/app/workforce/continuation/domain"
	interactionfacade "github.com/CaixyPromise/seven-agent/force/internal/app/workforce/interaction/facade"
	base "github.com/CaixyPromise/seven-agent/force/internal/app/workforce/resultprocessor/checkpoint"
	wcp "github.com/CaixyPromise/seven-agent/force/internal/worker/checkpoint"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
)

type suspendTestKeys struct{}

func (suspendTestKeys) Wrap(_ context.Context, key []byte) (application.KeyEnvelope, error) {
	return application.KeyEnvelope{KeyID: "test-only", Ciphertext: base64.StdEncoding.EncodeToString(key), EDEK: "test-only"}, nil
}
func (suspendTestKeys) Unwrap(_ context.Context, e application.KeyEnvelope) ([]byte, error) {
	return base64.StdEncoding.DecodeString(e.Ciphertext)
}

type suspendTestGate struct{ err error }

type suspendQuestionReader struct {
	target int64
	calls  int
}

func (r *suspendQuestionReader) RestoredQuestionTx(_ context.Context, tx pgx.Tx, tenant, task, target, response int64) (interactionfacade.PublishRequest, error) {
	r.calls++
	if tx == nil || tenant != 1 || task != 2 || target != r.target || response != 7 {
		return interactionfacade.PublishRequest{}, continuationdomain.ErrInteractionSuspend
	}
	var question interactionfacade.PublishRequest
	err := json.Unmarshal([]byte(`{"roundKey":"round:old","localRequestKey":"fake:question","questions":[{"key":"scope","title":"Choose scope","description":"Original server question","kind":"SINGLE_CHOICE","required":false,"allowFreeText":false,"options":[{"key":"small","label":"Small"}]}]}`), &question)
	return question, err
}

func (g suspendTestGate) AuthorizeInteractionSuspendTx(context.Context, pgx.Tx, application.InteractionSuspendOccurrence) error {
	return g.err
}

func TestInteractionSuspendPostgresReserveReadbackPublishAndSchema(t *testing.T) {
	dsn := os.Getenv("CONTINUATION_SUSPEND_TEST_POSTGRES_DSN")
	if dsn == "" {
		t.Skip("requires isolated local CONTINUATION_SUSPEND_TEST_POSTGRES_DSN")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	admin, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer admin.Close()
	schema := fmt.Sprintf("suspend_publish_%d", time.Now().UnixNano())
	if _, err = admin.Exec(ctx, "CREATE SCHEMA "+schema); err != nil {
		t.Fatal(err)
	}
	defer admin.Exec(context.Background(), "DROP SCHEMA "+schema+" CASCADE")
	cfg, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		t.Fatal(err)
	}
	cfg.ConnConfig.RuntimeParams["search_path"] = schema
	db, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if _, err = db.Exec(ctx, suspendTestSchema+suspendPublicationColumns+InteractionSchemaSQL); err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256([]byte("test-lease"))
	if _, err = db.Exec(ctx, `UPDATE workforce_task_attempts SET lease_token=$1`, hex.EncodeToString(sum[:])); err != nil {
		t.Fatal(err)
	}
	store := New(db, &suspendIDs{})
	objects := map[string][]byte{}
	var onRead func()
	reader := application.InteractionObjectReaderFunc(func(_ context.Context, key string) (io.ReadCloser, error) {
		if onRead != nil {
			f := onRead
			onRead = nil
			f()
		}
		body, ok := objects[key]
		if !ok {
			return nil, os.ErrNotExist
		}
		return io.NopCloser(bytes.NewReader(body)), nil
	})
	service := application.NewInteractionSuspend(store, suspendTestKeys{}, reader, suspendTestGate{})
	r := application.ReserveInteractionRequest{InteractionCaptureRequest: application.InteractionCaptureRequest{TenantID: 1, TaskID: 2, TaskTurnID: 3, InteractionID: 5, MachineAuthority: application.MachineAuthority{WorkerID: 20, InstanceID: "worker-session", HostBootID: "boot", AttemptID: 4, Generation: 1, OwnerEpoch: 2, LeaseToken: "test-lease"}}, AdapterSessionGeneration: 9, SessionDigest: "private-lineage"}
	r.CurrentRound = 1
	r.SuspendControlKey = "prepare-control-key"
	r.SessionDigest = ""
	answerWonService := application.NewInteractionSuspend(store, suspendTestKeys{}, reader, suspendTestGate{err: continuationdomain.ErrInteractionSuspendAnswerWon})
	won, err := answerWonService.ReserveInteractionSuspend(ctx, r)
	if err != nil || won.Disposition != "ANSWER_WON" || won.Reservation.ID != 0 || won.Key.PlainKeyBase64 != "" {
		t.Fatalf("answered occurrence became task failure or reserved capture: %v", err)
	}
	var checkpointCount int
	if err = db.QueryRow(ctx, `SELECT count(*) FROM workforce_continuation_checkpoints`).Scan(&checkpointCount); err != nil || checkpointCount != 0 {
		t.Fatal("answer winner mutated checkpoint state")
	}
	reserved, err := service.ReserveInteractionSuspend(ctx, r)
	if err != nil {
		t.Fatal(err)
	}
	replay, err := service.ReserveInteractionSuspend(ctx, r)
	if err != nil || replay.Reservation.ID != reserved.Reservation.ID || replay.Key.PlainKeyBase64 != reserved.Key.PlainKeyBase64 {
		t.Fatalf("reserve replay %+v %v", replay.Reservation, err)
	}
	r.CheckpointID = reserved.Reservation.ID
	r.CheckpointIdentity = reserved.Reservation.Identity
	for _, status := range []string{"EXPIRED", "INVALIDATED", "FAILED", "PENDING"} {
		if _, err = db.Exec(ctx, `UPDATE workforce_continuation_checkpoints SET status=$1 WHERE id=$2`, status, r.CheckpointID); err != nil {
			t.Fatalf("capturing terminal status %s: %v", status, err)
		}
	}
	key, _ := base64.StdEncoding.DecodeString(reserved.Key.PlainKeyBase64)
	root := t.TempDir()
	repo, state := filepath.Join(root, "repo"), filepath.Join(root, "state")
	for _, dir := range []string{repo, filepath.Join(state, "codex")} {
		if err = os.MkdirAll(dir, 0700); err != nil {
			t.Fatal(err)
		}
	}
	for path, content := range map[string]string{filepath.Join(repo, "file.txt"): "original", filepath.Join(state, "codex", "session.jsonl"): "native-private-session"} {
		if err = os.WriteFile(path, []byte(content), 0600); err != nil {
			t.Fatal(err)
		}
	}
	git := func(args ...string) string {
		out, e := wcp.Git(ctx, repo, args...)
		if e != nil {
			t.Fatal(e)
		}
		return out
	}
	git("init", "-q")
	git("add", ".")
	git("-c", "user.name=test", "-c", "user.email=test@example.invalid", "commit", "-qm", "fixture")
	baseCommit := git("rev-parse", "HEAD")
	if err = os.WriteFile(filepath.Join(repo, "file.txt"), []byte("changed"), 0600); err != nil {
		t.Fatal(err)
	}
	frozen, err := wcp.FreezeToSpool(ctx, wcp.Input{Checkpoint: base.Input{TaskID: 2, TaskTurnID: 3, AttemptID: 4, Generation: 1, Decision: "INTERACTION_SUSPEND", SessionDigest: "private-lineage", ExecutionSnapshotDigest: "frozen", RepoRoot: repo, StateRoot: state, BaseCommit: baseCommit}, InteractionID: 5, AdapterSessionGeneration: 9, AutomaticRevisionCount: 1, CurrentRound: r.CurrentRound, SuspendControlKeySHA256: reserved.Reservation.SuspendControlKeySHA256, ExecutionBudget: wcp.ExecutionBudget{Version: 1, TaskID: 2, AttemptID: 4, Generation: 1, OwnerEpoch: 2, ActiveLimitMillis: 600000, ActiveElapsedMillis: 1200, InteractionWaitingMillis: 90000, AbsoluteDeadlineUnixMillis: time.Now().Add(time.Hour).UnixMilli()}}, filepath.Join(root, "spool"))
	if err != nil || frozen.Identity != reserved.Reservation.Identity {
		t.Fatalf("blank-session reservation mismatched real native capture: %v", err)
	}
	read := func(path string) []byte {
		b, e := os.ReadFile(path)
		if e != nil {
			t.Fatal(e)
		}
		return b
	}
	bundle, overlay, session, body := read(frozen.RepositoryBundle.Path), read(frozen.WorkspaceOverlay.Path), read(frozen.Session.Path), read(frozen.Manifest.Path)
	m, err := wcp.DecodeManifest(body)
	if err != nil {
		t.Fatal(err)
	}
	bk, ok, sk, mk := base.ObjectKeysV2(base.Input{TaskID: 2, TaskTurnID: 3, AttemptID: 4}, r.CheckpointIdentity)
	encrypt := func(path, kind string, plain []byte) application.InteractionEncryptedBlob {
		var ciphertext bytes.Buffer
		if err := base.EncryptStream(bytes.NewReader(plain), &ciphertext, key, []byte("seven.continuation\x00"+r.CheckpointIdentity+"\x00"+kind)); err != nil {
			t.Fatal(err)
		}
		objects[path] = ciphertext.Bytes()
		sum := sha256.Sum256(ciphertext.Bytes())
		return application.InteractionEncryptedBlob{ObjectKey: path, SHA256: hex.EncodeToString(sum[:]), SizeBytes: int64(ciphertext.Len())}
	}
	publish := application.PublishInteractionRequest{InteractionCaptureRequest: r.InteractionCaptureRequest, Bundle: encrypt(bk, "repository-bundle", bundle), Overlay: encrypt(ok, "workspace-overlay", overlay), Session: encrypt(sk, "session", session), Manifest: encrypt(mk, "manifest", body)}
	checkPhase := func(want string) {
		var phase string
		if err := db.QueryRow(ctx, `SELECT interaction_phase FROM workforce_continuation_checkpoints WHERE id=$1`, r.CheckpointID).Scan(&phase); err != nil || phase != want {
			t.Fatalf("phase %s err=%v want=%s", phase, err, want)
		}
	}
	good := objects[sk]
	objects[sk] = []byte("tampered")
	if err = service.PublishInteractionSuspend(ctx, publish); err == nil {
		t.Fatal("tampered readback published")
	}
	checkPhase("CAPTURING")
	objects[sk] = good
	onRead = func() {
		if _, e := db.Exec(ctx, `UPDATE workforce_tasks SET cancel_requested_at=now() WHERE id=2`); e != nil {
			t.Fatal(e)
		}
	}
	if err = service.PublishInteractionSuspend(ctx, publish); err == nil {
		t.Fatal("cancel during readback published")
	}
	checkPhase("CAPTURING")
	if _, err = db.Exec(ctx, `UPDATE workforce_tasks SET cancel_requested_at=NULL WHERE id=2`); err != nil {
		t.Fatal(err)
	}
	if err = service.PublishInteractionSuspend(ctx, publish); err != nil {
		t.Fatal(err)
	}
	checkPhase("SUSPENDED")
	var sealed bool
	if err = db.QueryRow(ctx, `SELECT t.status='INTERACTION_SUSPENDED' AND tr.status='INTERACTION_SUSPENDED' AND a.status='INTERACTION_SUSPENDED' AND rt.status='INTERACTION_SUSPENDED' AND a.owner_epoch=3 AND rt.owner_epoch=3 AND a.lease_expires_at<=now() AND rt.execution_lease_expires_at<=now() AND rt.fenced_at IS NOT NULL FROM workforce_tasks t JOIN workforce_task_turns tr ON tr.id=t.current_task_turn_id JOIN workforce_task_attempts a ON a.id=t.latest_attempt_id JOIN workforce_attempt_runtimes rt ON rt.attempt_id=a.id WHERE t.id=2`).Scan(&sealed); err != nil || !sealed {
		t.Fatalf("publish did not durably park and seal source: %v", err)
	}
	if err = service.PublishInteractionSuspend(ctx, publish); err != nil {
		t.Fatalf("publish replay: %v", err)
	}
	var events int
	if err = db.QueryRow(ctx, `SELECT count(*) FROM workforce_task_events WHERE event_type='interaction.suspended'`).Scan(&events); err != nil || events != 1 {
		t.Fatalf("publication occurrence repeated: %d %v", events, err)
	}
	// The final copied constraint permits retention transitions and rejects raw
	// key/session additions even if a buggy writer bypasses the application.
	for _, status := range []string{"EXPIRED", "INVALIDATED", "FAILED", "AVAILABLE"} {
		if _, err = db.Exec(ctx, `UPDATE workforce_continuation_checkpoints SET status=$1 WHERE id=$2`, status, r.CheckpointID); err != nil {
			t.Fatalf("retention %s: %v", status, err)
		}
	}
	if _, err = db.Exec(ctx, `UPDATE workforce_continuation_checkpoints SET interaction_manifest_json=interaction_manifest_json||'{"plainKeyBase64":"forbidden"}'::jsonb WHERE id=$1`, r.CheckpointID); err == nil {
		t.Fatal("raw key allowed by schema")
	}
	if _, err = db.Exec(ctx, `UPDATE workforce_continuation_checkpoints SET interaction_manifest_json=jsonb_set(interaction_manifest_json,'{sessionDigest}',to_jsonb(repeat('x',1048577))) WHERE id=$1`, r.CheckpointID); err == nil {
		t.Fatal("unbounded manifest allowed")
	}
	if _, err = db.Exec(ctx, `UPDATE workforce_continuation_checkpoints SET interaction_manifest_json='[]'::jsonb WHERE id=$1`, r.CheckpointID); err == nil {
		t.Fatal("nonobject manifest allowed")
	}
	if _, err = db.Exec(ctx, InteractionSchemaSQL); err != nil {
		t.Fatalf("alignment replay: %v", err)
	}
	// Full consume under the real CHECK constraints, including atomic RESUMED
	// phase and private saved-answer copy, not just unconstrained fixture tables.
	tx, err := db.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(ctx)
	_, err = application.NewInteractionResume(store).ResumeInteractionTx(ctx, tx, application.InteractionResumeRequest{TenantID: 1, TaskID: 2, SourceTaskTurnID: 3, SourceAttemptID: 4, InteractionID: 5, CheckpointID: r.CheckpointID, AnswerID: 7, CreatorUserID: 8, Instruction: "cold answer", InteractionResponse: []byte(`{"interactionKey":"fake:question","responseId":"7","answers":[]}`)})
	if err != nil {
		t.Fatal(err)
	}
	if err = tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	checkPhase("RESUMED")
	var nextTurn int64
	if err = db.QueryRow(ctx, `SELECT current_task_turn_id FROM workforce_tasks WHERE id=2`).Scan(&nextTurn); err != nil {
		t.Fatal(err)
	}
	if _, err = db.Exec(ctx, `INSERT INTO workforce_task_attempts(id,task_id,task_turn_id,status,worker_id,worker_instance_id,host_boot_id,generation,owner_epoch,lease_token,lease_expires_at) VALUES(2000,2,$1,'PREPARING',20,'worker-session','boot',1,3,$2,now()+interval '1 hour')`, nextTurn, hex.EncodeToString(sum[:])); err != nil {
		t.Fatal(err)
	}
	if _, err = db.Exec(ctx, `INSERT INTO workforce_continuation_restore_runs(id,task_id,target_turn_id,checkpoint_id,attempt_id,status) VALUES(2001,2,$1,$2,2000,'PENDING')`, nextTurn, r.CheckpointID); err != nil {
		t.Fatal(err)
	}
	if _, err = db.Exec(ctx, `UPDATE workforce_tasks SET latest_attempt_id=2000,status='PREPARING' WHERE id=2`); err != nil {
		t.Fatal(err)
	}
	newAuthority := r.MachineAuthority
	newAuthority.AttemptID = 2000
	newAuthority.OwnerEpoch = 3
	if _, err = store.ResolveInteractionRestoreInput(ctx, newAuthority, 2001); err == nil {
		t.Fatal("missing trusted question reader restored")
	}
	questions := &suspendQuestionReader{target: nextTurn}
	store.WithInteractionRestoreQuestions(questions)
	restoreInput, err := store.ResolveInteractionRestoreInput(ctx, newAuthority, 2001)
	if err != nil || restoreInput.Manifest.CheckpointIdentity != m.CheckpointIdentity || !bytes.Contains(restoreInput.InteractionResponse, []byte("fake:question")) {
		t.Fatalf("private restore input %+v %v", restoreInput, err)
	}
	if !bytes.Contains(restoreInput.QuestionRequest, []byte("Original server question")) || !bytes.Contains(restoreInput.QuestionRequest, []byte(`"options"`)) || bytes.Contains(restoreInput.QuestionRequest, []byte("roundKey")) {
		t.Fatal("trusted full question projection lost or leaked old round")
	}
	questions.target = nextTurn + 1
	if _, err = store.ResolveInteractionRestoreInput(ctx, newAuthority, 2001); err == nil {
		t.Fatal("wrong resume target question restored")
	}
	questions.target = nextTurn
	keyService := application.New(store)
	keyService.SetKeyWrapper(suspendTestKeys{})
	material, err := keyService.ResolveRestoreKey(ctx, newAuthority, 2001)
	if err != nil || material.InteractionRestoreInput == nil || material.InteractionRestoreInput.Manifest != restoreInput.Manifest || !bytes.Equal(material.InteractionRestoreInput.InteractionResponse, restoreInput.InteractionResponse) {
		t.Fatalf("existing machine restore key contract missing verified input: %v", err)
	}
	if material.InteractionRestoreInput.ExecutionBudget != restoreInput.ExecutionBudget || restoreInput.ExecutionBudget.AttemptID != newAuthority.AttemptID || restoreInput.ExecutionBudget.ActiveElapsedMillis != m.ExecutionBudget.ActiveElapsedMillis || restoreInput.ExecutionBudget.InteractionWaitingMillis != m.ExecutionBudget.InteractionWaitingMillis {
		t.Fatal("restore replay extended deadline or reset source counters")
	}
	wire, err := json.Marshal(material)
	if err != nil || !bytes.Contains(wire, []byte(`"interactionRestoreInput"`)) || !bytes.Contains(wire, []byte(`"attemptId":"4"`)) || bytes.Contains(wire, []byte(`"leaseToken"`)) || bytes.Contains(wire, []byte(`"sessionArchive"`)) {
		t.Fatal("machine restore DTO lost decimal IDs or exposed raw authority/session")
	}
	if _, err = store.ResolveInteractionRestoreInput(ctx, r.MachineAuthority, 2001); err == nil {
		t.Fatal("old attempt obtained restored session contract")
	}
	receipt := sha256.Sum256([]byte(strings.Join([]string{strconv.FormatInt(r.CheckpointID, 10), m.SnapshotDigest, m.ResultCommit, m.ResultCommit, m.ResultTree, m.SessionDigest}, "\x00")))
	if _, err = db.Exec(ctx, `UPDATE workforce_continuation_restore_runs SET status='RESTORED',restored_identity_digest=$1 WHERE id=2001`, hex.EncodeToString(receipt[:])); err != nil {
		t.Fatal(err)
	}
	replayed, err := keyService.ResolveRestoreKey(ctx, newAuthority, 2001)
	if err != nil || replayed.InteractionRestoreInput == nil || replayed.InteractionRestoreInput.ExecutionBudget != restoreInput.ExecutionBudget || !bytes.Equal(replayed.InteractionRestoreInput.QuestionRequest, restoreInput.QuestionRequest) {
		t.Fatalf("lost restore response did not replay same authenticated metadata: %v", err)
	}
	if _, err = db.Exec(ctx, `UPDATE workforce_continuation_restore_runs SET restored_identity_digest='wrong' WHERE id=2001`); err != nil {
		t.Fatal(err)
	}
	if _, err = keyService.ResolveRestoreKey(ctx, newAuthority, 2001); err == nil {
		t.Fatal("wrong completed restore digest replayed")
	}
	if _, err = db.Exec(ctx, `UPDATE workforce_continuation_restore_runs SET restored_identity_digest=$1 WHERE id=2001`, hex.EncodeToString(receipt[:])); err != nil {
		t.Fatal(err)
	}
	if _, err = db.Exec(ctx, `UPDATE workforce_task_attempts SET status='RUNNING' WHERE id=2000`); err != nil {
		t.Fatal(err)
	}
	if _, err = keyService.ResolveRestoreKey(ctx, newAuthority, 2001); err == nil {
		t.Fatal("already launched attempt replayed restore key")
	}
	if _, err = db.Exec(ctx, `UPDATE workforce_task_attempts SET status='PREPARING' WHERE id=2000`); err != nil {
		t.Fatal(err)
	}
	if _, err = db.Exec(ctx, `UPDATE workforce_tasks SET cancel_requested_at=now() WHERE id=2`); err != nil {
		t.Fatal(err)
	}
	if _, err = store.ResolveInteractionRestoreInput(ctx, newAuthority, 2001); err == nil {
		t.Fatal("cancelled target restored")
	}
	if material, err = keyService.ResolveRestoreKey(ctx, newAuthority, 2001); err == nil || material.PlainKeyBase64 != "" || material.InteractionRestoreInput != nil {
		t.Fatal("cancelled target obtained key or private restore input through machine contract")
	}
	if _, err = db.Exec(ctx, `UPDATE workforce_continuation_checkpoints SET status='EXPIRED' WHERE id=$1`, r.CheckpointID); err != nil {
		t.Fatalf("consumed GC: %v", err)
	}
}

const suspendPublicationColumns = `
CREATE TABLE workforce_continuation_restore_runs(id BIGINT,task_id BIGINT,target_turn_id BIGINT,checkpoint_id BIGINT,attempt_id BIGINT,status TEXT,restored_identity_digest TEXT);
DELETE FROM workforce_continuation_checkpoints;
ALTER TABLE workforce_continuation_checkpoints ADD evaluation_cycle_id BIGINT,ADD checkpoint_identity_sha256 TEXT,
 ADD approved_by_principal_id BIGINT,ADD available_at TIMESTAMPTZ,
 ADD repository_bundle_object_key TEXT DEFAULT '',ADD repository_bundle_content_sha256 TEXT DEFAULT '',ADD repository_bundle_object_sha256 TEXT DEFAULT '',ADD repository_bundle_size_bytes BIGINT DEFAULT 0,
 ADD workspace_overlay_object_key TEXT DEFAULT '',ADD workspace_overlay_content_sha256 TEXT DEFAULT '',ADD workspace_overlay_object_sha256 TEXT DEFAULT '',ADD workspace_overlay_size_bytes BIGINT DEFAULT 0,ADD selection_digest TEXT DEFAULT '',
 ADD session_object_key TEXT NOT NULL DEFAULT '',ADD session_sha256 TEXT NOT NULL DEFAULT '',ADD session_content_sha256 TEXT DEFAULT '',ADD session_object_sha256 TEXT DEFAULT '',ADD session_size_bytes BIGINT DEFAULT 0,
 ADD identity_manifest_object_key TEXT NOT NULL DEFAULT '',ADD manifest_sha256 TEXT NOT NULL DEFAULT '',ADD manifest_content_sha256 TEXT DEFAULT '',ADD manifest_object_sha256 TEXT DEFAULT '',ADD manifest_size_bytes BIGINT DEFAULT 0,
 ADD base_commit TEXT DEFAULT '',ADD result_commit TEXT DEFAULT '',ADD result_tree TEXT DEFAULT '',ADD encryption_algorithm TEXT DEFAULT '',ADD encryption_key_id TEXT DEFAULT '',ADD wrapped_data_key TEXT DEFAULT '';
ALTER TABLE workforce_task_attempts ADD worker_id BIGINT,ADD worker_instance_id TEXT,ADD host_boot_id TEXT,ADD generation BIGINT,ADD lease_token TEXT;
CREATE TABLE workforce_workers(id BIGINT,current_instance_id TEXT);
CREATE TABLE workforce_worker_sessions(instance_id TEXT,host_boot_id TEXT,ended_at TIMESTAMPTZ);
INSERT INTO workforce_workers VALUES(20,'worker-session');INSERT INTO workforce_worker_sessions VALUES('worker-session','boot',NULL);
UPDATE workforce_task_attempts SET worker_id=20,worker_instance_id='worker-session',host_boot_id='boot',generation=1,owner_epoch=2,lease_expires_at=now()+interval '1 hour';
UPDATE workforce_tasks SET status='RUNNING';UPDATE workforce_task_turns SET status='ACTIVE';UPDATE workforce_task_attempts SET status='RUNNING';UPDATE workforce_execution_nodes SET status='RUNNING';
UPDATE workforce_attempt_runtimes SET status='RUNNING',owner_epoch=2,execution_lease_expires_at=now()+interval '1 hour',fenced_at=NULL;
`
