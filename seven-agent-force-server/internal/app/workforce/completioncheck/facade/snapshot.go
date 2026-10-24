package facade

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"regexp"
)

type Reference struct {
	ReferenceAssetID int64  `json:"referenceAssetId,string"`
	Name             string `json:"name"`
	ContentSHA256    string `json:"contentSha256"`
	ObjectSHA256     string `json:"objectSha256"`
	SizeBytes        int64  `json:"sizeBytes,string"`
}

// FrozenDigest binds the definition to the exact selected read-only bytes.
// Input Digest remains independently verifiable by the command runner.
func FrozenDigest(input Input, references []Reference) (string, error) {
	normalized, err := Normalize(input)
	if err != nil {
		return "", err
	}
	definition, err := Digest(normalized)
	if err != nil {
		return "", err
	}
	if len(normalized.ReferenceAssetIDs) != len(references) {
		return "", ErrInvalid
	}
	if len(references) == 0 {
		return definition, nil
	}
	ordered := make([]Reference, 0, len(references))
	hex64 := regexp.MustCompile(`^[0-9a-f]{64}$`)
	for _, id := range normalized.ReferenceAssetIDs {
		found := false
		for _, ref := range references {
			if ref.ReferenceAssetID == id {
				if found || !hex64.MatchString(ref.ContentSHA256) || !hex64.MatchString(ref.ObjectSHA256) || ref.SizeBytes < 0 {
					return "", ErrInvalid
				}
				found = true
				ordered = append(ordered, ref)
			}
		}
		if !found {
			return "", ErrInvalid
		}
	}
	raw, err := json.Marshal(struct {
		Definition string      `json:"definitionDigest"`
		References []Reference `json:"references"`
	}{definition, ordered})
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:]), nil
}
