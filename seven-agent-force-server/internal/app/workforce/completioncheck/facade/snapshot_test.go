package facade

import (
	"strings"
	"testing"
)

func TestFrozenDigestBindsReferenceBytes(t *testing.T) {
	input := Input{Mode: "COMMAND", Command: "true", ReferenceAssetIDs: []int64{1}}
	refs := []Reference{{ReferenceAssetID: 1, Name: "spec", ContentSHA256: strings.Repeat("a", 64), ObjectSHA256: strings.Repeat("b", 64), SizeBytes: 12}}
	original, err := FrozenDigest(input, refs)
	if err != nil {
		t.Fatal(err)
	}
	definition, _ := Digest(input)
	if original == definition {
		t.Fatal("references omitted from frozen identity")
	}
	refs[0].ContentSHA256 = strings.Repeat("c", 64)
	changed, _ := FrozenDigest(input, refs)
	if changed == original {
		t.Fatal("content drift accepted")
	}
	if _, err = FrozenDigest(input, nil); err == nil {
		t.Fatal("missing reference accepted")
	}
}
