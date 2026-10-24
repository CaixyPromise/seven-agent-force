package datasource

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestPostgresCleanInstallBaselineIsForwardOnlyAndSelfContained(t *testing.T) {
	t.Parallel()

	path := filepath.Join("..", "..", "..", "migrations", "postgres-baseline", "20260719110000_clean_install_baseline.sql")
	payload, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read PostgreSQL clean-install baseline: %v", err)
	}
	sql := string(payload)

	for _, required := range []string{
		"-- +goose Up",
		"-- +goose Down",
		`CREATE TABLE "public"."sys_role"`,
		`"systemKey" character varying`,
		`CREATE TABLE "public"."sys_dict_type"`,
		`CREATE TABLE "public"."sys_dict_item"`,
		"AUTHORIZATION_ROOT",
		"system:user:access:query",
		"system:user:access:explain",
		`INSERT INTO "public"."goose_db_version"`,
		"(20260718150000, true, CURRENT_TIMESTAMP)",
		"RAISE EXCEPTION 'forward-only PostgreSQL clean-install baseline",
	} {
		if !strings.Contains(sql, required) {
			t.Fatalf("PostgreSQL clean-install baseline is missing %q", required)
		}
	}

	for _, forbidden := range []string{
		"seven_batch_b4_bridge",
		`CREATE TABLE "public"."goose_db_version"`,
		`CREATE SEQUENCE "public"."goose_db_version_id_seq"`,
		"DROP TABLE",
		"DROP SCHEMA",
		"-- +goose Down\nSELECT 1;",
		"\\restrict",
		"\\unrestrict",
		"COPY ",
		"set_config('search_path'",
	} {
		if strings.Contains(sql, forbidden) {
			t.Fatalf("PostgreSQL clean-install baseline contains forbidden token %q", forbidden)
		}
	}

	markerPath := filepath.Join("..", "..", "..", "migrations", "postgres", "20260719110000_clean_install_baseline_marker.sql")
	marker, err := os.ReadFile(markerPath)
	if err != nil {
		t.Fatalf("read PostgreSQL clean-install baseline marker: %v", err)
	}
	if strings.Contains(string(marker), "CREATE TABLE") || strings.Contains(string(marker), "ALTER TABLE") {
		t.Fatal("PostgreSQL clean-install baseline marker must remain schema-neutral")
	}
}

func TestPostgresRBACAdminSeedKeepsAnonymousBlocksAtomic(t *testing.T) {
	t.Parallel()

	path := filepath.Join("..", "..", "..", "migrations", "postgres", "20260528100000_rbac_admin_seed.sql")
	payload, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read PostgreSQL RBAC admin seed migration: %v", err)
	}
	sql := string(payload)

	for _, required := range []string{
		"-- +goose Up\n-- +goose StatementBegin\nDO $rbac$",
		"$rbac$;\n-- +goose StatementEnd\n\n-- +goose Down",
		"-- +goose Down\n-- +goose StatementBegin\nDO $rbac$",
		"$rbac$;\n-- +goose StatementEnd",
	} {
		if !strings.Contains(sql, required) {
			t.Fatalf("PostgreSQL RBAC admin seed migration is missing atomic Goose boundary %q", required)
		}
	}
	if got := strings.Count(sql, "-- +goose StatementBegin"); got != 2 {
		t.Fatalf("expected two Goose statement starts, got %d", got)
	}
	if got := strings.Count(sql, "-- +goose StatementEnd"); got != 2 {
		t.Fatalf("expected two Goose statement ends, got %d", got)
	}
}

func TestPostgresProceduralBodiesDeclareGooseStatementBoundaries(t *testing.T) {
	t.Parallel()

	migrationsDir := filepath.Join("..", "..", "..", "migrations", "postgres")
	entries, err := os.ReadDir(migrationsDir)
	if err != nil {
		t.Fatalf("read PostgreSQL migrations directory: %v", err)
	}
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".sql" {
			continue
		}
		payload, err := os.ReadFile(filepath.Join(migrationsDir, entry.Name()))
		if err != nil {
			t.Fatalf("read PostgreSQL migration %s: %v", entry.Name(), err)
		}
		sql := string(payload)
		anonymousBlocks := 0
		functionBodies := 0
		for _, line := range strings.Split(sql, "\n") {
			trimmed := strings.TrimSpace(line)
			if strings.HasPrefix(trimmed, "DO $") {
				anonymousBlocks++
			}
			// Goose must receive each function body as one statement as well:
			// function bodies can contain semicolons that cannot be split by the
			// ordinary SQL statement parser.
			upper := strings.ToUpper(trimmed)
			if strings.HasPrefix(upper, "CREATE FUNCTION ") || strings.HasPrefix(upper, "CREATE OR REPLACE FUNCTION ") {
				functionBodies++
			}
		}
		expectedBoundaries := anonymousBlocks + functionBodies
		if expectedBoundaries == 0 {
			continue
		}
		statementStarts := strings.Count(sql, "-- +goose StatementBegin")
		statementEnds := strings.Count(sql, "-- +goose StatementEnd")
		if statementStarts != expectedBoundaries || statementEnds != expectedBoundaries {
			t.Errorf(
				"%s has %d anonymous blocks and %d function bodies but %d StatementBegin and %d StatementEnd annotations",
				entry.Name(),
				anonymousBlocks,
				functionBodies,
				statementStarts,
				statementEnds,
			)
		}
	}
}
