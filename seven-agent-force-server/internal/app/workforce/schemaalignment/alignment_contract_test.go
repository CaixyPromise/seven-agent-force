package schemaalignment

import (
	"os"
	"strings"
	"testing"

	"github.com/CaixyPromise/seven-agent/force/internal/app/workforce/shared/identitycontract"
)

func TestEnterpriseNotificationChannelAlignmentUsesCanonicalDigestColumn(t *testing.T) {
	source, err := os.ReadFile("alignment.go")
	if err != nil {
		t.Fatal(err)
	}
	text := string(source)
	if !strings.Contains(text, `RENAME COLUMN "enterpriseConnectionConfigDigest" TO "enterpriseConnectionDigest"`) {
		t.Fatal("alignment does not migrate the legacy enterprise connection digest column")
	}
	if !strings.Contains(text, `ADD COLUMN IF NOT EXISTS "enterpriseConnectionDigest" CHAR(64)`) {
		t.Fatal("alignment does not create the canonical enterprise connection digest column")
	}
}

func TestIntegrationApplicationCreatorRemainsFrameworkUserIdentity(t *testing.T) {
	if mappings, exists := principalColumns["workforce_integration_applications"]; exists {
		t.Fatalf("Integration Application created_by is a Framework user BIGINT and must not be principal-aligned: %#v", mappings)
	}
}

func TestOpenIntegrationTextIdentityExceptionsAreRecognizedByAlignment(t *testing.T) {
	for _, column := range []string{
		"workforce_integration_api_credentials.credential_locator",
		"workforce_public_events.correlation_id",
		"workforce_tasks.external_correlation_id",
		"workforce_webhook_deliveries.lease_owner",
	} {
		if !identitycontract.IsTextIdentityException(column) {
			t.Errorf("Schema Alignment does not recognize declared TEXT identity exception %s", column)
		}
	}
}

func TestIndependentReviewContainerTextIdentityExceptionsAreGoverned(t *testing.T) {
	for _, column := range []string{
		"workforce_review_runs.runtime_container_id",
		"workforce_review_run_attempts.container_id",
	} {
		if !identitycontract.IsTextIdentityException(column) {
			t.Fatalf("Slice 4 opaque Docker identity %s is not governed", column)
		}
	}
}

func TestOpenIntegrationCredentialTablesUseBigintRelationships(t *testing.T) {
	for _, table := range []string{"workforce_integration_api_credentials", "workforce_integration_signing_credentials"} {
		if _, legacy := principalColumns[table]; legacy {
			t.Fatalf("%s created_by must remain a Framework user BIGINT", table)
		}
	}
}
