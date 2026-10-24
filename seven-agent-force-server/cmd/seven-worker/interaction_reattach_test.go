package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	runtime "github.com/CaixyPromise/seven-agent/force/internal/app/workforce/attempt/runtime"
	task "github.com/CaixyPromise/seven-agent/force/internal/app/workforce/task/domain"
	dispatch "github.com/CaixyPromise/seven-agent/force/internal/app/workforce/taskdispatch/domain"
	registry "github.com/CaixyPromise/seven-agent/force/internal/app/workforce/workerregistry/domain"
	"github.com/CaixyPromise/seven-agent/force/internal/worker/control"
	"github.com/CaixyPromise/seven-agent/force/internal/worker/interactionspool"
)

func replayFixture(t *testing.T) (committedInteractionReattach, dispatch.ReattachResult, registry.RecoverableRuntime) {
	t.Helper()
	dir, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if err = os.Chmod(dir, 0700); err != nil {
		t.Fatal(err)
	}
	j, err := openPrivateInteractionBudgets(dir, make([]byte, 32))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { j.root.Close() })
	a := pumpAttempt
	a.OwnerEpoch++
	b := dispatch.InteractionReplayBinding{ReconciliationID: 8, TaskID: 1, AttemptID: 2, Generation: 3, OldOwnerEpoch: 4, NewOwnerEpoch: 5, ContainerID: "same-container", ExecutionIdentityDigest: strings.Repeat("a", 64), AdapterSessionGeneration: 1, RoundKey: "round:1"}
	r := dispatch.ReattachResult{RuntimeState: dispatch.RuntimeRunning, Mode: dispatch.ReattachFollowRunning, InteractionReplayBinding: &b, Contract: dispatch.ExecutionContract{WorkItem: task.WorkItem{TaskID: 1}, Attempt: a, OwnerEpoch: 5}}
	c := registry.RecoverableRuntime{TaskID: 1, AttemptID: 2, Generation: 3, OwnerEpoch: 4, ContainerID: b.ContainerID, ExecutionIdentityDigest: b.ExecutionIdentityDigest}
	p, err := committedInteractionReplay(r, c, b.ContainerID, a, j)
	if err != nil {
		t.Fatal(err)
	}
	return p, r, c
}

type replayTestControl struct {
	pumpTestControl
	expectedOwner int64
	t             *testing.T
}

func (c replayTestControl) PublishInteraction(ctx context.Context, a task.Attempt, g int64, f json.RawMessage) (control.InteractionPublication, error) {
	if a.OwnerEpoch != c.expectedOwner || a.ID != 2 || a.Generation != 3 || g != 1 {
		c.t.Error("publication did not use committed current authority")
	}
	return c.publish(ctx, f)
}

func TestInteractionReattachPublishesOriginalReceiptAfterCommit(t *testing.T) {
	proof, _, candidate := replayFixture(t)
	frame := pumpQuestion(t)
	path := pumpPath(t)
	var h *interactionTransportHub
	called := make(chan struct{}, 1)
	client := replayTestControl{expectedOwner: 5, t: t, pumpTestControl: pumpTestControl{publish: func(_ context.Context, raw json.RawMessage) (control.InteractionPublication, error) {
		if !bytes.Equal(raw, frame) {
			t.Error("original frame altered")
		}
		pending, err := h.spool.Pending(context.Background())
		if err != nil || len(pending) != 1 || pending[0].Key.Identity != pumpIdentity {
			t.Error("old receipt moved or removed before Server ACK")
		}
		select {
		case called <- struct{}{}:
		default:
		}
		return pumpReceipt(raw), nil
	}}}
	var err error
	h, err = newInteractionTransportHub(context.Background(), path, client)
	if err != nil {
		t.Fatal(err)
	}
	defer h.Close()
	evidence := dispatch.RuntimeEvidence{ContainerID: candidate.ContainerID, ExecutionIdentityDigest: candidate.ExecutionIdentityDigest, OwnerEpoch: 4, SupervisorState: "RUNNING", HarnessPID: 42, AdapterSessionGeneration: 1, RoundKey: "round:1"}
	if wanted, err := h.needsInteractionReplay(context.Background(), candidate, evidence); err != nil || wanted {
		t.Fatal("ordinary reattach requested replay without pending spool", err)
	}
	if _, err = h.spool.Put(context.Background(), interactionspool.Key{Identity: pumpIdentity, Occurrence: "1"}, frame); err != nil {
		t.Fatal(err)
	}
	if wanted, err := h.needsInteractionReplay(context.Background(), candidate, evidence); err != nil || !wanted {
		t.Fatal("pending old receipts not requested", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	// Channel has no start method and any unexpected Send fails the test.
	cleanup, err := h.RegisterReattached(ctx, proof, pumpTestChannel{})
	if err != nil {
		t.Fatal(err)
	}
	select {
	case <-called:
	case <-time.After(time.Second):
		t.Fatal("committed old receipt was not replayed")
	}
	deadline := time.Now().Add(time.Second)
	for pendingCount(t, h) != 0 && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	if pendingCount(t, h) != 0 {
		t.Fatal("verified ACK did not remove original receipt")
	}
	cancel()
	cleanup()
}

func TestInteractionReattachLostACKAndJournalRestartRetainOldFile(t *testing.T) {
	proof, result, candidate := replayFixture(t)
	path := pumpPath(t)
	frame := pumpQuestion(t)
	committed := false
	businessWrites := 0
	calls := 0
	client := pumpTestControl{publish: func(_ context.Context, raw json.RawMessage) (control.InteractionPublication, error) {
		calls++
		if !bytes.Equal(raw, frame) {
			t.Error("replay changed bytes")
		}
		if !committed {
			committed = true
			businessWrites++
			return control.InteractionPublication{}, io.EOF
		}
		return pumpReceipt(raw), nil
	}}
	h := pumpHub(t, path, client)
	r, err := h.spool.Put(context.Background(), interactionspool.Key{Identity: pumpIdentity, Occurrence: "1"}, frame)
	if err != nil {
		t.Fatal(err)
	}
	g, err := h.prepareInteractionReplay(context.Background(), proof, pumpTestChannel{})
	if err != nil {
		t.Fatal(err)
	}
	// Crash after the binding is durable but before the first HTTP request.
	h.Close()
	h = pumpHub(t, path, client)
	g, err = h.prepareInteractionReplay(context.Background(), proof, pumpTestChannel{})
	if err != nil {
		t.Fatal(err)
	}
	p := pumpFor(h, pumpTestChannel{})
	p.attempt = proof.attempt
	p.key.owner = 5
	p.replay = g
	if err = p.step(context.Background()); !errors.Is(err, errInteractionRetry) {
		t.Fatal("expected lost Server response", err)
	}
	pending, _ := h.spool.Pending(context.Background())
	if len(pending) != 1 || pending[0] != r {
		t.Fatal("original receipt lost or duplicated")
	}
	h.Close()
	// Reopen the spool and private journal after a committed-binding readback.
	name := proof.journal.root.Name()
	proof.journal.root.Close()
	proof.journal, err = openPrivateInteractionBudgets(name, make([]byte, 32))
	if err != nil {
		t.Fatal(err)
	}
	defer proof.journal.root.Close()
	h = pumpHub(t, path, client)
	// A second successful Server transfer must explicitly chain the prior
	// journal's current owner. The original owner-4 frame is never copied.
	proof.binding.ReconciliationID++
	proof.binding.OldOwnerEpoch = 5
	proof.binding.NewOwnerEpoch = 6
	proof.attempt.OwnerEpoch = 6
	result.InteractionReplayBinding = &proof.binding
	result.Contract.OwnerEpoch = 6
	result.Contract.Attempt = proof.attempt
	candidate.OwnerEpoch = 5
	proof, err = committedInteractionReplay(result, candidate, "same-container", proof.attempt, proof.journal)
	if err != nil {
		t.Fatal(err)
	}
	g, err = h.prepareInteractionReplay(context.Background(), proof, pumpTestChannel{})
	if err != nil {
		t.Fatal(err)
	}
	p = pumpFor(h, pumpTestChannel{})
	p.attempt = proof.attempt
	p.key.owner = 6
	p.replay = g
	if err = p.step(context.Background()); err != nil {
		t.Fatal(err)
	}
	if businessWrites != 1 || calls != 2 || pendingCount(t, h) != 0 {
		t.Fatal("lost ACK replay did not converge exactly once")
	}
}

func TestInteractionReattachRejectsUncommittedOrChangedBinding(t *testing.T) {
	for _, kind := range []string{"no-commit", "start-needed", "container", "execution", "owner", "session", "round", "exited", "ordinary-register", "journal-tamper", "wrong-digest"} {
		t.Run(kind, func(t *testing.T) {
			proof, result, candidate := replayFixture(t)
			frame := pumpQuestion(t)
			published := false
			h := pumpHub(t, pumpPath(t), pumpTestControl{publish: func(context.Context, json.RawMessage) (control.InteractionPublication, error) {
				published = true
				return pumpReceipt(frame), nil
			}})
			original, err := h.spool.Put(context.Background(), interactionspool.Key{Identity: pumpIdentity, Occurrence: "1"}, frame)
			if err != nil {
				t.Fatal(err)
			}
			channel := pumpTestChannel{}
			if kind == "journal-tamper" {
				if _, err = h.prepareInteractionReplay(context.Background(), proof, channel); err != nil {
					t.Fatal(err)
				}
				if err = os.WriteFile(filepath.Join(proof.journal.root.Name(), "1-2-3.interaction-replay.json"), []byte(`{}`), 0600); err != nil {
					t.Fatal(err)
				}
			}
			switch kind {
			case "no-commit":
				result.InteractionReplayBinding = nil
			case "start-needed":
				result.StartNeeded = true
			case "container":
				candidate.ContainerID = "other"
			case "execution":
				candidate.ExecutionIdentityDigest = strings.Repeat("b", 64)
			case "owner":
				candidate.OwnerEpoch++
			case "session", "round", "exited":
				channel.state = func(context.Context) (runtime.InteractionExecutionState, error) {
					s := runtime.InteractionExecutionState{State: "RUNNING", RoundKey: "round:1", SessionGeneration: 1}
					if kind == "session" {
						s.SessionGeneration = 2
					}
					if kind == "round" {
						s.RoundKey = "round:2"
					}
					if kind == "exited" {
						s.State = "SUCCEEDED"
					}
					return s, nil
				}
			}
			if kind == "wrong-digest" {
				h.client = pumpTestControl{publish: func(context.Context, json.RawMessage) (control.InteractionPublication, error) {
					return control.InteractionPublication{FrameSHA256: strings.Repeat("0", 64)}, nil
				}}
				grant, e := h.prepareInteractionReplay(context.Background(), proof, channel)
				if e != nil {
					t.Fatal(e)
				}
				p := pumpFor(h, channel)
				p.attempt = proof.attempt
				p.key.owner = 5
				p.replay = grant
				if err = p.step(context.Background()); !errors.Is(err, errInteractionBinding) {
					t.Fatal("wrong digest allowed", err)
				}
			} else if kind == "ordinary-register" {
				p := pumpFor(h, channel)
				p.attempt = proof.attempt
				p.key.owner = 5
				if err = p.step(context.Background()); err != nil {
					t.Fatal(err)
				}
			} else {
				proof, err = committedInteractionReplay(result, candidate, "same-container", proof.attempt, proof.journal)
				if err == nil {
					_, err = h.prepareInteractionReplay(context.Background(), proof, channel)
				}
				if err == nil {
					t.Fatal("invalid replay binding accepted")
				}
			}
			pending, _ := h.spool.Pending(context.Background())
			if published || len(pending) != 1 || pending[0] != original {
				t.Fatal("unauthorized publication or original receipt changed")
			}
		})
	}
}
