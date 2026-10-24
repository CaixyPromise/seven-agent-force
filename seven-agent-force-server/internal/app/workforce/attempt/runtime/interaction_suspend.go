package runtime

import (
	"context"
	"encoding/json"
	"errors"
	"os/exec"
	"strconv"
	"time"
)

var ErrInteractionSuspended = errors.New("INTERACTION_SUSPENDED")
var ErrInteractionSuspendAborted = errors.New("INTERACTION_SUSPEND_ABORTED")

func (c *ExecutionChannel) DecideSuspension(ctx context.Context, prepared InteractionExecutionState, commit bool) error {
	if c == nil || c.session == nil || prepared.RoundKey == "" || prepared.SessionGeneration <= 0 || prepared.SuspendControlKey == "" {
		return errors.New("INTERACTION_SUSPEND_BINDING_INVALID")
	}
	command := "interaction-suspend-abort"
	if commit {
		command = "interaction-suspend-commit"
	}
	r := c.envelope(command)
	r["currentRound"], r["sessionGeneration"] = prepared.RoundKey, strconv.FormatInt(prepared.SessionGeneration, 10)
	r["payload"] = map[string]string{"suspendControlKey": prepared.SuspendControlKey}
	response, err := c.session.Call(ctx, c.container, r)
	if err != nil {
		return err
	}
	if !response.OK {
		return errors.New("INTERACTION_SUSPEND_DECISION_REJECTED")
	}
	return nil
}

func (c *ExecutionChannel) AwaitSuspension(ctx context.Context, prepared InteractionExecutionState) (InteractionCapture, error) {
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()
	for {
		state, err := c.State(ctx)
		if err != nil {
			return InteractionCapture{}, err
		}
		if state.RoundKey != prepared.RoundKey || state.SessionGeneration != prepared.SessionGeneration || state.SuspendControlKey != prepared.SuspendControlKey {
			return InteractionCapture{}, errors.New("INTERACTION_SUSPEND_BINDING_CHANGED")
		}
		if state.State == "INTERACTION_SUSPENDED" {
			return c.Capture(ctx)
		}
		if state.State != "PAUSING" {
			return InteractionCapture{}, errors.New("INTERACTION_SUSPEND_NOT_COMMITTED")
		}
		select {
		case <-ctx.Done():
			return InteractionCapture{}, ctx.Err()
		case <-ticker.C:
		}
	}
}

// Capture readiness is an observation only. WithStoppedQuiescentWorkspace
// independently freezes the admitted container before any host-side packing.
type InteractionCapture struct {
	Purpose           string `json:"purpose"`
	AttemptID         int64  `json:"attemptId,string"`
	CurrentRound      string `json:"currentRound"`
	SessionGeneration int64  `json:"sessionGeneration,string"`
	SessionDigest     string `json:"sessionDigest"`
	AdapterStopped    bool   `json:"adapterStopped"`
	QuiescenceHeld    bool   `json:"quiescenceHeld"`
}

func (c *ExecutionChannel) Capture(ctx context.Context) (InteractionCapture, error) {
	state, err := c.State(ctx)
	if err != nil {
		return InteractionCapture{}, err
	}
	req := c.envelope("interaction-capture")
	req["currentRound"] = state.RoundKey
	req["sessionGeneration"] = strconv.FormatInt(state.SessionGeneration, 10)
	r, err := c.session.Call(ctx, c.container, req)
	if err != nil {
		return InteractionCapture{}, err
	}
	v := r.Capture
	if !r.OK || v.Purpose != "INTERACTION_SUSPEND" || v.AttemptID != c.session.attemptID || v.CurrentRound != state.RoundKey || v.SessionGeneration != state.SessionGeneration || !v.AdapterStopped || !v.QuiescenceHeld || v.SessionDigest == "" {
		return InteractionCapture{}, errors.New("INTERACTION_CAPTURE_UNAVAILABLE")
	}
	return v, nil
}

func (c *ExecutionChannel) WithStoppedQuiescentWorkspace(ctx context.Context, attemptID int64, capture func() error) (err error) {
	if c == nil || c.session == nil || attemptID != c.session.attemptID || capture == nil {
		return errors.New("INTERACTION_CAPTURE_INVALID")
	}
	if _, err = c.Capture(ctx); err != nil {
		return err
	}
	if err = c.checkContainer(ctx, false); err != nil {
		return err
	}
	if err = exec.CommandContext(ctx, "docker", "pause", c.container).Run(); err != nil {
		return errors.New("INTERACTION_QUIESCENCE_FAILED")
	}
	defer func() {
		cleanup, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		if e := exec.CommandContext(cleanup, "docker", "unpause", c.container).Run(); e != nil {
			err = errors.Join(err, errors.New("INTERACTION_UNPAUSE_FAILED"))
		}
	}()
	if err = c.checkContainer(ctx, true); err != nil {
		return err
	}
	return capture()
}

func (c *ExecutionChannel) checkContainer(ctx context.Context, paused bool) error {
	body, err := exec.CommandContext(ctx, "docker", "inspect", c.container).Output()
	if err != nil {
		return errors.New("INTERACTION_CONTAINER_UNAVAILABLE")
	}
	var rows []struct {
		ID     string `json:"Id"`
		State  struct{ Running, Paused bool }
		Config struct{ Labels map[string]string }
	}
	if json.Unmarshal(body, &rows) != nil || len(rows) != 1 || rows[0].ID != c.container || !rows[0].State.Running || rows[0].State.Paused != paused {
		return errors.New("INTERACTION_CONTAINER_MISMATCH")
	}
	expected := map[string]string{"seven.managed": "true", "seven.task.id": strconv.FormatInt(c.session.taskID, 10), "seven.attempt.id": strconv.FormatInt(c.session.attemptID, 10), "seven.attempt.generation": strconv.FormatInt(c.session.generation, 10)}
	for k, v := range expected {
		if rows[0].Config.Labels[k] != v {
			return errors.New("INTERACTION_CONTAINER_MISMATCH")
		}
	}
	return nil
}
