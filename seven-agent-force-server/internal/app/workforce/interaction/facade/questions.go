package facade

import "github.com/CaixyPromise/seven-agent/force/internal/app/workforce/interaction/domain"

// QuestionPage uses the same safe item projection as conversation, newest first.
type QuestionPage struct {
	Items      []domain.Item `json:"items"`
	NextCursor *string       `json:"nextCursor"`
}
