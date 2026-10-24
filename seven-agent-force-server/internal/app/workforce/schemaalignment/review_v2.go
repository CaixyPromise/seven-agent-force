package schemaalignment

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"
)

// Independent review has no user-managed policy schema. A ReviewRun only
// freezes the runtime and Task-owned model credential used for that execution.
func ensureReviewRunSchema(ctx context.Context, tx pgx.Tx) error {
	required := map[string][]string{
		"workforce_review_runs": {
			"runtime_release_id", "reviewer_image_digest", "adapter_identity", "adapter_version",
			"adapter_protocol", "adapter_capabilities_json", "model_credential_id",
			"credential_version", "model_protocol", "primary_model",
			"provider_proxy_image_digest", "selection_digest",
		},
		"workforce_review_run_attempts": {
			"runtime_release_id", "reviewer_image_digest", "adapter_identity", "adapter_version",
			"adapter_protocol", "adapter_capabilities_json", "model_credential_id",
			"credential_version", "model_protocol", "primary_model",
			"provider_proxy_image_digest", "selection_digest",
		},
	}
	for table, columns := range required {
		for _, column := range columns {
			exists, err := columnExists(ctx, tx, table, column)
			if err != nil {
				return err
			}
			if !exists {
				return fmt.Errorf("PROJECT_HARD_CUT_RESET_REQUIRED: review execution column %s.%s is missing", table, column)
			}
		}
	}
	matched, exists, err := reviewRunTaskReadIndexMatches(ctx, tx)
	if err != nil {
		return err
	}
	if exists && !matched {
		if _, err = tx.Exec(ctx, `DROP INDEX workforce_review_runs_task_read_idx`); err != nil {
			return fmt.Errorf("replace Independent Review task read index: %w", err)
		}
	}
	if !exists || !matched {
		if _, err = tx.Exec(ctx, `CREATE INDEX workforce_review_runs_task_read_idx ON workforce_review_runs (tenant_id,task_id,id DESC)`); err != nil {
			return fmt.Errorf("create Independent Review task read index: %w", err)
		}
	}
	return nil
}

type reviewSchemaQuery interface {
	QueryRow(context.Context, string, ...any) pgx.Row
}

func verifyReviewRunSchemaShape(ctx context.Context, q reviewSchemaQuery) error {
	expected := map[string][]string{
		"workforce_review_runs": {
			"runtime_release_id", "reviewer_image_digest", "adapter_identity", "adapter_version",
			"adapter_protocol", "adapter_capabilities_json", "model_credential_id", "primary_model",
			"provider_proxy_image_digest", "selection_digest", "worker_id",
		},
		"workforce_review_run_attempts": {
			"runtime_release_id", "reviewer_image_digest", "adapter_identity", "adapter_version",
			"adapter_protocol", "adapter_capabilities_json", "model_credential_id", "primary_model",
			"provider_proxy_image_digest", "selection_digest", "worker_id",
		},
	}
	for table, columns := range expected {
		for _, column := range columns {
			var exists bool
			if err := q.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM information_schema.columns WHERE table_schema='public' AND table_name=$1 AND column_name=$2)`, table, column).Scan(&exists); err != nil {
				return err
			}
			if !exists {
				return fmt.Errorf("Independent Review execution column is missing: %s.%s", table, column)
			}
		}
	}
	matched, exists, err := reviewRunTaskReadIndexMatches(ctx, q)
	if err != nil {
		return err
	}
	if !exists || !matched {
		return errors.New("Independent Review task read index is unavailable or malformed")
	}
	for _, removed := range []string{"review_profile_id", "review_profile_digest", "rubric_release_id", "rubric_release_digest", "review_system_contract", "review_report_schema_version", "review_command_sandbox_policy", "review_network_sandbox_policy"} {
		var exists bool
		if err := q.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM information_schema.columns WHERE table_schema='public' AND table_name='workforce_review_runs' AND column_name=$1)`, removed).Scan(&exists); err != nil {
			return err
		}
		if exists {
			return fmt.Errorf("Independent Review still contains removed policy column %s", removed)
		}
	}
	var definition string
	if err := q.QueryRow(ctx, `SELECT COALESCE((SELECT pg_get_constraintdef(oid) FROM pg_constraint WHERE conrelid='workforce_review_runs'::regclass AND conname='workforce_review_runs_assignment_ck' AND convalidated),'')`).Scan(&definition); err != nil {
		return err
	}
	definition = strings.ToLower(definition)
	for _, token := range []string{"runtime_release_id", "model_credential_id", "selection_digest", "worker_id", "assigned", "running"} {
		if !strings.Contains(definition, token) {
			return fmt.Errorf("Independent Review assignment constraint is incomplete: missing %s", token)
		}
	}
	return nil
}

func reviewRunTaskReadIndexMatches(ctx context.Context, q reviewSchemaQuery) (bool, bool, error) {
	var definition string
	err := q.QueryRow(ctx, `SELECT COALESCE((SELECT pg_get_indexdef(indexrelid) FROM pg_index WHERE indexrelid=to_regclass('public.workforce_review_runs_task_read_idx')),'')`).Scan(&definition)
	if err != nil {
		return false, false, err
	}
	if definition == "" {
		return false, false, nil
	}
	normalized := strings.ToLower(strings.Join(strings.Fields(definition), " "))
	return strings.Contains(normalized, "(tenant_id, task_id, id desc)") && !strings.Contains(normalized, "created_at"), true, nil
}

func verifyReviewRunSchemaBehavior(ctx context.Context, db DB) error {
	tx, err := db.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	return verifyReviewRunSchemaBehaviorInTx(ctx, tx)
}

func verifyReviewRunSchemaBehaviorInTx(ctx context.Context, tx pgx.Tx) error {
	var definition string
	if err := tx.QueryRow(ctx, `SELECT COALESCE((SELECT pg_get_constraintdef(oid) FROM pg_constraint WHERE conrelid='workforce_review_runs'::regclass AND conname='workforce_review_runs_assignment_ck' AND convalidated),'')`).Scan(&definition); err != nil {
		return err
	}
	if strings.TrimSpace(definition) == "" {
		return errors.New("Independent Review dynamic assignment constraint is unavailable")
	}
	return nil
}
