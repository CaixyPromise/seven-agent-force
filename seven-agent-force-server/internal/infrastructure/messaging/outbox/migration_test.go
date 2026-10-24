package outbox

import (
	"os"
	"strings"
	"testing"
)

func TestReliabilityMigrationPersistsOwnerAndFencingFields(t *testing.T) {
	for _, path := range []string{
		"../../../../migrations/mysql/20260722100000_notification_reliability_foundation.sql",
		"../../../../migrations/postgres/20260722100000_notification_reliability_foundation.sql",
	} {
		payload, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read migration %s: %v", path, err)
		}
		text := string(payload)
		for _, required := range []string{
			"eventOwner",
			"leaseOwner",
			"leaseToken",
			"leaseUntil",
			"idx_outbox_owner_type_status_retry",
			"idx_message_consume_status_lease",
			"unassigned",
		} {
			if !strings.Contains(text, required) {
				t.Fatalf("migration %s missing %q", path, required)
			}
		}
	}
}
