package infrastructure

import (
	"os"
	"strings"
	"testing"
)

func TestNotificationCoreInboxMigrationsKeepRecipientAndDeliverySeparate(t *testing.T) {
	files := []string{
		"../../../../migrations/mysql/20260722110000_notification_core_inbox.sql",
		"../../../../migrations/postgres/20260722110000_notification_core_inbox.sql",
	}
	for _, path := range files {
		payload, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read migration %s: %v", path, err)
		}
		normalized := strings.Join(strings.Fields(string(payload)), " ")
		for _, required := range []string{
			"sysNotification",
			"sysNotificationRecipient",
			"sysNotificationMaterializationTask",
			"scopeId",
			"eventKey",
			"idempotencyKey",
			"requestFingerprint",
			"mailboxVersion",
			"materializationCursor",
			"firstSeenAt",
			"readAt",
			"archivedAt",
			"leaseToken",
			"leaseUntil",
			"notificationId",
			"userId",
		} {
			if !strings.Contains(normalized, required) {
				t.Fatalf("migration %s missing %q", path, required)
			}
		}
		if strings.Contains(normalized, "creatorId BIGINT NOT NULL COMMENT '收件用户") || strings.Contains(normalized, "creatorId BIGINT NOT NULL") {
			t.Fatalf("migration %s appears to use creatorId as a required recipient owner", path)
		}
		if strings.Contains(normalized, "DROP TABLE") {
			t.Fatalf("migration %s must be forward-only for notification audit data", path)
		}
		if strings.Contains(normalized, "cursor VARCHAR") || strings.Contains(normalized, `"cursor" character varying`) {
			t.Fatalf("migration %s must not use the reserved materialization column name cursor", path)
		}
	}
}

func TestNotificationMailboxSyncMigrationsAddSerializedMailboxStateWithoutDestructiveRollback(t *testing.T) {
	files := []string{
		"../../../../migrations/mysql/20260722120000_notification_mailbox_sync.sql",
		"../../../../migrations/postgres/20260722120000_notification_mailbox_sync.sql",
	}
	for _, path := range files {
		payload, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read migration %s: %v", path, err)
		}
		normalized := strings.Join(strings.Fields(string(payload)), " ")
		for _, required := range []string{
			"sysNotificationMailbox",
			"scopeId",
			"userId",
			"mailboxKey",
			"changeSequence",
			"sysNotificationRecipient",
			"mailboxVersion",
			"ROW_NUMBER()",
		} {
			if !strings.Contains(normalized, required) {
				t.Fatalf("migration %s missing %q", path, required)
			}
		}
		if strings.Contains(normalized, "DROP TABLE") || strings.Contains(normalized, "DELETE FROM sysNotificationRecipient") {
			t.Fatalf("migration %s must preserve notification recipient history", path)
		}
	}
}

func TestNotificationInboxExpiryMigrationsKeepAuditRowsAndIndexDueWork(t *testing.T) {
	files := []string{
		"../../../../migrations/mysql/20260723110000_notification_inbox_expiry_sync.sql",
		"../../../../migrations/postgres/20260723110000_notification_inbox_expiry_sync.sql",
	}
	for _, path := range files {
		payload, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read migration %s: %v", path, err)
		}
		normalized := strings.Join(strings.Fields(string(payload)), " ")
		for _, required := range []string{
			"sysNotificationRecipient",
			"expiredAt",
			"expiresAt",
			"idxNotificationRecipientExpiry",
		} {
			if !strings.Contains(normalized, required) {
				t.Fatalf("migration %s missing %q", path, required)
			}
		}
		if strings.Contains(normalized, "DROP TABLE") || strings.Contains(normalized, "DELETE FROM sysNotificationRecipient") {
			t.Fatalf("migration %s must preserve notification recipient history", path)
		}
		assertForwardOnlyDownFails(t, path, normalized)
	}
}

func TestNotificationExternalApplicationMigrationsKeepTargetsOutsideInbox(t *testing.T) {
	files := []string{
		"../../../../migrations/mysql/20260723120000_notification_external_app_delivery.sql",
		"../../../../migrations/postgres/20260723120000_notification_external_app_delivery.sql",
	}
	for _, path := range files {
		payload, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read migration %s: %v", path, err)
		}
		normalized := strings.Join(strings.Fields(string(payload)), " ")
		for _, required := range []string{
			"sysNotificationExternalTarget",
			"sysNotificationDeliveryAttempt",
			"notificationId",
			"externalTargetId",
			"scopeId",
			"organizationId",
			"userId",
			"externalIdentityId",
			"externalIdentityVersion",
			"enterpriseConnectionId",
			"authorityDigest",
			"grantType",
			"connectionRef",
			"identityKind",
			"subjectCiphertext",
			"subjectDigest",
			"providerParamsJson",
			"providerReference",
			"failureClass",
		} {
			if !strings.Contains(normalized, required) {
				t.Fatalf("migration %s missing %q", path, required)
			}
		}
		if strings.Contains(normalized, "readAt") || strings.Contains(normalized, "archivedAt") {
			t.Fatalf("migration %s must not treat external application targets as inbox recipients", path)
		}
		if strings.Contains(normalized, "DROP TABLE") {
			t.Fatalf("migration %s must preserve external delivery audit data", path)
		}
		assertForwardOnlyDownFails(t, path, normalized)
	}
}

func TestNotificationMaterializationScopeFenceMigrationsLeadWithScope(t *testing.T) {
	files := []string{
		"../../../../migrations/mysql/20260728110000_notification_materialization_scope_fence.sql",
		"../../../../migrations/postgres/20260728110000_notification_materialization_scope_fence.sql",
	}
	for _, path := range files {
		payload, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read migration %s: %v", path, err)
		}
		normalized := strings.Join(strings.Fields(string(payload)), " ")
		if !strings.Contains(normalized, "idxNotificationMaterializationScopeReady") ||
			(!strings.Contains(normalized, "(scopeId, status, nextRunAt") &&
				!strings.Contains(normalized, `("scopeId", "status", "nextRunAt"`)) {
			t.Fatalf("migration %s must lead the ready-work index with scopeId: %s", path, normalized)
		}
	}
}

func TestNotificationForwardOnlyMigrationsRejectAutomatedDown(t *testing.T) {
	names := []string{
		"20260722110000_notification_core_inbox.sql",
		"20260722120000_notification_mailbox_sync.sql",
		"20260723110000_notification_inbox_expiry_sync.sql",
		"20260723120000_notification_external_app_delivery.sql",
		"20260727130000_notification_http_connector_delivery.sql",
		"20260727150000_notification_template_revisions.sql",
		"20260727160000_notification_scene_revisions.sql",
		"20260727170000_notification_delivery_diagnostics.sql",
	}
	for _, dialect := range []string{"mysql", "postgres"} {
		for _, name := range names {
			path := "../../../../migrations/" + dialect + "/" + name
			payload, err := os.ReadFile(path)
			if err != nil {
				t.Fatalf("read migration %s: %v", path, err)
			}
			assertForwardOnlyDownFails(t, path, strings.Join(strings.Fields(string(payload)), " "))
		}
	}
}

func assertForwardOnlyDownFails(t *testing.T, path, normalized string) {
	t.Helper()
	downIndex := strings.Index(normalized, "-- +goose Down")
	if downIndex < 0 {
		t.Fatalf("migration %s is missing a Down section", path)
	}
	down := normalized[downIndex:]
	if strings.Contains(down, "SELECT 1;") {
		t.Fatalf("migration %s must not let a no-op Down lower the Goose version", path)
	}
	if !strings.Contains(down, "SIGNAL SQLSTATE") && !strings.Contains(down, "RAISE EXCEPTION") {
		t.Fatalf("migration %s Down must fail explicitly to preserve its Goose version", path)
	}
}
