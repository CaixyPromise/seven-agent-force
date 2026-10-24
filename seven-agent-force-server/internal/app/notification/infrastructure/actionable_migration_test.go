package infrastructure

import (
	"os"
	"strings"
	"testing"
)

func TestActionableMessageFreshMigrationsFreezePlatformOwnedActionState(t *testing.T) {
	files := map[string][]string{
		"../../../../migrations/mysql/20260722110000_notification_core_inbox.sql": {
			"actionType", "actionLabelSnapshot", "actionStyle",
		},
		"../../../../migrations/postgres/20260722110000_notification_core_inbox.sql": {
			"actionType", "actionLabelSnapshot", "actionStyle", "email_verified_at", "email_version", "email_verification_source",
		},
		"../../../../migrations/mysql/20260723120000_notification_external_app_delivery.sql": {
			"presentationType", "actionLabelSnapshot", "actionUrlSnapshot", "renderedPayloadDigest",
		},
		"../../../../migrations/postgres/20260723120000_notification_external_app_delivery.sql": {
			"presentationType", "actionLabelSnapshot", "actionUrlSnapshot", "renderedPayloadDigest",
		},
		"../../../../migrations/mysql/20260727150000_notification_template_revisions.sql": {
			"primaryActionLabelTemplate", "NOT NULL DEFAULT ''",
		},
		"../../../../migrations/postgres/20260727150000_notification_template_revisions.sql": {
			"primaryActionLabelTemplate", "NOT NULL DEFAULT ''",
		},
	}
	for path, required := range files {
		payload, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read migration %s: %v", path, err)
		}
		normalized := strings.Join(strings.Fields(string(payload)), " ")
		for _, token := range required {
			if !strings.Contains(normalized, token) {
				t.Fatalf("migration %s missing %q", path, token)
			}
		}
		if strings.Contains(strings.ToUpper(normalized), "FOREIGN KEY") {
			t.Fatalf("migration %s introduced a database foreign key", path)
		}
	}
}
