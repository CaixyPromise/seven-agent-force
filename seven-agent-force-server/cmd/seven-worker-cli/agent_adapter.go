package main

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strconv"

	"github.com/CaixyPromise/seven-agent/force/internal/worker/agentprotocol"
)

func loadAgentDescriptor() (agentprotocol.Descriptor, error) {
	body, err := os.ReadFile(manifestPath)
	if err != nil {
		return agentprotocol.Descriptor{}, agentprotocol.ErrManifest
	}
	descriptor, err := agentprotocol.ParseDescriptor(body)
	if err != nil {
		return descriptor, err
	}
	if err := descriptor.VerifyEntrypoint(); err != nil {
		return descriptor, err
	}
	if _, err := currentResultProcessorImageIdentity(); err != nil {
		return descriptor, err
	}
	return descriptor, nil
}

// adapterEnvironment deliberately does not inherit the Processor materialized
// environment or a caller-controlled Python import path.
func adapterEnvironment(specs ...attemptSpec) []string {
	environment := []string{"PATH=/usr/local/bin:/usr/bin:/bin", "LANG=C.UTF-8", "PYTHONDONTWRITEBYTECODE=1"}
	if len(specs) == 1 {
		spec := specs[0]
		environment = append(environment, "SEVEN_TASK_ID="+strconv.FormatInt(spec.TaskID, 10), "SEVEN_ATTEMPT_ID="+strconv.FormatInt(spec.AttemptID, 10), "SEVEN_ATTEMPT_GENERATION="+strconv.FormatInt(spec.Generation, 10), "SEVEN_CONTROL_SOCKET="+filepath.Join(spec.Runtime, "s"))
		if spec.Hooks != nil {
			environment = append(environment, "SEVEN_HOOK_CONFIGURATION_ROOT="+spec.Hooks.Root, "SEVEN_HOOK_CONFIGURATION_DIGEST="+spec.Hooks.ConfigurationDigest)
		}
	}
	return environment
}

func openAdmittedAdapter(ctx context.Context, specs ...attemptSpec) (*agentprotocol.Process, error) {
	descriptor, err := loadAgentDescriptor()
	if err != nil {
		return nil, err
	}
	process, err := agentprotocol.OpenProcess(ctx, descriptor.Executable, descriptor.Args, adapterEnvironment(specs...))
	if err != nil {
		return nil, err
	}
	if err := process.ValidateReady(descriptor); err != nil {
		process.Close()
		return nil, err
	}
	return process, nil
}

func frozenAdapterToolDefinitions(spec attemptSpec) ([]agentprotocol.ToolDefinition, error) {
	return agentprotocol.ToolDefinitions(spec.AllowedAdapterTools)
}

func runAgentAdapterRound(ctx context.Context, spec attemptSpec, state *supervisorState, round int, started harnessStarted) (harnessResult, error) {
	state.mu.RLock()
	process := state.adapterProcess
	state.mu.RUnlock()
	if process == nil {
		process, err := openAdmittedAdapter(ctx, spec)
		if err != nil {
			return harnessResult{}, err
		}
		state.mu.Lock()
		state.adapterProcess = process
		state.interaction.sessionGeneration++
		state.mu.Unlock()
	}
	state.mu.RLock()
	process = state.adapterProcess
	state.mu.RUnlock()
	roundKey := "round:" + strconv.Itoa(round)
	state.beginAgentRound(roundKey)
	defer state.endAgentRound(roundKey)
	if err := state.registerRestoredInteraction(spec, round); err != nil {
		return harnessResult{}, err
	}
	go state.monitorInteractionWait(ctx, roundKey, process)
	state.mu.RLock()
	reference, feedback := state.sessionID, state.managedFeedback
	state.mu.RUnlock()
	if round == 1 && spec.Continuation == nil {
		reference = ""
	}
	model := ""
	if spec.Provider != nil {
		model = spec.Provider.Model
	}
	inputs := map[string]any{"mode": "AGENT", "taskId": strconv.FormatInt(spec.TaskID, 10), "attemptId": strconv.FormatInt(spec.AttemptID, 10), "instruction": spec.Instruction, "workspace": spec.Workspace, "output": spec.Output, "state": spec.State, "temporary": spec.Temporary, "provider": map[string]string{"model": model}, "feedback": feedback, "reviewCorrection": spec.ReviewCorrection}
	if process.HasCapability("tool.request") {
		// Definitions are platform-owned, but exposure is driven solely by the
		// frozen allowlist supplied with this Attempt. Adapter capabilities,
		// instructions and repository files cannot open a tool.
		definitions, err := frozenAdapterToolDefinitions(spec)
		if err != nil {
			return harnessResult{}, err
		}
		inputs["availableTools"] = definitions
	}
	if round == 1 && hasRestoredInteractionInput(spec) {
		inputs["interactionResponse"] = spec.InteractionResponse
		inputs["interactionQuestionRequest"] = spec.InteractionQuestionRequest
	}
	if round == 1 && spec.Continuation != nil && !absentRaw(spec.OperationToolResult) {
		inputs["operationToolResult"] = spec.OperationToolResult
	}
	result, err := process.Round(ctx, roundKey, inputs, reference, func(pid int) error {
		if round == 1 && started != nil {
			return started(pid)
		}
		state.mu.Lock()
		state.HarnessPID = pid
		state.mu.Unlock()
		return nil
	}, state.observeAgent)
	// Fence the UDS side before deciding whether a terminal round can evaluate.
	// A concurrent suspend either won this lock (and is held below) or is rejected
	// against the now inactive round; it cannot race a stale pausing snapshot.
	state.endAgentRound(roundKey)
	state.mu.RLock()
	pausing := state.interaction.pausing
	state.mu.RUnlock()
	if err != nil {
		if errors.Is(err, agentprotocol.ErrControlReceiptPending) && ctx.Err() == nil {
			return harnessResult{}, state.holdInteractionPauseFailure(ctx, process, "ADAPTER_CONTROL_RECEIPT_UNCONFIRMED")
		}
		if pausing && ctx.Err() == nil {
			return harnessResult{}, state.holdInteractionPauseFailure(ctx, process, "ADAPTER_SUSPEND_NOT_CONFIRMED")
		}
		return harnessResult{}, err
	}
	if result.Outcome == "SUSPENDED" {
		return harnessResult{}, state.holdInteractionSuspension(ctx, process, result)
	}
	if pausing {
		return harnessResult{}, state.holdInteractionPauseFailure(ctx, process, "ADAPTER_SUSPEND_NOT_CONFIRMED")
	}
	if result.Outcome != "FINISHED" {
		return harnessResult{}, errors.New("ADAPTER_ROUND_NOT_FINISHED")
	}
	if result.SessionReference == "" || len(result.SessionReference) > 512 {
		return harnessResult{}, errors.New("ADAPTER_SESSION_UNAVAILABLE")
	}
	state.mu.Lock()
	if state.sessionID != "" && state.sessionID != result.SessionReference {
		state.mu.Unlock()
		return harnessResult{}, errors.New("ADAPTER_SESSION_CHANGED")
	}
	state.sessionID = result.SessionReference
	state.SessionDigest = sha256Text(result.SessionReference)
	body, err := json.Marshal(map[string]any{"schemaVersion": 1, "kind": "agent-session", "sessionId": result.SessionReference, "sessionDigest": state.SessionDigest})
	if err == nil {
		err = atomicPrivateWrite(state.sessionArchivePath, body)
	}
	if err != nil {
		state.sessionArchiveErr = err
		state.mu.Unlock()
		return harnessResult{}, errors.New("ADAPTER_SESSION_ARCHIVE_FAILED")
	}
	state.mu.Unlock()
	return harnessResult{Summary: safeAgentText(result.Summary), ChangedFiles: proposedChangedFiles(ctx, spec.Workspace)}, nil
}
