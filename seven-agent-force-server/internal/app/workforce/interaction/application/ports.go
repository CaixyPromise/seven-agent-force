package application

import (
	"context"
	"github.com/CaixyPromise/seven-agent/force/internal/app/workforce/interaction/domain"
	"github.com/CaixyPromise/seven-agent/force/internal/app/workforce/interaction/facade"
	"github.com/jackc/pgx/v5"
)

// Repository contains persistence primitives only. Every method uses the
// application-owned transaction; none authorizes, commits or changes state.
type Repository interface {
	Summaries(context.Context, pgx.Tx, int64, int64, []int64, []int64) (map[int64]facade.TaskSummary, error)
	FindOccurrence(context.Context, pgx.Tx, int64, int64, string) (*domain.Interaction, error)
	FindMessage(context.Context, pgx.Tx, int64, int64, int64, string) (*domain.Interaction, error)
	Get(context.Context, pgx.Tx, int64, int64, int64) (domain.Interaction, error)
	Put(context.Context, pgx.Tx, domain.Interaction, bool) error
	FindResponse(context.Context, pgx.Tx, int64, int64, int64, string) (*domain.Response, error)
	Response(context.Context, pgx.Tx, int64, int64) (domain.Response, error)
	InsertResponse(context.Context, pgx.Tx, domain.Response) error
	Pending(context.Context, pgx.Tx, int64, int64, int64, int64, int64, int) ([]domain.Interaction, error)
	PutItem(context.Context, pgx.Tx, domain.Item) error
	FindItem(context.Context, pgx.Tx, int64, int64, string, string) (*domain.Item, error)
	Item(context.Context, pgx.Tx, int64, int64, int64) (domain.Item, error)
	OpenQuestions(context.Context, pgx.Tx, int64, int64) ([]domain.Interaction, error)
	AttemptInteractions(context.Context, pgx.Tx, int64, int64, int64) ([]domain.Interaction, error)
	SavedSourceMessages(context.Context, pgx.Tx, int64, int64, int64, int64) ([]domain.Interaction, error)
	ColdQuestion(context.Context, pgx.Tx, int64, int64, int64, int64) (*domain.Interaction, error)
	Items(context.Context, pgx.Tx, int64, int64, int64, int) ([]domain.Item, error)
	History(context.Context, pgx.Tx, int64, int64, int64, int) ([]domain.Item, error)
	QuestionHistory(context.Context, pgx.Tx, int64, int64, int64, int) ([]domain.Item, error)
	Watermark(context.Context, pgx.Tx, int64) (int64, error)
	ReadState(context.Context, pgx.Tx, int64, int64, int64) (int64, error)
	PutReadState(context.Context, pgx.Tx, int64, int64, int64, int64, int64) error
}
