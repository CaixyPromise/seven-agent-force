package runtime

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"

	"github.com/CaixyPromise/seven-agent/force/internal/app/workforce/completioncheck/domain"
)

var ErrCheckDefinitionUnavailable = errors.New("CHECK_DEFINITION_UNAVAILABLE")

// checkDefinitionDigest protects recognized repository entrypoints, not the
// semantics of arbitrary project code. It never evaluates a shell expression.
// Uploaded script bytes already belong to the frozen input identity.
func checkDefinitionDigest(repository string, input domain.Input) (string, error) {
	if input.Mode != "COMMAND" {
		return "", nil
	}
	words := strings.Fields(input.Command)
	if len(words) == 0 {
		return "", nil
	}
	var relative string
	packageScript := false
	switch words[0] {
	case "pnpm", "npm", "yarn", "bun":
		if len(words) < 2 {
			return "", nil
		}
		packageScript = true
		relative = "package.json"
	case "sh", "bash":
		if len(words) != 2 || strings.HasPrefix(words[1], "-") {
			return "", nil
		}
		relative = words[1]
	default:
		if !strings.HasPrefix(words[0], "./") || !strings.HasSuffix(words[0], ".sh") {
			return "", nil
		}
		relative = words[0]
	}
	relative = filepath.Clean(relative)
	if filepath.IsAbs(relative) || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) || strings.ContainsAny(relative, "\\\x00") {
		return "", ErrCheckDefinitionUnavailable
	}
	root, err := os.OpenRoot(repository)
	if err != nil {
		return "", ErrCheckDefinitionUnavailable
	}
	defer root.Close()
	info, err := root.Lstat(relative)
	var payload []byte
	if os.IsNotExist(err) {
		payload = []byte("MISSING")
	} else {
		if err != nil || !info.Mode().IsRegular() || info.Size() > 1<<20 {
			return "", ErrCheckDefinitionUnavailable
		}
		payload, err = root.ReadFile(relative)
		if err != nil {
			return "", ErrCheckDefinitionUnavailable
		}
		if packageScript {
			var manifest struct {
				Scripts map[string]string `json:"scripts"`
			}
			if json.Unmarshal(payload, &manifest) != nil {
				return "", ErrCheckDefinitionUnavailable
			}
			payload, err = json.Marshal(manifest.Scripts)
			if err != nil {
				return "", ErrCheckDefinitionUnavailable
			}
		}
	}
	sum := sha256.Sum256(append([]byte(relative+"\x00"), payload...))
	return hex.EncodeToString(sum[:]), nil
}
