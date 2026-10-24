package main

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strconv"
	"syscall"

	runtime "github.com/CaixyPromise/seven-agent/force/internal/app/workforce/attempt/runtime"
	base "github.com/CaixyPromise/seven-agent/force/internal/app/workforce/resultprocessor/checkpoint"
)

var errColdRestoreRecovery = errors.New("INTERACTION_RESTORE_RECOVERY_EVIDENCE_REQUIRED")
var errColdRestoreConfirmationPending = errors.New("INTERACTION_RESTORE_CONFIRMATION_PENDING")

type coldRestoreIdentity struct {
	runtime.BudgetIdentity
	RestoreRunID  int64  `json:"restoreRunId,string"`
	CheckpointID  int64  `json:"checkpointId,string"`
	BindingSHA256 string `json:"bindingSha256"`
	ResultSHA256  string `json:"resultSha256"`
}
type coldRestoreRecord struct {
	Version     int                 `json:"version"`
	Identity    coldRestoreIdentity `json:"identity"`
	RepoSHA256  string              `json:"repoSha256"`
	StateSHA256 string              `json:"stateSha256"`
	MAC         string              `json:"mac"`
}

func coldRestoreRecordName(id coldRestoreIdentity) string {
	return strconv.FormatInt(id.TaskID, 10) + "-" + strconv.FormatInt(id.AttemptID, 10) + "-" + strconv.FormatInt(id.Generation, 10) + "-" + strconv.FormatInt(id.OwnerEpoch, 10) + ".restore.json"
}
func coldRestoreMAC(s *privateInteractionBudgets, r coldRestoreRecord) string {
	r.MAC = ""
	b, _ := json.Marshal(r)
	h := hmac.New(sha256.New, s.key)
	_, _ = h.Write(b)
	return hex.EncodeToString(h.Sum(nil))
}

// The journal lives alongside the Worker-private budget key, never inside the
// Attempt mount. A READY record means local install is durable, not Server ACK.
// Reentry verifies exact contents and re-confirms the same Server result; it
// never resets state, merges archives or reruns native-session relocation.
func recoverColdRestore(ctx context.Context, s *privateInteractionBudgets, id coldRestoreIdentity, w runtime.Workspace, install func() error, confirm func() error) error {
	if s == nil || s.root == nil || len(s.key) < 32 || id.TaskID <= 0 || id.AttemptID <= 0 || id.Generation <= 0 || id.OwnerEpoch <= 0 || id.RestoreRunID <= 0 || id.CheckpointID <= 0 || len(id.BindingSHA256) != 64 || len(id.ResultSHA256) != 64 {
		return errColdRestoreRecovery
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	name := coldRestoreRecordName(id)
	lock, err := s.root.OpenFile(name+".lock", os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		return errColdRestoreRecovery
	}
	defer lock.Close()
	li, err := lock.Stat()
	if err != nil || !li.Mode().IsRegular() || !budgetPrivateOwner(li) {
		return errColdRestoreRecovery
	}
	linkInfo, e := s.root.Lstat(name + ".lock")
	if e != nil || !linkInfo.Mode().IsRegular() || !os.SameFile(li, linkInfo) {
		return errColdRestoreRecovery
	}
	if err = syscall.Flock(int(lock.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		return errColdRestoreRecovery
	}
	defer syscall.Flock(int(lock.Fd()), syscall.LOCK_UN)
	info, err := s.root.Lstat(name)
	if err == nil {
		if !info.Mode().IsRegular() || !budgetPrivateOwner(info) || info.Size() > 4096 {
			return errColdRestoreRecovery
		}
		f, e := s.root.Open(name)
		if e != nil {
			return errColdRestoreRecovery
		}
		opened, e := f.Stat()
		if e != nil || !os.SameFile(info, opened) {
			f.Close()
			return errColdRestoreRecovery
		}
		body, e := io.ReadAll(io.LimitReader(f, 4097))
		f.Close()
		if e != nil || len(body) > 4096 {
			return errColdRestoreRecovery
		}
		var record coldRestoreRecord
		if json.Unmarshal(body, &record) != nil || record.Version != 1 || record.Identity != id || !hmac.Equal([]byte(record.MAC), []byte(coldRestoreMAC(s, record))) {
			return errColdRestoreRecovery
		}
		repo, e := coldRestoreTree(ctx, w.Repo, false)
		if e != nil || repo != record.RepoSHA256 {
			return errColdRestoreRecovery
		}
		state, e := coldRestoreTree(ctx, w.State, false)
		if e != nil || state != record.StateSHA256 {
			return errColdRestoreRecovery
		}
	} else if errors.Is(err, os.ErrNotExist) {
		// Unknown/partially restored contents are evidence, not scratch space.
		for _, path := range []string{w.Repo, w.State} {
			fi, e := os.Lstat(path)
			if errors.Is(e, os.ErrNotExist) {
				continue
			}
			if e != nil || !fi.IsDir() || fi.Mode()&os.ModeSymlink != 0 {
				return errColdRestoreRecovery
			}
			entries, e := os.ReadDir(path)
			if e != nil || len(entries) != 0 {
				return errColdRestoreRecovery
			}
		}
		if err = install(); err != nil {
			return err
		}
		repo, e := coldRestoreTree(ctx, w.Repo, true)
		if e != nil {
			return e
		}
		state, e := coldRestoreTree(ctx, w.State, true)
		if e != nil {
			return e
		}
		r := coldRestoreRecord{Version: 1, Identity: id, RepoSHA256: repo, StateSHA256: state}
		r.MAC = coldRestoreMAC(s, r)
		body, _ := json.Marshal(r)
		var nonce [16]byte
		if _, err = rand.Read(nonce[:]); err != nil {
			return errColdRestoreRecovery
		}
		tmp := ".restore-" + hex.EncodeToString(nonce[:])
		f, e := s.root.OpenFile(tmp, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
		if e != nil {
			return errColdRestoreRecovery
		}
		defer s.root.Remove(tmp)
		_, err = f.Write(body)
		if err == nil {
			err = f.Sync()
		}
		err = errors.Join(err, f.Close())
		if err != nil {
			return errColdRestoreRecovery
		}
		if err = s.root.Rename(tmp, name); err != nil {
			return errColdRestoreRecovery
		}
		dir, e := s.root.Open(".")
		if e != nil {
			return errColdRestoreRecovery
		}
		err = dir.Sync()
		dir.Close()
		if err != nil {
			return errColdRestoreRecovery
		}
	} else {
		return errColdRestoreRecovery
	}
	if err = ctx.Err(); err != nil {
		return err
	}
	return confirm()
}

// Hash every installed path/content (including .git) with bounded traversal;
// reject links/special files and races rather than following attacker paths.
func coldRestoreTree(ctx context.Context, path string, durable bool) (string, error) {
	resolved, err := filepath.EvalSymlinks(path)
	if err != nil || resolved != filepath.Clean(path) {
		return "", errColdRestoreRecovery
	}
	root, err := os.OpenRoot(path)
	if err != nil {
		return "", errColdRestoreRecovery
	}
	defer root.Close()
	h := sha256.New()
	files := 0
	var total int64
	var dirs []string
	err = fs.WalkDir(root.FS(), ".", func(name string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return errColdRestoreRecovery
		}
		if e := ctx.Err(); e != nil {
			return e
		}
		files++
		if files > base.MaxArchiveFiles {
			return errColdRestoreRecovery
		}
		info, e := entry.Info()
		if e != nil || info.Mode()&os.ModeSymlink != 0 {
			return errColdRestoreRecovery
		}
		mode := uint32(info.Mode().Perm())
		if name == "." {
			mode = 0
		} // PrepareAttempt re-secures only the State root on entry.
		meta, _ := json.Marshal([]any{name, mode, info.IsDir()})
		_, _ = h.Write(meta)
		_, _ = h.Write([]byte{0})
		if info.IsDir() {
			dirs = append(dirs, name)
			return nil
		}
		if !info.Mode().IsRegular() || info.Size() > base.MaxArchiveBytes {
			return errColdRestoreRecovery
		}
		if st, ok := info.Sys().(*syscall.Stat_t); !ok || st.Nlink != 1 {
			return errColdRestoreRecovery
		}
		total += info.Size()
		if total > base.HardMaxArchiveBytes {
			return errColdRestoreRecovery
		}
		f, e := root.Open(name)
		if e != nil {
			return errColdRestoreRecovery
		}
		defer f.Close()
		opened, e := f.Stat()
		if e != nil || !os.SameFile(info, opened) {
			return errColdRestoreRecovery
		}
		content := sha256.New()
		n, e := io.Copy(content, io.LimitReader(coldRestoreReader{ctx, f}, info.Size()+1))
		if e != nil || n != info.Size() {
			return errColdRestoreRecovery
		}
		after, e := f.Stat()
		if e != nil || after.Size() != info.Size() || !after.ModTime().Equal(info.ModTime()) {
			return errColdRestoreRecovery
		}
		if durable {
			if e = f.Sync(); e != nil {
				return errColdRestoreRecovery
			}
		}
		_, _ = h.Write(content.Sum(nil))
		return nil
	})
	if err != nil {
		return "", err
	}
	if durable {
		for i := len(dirs) - 1; i >= 0; i-- {
			f, e := root.Open(dirs[i])
			if e != nil {
				return "", errColdRestoreRecovery
			}
			e = f.Sync()
			f.Close()
			if e != nil {
				return "", errColdRestoreRecovery
			}
		}
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

type coldRestoreReader struct {
	ctx    context.Context
	reader io.Reader
}

func (r coldRestoreReader) Read(p []byte) (int, error) {
	if err := r.ctx.Err(); err != nil {
		return 0, err
	}
	return r.reader.Read(p)
}
