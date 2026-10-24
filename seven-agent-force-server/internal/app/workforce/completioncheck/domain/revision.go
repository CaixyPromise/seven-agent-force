package domain

import (
	"strings"
	"time"
)

const (
	RevisionPending    = "PENDING"
	RevisionApplied    = "APPLIED"
	RevisionCancelled  = "CANCELLED"
	RevisionSuperseded = "SUPERSEDED"
)

// AdditiveRevision validates a complete proposed Input. Executable replacement
// cannot be proven additive by string-prefix tests, so an existing Command or
// Script stays byte-identical after the existing canonical normalization.
// Text grows only at a paragraph boundary; references form a superset.
func AdditiveRevision(base, next Input) (Input, error) {
	b, e := Normalize(base)
	if e != nil {
		return Input{}, ErrInvalid
	}
	n, e := Normalize(next)
	if e != nil {
		return Input{}, ErrInvalid
	}
	if b.Mode != "NONE" && (n.Mode != b.Mode || n.Command != b.Command || n.Script != b.Script) {
		return Input{}, ErrInvalid
	}
	for _, p := range [][2]string{{b.Instructions, n.Instructions}} {
		if p[0] != "" && p[0] != p[1] && !strings.HasPrefix(p[1], p[0]+"\n") {
			return Input{}, ErrInvalid
		}
	}
	refs := map[int64]bool{}
	for _, id := range n.ReferenceAssetIDs {
		refs[id] = true
	}
	for _, id := range b.ReferenceAssetIDs {
		if !refs[id] {
			return Input{}, ErrInvalid
		}
	}
	oldDigest, _ := Digest(b)
	newDigest, _ := Digest(n)
	if oldDigest == newDigest {
		return Input{}, ErrInvalid
	}
	return n, nil
}

type RevisionState struct {
	Status                 string
	Version, AppliedTurnID int64
	AppliedAt              *time.Time
}

func (r *RevisionState) Apply(expected, turn int64, now time.Time) error {
	if r.Status != RevisionPending || r.Version != expected || turn <= 0 {
		return ErrInvalid
	}
	r.Status = RevisionApplied
	r.Version++
	r.AppliedTurnID = turn
	r.AppliedAt = &now
	return nil
}
