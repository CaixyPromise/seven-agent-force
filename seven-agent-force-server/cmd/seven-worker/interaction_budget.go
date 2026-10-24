package main

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	attemptruntime "github.com/CaixyPromise/seven-agent/force/internal/app/workforce/attempt/runtime"
	task "github.com/CaixyPromise/seven-agent/force/internal/app/workforce/task/domain"
	wcp "github.com/CaixyPromise/seven-agent/force/internal/worker/checkpoint"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"syscall"
	"time"
)

// Baseline and runtime must share one ledger; a nested timeout would otherwise
// continue charging question waiting even while the Worker pauses its clock.
func withExecutionBudget(ctx context.Context, limit time.Duration, identity attemptruntime.BudgetIdentity, restore bool, confirmedActiveMillis ...int64) (context.Context, context.CancelFunc, error) {
	floor := int64(0)
	if len(confirmedActiveMillis) > 0 {
		floor = confirmedActiveMillis[0]
	}
	if floor < 0 {
		return nil, nil, attemptruntime.ErrBudgetRecovery
	}
	if !restore {
		return attemptruntime.ExecutionBudgetContextFromElapsed(ctx, limit, identity, time.Duration(floor)*time.Millisecond)
	}
	child, cancel, err := attemptruntime.ExecutionBudgetContext(ctx, limit, identity, true)
	if err != nil {
		return nil, nil, err
	}
	snapshot, err := attemptruntime.BudgetSnapshot(child)
	if err != nil || snapshot.ActiveElapsedMillis < floor {
		cancel()
		return nil, nil, attemptruntime.ErrBudgetRecovery
	}
	return child, cancel, nil
}

// Call only after the authenticated restore-key response AND encrypted archive
// verification both matched this manifest. It is not a container input parser.
func (w *worker) acceptVerifiedInteractionBudget(ctx context.Context, item task.WorkItem, attempt task.Attempt, input wcp.RestoreInput) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	r := item.ContinuationRestore
	m := input.Manifest
	if w.interactionBudgets == nil || r == nil || m.CheckpointIdentity != r.CheckpointIdentity || m.TaskID != item.TaskID || m.TaskTurnID != r.SourceTurnID || m.SnapshotDigest != item.TaskExecutionSnapshotDigest || m.SessionDigest != r.SessionLineageDigest || m.ExecutionBudget.TaskID != m.TaskID || m.ExecutionBudget.AttemptID != m.AttemptID || m.ExecutionBudget.Generation != m.Generation {
		return attemptruntime.ErrBudgetRecovery
	}
	if err := m.ExecutionBudget.Validate(); err != nil {
		return attemptruntime.ErrBudgetRecovery
	}
	b := input.ExecutionBudget
	if b.TaskID != item.TaskID || b.AttemptID != attempt.ID || b.Generation != attempt.Generation || b.OwnerEpoch != attempt.OwnerEpoch || b.ActiveLimitMillis != m.ExecutionBudget.ActiveLimitMillis || b.ActiveElapsedMillis != m.ExecutionBudget.ActiveElapsedMillis || b.ActiveElapsedMillis < item.ExecutionActiveMilliseconds || b.InteractionWaitingMillis != m.ExecutionBudget.InteractionWaitingMillis {
		return attemptruntime.ErrBudgetRecovery
	}
	limit := time.Duration(item.HarnessTimeoutSeconds) * time.Second
	s := attemptruntime.ExecutionBudgetSnapshot{Version: b.Version, BudgetIdentity: attemptruntime.BudgetIdentity{TaskID: item.TaskID, AttemptID: attempt.ID, Generation: attempt.Generation, OwnerEpoch: attempt.OwnerEpoch}, ActiveLimitMillis: b.ActiveLimitMillis, ActiveElapsedMillis: b.ActiveElapsedMillis, InteractionWaitingMillis: b.InteractionWaitingMillis, AbsoluteDeadlineUnixMillis: b.AbsoluteDeadlineUnixMillis}
	if err := attemptruntime.ValidateBudgetSnapshot(s, s.BudgetIdentity, limit, time.Now()); err != nil {
		return err
	}
	// Replaying the same verified restore must not overwrite locally consumed
	// active time. A damaged existing target record fails closed.
	if _, err := w.interactionBudgets.root.Lstat(budgetRecordName(s.BudgetIdentity)); err == nil {
		previous, err := w.interactionBudgets.Restore(ctx, s.BudgetIdentity)
		if err != nil {
			return err
		}
		if previous.ActiveLimitMillis != s.ActiveLimitMillis || previous.AbsoluteDeadlineUnixMillis != s.AbsoluteDeadlineUnixMillis || previous.InteractionWaitingMillis < s.InteractionWaitingMillis || previous.ActiveElapsedMillis < s.ActiveElapsedMillis {
			return attemptruntime.ErrBudgetRecovery
		}
		return nil
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return w.interactionBudgets.save(s)
}

// Directory and authentication key are Worker-private and never mounted into
// Attempts. Unobserved downtime on local restart is conservatively active time.
type privateInteractionBudgets struct {
	root *os.Root
	key  []byte
	now  func() time.Time
}
type privateBudgetRecord struct {
	Snapshot attemptruntime.ExecutionBudgetSnapshot `json:"snapshot"`
	SavedAt  int64                                  `json:"savedAt,string"`
	MAC      string                                 `json:"mac"`
}

func openPrivateInteractionBudgets(directory string, key []byte) (*privateInteractionBudgets, error) {
	if len(key) < 32 || !filepath.IsAbs(directory) {
		return nil, attemptruntime.ErrBudgetRecovery
	}
	resolved, err := filepath.EvalSymlinks(directory)
	if err != nil || resolved != filepath.Clean(directory) {
		return nil, attemptruntime.ErrBudgetRecovery
	}
	root, err := os.OpenRoot(directory)
	if err != nil {
		return nil, err
	}
	f, err := root.Open(".")
	if err != nil {
		root.Close()
		return nil, err
	}
	info, err := f.Stat()
	f.Close()
	if err != nil || !info.IsDir() || !budgetPrivateOwner(info) {
		root.Close()
		return nil, attemptruntime.ErrBudgetRecovery
	}
	return &privateInteractionBudgets{root: root, key: append([]byte(nil), key...), now: time.Now}, nil
}
func budgetRecordName(id attemptruntime.BudgetIdentity) string {
	return strconv.FormatInt(id.TaskID, 10) + "-" + strconv.FormatInt(id.AttemptID, 10) + "-" + strconv.FormatInt(id.Generation, 10) + "-" + strconv.FormatInt(id.OwnerEpoch, 10) + ".budget.json"
}
func (s *privateInteractionBudgets) sign(r privateBudgetRecord) string {
	r.MAC = ""
	body, _ := json.Marshal(r)
	h := hmac.New(sha256.New, s.key)
	_, _ = h.Write(body)
	return hex.EncodeToString(h.Sum(nil))
}
func (s *privateInteractionBudgets) Save(ctx context.Context) error {
	snapshot, err := attemptruntime.BudgetSnapshot(ctx)
	if err != nil {
		return err
	}
	return s.save(snapshot)
}
func (s *privateInteractionBudgets) save(snapshot attemptruntime.ExecutionBudgetSnapshot) error {
	now := s.now()
	if err := attemptruntime.ValidateBudgetSnapshot(snapshot, snapshot.BudgetIdentity, time.Duration(snapshot.ActiveLimitMillis)*time.Millisecond, now); err != nil {
		return err
	}
	r := privateBudgetRecord{Snapshot: snapshot, SavedAt: now.UnixMilli()}
	r.MAC = s.sign(r)
	body, err := json.Marshal(r)
	if err != nil {
		return err
	}
	var nonce [16]byte
	if _, err = rand.Read(nonce[:]); err != nil {
		return err
	}
	tmp := ".budget-" + hex.EncodeToString(nonce[:])
	f, err := s.root.OpenFile(tmp, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		return err
	}
	defer s.root.Remove(tmp)
	_, err = f.Write(body)
	if err == nil {
		err = f.Sync()
	}
	err = errors.Join(err, f.Close())
	if err != nil {
		return err
	}
	if err = s.root.Rename(tmp, budgetRecordName(snapshot.BudgetIdentity)); err != nil {
		return err
	}
	dir, err := s.root.Open(".")
	if err != nil {
		return err
	}
	defer dir.Close()
	return dir.Sync()
}

// Restore only accepts the exact local owner. Cross-owner/Worker migration is
// supplied separately through Server-verified checkpoint lineage.
func (s *privateInteractionBudgets) Restore(ctx context.Context, identity attemptruntime.BudgetIdentity) (attemptruntime.ExecutionBudgetSnapshot, error) {
	bad := attemptruntime.ExecutionBudgetSnapshot{}
	if err := ctx.Err(); err != nil {
		return bad, err
	}
	name := budgetRecordName(identity)
	info, err := s.root.Lstat(name)
	if err != nil || !info.Mode().IsRegular() || !budgetPrivateOwner(info) || info.Size() > 8192 {
		return bad, attemptruntime.ErrBudgetRecovery
	}
	f, err := s.root.Open(name)
	if err != nil {
		return bad, err
	}
	defer f.Close()
	opened, err := f.Stat()
	if err != nil || !os.SameFile(info, opened) {
		return bad, attemptruntime.ErrBudgetRecovery
	}
	body, err := io.ReadAll(io.LimitReader(f, 8193))
	if err != nil || len(body) > 8192 {
		return bad, attemptruntime.ErrBudgetRecovery
	}
	var r privateBudgetRecord
	if json.Unmarshal(body, &r) != nil || !hmac.Equal([]byte(r.MAC), []byte(s.sign(r))) || r.Snapshot.BudgetIdentity != identity {
		return bad, attemptruntime.ErrBudgetRecovery
	}
	now := s.now()
	delta := now.UnixMilli() - r.SavedAt
	if delta < 0 || (r.Snapshot.ActiveLimitMillis > 0 && (delta > r.Snapshot.ActiveLimitMillis || r.Snapshot.ActiveElapsedMillis > r.Snapshot.ActiveLimitMillis-delta)) {
		return bad, attemptruntime.ErrBudgetRecovery
	}
	r.Snapshot.ActiveElapsedMillis += delta
	if err := attemptruntime.ValidateBudgetSnapshot(r.Snapshot, identity, time.Duration(r.Snapshot.ActiveLimitMillis)*time.Millisecond, now); err != nil {
		return bad, err
	}
	return r.Snapshot, nil
}

// Called only after CommitReattach authorizes the exact old->new owner pair.
// Never search for a "latest" file or infer another Attempt's ledger.
func (s *privateInteractionBudgets) reattachProvider(source, target attemptruntime.BudgetIdentity) attemptruntime.BudgetRestoreProvider {
	return func(ctx context.Context, requested attemptruntime.BudgetIdentity) (attemptruntime.ExecutionBudgetSnapshot, error) {
		if s == nil || requested != target || source.TaskID != target.TaskID || source.AttemptID != target.AttemptID || source.Generation != target.Generation || source.OwnerEpoch <= 0 || target.OwnerEpoch <= source.OwnerEpoch {
			return attemptruntime.ExecutionBudgetSnapshot{}, attemptruntime.ErrBudgetRecovery
		}
		b, err := s.Restore(ctx, source)
		if err != nil {
			return b, err
		}
		b.BudgetIdentity = target
		return b, nil
	}
}

func initPrivateInteractionBudgets(directory string) (*privateInteractionBudgets, error) {
	if err := os.MkdirAll(directory, 0700); err != nil {
		return nil, err
	}
	resolved, err := filepath.EvalSymlinks(directory)
	if err != nil || resolved != filepath.Clean(directory) {
		return nil, attemptruntime.ErrBudgetRecovery
	}
	root, err := os.OpenRoot(directory)
	if err != nil {
		return nil, err
	}
	defer root.Close()
	dirCheck, err := root.Open(".")
	if err != nil {
		return nil, err
	}
	directoryInfo, err := dirCheck.Stat()
	dirCheck.Close()
	if err != nil || !budgetPrivateOwner(directoryInfo) {
		return nil, attemptruntime.ErrBudgetRecovery
	}
	info, err := root.Lstat("key")
	if errors.Is(err, os.ErrNotExist) {
		dir, err := root.Open(".")
		if err != nil {
			return nil, err
		}
		names, err := dir.Readdirnames(-1)
		dir.Close()
		if err != nil {
			return nil, err
		}
		if len(names) != 0 {
			return nil, attemptruntime.ErrBudgetRecovery
		}
		key := make([]byte, 32)
		if _, err := rand.Read(key); err != nil {
			return nil, err
		}
		defer clear(key)
		f, err := root.OpenFile("key", os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
		if err != nil {
			return nil, err
		}
		_, err = f.Write(key)
		if err == nil {
			err = f.Sync()
		}
		err = errors.Join(err, f.Close())
		if err != nil {
			return nil, err
		}
		dir, err = root.Open(".")
		if err != nil {
			return nil, err
		}
		err = dir.Sync()
		dir.Close()
		if err != nil {
			return nil, err
		}
		return openPrivateInteractionBudgets(directory, key)
	}
	if err != nil || !info.Mode().IsRegular() || !budgetPrivateOwner(info) || info.Size() != 32 {
		return nil, attemptruntime.ErrBudgetRecovery
	}
	f, err := root.Open("key")
	if err != nil {
		return nil, err
	}
	defer f.Close()
	opened, err := f.Stat()
	if err != nil || !os.SameFile(info, opened) {
		return nil, attemptruntime.ErrBudgetRecovery
	}
	key, err := io.ReadAll(io.LimitReader(f, 33))
	if err != nil || len(key) != 32 {
		return nil, attemptruntime.ErrBudgetRecovery
	}
	defer clear(key)
	return openPrivateInteractionBudgets(directory, key)
}

func budgetPrivateOwner(info os.FileInfo) bool {
	if info == nil || info.Mode().Perm()&0077 != 0 {
		return false
	}
	stat, ok := info.Sys().(*syscall.Stat_t)
	return ok && stat.Uid == uint32(os.Getuid())
}

func (s *privateInteractionBudgets) persist(ctx context.Context, cancelBudget context.CancelFunc) (func(), error) {
	if s == nil {
		return nil, attemptruntime.ErrBudgetRecovery
	}
	if err := s.Save(ctx); err != nil {
		return nil, err
	}
	loop, stop := context.WithCancel(ctx)
	done := make(chan struct{})
	go func() {
		defer close(done)
		ticker := time.NewTicker(250 * time.Millisecond)
		defer ticker.Stop()
		for {
			select {
			case <-loop.Done():
				return
			case <-ticker.C:
				if err := s.Save(ctx); err != nil {
					cancelBudget()
					return
				}
			}
		}
	}()
	return func() { stop(); <-done }, nil
}

func (w *worker) observeAcceptedBudgetQuestions(ctx context.Context, a task.Attempt, channel *attemptruntime.ExecutionChannel) func() {
	ctx, cancel := context.WithCancel(ctx)
	done := make(chan struct{})
	go func() {
		defer close(done)
		ticker := time.NewTicker(250 * time.Millisecond)
		defer ticker.Stop()
		for {
			probe, stop := context.WithTimeout(ctx, 500*time.Millisecond)
			state, err := channel.State(probe)
			if err == nil && state.Waiting {
				pending, err := w.control.PendingInteractions(probe, a, state.SessionGeneration)
				if err == nil {
					for _, q := range pending {
						if q.RoundKey == state.RoundKey {
							for _, key := range state.PendingInteractions {
								if key == q.LocalRequestKey {
									attemptruntime.ConfirmBudgetQuestion(ctx, state.RoundKey, state.SessionGeneration)
								}
							}
						}
					}
				}
			}
			stop()
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
			}
		}
	}()
	return func() { cancel(); <-done }
}
