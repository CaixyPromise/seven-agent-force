package infrastructure

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"

	taskfacade "github.com/CaixyPromise/seven-agent/force/internal/app/workforce/task/facade"
)

func (s *Store) FrozenRepositoryPlan(ctx context.Context, tenantID, taskID, taskTurnID int64) (taskfacade.FrozenRepositoryPlan, error) {
	var plan taskfacade.FrozenRepositoryPlan
	err := s.db.QueryRow(ctx, `SELECT project_id,project_version_id,repository_provider_type,repository_scheme,repository_host,repository_port,
		repository_path,repository_display_url,repository_web_url,repository_config_digest,
		encode(repository_credential_ciphertext,'base64'),encode(repository_credential_edek,'base64'),repository_credential_wrap_key_ref,repository_credential_version,
		review_base_branch,review_base_commit,result_branch,delivery_mode,delivery_authorization,
		COALESCE(delivery_authorized_by_principal_id,0),previous_delivery_commit,plan_digest,target_config_digest,
		push_authorization_digest,review_authorization_digest
		FROM workforce_task_git_plans
		WHERE tenant_id=$1 AND task_id=$2 AND task_turn_id=$3 AND status='ACTIVE'`, tenantID, taskID, taskTurnID).Scan(
		&plan.ProjectID, &plan.ProjectVersionID, &plan.ProviderType, &plan.Scheme, &plan.Host, &plan.Port,
		&plan.RepositoryPath, &plan.DisplayURL, &plan.WebURL, &plan.RepositoryConfigDigest,
		&plan.CredentialCiphertext, &plan.CredentialEDEK, &plan.CredentialWrapKeyRef, &plan.CredentialVersion,
		&plan.BaseBranch, &plan.BaseCommit, &plan.ResultBranch, &plan.DeliveryMode, &plan.DeliveryAuthorization,
		&plan.AuthorizedPrincipalID, &plan.PreviousDeliveryCommit, &plan.PlanDigest, &plan.TargetConfigDigest,
		&plan.PushAuthorizationDigest, &plan.ReviewAuthorizationDigest)
	if errors.Is(err, pgx.ErrNoRows) {
		return plan, errors.New("TASK_FROZEN_REPOSITORY_NOT_FOUND")
	}
	return plan, err
}
