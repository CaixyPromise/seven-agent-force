package agentprotocol

import (
	"bytes"
	"encoding/json"
	"strconv"
	"strings"
	"unicode/utf8"
)

const MaxAnswerBytes = 16 << 10

type QuestionOption struct {
	Key   string `json:"key"`
	Label string `json:"label"`
}
type Question struct {
	Key           string           `json:"key"`
	Title         string           `json:"title"`
	Description   string           `json:"description"`
	Kind          string           `json:"kind"`
	Required      bool             `json:"required"`
	Options       []QuestionOption `json:"options"`
	AllowFreeText bool             `json:"allowFreeText"`
}
type QuestionRequest struct {
	RoundKey       string     `json:"roundKey"`
	InteractionKey string     `json:"interactionKey"`
	Questions      []Question `json:"questions"`
	IsBlocking     bool       `json:"isBlocking"`
}
type Answer struct {
	QuestionKey        string   `json:"questionKey"`
	SelectedOptionKeys []string `json:"selectedOptionKeys"`
	Text               string   `json:"text"`
}
type InteractionAnswer struct {
	RoundKey       string   `json:"roundKey"`
	InteractionKey string   `json:"interactionKey"`
	ResponseID     string   `json:"responseId"`
	Answers        []Answer `json:"answers"`
}
type UserMessage struct {
	RoundKey   string `json:"roundKey"`
	MessageKey string `json:"messageKey"`
	Text       string `json:"text"`
}
type DeliveryStatus string

const (
	DeliveryReceived DeliveryStatus = "RECEIVED"
	DeliveryRejected DeliveryStatus = "REJECTED"
	DeliveryUnknown  DeliveryStatus = "UNKNOWN"
)

// Exact field sets deliberately exclude Task identities and authority claims.
func exactObject(raw json.RawMessage, fields ...string) bool {
	var object map[string]json.RawMessage
	if !strictDocument(raw) || json.Unmarshal(raw, &object) != nil || len(object) != len(fields) {
		return false
	}
	for _, field := range fields {
		if value, ok := object[field]; !ok || bytes.Equal(bytes.TrimSpace(value), []byte("null")) {
			return false
		}
	}
	return true
}

func boundedText(s string, limit int) bool {
	return utf8.ValidString(s) && !strings.ContainsRune(s, 0) && len(s) <= limit
}
func nonblank(s string, limit int) bool { return strings.TrimSpace(s) != "" && boundedText(s, limit) }
func validQuestionKey(s string) bool    { return nonblank(s, 128) && strings.TrimSpace(s) == s }
func validResponseID(s string) bool {
	_, err := strconv.ParseInt(s, 10, 64)
	return idPattern.MatchString(s) && err == nil
}

// ValidateInteractionPayload shares the SDK's closed, bounded wire shapes.
// It validates observations only; it cannot authorize an operation.
func ValidateInteractionPayload(method string, raw json.RawMessage) error {
	var fields map[string]json.RawMessage
	var binding struct {
		RoundKey         string         `json:"roundKey"`
		InteractionKey   string         `json:"interactionKey"`
		ResponseID       string         `json:"responseId"`
		MessageKey       string         `json:"messageKey"`
		Status           DeliveryStatus `json:"status"`
		SessionReference string         `json:"sessionReference"`
	}
	switch method {
	case "interaction.request", "interaction.respond", "interaction.received", "interaction.closed", "user.message", "message.received", "execution.suspend", "execution.suspended":
	default:
		return nil
	}
	if json.Unmarshal(raw, &fields) != nil || json.Unmarshal(raw, &binding) != nil || !keyPattern.MatchString(binding.RoundKey) {
		return ErrInvalid
	}
	switch method {
	case "interaction.request":
		var request QuestionRequest
		if !exactObject(raw, "roundKey", "interactionKey", "questions", "isBlocking") || json.Unmarshal(raw, &request) != nil || !keyPattern.MatchString(request.InteractionKey) || len(request.Questions) < 1 || len(request.Questions) > 3 {
			return ErrInvalid
		}
		var questions []json.RawMessage
		_ = json.Unmarshal(fields["questions"], &questions)
		seen := map[string]bool{}
		for i, q := range request.Questions {
			if !exactObject(questions[i], "key", "title", "description", "kind", "required", "options", "allowFreeText") || !validQuestionKey(q.Key) || seen[q.Key] || !nonblank(q.Title, 512) || !boundedText(q.Description, 4096) || q.Options == nil || len(q.Options) > 8 {
				return ErrInvalid
			}
			switch q.Kind {
			case "TEXT":
				if len(q.Options) != 0 {
					return ErrInvalid
				}
			case "SINGLE_CHOICE", "MULTIPLE_CHOICE":
				if len(q.Options) == 0 {
					return ErrInvalid
				}
			default:
				return ErrInvalid
			}
			seen[q.Key] = true
			var qFields map[string]json.RawMessage
			_ = json.Unmarshal(questions[i], &qFields)
			var options []json.RawMessage
			_ = json.Unmarshal(qFields["options"], &options)
			optionKeys := map[string]bool{}
			for j, option := range q.Options {
				if !exactObject(options[j], "key", "label") || !validQuestionKey(option.Key) || optionKeys[option.Key] || !nonblank(option.Label, 512) {
					return ErrInvalid
				}
				optionKeys[option.Key] = true
			}
		}
	case "interaction.respond":
		var answer InteractionAnswer
		if !exactObject(raw, "roundKey", "interactionKey", "responseId", "answers") || json.Unmarshal(raw, &answer) != nil || !keyPattern.MatchString(answer.InteractionKey) || !validResponseID(answer.ResponseID) || answer.Answers == nil || len(answer.Answers) > 3 {
			return ErrInvalid
		}
		// The SDK and backend use Go's canonical HTML/line-separator escaping.
		canonical, _ := json.Marshal(answer.Answers)
		if len(canonical) > MaxAnswerBytes {
			return ErrInvalid
		}
		var answers []json.RawMessage
		_ = json.Unmarshal(fields["answers"], &answers)
		seen := map[string]bool{}
		for i, a := range answer.Answers {
			if !exactObject(answers[i], "questionKey", "selectedOptionKeys", "text") || !validQuestionKey(a.QuestionKey) || seen[a.QuestionKey] || a.SelectedOptionKeys == nil || len(a.SelectedOptionKeys) > 8 || !boundedText(a.Text, MaxAnswerBytes) {
				return ErrInvalid
			}
			seen[a.QuestionKey] = true
			options := map[string]bool{}
			for _, option := range a.SelectedOptionKeys {
				if !validQuestionKey(option) || options[option] {
					return ErrInvalid
				}
				options[option] = true
			}
		}
	case "user.message":
		var message UserMessage
		if !exactObject(raw, "roundKey", "messageKey", "text") || json.Unmarshal(raw, &message) != nil || !keyPattern.MatchString(message.MessageKey) || !nonblank(message.Text, MaxAnswerBytes) {
			return ErrInvalid
		}
	case "interaction.received", "message.received":
		key := "interactionKey"
		value := binding.InteractionKey
		if method == "message.received" {
			key = "messageKey"
			value = binding.MessageKey
		}
		wanted := []string{"roundKey", key, "status"}
		if method == "interaction.received" {
			wanted = append(wanted, "responseId")
			if !validResponseID(binding.ResponseID) {
				return ErrInvalid
			}
		}
		if !exactObject(raw, wanted...) || !keyPattern.MatchString(value) || (binding.Status != DeliveryReceived && binding.Status != DeliveryRejected && binding.Status != DeliveryUnknown) {
			return ErrInvalid
		}
	case "interaction.closed":
		var closed struct {
			RoundKey       string `json:"roundKey"`
			InteractionKey string `json:"interactionKey"`
			Reason         string `json:"reason"`
		}
		if !exactObject(raw, "roundKey", "interactionKey", "reason") || json.Unmarshal(raw, &closed) != nil || !keyPattern.MatchString(closed.InteractionKey) || (closed.Reason != "NATIVE_REQUEST_CLEARED" && closed.Reason != "TURN_COMPLETED" && closed.Reason != "TURN_INTERRUPTED") {
			return ErrInvalid
		}
	case "execution.suspend":
		if !exactObject(raw, "roundKey") {
			return ErrInvalid
		}
	case "execution.suspended":
		if !exactObject(raw, "roundKey", "sessionReference") || !nonblank(binding.SessionReference, 4096) {
			return ErrInvalid
		}
	}
	return nil
}

func ValidateAnswer(request QuestionRequest, answer InteractionAnswer) error {
	requestBody, _ := json.Marshal(request)
	answerBody, _ := json.Marshal(answer)
	if ValidateInteractionPayload("interaction.request", requestBody) != nil || ValidateInteractionPayload("interaction.respond", answerBody) != nil || request.RoundKey != answer.RoundKey || request.InteractionKey != answer.InteractionKey || len(request.Questions) < len(answer.Answers) {
		return ErrInvalid
	}
	for _, question := range request.Questions {
		var match *Answer
		for i := range answer.Answers {
			if answer.Answers[i].QuestionKey == question.Key {
				if match != nil {
					return ErrInvalid
				}
				match = &answer.Answers[i]
			}
		}
		if match == nil {
			if question.Required {
				return ErrInvalid
			}
			continue
		}
		if (question.Kind != "TEXT" && !question.AllowFreeText && match.Text != "") || (question.Kind == "TEXT" && len(match.SelectedOptionKeys) > 0) || (question.Kind == "SINGLE_CHOICE" && len(match.SelectedOptionKeys) > 1) || (question.Required && len(match.SelectedOptionKeys) == 0 && strings.TrimSpace(match.Text) == "") {
			return ErrInvalid
		}
		for _, selected := range match.SelectedOptionKeys {
			found := false
			for _, option := range question.Options {
				if selected == option.Key {
					found = true
				}
			}
			if !found {
				return ErrInvalid
			}
		}
	}
	for _, a := range answer.Answers {
		found := false
		for _, q := range request.Questions {
			if q.Key == a.QuestionKey {
				found = true
			}
		}
		if !found {
			return ErrInvalid
		}
	}
	return nil
}
