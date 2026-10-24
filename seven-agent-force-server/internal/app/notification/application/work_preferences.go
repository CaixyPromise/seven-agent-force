package application

import (
	"context"
	"errors"
	"strings"

	"github.com/CaixyPromise/seven-agent/force/internal/app/notification/facade"
	apperrors "github.com/CaixyPromise/seven-agent/force/internal/common/errors"
)

var (
	ErrNotificationPreferenceInvalid  = errors.New("NOTIFICATION_PREFERENCE_INVALID")
	ErrNotificationPreferenceConflict = errors.New("NOTIFICATION_PREFERENCE_CONFLICT")
)

type workPreferenceRepository interface {
	ListWorkPreferences(context.Context, int64, int64) ([]facade.WorkPreference, error)
	FindWorkPreference(context.Context, int64, int64, string, string) (*facade.WorkPreference, error)
	InsertWorkPreference(context.Context, facade.WorkPreference) error
	UpdateWorkPreference(context.Context, facade.WorkPreference, int64) (bool, error)
}

func validWorkPreference(eventFamily, channelKind string) bool {
	eventFamily = strings.ToUpper(strings.TrimSpace(eventFamily))
	channelKind = strings.ToUpper(strings.TrimSpace(channelKind))
	validEvent := eventFamily == "TASK_ACTION_REQUIRED" || eventFamily == "TASK_INTERRUPTED" || eventFamily == "TASK_COMPLETED"
	validChannel := channelKind == "IN_APP" || channelKind == "EMAIL_USER" || channelKind == "FEISHU_DIRECT" || channelKind == "WECOM_DIRECT"
	return validEvent && validChannel
}

func (s *Service) ListMyWorkPreferences(ctx context.Context, organizationID, userID int64) ([]facade.WorkPreference, error) {
	if organizationID <= 0 || userID <= 0 {
		return nil, apperrors.Params(ErrNotificationPreferenceInvalid.Error())
	}
	repo, ok := s.repo.(workPreferenceRepository)
	if !ok {
		return nil, apperrors.Operation("工作通知设置不可用")
	}
	return repo.ListWorkPreferences(ctx, organizationID, userID)
}

func (s *Service) UpdateMyWorkPreference(ctx context.Context, organizationID, userID int64, eventFamily, channelKind string, request facade.WorkPreferenceUpdate) (*facade.WorkPreference, error) {
	eventFamily = strings.ToUpper(strings.TrimSpace(eventFamily))
	channelKind = strings.ToUpper(strings.TrimSpace(channelKind))
	if organizationID <= 0 || userID <= 0 || request.ExpectedVersion < 0 || !validWorkPreference(eventFamily, channelKind) {
		return nil, apperrors.Params(ErrNotificationPreferenceInvalid.Error())
	}
	repo, ok := s.repo.(workPreferenceRepository)
	if !ok {
		return nil, apperrors.Operation("工作通知设置不可用")
	}
	current, err := repo.FindWorkPreference(ctx, organizationID, userID, eventFamily, channelKind)
	if err != nil {
		return nil, err
	}
	if current == nil {
		if request.ExpectedVersion != 0 {
			return nil, apperrors.ObjectState(ErrNotificationPreferenceConflict.Error())
		}
		item := facade.WorkPreference{ID: s.nextID(), OrganizationID: organizationID, UserID: userID, EventFamily: eventFamily, ChannelKind: channelKind, Enabled: request.Enabled, Version: 1}
		if item.ID <= 0 {
			return nil, apperrors.Operation("工作通知设置身份生成失败")
		}
		if err = repo.InsertWorkPreference(ctx, item); err != nil {
			if replay, lookupErr := repo.FindWorkPreference(ctx, organizationID, userID, eventFamily, channelKind); lookupErr == nil && replay != nil {
				return nil, apperrors.ObjectState(ErrNotificationPreferenceConflict.Error())
			}
			return nil, err
		}
		return &item, nil
	}
	if request.ExpectedVersion != current.Version {
		return nil, apperrors.ObjectState(ErrNotificationPreferenceConflict.Error())
	}
	current.Enabled = request.Enabled
	ok, err = repo.UpdateWorkPreference(ctx, *current, request.ExpectedVersion)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, apperrors.ObjectState(ErrNotificationPreferenceConflict.Error())
	}
	current.Version++
	return current, nil
}

func (s *Service) workPreferenceEnabled(ctx context.Context, organizationID, userID int64, eventFamily, channelKind string) (bool, error) {
	repo, ok := s.repo.(workPreferenceRepository)
	if !ok {
		return true, nil
	}
	item, err := repo.FindWorkPreference(ctx, organizationID, userID, eventFamily, channelKind)
	if err != nil {
		return false, err
	}
	if item == nil {
		return true, nil
	}
	return item.Enabled, nil
}

func (s *Service) IsWorkPreferenceEnabled(ctx context.Context, organizationID, userID int64, eventFamily, channelKind string) (bool, error) {
	if organizationID <= 0 || userID <= 0 || !validWorkPreference(eventFamily, channelKind) {
		return false, apperrors.Params(ErrNotificationPreferenceInvalid.Error())
	}
	return s.workPreferenceEnabled(ctx, organizationID, userID, strings.ToUpper(strings.TrimSpace(eventFamily)), strings.ToUpper(strings.TrimSpace(channelKind)))
}
