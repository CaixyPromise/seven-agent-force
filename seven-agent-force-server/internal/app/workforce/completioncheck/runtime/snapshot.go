package runtime

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"syscall"
)

// SnapshotDigest runs only over the Worker-created pre-execution snapshot.
// The command cannot supply or overwrite this identity.
func SnapshotDigest(ctx context.Context, source string) (string, error) {
	root, err := os.OpenRoot(source)
	if err != nil {
		return "", errors.New("CHECK_SNAPSHOT_IDENTITY_UNAVAILABLE")
	}
	defer root.Close()
	sum := sha256.New()
	encoder := json.NewEncoder(sum)
	err = fs.WalkDir(root.FS(), ".", func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if path == "." {
			return nil
		}
		if entry.Type()&os.ModeSymlink != 0 {
			return errors.New("link")
		}
		if entry.IsDir() {
			return encoder.Encode([]any{path, "directory"})
		}
		file, err := root.OpenFile(path, os.O_RDONLY|syscall.O_NOFOLLOW|syscall.O_NONBLOCK, 0)
		if err != nil {
			return err
		}
		defer file.Close()
		info, err := file.Stat()
		if err != nil || !info.Mode().IsRegular() {
			return errors.New("file")
		}
		digest := sha256.New()
		if _, err = io.Copy(digest, file); err != nil {
			return err
		}
		return encoder.Encode([]any{path, info.Mode().Perm() & 0111, info.Size(), hex.EncodeToString(digest.Sum(nil))})
	})
	if err != nil {
		return "", errors.New("CHECK_SNAPSHOT_IDENTITY_UNAVAILABLE")
	}
	return hex.EncodeToString(sum.Sum(nil)), nil
}

// Snapshot copies ordinary repository bytes into an empty private directory.
// Root-scoped opens forbid escapes even if an entry changes while copying;
// symlinks and special files fail closed. Git configuration/credentials and
// installed node_modules never enter the check environment.
func Snapshot(ctx context.Context, source, destination string) error {
	source, err := filepath.EvalSymlinks(source)
	if err != nil {
		return errors.New("CHECK_SOURCE_UNAVAILABLE")
	}
	destination, err = filepath.Abs(destination)
	if err != nil {
		return errors.New("CHECK_SNAPSHOT_INVALID")
	}
	if canonical, e := filepath.EvalSymlinks(destination); e == nil {
		destination = canonical
	} else {
		parent, e := filepath.EvalSymlinks(filepath.Dir(destination))
		if e != nil {
			return errors.New("CHECK_SNAPSHOT_INVALID")
		}
		destination = filepath.Join(parent, filepath.Base(destination))
	}
	rel, err := filepath.Rel(source, destination)
	if err != nil || rel == "." || (!strings.HasPrefix(rel, ".."+string(filepath.Separator)) && rel != "..") {
		return errors.New("CHECK_SNAPSHOT_OVERLAP")
	}
	if err = os.MkdirAll(destination, 0700); err != nil {
		return errors.New("CHECK_SNAPSHOT_UNAVAILABLE")
	}
	entries, err := os.ReadDir(destination)
	if err != nil || len(entries) != 0 {
		return errors.New("CHECK_SNAPSHOT_NOT_EMPTY")
	}
	root, err := os.OpenRoot(source)
	if err != nil {
		return errors.New("CHECK_SOURCE_UNAVAILABLE")
	}
	defer root.Close()
	var count int
	var total int64
	err = fs.WalkDir(root.FS(), ".", func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if path == "." {
			return nil
		}
		if entry.Name() == ".git" || entry.Name() == "node_modules" {
			if entry.IsDir() {
				return fs.SkipDir
			}
			return nil
		}
		count++
		if count > 50000 {
			return errors.New("limit")
		}
		target := filepath.Join(destination, filepath.FromSlash(path))
		if entry.Type()&os.ModeSymlink != 0 {
			return errors.New("link")
		}
		if entry.IsDir() {
			return os.Mkdir(target, 0755)
		}
		input, err := root.OpenFile(path, os.O_RDONLY|syscall.O_NOFOLLOW|syscall.O_NONBLOCK, 0)
		if err != nil {
			return err
		}
		defer input.Close()
		info, err := input.Stat()
		if err != nil || !info.Mode().IsRegular() || info.Size() > 64<<20 {
			return errors.New("file")
		}
		total += info.Size()
		if total > 512<<20 {
			return errors.New("limit")
		}
		output, err := os.OpenFile(target, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0644|info.Mode().Perm()&0111)
		if err != nil {
			return err
		}
		written, copyErr := io.Copy(output, io.LimitReader(input, info.Size()+1))
		closeErr := output.Close()
		if copyErr != nil || closeErr != nil || written != info.Size() {
			return errors.New("copy")
		}
		return nil
	})
	if err != nil {
		return errors.New("CHECK_SNAPSHOT_UNAVAILABLE")
	}
	return nil
}
