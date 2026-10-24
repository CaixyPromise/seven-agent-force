package application

import (
	"context"
	"strings"

	"github.com/CaixyPromise/seven-agent/force/internal/app/system/user/domain"
	userfacade "github.com/CaixyPromise/seven-agent/force/internal/app/system/user/facade"
	apperrors "github.com/CaixyPromise/seven-agent/force/internal/common/errors"
)

const (
	defaultUserSelectorLimit = 20
	maxUserSelectorLimit     = 50
)

// ListUserOptions returns a bounded, data-scoped minimum user projection.
func (s *Service) ListUserOptions(ctx context.Context, query userfacade.UserSelectorQuery) ([]userfacade.SimpleUserVO, error) {
	records, err := s.repo.ListUserOptions(ctx, domain.UserSelectorQuery{
		Keyword: strings.TrimSpace(query.Keyword),
		Limit:   normalizeUserSelectorLimit(query.Limit),
		DeptID:  query.DeptID,
		Scope:   toDomainScope(query.Scope),
	})
	if err != nil {
		return nil, err
	}
	result := make([]userfacade.SimpleUserVO, 0, len(records))
	for _, record := range records {
		result = append(result, toSimpleUserVO(record))
	}
	return result, nil
}

// GetSimpleUser returns one minimum user projection when it is visible in the caller data scope.
func (s *Service) GetSimpleUser(ctx context.Context, userID int64, scope userfacade.DataScopeFilter) (*userfacade.SimpleUserVO, error) {
	if userID <= 0 {
		return nil, apperrors.Params("用户ID不能为空")
	}
	record, err := s.repo.FindVisibleUserOptionByID(ctx, userID, toDomainScope(scope))
	if err != nil {
		return nil, err
	}
	if record == nil {
		return nil, apperrors.NotFound("用户不存在")
	}
	result := toSimpleUserVO(*record)
	return &result, nil
}

func normalizeUserSelectorLimit(limit int) int {
	if limit <= 0 {
		return defaultUserSelectorLimit
	}
	if limit > maxUserSelectorLimit {
		return maxUserSelectorLimit
	}
	return limit
}

func toSimpleUserVO(record domain.UserSelectorRecord) userfacade.SimpleUserVO {
	return userfacade.SimpleUserVO{
		ID:       record.ID,
		Username: record.AccountName,
		NickName: record.NickName,
		Avatar:   record.Avatar,
		Status:   record.Status,
	}
}
