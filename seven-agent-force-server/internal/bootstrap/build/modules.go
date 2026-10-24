package build

import (
	"context"
	"fmt"
	"time"

	"github.com/CaixyPromise/seven-agent/force/internal/app/authorization"
	"github.com/CaixyPromise/seven-agent/force/internal/app/challenge"
	"github.com/CaixyPromise/seven-agent/force/internal/app/credential"
	externallogin "github.com/CaixyPromise/seven-agent/force/internal/app/external_login"
	externalfacade "github.com/CaixyPromise/seven-agent/force/internal/app/external_login/facade"
	filemodule "github.com/CaixyPromise/seven-agent/force/internal/app/file"
	hubcontrol "github.com/CaixyPromise/seven-agent/force/internal/app/hub_control"
	"github.com/CaixyPromise/seven-agent/force/internal/app/kernel"
	"github.com/CaixyPromise/seven-agent/force/internal/app/login"
	nodemanagement "github.com/CaixyPromise/seven-agent/force/internal/app/node_management"
	"github.com/CaixyPromise/seven-agent/force/internal/app/notification"
	notificationapp "github.com/CaixyPromise/seven-agent/force/internal/app/notification/application"
	"github.com/CaixyPromise/seven-agent/force/internal/app/observability"
	"github.com/CaixyPromise/seven-agent/force/internal/app/platform"
	platformfacade "github.com/CaixyPromise/seven-agent/force/internal/app/platform/facade"
	"github.com/CaixyPromise/seven-agent/force/internal/app/setup"
	"github.com/CaixyPromise/seven-agent/force/internal/app/sso"
	ssofacade "github.com/CaixyPromise/seven-agent/force/internal/app/sso/facade"
	systemadmin "github.com/CaixyPromise/seven-agent/force/internal/app/system/admin"
	cachegovernance "github.com/CaixyPromise/seven-agent/force/internal/app/system/cache_governance"
	systemconfig "github.com/CaixyPromise/seven-agent/force/internal/app/system/config"
	systemdict "github.com/CaixyPromise/seven-agent/force/internal/app/system/dict"
	systemuser "github.com/CaixyPromise/seven-agent/force/internal/app/system/user"
	userfacade "github.com/CaixyPromise/seven-agent/force/internal/app/system/user/facade"
	"github.com/CaixyPromise/seven-agent/force/internal/app/workforce"
	bootstrapruntime "github.com/CaixyPromise/seven-agent/force/internal/bootstrap/runtime"
	cacheinfra "github.com/CaixyPromise/seven-agent/force/internal/infrastructure/cache"
	"github.com/CaixyPromise/seven-agent/force/internal/shared/core"
	"github.com/CaixyPromise/seven-agent/force/internal/shared/features"
)

func Modules(deps bootstrapruntime.ModuleDeps) ([]core.Module, error) {
	featureSet := features.OrResolve(deps.Features, deps.Config)
	deps.Features = featureSet
	if featureSet.Enabled(features.FederationNode) && !deps.Config.ExternalLogin.Enabled {
		return nil, fmt.Errorf("node federation requires external login to be enabled")
	}
	modules := make([]core.Module, 0, 10)

	credentialModule, credentialFacade, err := credential.Install(deps)
	if err != nil {
		return nil, err
	}
	modules = append(modules, credentialModule)

	userModule, userFacades, err := systemuser.Install(deps)
	if err != nil {
		return nil, err
	}
	userModule.BindCredentialFacade(credentialFacade)
	modules = append(modules, userModule)

	notificationModule, notificationFacade, err := notification.Install(deps, notification.Dependencies{
		Audiences:      userFacades.NotificationAudience,
		VerifiedEmails: userFacades.NotificationRecipients,
	})
	if err != nil {
		return nil, err
	}
	if notificationModule != nil {
		modules = append(modules, notificationModule)
	}

	challengeModule, err := challenge.Install(deps, challenge.Dependencies{
		UserCredentials: credentialFacade,
		Subjects:        userFacades.Subjects,
		Notifications:   notificationFacade,
	})
	if err != nil {
		return nil, err
	}
	modules = append(modules, challengeModule)

	ssoModule, ssoFacades, err := sso.Install(deps, sso.Dependencies{
		Profiles: userFacades.Profiles,
		Subjects: userFacades.Subjects,
	})
	if err != nil {
		return nil, err
	}
	if ssoModule != nil {
		modules = append(modules, ssoModule)
		userModule.BindSessions(ssoFacades.Sessions)
	}

	authorizationModule, authFacade, permissionFacade, roleFacade, accessExplainFacade, err := authorization.Install(deps, authorization.Dependencies{
		SsoTokens:   ssoFacades.Tokens,
		SsoSessions: ssoFacades.Sessions,
		Challenges:  challengeModule.InternalFacade(),
		Proof:       challengeModule.ProofTokenVerifier(),
		Users:       userFacades.Context,
	})
	if err != nil {
		return nil, err
	}
	if authorizationModule != nil {
		modules = append(modules, authorizationModule)
		if notificationModule != nil {
			notificationModule.BindAuthorization(authFacade)
			notificationModule.BindPermissions(permissionFacade)
		}
		userModule.BindAuthorization(authFacade)
		userModule.BindPermissions(permissionFacade)
		userModule.BindRoleAssignments(roleFacade)
		userModule.BindAccessExplain(accessExplainFacade)
		challengeModule.BindAuthorization(authFacade)
		if ssoModule != nil {
			ssoModule.BindAuthorization(authFacade)
		}
	}

	cacheGovernanceModule, cacheInvalidations, err := cachegovernance.Install(deps)
	if err != nil {
		return nil, err
	}
	if cacheGovernanceModule != nil {
		modules = append(modules, cacheGovernanceModule)
		if ssoModule != nil {
			if targeted, ok := deps.Infra.CacheMgr.(cacheinfra.TargetedGovernedCache); ok {
				ssoModule.BindActiveSessionValidityGovernance(cacheGovernanceModule.TargetedInvalidations(), targeted)
			}
		}
	}
	// Cache governance is composed after authorization/user construction; bind
	// only its public Facade so authoritative writes can register durable
	// invalidations without reverse application dependencies.
	if authorizationModule != nil {
		authorizationModule.BindCacheInvalidations(cacheInvalidations)
	}
	if userModule != nil {
		userModule.BindCacheInvalidations(cacheInvalidations)
	}

	dictModule, _, err := systemdict.Install(deps, systemdict.Dependencies{CacheInvalidations: cacheInvalidations})
	if err != nil {
		return nil, err
	}
	if dictModule != nil {
		modules = append(modules, dictModule)
	}

	configModule, configFacade, err := systemconfig.Install(deps, systemconfig.Dependencies{
		Profiles:           userFacades.Profiles,
		CacheInvalidations: cacheInvalidations,
	})
	if err != nil {
		return nil, err
	}
	if configModule != nil {
		modules = append(modules, configModule)
		configModule.BindAuthorization(authFacade)
		configModule.BindRoleSecurity(roleFacade)
		if authorizationModule != nil {
			authorizationModule.BindRoleGrantConfigScopes(configFacade)
		}
	}

	adminModule, adminFacades, err := systemadmin.Install(deps, systemadmin.Dependencies{
		Subjects: userFacades.Subjects,
		Accounts: userFacades.Accounts,
		Auth:     authFacade,
		Sessions: ssoFacades.Sessions,
	})
	if err != nil {
		return nil, err
	}
	if adminModule != nil {
		modules = append(modules, adminModule)
		if cacheGovernanceModule != nil {
			cacheGovernanceModule.BindOperationLogger(adminFacades.Operation)
		}
		if dictModule != nil {
			dictModule.BindOperationLogger(adminFacades.Operation)
		}
		if configModule != nil {
			configModule.BindOperationLogger(adminFacades.Operation)
		}
		if userModule != nil {
			userModule.BindOperationLogger(adminFacades.Operation)
		}
		if authorizationModule != nil {
			authorizationModule.BindOperationLogger(adminFacades.Operation)
		}
		if challengeModule != nil {
			challengeModule.BindOperationLogger(adminFacades.Operation)
		}
		if ssoModule != nil {
			ssoModule.BindOperationLogger(adminFacades.Operation)
		}
		if notificationModule != nil {
			notificationModule.BindOperationLogger(adminFacades.Operation)
		}
	}

	platformPolicyCore, platformFacade, err := platform.InstallPolicyCore(deps, platform.Dependencies{
		AuthorizationSessions: ssoFacades.AuthorizationSessions,
		Sessions:              ssoFacades.Sessions,
	})
	if err != nil {
		return nil, err
	}
	platformModules := platformModules{
		PolicyCore:   platformPolicyCore,
		PublicFacade: platformFacade,
	}
	if platformPolicyCore != nil {
		platformPolicyCore.BindAuthorization(authFacade)
	}
	if featureSet.Enabled(features.PlatformControl) {
		controlPlane := platform.MountControlPlane(platformPolicyCore)
		platformModules.ControlPlane = controlPlane
		if controlPlane != nil && adminFacades.Operation != nil {
			controlPlane.BindOperationLogger(adminFacades.Operation)
		}
	}
	modules = appendPlatformModules(modules, featureSet, platformModules)

	if featureSet.Enabled(features.FederationHub) {
		hubModule, _, installErr := hubcontrol.Install(deps, hubcontrol.Dependencies{ManagedSSO: ssoFacades.ManagedClients})
		if installErr != nil {
			return nil, installErr
		}
		if hubModule != nil && adminFacades.Operation != nil {
			hubModule.BindOperationLogger(adminFacades.Operation)
		}
		modules = appendHubControlModule(modules, featureSet, hubModule)
	}

	var nodeManagementModule *nodemanagement.Module
	if featureSet.Enabled(features.FederationNode) {
		managedUsers, ok := userFacades.AdminUsers.(userfacade.ManagedUserStatusFacade)
		if !ok {
			return nil, fmt.Errorf("system user facade does not support managed status")
		}
		managedPolicies, ok := platformFacade.(platformfacade.ManagedLoginPolicyFacade)
		if !ok {
			return nil, fmt.Errorf("platform facade does not support managed login policy")
		}
		managedSessions, ok := ssoFacades.Sessions.(ssofacade.ManagedSessionFacade)
		if !ok {
			return nil, fmt.Errorf("sso facade does not support managed sessions")
		}
		userModule.BindManagedSessions(managedSessions)
		nodeModule, err := nodemanagement.Install(deps, nodemanagement.Dependencies{
			Users: userFacades.AdminUsers, ManagedUsers: managedUsers, Sessions: managedSessions,
			Policies: managedPolicies, Audit: adminFacades.OperationLogs,
		})
		if err != nil {
			return nil, err
		}
		nodeManagementModule = nodeModule
		modules = append(modules, nodeModule)
	}

	externalLoginModule, externalLoginFacades, err := externallogin.Install(deps, externallogin.Dependencies{
		Subjects:                 userFacades.Subjects,
		Profiles:                 userFacades.Profiles,
		AuthorizationSessions:    ssoFacades.AuthorizationSessions,
		AuthenticationCompletion: ssoFacades.AuthenticationCompletion,
		BootstrapSession:         ssoFacades.Bootstrap,
		Sessions:                 ssoFacades.Sessions,
		Platform:                 platformModules.PublicFacade,
		Relations:                userFacades.Relations,
		Organizations:            userFacades.Orgs,
	})
	if err != nil {
		return nil, err
	}
	if featureSet.Enabled(features.FederationNode) && (externalLoginModule == nil || externalLoginFacades.ManagedOIDC == nil) {
		return nil, fmt.Errorf("node federation requires the managed external login facade")
	}
	if externalLoginModule != nil {
		externalLoginModule.BindAuthorization(authFacade)
		externalLoginModule.BindOperationLogger(adminFacades.Operation)
		modules = append(modules, externalLoginModule)
		bindManagedOIDCProvider(featureSet, nodeManagementModule, externalLoginFacades.ManagedOIDC)
		if notificationModule != nil && externalLoginFacades.Enterprise != nil {
			notificationModule.BindEnterpriseIdentityResolver(enterpriseReadinessAdapter{facade: externalLoginFacades.Enterprise})
			externalLoginModule.BindEnterpriseNotificationRoutes(notificationModule)
		}
	}

	fileModule, fileFacades, err := filemodule.Install(deps)
	if err != nil {
		return nil, err
	}
	if fileModule != nil {
		fileModule.BindOperationLogger(adminFacades.Operation)
		userModule.BindFileAssets(fileFacades.Assets)
		if configModule != nil {
			configModule.BindConfigAssets(fileFacades.ConfigAssets)
		}
		modules = append(modules, fileModule)
	}

	workforceModule, err := workforce.Install(deps, workforce.Dependencies{Notifications: notificationFacade, Authorization: authFacade})
	if err != nil {
		return nil, err
	}
	if workforceModule != nil {
		workforceModule.BindFileFacade(fileFacades.WorkforceInputs)
		if adminFacades.Operation != nil {
			workforceModule.BindOperationLogger(adminFacades.Operation)
		}
		if configFacade != nil {
			if err := workforceModule.BindConfigFacade(configFacade); err != nil {
				return nil, err
			}
		}
		modules = append(modules, workforceModule)
	}

	observabilityModule, err := observability.Install(deps, observability.Dependencies{
		AuditEvents: ssoFacades.AuditEvents,
		Clients:     ssoFacades.Clients,
		Sessions:    ssoFacades.Sessions,
		RuntimeLogs: adminFacades.RuntimeLogs,
	})
	if err != nil {
		return nil, err
	}
	if observabilityModule != nil {
		modules = append(modules, observabilityModule)
	}

	setupModule, err := setup.Install(deps, setup.Dependencies{
		Users:        userFacades.Provisioning,
		Relations:    userFacades.Relations,
		Roles:        roleFacade,
		Permissions:  permissionFacade,
		SsoBootstrap: ssoFacades.Bootstrap,
	})
	if err != nil {
		return nil, err
	}
	if setupModule != nil {
		modules = append(modules, setupModule)
	}

	loginModule, _, err := login.Install(deps, login.Dependencies{
		UserCredentials:          credentialFacade,
		Subjects:                 userFacades.Subjects,
		ChallengeInternal:        challengeModule.InternalFacade(),
		ChallengeClient:          challengeModule.ClientFacade(),
		ProofVerifier:            challengeModule.ProofTokenVerifier(),
		LoginFailures:            adminFacades.LoginFailures,
		AuthorizationSessions:    ssoFacades.AuthorizationSessions,
		AuthenticationCompletion: ssoFacades.AuthenticationCompletion,
		BootstrapSession:         ssoFacades.Bootstrap,
		Platform:                 platformModules.PublicFacade,
	})
	if err != nil {
		return nil, err
	}
	if loginModule != nil {
		loginModule.BindOperationLogger(adminFacades.Operation)
		modules = append(modules, loginModule)
	}

	kernelModule, err := kernel.Install(deps)
	if err != nil {
		return nil, err
	}
	modules = append(modules, kernelModule)

	return modules, nil
}

type enterpriseReadinessAdapter struct {
	facade externalfacade.EnterpriseIdentityFacade
}

func (a enterpriseReadinessAdapter) ResolveEnterpriseReadiness(ctx context.Context, organizationID, userID int64, provider string) (bool, bool, bool, string, error) {
	result, err := a.facade.ResolveEnterpriseReadiness(ctx, organizationID, userID, provider)
	if err != nil || result == nil {
		return false, false, false, "", err
	}
	return result.Available, result.DirectReady, result.MentionReady, result.ReasonCode, nil
}

func (a enterpriseReadinessAdapter) ResolveEnterpriseNotificationAuthority(ctx context.Context, organizationID, userID int64, provider, grantType string) (string, string, string, string, int64, int64, int64, int64, error) {
	result, err := a.facade.ResolveNotificationAuthority(ctx, externalfacade.NotificationAuthorityRequest{OrganizationID: organizationID, UserID: userID, Provider: provider, GrantType: grantType})
	if err != nil || result == nil {
		return "", "", "", "", 0, 0, 0, 0, err
	}
	return result.IdentityKind, result.Subject, result.AuthorityDigest, result.ConnectionConfigDigest, result.ExternalIdentityID, result.IdentityVersion, result.EnterpriseConnectionID, result.ConnectionVersion, nil
}

func (a enterpriseReadinessAdapter) ValidateEnterpriseNotificationAuthority(ctx context.Context, organizationID, userID, identityID, identityVersion, connectionID, connectionVersion int64, authorityDigest, grantType string) error {
	return a.facade.ValidateNotificationAuthority(ctx, externalfacade.NotificationAuthoritySnapshot{OrganizationID: organizationID, UserID: userID, ExternalIdentityID: identityID, IdentityVersion: identityVersion, EnterpriseConnectionID: connectionID, ConnectionVersion: connectionVersion, AuthorityDigest: authorityDigest, GrantType: grantType})
}

func (a enterpriseReadinessAdapter) AcquireEnterpriseApplicationCredential(ctx context.Context, organizationID, connectionID, connectionVersion int64, connectionConfigDigest string) (string, string, string, string, string, int64, time.Time, error) {
	lease, err := a.facade.AcquireApplicationCredentialLease(ctx, organizationID, connectionID, connectionVersion, connectionConfigDigest)
	if err != nil || lease == nil {
		return "", "", "", "", "", 0, time.Time{}, err
	}
	return lease.Provider, lease.AppID, lease.CorpID, lease.AgentID, lease.ClientSecret, lease.CredentialVersion, lease.ExpiresAt, nil
}

func (a enterpriseReadinessAdapter) DiscoverEnterpriseGroups(ctx context.Context, organizationID, connectionID int64) ([]notificationapp.EnterpriseGroupTarget, error) {
	items, err := a.facade.DiscoverEnterpriseGroups(ctx, organizationID, connectionID)
	if err != nil {
		return nil, err
	}
	result := make([]notificationapp.EnterpriseGroupTarget, 0, len(items))
	for _, item := range items {
		result = append(result, notificationapp.EnterpriseGroupTarget{DisplayName: item.DisplayName, Description: item.Description, Target: item.Target, ConnectionID: item.ConnectionID, ConnectionVersion: item.ConnectionVersion, ConnectionConfigDigest: item.ConnectionConfigDigest})
	}
	return result, nil
}

type managedOIDCProviderBinder interface {
	BindManagedOIDCProvider(externalfacade.ManagedOIDCProviderFacade)
}

func bindManagedOIDCProvider(featureSet features.Set, binder managedOIDCProviderBinder, provider externalfacade.ManagedOIDCProviderFacade) bool {
	if !featureSet.Enabled(features.FederationNode) || binder == nil || provider == nil {
		return false
	}
	binder.BindManagedOIDCProvider(provider)
	return true
}

type platformModules struct {
	PolicyCore   core.Module
	ControlPlane core.Module
	PublicFacade platformfacade.PublicFacade
}

func appendPlatformModules(modules []core.Module, featureSet features.Set, platformModules platformModules) []core.Module {
	if platformModules.PolicyCore != nil {
		modules = append(modules, platformModules.PolicyCore)
	}
	if featureSet.Enabled(features.PlatformControl) && platformModules.ControlPlane != nil {
		modules = append(modules, platformModules.ControlPlane)
	}
	return modules
}

func appendHubControlModule(modules []core.Module, featureSet features.Set, module core.Module) []core.Module {
	if featureSet.Enabled(features.FederationHub) && module != nil {
		modules = append(modules, module)
	}
	return modules
}
