package runtime

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	processor "github.com/CaixyPromise/seven-agent/force/internal/app/workforce/resultprocessor/domain"
	"github.com/CaixyPromise/seven-agent/force/internal/app/workforce/resultprocessor/facade"
	"github.com/CaixyPromise/seven-agent/force/internal/app/workforce/task/domain"
	"io"
	"net"
	"os"
	"path/filepath"
	"reflect"
	"sync"
	"testing"
	"time"
)

func pendingFixture() (domain.WorkItem, domain.Attempt, supervisorStateSnapshot) {
	item := domain.WorkItem{TaskID: 1}
	item.ExecutionSnapshot.MaxEvaluationCycles = 3
	item.ExecutionSnapshot.ResultProcessorReleaseID = 9
	item.ExecutionSnapshot.ResultProcessorDigest = "processor"
	a := domain.Attempt{ID: 3, Generation: 1, OwnerEpoch: 2}
	state := supervisorStateSnapshot{State: "EVALUATION_PENDING_ACK", TaskID: 1, AttemptID: 3, Generation: 1, OwnerEpoch: 2, EvaluationCycleID: 5, EvaluationCycle: 1, SessionDigest: "session"}
	return item, a, state
}

// Exercises the real pending branch and persistent control transport. Drop the
// first ACK response only after receiving its request; no subprocess startup or
// sleep decides whether the injected failure is reached.
func TestPendingACKLostResponseReplaysServerWaitUser(t *testing.T) {
	testACKLostResponseReplaysServerWaitUser(t, false)
}

func TestReadyACKLostResponseReplaysServerWaitUser(t *testing.T) {
	testACKLostResponseReplaysServerWaitUser(t, true)
}

func TestDrainFailureBlocksTerminalACK(t *testing.T) {
	t.Run("pending", func(t *testing.T) { testACKLostResponseReplaysServerWaitUser(t, false, true) })
	t.Run("ready", func(t *testing.T) { testACKLostResponseReplaysServerWaitUser(t, true, true) })
}

func testACKLostResponseReplaysServerWaitUser(t *testing.T, initiallyReady bool, failDrain ...bool) {
	t.Helper()
	root := t.TempDir()
	item, a, state := pendingFixture()
	workspace := Workspace{}
	directive := waitDirective(t)
	if initiallyReady {
		state.State = "EVALUATION_READY"
		workspace.Repo = filepath.Join(root, "repo")
		workspace.Runtime = filepath.Join(root, "runtime")
		for _, p := range []string{workspace.Repo, filepath.Join(workspace.Runtime, "evaluations")} {
			if err := os.MkdirAll(p, 0700); err != nil {
				t.Fatal(err)
			}
		}
		if err := os.WriteFile(filepath.Join(workspace.Runtime, "evaluations", "5.result.json"), directive.Contract, 0600); err != nil {
			t.Fatal(err)
		}
	}
	var mu sync.Mutex
	var commands []string
	var acknowledgements []map[string]any
	var servers sync.WaitGroup
	dials := 0
	dial := func(context.Context, string) (executionTransport, error) {
		dials++
		client, server := net.Pipe()
		servers.Add(1)
		go func() {
			defer servers.Done()
			defer server.Close()
			decoder := json.NewDecoder(server)
			for {
				var frame struct {
					ID      string         `json:"id"`
					Request map[string]any `json:"request"`
				}
				if err := decoder.Decode(&frame); err != nil {
					return
				}
				command, _ := frame.Request["command"].(string)
				mu.Lock()
				commands = append(commands, command)
				loseResponse := false
				if command == "continue-evaluation" {
					acknowledgements = append(acknowledgements, frame.Request)
					loseResponse = len(acknowledgements) == 1
				}
				mu.Unlock()
				if loseResponse {
					return // Request received, response lost: the caller must observe EOF.
				}
				response := supervisorResponse{OK: true, State: state}
				if command == "run-evaluation" && initiallyReady {
					state.State = "EVALUATION_PENDING_ACK"
					response.State = state
					sum := sha256.Sum256(directive.Contract)
					response.Candidate = supervisorEvaluationCandidate{CycleID: 5, CycleNumber: 1, SessionDigest: "session", CandidateFile: "5.result.json", ContractSHA: hex.EncodeToString(sum[:])}
				} else if command != "state" && command != "evaluation-state" && command != "continue-evaluation" {
					response.OK = false
					response.ReasonCode = "UNEXPECTED_TEST_COMMAND"
				}
				if err := json.NewEncoder(server).Encode(map[string]any{"id": frame.ID, "response": response}); err != nil {
					return
				}
			}
		}()
		return client, nil
	}
	session := newExecutionSession(context.Background(), item.TaskID, a.ID, a.Generation, a.OwnerEpoch, dial)
	t.Cleanup(func() { _ = session.Close(); servers.Wait() })
	sessionCtx := context.WithValue(context.Background(), executionSessionKey{}, session)
	d := &Docker{}
	d.cfg.WorkspaceRoot = root
	reads := 0
	gate := RuntimeGate{
		DrainInteractions: func(context.Context) error { return nil },
		EvaluationReady:   func(int64, int) error { t.Fatal("re-emitted ready"); return nil },
		MaterializeEvaluation: func(int64, int) (facade.Materialization, error) {
			t.Fatal("re-materialized secrets in REVIEWING")
			return facade.Materialization{}, nil
		},
		CheckEvaluation: func(context.Context, int64) error { t.Fatal("reran check"); return nil },
		PersistEvaluation: func(facade.EvaluationCandidate) (facade.EvaluationDirective, error) {
			t.Fatal("resubmitted persisted decision")
			return facade.EvaluationDirective{}, nil
		},
		ReadEvaluation: func(_ context.Context, p facade.EvaluationReplayIdentity) (facade.EvaluationReplay, error) {
			reads++
			if p.CycleID != 5 || p.SessionDigest != "session" {
				t.Fatal("wrong lookup")
			}
			return facade.EvaluationReplay{Found: true, Directive: waitDirective(t)}, nil
		},
	}
	readyCalls, materializeCalls, checkCalls, persistCalls := 0, 0, 0, 0
	if initiallyReady {
		gate.EvaluationReady = func(int64, int) error { readyCalls++; return nil }
		gate.MaterializeEvaluation = func(int64, int) (facade.Materialization, error) {
			materializeCalls++
			return facade.Materialization{CycleID: 5}, nil
		}
		gate.CheckEvaluation = func(context.Context, int64) error { checkCalls++; return nil }
		gate.PersistEvaluation = func(c facade.EvaluationCandidate) (facade.EvaluationDirective, error) {
			persistCalls++
			if c.CycleID != 5 || c.ContractDigest != directive.ContractDigest {
				t.Fatal("candidate changed")
			}
			return directive, nil
		}
	}
	ctx, cancel := context.WithTimeout(sessionCtx, 10*time.Second)
	defer cancel()
	if len(failDrain) > 0 && failDrain[0] {
		failure := errors.New("spool persistence failed")
		gate.DrainInteractions = func(context.Context) error { return failure }
		if err := d.driveResultEvaluations(ctx, "local-double", item, a, workspace, gate); !errors.Is(err, failure) {
			t.Fatalf("drain failure hidden: %v", err)
		}
		mu.Lock()
		count := len(acknowledgements)
		mu.Unlock()
		if count != 0 {
			t.Fatal("terminal ACK escaped before spool persist")
		}
		if _, err := ReadAuthoritativeEvaluation(root, 1, a, 5, "WAIT_USER"); err != nil {
			t.Fatal("authority not persisted before drain", err)
		}
		return
	}
	if err := d.driveResultEvaluations(ctx, "local-double", item, a, workspace, gate); err != nil {
		t.Fatalf("single drive must recover lost ACK: %v", err)
	}
	wantReads := 2
	if initiallyReady {
		wantReads = 1
		if readyCalls != 1 || materializeCalls != 1 || checkCalls != 1 || persistCalls != 1 {
			t.Fatalf("evaluation reran: ready=%d materialize=%d check=%d persist=%d", readyCalls, materializeCalls, checkCalls, persistCalls)
		}
	}
	if reads != wantReads {
		t.Fatal("cached Server authority")
	}
	if _, err := ReadAuthoritativeEvaluation(root, 1, a, 5, "WAIT_USER"); err != nil {
		t.Fatal(err)
	}
	mu.Lock()
	defer mu.Unlock()
	want := []string{"state", "evaluation-state", "continue-evaluation", "state", "evaluation-state", "continue-evaluation"}
	if initiallyReady {
		want = []string{"state", "evaluation-state", "run-evaluation", "continue-evaluation", "state", "evaluation-state", "continue-evaluation"}
	}
	if dials != 2 || !reflect.DeepEqual(commands, want) {
		t.Fatalf("unexpected reconnect/replay: dials=%d commands=%v", dials, commands)
	}
	if len(acknowledgements) != 2 || !reflect.DeepEqual(acknowledgements[0], acknowledgements[1]) {
		t.Fatalf("ACK changed on replay: %v", acknowledgements)
	}
	ack := acknowledgements[0]
	if ack["cycleId"] != "5" || ack["decision"] != "WAIT_USER" || ack["directive"] != string(facade.DirectiveWaitForUser) {
		t.Fatalf("ACK did not carry Server WAIT_USER: %v", ack)
	}
}

func TestPendingACKMissingDecisionSubmitsExactPrivateCandidate(t *testing.T) {
	root := t.TempDir()
	item, a, state := pendingFixture()
	d := waitDirective(t)
	original := processor.ResultContract{Decision: "PASS", ReasonCode: "PROCESSOR_OK", Message: "original candidate"}
	body, _ := json.Marshal(original)
	digest, _ := processor.ContractDigest(original)
	candidate := facade.EvaluationCandidate{CycleID: 5, CycleNumber: 1, SessionDigest: "session", ProcessorReleaseID: 9, ProcessorDigest: "processor", Contract: body, ContractDigest: digest, EvaluationStartedAt: "2026-09-09T01:02:03Z", CandidateTreeDigest: "original-tree", FindingsDigest: "original-findings"}
	if err := persistPendingCandidate(root, 1, a, candidate); err != nil {
		t.Fatal(err)
	}
	steps := []string{}
	gate := RuntimeGate{
		ReadEvaluation: func(context.Context, facade.EvaluationReplayIdentity) (facade.EvaluationReplay, error) {
			steps = append(steps, "read")
			return facade.EvaluationReplay{}, nil
		},
		PersistEvaluation: func(got facade.EvaluationCandidate) (facade.EvaluationDirective, error) {
			steps = append(steps, "submit")
			if !reflect.DeepEqual(got, candidate) {
				t.Fatalf("candidate changed: %+v", got)
			}
			return d, nil
		},
	}
	for i := 0; i < 2; i++ {
		if _, err := recoverPendingEvaluation(context.Background(), root, item, a, state, gate); err != nil {
			t.Fatal(err)
		}
	}
	if !reflect.DeepEqual(steps, []string{"read", "submit", "read", "submit"}) {
		t.Fatal(steps)
	}
	p, _ := pendingCandidatePath(root, 1, a, 5)
	info, _ := os.Stat(p)
	if info.Mode().Perm() != 0600 {
		t.Fatal("candidate permissions")
	}
	candidate.SessionDigest = "changed"
	if err := persistPendingCandidate(root, 1, a, candidate); err == nil {
		t.Fatal("overwrote original candidate")
	}
}

func TestPendingACKFailsClosedWithoutFreshAuthorityOrSafeCandidate(t *testing.T) {
	for _, name := range []string{"read-error", "missing-candidate", "cancelled", "session", "owner"} {
		t.Run(name, func(t *testing.T) {
			item, a, state := pendingFixture()
			root := t.TempDir()
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			gate := RuntimeGate{ReadEvaluation: func(context.Context, facade.EvaluationReplayIdentity) (facade.EvaluationReplay, error) {
				if name == "read-error" {
					return facade.EvaluationReplay{}, errors.New("fenced")
				}
				return facade.EvaluationReplay{}, nil
			}, PersistEvaluation: func(facade.EvaluationCandidate) (facade.EvaluationDirective, error) {
				t.Fatal("submitted without safe candidate")
				return facade.EvaluationDirective{}, nil
			}}
			if name == "cancelled" {
				cancel()
			}
			if name == "owner" {
				state.OwnerEpoch++
			}
			if name == "session" {
				d := waitDirective(t)
				_ = persistPendingCandidate(root, 1, a, facade.EvaluationCandidate{CycleID: 5, CycleNumber: 1, SessionDigest: "other", Contract: d.Contract, ContractDigest: d.ContractDigest, ProcessorReleaseID: 9, ProcessorDigest: "processor"})
			}
			if _, err := recoverPendingEvaluation(ctx, root, item, a, state, gate); err == nil {
				t.Fatal("accepted unsafe recovery")
			}
		})
	}
}

func TestEvaluationACKDisconnectClassification(t *testing.T) {
	for _, tc := range []struct {
		name  string
		err   error
		retry bool
	}{
		{"eof", io.EOF, true}, {"truncated", io.ErrUnexpectedEOF, true},
		{"wrapped", fmt.Errorf("control: %w", io.EOF), true},
		{"business", errors.New("OWNER_REJECTED"), false},
		{"text-eof", errors.New("EOF"), false},
		{"deadline", context.DeadlineExceeded, false},
		{"cancel", context.Canceled, false}, {"nil", nil, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := retryableEvaluationACKDisconnect(context.Background(), tc.err); got != tc.retry {
				t.Fatalf("retry=%v", got)
			}
			ctx, cancel := context.WithCancel(context.Background())
			cancel()
			if retryableEvaluationACKDisconnect(ctx, tc.err) {
				t.Fatal("cancelled context retried")
			}
			ctx, done := context.WithDeadline(context.Background(), time.Now().Add(-time.Second))
			defer done()
			if retryableEvaluationACKDisconnect(ctx, tc.err) {
				t.Fatal("expired context retried")
			}
		})
	}
}

func TestPendingACKRecoveryKeepsFatalBoundaries(t *testing.T) {
	for _, kind := range []string{"business-reject", "reconnect-owner", "authority-reject", "deadline"} {
		t.Run(kind, func(t *testing.T) {
			item, a, state := pendingFixture()
			ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
			defer cancel()
			var servers sync.WaitGroup
			dials, acks, reads := 0, 0, 0
			session := newExecutionSession(ctx, item.TaskID, a.ID, a.Generation, a.OwnerEpoch, func(context.Context, string) (executionTransport, error) {
				dials++
				dialNumber := dials
				client, server := net.Pipe()
				servers.Add(1)
				go func() {
					defer servers.Done()
					defer server.Close()
					dec := json.NewDecoder(server)
					for {
						var frame struct {
							ID      string         `json:"id"`
							Request map[string]any `json:"request"`
						}
						if dec.Decode(&frame) != nil {
							return
						}
						response := supervisorResponse{OK: true, State: state}
						command := frame.Request["command"]
						if command == "state" && dialNumber == 2 && kind == "reconnect-owner" {
							response.State.OwnerEpoch++
						}
						if command == "continue-evaluation" {
							acks++
							if kind == "business-reject" {
								response.OK = false
								response.ReasonCode = "OWNER_REJECTED"
							} else if kind == "deadline" {
								<-ctx.Done()
								return
							} else {
								return
							}
						}
						if json.NewEncoder(server).Encode(map[string]any{"id": frame.ID, "response": response}) != nil {
							return
						}
					}
				}()
				return client, nil
			})
			t.Cleanup(func() { _ = session.Close(); servers.Wait() })
			gate := RuntimeGate{
				DrainInteractions: func(context.Context) error { return nil },
				EvaluationReady:   func(int64, int) error { t.Fatal("new ready"); return nil },
				MaterializeEvaluation: func(int64, int) (facade.Materialization, error) {
					t.Fatal("materialized")
					return facade.Materialization{}, nil
				},
				PersistEvaluation: func(facade.EvaluationCandidate) (facade.EvaluationDirective, error) {
					t.Fatal("persisted")
					return facade.EvaluationDirective{}, nil
				},
				ReadEvaluation: func(context.Context, facade.EvaluationReplayIdentity) (facade.EvaluationReplay, error) {
					reads++
					if kind == "authority-reject" && reads == 2 {
						return facade.EvaluationReplay{}, errors.New("AUTHORITY_REJECTED")
					}
					return facade.EvaluationReplay{Found: true, Directive: waitDirective(t)}, nil
				},
			}
			d := &Docker{}
			d.cfg.WorkspaceRoot = t.TempDir()
			err := d.driveResultEvaluations(context.WithValue(ctx, executionSessionKey{}, session), "pipe-only", item, a, Workspace{}, gate)
			_ = session.Close()
			servers.Wait()
			if err == nil {
				t.Fatal("fatal boundary accepted")
			}
			if kind == "deadline" {
				if !errors.Is(err, context.DeadlineExceeded) {
					t.Fatalf("deadline: %v", err)
				}
			} else if errors.Is(err, context.DeadlineExceeded) {
				t.Fatalf("fatal rejection swallowed until deadline: %v", err)
			}
			wantDials, wantReads := 1, 1
			if kind == "reconnect-owner" {
				wantDials = 2
			}
			if kind == "authority-reject" {
				wantDials, wantReads = 2, 2
			}
			if dials != wantDials || reads != wantReads || acks != 1 {
				t.Fatalf("dials=%d reads=%d ACKs=%d err=%v", dials, reads, acks, err)
			}
		})
	}
}
