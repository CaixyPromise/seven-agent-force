package application

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/CaixyPromise/seven-agent/force/internal/app/external_login/domain"
	"github.com/CaixyPromise/seven-agent/force/internal/app/external_login/facade"
	apperrors "github.com/CaixyPromise/seven-agent/force/internal/common/errors"
)

const managedProviderOwner = "hub_control"

type managedProviderMetadata struct {
	ManagedBy               string `json:"managedBy"`
	OwnerNodeCode           string `json:"ownerNodeCode"`
	ConnectionVersion       string `json:"connectionVersion"`
	ConnectionHash          string `json:"connectionHash"`
	TargetRevision          int64  `json:"targetRevision,omitempty"`
	TokenEndpointAuthMethod string `json:"tokenEndpointAuthMethod"`
	PersistLoginTokens      bool   `json:"persistLoginTokens"`
}

func (s *Service) ApplyManagedOIDCProvider(ctx context.Context, command facade.ManagedOIDCProviderCommand) error {
	if s == nil || s.repo == nil || s.discovery == nil || s.secrets == nil {
		return apperrors.ServiceUnavailable("系统托管OIDC Provider能力不可用")
	}
	providerCode, err := domain.ManagedProviderCode(command.OwnerNodeCode)
	if err != nil {
		return apperrors.Params(err.Error())
	}
	version := strings.TrimSpace(command.ConnectionVersion)
	if version == "" || len(version) > 128 {
		return apperrors.Params("connectionVersion格式无效")
	}
	if command.TargetRevision < 0 {
		return apperrors.Params("targetRevision格式无效")
	}
	issuer, err := domain.CanonicalIssuer(command.Issuer)
	if err != nil {
		return apperrors.Params(err.Error())
	}
	hash, err := managedProviderCommandHash(command, issuer)
	if err != nil {
		return apperrors.System("系统托管OIDC Provider请求摘要生成失败")
	}
	replayed := false
	if err := s.withTransaction(ctx, func(txCtx context.Context) error {
		existing, findErr := s.repo.FindProviderForUpdate(txCtx, providerCode)
		if findErr != nil {
			return findErr
		}
		var replayErr error
		replayed, replayErr = s.managedProviderReplay(txCtx, existing, strings.TrimSpace(command.OwnerNodeCode), version, command.TargetRevision, hash)
		return replayErr
	}); err != nil || replayed {
		return err
	}
	discovered, err := s.discovery.DiscoverOIDC(ctx, issuer)
	if err != nil {
		return err
	}
	if discovered.Issuer != issuer {
		return apperrors.ObjectState("OIDC discovery issuer不匹配")
	}
	if strings.TrimSpace(command.ClientID) == "" || strings.TrimSpace(command.ClientSecret) == "" || strings.TrimSpace(command.RedirectURI) == "" {
		return apperrors.Params("系统托管OIDC Provider参数不完整")
	}
	encrypted, err := s.secrets.EncryptString(ctx, command.ClientSecret)
	if err != nil {
		return err
	}
	now := s.now()
	return s.withTransaction(ctx, func(txCtx context.Context) error {
		existing, findErr := s.repo.FindProviderForUpdate(txCtx, providerCode)
		if findErr != nil {
			return findErr
		}
		if replayed, replayErr := s.managedProviderReplay(txCtx, existing, strings.TrimSpace(command.OwnerNodeCode), version, command.TargetRevision, hash); replayErr != nil || replayed {
			return replayErr
		}
		if existing != nil {
			if existing.Issuer != issuer {
				count, countErr := s.repo.CountIdentitiesByProvider(txCtx, providerCode)
				if countErr != nil {
					return countErr
				}
				if count > 0 {
					return apperrors.ObjectState("OIDC issuer已有身份绑定，禁止变更")
				}
			}
		}
		metadata, marshalErr := json.Marshal(managedProviderMetadata{
			ManagedBy: managedProviderOwner, OwnerNodeCode: strings.TrimSpace(command.OwnerNodeCode),
			ConnectionVersion: version, ConnectionHash: hash, TargetRevision: command.TargetRevision,
			TokenEndpointAuthMethod: domain.TokenEndpointAuthMethodClientSecretBasic, PersistLoginTokens: false,
		})
		if marshalErr != nil {
			return marshalErr
		}
		provider := domain.Provider{
			ProviderCode: providerCode, ProviderName: "Hub OIDC " + strings.TrimSpace(command.OwnerNodeCode), ProtocolType: domain.ProtocolTypeOIDC,
			Issuer: issuer, AuthorizationEndpoint: discovered.AuthorizationEndpoint, TokenEndpoint: discovered.TokenEndpoint,
			TokenEndpointAuthMethod: domain.TokenEndpointAuthMethodClientSecretBasic,
			UserinfoEndpoint:        discovered.UserinfoEndpoint, JWKSURI: discovered.JWKSURI, ClientID: strings.TrimSpace(command.ClientID),
			ClientSecretCiphertext: encrypted.CiphertextB64, ClientSecretEDEK: encrypted.EDEKB64, ClientSecretWrapKeyRef: encrypted.WrapKeyRef,
			Scopes: []string{"openid", "profile", "email"}, RedirectURI: strings.TrimSpace(command.RedirectURI),
			DisplayName: firstNonBlankString(strings.TrimSpace(command.DisplayName), "Hub"), DisplayEnabled: command.Enabled,
			LoginEnabled: command.Enabled, BindEnabled: true, EmailAutoBindEnabled: false, AccountAutoCreateEnabled: true,
			Status: domain.ProviderStatusActive, MetadataJSON: string(metadata),
		}
		if !command.Enabled {
			provider.Status = domain.ProviderStatusDisabled
		}
		if err := domain.ValidateProvider(provider); err != nil {
			return err
		}
		if existing == nil {
			if err := s.repo.InsertProvider(txCtx, &provider, 0); err != nil {
				return err
			}
		} else {
			if err := s.repo.UpdateProvider(txCtx, &provider, 0); err != nil {
				return err
			}
			if existing.Status != provider.Status {
				changed, statusErr := s.repo.UpdateProviderStatus(txCtx, providerCode, provider.Status, 0, now)
				if statusErr != nil {
					return statusErr
				}
				if !changed {
					return apperrors.ObjectState("系统托管OIDC Provider状态未更新")
				}
			}
		}
		if err := s.repo.ReplaceProviderMethods(txCtx, providerCode, []domain.ProviderMethod{{
			ProviderCode: providerCode, MethodKey: "oidc-login", CapabilityCode: domain.CapabilityOIDCLogin,
			RequiredScopes: []string{"openid"}, Status: domain.ProviderMethodStatusActive,
		}}); err != nil {
			return err
		}
		return s.repo.InsertManagedProviderCommand(txCtx, &domain.ManagedProviderCommand{ProviderCode: providerCode, ConnectionVersion: version, RequestHash: hash, CreateTime: now})
	})
}

func (s *Service) managedProviderReplay(ctx context.Context, existing *domain.Provider, ownerNodeCode, version string, targetRevision int64, requestHash string) (bool, error) {
	if existing == nil {
		return false, nil
	}
	ownership, err := parseManagedProviderMetadata(existing.MetadataJSON)
	if err != nil || ownership.ManagedBy != managedProviderOwner || ownership.OwnerNodeCode != ownerNodeCode {
		return false, apperrors.ObjectState("外部登录Provider已存在且不属于当前Node")
	}
	if targetRevision > 0 {
		if ownership.TargetRevision > targetRevision {
			return false, apperrors.ObjectState("targetRevision已被后续配置取代")
		}
		if ownership.TargetRevision == targetRevision && ownership.ConnectionVersion != version {
			return false, apperrors.ObjectState("相同targetRevision的OIDC命令不一致")
		}
		if ownership.TargetRevision < targetRevision {
			return false, nil
		}
	} else if ownership.TargetRevision > 0 {
		return false, apperrors.ObjectState("缺少targetRevision的旧命令已被后续配置取代")
	}
	previous, err := s.repo.FindManagedProviderCommand(ctx, existing.ProviderCode, version)
	if err != nil || previous == nil {
		return false, err
	}
	if previous.RequestHash != requestHash {
		return false, apperrors.ObjectState("相同connectionVersion的OIDC配置不一致")
	}
	if ownership.ConnectionVersion != version {
		return false, apperrors.ObjectState("connectionVersion已被后续配置取代")
	}
	return true, nil
}

func (s *Service) DisableManagedOIDCProvider(ctx context.Context, ownerNodeCode, connectionVersion string, targetRevision int64) error {
	if s == nil || s.repo == nil {
		return apperrors.ServiceUnavailable("系统托管OIDC Provider能力不可用")
	}
	providerCode, err := domain.ManagedProviderCode(ownerNodeCode)
	if err != nil {
		return apperrors.Params(err.Error())
	}
	version := strings.TrimSpace(connectionVersion)
	if version == "" || len(version) > 128 {
		return apperrors.Params("connectionVersion格式无效")
	}
	if targetRevision < 0 {
		return apperrors.Params("targetRevision格式无效")
	}
	disableIdentity := providerCode + "\x00" + version + "\x00disabled"
	if targetRevision > 0 {
		disableIdentity = providerCode + "\x00" + version + "\x00" + fmt.Sprintf("%d", targetRevision) + "\x00disabled"
	}
	disableSum := sha256.Sum256([]byte(disableIdentity))
	disableHash := hex.EncodeToString(disableSum[:])
	return s.withTransaction(ctx, func(txCtx context.Context) error {
		provider, findErr := s.repo.FindProviderForUpdate(txCtx, providerCode)
		if findErr != nil || provider == nil {
			return findErr
		}
		metadata, parseErr := parseManagedProviderMetadata(provider.MetadataJSON)
		if parseErr != nil || metadata.ManagedBy != managedProviderOwner || metadata.OwnerNodeCode != strings.TrimSpace(ownerNodeCode) {
			return apperrors.ObjectState("外部登录Provider不属于当前Node")
		}
		if targetRevision > 0 {
			if metadata.TargetRevision > targetRevision {
				return apperrors.ObjectState("targetRevision已被后续配置取代")
			}
			if metadata.TargetRevision == targetRevision && metadata.ConnectionVersion != version {
				return apperrors.ObjectState("相同targetRevision的OIDC命令不一致")
			}
		} else if metadata.TargetRevision > 0 {
			return apperrors.ObjectState("缺少targetRevision的旧命令已被后续配置取代")
		}
		previous, commandErr := s.repo.FindManagedProviderCommand(txCtx, providerCode, version)
		if commandErr != nil {
			return commandErr
		}
		if previous != nil {
			if previous.RequestHash != disableHash {
				return apperrors.ObjectState("相同connectionVersion的OIDC配置不一致")
			}
			if metadata.ConnectionVersion != version {
				return apperrors.ObjectState("connectionVersion已被后续配置取代")
			}
			return nil
		}
		metadata.ConnectionVersion = version
		metadata.ConnectionHash = disableHash
		metadata.TargetRevision = targetRevision
		encoded, marshalErr := json.Marshal(metadata)
		if marshalErr != nil {
			return marshalErr
		}
		provider.MetadataJSON = string(encoded)
		statusChanged := provider.Status != domain.ProviderStatusDisabled
		provider.Status = domain.ProviderStatusDisabled
		provider.DisplayEnabled = false
		provider.LoginEnabled = false
		if updateErr := s.repo.UpdateProvider(txCtx, provider, 0); updateErr != nil {
			return updateErr
		}
		if statusChanged {
			changed, statusErr := s.repo.UpdateProviderStatus(txCtx, providerCode, domain.ProviderStatusDisabled, 0, s.now())
			if statusErr != nil {
				return statusErr
			}
			if !changed {
				return apperrors.ObjectState("系统托管OIDC Provider状态未更新")
			}
		}
		if methodErr := s.repo.ReplaceProviderMethods(txCtx, providerCode, []domain.ProviderMethod{{
			ProviderCode: providerCode, MethodKey: "oidc-login", CapabilityCode: domain.CapabilityOIDCLogin,
			RequiredScopes: []string{"openid"}, Status: domain.ProviderMethodStatusDisabled,
		}}); methodErr != nil {
			return methodErr
		}
		return s.repo.InsertManagedProviderCommand(txCtx, &domain.ManagedProviderCommand{ProviderCode: providerCode, ConnectionVersion: version, RequestHash: disableHash, CreateTime: s.now()})
	})
}

func parseManagedProviderMetadata(raw string) (managedProviderMetadata, error) {
	var metadata managedProviderMetadata
	err := json.Unmarshal([]byte(strings.TrimSpace(raw)), &metadata)
	return metadata, err
}

func isManagedProvider(provider *domain.Provider) bool {
	if provider == nil {
		return false
	}
	metadata, err := parseManagedProviderMetadata(provider.MetadataJSON)
	return domain.IsManagedProviderCode(provider.ProviderCode) || (err == nil && metadata.ManagedBy != "")
}

func persistLoginTokens(provider domain.Provider) bool {
	if domain.IsManagedProviderCode(provider.ProviderCode) {
		return false
	}
	if !isManagedProvider(&provider) {
		return true
	}
	metadata, err := parseManagedProviderMetadata(provider.MetadataJSON)
	return err == nil && metadata.PersistLoginTokens
}

func managedProviderConfigDigest(provider *domain.Provider) string {
	if !isManagedProvider(provider) {
		return ""
	}
	metadata, err := parseManagedProviderMetadata(provider.MetadataJSON)
	if err != nil {
		return ""
	}
	payload := strings.Join([]string{metadata.ConnectionVersion, metadata.ConnectionHash, fmt.Sprintf("%d", metadata.TargetRevision)}, "\x00")
	sum := sha256.Sum256([]byte(payload))
	return hex.EncodeToString(sum[:])
}

func managedProviderCommandHash(command facade.ManagedOIDCProviderCommand, issuer string) (string, error) {
	if command.TargetRevision == 0 {
		payload := struct {
			OwnerNodeCode, ConnectionVersion, DisplayName, Issuer, ClientID, ClientSecret, RedirectURI string
			Enabled                                                                                    bool
		}{strings.TrimSpace(command.OwnerNodeCode), strings.TrimSpace(command.ConnectionVersion), strings.TrimSpace(command.DisplayName), issuer,
			strings.TrimSpace(command.ClientID), command.ClientSecret, strings.TrimSpace(command.RedirectURI), command.Enabled}
		encoded, err := json.Marshal(payload)
		if err != nil {
			return "", err
		}
		sum := sha256.Sum256(encoded)
		return hex.EncodeToString(sum[:]), nil
	}
	payload := struct {
		OwnerNodeCode, ConnectionVersion, DisplayName, Issuer, ClientID, ClientSecret, RedirectURI string
		TargetRevision                                                                             int64
		Enabled                                                                                    bool
	}{strings.TrimSpace(command.OwnerNodeCode), strings.TrimSpace(command.ConnectionVersion), strings.TrimSpace(command.DisplayName), issuer,
		strings.TrimSpace(command.ClientID), command.ClientSecret, strings.TrimSpace(command.RedirectURI), command.TargetRevision, command.Enabled}
	encoded, err := json.Marshal(payload)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(encoded)
	return hex.EncodeToString(sum[:]), nil
}

var _ facade.ManagedOIDCProviderFacade = (*Service)(nil)
