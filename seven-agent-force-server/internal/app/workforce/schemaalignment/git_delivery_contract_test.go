package schemaalignment

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestBatch5GitDeliveryMigrationContract(t *testing.T) {
	path := filepath.Join("..", "..", "..", "..", "migrations", "postgres", "20260825130000_workforce_worker_fleet_mvp.sql")
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	parts := strings.Split(strings.ToUpper(string(body)), "-- +GOOSE DOWN")
	if len(parts) != 2 {
		t.Fatal("expected one Goose Down boundary")
	}
	up, down := parts[0], parts[1]
	for _, token := range []string{
		"CREATE TABLE WORKFORCE_DELIVERY_INTENTS",
		"CREATE TABLE WORKFORCE_DELIVERY_DECISIONS",
		"VERSION INTEGER NOT NULL DEFAULT 1 CONSTRAINT WORKFORCE_DELIVERY_REQUESTS_VERSION_CK",
		"WORKFORCE_TASK_GIT_PLANS_DELIVERY_AUTHORIZATION_CK",
		"WORKFORCE_DELIVERY_INTENTS_SUMMARY_CK",
		"OCTET_LENGTH(SUMMARY) BETWEEN 1 AND 2048",
		"WORKFORCE_DELIVERY_INTENTS_DIGEST_CK",
		"WORKFORCE_DELIVERY_DECISIONS_ACTION_CK",
		"WORKFORCE_DELIVERY_REQUESTS_ID_VERSION_UIDX",
	} {
		if !strings.Contains(up, token) {
			t.Errorf("Batch 5 migration Up is missing %q", token)
		}
	}
	for _, table := range []string{"WORKFORCE_DELIVERY_DECISIONS", "WORKFORCE_DELIVERY_INTENTS"} {
		if !strings.Contains(down, "DROP TABLE IF EXISTS "+table) {
			t.Errorf("Batch 5 migration Down is missing %s", table)
		}
	}
	blockStart := strings.Index(up, "CREATE TABLE WORKFORCE_DELIVERY_INTENTS")
	if blockStart < 0 {
		t.Fatal("delivery intent migration block is missing")
	}
	blockEnd := strings.Index(up[blockStart:], "CREATE TABLE WORKFORCE_DELIVERY_ATTEMPTS")
	if blockEnd < 0 {
		t.Fatal("delivery intent migration block has no delivery-attempt boundary")
	}
	deliveryBlock := up[blockStart : blockStart+blockEnd]
	if strings.Contains(deliveryBlock, "FOREIGN KEY") || strings.Contains(deliveryBlock, "REFERENCES ") {
		t.Fatal("Batch 5 Git delivery schema must not add database foreign keys")
	}
}

func TestBatch5GitDeliveryAlignmentAndTextIdentityGovernanceAreExplicit(t *testing.T) {
	body, err := os.ReadFile("git_delivery.go")
	if err != nil {
		t.Fatal(err)
	}
	text := strings.ToUpper(string(body))
	for _, token := range []string{
		"ADD COLUMN IF NOT EXISTS DELIVERY_AUTHORIZATION VARCHAR(32)",
		"ADD COLUMN IF NOT EXISTS VERSION INTEGER",
		"CREATE TABLE IF NOT EXISTS WORKFORCE_DELIVERY_INTENTS",
		"CREATE TABLE IF NOT EXISTS WORKFORCE_DELIVERY_DECISIONS",
		"WORKFORCE_DELIVERY_REQUESTS_ID_VERSION_UIDX",
		"WORKFORCE_DELIVERY_INTENTS.NATIVE_TOOL_CALL_ID",
		"ADAPTER-NATIVE EXTERNAL GIT DELIVERY TOOL-CALL IDENTITY",
	} {
		if !strings.Contains(text, token) {
			t.Errorf("Batch 5 alignment is missing %q", token)
		}
	}
	if _, ok := unpublishedSchemaTextIdentityExceptions["workforce_delivery_intents.native_tool_call_id"]; !ok {
		t.Fatal("Adapter-native Delivery Intent identity is not allowlisted")
	}
	for _, column := range []string{
		"workforce_execution_operation_attempts.worker_instance_id",
		"workforce_execution_operation_attempts.host_boot_id",
	} {
		if _, ok := unpublishedSchemaTextIdentityExceptions[column]; !ok {
			t.Errorf("unpublished Worker runtime identity %s is not allowlisted", column)
		}
	}
	if len(unpublishedSchemaTextIdentityExceptions) != 3 {
		t.Fatalf("unexpected unpublished TEXT identity exception count: %d", len(unpublishedSchemaTextIdentityExceptions))
	}
}
