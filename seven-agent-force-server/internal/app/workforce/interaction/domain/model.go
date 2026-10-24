package domain

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"strings"
	"time"
	"unicode/utf8"
)

var (
	ErrInvalid     = errors.New("INTERACTION_INVALID")
	ErrNotFound    = errors.New("INTERACTION_NOT_FOUND")
	ErrStale       = errors.New("INTERACTION_STALE")
	ErrConflict    = errors.New("INTERACTION_IDEMPOTENCY_CONFLICT")
	ErrNotAllowed  = errors.New("INTERACTION_ACTION_NOT_ALLOWED")
	ErrUnavailable = errors.New("INTERACTION_UNAVAILABLE")
	ErrFenced      = errors.New("INTERACTION_FENCED")
	ErrBusy        = errors.New("INTERACTION_BUSY")
)

const (
	QuestionKind   = "QUESTION"
	MessageKind    = "USER_MESSAGE"
	Open           = "OPEN"
	Resolved       = "RESOLVED"
	Expired        = "EXPIRED"
	Cancelled      = "CANCELLED"
	Superseded     = "SUPERSEDED"
	NoDelivery     = "NONE"
	Saved          = "SAVED"
	Delivered      = "DELIVERED"
	MaxAnswerBytes = 16 << 10
)

type Option struct {
	Key   string `json:"key"`
	Label string `json:"label"`
}
type Question struct {
	Key           string   `json:"key"`
	Title         string   `json:"title"`
	Description   string   `json:"description"`
	Kind          string   `json:"kind"`
	Required      bool     `json:"required"`
	AllowFreeText bool     `json:"allowFreeText"`
	Options       []Option `json:"options"`
}
type Answer struct {
	QuestionKey        string   `json:"questionKey"`
	SelectedOptionKeys []string `json:"selectedOptionKeys"`
	Text               string   `json:"text"`
}
type Interaction struct {
	ID                       int64      `json:"id,string"`
	TenantID                 int64      `json:"-"`
	TaskID                   int64      `json:"taskId,string"`
	TaskTurnID               int64      `json:"taskTurnId,string"`
	AttemptID                int64      `json:"attemptId,string"`
	AdapterSessionGeneration int64      `json:"adapterSessionGeneration,string"`
	LocalRequestKey          string     `json:"localRequestKey"`
	RoundKey                 string     `json:"roundKey,omitempty"`
	PrincipalID              int64      `json:"-"`
	Kind                     string     `json:"kind"`
	State                    string     `json:"state"`
	DeliveryState            string     `json:"deliveryState"`
	Version                  int64      `json:"version,string"`
	Digest                   string     `json:"-"`
	Questions                []Question `json:"questions,omitempty"`
	IsBlocking               bool       `json:"isBlocking"`
	Text                     string     `json:"text,omitempty"`
	ResponseID               int64      `json:"responseId,string,omitempty"`
	Answers                  []Answer   `json:"answers,omitempty"`
	ResumeTaskTurnID         int64      `json:"resumeTaskTurnId,string,omitempty"`
	CreatedAt                time.Time  `json:"createdAt"`
	ExpiresAt                time.Time  `json:"expiresAt"`
}
type Response struct {
	ID, TenantID, TaskID, InteractionID, PrincipalID int64
	IdempotencyKey, Digest                           string
	Answers                                          []Answer
	CreatedAt                                        time.Time
}
type Item struct {
	ID                              int64           `json:"id,string"`
	Version                         int64           `json:"version,string"`
	Type                            string          `json:"type"`
	Body                            json.RawMessage `json:"body"`
	CreatedAt                       time.Time       `json:"createdAt"`
	Sequence                        int64           `json:"-"`
	TenantID, TaskID, InteractionID int64           `json:"-"`
	SourceKind, SourceKey, Digest   string          `json:"-"`
}
type Conversation struct {
	Items          []Item   `json:"items"`
	NextCursor     *string  `json:"nextCursor"`
	Watermark      int64    `json:"watermark,string"`
	AllowedActions []string `json:"allowedActions"`
}

func bounded(s string, max int, required bool) bool {
	return utf8.ValidString(s) && !strings.ContainsRune(s, 0) && len(s) <= max && (!required || strings.TrimSpace(s) != "")
}
func ValidKey(s string) bool { return bounded(s, 128, true) && strings.TrimSpace(s) == s }
func ValidateQuestions(qs []Question) error {
	if len(qs) < 1 || len(qs) > 3 {
		return ErrInvalid
	}
	seen := map[string]bool{}
	for _, q := range qs {
		if !ValidKey(q.Key) || seen[q.Key] || !bounded(q.Title, 512, true) || !bounded(q.Description, 4096, false) || len(q.Options) > 8 {
			return ErrInvalid
		}
		seen[q.Key] = true
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
		opts := map[string]bool{}
		for _, o := range q.Options {
			if !ValidKey(o.Key) || opts[o.Key] || !bounded(o.Label, 512, true) {
				return ErrInvalid
			}
			opts[o.Key] = true
		}
	}
	return nil
}
func ValidateAnswers(qs []Question, as []Answer) error {
	if ValidateQuestions(qs) != nil || len(as) > len(qs) {
		return ErrInvalid
	}
	raw, err := json.Marshal(as)
	if err != nil || len(raw) > MaxAnswerBytes {
		return ErrInvalid
	}
	answers := map[string]Answer{}
	for _, a := range as {
		if _, ok := answers[a.QuestionKey]; ok {
			return ErrInvalid
		}
		answers[a.QuestionKey] = a
	}
	for _, q := range qs {
		a, ok := answers[q.Key]
		if !ok {
			if q.Required {
				return ErrInvalid
			}
			continue
		}
		delete(answers, q.Key)
		if !bounded(a.Text, MaxAnswerBytes, false) || (q.Kind != "TEXT" && !q.AllowFreeText && a.Text != "") || (q.Kind == "TEXT" && len(a.SelectedOptionKeys) > 0) || (q.Kind == "SINGLE_CHOICE" && len(a.SelectedOptionKeys) > 1) {
			return ErrInvalid
		}
		if q.Required && len(a.SelectedOptionKeys) == 0 && strings.TrimSpace(a.Text) == "" {
			return ErrInvalid
		}
		opts := map[string]bool{}
		for _, o := range q.Options {
			opts[o.Key] = true
		}
		selected := map[string]bool{}
		for _, k := range a.SelectedOptionKeys {
			if !opts[k] || selected[k] {
				return ErrInvalid
			}
			selected[k] = true
		}
	}
	if len(answers) > 0 {
		return ErrInvalid
	}
	return nil
}
func ValidateMessage(text, key string) error {
	if !ValidKey(key) || !bounded(text, MaxAnswerBytes, true) {
		return ErrInvalid
	}
	return nil
}
func Digest(v any) string {
	b, _ := json.Marshal(v)
	h := sha256.Sum256(b)
	return hex.EncodeToString(h[:])
}
func (i *Interaction) SaveAnswer(version, responseID int64, answers []Answer) error {
	if version != i.Version {
		return ErrStale
	}
	if i.Kind != QuestionKind || i.State != Open || i.ResponseID != 0 {
		return ErrNotAllowed
	}
	if responseID <= 0 {
		return ErrInvalid
	}
	if err := ValidateAnswers(i.Questions, answers); err != nil {
		return err
	}
	i.ResponseID = responseID
	i.Answers = answers
	i.State = Resolved
	i.DeliveryState = Saved
	i.Version++
	return nil
}
func (i *Interaction) Acknowledge(responseID int64) (bool, error) {
	if i.ResponseID != responseID || (i.Kind == QuestionKind && responseID <= 0) {
		return false, ErrNotAllowed
	}
	if i.State != Resolved || i.ResumeTaskTurnID != 0 {
		return false, ErrNotAllowed
	}
	if i.DeliveryState == Delivered {
		return true, nil
	}
	if i.DeliveryState != Saved {
		return false, ErrNotAllowed
	}
	i.DeliveryState = Delivered
	i.Version++
	return false, nil
}

func (i *Interaction) AcknowledgeCold(responseID, authorizedTurn int64) (bool, error) {
	if authorizedTurn <= 0 || i.ResumeTaskTurnID != authorizedTurn || i.Kind != QuestionKind || i.State != Resolved || responseID <= 0 || i.ResponseID != responseID {
		return false, ErrNotAllowed
	}
	if i.DeliveryState == Delivered {
		return true, nil
	}
	if i.DeliveryState != Saved {
		return false, ErrNotAllowed
	}
	i.DeliveryState = Delivered
	i.Version++
	return false, nil
}
func (i *Interaction) BindMessage(turn, attempt, generation int64) error {
	if i.Kind != MessageKind || i.State != Resolved || i.DeliveryState != Saved || i.AttemptID != 0 || turn <= 0 || attempt <= 0 || generation <= 0 || (i.TaskTurnID != 0 && i.TaskTurnID != turn) {
		return ErrNotAllowed
	}
	i.TaskTurnID = turn
	i.AttemptID = attempt
	i.AdapterSessionGeneration = generation
	i.Version++
	return nil
}

// CarryMessage preserves the stable delivery identity across a cold resume.
// SAVED is not proof that the old native process never consumed the message.
func (i *Interaction) CarryMessage(sourceTurn, sourceAttempt, nextTurn int64) error {
	if i.Kind != MessageKind || i.State != Resolved || i.DeliveryState != Saved || i.TaskTurnID != sourceTurn || (i.AttemptID != 0 && i.AttemptID != sourceAttempt) || sourceTurn <= 0 || sourceAttempt <= 0 || nextTurn <= 0 || nextTurn == sourceTurn {
		return ErrNotAllowed
	}
	i.TaskTurnID, i.AttemptID, i.AdapterSessionGeneration = nextTurn, 0, 0
	i.Version++
	return nil
}

// Close preserves delivery facts while fencing further answers and delivery.
func (i *Interaction) Close(state string) error {
	if state != Expired && state != Cancelled && state != Superseded {
		return ErrInvalid
	}
	if i.State == state {
		return nil
	}
	if i.State != Open && i.State != Resolved {
		return ErrNotAllowed
	}
	i.State = state
	i.Version++
	return nil
}
func (i *Interaction) Expire(now time.Time) bool {
	if i.Kind == QuestionKind && i.State == Open && !now.Before(i.ExpiresAt) {
		_ = i.Close(Expired)
		return true
	}
	return false
}

// Projection is the browser allowlist. Runtime occurrence identities, principal
// relations and request digests never cross this boundary.
func (i Interaction) Projection() json.RawMessage {
	i.Questions = NormalizeQuestions(i.Questions)
	i.Answers = NormalizeAnswers(i.Answers)
	if i.Kind == MessageKind {
		b, _ := json.Marshal(struct {
			Text           string `json:"text"`
			DeliveryStatus string `json:"deliveryStatus"`
			Status         string `json:"status"`
		}{i.Text, i.DeliveryState, i.State})
		return b
	}
	type answer struct {
		ID      int64    `json:"id,string"`
		Answers []Answer `json:"answers"`
	}
	var response *answer
	if i.ResponseID > 0 {
		response = &answer{i.ResponseID, i.Answers}
	}
	b, _ := json.Marshal(struct {
		InteractionID  int64      `json:"interactionId,string"`
		Status         string     `json:"status"`
		Version        int64      `json:"version,string"`
		IsBlocking     bool       `json:"isBlocking"`
		Questions      []Question `json:"questions"`
		Response       *answer    `json:"response,omitempty"`
		DeliveryStatus string     `json:"deliveryStatus"`
	}{i.ID, i.State, i.Version, i.IsBlocking, i.Questions, response, i.DeliveryState})
	return b
}
func NormalizeQuestions(qs []Question) []Question {
	out := append([]Question{}, qs...)
	for n := range out {
		out[n].Options = append([]Option{}, out[n].Options...)
	}
	return out
}
func NormalizeAnswers(as []Answer) []Answer {
	out := append([]Answer{}, as...)
	for n := range out {
		out[n].SelectedOptionKeys = append([]string{}, out[n].SelectedOptionKeys...)
	}
	return out
}
