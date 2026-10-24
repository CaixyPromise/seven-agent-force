package domain

import (
	"testing"
	"time"
)

func transition(reason string, source SourceType, recovery Recoverability) TaskNotificationTransition {
	return TaskNotificationTransition{TenantID: 1, TaskID: 2, TaskTurnID: 3, SourceType: source, SourceID: 4, SourceOccurrence: 5, ReasonCode: reason, Recoverability: recovery, OccurredAt: time.Now().UTC()}
}

func TestTransitionPolicySuppressesAutomaticRecoveryNoise(t *testing.T) {
	for _, reason := range []string{
		"AGENT_RETRY_EXHAUSTED",
		"RUNTIME_UNRECOVERABLE",
		"TASK_FAILED",
		"ATTEMPT_REATTACHED",
		"HOST_LOSS_RECOVERED",
		"PROCESSOR_REVISE",
	} {
		input := transition(reason, SourceTaskAttempt, RecoverabilityAutoRecovering)
		got, err := ClassifyTransition(input)
		if err != nil {
			t.Fatal(err)
		}
		if got.Disposition != DispositionNone {
			t.Fatalf("reason=%s disposition=%s", reason, got.Disposition)
		}
	}
}

func TestTransitionPolicyClassifiesTerminalAndActionableFacts(t *testing.T) {
	tests := []struct {
		reason   string
		source   SourceType
		recovery Recoverability
		want     TransitionDisposition
		focus    Focus
		mention  bool
	}{
		{"AGENT_INPUT_REQUIRED", SourceTaskAttempt, RecoverabilityUserRecoverable, DispositionActionRequired, FocusContinuation, true},
		{"AGENT_OPERATION_APPROVAL_REQUIRED", SourceTaskAttempt, RecoverabilityUserRecoverable, DispositionActionRequired, FocusContinuation, true},
		{"PROCESSOR_WAIT_USER", SourceContinuationCheckpoint, RecoverabilityUserRecoverable, DispositionActionRequired, FocusEvaluation, true},
		{"PROCESSOR_MANUAL_REVIEW", SourceContinuationCheckpoint, RecoverabilityUserRecoverable, DispositionActionRequired, FocusContinuation, false},
		{"PROCESSOR_REJECT", SourceResultEvaluation, RecoverabilityTerminal, DispositionInterrupted, FocusEvaluation, true},
		{"AGENT_RETRY_EXHAUSTED", SourceTaskAttempt, RecoverabilityTerminal, DispositionInterrupted, FocusContinuation, true},
		{"RUNTIME_UNRECOVERABLE", SourceRuntimeRecovery, RecoverabilityTerminal, DispositionInterrupted, FocusContinuation, true},
		{"TASK_FAILED", SourceTaskAttempt, RecoverabilityTerminal, DispositionInterrupted, FocusContinuation, true},
		{"QUEUE_TIMEOUT", SourceTaskAttempt, RecoverabilityTerminal, DispositionInterrupted, FocusContinuation, true},
		{"SUPERVISOR_STARTUP_TIMEOUT", SourceTaskAttempt, RecoverabilityTerminal, DispositionInterrupted, FocusContinuation, true},
		{"TASK_TIMED_OUT", SourceTaskAttempt, RecoverabilityTerminal, DispositionInterrupted, FocusContinuation, true},
		{"TASK_CANCELLED_BY_OPERATOR", SourceTaskCancel, RecoverabilityTerminal, DispositionInterrupted, FocusContinuation, true},
		{"TASK_CANCELLED_BY_SYSTEM", SourceTaskCancel, RecoverabilityTerminal, DispositionInterrupted, FocusContinuation, true},
		{"FINALIZATION_RETRY_EXHAUSTED", SourceFinalizationRun, RecoverabilityUserRecoverable, DispositionActionRequired, FocusResult, false},
		{"FINALIZATION_SOURCE_LOST", SourceFinalizationRun, RecoverabilityTerminal, DispositionInterrupted, FocusResult, false},
		{"ARTIFACT_UPLOAD_RETRY_EXHAUSTED", SourceArtifactUpload, RecoverabilityUserRecoverable, DispositionActionRequired, FocusResult, false},
		{"INDEPENDENT_REVIEW_REVISION_LIMIT_EXHAUSTED", SourceIndependentReviewRun, RecoverabilityUserRecoverable, DispositionActionRequired, FocusResult, false},
		{"INDEPENDENT_REVIEW_NO_MATERIAL_PROGRESS", SourceIndependentReviewRun, RecoverabilityUserRecoverable, DispositionActionRequired, FocusResult, false},
		{"INDEPENDENT_REVIEW_CHECKPOINT_UNAVAILABLE", SourceIndependentReviewRun, RecoverabilityUserRecoverable, DispositionActionRequired, FocusResult, false},
		{"INDEPENDENT_REVIEW_REVISION_IDENTITY_CONFLICT", SourceIndependentReviewRun, RecoverabilityUserRecoverable, DispositionActionRequired, FocusResult, false},
		{"DELIVERY_WAITING_APPROVAL", SourceDeliveryRequest, RecoverabilityUserRecoverable, DispositionActionRequired, FocusDelivery, false},
		{"DELIVERY_MANUAL_REVIEW", SourceDeliveryRequest, RecoverabilityUserRecoverable, DispositionActionRequired, FocusDelivery, false},
		{"DELIVERY_OUTCOME_UNKNOWN", SourceDeliveryRequest, RecoverabilityUserRecoverable, DispositionActionRequired, FocusDelivery, false},
		{"DELIVERY_FAILED", SourceDeliveryRequest, RecoverabilityUserRecoverable, DispositionActionRequired, FocusDelivery, false},
		{"REVIEW_ACCESS_NOT_VERIFIED", SourceReviewOperation, RecoverabilityUserRecoverable, DispositionActionRequired, FocusDelivery, false},
		{"REVIEW_AUTHORIZATION_STALE", SourceReviewOperation, RecoverabilityUserRecoverable, DispositionActionRequired, FocusDelivery, false},
		{"REVIEW_RETRY_EXHAUSTED", SourceReviewOperation, RecoverabilityUserRecoverable, DispositionActionRequired, FocusDelivery, false},
		{"TASK_TURN_SUCCEEDED", SourceFinalizationRun, RecoverabilityTerminal, DispositionCompleted, FocusResult, false},
		{"INDEPENDENT_REVIEW_HUMAN_ACCEPTED", SourceIndependentReviewRun, RecoverabilityTerminal, DispositionCompleted, FocusResult, false},
		{"DELIVERY_SUCCEEDED", SourceDeliveryRequest, RecoverabilityTerminal, DispositionDeliveryCompleted, FocusDelivery, false},
	}
	for _, test := range tests {
		t.Run(test.reason, func(t *testing.T) {
			got, err := ClassifyTransition(transition(test.reason, test.source, test.recovery))
			if err != nil {
				t.Fatal(err)
			}
			if got.Disposition != test.want || got.Focus != test.focus || got.MentionUser != test.mention {
				t.Fatalf("decision=%+v", got)
			}
		})
	}
}

func TestTransitionPolicySilencesCreatorCancel(t *testing.T) {
	got, err := ClassifyTransition(transition("TASK_CANCELLED_BY_CREATOR", SourceTaskCancel, RecoverabilityTerminal))
	if err != nil {
		t.Fatal(err)
	}
	if got.Disposition != DispositionNone || !got.Silent {
		t.Fatalf("decision=%+v", got)
	}
}

func TestV2FactIdentityIncludesOccurrence(t *testing.T) {
	record := Record{TaskID: 2, TaskTurnID: 3, FactType: FactActionRequired, SourceType: SourceDeliveryRequest, SourceID: 4, SourceOccurrence: 9, FactIdentityVersion: 2, SubscriberUserID: 7}
	got := BuildIdempotencyIdentity(record)
	want := "workforce-task:2:turn:3:TASK_ACTION_REQUIRED:DELIVERY_REQUEST:4:occ:9:user:7"
	if got != want {
		t.Fatalf("identity=%s want=%s", got, want)
	}
	record.SourceOccurrence = 10
	if BuildIdempotencyIdentity(record) == got {
		t.Fatal("different occurrences collapsed")
	}
}
