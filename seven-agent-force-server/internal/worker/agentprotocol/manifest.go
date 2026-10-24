package agentprotocol

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// Descriptor belongs to the admitted image, never a Task input.
type Descriptor struct {
	ID           string   `json:"id"`
	Version      string   `json:"version"`
	Executable   string   `json:"executable"`
	Args         []string `json:"args"`
	BinarySHA256 string   `json:"binarySha256"`
	SourceSHA256 string   `json:"sourceSha256"`
	Protocol     struct {
		Name string `json:"name"`
		Min  int    `json:"min"`
		Max  int    `json:"max"`
	} `json:"protocol"`
	Transport    string   `json:"transport"`
	Capabilities []string `json:"capabilities"`
}

var ErrManifest = errors.New("ADAPTER_MANIFEST_INVALID")

func ParseDescriptor(body []byte) (Descriptor, error) {
	var manifest struct {
		Adapter Descriptor `json:"adapter"`
	}
	if len(body) == 0 || len(body) > MaxFrameBytes || !strictDocument(body) || json.Unmarshal(body, &manifest) != nil {
		return Descriptor{}, ErrManifest
	}
	a := manifest.Adapter
	if !keyPattern.MatchString(a.ID) || !keyPattern.MatchString(a.Version) || a.Transport != "stdio" || a.Protocol.Name != "seven.agent-adapter" || a.Protocol.Min != 1 || a.Protocol.Max < 1 {
		return Descriptor{}, ErrManifest
	}
	if !filepath.IsAbs(a.Executable) || filepath.Clean(a.Executable) != a.Executable || !strings.HasPrefix(a.Executable, "/opt/seven/adapter/") {
		return Descriptor{}, ErrManifest
	}
	for _, digest := range []string{a.BinarySHA256, a.SourceSHA256} {
		decoded, err := hex.DecodeString(digest)
		if err != nil || len(decoded) != 32 || strings.ToLower(digest) != digest {
			return Descriptor{}, ErrManifest
		}
	}
	if len(a.Args) > 8 || len(a.Capabilities) > 16 {
		return Descriptor{}, ErrManifest
	}
	for _, arg := range a.Args {
		if len(arg) > 256 || strings.ContainsAny(arg, "\x00\n\r") {
			return Descriptor{}, ErrManifest
		}
	}
	seen := map[string]bool{}
	for _, capability := range a.Capabilities {
		if seen[capability] {
			return Descriptor{}, ErrManifest
		}
		seen[capability] = true
		switch capability {
		case "execution.start", "execution.resume", "execution.interrupt", "activity", "interaction.request", "interaction.respond", "interaction.received", "interaction.closed", "user.message", "message.received", "execution.suspend", "execution.suspended", "tool.request", "tool.result", "tool.received", "tool.closed":
		default:
			return Descriptor{}, ErrManifest
		}
	}
	if !seen["execution.start"] || !seen["execution.interrupt"] {
		return Descriptor{}, ErrManifest
	}
	for _, group := range [][]string{{"interaction.request", "interaction.respond", "interaction.received", "interaction.closed"}, {"user.message", "message.received"}, {"execution.suspend", "execution.suspended"}} {
		count := 0
		for _, name := range group {
			if seen[name] {
				count++
			}
		}
		if count != 0 && count != len(group) {
			return Descriptor{}, ErrManifest
		}
	}
	toolCount := 0
	for _, name := range []string{"tool.request", "tool.result", "tool.received", "tool.closed"} {
		if seen[name] {
			toolCount++
		}
	}
	if toolCount != 0 && toolCount != 4 {
		return Descriptor{}, ErrManifest
	}
	if seen["execution.suspend"] && !seen["execution.resume"] {
		return Descriptor{}, ErrManifest
	}
	return a, nil
}

// VerifyEntrypoint is only image-local integrity validation. It cannot replace
// image digest admission or establish container messages as trusted authority.
func (a Descriptor) VerifyEntrypoint() error {
	info, err := os.Lstat(a.Executable)
	if err != nil || !info.Mode().IsRegular() || info.Mode().Perm()&0o222 != 0 || info.Mode().Perm()&0o111 == 0 || info.Size() > MaxFrameBytes {
		return ErrManifest
	}
	file, err := os.Open(a.Executable)
	if err != nil {
		return ErrManifest
	}
	defer file.Close()
	hash := sha256.New()
	if _, err := io.Copy(hash, io.LimitReader(file, MaxFrameBytes+1)); err != nil {
		return ErrManifest
	}
	if hex.EncodeToString(hash.Sum(nil)) != a.BinarySHA256 {
		return ErrManifest
	}
	indexPath := filepath.Join(filepath.Dir(a.Executable), "source-index.json")
	indexInfo, err := os.Lstat(indexPath)
	if err != nil || !indexInfo.Mode().IsRegular() || indexInfo.Mode().Perm()&0o222 != 0 || indexInfo.Size() > MaxFrameBytes {
		return ErrManifest
	}
	body, err := os.ReadFile(indexPath)
	if err != nil {
		return ErrManifest
	}
	indexDigest := sha256.Sum256(body)
	if hex.EncodeToString(indexDigest[:]) != a.SourceSHA256 {
		return ErrManifest
	}
	var entries []struct {
		Path   string `json:"path"`
		SHA256 string `json:"sha256"`
	}
	if !strictDocument(body) || json.Unmarshal(body, &entries) != nil || len(entries) == 0 || len(entries) > 2048 {
		return ErrManifest
	}
	seen := map[string]bool{}
	for _, entry := range entries {
		if entry.Path == "" || filepath.IsAbs(entry.Path) || filepath.Clean(entry.Path) != entry.Path || strings.HasPrefix(entry.Path, "../") || seen[entry.Path] {
			return ErrManifest
		}
		seen[entry.Path] = true
		path := filepath.Join(filepath.Dir(a.Executable), entry.Path)
		resolved, err := filepath.EvalSymlinks(path)
		if err != nil || resolved != path {
			return ErrManifest
		}
		info, err := os.Lstat(path)
		if err != nil || !info.Mode().IsRegular() || info.Mode().Perm()&0o222 != 0 || info.Size() > MaxFrameBytes {
			return ErrManifest
		}
		body, err := os.ReadFile(path)
		if err != nil {
			return ErrManifest
		}
		digest := sha256.Sum256(body)
		if hex.EncodeToString(digest[:]) != entry.SHA256 {
			return ErrManifest
		}
	}
	if !seen[filepath.Base(a.Executable)] {
		return ErrManifest
	}
	return nil
}

func strictDocument(body []byte) bool {
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.UseNumber()
	if strictValue(decoder, 0) != nil {
		return false
	}
	_, err := decoder.Token()
	return err == io.EOF
}
