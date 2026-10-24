package docker

import (
	"context"
	"database/sql"
	"fmt"
	"strings"

	apperrors "github.com/CaixyPromise/seven-agent/force/internal/common/errors"
)

func (s *service) ListRegistries(ctx context.Context) ([]RemoteRegistryView, error) {
	repo, err := s.requireRegistryRepository()
	if err != nil {
		return nil, err
	}
	rows, err := repo.List(ctx)
	if err != nil {
		return nil, err
	}
	result := make([]RemoteRegistryView, 0, len(rows))
	for _, row := range rows {
		result = append(result, s.toRegistryView(row))
	}
	return result, nil
}

func (s *service) GetRegistry(ctx context.Context, id int64) (*RemoteRegistryView, error) {
	row, err := s.requireRegistry(ctx, id)
	if err != nil {
		return nil, err
	}
	view := s.toRegistryView(*row)
	return &view, nil
}

func (s *service) CreateRegistry(ctx context.Context, command RemoteRegistryCommand) (int64, error) {
	repo, err := s.requireRegistryRepository()
	if err != nil {
		return 0, err
	}
	if err := s.validateRegistryCommand(ctx, command, 0); err != nil {
		return 0, err
	}
	id := int64(0)
	if s.idGen != nil {
		id = s.idGen.NextID()
	}
	if id == 0 {
		return 0, apperrors.System("docker registry id generator is not configured")
	}
	row, err := s.commandToRecord(ctx, id, command, nil)
	if err != nil {
		return 0, err
	}
	if err := repo.Insert(ctx, row); err != nil {
		return 0, err
	}
	if row.DefaultRegistry {
		_ = repo.ClearOtherDefaults(ctx, row.ID)
	}
	return row.ID, nil
}

func (s *service) UpdateRegistry(ctx context.Context, id int64, command RemoteRegistryCommand) (bool, error) {
	repo, err := s.requireRegistryRepository()
	if err != nil {
		return false, err
	}
	existing, err := s.requireRegistry(ctx, id)
	if err != nil {
		return false, err
	}
	if err := s.validateRegistryCommand(ctx, command, id); err != nil {
		return false, err
	}
	row, err := s.commandToRecord(ctx, id, command, existing)
	if err != nil {
		return false, err
	}
	if err := repo.Update(ctx, row); err != nil {
		return false, err
	}
	if row.DefaultRegistry {
		_ = repo.ClearOtherDefaults(ctx, row.ID)
	}
	return true, nil
}

func (s *service) DeleteRegistry(ctx context.Context, id int64) (bool, error) {
	repo, err := s.requireRegistryRepository()
	if err != nil {
		return false, err
	}
	if _, err := s.requireRegistry(ctx, id); err != nil {
		return false, err
	}
	if err := repo.Delete(ctx, id); err != nil {
		return false, err
	}
	return true, nil
}

func (s *service) TestRegistry(ctx context.Context, id int64) (*RegistryConnectionTestView, error) {
	rt, err := s.registryRuntime(ctx, id)
	if err != nil {
		return nil, err
	}
	view := s.registry.Test(ctx, rt)
	if view.Success && (view.TokenRealm != "" || view.TokenService != "") {
		if repo, repoErr := s.requireRegistryRepository(); repoErr == nil {
			if existing, existingErr := repo.Get(ctx, id); existingErr == nil && existing != nil {
				updated := *existing
				if view.TokenRealm != "" {
					updated.TokenRealm = sql.NullString{String: view.TokenRealm, Valid: true}
				}
				if view.TokenService != "" {
					updated.TokenService = sql.NullString{String: view.TokenService, Valid: true}
				}
				_ = repo.Update(ctx, updated)
			}
		}
	}
	return &view, nil
}

func (s *service) ListRepositories(ctx context.Context, id, current, size int64, keyword string) (*PageResult[RemoteRepositoryView], error) {
	rt, err := s.registryRuntime(ctx, id)
	if err != nil {
		return nil, err
	}
	if size <= 0 {
		size = s.cfg.Registry.DefaultPageSize
	}
	if s.cfg.Registry.MaxPageSize > 0 && size > s.cfg.Registry.MaxPageSize {
		size = s.cfg.Registry.MaxPageSize
	}
	return s.registry.ListRepositories(ctx, rt, current, size, keyword, s.cfg.Registry.MaxPages)
}

func (s *service) ListTags(ctx context.Context, id int64, repository string) (*RemoteTagsView, error) {
	rt, err := s.registryRuntime(ctx, id)
	if err != nil {
		return nil, err
	}
	return s.registry.ListTags(ctx, rt, repository)
}

func (s *service) GetManifest(ctx context.Context, id int64, repository, reference string) (*RemoteManifestView, error) {
	rt, err := s.registryRuntime(ctx, id)
	if err != nil {
		return nil, err
	}
	return s.registry.GetManifest(ctx, rt, repository, reference)
}

func (s *service) validateRegistryCommand(ctx context.Context, command RemoteRegistryCommand, excludeID int64) error {
	if strings.TrimSpace(command.RegistryType) == "" {
		command.RegistryType = "REGISTRY"
	}
	if !strings.EqualFold(strings.TrimSpace(command.RegistryType), "REGISTRY") {
		return apperrors.Params("当前仅支持 registry v2")
	}
	if strings.TrimSpace(command.Name) == "" || strings.TrimSpace(command.Code) == "" {
		return apperrors.Params("名称和编码不能为空")
	}
	if strings.TrimSpace(command.Endpoint) == "" {
		return apperrors.Params("endpoint 不能为空")
	}
	authType := strings.ToUpper(firstNonBlank(command.AuthType, "ANONYMOUS"))
	if authType != "ANONYMOUS" && authType != "BASIC" && authType != "TOKEN_CHALLENGE" {
		return apperrors.Params("当前仅支持匿名、Basic 和 Bearer challenge 认证")
	}
	if authType == "BASIC" || authType == "TOKEN_CHALLENGE" {
		if strings.TrimSpace(command.Username) == "" {
			return apperrors.Params("带认证的 registry 必须配置用户名")
		}
		if strings.TrimSpace(command.Password) == "" {
			if excludeID == 0 {
				return apperrors.Params("带认证的 registry 必须配置密码")
			}
			repo, err := s.requireRegistryRepository()
			if err != nil {
				return err
			}
			existing, err := repo.Get(ctx, excludeID)
			if err != nil {
				return err
			}
			if existing == nil || !existing.SecretCiphertext.Valid || !existing.SecretEDEK.Valid || !existing.WrapKeyRef.Valid {
				return apperrors.Params("切换为认证 registry 时必须配置密码")
			}
		}
	}
	if authType != "TOKEN_CHALLENGE" && (strings.TrimSpace(command.TokenRealm) != "" || strings.TrimSpace(command.TokenService) != "") {
		return apperrors.Params("仅 TOKEN_CHALLENGE 支持 tokenRealm/tokenService")
	}
	repo, err := s.requireRegistryRepository()
	if err != nil {
		return err
	}
	exists, err := repo.CodeExists(ctx, command.Code, excludeID)
	if err != nil {
		return err
	}
	if exists {
		return apperrors.Params("registry 编码已存在")
	}
	return nil
}

func (s *service) commandToRecord(ctx context.Context, id int64, command RemoteRegistryCommand, existing *RegistryRecord) (RegistryRecord, error) {
	authType := strings.ToUpper(firstNonBlank(command.AuthType, "ANONYMOUS"))
	row := RegistryRecord{
		ID:                     id,
		Name:                   strings.TrimSpace(command.Name),
		Code:                   strings.TrimSpace(command.Code),
		RegistryType:           strings.ToUpper(firstNonBlank(command.RegistryType, "REGISTRY")),
		Endpoint:               trimTrailingSlash(command.Endpoint),
		APIBaseURL:             sql.NullString{String: trimTrailingSlash(command.APIBaseURL), Valid: strings.TrimSpace(command.APIBaseURL) != ""},
		AuthType:               authType,
		Username:               sql.NullString{String: strings.TrimSpace(command.Username), Valid: strings.TrimSpace(command.Username) != ""},
		TokenRealm:             sql.NullString{String: strings.TrimSpace(command.TokenRealm), Valid: authType == "TOKEN_CHALLENGE" && strings.TrimSpace(command.TokenRealm) != ""},
		TokenService:           sql.NullString{String: strings.TrimSpace(command.TokenService), Valid: authType == "TOKEN_CHALLENGE" && strings.TrimSpace(command.TokenService) != ""},
		CredentialID:           sql.NullInt64{Int64: command.CredentialID, Valid: command.CredentialID > 0},
		NamespaceWhitelistJSON: sql.NullString{String: strings.TrimSpace(command.NamespaceWhitelistJSON), Valid: strings.TrimSpace(command.NamespaceWhitelistJSON) != ""},
		TLSEnabled:             boolValue(command.TLSEnabled),
		InsecureSkipVerify:     boolValue(command.InsecureSkipVerify),
		DefaultRegistry:        boolValue(command.DefaultRegistry),
		Status:                 intValueOr(command.Status, 0),
		Description:            sql.NullString{String: strings.TrimSpace(command.Description), Valid: strings.TrimSpace(command.Description) != ""},
		Sort:                   intValueOr(command.Sort, 0),
	}
	if authType == "BASIC" || authType == "TOKEN_CHALLENGE" {
		if password := strings.TrimSpace(command.Password); password != "" {
			if s.secret == nil {
				return row, apperrors.System("registry secret service is not configured")
			}
			secret, err := s.secret.EncryptString(ctx, password)
			if err != nil {
				return row, fmt.Errorf("encrypt docker registry secret: %w", err)
			}
			row.SecretCiphertext = sql.NullString{String: secret.CiphertextB64, Valid: true}
			row.SecretEDEK = sql.NullString{String: secret.EDEKB64, Valid: true}
			row.WrapKeyRef = sql.NullString{String: secret.WrapKeyRef, Valid: true}
		} else if existing != nil {
			row.SecretCiphertext = existing.SecretCiphertext
			row.SecretEDEK = existing.SecretEDEK
			row.WrapKeyRef = existing.WrapKeyRef
		}
	}
	return row, nil
}

func (s *service) registryRuntime(ctx context.Context, id int64) (registryRuntime, error) {
	row, err := s.requireRegistry(ctx, id)
	if err != nil {
		return registryRuntime{}, err
	}
	rt := registryRuntime{
		ID:                     row.ID,
		Code:                   row.Code,
		Name:                   row.Name,
		Endpoint:               row.Endpoint,
		APIBaseURL:             normalizeAPIBaseURL(row.Endpoint, row.APIBaseURL.String),
		AuthType:               row.AuthType,
		Username:               row.Username.String,
		TokenRealm:             row.TokenRealm.String,
		TokenService:           row.TokenService.String,
		TLSEnabled:             row.TLSEnabled,
		InsecureSkipVerify:     row.InsecureSkipVerify,
		NamespaceWhitelistJSON: row.NamespaceWhitelistJSON.String,
	}
	if (rt.AuthType == "BASIC" || rt.AuthType == "TOKEN_CHALLENGE") && row.SecretCiphertext.Valid {
		if s.secret == nil {
			return rt, apperrors.System("registry secret service is not configured")
		}
		password, err := s.secret.DecryptString(ctx, row.SecretValue())
		if err != nil {
			return rt, fmt.Errorf("decrypt docker registry secret: %w", err)
		}
		rt.Password = password
	}
	return rt, nil
}

func (s *service) requireRegistry(ctx context.Context, id int64) (*RegistryRecord, error) {
	if id <= 0 {
		return nil, apperrors.Params("registry id 不能为空")
	}
	repo, err := s.requireRegistryRepository()
	if err != nil {
		return nil, err
	}
	row, err := repo.Get(ctx, id)
	if err != nil {
		return nil, err
	}
	if row == nil {
		return nil, apperrors.NotFound("registry 配置不存在")
	}
	return row, nil
}

func (s *service) requireRegistryRepository() (*RegistryRepository, error) {
	if s == nil || s.repo == nil {
		return nil, apperrors.Operation("Docker registry repository 未配置")
	}
	return s.repo, nil
}

func (s *service) toRegistryView(row RegistryRecord) RemoteRegistryView {
	configured := row.SecretCiphertext.Valid && row.SecretEDEK.Valid && row.WrapKeyRef.Valid
	return RemoteRegistryView{
		ID:                     row.ID,
		Name:                   row.Name,
		Code:                   row.Code,
		RegistryType:           row.RegistryType,
		Endpoint:               row.Endpoint,
		APIBaseURL:             row.APIBaseURL.String,
		AuthType:               row.AuthType,
		Username:               row.Username.String,
		TokenRealm:             row.TokenRealm.String,
		TokenService:           row.TokenService.String,
		CredentialID:           row.CredentialID.Int64,
		NamespaceWhitelistJSON: row.NamespaceWhitelistJSON.String,
		TLSEnabled:             row.TLSEnabled,
		InsecureSkipVerify:     row.InsecureSkipVerify,
		DefaultRegistry:        row.DefaultRegistry,
		Status:                 row.Status,
		Description:            row.Description.String,
		Sort:                   row.Sort,
		SecretConfigured:       configured,
		SecretHint:             map[bool]string{true: "******", false: ""}[configured],
		CreateTime:             formatTime(row.CreateTime.Time),
		UpdateTime:             formatTime(row.UpdateTime.Time),
	}
}

func boolValue(value *bool) bool {
	return value != nil && *value
}

func intValueOr(value *int, fallback int) int {
	if value == nil {
		return fallback
	}
	return *value
}

func trimTrailingSlash(value string) string {
	return strings.TrimRight(strings.TrimSpace(value), "/")
}

func normalizeAPIBaseURL(endpoint, apiBaseURL string) string {
	if strings.TrimSpace(apiBaseURL) != "" {
		return trimTrailingSlash(apiBaseURL)
	}
	return trimTrailingSlash(endpoint) + "/v2"
}
