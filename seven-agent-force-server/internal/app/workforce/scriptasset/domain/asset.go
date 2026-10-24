// Package domain owns the generic, security-sensitive representation of a
// platform-managed script asset.  It deliberately contains no Hook or Result
// Processor policy: callers provide the manifest name and any reserved paths,
// while this package makes archive identity and filesystem handling stable.
package domain

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
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
	// MaxArchiveBytes bounds an uploaded and canonicalized asset archive.
	MaxArchiveBytes = 8 << 20
	// MaxFileBytes bounds each UTF-8 source file.
	MaxFileBytes = 2 << 20
	// MaxFiles prevents archive fan-out and path-confusion attacks.
	MaxFiles = 256
)

// SourceFile is an in-memory text member of a canonical script asset.
// Modes are retained in the digest but only ordinary non-privileged bits are
// accepted by the canonicalizer.
type SourceFile struct {
	Path    string `json:"path"`
	Content string `json:"content"`
	Mode    int64  `json:"mode,omitempty"`
}

// Validation reports every bounded input error in one response.  It is safe to
// return to an editor because it contains paths and policy errors only.
type Validation struct {
	Valid      bool     `json:"valid"`
	ReasonCode string   `json:"reasonCode"`
	Errors     []string `json:"errors"`
	SourceHash string   `json:"sourceSha256,omitempty"`
}

// ValidateFiles checks reusable archive invariants. reserved must contain
// exact, slash-normalized paths that callers own (for example processor.json).
func ValidateFiles(files []SourceFile, reserved ...string) Validation {
	result := Validation{ReasonCode: "SCRIPT_ASSET_INVALID", Errors: []string{}}
	if len(files) == 0 || len(files) > MaxFiles {
		result.Errors = append(result.Errors, fmt.Sprintf("asset must contain between 1 and %d files", MaxFiles))
	}
	reservedSet := make(map[string]struct{}, len(reserved))
	for _, item := range reserved {
		if clean, err := CleanPath(item); err == nil {
			reservedSet[clean] = struct{}{}
		}
	}
	seen := make(map[string]struct{}, len(files))
	seenFolded := make(map[string]struct{}, len(files))
	for _, file := range files {
		clean, err := CleanPath(file.Path)
		if err != nil {
			result.Errors = append(result.Errors, err.Error())
			continue
		}
		if _, found := reservedSet[clean]; found {
			result.Errors = append(result.Errors, "reserved asset path: "+clean)
			continue
		}
		folded := strings.ToLower(clean)
		if _, found := seen[clean]; found {
			result.Errors = append(result.Errors, "duplicate asset path: "+clean)
			continue
		}
		if _, found := seenFolded[folded]; found {
			result.Errors = append(result.Errors, "case-colliding asset path: "+clean)
			continue
		}
		seen[clean] = struct{}{}
		seenFolded[folded] = struct{}{}
		if int64(len(file.Content)) > MaxFileBytes {
			result.Errors = append(result.Errors, "asset file exceeds 2 MiB: "+clean)
		}
		if !utf8.ValidString(file.Content) {
			result.Errors = append(result.Errors, "asset file is not UTF-8: "+clean)
		}
		if file.Mode&^(int64(0o755)) != 0 || file.Mode&0o6000 != 0 {
			result.Errors = append(result.Errors, "asset file has forbidden mode bits: "+clean)
		}
	}
	canonical, err := CanonicalFiles(files)
	if err == nil {
		result.SourceHash = SHA256(canonical)
	}
	result.Valid = len(result.Errors) == 0
	if result.Valid {
		result.ReasonCode = "OK"
	}
	return result
}

// CleanPath returns a slash-normalized ordinary relative path.  Archives never
// accept links, directories, parent traversal, or platform-dependent paths.
func CleanPath(value string) (string, error) {
	value = strings.TrimSpace(strings.ReplaceAll(value, "\\", "/"))
	clean := path.Clean(value)
	if value == "" || clean == "." || strings.HasPrefix(value, "/") || clean == ".." || strings.HasPrefix(clean, "../") || clean != value {
		return "", fmt.Errorf("unsafe asset path: %s", value)
	}
	return clean, nil
}

// CanonicalFiles gives callers stable JSON bytes for an index/digest without
// exposing a filesystem representation.
func CanonicalFiles(files []SourceFile) ([]byte, error) {
	normalized := append([]SourceFile(nil), files...)
	for index := range normalized {
		clean, err := CleanPath(normalized[index].Path)
		if err != nil {
			return nil, err
		}
		normalized[index].Path = clean
		if normalized[index].Mode == 0 {
			normalized[index].Mode = 0o644
		}
	}
	sort.Slice(normalized, func(i, j int) bool { return normalized[i].Path < normalized[j].Path })
	var out bytes.Buffer
	for _, file := range normalized {
		// A length-delimited wire format avoids ambiguous concatenations while
		// retaining mode/path/payload identity in the digest.
		fmt.Fprintf(&out, "%d:%s:%o:%d:", len(file.Path), file.Path, file.Mode&0o755, len(file.Content))
		out.WriteString(file.Content)
	}
	return out.Bytes(), nil
}

// CanonicalArchive creates a reproducible .tar.gz with fixed timestamps and
// only regular files. manifestName is injected as the first, reserved member.
func CanonicalArchive(manifestName string, manifest []byte, files []SourceFile) ([]byte, string, error) {
	manifestName, err := CleanPath(manifestName)
	if err != nil {
		return nil, "", err
	}
	validation := ValidateFiles(files, manifestName)
	if !validation.Valid {
		return nil, "", errors.New(strings.Join(validation.Errors, "; "))
	}
	if len(manifest) == 0 || !utf8.Valid(manifest) || len(manifest) > MaxFileBytes {
		return nil, "", errors.New("canonical asset manifest is invalid")
	}
	normalized := append([]SourceFile(nil), files...)
	for index := range normalized {
		normalized[index].Path, _ = CleanPath(normalized[index].Path)
		if normalized[index].Mode == 0 {
			normalized[index].Mode = 0o644
		}
	}
	sort.Slice(normalized, func(i, j int) bool { return normalized[i].Path < normalized[j].Path })

	var result bytes.Buffer
	gzipWriter, err := gzip.NewWriterLevel(&result, gzip.BestCompression)
	if err != nil {
		return nil, "", err
	}
	gzipWriter.Header.ModTime = time.Unix(0, 0)
	gzipWriter.Header.OS = 255
	tarWriter := tar.NewWriter(gzipWriter)
	all := append([]SourceFile{{Path: manifestName, Content: string(manifest), Mode: 0o644}}, normalized...)
	for _, file := range all {
		mode := file.Mode
		if mode == 0 {
			mode = 0o644
		}
		header := &tar.Header{
			Name:     file.Path,
			Mode:     mode & 0o755,
			Size:     int64(len(file.Content)),
			ModTime:  time.Unix(0, 0),
			Typeflag: tar.TypeReg,
			Format:   tar.FormatPAX,
		}
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
	if err := gzipWriter.Close(); err != nil {
		return nil, "", err
	}
	if result.Len() > MaxArchiveBytes {
		return nil, "", errors.New("canonical script asset exceeds 8 MiB")
	}
	return result.Bytes(), SHA256(result.Bytes()), nil
}

// ImportArchive safely decodes an uploaded canonical-or-equivalent archive.
// It returns raw manifest bytes so the owning domain can enforce its own
// schema.  Any non-regular archive member is rejected before materialization.
func ImportArchive(manifestName string, body []byte) ([]byte, []SourceFile, error) {
	manifestName, err := CleanPath(manifestName)
	if err != nil {
		return nil, nil, err
	}
	if len(body) == 0 || len(body) > MaxArchiveBytes {
		return nil, nil, errors.New("script asset archive is empty or exceeds 8 MiB")
	}
	gzipReader, err := gzip.NewReader(bytes.NewReader(body))
	if err != nil {
		return nil, nil, fmt.Errorf("open asset gzip: %w", err)
	}
	defer gzipReader.Close()
	tarReader := tar.NewReader(io.LimitReader(gzipReader, MaxArchiveBytes*4))
	var manifest []byte
	files := make([]SourceFile, 0)
	for count := 0; ; count++ {
		if count > MaxFiles+1 {
			return nil, nil, errors.New("script asset archive contains too many members")
		}
		header, nextErr := tarReader.Next()
		if errors.Is(nextErr, io.EOF) {
			break
		}
		if nextErr != nil {
			return nil, nil, fmt.Errorf("read script asset archive: %w", nextErr)
		}
		if header.Typeflag != tar.TypeReg && header.Typeflag != tar.TypeRegA {
			return nil, nil, fmt.Errorf("asset member %q is not a regular file", header.Name)
		}
		clean, pathErr := CleanPath(header.Name)
		if pathErr != nil {
			return nil, nil, pathErr
		}
		if header.Size < 0 || header.Size > MaxFileBytes || header.Mode&0o6000 != 0 {
			return nil, nil, fmt.Errorf("asset member %q is oversized or privileged", clean)
		}
		content, readErr := io.ReadAll(io.LimitReader(tarReader, MaxFileBytes+1))
		if readErr != nil || len(content) > MaxFileBytes || !utf8.Valid(content) {
			return nil, nil, fmt.Errorf("asset member %q is not bounded UTF-8", clean)
		}
		if clean == manifestName {
			if manifest != nil {
				return nil, nil, errors.New("script asset archive contains duplicate manifest")
			}
			manifest = append([]byte(nil), content...)
			continue
		}
		files = append(files, SourceFile{Path: clean, Content: string(content), Mode: header.Mode & 0o755})
	}
	if manifest == nil {
		return nil, nil, fmt.Errorf("%s is required", manifestName)
	}
	validation := ValidateFiles(files, manifestName)
	if !validation.Valid {
		return nil, nil, errors.New(strings.Join(validation.Errors, "; "))
	}
	return manifest, files, nil
}

// SHA256 uses hexadecimal lowercase output throughout Workforce identities.
func SHA256(body []byte) string {
	digest := sha256.Sum256(body)
	return hex.EncodeToString(digest[:])
}
