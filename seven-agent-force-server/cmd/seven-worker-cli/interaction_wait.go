package main

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"strconv"
	"time"

	"github.com/CaixyPromise/seven-agent/force/internal/worker/agentprotocol"
)

type interactionTimer interface {
	C() <-chan time.Time
	Stop() bool
}
type interactionClock interface {
	Now() time.Time
	NewTimer(time.Duration) interactionTimer
}
type realInteractionClock struct{}
type realInteractionTimer struct{ *time.Timer }

func (realInteractionClock) Now() time.Time { return time.Now() }
func (realInteractionClock) NewTimer(d time.Duration) interactionTimer {
	return realInteractionTimer{time.NewTimer(d)}
}
func (t realInteractionTimer) C() <-chan time.Time { return t.Timer.C }

// Image/operator configuration only; no Task field can extend a wait budget.
func (state *supervisorState) configureInteractionWait() error {
	limit := 10 * time.Minute
	if raw := os.Getenv("SEVEN_INTERACTION_HOT_WAIT"); raw != "" {
		value, err := time.ParseDuration(raw)
		if err != nil || value <= 0 || value > 24*time.Hour {
			return errors.New("SUPERVISOR_HOT_WAIT_CONFIG_INVALID")
		}
		limit = value
	}
	state.interaction.clock = realInteractionClock{}
	state.interaction.hotWaitLimit = limit
	return nil
}

// All accounting writes run under state.mu; snapshots calculate a nonmutating
// delta so frequent polling neither advances nor loses time.
func (state *supervisorState) accountInteractionTime(now time.Time) {
	i := &state.interaction
	if !i.accountedAt.IsZero() && i.active && !i.pausing {
		delta := now.Sub(i.accountedAt)
		if delta < 0 {
			delta = 0
		}
		if !i.waitingSince.IsZero() {
			i.waitingElapsed += delta
		} else {
			i.activeElapsed += delta
		}
	}
	i.accountedAt = now
}
func (state *supervisorState) interactionTimes() (active, waiting time.Duration) {
	i := &state.interaction
	active, waiting = i.activeElapsed, i.waitingElapsed
	if i.clock != nil && !i.accountedAt.IsZero() && i.active && !i.pausing {
		delta := i.clock.Now().Sub(i.accountedAt)
		if delta < 0 {
			delta = 0
		}
		if !i.waitingSince.IsZero() {
			waiting += delta
		} else {
			active += delta
		}
	}
	return
}
func (state *supervisorState) refreshInteractionWaiting() {
	i := &state.interaction
	if i.clock == nil {
		return
	}
	now := i.clock.Now()
	state.accountInteractionTime(now)
	waiting := false
	for _, pending := range i.pending {
		if pending.request.IsBlocking && !pending.resolved && pending.accepted == nil {
			waiting = true
			break
		}
	}
	if !waiting {
		for _, pending := range i.pendingTools {
			if !pending.resolved {
				waiting = true
				break
			}
		}
	}
	if waiting && i.active && !i.pausing {
		if i.waitingSince.IsZero() {
			i.waitingSince = now
		}
	} else {
		i.waitingSince = time.Time{}
		if !waiting {
			i.waitAborted = false
		}
	}
	if i.changed != nil {
		select {
		case i.changed <- struct{}{}:
		default:
		}
	}
}

// Prepare fences new input but deliberately leaves native execution and the
// unresolved ask alive until the trusted Worker resolves Server reservation.
func (state *supervisorState) prepareInteractionSuspension(reason string) {
	state.enterInteractionPausing(reason)
	i := &state.interaction
	i.prepareSequence++
	i.suspendControlKey = "suspend:" + strconv.FormatInt(i.sessionGeneration, 10) + ":" + strconv.FormatInt(i.prepareSequence, 10)
	i.suspendDecision = ""
	state.State = "PAUSING_PREPARE"
}
func (state *supervisorState) enterInteractionPausing(reason string) {
	i := &state.interaction
	if i.clock != nil {
		state.accountInteractionTime(i.clock.Now())
	}
	i.pausing = true
	i.suspendReason = reason
	state.State = "PAUSING"
	if i.changed != nil {
		select {
		case i.changed <- struct{}{}:
		default:
		}
	}
}

// The parent owns absolute resource limits and the trusted active deadline.
// This timer only bounds hot question waiting and requests a capability-gated
// native safe point. Unsupported adapters remain PAUSING for parent handling.
func (state *supervisorState) monitorInteractionWait(ctx context.Context, round string, process *agentprotocol.Process) {
	for {
		state.mu.Lock()
		i := &state.interaction
		if i.currentRound != round || !i.active || i.pausing {
			state.mu.Unlock()
			return
		}
		changed, done := i.changed, i.roundDone
		var timer interactionTimer
		if !i.waitingSince.IsZero() && !i.waitAborted {
			remaining := i.hotWaitLimit - i.clock.Now().Sub(i.waitingSince)
			if remaining <= 0 {
				state.prepareInteractionSuspension("HOT_WAIT_EXPIRED")
				state.mu.Unlock()
				return
			}
			timer = i.clock.NewTimer(remaining)
		}
		state.mu.Unlock()
		var tick <-chan time.Time
		if timer != nil {
			tick = timer.C()
		}
		select {
		case <-ctx.Done():
			if timer != nil {
				timer.Stop()
			}
			return
		case <-done:
			if timer != nil {
				timer.Stop()
			}
			return
		case <-changed:
		case <-tick:
		}
		if timer != nil {
			timer.Stop()
		}
	}
}

func (state *supervisorState) interactionCaptureSnapshot() map[string]any {
	i := &state.interaction
	return map[string]any{"purpose": "INTERACTION_SUSPEND", "attemptId": strconv.FormatInt(state.AttemptID, 10), "currentRound": i.currentRound, "roundKey": i.currentRound, "sessionGeneration": strconv.FormatInt(i.sessionGeneration, 10), "adapterSessionGeneration": strconv.FormatInt(i.sessionGeneration, 10), "sessionDigest": state.SessionDigest, "suspensionReferenceDigest": state.SessionDigest, "sessionArchiveFile": "session.json", "adapterStopped": i.adapterStopped, "quiescenceHeld": i.quiescenceHeld, "suspendControlKey": i.suspendControlKey, "captureReadyAt": i.captureReadyAt}
}

func (state *supervisorState) closeLocalQuestionWaits() {
	for _, pending := range state.interaction.pending {
		if pending.answer != nil && !pending.closed {
			close(pending.answer)
			pending.closed = true
		}
	}
}

// Held quiescence is a coordination promise by this Supervisor, not proof that
// hostile detached descendants cannot mutate files. The trusted Worker must
// hold its Docker stopped/paused guard throughout checkpoint capture/readback.
func (state *supervisorState) holdInteractionSuspension(ctx context.Context, process *agentprotocol.Process, result agentprotocol.NativeResult) error {
	state.mu.Lock()
	if !state.interaction.pausing {
		state.enterInteractionPausing("NATIVE_SUSPENDED")
	}
	state.closeLocalQuestionWaits()
	state.mu.Unlock()
	process.Close()
	state.mu.Lock()
	i := &state.interaction
	i.adapterStopped = true
	i.active = false
	var err error
	if result.SessionReference == "" || len(result.SessionReference) > 4096 || (state.sessionID != "" && state.sessionID != result.SessionReference) {
		err = errors.New("ADAPTER_SESSION_CHANGED")
	} else {
		state.sessionID = result.SessionReference
		state.SessionDigest = sha256Text(result.SessionReference)
		body, _ := json.Marshal(map[string]any{"schemaVersion": 1, "kind": "agent-session", "sessionId": result.SessionReference, "sessionDigest": state.SessionDigest})
		err = atomicPrivateWrite(state.sessionArchivePath, body)
	}
	if err != nil {
		i.captureError = "ADAPTER_SESSION_ARCHIVE_FAILED"
		state.sessionArchiveErr = err
	} else {
		i.quiescenceHeld = true
		i.captureReadyAt = i.clock.Now().UTC().Format(time.RFC3339Nano)
		state.State = "INTERACTION_SUSPENDED"
	}
	state.mu.Unlock()
	// Never enter evaluation or return normally while the checkpoint is only a
	// proposal. Parent confirms durable persistence, then interrupts/cleans up.
	<-ctx.Done()
	return ctx.Err()
}

// A failed/unconfirmed safe point cannot become ordinary completion, even if
// the Adapter races the suspend control with execution.finished.
func (state *supervisorState) holdInteractionPauseFailure(ctx context.Context, process *agentprotocol.Process, reason string) error {
	state.mu.Lock()
	state.enterInteractionPausing(reason)
	state.closeLocalQuestionWaits()
	state.mu.Unlock()
	process.Close()
	state.mu.Lock()
	state.interaction.adapterStopped = true
	state.interaction.active = false
	state.interaction.captureError = reason
	state.mu.Unlock()
	<-ctx.Done()
	return ctx.Err()
}
