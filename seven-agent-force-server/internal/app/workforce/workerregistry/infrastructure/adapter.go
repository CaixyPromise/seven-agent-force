package infrastructure

import (
	"context"
	"github.com/CaixyPromise/seven-agent/force/internal/worker/agentprotocol"
	"github.com/jackc/pgx/v5"
)

func (s *Store) AdapterCapabilities(ctx context.Context, tx pgx.Tx, worker int64, instance, digest string) ([]string, error) {
	var raw []byte
	err := tx.QueryRow(ctx, `SELECT manifest_json FROM workforce_worker_images WHERE worker_id=$1 AND instance_id=$2 AND image_digest=$3 AND role='attempt' AND admission_status='ADMITTED' AND presence_status='PRESENT' FOR SHARE`, worker, instance, digest).Scan(&raw)
	if err != nil {
		return nil, err
	}
	descriptor, err := agentprotocol.ParseDescriptor(raw)
	if err != nil {
		return nil, err
	}
	return descriptor.Capabilities, nil
}
