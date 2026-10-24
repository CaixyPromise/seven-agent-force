package governance

import (
	"os"
	"path/filepath"
	"regexp"
	"testing"
)

// PostgreSQL otherwise infers the zero literal in NULLIF($n,0) as int4 before
// considering the destination column. A valid Snowflake value then fails in
// the driver before the statement reaches PostgreSQL. Require every numeric
// NULLIF parameter to declare its protocol width explicitly.
func TestWorkforceNumericNullIFParametersDeclareWidth(t *testing.T) {
	root := governanceRepositoryRoot(t)
	pattern := regexp.MustCompile(`NULLIF\(\$[0-9]+,0\)`)
	err := filepath.WalkDir(filepath.Join(root, "internal", "app", "workforce"), func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() || filepath.Ext(path) != ".go" {
			return nil
		}
		body, readErr := os.ReadFile(path)
		if readErr != nil {
			return readErr
		}
		if pattern.Match(body) {
			relative, _ := filepath.Rel(root, path)
			t.Errorf("%s contains an untyped numeric NULLIF parameter; use ::BIGINT or the exact non-identity width", relative)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}
