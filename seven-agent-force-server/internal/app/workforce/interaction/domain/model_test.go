package domain

import (
	"encoding/json"
	"strings"
	"testing"
	"time"
)

func questions() []Question {
	return []Question{{Key: "choice", Title: "Choose", Kind: "SINGLE_CHOICE", Required: true, Options: []Option{{Key: "a", Label: "A"}, {Key: "b", Label: "B"}}}, {Key: "text", Title: "Explain", Kind: "TEXT", Options: []Option{}}}
}
func TestQuestionValidation(t *testing.T) {
	good := questions()
	if ValidateQuestions(good) != nil {
		t.Fatal("valid questions rejected")
	}
	tests := map[string][]Question{"zero": {}, "four": append(append([]Question{}, good...), good...), "duplicate": {good[0], good[0]}, "unknown": {{Key: "x", Title: "x", Kind: "FILE"}}, "textOptions": {{Key: "x", Title: "x", Kind: "TEXT", Options: good[0].Options}}, "emptyChoice": {{Key: "x", Title: "x", Kind: "SINGLE_CHOICE"}}, "duplicateOption": {{Key: "x", Title: "x", Kind: "SINGLE_CHOICE", Options: []Option{{Key: "a", Label: "A"}, {Key: "a", Label: "again"}}}}}
	for name, q := range tests {
		t.Run(name, func(t *testing.T) {
			if ValidateQuestions(q) == nil {
				t.Fatal("accepted malformed question")
			}
		})
	}
	q := good[0]
	q.Options = make([]Option, 9)
	if ValidateQuestions([]Question{q}) == nil {
		t.Fatal("nine options accepted")
	}
}
func TestAnswersBoundToQuestionSchema(t *testing.T) {
	good := []Answer{{QuestionKey: "choice", SelectedOptionKeys: []string{"a"}}}
	if ValidateAnswers(questions(), good) != nil {
		t.Fatal("valid answer rejected")
	}
	tests := map[string][]Answer{"missingRequired": {}, "unknownQuestion": {{QuestionKey: "no"}}, "unknownOption": {{QuestionKey: "choice", SelectedOptionKeys: []string{"no"}}}, "duplicateQuestion": {good[0], good[0]}, "duplicateOption": {{QuestionKey: "choice", SelectedOptionKeys: []string{"a", "a"}}}, "multipleSingle": {{QuestionKey: "choice", SelectedOptionKeys: []string{"a", "b"}}}, "unallowedFreeText": {{QuestionKey: "choice", Text: "arbitrary"}}, "textChoice": {good[0], {QuestionKey: "text", SelectedOptionKeys: []string{"a"}}}, "huge": {good[0], {QuestionKey: "text", Text: strings.Repeat("界", 6000)}}}
	for name, a := range tests {
		t.Run(name, func(t *testing.T) {
			if ValidateAnswers(questions(), a) == nil {
				t.Fatal("accepted malformed answer")
			}
		})
	}
	qs := questions()
	qs[0].Kind = "MULTIPLE_CHOICE"
	if ValidateAnswers(qs, []Answer{{QuestionKey: "choice", SelectedOptionKeys: []string{"a", "b"}}}) != nil {
		t.Fatal("multi choice rejected")
	}
	qs[0].AllowFreeText = true
	if ValidateAnswers(qs, []Answer{{QuestionKey: "choice", Text: "Other"}}) != nil {
		t.Fatal("allowed free text rejected")
	}
}
func TestTransitionsSeparateAnswerDeliveryAndExpiry(t *testing.T) {
	i := Interaction{Kind: QuestionKind, State: Open, DeliveryState: NoDelivery, Version: 1, Questions: questions(), ExpiresAt: time.Now().Add(time.Hour)}
	if _, e := i.Acknowledge(2); e == nil {
		t.Fatal("unanswered acknowledgement accepted")
	}
	if e := i.SaveAnswer(2, 2, []Answer{{QuestionKey: "choice", SelectedOptionKeys: []string{"a"}}}); e != ErrStale {
		t.Fatal(e)
	}
	if e := i.SaveAnswer(1, 2, []Answer{{QuestionKey: "choice", SelectedOptionKeys: []string{"a"}}}); e != nil {
		t.Fatal(e)
	}
	if i.State != Resolved || i.DeliveryState != Saved || i.Version != 2 {
		t.Fatal(i)
	}
	if replay, e := i.Acknowledge(2); e != nil || replay || i.State != Resolved || i.DeliveryState != Delivered || i.Version != 3 {
		t.Fatal(i, e)
	}
	if replay, e := i.Acknowledge(2); e != nil || !replay || i.Version != 3 {
		t.Fatal("ack replay changed version")
	}
	if e := i.SaveAnswer(3, 3, []Answer{}); e != ErrNotAllowed {
		t.Fatal("second answer allowed", e)
	}
	expired := Interaction{Kind: QuestionKind, State: Open, Version: 1, ExpiresAt: time.Now().Add(-time.Second)}
	if !expired.Expire(time.Now()) || expired.State != Expired || expired.Expire(time.Now()) {
		t.Fatal(expired)
	}
	for _, state := range []string{Expired, Cancelled, Superseded} {
		i.State = Open
		if e := i.Close(state); e != nil {
			t.Fatal(e)
		}
		if e := i.SaveAnswer(i.Version, 4, nil); e != ErrNotAllowed {
			t.Fatal("closed answered", e)
		}
	}
}
func TestPublicProjectionAndUTF8Bounds(t *testing.T) {
	i := Interaction{ID: 9007199254740993, Version: 2, Kind: QuestionKind, State: Resolved, Questions: []Question{{Key: "text", Title: "Text", Kind: "TEXT"}}, Answers: []Answer{{QuestionKey: "text", Text: "hello"}}, ResponseID: 9, RoundKey: "private-round", LocalRequestKey: "private-key", AttemptID: 123, DeliveryState: Saved}
	raw := string(i.Projection())
	for _, s := range []string{`"interactionId":"9007199254740993"`, `"version":"2"`, `"options":[]`, `"selectedOptionKeys":[]`, `"deliveryStatus":"SAVED"`} {
		if !strings.Contains(raw, s) {
			t.Fatalf("missing %s: %s", s, raw)
		}
	}
	for _, s := range []string{"private", "attemptId", "localRequestKey", "roundKey"} {
		if strings.Contains(raw, s) {
			t.Fatal("private identity leaked")
		}
	}
	if !json.Valid([]byte(raw)) {
		t.Fatal(raw)
	}
	if ValidateMessage(strings.Repeat("a", MaxAnswerBytes), "key") != nil || ValidateMessage(strings.Repeat("a", MaxAnswerBytes+1), "key") == nil || ValidateMessage(string([]byte{0xff}), "key") == nil || ValidateMessage("\x00", "key") == nil {
		t.Fatal("message byte bound")
	}
}
