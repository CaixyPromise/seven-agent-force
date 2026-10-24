// Package facade exposes authority checks without granting access to private stores.
package facade

import (
	"context"
	interaction "github.com/CaixyPromise/seven-agent/force/internal/app/workforce/interaction/facade"
	"github.com/jackc/pgx/v5"
)

type InteractionAuthority interface {
	AuthorizeSummary(context.Context, pgx.Tx, int64, int64, []int64) (interaction.SummaryScope, error)
	Validate(context.Context, pgx.Tx, interaction.Machine) (interaction.Target, error)
	AuthorizeUser(context.Context, pgx.Tx, interaction.User, string) (interaction.Target, error)
}
