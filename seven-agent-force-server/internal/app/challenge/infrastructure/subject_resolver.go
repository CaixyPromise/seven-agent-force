package infrastructure

import (
	"context"
	"strconv"
	"strings"

	"github.com/CaixyPromise/seven-agent/force/internal/app/challenge/domain"
	userfacade "github.com/CaixyPromise/seven-agent/force/internal/app/system/user/facade"
)

type ResolvedSubject struct {
	UserID      int64
	AccountName string
	Email       string
	Enabled     bool
}

type SubjectResolver struct {
	users userfacade.SubjectFacade
}

func NewSubjectResolver(users userfacade.SubjectFacade) *SubjectResolver {
	return &SubjectResolver{users: users}
}

func (r *SubjectResolver) Resolve(ctx context.Context, session *domain.ChallengeSession) (*ResolvedSubject, error) {
	if session == nil {
		return nil, nil
	}
	raw := strings.TrimSpace(session.SubjectIdentifier)
	if raw == "" {
		return nil, nil
	}
	if strings.HasPrefix(raw, "user:") {
		id, err := strconv.ParseInt(strings.TrimSpace(strings.TrimPrefix(raw, "user:")), 10, 64)
		if err != nil || id <= 0 {
			return nil, nil
		}
		return r.findByID(ctx, id)
	}
	if strings.HasPrefix(raw, "login:") {
		account := strings.TrimSpace(strings.TrimPrefix(raw, "login:"))
		if account == "" {
			return nil, nil
		}
		return r.findByAccount(ctx, account)
	}
	return nil, nil
}

func (r *SubjectResolver) findByID(ctx context.Context, userID int64) (*ResolvedSubject, error) {
	if r == nil || r.users == nil || userID <= 0 {
		return nil, nil
	}
	record, err := r.users.FindSubjectByID(ctx, userID)
	if err != nil || record == nil {
		return nil, err
	}
	return &ResolvedSubject{
		UserID:      record.UserID,
		AccountName: record.AccountName,
		Email:       strings.TrimSpace(record.Email),
		Enabled:     record.Enabled,
	}, nil
}

func (r *SubjectResolver) findByAccount(ctx context.Context, account string) (*ResolvedSubject, error) {
	if r == nil || r.users == nil {
		return nil, nil
	}
	record, err := r.users.FindSubjectByAccount(ctx, account)
	if err != nil || record == nil {
		return nil, err
	}
	return &ResolvedSubject{
		UserID:      record.UserID,
		AccountName: record.AccountName,
		Email:       strings.TrimSpace(record.Email),
		Enabled:     record.Enabled,
	}, nil
}
