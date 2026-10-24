package infrastructure

import (
	"context"
	"errors"
	"fmt"
	"strings"

	principalfacade "github.com/CaixyPromise/seven-agent/force/internal/app/workforce/principal/facade"
	"github.com/jackc/pgx/v5"
)

// IDGenerator is the narrow Framework Snowflake seam used for Workforce
// principals. Callers must pass the same generator injected through app deps.
type IDGenerator interface{ NextID() int64 }

// Resolver is the Principal module's facade implementation. Keeping this
// adapter here means other modules consume principal/facade and never import
// this package's private SQL helpers directly.
type Resolver struct{}

func NewResolver() principalfacade.Resolver { return Resolver{} }

func (Resolver) EnsureUser(ctx context.Context, tx pgx.Tx, idGen principalfacade.IDGenerator, userID int64) (int64, error) {
	return EnsureUser(ctx, tx, idGen, userID)
}

func (Resolver) EnsureService(ctx context.Context, tx pgx.Tx, idGen principalfacade.IDGenerator, code string) (int64, error) {
	return EnsureService(ctx, tx, idGen, code)
}

func (Resolver) EnsureWorker(ctx context.Context, tx pgx.Tx, idGen principalfacade.IDGenerator, workerID int64) (int64, error) {
	return EnsureWorker(ctx, tx, idGen, workerID)
}

// EnsureUser resolves a Framework user to a stable Workforce principal. It
// never accepts a display name or a client-provided actor string.
func EnsureUser(ctx context.Context, tx pgx.Tx, idGen IDGenerator, userID int64) (int64, error) {
	if idGen == nil || userID < 1 {
		return 0, fmt.Errorf("invalid USER principal identity")
	}
	var exists bool
	if err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM sys_user WHERE id=$1)`, userID).Scan(&exists); err != nil || !exists {
		if err == nil {
			err = fmt.Errorf("Framework user does not exist")
		}
		return 0, err
	}
	var id int64
	err := tx.QueryRow(ctx, `SELECT id FROM workforce_principals WHERE principal_type='USER' AND user_id=$1`, userID).Scan(&id)
	if err == nil {
		return id, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return 0, err
	}
	id = idGen.NextID()
	err = tx.QueryRow(ctx, `INSERT INTO workforce_principals (id,principal_type,user_id) VALUES ($1,'USER',$2) ON CONFLICT (user_id) WHERE principal_type='USER' DO NOTHING RETURNING id`, id, userID).Scan(&id)
	if errors.Is(err, pgx.ErrNoRows) {
		err = tx.QueryRow(ctx, `SELECT id FROM workforce_principals WHERE principal_type='USER' AND user_id=$1`, userID).Scan(&id)
	}
	return id, err
}

func EnsureWorker(ctx context.Context, tx pgx.Tx, idGen IDGenerator, workerID int64) (int64, error) {
	if idGen == nil || workerID < 1 {
		return 0, fmt.Errorf("invalid WORKER principal identity")
	}
	var exists bool
	if err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM workforce_workers WHERE id=$1)`, workerID).Scan(&exists); err != nil || !exists {
		if err == nil {
			err = fmt.Errorf("Workforce worker does not exist")
		}
		return 0, err
	}
	var id int64
	err := tx.QueryRow(ctx, `SELECT id FROM workforce_principals WHERE principal_type='WORKER' AND worker_id=$1`, workerID).Scan(&id)
	if err == nil {
		return id, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return 0, err
	}
	id = idGen.NextID()
	err = tx.QueryRow(ctx, `INSERT INTO workforce_principals (id,principal_type,worker_id) VALUES ($1,'WORKER',$2) ON CONFLICT (worker_id) WHERE principal_type='WORKER' DO NOTHING RETURNING id`, id, workerID).Scan(&id)
	if errors.Is(err, pgx.ErrNoRows) {
		err = tx.QueryRow(ctx, `SELECT id FROM workforce_principals WHERE principal_type='WORKER' AND worker_id=$1`, workerID).Scan(&id)
	}
	return id, err
}

func EnsureService(ctx context.Context, tx pgx.Tx, idGen IDGenerator, code string) (int64, error) {
	code = strings.TrimSpace(code)
	if idGen == nil || code == "" || len(code) > 128 {
		return 0, fmt.Errorf("invalid SERVICE principal identity")
	}
	var id int64
	err := tx.QueryRow(ctx, `SELECT id FROM workforce_principals WHERE principal_type='SERVICE' AND service_code=$1`, code).Scan(&id)
	if err == nil {
		return id, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return 0, err
	}
	id = idGen.NextID()
	err = tx.QueryRow(ctx, `INSERT INTO workforce_principals (id,principal_type,service_code) VALUES ($1,'SERVICE',$2) ON CONFLICT (service_code) WHERE principal_type='SERVICE' DO NOTHING RETURNING id`, id, code).Scan(&id)
	if errors.Is(err, pgx.ErrNoRows) {
		err = tx.QueryRow(ctx, `SELECT id FROM workforce_principals WHERE principal_type='SERVICE' AND service_code=$1`, code).Scan(&id)
	}
	return id, err
}
