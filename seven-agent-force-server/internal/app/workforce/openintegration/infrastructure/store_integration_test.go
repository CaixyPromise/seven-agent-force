package infrastructure

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/CaixyPromise/seven-agent/force/internal/app/workforce/openintegration/domain"
	secretvalue "github.com/CaixyPromise/seven-agent/force/internal/infrastructure/security/crypto/secretvalue"
)

type integrationTestIDGen struct{ next int64 }

func (g *integrationTestIDGen) NextID() int64 { g.next++; return g.next }

type atomicIntegrationTestIDGen struct{ next atomic.Int64 }

func newAtomicIntegrationTestIDGen(start int64) *atomicIntegrationTestIDGen {
	result := &atomicIntegrationTestIDGen{}
	result.next.Store(start)
	return result
}

func (g *atomicIntegrationTestIDGen) NextID() int64 { return g.next.Add(1) }

func TestCredentialRotationSerializesAndKeepsOnlyCurrentAndGrace(t *testing.T) {
	dsn := strings.TrimSpace(os.Getenv("OPEN_INTEGRATION_TEST_POSTGRES_DSN"))
	if dsn == "" {
		t.Skip("set OPEN_INTEGRATION_TEST_POSTGRES_DSN to an isolated migrated PostgreSQL database")
	}
	ctx := context.Background()
	db, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	idGen := newAtomicIntegrationTestIDGen(8_700_000_000_000_000_000)
	store := NewStore(db, idGen)
	locatorV1, tokenV1, digestV1, hintV1, err := domain.GenerateAPIKey()
	if err != nil {
		t.Fatal(err)
	}
	application, err := store.CreateApplication(ctx, CreateApplicationParams{
		TenantID: 94_001, Name: "rotation integration", WebhookURL: "https://example.com/hook",
		CredentialID: locatorV1, CredentialDigest: digestV1, CredentialHint: hintV1,
		SigningCredential: &CreateSigningCredential{
			Hint: "sign-v1", Secret: secretvalue.SecretValue{CiphertextB64: "Y2lwaGVyLXYx", EDEKB64: "ZWRlay12MQ==", WrapKeyRef: "key-v1"},
		},
		CreatedByUserID: 101,
	})
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		_, _ = db.Exec(context.Background(), `DELETE FROM workforce_integration_api_credentials WHERE integration_application_id=$1`, application.ID)
		_, _ = db.Exec(context.Background(), `DELETE FROM workforce_integration_signing_credentials WHERE integration_application_id=$1`, application.ID)
		_, _ = db.Exec(context.Background(), `DELETE FROM workforce_integration_applications WHERE id=$1`, application.ID)
	}()
	if application.APICredential.ID <= 0 || application.APICredential.Version != 1 || application.SigningCredential == nil || application.SigningCredential.ID <= 0 || application.SigningCredential.Version != 1 {
		t.Fatalf("CreateApplication summaries are incomplete: %+v", application)
	}

	now := time.Now().UTC().Truncate(time.Microsecond)
	if _, err = store.RotateAPIKey(ctx, RotateAPIKeyParams{
		TenantID: application.TenantID, ApplicationID: application.ID, ActorUserID: 101,
		CredentialLocator: locatorV1, CredentialDigest: digestV1, CredentialHint: hintV1,
		GracePeriod: 15 * time.Minute,
	}); err == nil {
		t.Fatal("duplicate locator rotation unexpectedly succeeded")
	}
	var originalStatus string
	var credentialCount int
	if err = db.QueryRow(ctx, `SELECT status FROM workforce_integration_api_credentials WHERE id=$1`, application.APICredential.ID).Scan(&originalStatus); err != nil {
		t.Fatal(err)
	}
	if err = db.QueryRow(ctx, `SELECT count(*) FROM workforce_integration_api_credentials WHERE integration_application_id=$1`, application.ID).Scan(&credentialCount); err != nil {
		t.Fatal(err)
	}
	if originalStatus != domain.CredentialCurrent || credentialCount != 1 {
		t.Fatalf("failed rotation did not roll back: status=%s count=%d", originalStatus, credentialCount)
	}
	type rotationInput struct {
		locator, token, digest, hint string
	}
	inputs := make([]rotationInput, 2)
	for i := range inputs {
		inputs[i].locator, inputs[i].token, inputs[i].digest, inputs[i].hint, err = domain.GenerateAPIKey()
		if err != nil {
			t.Fatal(err)
		}
	}
	records := make([]CredentialRotationRecord, len(inputs))
	errorsByIndex := make([]error, len(inputs))
	var ready sync.WaitGroup
	ready.Add(len(inputs))
	start := make(chan struct{})
	var done sync.WaitGroup
	done.Add(len(inputs))
	for i := range inputs {
		go func(index int) {
			defer done.Done()
			ready.Done()
			<-start
			records[index], errorsByIndex[index] = store.RotateAPIKey(ctx, RotateAPIKeyParams{
				TenantID: application.TenantID, ApplicationID: application.ID, ActorUserID: 101,
				CredentialLocator: inputs[index].locator, CredentialDigest: inputs[index].digest, CredentialHint: inputs[index].hint,
				GracePeriod: 15 * time.Minute,
			})
		}(i)
	}
	ready.Wait()
	close(start)
	done.Wait()
	for i, rotationErr := range errorsByIndex {
		if rotationErr != nil {
			t.Fatalf("concurrent rotation %d: %v", i, rotationErr)
		}
	}
	versions := map[int]bool{records[0].Version: true, records[1].Version: true}
	if !versions[2] || !versions[3] {
		t.Fatalf("serialized versions=%v, want v2 and v3", versions)
	}
	recordByVersion := map[int]CredentialRotationRecord{records[0].Version: records[0], records[1].Version: records[1]}
	if recordByVersion[3].CreatedAt.Before(recordByVersion[2].CreatedAt) {
		t.Fatalf("serialized rotation times are not monotonic: v2=%s v3=%s", recordByVersion[2].CreatedAt, recordByVersion[3].CreatedAt)
	}
	for version, record := range recordByVersion {
		if got := record.PreviousValidUntil.Sub(record.CreatedAt); got != 15*time.Minute {
			t.Fatalf("v%d grace duration=%s, want 15m from the locked database rotation time", version, got)
		}
	}

	var currentCount, graceCount, revokedCount int
	if err = db.QueryRow(ctx, `SELECT count(*) FILTER (WHERE status='CURRENT'),count(*) FILTER (WHERE status='GRACE'),count(*) FILTER (WHERE status='REVOKED')
FROM workforce_integration_api_credentials WHERE integration_application_id=$1`, application.ID).Scan(&currentCount, &graceCount, &revokedCount); err != nil {
		t.Fatal(err)
	}
	if currentCount != 1 || graceCount != 1 || revokedCount != 1 {
		t.Fatalf("credential states current=%d grace=%d revoked=%d", currentCount, graceCount, revokedCount)
	}
	if _, err = store.Authenticate(ctx, tokenV1); !errors.Is(err, ErrUnauthorized) {
		t.Fatalf("revoked v1 Authenticate error=%v, want unauthorized", err)
	}
	var currentLocator, graceLocator string
	if err = db.QueryRow(ctx, `SELECT credential_locator FROM workforce_integration_api_credentials WHERE integration_application_id=$1 AND status='CURRENT'`, application.ID).Scan(&currentLocator); err != nil {
		t.Fatal(err)
	}
	if err = db.QueryRow(ctx, `SELECT credential_locator FROM workforce_integration_api_credentials WHERE integration_application_id=$1 AND status='GRACE'`, application.ID).Scan(&graceLocator); err != nil {
		t.Fatal(err)
	}
	for _, input := range inputs {
		if input.locator == currentLocator || input.locator == graceLocator {
			if _, err = store.Authenticate(ctx, input.token); err != nil {
				t.Fatalf("active locator %s was rejected: %v", input.locator, err)
			}
		}
	}
	if _, err = db.Exec(ctx, `UPDATE workforce_integration_api_credentials SET valid_until=$2 WHERE integration_application_id=$1 AND status='GRACE'`, application.ID, now.Add(-time.Minute)); err != nil {
		t.Fatal(err)
	}
	for _, input := range inputs {
		if input.locator == graceLocator {
			if _, err = store.Authenticate(ctx, input.token); !errors.Is(err, ErrUnauthorized) {
				t.Fatalf("expired grace Authenticate error=%v, want unauthorized", err)
			}
		}
	}

	signingV2, err := store.RotateSigningSecret(ctx, RotateSigningSecretParams{
		TenantID: application.TenantID, ApplicationID: application.ID, ActorUserID: 101,
		SigningSecretHint: "sign-v2", GracePeriod: 24 * time.Hour,
	}, secretvalue.SecretValue{CiphertextB64: "Y2lwaGVyLXYy", EDEKB64: "ZWRlay12Mg==", WrapKeyRef: "key-v2"})
	if err != nil || signingV2.Version != 2 || signingV2.PreviousCredentialID != application.SigningCredential.ID {
		t.Fatalf("RotateSigningSecret v2=%+v err=%v", signingV2, err)
	}
	if _, err = store.RotateSigningSecret(ctx, RotateSigningSecretParams{
		TenantID: application.TenantID, ApplicationID: application.ID, ActorUserID: 101,
		SigningSecretHint: "sign-v3", GracePeriod: 24 * time.Hour,
	}, secretvalue.SecretValue{CiphertextB64: "Y2lwaGVyLXYz", EDEKB64: "ZWRlay12Mw==", WrapKeyRef: "key-v3"}); err != nil {
		t.Fatal(err)
	}
	if err = db.QueryRow(ctx, `SELECT count(*) FILTER (WHERE status='CURRENT'),count(*) FILTER (WHERE status='GRACE'),count(*) FILTER (WHERE status='REVOKED')
FROM workforce_integration_signing_credentials WHERE integration_application_id=$1`, application.ID).Scan(&currentCount, &graceCount, &revokedCount); err != nil {
		t.Fatal(err)
	}
	if currentCount != 1 || graceCount != 1 || revokedCount != 1 {
		t.Fatalf("signing states current=%d grace=%d revoked=%d", currentCount, graceCount, revokedCount)
	}

	applications, err := store.ListApplications(ctx, application.TenantID, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(applications) != 1 || applications[0].APICredential.ID <= 0 || applications[0].SigningCredential.Version != 3 || applications[0].SigningSecretGrace == nil {
		t.Fatalf("ListApplications credential summaries=%+v", applications)
	}
	if _, err = db.Exec(ctx, `UPDATE workforce_integration_signing_credentials SET valid_until=CURRENT_TIMESTAMP-INTERVAL '1 second' WHERE integration_application_id=$1 AND status='GRACE'`, application.ID); err != nil {
		t.Fatal(err)
	}
	publicEventID, deliveryID := idGen.NextID(), idGen.NextID()
	sourceTaskEventID, taskID, taskTurnID := idGen.NextID(), idGen.NextID(), idGen.NextID()
	if _, err = db.Exec(ctx, `INSERT INTO workforce_public_events
(id,tenant_id,integration_application_id,source_task_event_id,task_id,task_turn_id,event_type,sequence,correlation_id,payload_json,occurred_at)
VALUES ($1,$2,$3,$4,$5,$6,'task.succeeded',1,'rotation-expiry','{}'::jsonb,CURRENT_TIMESTAMP)`, publicEventID, application.TenantID, application.ID, sourceTaskEventID, taskID, taskTurnID); err != nil {
		t.Fatal(err)
	}
	if _, err = db.Exec(ctx, `INSERT INTO workforce_webhook_deliveries
(id,tenant_id,integration_application_id,public_event_id,task_id,event_type,status)
VALUES ($1,$2,$3,$4,$5,'task.succeeded','PENDING')`, deliveryID, application.TenantID, application.ID, publicEventID, taskID); err != nil {
		t.Fatal(err)
	}
	defer func() {
		_, _ = db.Exec(context.Background(), `DELETE FROM workforce_webhook_deliveries WHERE id=$1`, deliveryID)
		_, _ = db.Exec(context.Background(), `DELETE FROM workforce_public_events WHERE id=$1`, publicEventID)
	}()
	claims, err := store.ClaimDeliveries(ctx, "rotation-expiry-test", 1, time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	if len(claims) != 1 || claims[0].CurrentSigningCredential.ID != applications[0].SigningCredential.ID || claims[0].GraceSigningCredential != nil {
		t.Fatalf("expired Signing GRACE participated in claim: %+v", claims)
	}
	if err = store.SetApplicationStatus(ctx, application.TenantID, application.ID, domain.ApplicationDisabled); err != nil {
		t.Fatal(err)
	}
	for _, input := range inputs {
		if input.locator == currentLocator {
			if _, err = store.Authenticate(ctx, input.token); !errors.Is(err, ErrApplicationDisabled) {
				t.Fatalf("disabled current Authenticate error=%v, want application disabled", err)
			}
		}
	}
}

func TestAuthenticateThrottlesConcurrentLastUsedWrites(t *testing.T) {
	dsn := strings.TrimSpace(os.Getenv("OPEN_INTEGRATION_TEST_POSTGRES_DSN"))
	if dsn == "" {
		t.Skip("set OPEN_INTEGRATION_TEST_POSTGRES_DSN to an isolated migrated PostgreSQL database")
	}
	ctx := context.Background()
	db, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	idGen := newAtomicIntegrationTestIDGen(8_710_000_000_000_000_000)
	store := NewStore(db, idGen)
	locator, token, digest, hint, err := domain.GenerateAPIKey()
	if err != nil {
		t.Fatal(err)
	}
	application, err := store.CreateApplication(ctx, CreateApplicationParams{
		TenantID: 94_101, Name: "authentication touch throttle", WebhookURL: "",
		CredentialID: locator, CredentialDigest: digest, CredentialHint: hint, CreatedByUserID: 101,
	})
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		_, _ = db.Exec(context.Background(), `DELETE FROM workforce_integration_api_credentials WHERE integration_application_id=$1`, application.ID)
		_, _ = db.Exec(context.Background(), `DELETE FROM workforce_integration_applications WHERE id=$1`, application.ID)
	}()

	if _, err = store.Authenticate(ctx, token); err != nil {
		t.Fatal(err)
	}
	var initialCredentialUse, initialApplicationUse time.Time
	if err = db.QueryRow(ctx, `SELECT c.last_used_at,a.last_used_at
FROM workforce_integration_api_credentials c JOIN workforce_integration_applications a ON a.id=c.integration_application_id
WHERE c.id=$1`, application.APICredential.ID).Scan(&initialCredentialUse, &initialApplicationUse); err != nil {
		t.Fatal(err)
	}

	const callers = 24
	var ready sync.WaitGroup
	ready.Add(callers)
	start := make(chan struct{})
	errByCaller := make([]error, callers)
	var done sync.WaitGroup
	done.Add(callers)
	for i := 0; i < callers; i++ {
		go func(index int) {
			defer done.Done()
			ready.Done()
			<-start
			_, errByCaller[index] = store.Authenticate(ctx, token)
		}(i)
	}
	ready.Wait()
	close(start)
	done.Wait()
	for i, authenticateErr := range errByCaller {
		if authenticateErr != nil {
			t.Fatalf("concurrent Authenticate %d: %v", i, authenticateErr)
		}
	}
	var throttledCredentialUse, throttledApplicationUse time.Time
	if err = db.QueryRow(ctx, `SELECT c.last_used_at,a.last_used_at
FROM workforce_integration_api_credentials c JOIN workforce_integration_applications a ON a.id=c.integration_application_id
WHERE c.id=$1`, application.APICredential.ID).Scan(&throttledCredentialUse, &throttledApplicationUse); err != nil {
		t.Fatal(err)
	}
	if !throttledCredentialUse.Equal(initialCredentialUse) || !throttledApplicationUse.Equal(initialApplicationUse) {
		t.Fatalf("last_used_at changed inside throttle window: credential %s -> %s, application %s -> %s",
			initialCredentialUse, throttledCredentialUse, initialApplicationUse, throttledApplicationUse)
	}

	stale := time.Now().UTC().Add(-10 * time.Minute).Truncate(time.Microsecond)
	if _, err = db.Exec(ctx, `UPDATE workforce_integration_api_credentials SET last_used_at=$2 WHERE id=$1`, application.APICredential.ID, stale); err != nil {
		t.Fatal(err)
	}
	if _, err = db.Exec(ctx, `UPDATE workforce_integration_applications SET last_used_at=$2 WHERE id=$1`, application.ID, stale); err != nil {
		t.Fatal(err)
	}
	if _, err = store.Authenticate(ctx, token); err != nil {
		t.Fatal(err)
	}
	var refreshedCredentialUse, refreshedApplicationUse time.Time
	if err = db.QueryRow(ctx, `SELECT c.last_used_at,a.last_used_at
FROM workforce_integration_api_credentials c JOIN workforce_integration_applications a ON a.id=c.integration_application_id
WHERE c.id=$1`, application.APICredential.ID).Scan(&refreshedCredentialUse, &refreshedApplicationUse); err != nil {
		t.Fatal(err)
	}
	if !refreshedCredentialUse.After(stale) || !refreshedApplicationUse.After(stale) {
		t.Fatalf("stale last_used_at was not refreshed: credential=%s application=%s stale=%s", refreshedCredentialUse, refreshedApplicationUse, stale)
	}
}

func TestProjectorObservesConcurrentApplicationDisable(t *testing.T) {
	dsn := strings.TrimSpace(os.Getenv("OPEN_INTEGRATION_TEST_POSTGRES_DSN"))
	if dsn == "" {
		t.Skip("set OPEN_INTEGRATION_TEST_POSTGRES_DSN to an isolated migrated PostgreSQL database")
	}
	ctx := context.Background()
	db, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	tenantID, applicationID, taskID, turnID := int64(91001), int64(91002), int64(91003), int64(91004)
	if err = insertIntegrationTestApplication(ctx, db, tenantID, applicationID, "projection race", domain.ApplicationEnabled, "credential-projection-race"); err != nil {
		t.Fatal(err)
	}
	_, err = db.Exec(ctx, `INSERT INTO workforce_tasks
(id,tenant_id,idempotency_key,request_hash,created_by,repository_url,repository_ref,task_instruction,status,submission_channel,integration_application_id,scenario_key,scenario_version,scenario_digest,external_correlation_id,current_task_turn_id,result_processor_release_id,project_id,project_version_id,review_enabled,execution_requirements_json,execution_requirements_digest)
VALUES ($1,$2,$3,$4,$5,'','','test','SUCCEEDED','OPEN_REST',$6,'project-task',1,$7,'race-1',$8,1,$1,$1,TRUE,'{}'::jsonb,$4)`, taskID, tenantID, "idem-projection-race", strings.Repeat("a", 64), int64(101), applicationID, strings.Repeat("b", 64), turnID)
	if err != nil {
		t.Fatal(err)
	}
	_, err = db.Exec(ctx, `INSERT INTO workforce_task_events (task_id,event_type,sequence,payload_json) VALUES ($1,'task.succeeded',1,'{}'::jsonb)`, taskID)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		_, _ = db.Exec(context.Background(), `DELETE FROM workforce_webhook_delivery_attempts WHERE integration_application_id=$1`, applicationID)
		_, _ = db.Exec(context.Background(), `DELETE FROM workforce_webhook_deliveries WHERE integration_application_id=$1`, applicationID)
		_, _ = db.Exec(context.Background(), `DELETE FROM workforce_public_events WHERE integration_application_id=$1`, applicationID)
		_, _ = db.Exec(context.Background(), `DELETE FROM workforce_task_events WHERE task_id=$1`, taskID)
		_, _ = db.Exec(context.Background(), `DELETE FROM workforce_tasks WHERE id=$1`, taskID)
		_, _ = db.Exec(context.Background(), `DELETE FROM workforce_integration_api_credentials WHERE integration_application_id=$1`, applicationID)
		_, _ = db.Exec(context.Background(), `DELETE FROM workforce_integration_signing_credentials WHERE integration_application_id=$1`, applicationID)
		_, _ = db.Exec(context.Background(), `DELETE FROM workforce_integration_applications WHERE id=$1`, applicationID)
	}()

	reached := make(chan struct{})
	proceed := make(chan struct{})
	projector := NewStore(db, &integrationTestIDGen{next: 92000})
	projector.beforeProjectionApplicationLock = func() {
		close(reached)
		<-proceed
	}
	projected := make(chan error, 1)
	go func() {
		_, err := projector.ProjectTerminalEvents(ctx, 1)
		projected <- err
	}()
	select {
	case <-reached:
	case <-time.After(5 * time.Second):
		t.Fatal("projector did not reach application status boundary")
	}
	if err := NewStore(db, &integrationTestIDGen{}).SetApplicationStatus(ctx, tenantID, applicationID, domain.ApplicationDisabled); err != nil {
		t.Fatal(err)
	}
	close(proceed)
	if err := <-projected; err != nil {
		t.Fatal(err)
	}
	var deliveryStatus string
	if err := db.QueryRow(ctx, `SELECT status FROM workforce_webhook_deliveries WHERE integration_application_id=$1`, applicationID).Scan(&deliveryStatus); err != nil {
		t.Fatal(err)
	}
	if deliveryStatus != "SUSPENDED" {
		t.Fatalf("delivery created after disable has status %q", deliveryStatus)
	}
	var projectionStatus string
	if err := db.QueryRow(ctx, `SELECT open_integration_projection_status FROM workforce_task_events WHERE task_id=$1 AND event_type='task.succeeded'`, taskID).Scan(&projectionStatus); err != nil {
		t.Fatal(err)
	}
	if projectionStatus != "PROJECTED" {
		t.Fatalf("webhook terminal event projection status=%q, want PROJECTED", projectionStatus)
	}
	var payload []byte
	if err := db.QueryRow(ctx, `SELECT payload_json FROM workforce_public_events WHERE integration_application_id=$1`, applicationID).Scan(&payload); err != nil {
		t.Fatal(err)
	}
	var event map[string]any
	if err := json.Unmarshal(payload, &event); err != nil {
		t.Fatal(err)
	}
	links, ok := event["links"].(map[string]any)
	if !ok || links["task"] != "/api/open/workforce/v1/tasks/91003" || links["outputs"] != "/api/open/workforce/v1/tasks/91003/outputs" || len(links) != 2 {
		t.Fatalf("unexpected public event links: %#v", event["links"])
	}
}

func TestProjectorDoesNotRelockProjectedTerminalPrefix(t *testing.T) {
	dsn := strings.TrimSpace(os.Getenv("OPEN_INTEGRATION_TEST_POSTGRES_DSN"))
	if dsn == "" {
		t.Skip("set OPEN_INTEGRATION_TEST_POSTGRES_DSN to an isolated migrated PostgreSQL database")
	}
	ctx := context.Background()
	db, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	tenantID, applicationID := int64(94_201), int64(94_202)
	if err = insertIntegrationTestApplication(ctx, db, tenantID, applicationID, "projection prefix", domain.ApplicationEnabled, "credential-projection-prefix"); err != nil {
		t.Fatal(err)
	}
	defer func() {
		_, _ = db.Exec(context.Background(), `DELETE FROM workforce_webhook_delivery_attempts WHERE integration_application_id=$1`, applicationID)
		_, _ = db.Exec(context.Background(), `DELETE FROM workforce_webhook_deliveries WHERE integration_application_id=$1`, applicationID)
		_, _ = db.Exec(context.Background(), `DELETE FROM workforce_public_events WHERE integration_application_id=$1`, applicationID)
		_, _ = db.Exec(context.Background(), `DELETE FROM workforce_task_events WHERE task_id BETWEEN 9420300 AND 9420396`)
		_, _ = db.Exec(context.Background(), `DELETE FROM workforce_tasks WHERE integration_application_id=$1`, applicationID)
		_, _ = db.Exec(context.Background(), `DELETE FROM workforce_integration_api_credentials WHERE integration_application_id=$1`, applicationID)
		_, _ = db.Exec(context.Background(), `DELETE FROM workforce_integration_signing_credentials WHERE integration_application_id=$1`, applicationID)
		_, _ = db.Exec(context.Background(), `DELETE FROM workforce_integration_applications WHERE id=$1`, applicationID)
	}()
	const historicalCount = 96
	var firstHistoricalEventID int64
	for i := 0; i < historicalCount+1; i++ {
		taskID := int64(9_420_300 + i)
		turnID := int64(9_421_300 + i)
		if _, err = db.Exec(ctx, `INSERT INTO workforce_tasks
(id,tenant_id,idempotency_key,request_hash,created_by,repository_url,repository_ref,task_instruction,status,submission_channel,integration_application_id,scenario_key,scenario_version,scenario_digest,external_correlation_id,current_task_turn_id,result_processor_release_id,project_id,project_version_id,review_enabled,execution_requirements_json,execution_requirements_digest)
VALUES ($1,$2,$3,$4,$5,'','','test','SUCCEEDED','OPEN_REST',$6,'project-task',1,$7,$8,$9,1,$1,$1,TRUE,'{}'::jsonb,$4)`, taskID, tenantID, "idem-projection-prefix-"+strconv.Itoa(i), strings.Repeat("a", 64), int64(101), applicationID, strings.Repeat("b", 64), "prefix-"+strconv.Itoa(i), turnID); err != nil {
			t.Fatal(err)
		}
		status := "PROJECTED"
		if i == historicalCount {
			status = "PENDING"
		}
		var eventID int64
		if err = db.QueryRow(ctx, `INSERT INTO workforce_task_events (task_id,event_type,sequence,payload_json,open_integration_projection_status)
VALUES ($1,'task.succeeded',1,'{}'::jsonb,$2) RETURNING id`, taskID, status).Scan(&eventID); err != nil {
			t.Fatal(err)
		}
		if i == 0 {
			firstHistoricalEventID = eventID
		}
	}
	reached := make(chan struct{})
	proceed := make(chan struct{})
	projector := NewStore(db, &integrationTestIDGen{next: 9_430_000})
	projector.beforeProjectionApplicationLock = func() {
		close(reached)
		<-proceed
	}
	projected := make(chan error, 1)
	go func() {
		count, projectErr := projector.ProjectTerminalEvents(ctx, 1)
		if projectErr == nil && count != 1 {
			projectErr = fmt.Errorf("projected count=%d, want 1", count)
		}
		projected <- projectErr
	}()
	select {
	case <-reached:
	case <-time.After(5 * time.Second):
		t.Fatal("projector did not select the pending event")
	}
	probeTx, err := db.Begin(ctx)
	if err != nil {
		close(proceed)
		t.Fatal(err)
	}
	var lockedID int64
	probeErr := probeTx.QueryRow(ctx, `SELECT id FROM workforce_task_events WHERE id=$1 FOR UPDATE NOWAIT`, firstHistoricalEventID).Scan(&lockedID)
	_ = probeTx.Rollback(ctx)
	close(proceed)
	if probeErr != nil {
		t.Fatalf("projector relocked historical PROJECTED event %d: %v", firstHistoricalEventID, probeErr)
	}
	if projectErr := <-projected; projectErr != nil {
		t.Fatal(projectErr)
	}
}

func TestProjectorConsumesOrdinaryTerminalPrefixBeforeOpenEvent(t *testing.T) {
	dsn := strings.TrimSpace(os.Getenv("OPEN_INTEGRATION_TEST_POSTGRES_DSN"))
	if dsn == "" {
		t.Skip("set OPEN_INTEGRATION_TEST_POSTGRES_DSN to an isolated migrated PostgreSQL database")
	}
	ctx := context.Background()
	db, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	tenantID, applicationID := int64(94_401), int64(94_402)
	if err = insertIntegrationTestApplication(ctx, db, tenantID, applicationID, "ordinary prefix", domain.ApplicationEnabled, "credential-ordinary-prefix"); err != nil {
		t.Fatal(err)
	}
	defer func() {
		_, _ = db.Exec(context.Background(), `DELETE FROM workforce_webhook_delivery_attempts WHERE integration_application_id=$1`, applicationID)
		_, _ = db.Exec(context.Background(), `DELETE FROM workforce_webhook_deliveries WHERE integration_application_id=$1`, applicationID)
		_, _ = db.Exec(context.Background(), `DELETE FROM workforce_public_events WHERE integration_application_id=$1`, applicationID)
		_, _ = db.Exec(context.Background(), `DELETE FROM workforce_task_events WHERE task_id BETWEEN 9440300 AND 9440365`)
		_, _ = db.Exec(context.Background(), `DELETE FROM workforce_tasks WHERE id BETWEEN 9440300 AND 9440365`)
		_, _ = db.Exec(context.Background(), `DELETE FROM workforce_integration_api_credentials WHERE integration_application_id=$1`, applicationID)
		_, _ = db.Exec(context.Background(), `DELETE FROM workforce_integration_signing_credentials WHERE integration_application_id=$1`, applicationID)
		_, _ = db.Exec(context.Background(), `DELETE FROM workforce_integration_applications WHERE id=$1`, applicationID)
	}()

	const ordinaryCount = 65
	for i := 0; i < ordinaryCount; i++ {
		taskID := int64(9_440_300 + i)
		if _, err = db.Exec(ctx, `INSERT INTO workforce_tasks
(id,tenant_id,idempotency_key,request_hash,created_by,repository_url,repository_ref,task_instruction,status,submission_channel,current_task_turn_id,result_processor_release_id,project_id,project_version_id,review_enabled,execution_requirements_json,execution_requirements_digest)
VALUES ($1,$2,$3,$4,$5,'','','test','SUCCEEDED','WEB',$6,1,$1,$1,TRUE,'{}'::jsonb,$4)`, taskID, tenantID, "idem-ordinary-prefix-"+strconv.Itoa(i), strings.Repeat("a", 64), int64(101), int64(9_441_300+i)); err != nil {
			t.Fatal(err)
		}
		if _, err = db.Exec(ctx, `INSERT INTO workforce_task_events (task_id,event_type,sequence,payload_json)
VALUES ($1,'task.succeeded',1,'{}'::jsonb)`, taskID); err != nil {
			t.Fatal(err)
		}
	}
	openTaskID, openTurnID := int64(9_440_365), int64(9_441_365)
	if _, err = db.Exec(ctx, `INSERT INTO workforce_tasks
(id,tenant_id,idempotency_key,request_hash,created_by,repository_url,repository_ref,task_instruction,status,submission_channel,integration_application_id,scenario_key,scenario_version,scenario_digest,external_correlation_id,current_task_turn_id,result_processor_release_id,project_id,project_version_id,review_enabled,execution_requirements_json,execution_requirements_digest)
VALUES ($1,$2,$3,$4,$5,'','','test','SUCCEEDED','OPEN_REST',$6,'project-task',1,$7,'ordinary-prefix-open',$8,1,$1,$1,TRUE,'{}'::jsonb,$4)`, openTaskID, tenantID, "idem-ordinary-prefix-open", strings.Repeat("a", 64), int64(101), applicationID, strings.Repeat("b", 64), openTurnID); err != nil {
		t.Fatal(err)
	}
	if _, err = db.Exec(ctx, `INSERT INTO workforce_task_events (task_id,event_type,sequence,payload_json)
VALUES ($1,'task.succeeded',1,'{}'::jsonb)`, openTaskID); err != nil {
		t.Fatal(err)
	}

	projector := NewStore(db, &integrationTestIDGen{next: 9_450_000})
	for poll := 1; poll <= 2; poll++ {
		projected, projectErr := projector.ProjectTerminalEvents(ctx, 32)
		if projectErr != nil || projected != 0 {
			t.Fatalf("ordinary prefix poll %d projected=%d err=%v", poll, projected, projectErr)
		}
	}
	projected, err := projector.ProjectTerminalEvents(ctx, 32)
	if err != nil || projected != 1 {
		t.Fatalf("Open event behind ordinary prefix projected=%d err=%v", projected, err)
	}
	var skipped, pending, publicEvents int
	if err = db.QueryRow(ctx, `SELECT count(*) FILTER (WHERE e.open_integration_projection_status='SKIPPED'),
count(*) FILTER (WHERE e.open_integration_projection_status='PENDING')
FROM workforce_task_events e JOIN workforce_tasks t ON t.id=e.task_id
WHERE t.id BETWEEN 9440300 AND 9440364`).Scan(&skipped, &pending); err != nil {
		t.Fatal(err)
	}
	if err = db.QueryRow(ctx, `SELECT count(*) FROM workforce_public_events WHERE integration_application_id=$1 AND task_id=$2`, applicationID, openTaskID).Scan(&publicEvents); err != nil {
		t.Fatal(err)
	}
	if skipped != ordinaryCount || pending != 0 || publicEvents != 1 {
		t.Fatalf("ordinary prefix disposition skipped=%d pending=%d publicEvents=%d", skipped, pending, publicEvents)
	}
	if projected, err = projector.ProjectTerminalEvents(ctx, 32); err != nil || projected != 0 {
		t.Fatalf("settled prefix was reconsidered: projected=%d err=%v", projected, err)
	}
}

func TestSetApplicationStatusResumesSuspendedDeliveries(t *testing.T) {
	dsn := strings.TrimSpace(os.Getenv("OPEN_INTEGRATION_TEST_POSTGRES_DSN"))
	if dsn == "" {
		t.Skip("set OPEN_INTEGRATION_TEST_POSTGRES_DSN to an isolated migrated PostgreSQL database")
	}
	ctx := context.Background()
	db, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	tenantID := int64(93001)
	applicationID := int64(93002)
	deliveryID := int64(93003)
	if err = insertIntegrationTestApplication(ctx, db, tenantID, applicationID, "resume test", domain.ApplicationDisabled, "credential-resume-test"); err != nil {
		t.Fatal(err)
	}
	_, err = db.Exec(ctx, `INSERT INTO workforce_webhook_deliveries
(id,tenant_id,integration_application_id,public_event_id,task_id,event_type,status)
VALUES ($1,$2,$3,$4,$5,'task.succeeded','SUSPENDED')`, deliveryID, tenantID, applicationID, int64(93004), int64(93005))
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		_, _ = db.Exec(context.Background(), `DELETE FROM workforce_webhook_deliveries WHERE id=$1`, deliveryID)
		_, _ = db.Exec(context.Background(), `DELETE FROM workforce_integration_api_credentials WHERE integration_application_id=$1`, applicationID)
		_, _ = db.Exec(context.Background(), `DELETE FROM workforce_integration_signing_credentials WHERE integration_application_id=$1`, applicationID)
		_, _ = db.Exec(context.Background(), `DELETE FROM workforce_integration_applications WHERE id=$1`, applicationID)
	}()

	if err := NewStore(db, &integrationTestIDGen{}).SetApplicationStatus(ctx, tenantID, applicationID, domain.ApplicationEnabled); err != nil {
		t.Fatal(err)
	}
	var applicationStatus, deliveryStatus string
	if err := db.QueryRow(ctx, `SELECT status FROM workforce_integration_applications WHERE id=$1`, applicationID).Scan(&applicationStatus); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRow(ctx, `SELECT status FROM workforce_webhook_deliveries WHERE id=$1`, deliveryID).Scan(&deliveryStatus); err != nil {
		t.Fatal(err)
	}
	if applicationStatus != domain.ApplicationEnabled || deliveryStatus != "PENDING" {
		t.Fatalf("unexpected statuses: application=%s delivery=%s", applicationStatus, deliveryStatus)
	}
}

func TestMCPOnlyApplicationHasNoSigningCredentialOrWebhookProjection(t *testing.T) {
	dsn := strings.TrimSpace(os.Getenv("OPEN_INTEGRATION_TEST_POSTGRES_DSN"))
	if dsn == "" {
		t.Skip("set OPEN_INTEGRATION_TEST_POSTGRES_DSN to an isolated migrated PostgreSQL database")
	}
	ctx := context.Background()
	db, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	locator, _, digest, hint, err := domain.GenerateAPIKey()
	if err != nil {
		t.Fatal(err)
	}
	store := NewStore(db, newAtomicIntegrationTestIDGen(8_650_000_000_000_000_000))
	application, err := store.CreateApplication(ctx, CreateApplicationParams{
		TenantID: 94_101, Name: "mcp only", WebhookURL: "",
		CredentialID: locator, CredentialDigest: digest, CredentialHint: hint, CreatedByUserID: 101,
	})
	if err != nil {
		t.Fatal(err)
	}
	taskID, turnID := int64(94_101_001), int64(94_101_002)
	defer func() {
		_, _ = db.Exec(context.Background(), `DELETE FROM workforce_webhook_delivery_attempts WHERE integration_application_id=$1`, application.ID)
		_, _ = db.Exec(context.Background(), `DELETE FROM workforce_webhook_deliveries WHERE integration_application_id=$1`, application.ID)
		_, _ = db.Exec(context.Background(), `DELETE FROM workforce_public_events WHERE integration_application_id=$1`, application.ID)
		_, _ = db.Exec(context.Background(), `DELETE FROM workforce_task_events WHERE task_id=$1`, taskID)
		_, _ = db.Exec(context.Background(), `DELETE FROM workforce_tasks WHERE id=$1`, taskID)
		_, _ = db.Exec(context.Background(), `DELETE FROM workforce_integration_api_credentials WHERE integration_application_id=$1`, application.ID)
		_, _ = db.Exec(context.Background(), `DELETE FROM workforce_integration_signing_credentials WHERE integration_application_id=$1`, application.ID)
		_, _ = db.Exec(context.Background(), `DELETE FROM workforce_integration_applications WHERE id=$1`, application.ID)
	}()
	if application.SigningCredential != nil {
		t.Fatalf("MCP-only application received signing credential: %+v", application.SigningCredential)
	}
	var signingCount int
	if err := db.QueryRow(ctx, `SELECT count(*) FROM workforce_integration_signing_credentials WHERE integration_application_id=$1`, application.ID).Scan(&signingCount); err != nil {
		t.Fatal(err)
	}
	if signingCount != 0 {
		t.Fatalf("MCP-only signing credential count=%d", signingCount)
	}
	rotatedLocator, _, rotatedDigest, rotatedHint, err := domain.GenerateAPIKey()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.RotateAPIKey(ctx, RotateAPIKeyParams{
		TenantID: application.TenantID, ApplicationID: application.ID, ActorUserID: 101,
		CredentialLocator: rotatedLocator, CredentialDigest: rotatedDigest, CredentialHint: rotatedHint, GracePeriod: time.Minute,
	}); err != nil {
		t.Fatalf("RotateAPIKey for MCP-only application: %v", err)
	}
	if _, err := store.RotateSigningSecret(ctx, RotateSigningSecretParams{
		TenantID: application.TenantID, ApplicationID: application.ID, ActorUserID: 101,
		SigningSecretHint: "unused", GracePeriod: time.Hour,
	}, secretvalue.SecretValue{CiphertextB64: "unused", EDEKB64: "unused", WrapKeyRef: "unused"}); !errors.Is(err, ErrWebhookNotConfigured) {
		t.Fatalf("RotateSigningSecret err=%v, want ErrWebhookNotConfigured", err)
	}
	if _, err := db.Exec(ctx, `INSERT INTO workforce_tasks
(id,tenant_id,idempotency_key,request_hash,created_by,repository_url,repository_ref,task_instruction,status,submission_channel,integration_application_id,scenario_key,scenario_version,scenario_digest,external_correlation_id,current_task_turn_id,result_processor_release_id,project_id,project_version_id,review_enabled,execution_requirements_json,execution_requirements_digest)
VALUES ($1,$2,$3,$4,$5,'','','test','SUCCEEDED','OPEN_REST',$6,'project-task',1,$7,'mcp-only',$8,1,$1,$1,TRUE,'{}'::jsonb,$4)`, taskID, application.TenantID, "idem-mcp-only", strings.Repeat("a", 64), int64(101), application.ID, strings.Repeat("b", 64), turnID); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(ctx, `INSERT INTO workforce_task_events (task_id,event_type,sequence,payload_json) VALUES ($1,'task.succeeded',1,'{}'::jsonb)`, taskID); err != nil {
		t.Fatal(err)
	}
	projected, err := store.ProjectTerminalEvents(ctx, 32)
	if err != nil {
		t.Fatal(err)
	}
	var publicEvents, deliveries int
	if err := db.QueryRow(ctx, `SELECT count(*) FROM workforce_public_events WHERE integration_application_id=$1`, application.ID).Scan(&publicEvents); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRow(ctx, `SELECT count(*) FROM workforce_webhook_deliveries WHERE integration_application_id=$1`, application.ID).Scan(&deliveries); err != nil {
		t.Fatal(err)
	}
	if projected != 0 || publicEvents != 0 || deliveries != 0 {
		t.Fatalf("MCP-only projection leaked webhook records: projected=%d events=%d deliveries=%d", projected, publicEvents, deliveries)
	}
	var projectionStatus string
	if err := db.QueryRow(ctx, `SELECT open_integration_projection_status FROM workforce_task_events WHERE task_id=$1 AND event_type='task.succeeded'`, taskID).Scan(&projectionStatus); err != nil {
		t.Fatal(err)
	}
	if projectionStatus != "SKIPPED" {
		t.Fatalf("MCP-only projection status=%q, want SKIPPED", projectionStatus)
	}
	if projected, err = store.ProjectTerminalEvents(ctx, 32); err != nil || projected != 0 {
		t.Fatalf("MCP-only terminal event was reconsidered: projected=%d err=%v", projected, err)
	}
}

func insertIntegrationTestApplication(ctx context.Context, db *pgxpool.Pool, tenantID, applicationID int64, name, status, locator string) error {
	if _, err := db.Exec(ctx, `INSERT INTO workforce_integration_applications
(id,tenant_id,name,status,webhook_url,created_by)
VALUES ($1,$2,$3,$4,'https://example.com/hook',$5)`, applicationID, tenantID, name, status, int64(101)); err != nil {
		return err
	}
	if _, err := db.Exec(ctx, `INSERT INTO workforce_integration_api_credentials
(id,tenant_id,integration_application_id,credential_locator,credential_digest,credential_hint,version,status,created_by)
VALUES ($1,$2,$3,$4,'digest','hint',1,'CURRENT',$5)`, applicationID+100_000, tenantID, applicationID, locator, int64(101)); err != nil {
		return err
	}
	_, err := db.Exec(ctx, `INSERT INTO workforce_integration_signing_credentials
(id,tenant_id,integration_application_id,signing_secret_ciphertext,signing_secret_edek,signing_secret_wrap_key_ref,signing_secret_hint,version,status,created_by)
VALUES ($1,$2,$3,'ciphertext','edek','key','hint',1,'CURRENT',$4)`, applicationID+200_000, tenantID, applicationID, int64(101))
	return err
}

func TestOrphanDiagnosticsAreZero(t *testing.T) {
	dsn := strings.TrimSpace(os.Getenv("OPEN_INTEGRATION_TEST_POSTGRES_DSN"))
	if dsn == "" {
		t.Skip("set OPEN_INTEGRATION_TEST_POSTGRES_DSN to an isolated migrated PostgreSQL database")
	}
	ctx := context.Background()
	db, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	diagnostics, err := NewStore(db, &integrationTestIDGen{}).DiagnoseOrphans(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if diagnostics.Total() != 0 {
		t.Fatalf("open integration orphan diagnostics are non-zero: %+v", diagnostics)
	}
}
