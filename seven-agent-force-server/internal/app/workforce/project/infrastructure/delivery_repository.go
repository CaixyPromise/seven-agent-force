package infrastructure

import (
	"context"
	"encoding/json"
	"github.com/CaixyPromise/seven-agent/force/internal/app/workforce/project/domain"
	projectfacade "github.com/CaixyPromise/seven-agent/force/internal/app/workforce/project/facade"
)

func (s *Store) DeliveryRepositoryBinding(ctx context.Context, tenant, project, version int64) (projectfacade.DeliveryRepositoryBinding, error) {
	var b projectfacade.DeliveryRepositoryBinding
	var caps []byte
	err := s.db.QueryRow(ctx, `SELECT v.project_id,v.id,v.id=p.current_version_id,v.repository_provider_type,v.repository_scheme,v.repository_host,v.repository_port,v.repository_path,v.repository_web_url,v.repository_config_digest,v.repository_credential_version,
		encode(v.repository_credential_ciphertext,'base64'),encode(v.repository_credential_edek,'base64'),v.repository_credential_wrap_key_ref,v.repository_capabilities_json
		FROM workforce_project_versions v JOIN workforce_projects p ON p.id=v.project_id AND p.tenant_id=v.tenant_id
		WHERE v.tenant_id=$1 AND v.project_id=$2 AND v.id=$3 AND p.status='ACTIVE' AND v.kind='CODEBASE' AND v.repository_last_check_status='SUCCEEDED'`, tenant, project, version).Scan(&b.ProjectID, &b.VersionID, &b.Current, &b.ProviderType, &b.Scheme, &b.Host, &b.Port, &b.RepositoryPath, &b.WebURL, &b.ConfigDigest, &b.CredentialVersion, &b.Ciphertext, &b.EDEK, &b.WrapKeyRef, &caps)
	if err != nil {
		return b, err
	}
	var c domain.RepositoryCapabilities
	if json.Unmarshal(caps, &c) != nil || !c.Read || b.CredentialVersion <= 0 || b.Ciphertext == "" || b.EDEK == "" || b.WrapKeyRef == "" {
		return projectfacade.DeliveryRepositoryBinding{}, domain.ErrRepositoryCredentialRequired
	}
	b.Push, b.DraftReview = c.Push, c.DraftReview
	return b, nil
}
