package runtime

import (
	"context"
	"encoding/json"
	"errors"
	"net"
	"sync"
	"testing"
	"time"

	"github.com/CaixyPromise/seven-agent/force/internal/app/workforce/resultprocessor/facade"
)

func TestEvaluationStateReadTimeoutDoesNotSpendRemainingExecutionBudget(t *testing.T) {
	for _, expires := range []bool{false, true} {
		name := "read_recovers_without_restarting_agent"
		limit := 15 * time.Second
		if expires {
			name, limit = "parent_budget_still_expires", 200*time.Millisecond
		}
		t.Run(name, func(t *testing.T) {
			item, attempt, state := pendingFixture()
			state.State = "RUNNING"
			ctx, cancel := context.WithTimeout(context.Background(), limit)
			defer cancel()
			var servers sync.WaitGroup
			reads := 0
			session := newExecutionSession(ctx, item.TaskID, attempt.ID, attempt.Generation, attempt.OwnerEpoch, func(context.Context, string) (executionTransport, error) {
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
						if decoder.Decode(&frame) != nil {
							return
						}
						response := supervisorResponse{OK: true, State: state}
						switch frame.Request["command"] {
						case "state":
						case "evaluation-state":
							reads++
							if reads == 1 {
								// Lose a read response while the same connection and
								// execution remain healthy. No command is replayed.
								continue
							}
							response.State.State = "COMPLETED"
						default:
							t.Errorf("unexpected mutation or Agent restart: %v", frame.Request["command"])
							return
						}
						if json.NewEncoder(server).Encode(map[string]any{"id": frame.ID, "response": response}) != nil {
							return
						}
					}
				}()
				return client, nil
			})
			defer func() { _ = session.Close(); servers.Wait() }()
			drains := 0
			gate := RuntimeGate{
				DrainInteractions: func(context.Context) error { drains++; return nil },
				EvaluationReady:   func(int64, int) error { t.Fatal("unexpected evaluation"); return nil },
				MaterializeEvaluation: func(int64, int) (facade.Materialization, error) {
					t.Fatal("unexpected materialization")
					return facade.Materialization{}, nil
				},
				PersistEvaluation: func(facade.EvaluationCandidate) (facade.EvaluationDirective, error) {
					t.Fatal("unexpected write")
					return facade.EvaluationDirective{}, nil
				},
			}
			err := (&Docker{}).driveResultEvaluations(context.WithValue(ctx, executionSessionKey{}, session), "read-only-pipe", item, attempt, Workspace{}, gate)
			_ = session.Close()
			servers.Wait()
			if expires {
				if !errors.Is(err, context.DeadlineExceeded) || reads != 1 || drains != 0 {
					t.Fatalf("parent expiry: err=%v reads=%d drains=%d", err, reads, drains)
				}
			} else if err != nil || ctx.Err() != nil || reads != 2 || drains != 1 {
				t.Fatalf("premature execution failure: err=%v parent=%v reads=%d drains=%d", err, ctx.Err(), reads, drains)
			}
		})
	}
}
