package application

import (
	"context"
	"errors"
	routingfacade "github.com/CaixyPromise/seven-agent/force/internal/app/workforce/executionrouting/facade"
	"github.com/CaixyPromise/seven-agent/force/internal/app/workforce/openintegration/domain"
	projectfacade "github.com/CaixyPromise/seven-agent/force/internal/app/workforce/project/facade"
	providerfacade "github.com/CaixyPromise/seven-agent/force/internal/app/workforce/provider/facade"
)

var ErrSelectionUnavailable = errors.New("open task selection queries unavailable")

func (s *Service) SetSelectionCatalogs(models providerfacade.TaskModelCatalog, projects projectfacade.RequirementsCompiler, runtimes routingfacade.RuntimeCatalog) {
	s.models, s.projects, s.runtimes = models, projects, runtimes
}

func (s *Service) validateSelectionProject(ctx context.Context, identity domain.Identity, projectID int64) error {
	if identity.ApplicationID <= 0 || identity.TenantID <= 0 || identity.CreatedByUserID <= 0 || projectID <= 0 {
		return domain.ErrInvalidRequest
	}
	if s.projects == nil {
		return ErrSelectionUnavailable
	}
	// This is the same tenant-level Project visibility as creation. It does not
	// impersonate the application's owner or inherit operator RBAC.
	_, err := s.projects.CompileExecutionRequirements(ctx, identity.TenantID, projectID)
	return err
}

func (s *Service) ListModelOptions(ctx context.Context, identity domain.Identity, projectID int64) (domain.ModelOptions, error) {
	if err := s.validateSelectionProject(ctx, identity, projectID); err != nil {
		return domain.ModelOptions{}, err
	}
	if s.models == nil {
		return domain.ModelOptions{}, ErrSelectionUnavailable
	}
	options, err := s.models.ListTaskModelOptions(ctx, identity.TenantID)
	if err != nil {
		return domain.ModelOptions{}, err
	}
	result := domain.ModelOptions{Schema: "seven.workforce.open-model-options/v1", ProjectID: projectID, Items: make([]domain.ModelOption, 0, len(options))}
	for _, option := range options {
		result.Items = append(result.Items, domain.ModelOption{CredentialID: option.CredentialID, Model: option.Model, Protocol: option.Protocol, Label: option.Label})
	}
	return result, nil
}

func (s *Service) ListRuntimeOptions(ctx context.Context, identity domain.Identity, projectID, credentialID int64) (domain.RuntimeOptions, error) {
	if err := s.validateSelectionProject(ctx, identity, projectID); err != nil {
		return domain.RuntimeOptions{}, err
	}
	if credentialID <= 0 {
		return domain.RuntimeOptions{}, domain.ErrInvalidRequest
	}
	if s.runtimes == nil {
		return domain.RuntimeOptions{}, ErrSelectionUnavailable
	}
	options, err := s.runtimes.ListTaskOptions(ctx, identity.TenantID, projectID, credentialID)
	if err != nil {
		return domain.RuntimeOptions{}, err
	}
	result := domain.RuntimeOptions{Schema: "seven.workforce.open-runtime-options/v1", ProjectVersionID: options.ProjectVersionID, Items: make([]domain.RuntimeOption, 0, len(options.Items))}
	for _, option := range options.Items {
		result.Items = append(result.Items, domain.RuntimeOption{RuntimeReleaseID: option.RuntimeReleaseID, Name: option.Name, Recommended: option.Recommended})
	}
	return result, nil
}
