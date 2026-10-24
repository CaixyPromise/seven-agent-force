package schemaalignment

import (
	"context"
	"errors"
	"github.com/jackc/pgx/v5"
)

func verifyFactProducerSchema(ctx context.Context, q interface {
	QueryRow(context.Context, string, ...any) pgx.Row
}) error {
	var ok bool
	err := q.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM information_schema.columns WHERE table_schema=current_schema() AND table_name='workforce_attempt_facts' AND column_name='producer_source' AND data_type='text' AND is_nullable='NO')
 AND EXISTS(SELECT 1 FROM pg_indexes WHERE schemaname=current_schema() AND tablename='workforce_attempt_facts' AND indexdef LIKE 'CREATE UNIQUE INDEX% (attempt_id, producer_source, producer_sequence)')
 AND NOT EXISTS(SELECT 1 FROM pg_constraint WHERE conrelid='workforce_attempt_facts'::regclass AND contype='u' AND pg_get_constraintdef(oid)='UNIQUE (attempt_id, producer_sequence)')`).Scan(&ok)
	if err != nil {
		return err
	}
	if !ok {
		return errors.New("attempt fact producer namespace schema mismatch")
	}
	if err = q.QueryRow(ctx, `SELECT NOT EXISTS(SELECT 1 FROM workforce_attempt_facts WHERE producer_source NOT IN ('WORKER','SUPERVISOR_JOURNAL'))`).Scan(&ok); err != nil {
		return err
	}
	if !ok {
		return errors.New("attempt fact producer source mismatch")
	}
	return nil
}
