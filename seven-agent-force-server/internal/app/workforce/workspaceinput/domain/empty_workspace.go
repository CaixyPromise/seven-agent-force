package domain

import (
	"crypto/sha1" // Git SHA-1 object format is the admitted repository contract.
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
)

const (
	SourceKindEmptyWorkspace      = "EMPTY_WORKSPACE"
	SourceKindSourceRevision      = "SOURCE_REVISION"
	EmptyWorkspaceContractVersion = 1
	EmptyWorkspaceTree            = "4b825dc642cb6eb9a060e54bf8d69288fbee4904"
)

type EmptyWorkspaceIdentity struct {
	ContractVersion int    `json:"contractVersion"`
	Commit          string `json:"commit"`
	Tree            string `json:"tree"`
	IdentityDigest  string `json:"identityDigest"`
}

func CanonicalEmptyWorkspaceIdentity() EmptyWorkspaceIdentity {
	body := EmptyWorkspaceCommitBody()
	header := []byte(fmt.Sprintf("commit %d\x00", len(body)))
	gitSum := sha1.Sum(append(header, body...))
	commit := hex.EncodeToString(gitSum[:])
	canonical, _ := json.Marshal(struct {
		ContractVersion int    `json:"contractVersion"`
		Commit          string `json:"commit"`
		Tree            string `json:"tree"`
	}{EmptyWorkspaceContractVersion, commit, EmptyWorkspaceTree})
	identity := sha256.Sum256(canonical)
	return EmptyWorkspaceIdentity{EmptyWorkspaceContractVersion, commit, EmptyWorkspaceTree, hex.EncodeToString(identity[:])}
}

func EmptyWorkspaceCommitBody() []byte {
	return []byte("tree " + EmptyWorkspaceTree + "\n" +
		"author Seven Agent Force <workforce@seven.invalid> 0 +0000\n" +
		"committer Seven Agent Force <workforce@seven.invalid> 0 +0000\n\n" +
		"Seven Agent Force empty workspace\n")
}
