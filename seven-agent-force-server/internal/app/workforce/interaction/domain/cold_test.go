package domain

import "testing"

func TestColdDeliveryTransitions(t *testing.T) {
	message := Interaction{ID: 9, Kind: MessageKind, State: Resolved, DeliveryState: Saved, TaskTurnID: 30, AttemptID: 40, AdapterSessionGeneration: 1, LocalRequestKey: "stable", Version: 1}
	if e := message.CarryMessage(30, 40, 31); e != nil {
		t.Fatal(e)
	}
	if message.ID != 9 || message.LocalRequestKey != "stable" || message.TaskTurnID != 31 || message.AttemptID != 0 || message.AdapterSessionGeneration != 0 {
		t.Fatal(message)
	}
	message.DeliveryState = Delivered
	if e := message.CarryMessage(31, 41, 32); e != ErrNotAllowed {
		t.Fatal(e)
	}
	question := Interaction{Kind: QuestionKind, State: Resolved, DeliveryState: Saved, ResponseID: 8, ResumeTaskTurnID: 31, TaskTurnID: 30, AttemptID: 40, Version: 2}
	if _, e := question.Acknowledge(8); e != ErrNotAllowed {
		t.Fatal(e)
	}
	if _, e := question.AcknowledgeCold(8, 32); e != ErrNotAllowed {
		t.Fatal(e)
	}
	if _, e := question.AcknowledgeCold(9, 31); e != ErrNotAllowed {
		t.Fatal(e)
	}
	if replay, e := question.AcknowledgeCold(8, 31); e != nil || replay {
		t.Fatal(replay, e)
	}
	if replay, e := question.AcknowledgeCold(8, 31); e != nil || !replay {
		t.Fatal(replay, e)
	}
	if question.TaskTurnID != 30 || question.AttemptID != 40 || question.Version != 3 {
		t.Fatal(question)
	}
}
