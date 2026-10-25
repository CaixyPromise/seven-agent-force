package schemacontract

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestMigrationUsesSharedStoragePolicy(t *testing.T) {
	p := filepath.Join("..", "..", "..", "..", "migrations", "postgres", "20261025130000_workforce_storage_contract.sql")
	data, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	up := strings.Split(string(data), "-- +goose Down")[0]
	up = strings.ReplaceAll(up, "-- +goose Up\n", "")
	up = strings.ReplaceAll(up, "-- +goose StatementBegin\n", "")
	up = strings.ReplaceAll(up, "-- +goose StatementEnd\n", "")
	if strings.TrimSpace(up) != strings.TrimSpace(SQL) {
		t.Fatal("migration and legacy alignment storage policies diverge")
	}
}
