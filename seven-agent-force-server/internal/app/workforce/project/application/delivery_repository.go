package application

import (
	"context"
	"github.com/CaixyPromise/seven-agent/force/internal/app/workforce/project/domain"
	projectfacade "github.com/CaixyPromise/seven-agent/force/internal/app/workforce/project/facade"
)

func (s *Service) DeliveryRepositoryBinding(ctx context.Context, tenant, project, version int64) (projectfacade.DeliveryRepositoryBinding, error) {
	reader, ok := s.repository.(projectfacade.DeliveryRepositoryBindingReader)
	if !ok || tenant <= 0 || project <= 0 || version <= 0 {
		return projectfacade.DeliveryRepositoryBinding{}, domain.ErrInvalid
	}
	return reader.DeliveryRepositoryBinding(ctx, tenant, project, version)
}
