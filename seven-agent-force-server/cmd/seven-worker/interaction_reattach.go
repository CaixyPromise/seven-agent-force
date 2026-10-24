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
	"os"
	"strconv"

	task "github.com/CaixyPromise/seven-agent/force/internal/app/workforce/task/domain"
	dispatch "github.com/CaixyPromise/seven-agent/force/internal/app/workforce/taskdispatch/domain"
	registry "github.com/CaixyPromise/seven-agent/force/internal/app/workforce/workerregistry/domain"
	"github.com/CaixyPromise/seven-agent/force/internal/worker/interactionspool"
)

// Constructed only in the authenticated CommitReattach/readback success path.
// Neither container JSON nor ordinary Register can supply this authorization.
type committedInteractionReattach struct {
	binding dispatch.InteractionReplayBinding
	attempt task.Attempt
	journal *privateInteractionBudgets
}

// This requests a Server decision; it does not authorize replay. The private
// spool, not a container capability flag, establishes that old receipts exist.
func (h *interactionTransportHub) needsInteractionReplay(ctx context.Context, c registry.RecoverableRuntime, e dispatch.RuntimeEvidence) (bool, error) {
	if h == nil {
		return false, errInteractionTransport
	}
	mode, start, err := dispatch.ResolveReattachMode(e.SupervisorState, e.HarnessPID)
	if err != nil {
		return false, errInteractionBinding
	}
	pending, err := h.spool.Pending(ctx)
	if err != nil {
		return false, err
	}
	wanted := false
	for _, r := range pending {
		i := r.Key.Identity
		if i.TaskID != c.TaskID || i.AttemptID != c.AttemptID {
			continue
		}
		if start || mode != dispatch.ReattachFollowRunning || e.ContainerID != c.ContainerID || e.ExecutionIdentityDigest != c.ExecutionIdentityDigest || e.OwnerEpoch != c.OwnerEpoch || i.Generation != c.Generation || i.OwnerEpoch > c.OwnerEpoch || i.SessionGeneration != e.AdapterSessionGeneration || i.CurrentRound != e.RoundKey {
			return false, errInteractionBinding
		}
		wanted = true
	}
	return wanted, nil
}

func committedInteractionReplay(result dispatch.ReattachResult, recovered registry.RecoverableRuntime, container string, attempt task.Attempt, journal *privateInteractionBudgets) (committedInteractionReattach, error) {
	bad := committedInteractionReattach{}
	b := result.InteractionReplayBinding
	if b == nil || result.StartNeeded || result.Mode != dispatch.ReattachFollowRunning || result.RuntimeState != dispatch.RuntimeRunning || journal == nil || journal.root == nil || len(journal.key) < 32 {
		return bad, errInteractionBinding
	}
	if b.ReconciliationID <= 0 || b.TaskID != attempt.TaskID || b.AttemptID != attempt.ID || b.Generation != attempt.Generation || b.NewOwnerEpoch != attempt.OwnerEpoch || b.OldOwnerEpoch <= 0 || b.OldOwnerEpoch+1 != b.NewOwnerEpoch || b.AdapterSessionGeneration <= 0 || b.RoundKey == "" || len(b.RoundKey) > 128 {
		return bad, errInteractionBinding
	}
	if b.TaskID != recovered.TaskID || b.AttemptID != recovered.AttemptID || b.Generation != recovered.Generation || b.OldOwnerEpoch != recovered.OwnerEpoch || b.ContainerID == "" || b.ContainerID != container || b.ContainerID != recovered.ContainerID || len(b.ExecutionIdentityDigest) != 64 || b.ExecutionIdentityDigest != recovered.ExecutionIdentityDigest {
		return bad, errInteractionBinding
	}
	if result.Contract.WorkItem.TaskID != b.TaskID || result.Contract.Attempt.ID != b.AttemptID || result.Contract.Attempt.Generation != b.Generation || result.Contract.OwnerEpoch != b.NewOwnerEpoch {
		return bad, errInteractionBinding
	}
	return committedInteractionReattach{*b, attempt, journal}, nil
}

type interactionReplayGrant struct {
	binding  dispatch.InteractionReplayBinding
	receipts map[interactionspool.Receipt]bool
}
type interactionReplayRecord struct {
	Version  int                               `json:"version"`
	Binding  dispatch.InteractionReplayBinding `json:"binding"`
	Receipts []interactionspool.Receipt        `json:"receipts"`
	MAC      string                            `json:"mac"`
}

// RegisterReattached leaves every original frame and receipt in place. The
// current owner may deliver only the exact committed predecessor receipts.
func (h *interactionTransportHub) RegisterReattached(ctx context.Context, proof committedInteractionReattach, channel interactionRuntimeChannel) (func(), error) {
	h.replayMu.Lock()
	defer h.replayMu.Unlock()
	grant, err := h.prepareInteractionReplay(ctx, proof, channel)
	if err != nil {
		return nil, err
	}
	return h.register(ctx, proof.attempt.TaskID, proof.attempt, channel, grant, 0, 0)
}

func sameReplayRuntime(a, b dispatch.InteractionReplayBinding) bool {
	return a.TaskID == b.TaskID && a.AttemptID == b.AttemptID && a.Generation == b.Generation && a.ContainerID == b.ContainerID && a.ExecutionIdentityDigest == b.ExecutionIdentityDigest && a.AdapterSessionGeneration == b.AdapterSessionGeneration && a.RoundKey == b.RoundKey
}

func (h *interactionTransportHub) prepareInteractionReplay(ctx context.Context, p committedInteractionReattach, channel interactionRuntimeChannel) (*interactionReplayGrant, error) {
	b := p.binding
	if channel == nil || p.journal == nil || b.ReconciliationID <= 0 || b.NewOwnerEpoch != p.attempt.OwnerEpoch || b.OldOwnerEpoch+1 != b.NewOwnerEpoch || b.OldOwnerEpoch <= 0 || b.TaskID != p.attempt.TaskID || b.AttemptID != p.attempt.ID || b.Generation != p.attempt.Generation {
		return nil, errInteractionBinding
	}
	state, err := channel.State(ctx)
	if err != nil {
		return nil, err
	}
	if state.State != "RUNNING" || state.RoundKey != b.RoundKey || state.SessionGeneration != b.AdapterSessionGeneration {
		return nil, errInteractionBinding
	}
	h.mu.Lock()
	busy := h.closed || h.firstErr != nil
	for key := range h.active {
		if key.task == b.TaskID && key.attempt == b.AttemptID {
			busy = true
		}
	}
	h.mu.Unlock()
	if busy {
		return nil, errInteractionBinding
	}
	name := strconv.FormatInt(b.TaskID, 10) + "-" + strconv.FormatInt(b.AttemptID, 10) + "-" + strconv.FormatInt(b.Generation, 10) + ".interaction-replay.json"
	previous, err := readInteractionReplay(p.journal, name)
	if err != nil {
		return nil, err
	}
	authorized := map[interactionspool.Receipt]bool{}
	if previous != nil {
		// A durable earlier binding can form a chain only when a fresh Server
		// commit explicitly adopts precisely that binding's current owner.
		prior := previous.Binding
		if prior.TaskID != b.TaskID || prior.AttemptID != b.AttemptID || prior.Generation != b.Generation || prior.ContainerID != b.ContainerID || prior.ExecutionIdentityDigest != b.ExecutionIdentityDigest || (prior != b && prior.NewOwnerEpoch != b.OldOwnerEpoch) {
			return nil, errInteractionBinding
		}
		// Completed earlier rounds do not prevent a later normal transfer. Their
		// receipts cannot gain authority in the new native session/round, however.
		if sameReplayRuntime(prior, b) {
			for _, r := range previous.Receipts {
				authorized[r] = true
			}
		}
	}
	pending, err := h.spool.Pending(ctx)
	if err != nil {
		return nil, err
	}
	record := interactionReplayRecord{Version: 1, Binding: b}
	grant := &interactionReplayGrant{binding: b, receipts: map[interactionspool.Receipt]bool{}}
	for _, r := range pending {
		i := r.Key.Identity
		if i.TaskID != b.TaskID || i.AttemptID != b.AttemptID {
			continue
		}
		if i.OwnerEpoch == b.NewOwnerEpoch {
			continue
		}
		if i.Generation != b.Generation || i.CurrentRound != b.RoundKey || i.SessionGeneration != b.AdapterSessionGeneration || (i.OwnerEpoch != b.OldOwnerEpoch && !authorized[r]) {
			return nil, errInteractionBinding
		}
		if len(record.Receipts) >= 4096 {
			return nil, errInteractionTransport
		}
		record.Receipts = append(record.Receipts, r)
		grant.receipts[r] = true
	}
	if err = ctx.Err(); err != nil {
		return nil, err
	}
	if err = writeInteractionReplay(p.journal, name, record); err != nil {
		return nil, err
	}
	return grant, nil
}

func (g *interactionReplayGrant) matchesNotification(occurrence string, frame []byte) (bool, error) {
	if g == nil {
		return false, nil
	}
	sum := sha256.Sum256(frame)
	for r := range g.receipts {
		if r.Key.Occurrence == occurrence {
			if r.SHA256 != hex.EncodeToString(sum[:]) {
				return false, errInteractionBinding
			}
			return true, nil
		}
	}
	return false, nil
}
func (g *interactionReplayGrant) entries(ctx context.Context, h *interactionTransportHub) ([]interactionspool.Entry, error) {
	var out []interactionspool.Entry
	seen := map[interactionspool.Identity]bool{}
	for r := range g.receipts {
		if seen[r.Key.Identity] {
			continue
		}
		seen[r.Key.Identity] = true
		entries, err := h.spool.Replay(ctx, r.Key.Identity)
		if err != nil {
			return nil, err
		}
		for _, entry := range entries {
			if !g.receipts[entry.Receipt] {
				return nil, errInteractionBinding
			}
			out = append(out, entry)
		}
	}
	return out, nil
}
func (g *interactionReplayGrant) requireComplete(ctx context.Context, h *interactionTransportHub) error {
	entries, err := g.entries(ctx, h)
	if err != nil {
		return err
	}
	if len(entries) != 0 {
		return errInteractionBinding
	}
	return nil
}

const maxInteractionReplayRecord = 2 << 20

func interactionReplayMAC(s *privateInteractionBudgets, r interactionReplayRecord) string {
	r.MAC = ""
	body, _ := json.Marshal(r)
	mac := hmac.New(sha256.New, s.key)
	_, _ = mac.Write(body)
	return hex.EncodeToString(mac.Sum(nil))
}
func readInteractionReplay(s *privateInteractionBudgets, name string) (*interactionReplayRecord, error) {
	info, err := s.root.Lstat(name)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil || !info.Mode().IsRegular() || !budgetPrivateOwner(info) || info.Size() > maxInteractionReplayRecord {
		return nil, errInteractionBinding
	}
	f, err := s.root.Open(name)
	if err != nil {
		return nil, errInteractionBinding
	}
	defer f.Close()
	opened, err := f.Stat()
	if err != nil || !os.SameFile(info, opened) {
		return nil, errInteractionBinding
	}
	body, err := io.ReadAll(io.LimitReader(f, maxInteractionReplayRecord+1))
	if err != nil || len(body) > maxInteractionReplayRecord {
		return nil, errInteractionBinding
	}
	var r interactionReplayRecord
	if json.Unmarshal(body, &r) != nil || r.Version != 1 || len(r.Receipts) > 4096 || !hmac.Equal([]byte(r.MAC), []byte(interactionReplayMAC(s, r))) {
		return nil, errInteractionBinding
	}
	return &r, nil
}
func writeInteractionReplay(s *privateInteractionBudgets, name string, r interactionReplayRecord) error {
	r.MAC = interactionReplayMAC(s, r)
	body, err := json.Marshal(r)
	if err != nil || len(body) > maxInteractionReplayRecord {
		return errInteractionBinding
	}
	var nonce [16]byte
	if _, err = rand.Read(nonce[:]); err != nil {
		return errInteractionTransport
	}
	tmp := ".interaction-replay-" + hex.EncodeToString(nonce[:])
	f, err := s.root.OpenFile(tmp, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		return errInteractionTransport
	}
	defer s.root.Remove(tmp)
	_, err = f.Write(body)
	if err == nil {
		err = f.Sync()
	}
	err = errors.Join(err, f.Close())
	if err != nil {
		return errInteractionTransport
	}
	if err = s.root.Rename(tmp, name); err != nil {
		return errInteractionTransport
	}
	dir, err := s.root.Open(".")
	if err != nil {
		return errInteractionTransport
	}
	defer dir.Close()
	if dir.Sync() != nil {
		return errInteractionTransport
	}
	return nil
}
