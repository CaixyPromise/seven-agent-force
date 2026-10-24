package infrastructure

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/CaixyPromise/seven-agent/force/internal/app/external_login/domain"
	"github.com/jmoiron/sqlx"
)

type enterpriseConnectionRow struct {
	ID                         int64          `db:"id"`
	OrganizationID             int64          `db:"organization_id"`
	ProviderType               string         `db:"provider_type"`
	DisplayName                string         `db:"display_name"`
	AppID                      sql.NullString `db:"app_id"`
	CorpID                     sql.NullString `db:"corp_id"`
	AgentID                    sql.NullString `db:"agent_id"`
	EndpointProfile            string         `db:"endpoint_profile"`
	ClientSecretCiphertext     string         `db:"client_secret_ciphertext"`
	ClientSecretEDEK           string         `db:"client_secret_edek"`
	ClientSecretWrapKeyRef     string         `db:"client_secret_wrap_key_ref"`
	CredentialVersion          int64          `db:"credential_version"`
	BindingEnabled             bool           `db:"binding_enabled"`
	NotificationDirectEnabled  bool           `db:"notification_direct_enabled"`
	NotificationMentionEnabled bool           `db:"notification_mention_enabled"`
	LoginEnabled               bool           `db:"login_enabled"`
	ProtocolCapabilitiesJSON   string         `db:"protocol_capabilities_json"`
	ConfigDigest               string         `db:"config_digest"`
	Status                     string         `db:"status"`
	LastProbeStatus            string         `db:"last_probe_status"`
	LastProbeReasonCode        string         `db:"last_probe_reason_code"`
	LastProbeAt                sql.NullTime   `db:"last_probe_at"`
	Version                    int64          `db:"version"`
	CreatorID                  int64          `db:"creator_id"`
	UpdaterID                  int64          `db:"updater_id"`
	CreatedAt                  time.Time      `db:"created_at"`
	UpdatedAt                  time.Time      `db:"updated_at"`
}

func (r *Repository) ListEnterpriseConnections(ctx context.Context, organizationID int64) ([]domain.EnterpriseConnection, error) {
	exec, err := r.requireExecutor(ctx)
	if err != nil {
		return nil, err
	}
	var rows []enterpriseConnectionRow
	if err := sqlx.SelectContext(ctx, exec, &rows, r.rebind(exec, `SELECT id,organization_id,provider_type,display_name,app_id,corp_id,agent_id,endpoint_profile,client_secret_ciphertext,client_secret_edek,client_secret_wrap_key_ref,credential_version,binding_enabled,notification_direct_enabled,notification_mention_enabled,login_enabled,protocol_capabilities_json,config_digest,status,last_probe_status,last_probe_reason_code,last_probe_at,version,creator_id,updater_id,created_at,updated_at FROM sys_enterprise_identity_connection WHERE organization_id=? ORDER BY provider_type`), organizationID); err != nil {
		return nil, fmt.Errorf("list enterprise identity connections: %w", err)
	}
	result := make([]domain.EnterpriseConnection, 0, len(rows))
	for _, row := range rows {
		result = append(result, mapEnterpriseConnection(row))
	}
	return result, nil
}

func (r *Repository) FindEnterpriseConnection(ctx context.Context, organizationID int64, provider domain.EnterpriseProvider) (*domain.EnterpriseConnection, error) {
	exec, err := r.requireExecutor(ctx)
	if err != nil {
		return nil, err
	}
	var row enterpriseConnectionRow
	err = sqlx.GetContext(ctx, exec, &row, r.rebind(exec, `SELECT id,organization_id,provider_type,display_name,app_id,corp_id,agent_id,endpoint_profile,client_secret_ciphertext,client_secret_edek,client_secret_wrap_key_ref,credential_version,binding_enabled,notification_direct_enabled,notification_mention_enabled,login_enabled,protocol_capabilities_json,config_digest,status,last_probe_status,last_probe_reason_code,last_probe_at,version,creator_id,updater_id,created_at,updated_at FROM sys_enterprise_identity_connection WHERE organization_id=? AND provider_type=? LIMIT 1`), organizationID, string(provider))
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("find enterprise identity connection: %w", err)
	}
	item := mapEnterpriseConnection(row)
	return &item, nil
}

func (r *Repository) FindEnterpriseConnectionByID(ctx context.Context, organizationID, connectionID int64) (*domain.EnterpriseConnection, error) {
	exec, err := r.requireExecutor(ctx)
	if err != nil {
		return nil, err
	}
	var row enterpriseConnectionRow
	err = sqlx.GetContext(ctx, exec, &row, r.rebind(exec, `SELECT id,organization_id,provider_type,display_name,app_id,corp_id,agent_id,endpoint_profile,client_secret_ciphertext,client_secret_edek,client_secret_wrap_key_ref,credential_version,binding_enabled,notification_direct_enabled,notification_mention_enabled,login_enabled,protocol_capabilities_json,config_digest,status,last_probe_status,last_probe_reason_code,last_probe_at,version,creator_id,updater_id,created_at,updated_at FROM sys_enterprise_identity_connection WHERE organization_id=? AND id=? LIMIT 1`), organizationID, connectionID)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("find enterprise identity connection: %w", err)
	}
	item := mapEnterpriseConnection(row)
	return &item, nil
}

func (r *Repository) InsertEnterpriseConnection(ctx context.Context, item domain.EnterpriseConnection) error {
	exec, err := r.requireExecutor(ctx)
	if err != nil {
		return err
	}
	_, err = exec.ExecContext(ctx, r.rebind(exec, `INSERT INTO sys_enterprise_identity_connection (id,organization_id,provider_type,connection_role,display_name,app_id,corp_id,agent_id,endpoint_profile,client_secret_ciphertext,client_secret_edek,client_secret_wrap_key_ref,credential_version,binding_enabled,notification_direct_enabled,notification_mention_enabled,login_enabled,protocol_capabilities_json,config_digest,status,last_probe_status,last_probe_reason_code,last_probe_at,version,creator_id,updater_id,created_at,updated_at) VALUES (?,?,?,'PRIMARY',?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`), item.ID, item.OrganizationID, string(item.Provider), item.DisplayName, nullIfBlank(item.AppID), nullIfBlank(item.CorpID), nullIfBlank(item.AgentID), item.EndpointProfile, item.ClientSecretCiphertext, item.ClientSecretEDEK, item.ClientSecretWrapKeyRef, item.CredentialVersion, item.BindingEnabled, item.NotificationDirectEnabled, item.NotificationMentionEnabled, item.LoginEnabled, jsonString(item.ProtocolCapabilities), item.ConfigDigest, string(item.Status), item.LastProbeStatus, item.LastProbeReasonCode, item.LastProbeAt, item.Version, item.CreatorID, item.UpdaterID, item.CreatedAt, item.UpdatedAt)
	if err != nil {
		return fmt.Errorf("insert enterprise identity connection: %w", err)
	}
	return nil
}

func (r *Repository) UpdateEnterpriseConnection(ctx context.Context, item domain.EnterpriseConnection, expectedVersion int64) (bool, error) {
	exec, err := r.requireExecutor(ctx)
	if err != nil {
		return false, err
	}
	res, err := exec.ExecContext(ctx, r.rebind(exec, `UPDATE sys_enterprise_identity_connection SET display_name=?,app_id=?,corp_id=?,agent_id=?,endpoint_profile=?,client_secret_ciphertext=?,client_secret_edek=?,client_secret_wrap_key_ref=?,credential_version=?,binding_enabled=?,notification_direct_enabled=?,notification_mention_enabled=?,login_enabled=?,protocol_capabilities_json=?,config_digest=?,status=?,last_probe_status=?,last_probe_reason_code=?,last_probe_at=?,version=version+1,updater_id=?,updated_at=? WHERE id=? AND organization_id=? AND version=?`), item.DisplayName, nullIfBlank(item.AppID), nullIfBlank(item.CorpID), nullIfBlank(item.AgentID), item.EndpointProfile, item.ClientSecretCiphertext, item.ClientSecretEDEK, item.ClientSecretWrapKeyRef, item.CredentialVersion, item.BindingEnabled, item.NotificationDirectEnabled, item.NotificationMentionEnabled, item.LoginEnabled, jsonString(item.ProtocolCapabilities), item.ConfigDigest, string(item.Status), item.LastProbeStatus, item.LastProbeReasonCode, item.LastProbeAt, item.UpdaterID, item.UpdatedAt, item.ID, item.OrganizationID, expectedVersion)
	if err != nil {
		return false, fmt.Errorf("update enterprise identity connection: %w", err)
	}
	n, _ := res.RowsAffected()
	return n == 1, nil
}

type enterpriseAuthorizationRow struct {
	ID                     int64          `db:"id"`
	AuthorizationIdentity  string         `db:"authorization_identity"`
	Purpose                string         `db:"purpose"`
	OrganizationID         int64          `db:"organization_id"`
	ConnectionID           int64          `db:"connection_id"`
	ConnectionVersion      int64          `db:"connection_version"`
	ConnectionConfigDigest string         `db:"connection_config_digest"`
	BindUserID             sql.NullInt64  `db:"bind_user_id"`
	RequestedGrantsJSON    string         `db:"requested_grants_json"`
	RequestedGrantsDigest  string         `db:"requested_grants_digest"`
	StateHash              string         `db:"state_hash"`
	NonceHash              string         `db:"nonce_hash"`
	CodeVerifierCiphertext sql.NullString `db:"code_verifier_ciphertext"`
	CodeVerifierEDEK       sql.NullString `db:"code_verifier_edek"`
	CodeVerifierWrapKeyRef sql.NullString `db:"code_verifier_wrap_key_ref"`
	RedirectAfter          string         `db:"redirect_after"`
	ExpiresAt              time.Time      `db:"expires_at"`
	Status                 string         `db:"status"`
	ResultIdentityID       sql.NullInt64  `db:"result_identity_id"`
	ReasonCode             string         `db:"reason_code"`
	ConsumedAt             sql.NullTime   `db:"consumed_at"`
	CreatedAt              time.Time      `db:"created_at"`
	UpdatedAt              time.Time      `db:"updated_at"`
}

func (r *Repository) InsertEnterpriseAuthorization(ctx context.Context, item domain.EnterpriseAuthorization) error {
	exec, err := r.requireExecutor(ctx)
	if err != nil {
		return err
	}
	_, err = exec.ExecContext(ctx, r.rebind(exec, `INSERT INTO sys_enterprise_identity_authorization (id,authorization_identity,purpose,organization_id,connection_id,connection_version,connection_config_digest,bind_user_id,requested_grants_json,requested_grants_digest,state_hash,nonce_hash,code_verifier_ciphertext,code_verifier_edek,code_verifier_wrap_key_ref,redirect_after,expires_at,status,result_identity_id,reason_code,consumed_at,created_at,updated_at) VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`), item.ID, item.AuthorizationIdentity, string(item.Purpose), item.OrganizationID, item.ConnectionID, item.ConnectionVersion, item.ConnectionConfigDigest, nullInt64(item.BindUserID), jsonString(item.RequestedGrants), item.RequestedGrantsDigest, item.StateHash, item.NonceHash, nullIfBlank(item.CodeVerifierCiphertext), nullIfBlank(item.CodeVerifierEDEK), nullIfBlank(item.CodeVerifierWrapKeyRef), item.RedirectAfter, item.ExpiresAt, string(item.Status), item.ResultIdentityID, item.ReasonCode, item.ConsumedAt, item.CreatedAt, item.UpdatedAt)
	if err != nil {
		return fmt.Errorf("insert enterprise authorization: %w", err)
	}
	return nil
}

func (r *Repository) ConsumeEnterpriseAuthorization(ctx context.Context, stateHash string, now time.Time) (*domain.EnterpriseAuthorization, error) {
	var result *domain.EnterpriseAuthorization
	err := r.withinTransaction(ctx, func(txCtx context.Context) error {
		exec, err := r.requireExecutor(txCtx)
		if err != nil {
			return err
		}
		var row enterpriseAuthorizationRow
		query := `SELECT id,authorization_identity,purpose,organization_id,connection_id,connection_version,connection_config_digest,bind_user_id,requested_grants_json,requested_grants_digest,state_hash,nonce_hash,code_verifier_ciphertext,code_verifier_edek,code_verifier_wrap_key_ref,redirect_after,expires_at,status,result_identity_id,reason_code,consumed_at,created_at,updated_at FROM sys_enterprise_identity_authorization WHERE state_hash=? FOR UPDATE`
		if err := sqlx.GetContext(txCtx, exec, &row, r.rebind(exec, query), stateHash); err != nil {
			if err == sql.ErrNoRows {
				return nil
			}
			return err
		}
		if row.Status != string(domain.EnterpriseAuthorizationActive) || !row.ExpiresAt.After(now) {
			return nil
		}
		res, err := exec.ExecContext(txCtx, r.rebind(exec, `UPDATE sys_enterprise_identity_authorization SET status='PROCESSING',consumed_at=?,updated_at=? WHERE id=? AND status='ACTIVE'`), now, now, row.ID)
		if err != nil {
			return err
		}
		n, _ := res.RowsAffected()
		if n != 1 {
			return nil
		}
		item, err := mapEnterpriseAuthorization(row)
		if err != nil {
			return err
		}
		item.Status = domain.EnterpriseAuthorizationProcessing
		item.ConsumedAt = &now
		result = &item
		return nil
	})
	return result, err
}

func (r *Repository) CompleteEnterpriseAuthorization(ctx context.Context, id int64, status domain.EnterpriseAuthorizationStatus, identityID *int64, reason string, now time.Time) error {
	exec, err := r.requireExecutor(ctx)
	if err != nil {
		return err
	}
	_, err = exec.ExecContext(ctx, r.rebind(exec, `UPDATE sys_enterprise_identity_authorization SET status=?,result_identity_id=?,reason_code=?,updated_at=? WHERE id=? AND status='PROCESSING'`), string(status), identityID, reason, now, id)
	return err
}

type enterpriseIdentityRow struct {
	ID                  int64          `db:"id"`
	OrganizationID      int64          `db:"organization_id"`
	ConnectionID        int64          `db:"enterprise_connection_id"`
	ProviderCode        string         `db:"providerCode"`
	UserID              int64          `db:"userId"`
	SubjectKind         sql.NullString `db:"subject_kind"`
	SubjectCiphertext   sql.NullString `db:"subject_ciphertext"`
	SubjectEDEK         sql.NullString `db:"subject_edek"`
	SubjectWrapKeyRef   sql.NullString `db:"subject_wrap_key_ref"`
	SubjectDigest       sql.NullString `db:"subject_digest"`
	SubjectDigestKeyRef sql.NullString `db:"subject_digest_key_ref"`
	DisplayHint         sql.NullString `db:"display_hint"`
	Status              int            `db:"status"`
	IdentityVersion     int64          `db:"identity_version"`
	VerifiedAt          sql.NullTime   `db:"verified_at"`
	RevokedAt           sql.NullTime   `db:"revoked_at"`
	CreatorID           sql.NullInt64  `db:"creatorId"`
	UpdaterID           sql.NullInt64  `db:"updaterId"`
	CreatedAt           time.Time      `db:"createTime"`
	UpdatedAt           time.Time      `db:"updateTime"`
}

func (r *Repository) FindEnterpriseIdentityByDigest(ctx context.Context, organizationID, connectionID int64, digest string) (*domain.EnterpriseExternalIdentity, error) {
	return r.findEnterpriseIdentity(ctx, `organization_id=? AND enterprise_connection_id=? AND subject_digest=? AND scope_type='ORGANIZATION' AND isDeleted=0`, organizationID, connectionID, digest)
}
func (r *Repository) FindEnterpriseIdentityByUser(ctx context.Context, organizationID, connectionID, userID int64) (*domain.EnterpriseExternalIdentity, error) {
	return r.findEnterpriseIdentity(ctx, `organization_id=? AND enterprise_connection_id=? AND userId=? AND scope_type='ORGANIZATION' AND isDeleted=0`, organizationID, connectionID, userID)
}
func (r *Repository) FindEnterpriseIdentityByID(ctx context.Context, organizationID, identityID int64) (*domain.EnterpriseExternalIdentity, error) {
	return r.findEnterpriseIdentity(ctx, `organization_id=? AND id=? AND scope_type='ORGANIZATION' AND isDeleted=0`, organizationID, identityID)
}

func (r *Repository) findEnterpriseIdentity(ctx context.Context, where string, args ...any) (*domain.EnterpriseExternalIdentity, error) {
	exec, err := r.requireExecutor(ctx)
	if err != nil {
		return nil, err
	}
	var row enterpriseIdentityRow
	q := `SELECT id,organization_id,enterprise_connection_id,providerCode,userId,subject_kind,subject_ciphertext,subject_edek,subject_wrap_key_ref,subject_digest,subject_digest_key_ref,display_hint,status,identity_version,verified_at,revoked_at,creatorId,updaterId,createTime,updateTime FROM sys_external_user_identity WHERE ` + where + ` LIMIT 1`
	if err = sqlx.GetContext(ctx, exec, &row, r.rebind(exec, q), args...); err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	item := mapEnterpriseIdentity(row)
	return &item, nil
}

func (r *Repository) ListEnterpriseIdentitiesByUser(ctx context.Context, organizationID, userID int64) ([]domain.EnterpriseExternalIdentity, error) {
	exec, err := r.requireExecutor(ctx)
	if err != nil {
		return nil, err
	}
	var rows []enterpriseIdentityRow
	err = sqlx.SelectContext(ctx, exec, &rows, r.rebind(exec, `SELECT id,organization_id,enterprise_connection_id,providerCode,userId,subject_kind,subject_ciphertext,subject_edek,subject_wrap_key_ref,subject_digest,subject_digest_key_ref,display_hint,status,identity_version,verified_at,revoked_at,creatorId,updaterId,createTime,updateTime FROM sys_external_user_identity WHERE organization_id=? AND userId=? AND scope_type='ORGANIZATION' AND isDeleted=0 ORDER BY providerCode`), organizationID, userID)
	if err != nil {
		return nil, err
	}
	out := make([]domain.EnterpriseExternalIdentity, 0, len(rows))
	for _, row := range rows {
		out = append(out, mapEnterpriseIdentity(row))
	}
	return out, nil
}

func (r *Repository) ListEnterpriseIdentitiesByOrganization(ctx context.Context, organizationID int64) ([]domain.EnterpriseExternalIdentity, error) {
	exec, err := r.requireExecutor(ctx)
	if err != nil {
		return nil, err
	}
	var rows []enterpriseIdentityRow
	err = sqlx.SelectContext(ctx, exec, &rows, r.rebind(exec, `SELECT id,organization_id,enterprise_connection_id,providerCode,userId,subject_kind,subject_ciphertext,subject_edek,subject_wrap_key_ref,subject_digest,subject_digest_key_ref,display_hint,status,identity_version,verified_at,revoked_at,creatorId,updaterId,createTime,updateTime FROM sys_external_user_identity WHERE organization_id=? AND scope_type='ORGANIZATION' AND isDeleted=0 ORDER BY userId,providerCode`), organizationID)
	if err != nil {
		return nil, err
	}
	out := make([]domain.EnterpriseExternalIdentity, 0, len(rows))
	for _, row := range rows {
		out = append(out, mapEnterpriseIdentity(row))
	}
	return out, nil
}

func (r *Repository) InsertEnterpriseIdentity(ctx context.Context, item domain.EnterpriseExternalIdentity) error {
	exec, err := r.requireExecutor(ctx)
	if err != nil {
		return err
	}
	_, err = exec.ExecContext(ctx, r.rebind(exec, insertEnterpriseIdentitySQL), item.ID, string(item.Provider), item.UserID, item.VerifiedAt, item.VerifiedAt, item.CreatorID, item.UpdaterID, item.CreatedAt, item.UpdatedAt, item.OrganizationID, item.ConnectionID, item.SubjectKind, item.SubjectCiphertext, item.SubjectEDEK, item.SubjectWrapKeyRef, item.SubjectDigest, item.SubjectDigestKeyRef, item.DisplayHint, item.IdentityVersion, item.VerifiedAt)
	return err
}

const insertEnterpriseIdentitySQL = `INSERT INTO sys_external_user_identity (id,providerCode,externalSubject,userId,externalLogin,externalEmail,emailVerified,displayName,avatarUrl,profileJson,status,firstLinkedAt,lastVerifiedAt,metadataJson,creatorId,updaterId,createTime,updateTime,isDeleted,scope_type,organization_id,enterprise_connection_id,subject_kind,subject_ciphertext,subject_edek,subject_wrap_key_ref,subject_digest,subject_digest_key_ref,display_hint,identity_version,verified_at,revoked_at) VALUES (?,?,NULL,?,'','',0,'','',NULL,0,?,?,NULL,?,?,?,?,0,'ORGANIZATION',?,?,?,?,?,?,?,?,?,?,?,NULL)`

func (r *Repository) UpdateEnterpriseIdentityStatus(ctx context.Context, organizationID, identityID, expectedVersion, actorID int64, status domain.EnterpriseIdentityStatus, now time.Time) (bool, error) {
	numeric := 0
	if status == domain.EnterpriseIdentityRevoked {
		numeric = 2
	} else if status == domain.EnterpriseIdentityDisabled {
		numeric = 1
	}
	exec, err := r.requireExecutor(ctx)
	if err != nil {
		return false, err
	}
	res, err := exec.ExecContext(ctx, r.rebind(exec, `UPDATE sys_external_user_identity SET status=?,identity_version=identity_version+1,revoked_at=?,updaterId=?,updateTime=? WHERE id=? AND organization_id=? AND identity_version=? AND scope_type='ORGANIZATION' AND isDeleted=0`), numeric, nullableWhen(status == domain.EnterpriseIdentityRevoked, now), actorID, now, identityID, organizationID, expectedVersion)
	if err != nil {
		return false, err
	}
	n, _ := res.RowsAffected()
	return n == 1, nil
}

func (r *Repository) AdvanceEnterpriseIdentityVersion(ctx context.Context, organizationID, identityID, expectedVersion, actorID int64, now time.Time) (bool, error) {
	exec, err := r.requireExecutor(ctx)
	if err != nil {
		return false, err
	}
	result, err := exec.ExecContext(ctx, r.rebind(exec, `UPDATE sys_external_user_identity SET identity_version=identity_version+1,updaterId=?,updateTime=? WHERE id=? AND organization_id=? AND identity_version=? AND scope_type='ORGANIZATION' AND status=0 AND isDeleted=0`), actorID, now, identityID, organizationID, expectedVersion)
	if err != nil {
		return false, err
	}
	affected, err := result.RowsAffected()
	return affected == 1, err
}

type enterpriseGrantRow struct {
	ID                 int64         `db:"id"`
	OrganizationID     int64         `db:"organization_id"`
	ExternalIdentityID int64         `db:"external_identity_id"`
	GrantType          string        `db:"grant_type"`
	Status             string        `db:"status"`
	Version            int64         `db:"version"`
	GrantedBy          int64         `db:"granted_by"`
	GrantedAt          time.Time     `db:"granted_at"`
	RevokedBy          sql.NullInt64 `db:"revoked_by"`
	RevokedAt          sql.NullTime  `db:"revoked_at"`
	ReasonCode         string        `db:"reason_code"`
	CreatedAt          time.Time     `db:"created_at"`
	UpdatedAt          time.Time     `db:"updated_at"`
}

func (r *Repository) ListEnterpriseGrants(ctx context.Context, organizationID, identityID int64) ([]domain.EnterpriseIdentityGrant, error) {
	exec, err := r.requireExecutor(ctx)
	if err != nil {
		return nil, err
	}
	var rows []enterpriseGrantRow
	err = sqlx.SelectContext(ctx, exec, &rows, r.rebind(exec, `SELECT id,organization_id,external_identity_id,grant_type,status,version,granted_by,granted_at,revoked_by,revoked_at,reason_code,created_at,updated_at FROM sys_enterprise_identity_grant WHERE organization_id=? AND external_identity_id=?`), organizationID, identityID)
	if err != nil {
		return nil, err
	}
	out := make([]domain.EnterpriseIdentityGrant, 0, len(rows))
	for _, row := range rows {
		out = append(out, mapEnterpriseGrant(row))
	}
	return out, nil
}

func (r *Repository) UpsertEnterpriseGrant(ctx context.Context, item domain.EnterpriseIdentityGrant) error {
	exec, err := r.requireExecutor(ctx)
	if err != nil {
		return err
	}
	if r.postgres {
		_, err = exec.ExecContext(ctx, r.rebind(exec, `INSERT INTO sys_enterprise_identity_grant (id,organization_id,external_identity_id,grant_type,status,version,granted_by,granted_at,revoked_by,revoked_at,reason_code,created_at,updated_at) VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?) ON CONFLICT (organization_id,external_identity_id,grant_type) DO UPDATE SET status=EXCLUDED.status,version=sys_enterprise_identity_grant.version+1,granted_by=EXCLUDED.granted_by,granted_at=EXCLUDED.granted_at,revoked_by=EXCLUDED.revoked_by,revoked_at=EXCLUDED.revoked_at,reason_code=EXCLUDED.reason_code,updated_at=EXCLUDED.updated_at`), item.ID, item.OrganizationID, item.ExternalIdentityID, string(item.GrantType), string(item.Status), item.Version, item.GrantedBy, item.GrantedAt, item.RevokedBy, item.RevokedAt, item.ReasonCode, item.CreatedAt, item.UpdatedAt)
	} else {
		_, err = exec.ExecContext(ctx, r.rebind(exec, `INSERT INTO sys_enterprise_identity_grant (id,organization_id,external_identity_id,grant_type,status,version,granted_by,granted_at,revoked_by,revoked_at,reason_code,created_at,updated_at) VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?) ON DUPLICATE KEY UPDATE status=VALUES(status),version=version+1,granted_by=VALUES(granted_by),granted_at=VALUES(granted_at),revoked_by=VALUES(revoked_by),revoked_at=VALUES(revoked_at),reason_code=VALUES(reason_code),updated_at=VALUES(updated_at)`), item.ID, item.OrganizationID, item.ExternalIdentityID, string(item.GrantType), string(item.Status), item.Version, item.GrantedBy, item.GrantedAt, item.RevokedBy, item.RevokedAt, item.ReasonCode, item.CreatedAt, item.UpdatedAt)
	}
	return err
}

func mapEnterpriseConnection(row enterpriseConnectionRow) domain.EnterpriseConnection {
	var caps domain.EnterpriseProtocolCapabilities
	_ = jsonUnmarshal(row.ProtocolCapabilitiesJSON, &caps)
	return domain.EnterpriseConnection{ID: row.ID, OrganizationID: row.OrganizationID, Provider: domain.EnterpriseProvider(row.ProviderType), DisplayName: row.DisplayName, AppID: row.AppID.String, CorpID: row.CorpID.String, AgentID: row.AgentID.String, EndpointProfile: row.EndpointProfile, ClientSecretCiphertext: row.ClientSecretCiphertext, ClientSecretEDEK: row.ClientSecretEDEK, ClientSecretWrapKeyRef: row.ClientSecretWrapKeyRef, CredentialVersion: row.CredentialVersion, BindingEnabled: row.BindingEnabled, NotificationDirectEnabled: row.NotificationDirectEnabled, NotificationMentionEnabled: row.NotificationMentionEnabled, LoginEnabled: row.LoginEnabled, ProtocolCapabilities: caps, ConfigDigest: row.ConfigDigest, Status: domain.EnterpriseConnectionStatus(row.Status), LastProbeStatus: row.LastProbeStatus, LastProbeReasonCode: row.LastProbeReasonCode, LastProbeAt: nullTimePtr(row.LastProbeAt), Version: row.Version, CreatorID: row.CreatorID, UpdaterID: row.UpdaterID, CreatedAt: row.CreatedAt, UpdatedAt: row.UpdatedAt}
}
func mapEnterpriseAuthorization(row enterpriseAuthorizationRow) (domain.EnterpriseAuthorization, error) {
	var grants []domain.EnterpriseGrantType
	if err := jsonUnmarshal(row.RequestedGrantsJSON, &grants); err != nil {
		return domain.EnterpriseAuthorization{}, err
	}
	return domain.EnterpriseAuthorization{ID: row.ID, AuthorizationIdentity: row.AuthorizationIdentity, Purpose: domain.AuthorizationPurpose(row.Purpose), OrganizationID: row.OrganizationID, ConnectionID: row.ConnectionID, ConnectionVersion: row.ConnectionVersion, ConnectionConfigDigest: row.ConnectionConfigDigest, BindUserID: row.BindUserID.Int64, RequestedGrants: grants, RequestedGrantsDigest: row.RequestedGrantsDigest, StateHash: row.StateHash, NonceHash: row.NonceHash, CodeVerifierCiphertext: row.CodeVerifierCiphertext.String, CodeVerifierEDEK: row.CodeVerifierEDEK.String, CodeVerifierWrapKeyRef: row.CodeVerifierWrapKeyRef.String, RedirectAfter: row.RedirectAfter, ExpiresAt: row.ExpiresAt, Status: domain.EnterpriseAuthorizationStatus(row.Status), ResultIdentityID: nullInt64Ptr(row.ResultIdentityID), ReasonCode: row.ReasonCode, ConsumedAt: nullTimePtr(row.ConsumedAt), CreatedAt: row.CreatedAt, UpdatedAt: row.UpdatedAt}, nil
}
func mapEnterpriseIdentity(row enterpriseIdentityRow) domain.EnterpriseExternalIdentity {
	status := domain.EnterpriseIdentityActive
	if row.Status == 1 {
		status = domain.EnterpriseIdentityDisabled
	} else if row.Status == 2 {
		status = domain.EnterpriseIdentityRevoked
	}
	return domain.EnterpriseExternalIdentity{ID: row.ID, OrganizationID: row.OrganizationID, ConnectionID: row.ConnectionID, Provider: domain.EnterpriseProvider(row.ProviderCode), UserID: row.UserID, SubjectKind: row.SubjectKind.String, SubjectCiphertext: row.SubjectCiphertext.String, SubjectEDEK: row.SubjectEDEK.String, SubjectWrapKeyRef: row.SubjectWrapKeyRef.String, SubjectDigest: row.SubjectDigest.String, SubjectDigestKeyRef: row.SubjectDigestKeyRef.String, DisplayHint: row.DisplayHint.String, Status: status, IdentityVersion: row.IdentityVersion, VerifiedAt: row.VerifiedAt.Time, RevokedAt: nullTimePtr(row.RevokedAt), CreatorID: row.CreatorID.Int64, UpdaterID: row.UpdaterID.Int64, CreatedAt: row.CreatedAt, UpdatedAt: row.UpdatedAt}
}
func mapEnterpriseGrant(row enterpriseGrantRow) domain.EnterpriseIdentityGrant {
	return domain.EnterpriseIdentityGrant{ID: row.ID, OrganizationID: row.OrganizationID, ExternalIdentityID: row.ExternalIdentityID, GrantType: domain.EnterpriseGrantType(row.GrantType), Status: domain.EnterpriseGrantStatus(row.Status), Version: row.Version, GrantedBy: row.GrantedBy, GrantedAt: row.GrantedAt, RevokedBy: nullInt64Ptr(row.RevokedBy), RevokedAt: nullTimePtr(row.RevokedAt), ReasonCode: row.ReasonCode, CreatedAt: row.CreatedAt, UpdatedAt: row.UpdatedAt}
}
func nullTimePtr(v sql.NullTime) *time.Time {
	if !v.Valid {
		return nil
	}
	value := v.Time
	return &value
}
func nullInt64Ptr(v sql.NullInt64) *int64 {
	if !v.Valid {
		return nil
	}
	value := v.Int64
	return &value
}
func nullInt64(v int64) any {
	if v <= 0 {
		return nil
	}
	return v
}
func nullableWhen(ok bool, v time.Time) any {
	if !ok {
		return nil
	}
	return v
}
func jsonUnmarshal(raw string, target any) error {
	return sonicUnmarshal([]byte(strings.TrimSpace(raw)), target)
}

var sonicUnmarshal = func(raw []byte, target any) error { return json.Unmarshal(raw, target) }
