package agentprotocol

import "encoding/json"

// ValidateRestoredInteraction validates a pair supplied through the trusted
// Worker's private restore input. This function validates data, not authority:
// callers must never populate that input from a native frame or public Task JSON.
// The old round is deliberately absent; only the current runtime binds a round.
func ValidateRestoredInteraction(question, response json.RawMessage, round string) (QuestionRequest, InteractionAnswer, error) {
	var q QuestionRequest
	var a InteractionAnswer
	if len(question) > MaxMessageBytes || len(response) > MaxAnswerBytes || !exactObject(question, "interactionKey", "questions", "isBlocking") || !exactObject(response, "interactionKey", "responseId", "answers") {
		return q, a, ErrInvalid
	}
	bind := func(raw json.RawMessage) json.RawMessage {
		var fields map[string]json.RawMessage
		_ = json.Unmarshal(raw, &fields)
		fields["roundKey"], _ = json.Marshal(round)
		body, _ := json.Marshal(fields)
		return body
	}
	question, response = bind(question), bind(response)
	if ValidateInteractionPayload("interaction.request", question) != nil || ValidateInteractionPayload("interaction.respond", response) != nil {
		return q, a, ErrInvalid
	}
	_ = json.Unmarshal(question, &q)
	_ = json.Unmarshal(response, &a)
	if ValidateAnswer(q, a) != nil {
		return QuestionRequest{}, InteractionAnswer{}, ErrInvalid
	}
	return q, a, nil
}
