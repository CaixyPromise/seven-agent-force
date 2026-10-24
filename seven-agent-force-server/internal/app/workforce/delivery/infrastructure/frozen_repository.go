package infrastructure

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	projectfacade "github.com/CaixyPromise/seven-agent/force/internal/app/workforce/project/facade"
	"strconv"
	"strings"

	"github.com/CaixyPromise/seven-agent/force/internal/app/workforce/delivery/application"
	"github.com/CaixyPromise/seven-agent/force/internal/app/workforce/delivery/domain"
)

const (
	frozenCommitAuthorName  = "Seven Agent Force"
	frozenCommitAuthorEmail = "workforce@seven.invalid"
)

type frozenRepositoryPlan struct {
	ProjectID, ProjectVersionID int64
	ProviderType                string
	Scheme                      string
	Host                        string
	Port                        int
	RepositoryPath              string
	DisplayURL                  string
	WebURL                      string
	RepositoryConfigDigest      string
	Credential                  application.EncryptedCredentialEnvelope
	BaseBranch                  string
	BaseCommit                  string
	ResultBranch                string
	DeliveryMode                string
	DeliveryAuthorization       string
	AuthorizedPrincipalID       int64
	PreviousDeliveryCommit      string
	PlanDigest                  string
	TargetConfigDigest          string
	PushAuthorizationDigest     string
	ReviewAuthorizationDigest   string
}

func (p frozenRepositoryPlan) credentialVersion() string {
	return strconv.FormatInt(p.Credential.Version, 10)
}

func (s *Store) loadFrozenRepositoryPlan(ctx context.Context, tenantID, taskID, taskTurnID int64) (frozenRepositoryPlan, error) {
	if s.frozenRepositories == nil {
		return frozenRepositoryPlan{}, application.ErrNotFound
	}
	value, err := s.frozenRepositories.FrozenRepositoryPlan(ctx, tenantID, taskID, taskTurnID)
	if err != nil {
		return frozenRepositoryPlan{}, application.ErrNotFound
	}
	plan := frozenRepositoryPlan{
		ProjectID: value.ProjectID, ProjectVersionID: value.ProjectVersionID, ProviderType: value.ProviderType,
		Scheme: value.Scheme, Host: value.Host, Port: value.Port, RepositoryPath: value.RepositoryPath, DisplayURL: value.DisplayURL,
		WebURL: value.WebURL, RepositoryConfigDigest: value.RepositoryConfigDigest,
		Credential: application.EncryptedCredentialEnvelope{Ciphertext: value.CredentialCiphertext, EDEK: value.CredentialEDEK, WrapKeyRef: value.CredentialWrapKeyRef, Version: value.CredentialVersion},
		BaseBranch: value.BaseBranch, BaseCommit: value.BaseCommit, ResultBranch: value.ResultBranch, DeliveryMode: value.DeliveryMode,
		DeliveryAuthorization: value.DeliveryAuthorization, AuthorizedPrincipalID: value.AuthorizedPrincipalID,
		PreviousDeliveryCommit: value.PreviousDeliveryCommit, PlanDigest: value.PlanDigest, TargetConfigDigest: value.TargetConfigDigest,
		PushAuthorizationDigest: value.PushAuthorizationDigest, ReviewAuthorizationDigest: value.ReviewAuthorizationDigest,
	}
	if plan.ProjectID <= 0 || plan.ProjectVersionID <= 0 || plan.DeliveryMode == "NONE" ||
		plan.RepositoryConfigDigest == "" || plan.TargetConfigDigest != plan.RepositoryConfigDigest ||
		len(plan.PushAuthorizationDigest) != 64 || plan.Credential.Version <= 0 ||
		strings.TrimSpace(plan.Credential.Ciphertext) == "" || strings.TrimSpace(plan.Credential.EDEK) == "" || strings.TrimSpace(plan.Credential.WrapKeyRef) == "" {
		return frozenRepositoryPlan{}, domain.ErrIdentityConflict
	}
	if plan.DeliveryMode == "NEW_BRANCH_AND_DRAFT_REVIEW" && len(plan.ReviewAuthorizationDigest) != 64 {
		return frozenRepositoryPlan{}, domain.ErrIdentityConflict
	}
	return plan, nil
}

func (p frozenRepositoryPlan) target(tenantID int64) (domain.Target, error) {
	transport := domain.TransportHTTPS
	switch strings.ToLower(strings.TrimSpace(p.Scheme)) {
	case "https":
		transport = domain.TransportHTTPS
	case "http":
		transport = domain.TransportHTTP
	default:
		return domain.Target{}, domain.ErrInvalidTarget
	}
	egress, _ := json.Marshal(domain.EgressPolicy{
		AllowPrivateNetwork: true,
		AllowedHosts:        []string{p.Host},
		AllowedCIDRs:        []string{"0.0.0.0/0", "::/0"},
		AllowedPorts:        []int{p.Port},
	})
	egressSum := sha256.Sum256(egress)
	target := domain.Target{
		ID: p.ProjectID, TenantID: tenantID, Name: p.DisplayURL, ProviderType: p.ProviderType,
		Transport: transport, Host: p.Host, Port: p.Port, RepositoryPath: p.RepositoryPath,
		WebRepositoryURL: p.WebURL, CredentialVersion: p.credentialVersion(),
		TLSPolicy: json.RawMessage(`{}`), Egress: egress, EgressPolicyDigest: hex.EncodeToString(egressSum[:]),
		CommitAuthorName: frozenCommitAuthorName, CommitAuthorEmail: frozenCommitAuthorEmail,
		Status: "ENABLED", ConfigDigest: p.TargetConfigDigest,
	}
	if err := domain.ValidateTarget(target, false); err != nil {
		return domain.Target{}, err
	}
	return target, nil
}

// A new delivery authority may replace only the encrypted credential. The
// original Task GitPlan, result and remote identity remain immutable.
func bindDeliveryRepository(plan frozenRepositoryPlan, b projectfacade.DeliveryRepositoryBinding) (frozenRepositoryPlan, error) {
	if b.ProjectID != plan.ProjectID || b.VersionID <= 0 || b.CredentialVersion <= plan.Credential.Version || !b.Push || (plan.DeliveryMode == "NEW_BRANCH_AND_DRAFT_REVIEW" && !b.DraftReview) ||
		b.ProviderType != plan.ProviderType || b.Scheme != plan.Scheme || b.Host != plan.Host || b.Port != plan.Port || b.RepositoryPath != plan.RepositoryPath || b.WebURL != plan.WebURL ||
		len(b.ConfigDigest) != 64 || b.Ciphertext == "" || b.EDEK == "" || b.WrapKeyRef == "" {
		return frozenRepositoryPlan{}, domain.ErrReauthorization
	}
	plan.RepositoryConfigDigest = b.ConfigDigest
	plan.TargetConfigDigest = b.ConfigDigest
	plan.Credential = application.EncryptedCredentialEnvelope{Ciphertext: b.Ciphertext, EDEK: b.EDEK, WrapKeyRef: b.WrapKeyRef, Version: b.CredentialVersion}
	ref := fmt.Sprintf("project-version:%d", b.VersionID)
	plan.PushAuthorizationDigest = domain.AuthorizationDigest(b.ConfigDigest, "PUSH_BRANCH", ref, plan.credentialVersion())
	if plan.DeliveryMode == "NEW_BRANCH_AND_DRAFT_REVIEW" {
		plan.ReviewAuthorizationDigest = domain.AuthorizationDigest(b.ConfigDigest, "CREATE_REVIEW", ref, plan.credentialVersion())
	}
	return plan, nil
}

func (s *Store) loadDeliveryRepositoryPlan(ctx context.Context, tenant, task, turn, request int64) (frozenRepositoryPlan, error) {
	plan, err := s.loadFrozenRepositoryPlan(ctx, tenant, task, turn)
	if err != nil {
		return plan, err
	}
	var version *int64
	var authorized bool
	var target, credential, authorization string
	err = s.db.QueryRow(ctx, `SELECT reauthorized_project_version_id,reauthorized_at IS NOT NULL AND reauthorized_by_principal_id IS NOT NULL,target_config_digest,credential_version,authorization_digest FROM workforce_delivery_requests WHERE id=$1 AND tenant_id=$2 AND task_id=$3 AND task_turn_id=$4`, request, tenant, task, turn).Scan(&version, &authorized, &target, &credential, &authorization)
	if err != nil {
		return plan, err
	}
	if version == nil {
		return plan, nil
	}
	if !authorized || s.repositoryBindings == nil {
		return plan, domain.ErrReauthorization
	}
	b, err := s.repositoryBindings.DeliveryRepositoryBinding(ctx, tenant, plan.ProjectID, *version)
	if err != nil {
		return plan, domain.ErrReauthorization
	}
	plan, err = bindDeliveryRepository(plan, b)
	if err != nil || target != plan.TargetConfigDigest || credential != plan.credentialVersion() || authorization != plan.PushAuthorizationDigest {
		return frozenRepositoryPlan{}, domain.ErrReauthorization
	}
	return plan, nil
}
