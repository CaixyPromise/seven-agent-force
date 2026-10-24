package governance

import (
	"encoding/csv"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"
)

const dg1RegistryCutoff int64 = dg0LegacyMigrationCutoff

var (
	lowerSnakeTablePattern = regexp.MustCompile(`^[a-z][a-z0-9_]*$`)
	createTablePattern     = regexp.MustCompile(`(?im)CREATE\s+TABLE(?:\s+IF\s+NOT\s+EXISTS)?\s+(?:"public"\.)?"?([A-Za-z_][A-Za-z0-9_]*)"?`)
)

func TestDG1TableRegistryCoversCheckpointSchema(t *testing.T) {
	root, err := filepath.Abs(filepath.Join("..", "..", "..", ".."))
	if err != nil {
		t.Fatalf("resolve repository root: %v", err)
	}
	file, err := os.Open(filepath.Join(root, "migrations", "governance", "table_registry.csv"))
	if err != nil {
		t.Fatalf("open DG1 table registry: %v", err)
	}
	defer file.Close()

	rows, err := csv.NewReader(file).ReadAll()
	if err != nil {
		t.Fatalf("read DG1 table registry: %v", err)
	}
	if len(rows) < 2 {
		t.Fatal("DG1 table registry is empty")
	}
	wantHeader := []string{"current_table", "target_table", "domain_batch", "rename", "verify", "release", "recovery"}
	if strings.Join(rows[0], ",") != strings.Join(wantHeader, ",") {
		t.Fatalf("DG1 table registry header=%v, want=%v", rows[0], wantHeader)
	}

	registry := make(map[string][]string, len(rows)-1)
	targetOwners := make(map[string]string, len(rows)-1)
	for _, row := range rows[1:] {
		if len(row) != len(wantHeader) {
			t.Fatalf("invalid registry row: %v", row)
		}
		current, target := row[0], row[1]
		if _, exists := registry[current]; exists {
			t.Fatalf("duplicate table registry entry %q", current)
		}
		if owner, exists := targetOwners[target]; exists && owner != current {
			t.Fatalf("target table %q is claimed by both %q and %q", target, owner, current)
		}
		if !lowerSnakeTablePattern.MatchString(target) {
			t.Fatalf("target table %q is not lower snake_case", target)
		}
		if row[2] == "" || row[6] != "resume_forward" {
			t.Fatalf("table %q lacks batch/recovery contract: %v", current, row)
		}
		if current == target {
			if row[3] != "no_op" || row[5] != "no_op" {
				t.Fatalf("unchanged table %q has mutation stages: %v", current, row)
			}
		} else if !validRenameLifecycle(row[3], row[4], row[5]) {
			t.Fatalf("rename table %q has an invalid direct-migration lifecycle: %v", current, row)
		}
		registry[current] = row
		targetOwners[target] = current
	}

	schemaTables := collectCheckpointPostgresTables(t, root)
	if len(registry) != len(schemaTables) {
		t.Fatalf("registry tables=%d, checkpoint schema tables=%d", len(registry), len(schemaTables))
	}
	for table := range schemaTables {
		if _, ok := registry[table]; !ok {
			t.Errorf("checkpoint schema table %q is missing from registry", table)
		}
	}
	for table := range registry {
		if _, ok := schemaTables[table]; !ok {
			t.Errorf("registry table %q is absent from checkpoint schema", table)
		}
	}
}

func validRenameLifecycle(rename, verify, release string) bool {
	return (rename == "planned" && verify == "blocked" && release == "blocked") ||
		(rename == "renamed" && verify == "passed" && release == "complete")
}

func collectCheckpointPostgresTables(t *testing.T, root string) map[string]struct{} {
	t.Helper()
	paths := []string{
		filepath.Join(root, "migrations", "postgres-baseline", "20260719110000_clean_install_baseline.sql"),
	}
	entries, err := os.ReadDir(filepath.Join(root, "migrations", "postgres"))
	if err != nil {
		t.Fatalf("read PostgreSQL migration directory: %v", err)
	}
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".sql" {
			continue
		}
		versionText, _, ok := strings.Cut(entry.Name(), "_")
		if !ok {
			t.Fatalf("invalid migration filename %q", entry.Name())
		}
		version, err := strconv.ParseInt(versionText, 10, 64)
		if err != nil {
			t.Fatalf("parse migration version %q: %v", entry.Name(), err)
		}
		if version > 20260719110000 && version <= dg1RegistryCutoff {
			paths = append(paths, filepath.Join(root, "migrations", "postgres", entry.Name()))
		}
	}

	result := map[string]struct{}{}
	for _, path := range paths {
		payload, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read migration %s: %v", path, err)
		}
		for _, match := range createTablePattern.FindAllStringSubmatch(string(payload), -1) {
			result[match[1]] = struct{}{}
		}
	}
	return result
}
