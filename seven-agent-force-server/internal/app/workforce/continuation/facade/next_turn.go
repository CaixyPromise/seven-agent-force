package facade

import (
	"context"
	"github.com/CaixyPromise/seven-agent/force/internal/app/workforce/continuation/application"
	"github.com/CaixyPromise/seven-agent/force/internal/app/workforce/continuation/domain"
	"github.com/jackc/pgx/v5"
)

type NextTurnRequest = application.CreateNextTurnRequest
type Turn = domain.Turn

// NextTurnCreator must neither begin nor commit the caller's transaction.
type NextTurnCreator interface {
	CreateNextTurnTx(context.Context, pgx.Tx, NextTurnRequest) (Turn, bool, error)
}
