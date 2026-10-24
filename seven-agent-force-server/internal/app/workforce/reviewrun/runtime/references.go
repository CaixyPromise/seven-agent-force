package runtime

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// materializeReviewReferences copies only the frozen selected attachments into
// an owner-attempt input directory. The Task input root is never mounted into
// the Reviewer. Existing copies are independently verified on recovery.
func materializeReviewReferences(taskRoot, destination string, requirements []byte) (bool, error) {
	var doc struct {
		References []struct {
			ID     int64  `json:"referenceAssetId,string"`
			Name   string `json:"name"`
			Digest string `json:"contentSha256"`
			Size   int64  `json:"sizeBytes,string"`
		} `json:"references"`
	}
	if json.Unmarshal(requirements, &doc) != nil || len(doc.References) > 64 {
		return false, ErrReviewInputInvalid
	}
	if len(doc.References) == 0 {
		return false, nil
	}
	if !filepath.IsAbs(taskRoot) || !filepath.IsAbs(destination) {
		return false, ErrReviewInputInvalid
	}
	if err := os.MkdirAll(destination, 0755); err != nil {
		return false, ErrReviewInputInvalid
	}
	if !plainDirectory(destination) {
		return false, ErrReviewInputInvalid
	}
	source := filepath.Join(taskRoot, "input", "references")
	if !plainDirectory(taskRoot) || !plainDirectory(filepath.Join(taskRoot, "input")) || !plainDirectory(source) {
		return false, ErrReviewInputInvalid
	}
	seen := map[int64]bool{}
	var total int64
	for _, ref := range doc.References {
		total += ref.Size
		if ref.ID <= 0 || seen[ref.ID] || ref.Size < 0 || ref.Size > 16<<20 || total > 64<<20 || len(ref.Digest) != 64 || strings.ToLower(ref.Digest) != ref.Digest {
			return false, ErrReviewInputInvalid
		}
		if _, err := hex.DecodeString(ref.Digest); err != nil {
			return false, ErrReviewInputInvalid
		}
		seen[ref.ID] = true
		id := strconv.FormatInt(ref.ID, 10)
		name := filepath.Base(ref.Name)
		if name == "" || name == "." || name == ".." || strings.ContainsAny(name, "\\\x00") {
			return false, ErrReviewInputInvalid
		}
		sourceDir := filepath.Join(source, id)
		if !plainDirectory(sourceDir) {
			return false, ErrReviewInputInvalid
		}
		body, err := verifiedReference(filepath.Join(sourceDir, name), ref.Size, ref.Digest)
		if err != nil {
			return false, err
		}
		targetDir := filepath.Join(destination, id)
		if err = os.MkdirAll(targetDir, 0755); err != nil || !plainDirectory(targetDir) {
			return false, ErrReviewInputInvalid
		}
		target := filepath.Join(targetDir, "content")
		if _, err = os.Lstat(target); err == nil {
			if _, err = verifiedReference(target, ref.Size, ref.Digest); err != nil {
				return false, err
			}
			continue
		} else if !os.IsNotExist(err) {
			return false, ErrReviewInputInvalid
		}
		file, err := os.OpenFile(target, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0444)
		if err != nil {
			return false, ErrReviewInputInvalid
		}
		_, writeErr := file.Write(body)
		syncErr := file.Sync()
		closeErr := file.Close()
		if writeErr != nil || syncErr != nil || closeErr != nil {
			return false, ErrReviewInputInvalid
		}
	}
	return true, nil
}

func plainDirectory(path string) bool {
	info, err := os.Lstat(path)
	return err == nil && info.IsDir() && info.Mode()&os.ModeSymlink == 0
}

func verifiedReference(path string, size int64, digest string) ([]byte, error) {
	info, err := os.Lstat(path)
	if err != nil || !info.Mode().IsRegular() || info.Size() != size {
		return nil, ErrReviewInputInvalid
	}
	file, err := os.Open(path)
	if err != nil {
		return nil, ErrReviewInputInvalid
	}
	defer file.Close()
	opened, err := file.Stat()
	if err != nil || !os.SameFile(info, opened) {
		return nil, ErrReviewInputInvalid
	}
	body, err := io.ReadAll(io.LimitReader(file, size+1))
	sum := sha256.Sum256(body)
	if err != nil || int64(len(body)) != size || hex.EncodeToString(sum[:]) != digest {
		return nil, ErrReviewInputInvalid
	}
	return body, nil
}
