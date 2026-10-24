package checkpoint

import (
	"archive/tar"
	"archive/zip"
	"compress/gzip"
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	processordomain "github.com/CaixyPromise/seven-agent/force/internal/app/workforce/resultprocessor/domain"
)

// PackSelectionArchive resolves a published Output selection against a trusted
// restored checkpoint and creates deterministic bytes. The Processor never
// supplies the final file list, sizes, modes, or digests.
func PackSelectionArchive(ctx context.Context, root string, selection processordomain.PathSelection, format, destination string) (SpoolBlob, []SelectionEntry, error) {
	includes, err := compileOutputPatterns(selection.Includes)
	if err != nil {
		return SpoolBlob{}, nil, err
	}
	excludes, err := compileOutputPatterns(selection.Excludes)
	if err != nil {
		return SpoolBlob{}, nil, err
	}
	root, err = filepath.Abs(root)
	if err != nil {
		return SpoolBlob{}, nil, err
	}
	entries := make([]SelectionEntry, 0, 64)
	err = filepath.WalkDir(root, func(candidate string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		relative, err := filepath.Rel(root, candidate)
		if err != nil || relative == "." {
			return err
		}
		relative = filepath.ToSlash(relative)
		if protectedOutputPath(relative) {
			if entry.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if entry.IsDir() {
			return nil
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		if !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 || info.Mode()&(os.ModeSetuid|os.ModeSetgid) != 0 || info.Size() > MaxFileBytes {
			return errors.New("RESULT_OUTPUT_FILE_TYPE_FORBIDDEN")
		}
		if !outputMatchesAny(includes, relative) || outputMatchesAny(excludes, relative) {
			return nil
		}
		digest, err := fileDigest(candidate)
		if err != nil {
			return err
		}
		entries = append(entries, SelectionEntry{Path: relative, Type: "FILE", GitMode: "100644", SizeBytes: info.Size(), SHA256: digest, Source: "OUTPUT_SELECTION", ReasonCode: "PROCESSOR_DECLARED"})
		return nil
	})
	if err != nil {
		return SpoolBlob{}, nil, err
	}
	if len(entries) == 0 || len(entries) > MaxArchiveFiles {
		return SpoolBlob{}, nil, errors.New("RESULT_OUTPUT_SELECTION_EMPTY")
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].Path < entries[j].Path })
	var result SpoolBlob
	switch format {
	case "ZIP":
		result, err = writeDeterministicZIP(root, destination, entries)
	case "TAR_GZ":
		result, err = writeDeterministicTarGzip(root, destination, entries)
	default:
		err = errors.New("RESULT_OUTPUT_ARCHIVE_FORMAT_INVALID")
	}
	return result, entries, err
}

func writeDeterministicZIP(root, destination string, entries []SelectionEntry) (SpoolBlob, error) {
	return writeAtomic(destination, func(target io.Writer) error {
		writer := zip.NewWriter(target)
		for _, entry := range entries {
			header := &zip.FileHeader{Name: entry.Path, Method: zip.Deflate}
			header.SetModTime(time.Date(1980, 1, 1, 0, 0, 0, 0, time.UTC))
			header.SetMode(0o600)
			out, err := writer.CreateHeader(header)
			if err != nil {
				return err
			}
			if err = copyExactFile(filepath.Join(root, filepath.FromSlash(entry.Path)), out, entry.SizeBytes); err != nil {
				return err
			}
		}
		return writer.Close()
	})
}

func writeDeterministicTarGzip(root, destination string, entries []SelectionEntry) (SpoolBlob, error) {
	return writeAtomic(destination, func(target io.Writer) error {
		gz := gzip.NewWriter(target)
		gz.Name, gz.Comment, gz.ModTime = "", "", time.Unix(0, 0).UTC()
		writer := tar.NewWriter(gz)
		for _, entry := range entries {
			header := &tar.Header{Name: entry.Path, Mode: 0o600, Size: entry.SizeBytes, ModTime: time.Unix(0, 0).UTC(), AccessTime: time.Unix(0, 0).UTC(), ChangeTime: time.Unix(0, 0).UTC(), Uid: 0, Gid: 0, Uname: "", Gname: "", Format: tar.FormatPAX}
			if err := writer.WriteHeader(header); err != nil {
				return err
			}
			if err := copyExactFile(filepath.Join(root, filepath.FromSlash(entry.Path)), writer, entry.SizeBytes); err != nil {
				return err
			}
		}
		if err := writer.Close(); err != nil {
			return err
		}
		return gz.Close()
	})
}

func copyExactFile(source string, target io.Writer, size int64) error {
	file, err := os.Open(source)
	if err != nil {
		return err
	}
	written, copyErr := io.Copy(target, io.LimitReader(file, size+1))
	closeErr := file.Close()
	if copyErr != nil || closeErr != nil || written != size {
		return errors.New("RESULT_OUTPUT_SOURCE_CHANGED")
	}
	return nil
}

func compileOutputPatterns(patterns []string) ([]*regexp.Regexp, error) {
	compiled := make([]*regexp.Regexp, 0, len(patterns))
	for _, pattern := range patterns {
		pattern = filepath.ToSlash(strings.TrimSpace(pattern))
		if pattern == "" || strings.HasPrefix(pattern, "/") || strings.Contains(pattern, "..") || protectedOutputPath(strings.TrimSuffix(pattern, "/**")) {
			return nil, errors.New("RESULT_OUTPUT_PATTERN_INVALID")
		}
		var expression strings.Builder
		expression.WriteString("^")
		for index := 0; index < len(pattern); {
			switch pattern[index] {
			case '*':
				if index+1 < len(pattern) && pattern[index+1] == '*' {
					if index+2 < len(pattern) && pattern[index+2] == '/' {
						expression.WriteString("(?:.*/)?")
						index += 3
					} else {
						expression.WriteString(".*")
						index += 2
					}
				} else {
					expression.WriteString("[^/]*")
					index++
				}
			case '?':
				expression.WriteString("[^/]")
				index++
			default:
				expression.WriteString(regexp.QuoteMeta(string(pattern[index])))
				index++
			}
		}
		expression.WriteString("$")
		item, err := regexp.Compile(expression.String())
		if err != nil {
			return nil, errors.New("RESULT_OUTPUT_PATTERN_INVALID")
		}
		compiled = append(compiled, item)
	}
	return compiled, nil
}

func outputMatchesAny(patterns []*regexp.Regexp, value string) bool {
	for _, pattern := range patterns {
		if pattern.MatchString(value) {
			return true
		}
	}
	return false
}

func protectedOutputPath(value string) bool {
	value = strings.TrimPrefix(filepath.ToSlash(strings.TrimSpace(value)), "./")
	first := strings.Split(value, "/")[0]
	switch first {
	case ".git", ".seven", ".codex", "codex", "session", "runtime", "control", "secrets":
		return true
	default:
		return false
	}
}
