package application

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/jackc/pgx/v5"

	"github.com/CaixyPromise/seven-agent/force/internal/app/workforce/provider/domain"
	providerfacade "github.com/CaixyPromise/seven-agent/force/internal/app/workforce/provider/facade"
	secretvalueinfra "github.com/CaixyPromise/seven-agent/force/internal/infrastructure/security/crypto/secretvalue"
)

var (
	ErrNotFound          = errors.New("MODEL_CREDENTIAL_NOT_FOUND")
	ErrNameConflict      = errors.New("MODEL_CREDENTIAL_NAME_CONFLICT")
	ErrVersionConflict   = errors.New("MODEL_CREDENTIAL_VERSION_CONFLICT")
	ErrProtocolInUse     = errors.New("MODEL_CREDENTIAL_PROTOCOL_IN_USE")
	ErrDisabled          = errors.New("MODEL_CREDENTIAL_DISABLED")
	ErrModelNotInCatalog = errors.New("TASK_MODEL_NOT_IN_CATALOG")
	ErrModelSelection    = errors.New("TASK_MODEL_SELECTION_REQUIRED")
)

type validationError struct{ cause error }

func (e *validationError) Error() string { return e.cause.Error() }
func (e *validationError) Unwrap() error { return e.cause }
func invalid(err error) error            { return &validationError{cause: err} }
func IsRequestValidationError(err error) bool {
	var target *validationError
	return errors.As(err, &target)
}

type Repository interface {
	Create(context.Context, domain.Credential) (domain.Credential, error)
	ListTenant(context.Context, int64) ([]domain.Credential, error)
	GetTenant(context.Context, int64, int64) (domain.Credential, error)
	GetTenantForShare(context.Context, pgx.Tx, int64, int64) (domain.Credential, error)
	WithLockedCredential(context.Context, int64, int64, func(domain.Credential, CredentialUpdateRepository) error) error
	Update(context.Context, domain.Credential, bool) (domain.Credential, error)
	UpdateDiscovery(context.Context, int64, int64, string, string) error
	Delete(context.Context, int64, int64, int64) (domain.DeleteResult, error)
}

// CredentialUpdateRepository operates inside the transaction holding the
// credential's exclusive row lock. Application owns the protocol policy.
type CredentialUpdateRepository interface {
	CountActiveProtocolUses(context.Context, int64, int64) (int, error)
	Update(context.Context, domain.Credential, bool) (domain.Credential, error)
}

type DiscoveryClient interface {
	Discover(context.Context, domain.RuntimeCredential) ([]string, error)
}

type InvocationCanceller interface {
	CancelModelInvocations(context.Context, int64, int64, string) error
}

type IDGenerator interface{ NextID() int64 }

type Service struct {
	repository Repository
	secrets    secretvalueinfra.Service
	discovery  DiscoveryClient
	canceller  InvocationCanceller
	idGen      IDGenerator
}

func NewService(repository Repository, secrets secretvalueinfra.Service, discovery DiscoveryClient, idGen ...IDGenerator) *Service {
	service := &Service{repository: repository, secrets: secrets, discovery: discovery}
	if len(idGen) > 0 {
		service.idGen = idGen[0]
	}
	return service
}

func (s *Service) SetInvocationCanceller(c InvocationCanceller) { s.canceller = c }

type CreateCommand struct {
	Name     string
	Protocol string
	BaseURL  string
	APIKey   string
	Models   []string
}

func (s *Service) Create(ctx context.Context, tenantID int64, command CreateCommand) (domain.View, error) {
	name, protocol, baseURL, models, err := normalize(command.Name, command.Protocol, command.BaseURL, command.Models)
	if err != nil {
		return domain.View{}, invalid(err)
	}
	apiKey := strings.TrimSpace(command.APIKey)
	if err = validateSecret(apiKey); err != nil {
		return domain.View{}, invalid(err)
	}
	encrypted, err := s.secrets.EncryptString(ctx, apiKey)
	if err != nil {
		return domain.View{}, fmt.Errorf("encrypt model credential: %w", err)
	}
	item, err := s.repository.Create(ctx, domain.Credential{
		ID: s.idGen.NextID(), TenantID: tenantID, Name: name, Protocol: protocol, BaseURL: baseURL, Models: models,
		SecretCiphertext: encrypted.CiphertextB64, SecretEDEK: encrypted.EDEKB64, SecretWrapKeyRef: encrypted.WrapKeyRef,
		SecretHint: secretHint(apiKey), SecretVersion: 1, Status: domain.StatusActive, Version: 1,
	})
	if err != nil {
		return domain.View{}, err
	}
	return item.View(), nil
}

func (s *Service) List(ctx context.Context, tenantID int64) ([]domain.View, error) {
	items, err := s.repository.ListTenant(ctx, tenantID)
	if err != nil {
		return nil, err
	}
	views := make([]domain.View, 0, len(items))
	for _, item := range items {
		views = append(views, item.View())
	}
	return views, nil
}

func (s *Service) Get(ctx context.Context, tenantID, id int64) (domain.View, error) {
	item, err := s.repository.GetTenant(ctx, tenantID, id)
	if err != nil {
		return domain.View{}, err
	}
	return item.View(), nil
}

type UpdateCommand struct {
	ExpectedVersion int64
	Name            string
	Protocol        string
	BaseURL         string
	APIKey          string
	Models          []string
}

func (s *Service) Update(ctx context.Context, tenantID, id int64, command UpdateCommand) (domain.View, error) {
	item, err := s.repository.GetTenant(ctx, tenantID, id)
	if err != nil {
		return domain.View{}, err
	}
	if command.ExpectedVersion < 1 || command.ExpectedVersion != item.Version {
		return domain.View{}, ErrVersionConflict
	}
	name, protocol, baseURL, models, err := normalize(command.Name, command.Protocol, command.BaseURL, command.Models)
	if err != nil {
		return domain.View{}, invalid(err)
	}
	item.Name, item.Protocol, item.BaseURL, item.Models = name, protocol, baseURL, models
	rotate := strings.TrimSpace(command.APIKey) != ""
	if rotate {
		if err = validateSecret(strings.TrimSpace(command.APIKey)); err != nil {
			return domain.View{}, invalid(err)
		}
		encrypted, encryptErr := s.secrets.EncryptString(ctx, strings.TrimSpace(command.APIKey))
		if encryptErr != nil {
			return domain.View{}, fmt.Errorf("encrypt model credential: %w", encryptErr)
		}
		item.SecretCiphertext, item.SecretEDEK, item.SecretWrapKeyRef = encrypted.CiphertextB64, encrypted.EDEKB64, encrypted.WrapKeyRef
		item.SecretHint = secretHint(command.APIKey)
	}
	var updated domain.Credential
	err = s.repository.WithLockedCredential(ctx, tenantID, id, func(current domain.Credential, repository CredentialUpdateRepository) error {
		if current.Version != command.ExpectedVersion {
			return ErrVersionConflict
		}
		if current.Protocol != protocol {
			uses, err := repository.CountActiveProtocolUses(ctx, tenantID, id)
			if err != nil {
				return err
			}
			if uses > 0 {
				return ErrProtocolInUse
			}
		}
		var err error
		updated, err = repository.Update(ctx, item, rotate)
		return err
	})
	if err != nil {
		return domain.View{}, err
	}
	return updated.View(), nil
}

func (s *Service) SetStatus(ctx context.Context, tenantID, id int64, status string, expectedVersion int64) (domain.View, error) {
	item, err := s.repository.GetTenant(ctx, tenantID, id)
	if err != nil {
		return domain.View{}, err
	}
	if expectedVersion != item.Version {
		return domain.View{}, ErrVersionConflict
	}
	status = strings.ToUpper(strings.TrimSpace(status))
	if status != domain.StatusActive && status != domain.StatusDisabled {
		return domain.View{}, invalid(errors.New("status is invalid"))
	}
	item.Status = status
	updated, err := s.repository.Update(ctx, item, false)
	if err != nil {
		return domain.View{}, err
	}
	if status == domain.StatusDisabled && s.canceller != nil {
		if err = s.canceller.CancelModelInvocations(ctx, tenantID, id, "MODEL_CREDENTIAL_DISABLED"); err != nil {
			return domain.View{}, err
		}
	}
	return updated.View(), nil
}

func (s *Service) Delete(ctx context.Context, tenantID, id, expectedVersion int64) (domain.DeleteResult, error) {
	if id <= 0 || expectedVersion <= 0 {
		return domain.DeleteResult{}, invalid(errors.New("model credential delete request is invalid"))
	}
	result, err := s.repository.Delete(ctx, tenantID, id, expectedVersion)
	if err != nil {
		return domain.DeleteResult{}, err
	}
	if result.RetainedHistory && s.canceller != nil {
		if err = s.canceller.CancelModelInvocations(ctx, tenantID, id, "MODEL_CREDENTIAL_DELETED"); err != nil {
			return domain.DeleteResult{}, err
		}
	}
	return result, nil
}

func (s *Service) ListTaskModelOptions(ctx context.Context, tenantID int64) ([]providerfacade.ModelOption, error) {
	if tenantID < 1 {
		return nil, ErrNotFound
	}
	return s.Options(ctx, tenantID)
}

func (s *Service) Options(ctx context.Context, tenantID int64) ([]domain.ModelOption, error) {
	items, err := s.repository.ListTenant(ctx, tenantID)
	if err != nil {
		return nil, err
	}
	result := make([]domain.ModelOption, 0)
	for _, item := range items {
		if item.Status != domain.StatusActive {
			continue
		}
		for _, model := range item.Models {
			result = append(result, domain.ModelOption{CredentialID: item.ID, CredentialName: item.Name, Protocol: item.Protocol, Model: model, Label: model + " · " + item.Name})
		}
	}
	return result, nil
}

type DiscoverCommand struct {
	CredentialID int64
	Protocol     string
	BaseURL      string
	APIKey       string
}

func (s *Service) Discover(ctx context.Context, tenantID int64, command DiscoverCommand) ([]string, error) {
	protocol, err := normalizeProtocol(command.Protocol)
	if err != nil {
		return nil, invalid(err)
	}
	baseURL, err := normalizeURL(command.BaseURL)
	if err != nil {
		return nil, invalid(err)
	}
	apiKey := strings.TrimSpace(command.APIKey)
	var item domain.Credential
	if command.CredentialID > 0 {
		item, err = s.repository.GetTenant(ctx, tenantID, command.CredentialID)
		if err != nil {
			return nil, err
		}
		if apiKey == "" {
			apiKey, err = s.decrypt(ctx, item)
			if err != nil {
				return nil, err
			}
		}
	} else if err = validateSecret(apiKey); err != nil {
		return nil, invalid(err)
	}
	models, err := s.discovery.Discover(ctx, domain.RuntimeCredential{CredentialID: command.CredentialID, Protocol: protocol, BaseURL: baseURL, APIKey: apiKey})
	if command.CredentialID > 0 {
		status, reason := "SUCCEEDED", ""
		if err != nil {
			status, reason = "FAILED", "MODEL_DISCOVERY_UNAVAILABLE"
		}
		_ = s.repository.UpdateDiscovery(ctx, tenantID, command.CredentialID, status, reason)
	}
	if err != nil {
		return nil, err
	}
	return normalizeModels(models)
}

func (s *Service) ValidateTaskSelection(ctx context.Context, tenantID, credentialID int64, model string) (domain.RuntimeSelection, error) {
	model = strings.TrimSpace(model)
	if credentialID <= 0 || model == "" {
		return domain.RuntimeSelection{}, taskModelSelectionError(ErrModelSelection)
	}
	item, err := s.repository.GetTenant(ctx, tenantID, credentialID)
	if err != nil {
		return domain.RuntimeSelection{}, taskModelSelectionError(err)
	}
	return validateTaskSelection(item, model)
}

// The caller owns the Task transaction; the share lock lasts until its commit.
func (s *Service) ValidateTaskSelectionInTransaction(ctx context.Context, tx pgx.Tx, tenantID, credentialID int64, model string) (domain.RuntimeSelection, error) {
	model = strings.TrimSpace(model)
	if tx == nil {
		return domain.RuntimeSelection{}, errors.New("model selection transaction is unavailable")
	}
	if credentialID <= 0 || model == "" {
		return domain.RuntimeSelection{}, taskModelSelectionError(ErrModelSelection)
	}
	item, err := s.repository.GetTenantForShare(ctx, tx, tenantID, credentialID)
	if err != nil {
		return domain.RuntimeSelection{}, taskModelSelectionError(err)
	}
	return validateTaskSelection(item, model)
}

func validateTaskSelection(item domain.Credential, model string) (domain.RuntimeSelection, error) {
	if item.Status != domain.StatusActive {
		return domain.RuntimeSelection{}, taskModelSelectionError(ErrDisabled)
	}
	if !item.HasModel(model) {
		return domain.RuntimeSelection{}, taskModelSelectionError(ErrModelNotInCatalog)
	}
	return domain.RuntimeSelection{CredentialID: item.ID, CredentialVersion: item.SecretVersion, Protocol: item.Protocol, PrimaryModel: model}, nil
}

func taskModelSelectionError(err error) error {
	if errors.Is(err, ErrModelSelection) || errors.Is(err, ErrModelNotInCatalog) || errors.Is(err, ErrDisabled) || errors.Is(err, ErrNotFound) {
		return fmt.Errorf("%w: %w", providerfacade.ErrInvalidTaskModelSelection, err)
	}
	return err
}

// ResolveInvocationCredential intentionally does not validate requestedModel
// against the catalog. The Agent may use helper models under the same fixed
// credential and upstream address.
func (s *Service) ResolveInvocationCredential(ctx context.Context, tenantID, credentialID, credentialVersion int64, primaryModel string) (domain.InvocationCredential, error) {
	item, err := s.repository.GetTenant(ctx, tenantID, credentialID)
	if err != nil {
		return domain.InvocationCredential{}, err
	}
	if item.Status != domain.StatusActive {
		return domain.InvocationCredential{}, ErrDisabled
	}
	apiKey, err := s.decrypt(ctx, item)
	if err != nil {
		return domain.InvocationCredential{}, err
	}
	return domain.InvocationCredential{CredentialID: item.ID, CredentialVersion: item.SecretVersion, Protocol: item.Protocol, BaseURL: item.BaseURL, PrimaryModel: strings.TrimSpace(primaryModel), APIKey: apiKey}, nil
}

func (s *Service) decrypt(ctx context.Context, item domain.Credential) (string, error) {
	return s.secrets.DecryptString(ctx, secretvalueinfra.SecretValue{CiphertextB64: item.SecretCiphertext, EDEKB64: item.SecretEDEK, WrapKeyRef: item.SecretWrapKeyRef})
}

func normalize(name, protocol, rawURL string, models []string) (string, string, string, []string, error) {
	name = strings.TrimSpace(name)
	if name == "" || utf8.RuneCountInString(name) > 100 {
		return "", "", "", nil, errors.New("name is invalid")
	}
	protocol, err := normalizeProtocol(protocol)
	if err != nil {
		return "", "", "", nil, err
	}
	baseURL, err := normalizeURL(rawURL)
	if err != nil {
		return "", "", "", nil, err
	}
	models, err = normalizeModels(models)
	if err != nil {
		return "", "", "", nil, err
	}
	return name, protocol, baseURL, models, nil
}

func normalizeProtocol(value string) (string, error) {
	value = strings.ToUpper(strings.TrimSpace(value))
	if value != domain.ProtocolOpenAI && value != domain.ProtocolAnthropic {
		return "", errors.New("protocol is invalid")
	}
	return value, nil
}

func normalizeURL(value string) (string, error) {
	value = strings.TrimSpace(value)
	if len(value) > 2048 {
		return "", errors.New("baseUrl is invalid")
	}
	parsed, err := url.Parse(value)
	if err != nil || (parsed.Scheme != "https" && parsed.Scheme != "http") || parsed.Host == "" || parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" {
		return "", errors.New("baseUrl is invalid")
	}
	return strings.TrimRight(parsed.String(), "/"), nil
}

func normalizeModels(values []string) ([]string, error) {
	if len(values) > 5000 {
		return nil, errors.New("too many models")
	}
	seen := map[string]struct{}{}
	result := make([]string, 0, len(values))
	for _, raw := range values {
		value := strings.TrimSpace(raw)
		if value == "" || len(value) > 256 || !utf8.ValidString(value) {
			return nil, errors.New("model is invalid")
		}
		for _, r := range value {
			if unicode.IsControl(r) {
				return nil, errors.New("model is invalid")
			}
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		result = append(result, value)
	}
	return result, nil
}

func validateSecret(value string) error {
	if len(value) < 8 || len(value) > 8192 {
		return errors.New("apiKey is invalid")
	}
	return nil
}

func secretHint(value string) string {
	value = strings.TrimSpace(value)
	if len(value) <= 4 {
		return "••••"
	}
	return "••••" + value[len(value)-4:]
}
