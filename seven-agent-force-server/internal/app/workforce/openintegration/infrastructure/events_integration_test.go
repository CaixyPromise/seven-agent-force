package infrastructure

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"

	secretvalue "github.com/CaixyPromise/seven-agent/force/internal/infrastructure/security/crypto/secretvalue"
	"github.com/jackc/pgx/v5/pgxpool"
)

func TestClaimAndFinishDeliveryUsesCurrentAndValidGraceSigningCredentials(t *testing.T) {
	dsn := strings.TrimSpace(os.Getenv("OPEN_INTEGRATION_TEST_POSTGRES_DSN"))
	if dsn == "" {
		t.Skip("set OPEN_INTEGRATION_TEST_POSTGRES_DSN to an isolated migrated PostgreSQL database")
	}
	ctx := context.Background()
	config, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		t.Fatal(err)
	}
	config.MaxConns = 1
	db, err := pgxpool.NewWithConfig(ctx, config)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	rotationDB, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer rotationDB.Close()

	const (
		tenantID       int64 = 8_620_000_000_000_000_001
		applicationID  int64 = 8_620_000_000_000_000_002
		currentID      int64 = 8_620_000_000_000_000_003
		graceID        int64 = 8_620_000_000_000_000_004
		expiredGraceID int64 = 8_620_000_000_000_000_005
		eventID        int64 = 8_620_000_000_000_000_006
		deliveryID     int64 = 8_620_000_000_000_000_007
	)
	cleanup := func() {
		_, _ = db.Exec(context.Background(), `DELETE FROM workforce_webhook_delivery_attempts WHERE delivery_id=$1`, deliveryID)
		_, _ = db.Exec(context.Background(), `DELETE FROM workforce_webhook_deliveries WHERE id=$1`, deliveryID)
		_, _ = db.Exec(context.Background(), `DELETE FROM workforce_public_events WHERE id=$1`, eventID)
		_, _ = db.Exec(context.Background(), `DELETE FROM workforce_integration_signing_credentials WHERE integration_application_id=$1`, applicationID)
		_, _ = db.Exec(context.Background(), `DELETE FROM workforce_integration_applications WHERE id=$1`, applicationID)
	}
	cleanup()
	defer cleanup()

	if _, err := db.Exec(ctx, `INSERT INTO workforce_integration_applications
(id,tenant_id,name,status,webhook_url,created_by)
VALUES ($1,$2,'webhook signing claim','ENABLED','https://example.com/seven-webhook',$3)`, applicationID, tenantID, int64(101)); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(ctx, `INSERT INTO workforce_integration_signing_credentials
(id,tenant_id,integration_application_id,signing_secret_ciphertext,signing_secret_edek,signing_secret_wrap_key_ref,signing_secret_hint,version,status,valid_until,created_by)
VALUES
($1,$4,$5,'current-cipher','current-edek','current-key','current-hint',3,'CURRENT',NULL,$6),
($2,$4,$5,'grace-cipher','grace-edek','grace-key','grace-hint',2,'GRACE',CURRENT_TIMESTAMP+INTERVAL '1 hour',$6),
($3,$4,$5,'expired-cipher','expired-edek','expired-key','expired-hint',1,'GRACE',CURRENT_TIMESTAMP-INTERVAL '1 second',$6)`, currentID, graceID, expiredGraceID, tenantID, applicationID, int64(101)); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(ctx, `INSERT INTO workforce_public_events
(id,tenant_id,integration_application_id,source_task_event_id,task_id,task_turn_id,event_type,sequence,correlation_id,payload_json,occurred_at)
VALUES ($1,$2,$3,$4,$5,$6,'task.succeeded',1,'claim-test','{}'::jsonb,CURRENT_TIMESTAMP)`, eventID, tenantID, applicationID, int64(8_620_000_000_000_000_008), int64(8_620_000_000_000_000_009), int64(8_620_000_000_000_000_010)); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(ctx, `INSERT INTO workforce_webhook_deliveries
(id,tenant_id,integration_application_id,public_event_id,task_id,event_type,status,next_attempt_at)
VALUES ($1,$2,$3,$4,$5,'task.succeeded','PENDING',CURRENT_TIMESTAMP)`, deliveryID, tenantID, applicationID, eventID, int64(8_620_000_000_000_000_009)); err != nil {
		t.Fatal(err)
	}

	store := NewStore(db, &integrationTestIDGen{next: 8_620_000_000_000_000_100})
	claims, err := store.ClaimDeliveries(ctx, "credential-claim-test", 1, time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	if len(claims) != 1 {
		t.Fatalf("claim count=%d", len(claims))
	}
	claim := claims[0]
	if claim.CurrentSigningCredential.ID != currentID || claim.Delivery.SigningCredentialID != currentID {
		t.Fatalf("current signing credential=%+v delivery=%+v", claim.CurrentSigningCredential, claim.Delivery)
	}
	if claim.GraceSigningCredential == nil || claim.GraceSigningCredential.ID != graceID || claim.Delivery.GraceSigningCredentialID == nil || *claim.Delivery.GraceSigningCredentialID != graceID {
		t.Fatalf("grace signing credential=%+v delivery=%+v", claim.GraceSigningCredential, claim.Delivery)
	}
	credentialLease, err := store.AcquireSigningCredentialLease(ctx, tenantID, applicationID)
	if err != nil {
		t.Fatalf("acquire signing credential lease for delivery: %v", err)
	}
	defer credentialLease.Release()
	claim.CurrentSigningCredential = credentialLease.Current
	claim.GraceSigningCredential = credentialLease.Grace
	claim.Delivery.SigningCredentialID = credentialLease.Current.ID
	if credentialLease.Grace != nil {
		graceCredentialID := credentialLease.Grace.ID
		claim.Delivery.GraceSigningCredentialID = &graceCredentialID
	}
	if err := store.RenewDeliveryLease(ctx, credentialLease, claim.Delivery, "credential-claim-test", time.Minute); err != nil {
		t.Fatalf("renew delivery lease on held connection: %v", err)
	}
	if err := store.FinishDeliveryWithLease(ctx, credentialLease, claim.Delivery, "credential-claim-test", DeliveryOutcome{Status: "DELIVERED", Duration: time.Millisecond, SigningCredentialsUsed: true}); err != nil {
		t.Fatal(err)
	}
	if err := credentialLease.Release(); err != nil {
		t.Fatalf("release delivery signing credential lease: %v", err)
	}

	var attemptCurrent int64
	var attemptGrace *int64
	if err := db.QueryRow(ctx, `SELECT signing_credential_id,grace_signing_credential_id
FROM workforce_webhook_delivery_attempts WHERE delivery_id=$1`, deliveryID).Scan(&attemptCurrent, &attemptGrace); err != nil {
		t.Fatal(err)
	}
	if attemptCurrent != currentID || attemptGrace == nil || *attemptGrace != graceID {
		t.Fatalf("attempt signing IDs current=%d grace=%v", attemptCurrent, attemptGrace)
	}
	var usedCount int
	if err := db.QueryRow(ctx, `SELECT COUNT(*) FROM workforce_integration_signing_credentials
WHERE id=ANY($1::bigint[]) AND last_used_at IS NOT NULL`, []int64{currentID, graceID}).Scan(&usedCount); err != nil {
		t.Fatal(err)
	}
	if usedCount != 2 {
		t.Fatalf("used signing credential count=%d", usedCount)
	}
	var expiredUsed bool
	if err := db.QueryRow(ctx, `SELECT last_used_at IS NOT NULL FROM workforce_integration_signing_credentials WHERE id=$1`, expiredGraceID).Scan(&expiredUsed); err != nil {
		t.Fatal(err)
	}
	if expiredUsed {
		t.Fatal("expired grace signing credential was claimed or marked used")
	}

	lease, err := store.AcquireSigningCredentialLease(ctx, tenantID, applicationID)
	if err != nil {
		t.Fatalf("acquire signing credential lease: %v", err)
	}
	defer lease.Release()
	rotationDone := make(chan error, 1)
	go func() {
		rotationStore := NewStore(rotationDB, &integrationTestIDGen{next: 8_620_000_000_000_000_200})
		_, rotateErr := rotationStore.RotateSigningSecret(ctx, RotateSigningSecretParams{
			TenantID: tenantID, ApplicationID: applicationID, ActorUserID: 101,
			SigningSecretHint: "rotated-hint", GracePeriod: 24 * time.Hour,
		}, secretvalue.SecretValue{CiphertextB64: "cm90YXRlZC1jaXBoZXI=", EDEKB64: "cm90YXRlZC1lZGVr", WrapKeyRef: "rotated-key"})
		rotationDone <- rotateErr
	}()
	select {
	case rotateErr := <-rotationDone:
		t.Fatalf("Signing Secret rotation crossed an active delivery lease: %v", rotateErr)
	case <-time.After(150 * time.Millisecond):
	}
	if err := lease.Release(); err != nil {
		t.Fatalf("release signing credential lease: %v", err)
	}
	select {
	case rotateErr := <-rotationDone:
		if rotateErr != nil {
			t.Fatalf("Signing Secret rotation after delivery lease: %v", rotateErr)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Signing Secret rotation did not resume after delivery lease release")
	}
}
