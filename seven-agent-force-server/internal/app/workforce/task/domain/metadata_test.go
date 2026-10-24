package domain

import (
	"errors"
	"strings"
	"testing"
)

func TestNormalizeTaskMetadataSuggestsStableNameAndNormalizesTags(t *testing.T) {
	name, tags, err := NormalizeTaskMetadata("", "\n  补充   用户查询接口  \n第二行", []string{" Backend ", "ＣＵＳＴＯＭＥＲ-api"})
	if err != nil {
		t.Fatal(err)
	}
	if name != "补充 用户查询接口" {
		t.Fatalf("unexpected name %q", name)
	}
	if len(tags) != 2 || tags[0] != "backend" || tags[1] != "customer-api" {
		t.Fatalf("unexpected tags %#v", tags)
	}
}

func TestNormalizeTaskMetadataRejectsUnsafeOrDuplicateValues(t *testing.T) {
	if _, _, err := NormalizeTaskMetadata("bad\nname", "instruction", nil); !errors.Is(err, ErrTaskNameInvalid) {
		t.Fatalf("expected name error, got %v", err)
	}
	if _, _, err := NormalizeTaskMetadata("task", "instruction", []string{"API", "api"}); !errors.Is(err, ErrTaskTagInvalid) {
		t.Fatalf("expected duplicate tag error, got %v", err)
	}
	if _, _, err := NormalizeTaskMetadata("task", "instruction", strings.Split("a b c d e f g h i j k", " ")); !errors.Is(err, ErrTaskTagLimitExceeded) {
		t.Fatalf("expected tag limit error, got %v", err)
	}
}
