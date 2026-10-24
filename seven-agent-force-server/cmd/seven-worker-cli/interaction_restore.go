package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"github.com/CaixyPromise/seven-agent/force/internal/worker/agentprotocol"
	"strconv"
)

func hasRestoredInteractionInput(spec attemptSpec) bool {
	// Optional RawMessage fields decode explicit JSON null as four bytes, not
	// nil. Treat only absent/null pairs as no restore; partial/non-null input
	// still passes through the full continuation and question/answer validator.
	return !absentRaw(spec.InteractionQuestionRequest) || !absentRaw(spec.InteractionResponse)
}

func absentRaw(raw json.RawMessage) bool {
	return len(raw) == 0 || bytes.Equal(bytes.TrimSpace(raw), []byte("null"))
}

// Private attempt spec is prepared by the trusted Worker from the authenticated
// restore-key response. Native frames and UDS ask cannot invoke this admission.
func (state *supervisorState) registerRestoredInteraction(spec attemptSpec, round int) error {
	if round != 1 {
		return nil
	}
	if !hasRestoredInteractionInput(spec) {
		return nil
	}
	bad := errors.New("INTERACTION_RESTORE_INPUT_INVALID")
	if spec.Continuation == nil || spec.Continuation.CheckpointID <= 0 {
		return bad
	}
	roundKey := "round:" + strconv.Itoa(round)
	question, answer, err := agentprotocol.ValidateRestoredInteraction(spec.InteractionQuestionRequest, spec.InteractionResponse, roundKey)
	if err != nil {
		return bad
	}
	body, _ := json.Marshal(question)
	state.mu.Lock()
	defer state.mu.Unlock()
	i := &state.interaction
	if !i.active || i.currentRound != roundKey || i.pausing || state.unresolvedInteractionCount() >= unresolvedInteractionLimit {
		return bad
	}
	i.pending[question.InteractionKey] = &pendingInteraction{request: question, identity: canonicalPayload(body), restoredExpected: &answer}
	// The saved answer is expected, not delivered: keep accepted/resolved unset,
	// do not enqueue interaction.request or fabricate interaction.received.
	state.refreshInteractionWaiting()
	return nil
}
