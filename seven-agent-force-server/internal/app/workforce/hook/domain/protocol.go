package domain

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"path"
	"sort"
	"strings"
	"time"
	"unicode/utf8"
)

const (
	PackageSchema    = "seven.hook-package/v1"
	MaxArchiveBytes  = 8 << 20
	MaxFileBytes     = 2 << 20
	MaxFiles         = 256
	MaxInputBytes    = 1 << 20
	MaxOutputBytes   = 1 << 20
	MaxErrorBytes    = 512 << 10
	DefaultTimeoutMS = 5000
	MaximumTimeoutMS = 30000
)

var supportedEvents = map[string]bool{
	"attempt.prepare": true, "attempt.propose": true, "attempt.end": true,
	"codex.PreToolUse": true, "codex.PostToolUse": true,
	"claude.PreToolUse": true, "claude.PostToolUse": true,
}

type Runtime struct {
	Kind        string `json:"kind"`
	Requirement string `json:"requirement,omitempty"`
}

type Entrypoint struct {
	Event       string `json:"event"`
	Path        string `json:"path"`
	TimeoutMS   int    `json:"timeoutMs"`
	FailureMode string `json:"failureMode"`
}

type Limits struct {
	StdinBytes  int64 `json:"stdinBytes"`
	StdoutBytes int64 `json:"stdoutBytes"`
	StderrBytes int64 `json:"stderrBytes"`
}

type Manifest struct {
	SchemaVersion string       `json:"schemaVersion"`
	Name          string       `json:"name"`
	Version       string       `json:"version,omitempty"`
	TrustClass    string       `json:"trustClass"`
	Runtime       Runtime      `json:"runtime"`
	Entrypoints   []Entrypoint `json:"entrypoints"`
	Limits        Limits       `json:"limits"`
}

type SourceFile struct {
	Path    string `json:"path"`
	Content string `json:"content"`
	Mode    int64  `json:"mode,omitempty"`
}

type HookResult struct {
	SchemaVersion   int              `json:"schemaVersion"`
	Decision        string           `json:"decision"`
	ReasonCode      string           `json:"reasonCode"`
	OperatorMessage string           `json:"operatorMessage"`
	Evidence        []map[string]any `json:"evidence"`
}

type Validation struct {
	Valid        bool     `json:"valid"`
	ReasonCode   string   `json:"reasonCode"`
	Errors       []string `json:"errors"`
	ManifestHash string   `json:"manifestSha256,omitempty"`
	SourceHash   string   `json:"sourceSha256,omitempty"`
}

func Validate(manifest Manifest, files []SourceFile) Validation {
	result := Validation{Valid: false, ReasonCode: "HOOK_PACKAGE_INVALID", Errors: []string{}}
	if manifest.SchemaVersion != PackageSchema {
		result.Errors = append(result.Errors, "schemaVersion must be seven.hook-package/v1")
	}
	if strings.TrimSpace(manifest.Name) == "" || len(manifest.Name) > 128 {
		result.Errors = append(result.Errors, "name is required and must be at most 128 characters")
	}
	if manifest.TrustClass != "PLATFORM_TRUSTED" {
		result.Errors = append(result.Errors, "trustClass must be PLATFORM_TRUSTED")
	}
	runtimeKind := strings.ToLower(strings.TrimSpace(manifest.Runtime.Kind))
	if runtimeKind != "shell" && runtimeKind != "node" {
		result.Errors = append(result.Errors, "runtime.kind must be shell or node")
	}
	if runtimeKind == "node" && manifest.Runtime.Requirement != "" && manifest.Runtime.Requirement != ">=22 <23" {
		result.Errors = append(result.Errors, "node runtime requirement must be >=22 <23")
	}
	if len(files) == 0 || len(files) > MaxFiles {
		result.Errors = append(result.Errors, fmt.Sprintf("package must contain between 1 and %d files", MaxFiles))
	}
	seen := map[string]bool{}
	seenFolded := map[string]bool{}
	fileByPath := map[string]SourceFile{}
	for _, file := range files {
		clean, err := safePath(file.Path)
		if err != nil {
			result.Errors = append(result.Errors, err.Error())
			continue
		}
		if clean == "seven-hook-package.json" {
			result.Errors = append(result.Errors, "seven-hook-package.json is reserved for the canonical manifest")
			continue
		}
		folded := strings.ToLower(clean)
		if seen[clean] || seenFolded[folded] {
			result.Errors = append(result.Errors, "duplicate or case-colliding path: "+clean)
			continue
		}
		seen[clean], seenFolded[folded] = true, true
		if len(file.Content) > MaxFileBytes || !utf8.ValidString(file.Content) {
			result.Errors = append(result.Errors, "file is not bounded UTF-8 text: "+clean)
		}
		if forbiddenSource(clean, file.Content) {
			result.Errors = append(result.Errors, "online dependency installation is forbidden: "+clean)
		}
		fileByPath[clean] = file
	}
	if len(manifest.Entrypoints) == 0 {
		result.Errors = append(result.Errors, "at least one entrypoint is required")
	}
	entryEvents := map[string]bool{}
	for index := range manifest.Entrypoints {
		entry := &manifest.Entrypoints[index]
		if !supportedEvents[entry.Event] {
			result.Errors = append(result.Errors, "unsupported event: "+entry.Event)
		}
		if entryEvents[entry.Event] {
			result.Errors = append(result.Errors, "duplicate event entrypoint: "+entry.Event)
		}
		entryEvents[entry.Event] = true
		clean, err := safePath(entry.Path)
		if err != nil || fileByPath[clean].Path == "" {
			result.Errors = append(result.Errors, "entrypoint file is missing or unsafe: "+entry.Path)
		}
		if entry.TimeoutMS == 0 {
			entry.TimeoutMS = DefaultTimeoutMS
		}
		if entry.TimeoutMS < 1 || entry.TimeoutMS > MaximumTimeoutMS {
			result.Errors = append(result.Errors, "entrypoint timeoutMs must be between 1 and 30000")
		}
		if entry.FailureMode != "BLOCK" && entry.FailureMode != "OBSERVE" {
			result.Errors = append(result.Errors, "failureMode must be BLOCK or OBSERVE")
		}
	}
	if manifest.Limits.StdinBytes == 0 {
		manifest.Limits.StdinBytes = MaxInputBytes
	}
	if manifest.Limits.StdoutBytes == 0 {
		manifest.Limits.StdoutBytes = MaxOutputBytes
	}
	if manifest.Limits.StderrBytes == 0 {
		manifest.Limits.StderrBytes = MaxErrorBytes
	}
	if manifest.Limits.StdinBytes < 1 || manifest.Limits.StdinBytes > MaxInputBytes || manifest.Limits.StdoutBytes < 1 || manifest.Limits.StdoutBytes > MaxOutputBytes || manifest.Limits.StderrBytes < 1 || manifest.Limits.StderrBytes > MaxErrorBytes {
		result.Errors = append(result.Errors, "manifest limits exceed the v1 protocol maximum")
	}
	manifestBody, _ := json.Marshal(manifest)
	result.ManifestHash = sum(manifestBody)
	canonicalFiles, _ := json.Marshal(sortedFiles(files))
	result.SourceHash = sum(canonicalFiles)
	result.Valid = len(result.Errors) == 0
	if result.Valid {
		result.ReasonCode = "OK"
	}
	return result
}

func CanonicalArchive(manifest Manifest, files []SourceFile) ([]byte, string, error) {
	validation := Validate(manifest, files)
	if !validation.Valid {
		return nil, "", errors.New(strings.Join(validation.Errors, "; "))
	}
	manifestBody, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return nil, "", err
	}
	manifestBody = append(manifestBody, '\n')
	all := append([]SourceFile{{Path: "seven-hook-package.json", Content: string(manifestBody), Mode: 0o644}}, sortedFiles(files)...)
	var raw bytes.Buffer
	gz, _ := gzip.NewWriterLevel(&raw, gzip.BestCompression)
	gz.Header.ModTime = time.Unix(0, 0)
	gz.Header.OS = 255
	tarWriter := tar.NewWriter(gz)
	for _, file := range all {
		mode := file.Mode
		if mode == 0 {
			mode = 0o644
		}
		header := &tar.Header{Name: file.Path, Mode: mode & 0o755, Size: int64(len(file.Content)), ModTime: time.Unix(0, 0), Typeflag: tar.TypeReg, Format: tar.FormatPAX}
		if err := tarWriter.WriteHeader(header); err != nil {
			return nil, "", err
		}
		if _, err := tarWriter.Write([]byte(file.Content)); err != nil {
			return nil, "", err
		}
	}
	if err := tarWriter.Close(); err != nil {
		return nil, "", err
	}
	if err := gz.Close(); err != nil {
		return nil, "", err
	}
	if raw.Len() > MaxArchiveBytes {
		return nil, "", errors.New("canonical archive exceeds 8 MiB")
	}
	return raw.Bytes(), sum(raw.Bytes()), nil
}

func ImportArchive(body []byte) (Manifest, []SourceFile, error) {
	if len(body) == 0 || len(body) > MaxArchiveBytes {
		return Manifest{}, nil, errors.New("archive is empty or exceeds 8 MiB")
	}
	gz, err := gzip.NewReader(bytes.NewReader(body))
	if err != nil {
		return Manifest{}, nil, fmt.Errorf("open gzip archive: %w", err)
	}
	defer gz.Close()
	reader := tar.NewReader(io.LimitReader(gz, MaxArchiveBytes*4))
	var manifest Manifest
	files := []SourceFile{}
	seenManifest := false
	for count := 0; ; count++ {
		if count > MaxFiles+1 {
			return Manifest{}, nil, errors.New("archive contains too many files")
		}
		header, err := reader.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return Manifest{}, nil, fmt.Errorf("read archive: %w", err)
		}
		if header.Typeflag != tar.TypeReg && header.Typeflag != tar.TypeRegA {
			return Manifest{}, nil, fmt.Errorf("archive member %q is not a regular file", header.Name)
		}
		clean, err := safePath(header.Name)
		if err != nil {
			return Manifest{}, nil, err
		}
		if header.Size < 0 || header.Size > MaxFileBytes {
			return Manifest{}, nil, fmt.Errorf("archive member %q is oversized", clean)
		}
		content, err := io.ReadAll(io.LimitReader(reader, MaxFileBytes+1))
		if err != nil || len(content) > MaxFileBytes {
			return Manifest{}, nil, fmt.Errorf("read archive member %q", clean)
		}
		if clean == "seven-hook-package.json" {
			if seenManifest {
				return Manifest{}, nil, errors.New("archive contains duplicate manifest")
			}
			seenManifest = true
			decoder := json.NewDecoder(bytes.NewReader(content))
			decoder.DisallowUnknownFields()
			if err := decoder.Decode(&manifest); err != nil {
				return Manifest{}, nil, fmt.Errorf("decode manifest: %w", err)
			}
			continue
		}
		files = append(files, SourceFile{Path: clean, Content: string(content), Mode: header.Mode & 0o755})
	}
	if !seenManifest {
		return Manifest{}, nil, errors.New("seven-hook-package.json is required")
	}
	validation := Validate(manifest, files)
	if !validation.Valid {
		return Manifest{}, nil, errors.New(strings.Join(validation.Errors, "; "))
	}
	return manifest, files, nil
}

func ValidateResult(body []byte) (HookResult, error) {
	if len(body) == 0 || len(body) > MaxOutputBytes {
		return HookResult{}, errors.New("hook output is empty or oversized")
	}
	var result HookResult
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&result); err != nil {
		return HookResult{}, fmt.Errorf("decode hook result: %w", err)
	}
	if decoder.Decode(&struct{}{}) != io.EOF {
		return HookResult{}, errors.New("hook result must contain exactly one JSON value")
	}
	if result.SchemaVersion != 1 || (result.Decision != "ALLOW" && result.Decision != "DENY") || strings.TrimSpace(result.ReasonCode) == "" {
		return HookResult{}, errors.New("hook result schema, decision, or reasonCode is invalid")
	}
	return result, nil
}

func safePath(value string) (string, error) {
	value = strings.TrimSpace(strings.ReplaceAll(value, "\\", "/"))
	clean := path.Clean(value)
	if value == "" || clean == "." || strings.HasPrefix(value, "/") || clean == ".." || strings.HasPrefix(clean, "../") || clean != value {
		return "", fmt.Errorf("unsafe package path: %s", value)
	}
	return clean, nil
}

func forbiddenSource(name, content string) bool {
	lowerName, lower := strings.ToLower(name), strings.ToLower(content)
	if lowerName == "package-lock.json" || lowerName == "pnpm-lock.yaml" || lowerName == "yarn.lock" || strings.HasPrefix(lowerName, "node_modules/") {
		return true
	}
	if lowerName == "package.json" {
		var packageJSON struct {
			Scripts map[string]string `json:"scripts"`
		}
		if json.Unmarshal([]byte(content), &packageJSON) != nil {
			return true
		}
		for script := range packageJSON.Scripts {
			switch strings.ToLower(script) {
			case "preinstall", "install", "postinstall", "prepare", "prepublish", "prepublishonly":
				return true
			}
		}
	}
	for _, needle := range []string{"npm install", "npm ci", "pnpm install", "yarn install", "curl http", "wget http"} {
		if strings.Contains(lower, needle) {
			return true
		}
	}
	return false
}

func sortedFiles(files []SourceFile) []SourceFile {
	result := append([]SourceFile(nil), files...)
	for index := range result {
		result[index].Path = strings.ReplaceAll(strings.TrimSpace(result[index].Path), "\\", "/")
		if result[index].Mode == 0 {
			result[index].Mode = 0o644
		}
	}
	sort.Slice(result, func(i, j int) bool { return result[i].Path < result[j].Path })
	return result
}

func sum(value []byte) string { digest := sha256.Sum256(value); return hex.EncodeToString(digest[:]) }
