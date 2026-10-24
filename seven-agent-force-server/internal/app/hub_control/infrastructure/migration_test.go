package infrastructure

import (
	"os"
	"regexp"
	"strings"
	"testing"
)

func TestFederatedHubNodeMigrationContract(t *testing.T) {
	payload, err := os.ReadFile("../../../../migrations/mysql/20260712120000_federated_hub_node_v1.sql")
	if err != nil {
		t.Fatalf("read migration: %v", err)
	}
	normalized := strings.Join(strings.Fields(string(payload)), " ")
	patterns := []string{
		`DROP PROCEDURE IF EXISTS task7FederatedIdentityPreflight.*CREATE PROCEDURE task7FederatedIdentityPreflight`,
		`SIGNAL SQLSTATE '45000'.*unsafe legacy OIDC issuer`,
		`SIGNAL SQLSTATE '45000'.*duplicate legacy OIDC issuer and subject`,
		`CALL task7FederatedIdentityPreflight\(\)`,
		`information_schema.COLUMNS`,
		`ALTER TABLE sysExternalUserIdentity ADD COLUMN externalIssuer VARCHAR\(512\) NULL`,
		`UPDATE sysExternalUserIdentity i JOIN sysExternalLoginProvider p`,
		`p.protocolType = 'OIDC'`,
		`UNIQUE KEY uk_sysExternalUserIdentity_issuer_subject_deleted \(externalIssuerDigest, externalSubject, isDeleted\)`,
		`CREATE TABLE IF NOT EXISTS sysExternalManagedProviderCommand`,
		`PRIMARY KEY \(providerCode, connectionVersion\)`,
		`-- \+goose Up .*CREATE TABLE IF NOT EXISTS sysFederatedNode`,
		`UNIQUE KEY uk_sysFederatedNode_nodeCode_active \(nodeCode, activeKey\)`,
		`oidcClientSecretCiphertext TEXT NULL`, `oidcClientSecretEdek TEXT NULL`, `oidcClientSecretWrapKeyRef VARCHAR\(128\) NULL`,
		`managementBearerCiphertext TEXT NULL`, `managementBearerEdek TEXT NULL`, `managementBearerWrapKeyRef VARCHAR\(128\) NULL`,
		`issuerLockedAt DATETIME\(6\) NULL`,
		`targetRevision BIGINT NOT NULL DEFAULT 1`,
		`CREATE TABLE IF NOT EXISTS sysFederatedNodeConnectionCommand`,
		`PRIMARY KEY \(nodeCode, connectionVersion\)`,
		`requestHash CHAR\(64\) NOT NULL`,
		`targetRevision BIGINT NOT NULL`,
		`terminalState VARCHAR\(16\) NOT NULL`,
		`ON DUPLICATE KEY UPDATE updatedAt = sysFederatedNodeConnectionCommand.updatedAt`,
		`-- \+goose Down .*DROP TABLE IF EXISTS sysFederatedNodeConnectionCommand; DROP TABLE IF EXISTS sysFederatedNode`,
		`DROP TABLE IF EXISTS sysExternalManagedProviderCommand`,
		`information_schema.STATISTICS`,
	}
	exactPayload, err := os.ReadFile("../../../../migrations/mysql/20260712130000_external_identity_exact_subject.sql")
	if err != nil {
		t.Fatalf("read exact identity migration: %v", err)
	}
	exact := strings.Join(strings.Fields(string(exactPayload)), " ")
	for _, pattern := range []string{
		`providerSubjectDigest BINARY\(32\) GENERATED ALWAYS AS`,
		`externalIdentityDigest BINARY\(32\) GENERATED ALWAYS AS`,
		`DROP INDEX uk_sysExternalUserIdentity_subject_deleted`,
		`UNIQUE KEY uk_sysExternalUserIdentity_provider_subject_digest_deleted \(providerSubjectDigest, isDeleted\)`,
		`UNIQUE KEY uk_sysExternalUserIdentity_issuer_subject_deleted \(externalIdentityDigest, isDeleted\)`,
		`providerConfigDigest CHAR\(64\) NULL`,
	} {
		if !regexp.MustCompile(pattern).MatchString(exact) {
			t.Fatalf("missing exact identity migration contract %q", pattern)
		}
	}
	for _, pattern := range patterns {
		if !regexp.MustCompile(pattern).MatchString(normalized) {
			t.Fatalf("missing migration contract %q\n%s", pattern, normalized)
		}
	}
	if strings.Contains(normalized, "UNIQUE KEY uk_sysFederatedNode_hubIssuer") {
		t.Fatal("hubIssuer must not be unique")
	}
	preflight := strings.Index(normalized, "CALL task7FederatedIdentityPreflight()")
	firstTargetDDL := strings.Index(normalized, "ALTER TABLE sysExternalUserIdentity ADD COLUMN externalIssuer")
	if preflight < 0 || firstTargetDDL < 0 || preflight > firstTargetDDL {
		t.Fatal("legacy identity preflight must run before the first target-table DDL")
	}
	externalBase, err := os.ReadFile("../../../../migrations/mysql/20260621100000_external_oauth_consumer_v1.sql")
	if err != nil {
		t.Fatalf("read external identity base migration: %v", err)
	}
	if !strings.Contains(strings.Join(strings.Fields(string(externalBase)), " "), "uk_sysExternalUserIdentity_subject_deleted (providerCode, externalSubject, isDeleted)") {
		t.Fatal("Task 7 migration must preserve ordinary provider+subject uniqueness from the existing schema")
	}
	if strings.Contains(normalized, "remoteUser") || strings.Contains(normalized, "remoteSession") {
		t.Fatal("remote identity snapshots must not be persisted")
	}
}
