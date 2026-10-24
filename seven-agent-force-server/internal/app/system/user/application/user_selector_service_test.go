package application

import (
	"context"
	"testing"

	"github.com/CaixyPromise/seven-agent/force/internal/app/system/user/domain"
	userfacade "github.com/CaixyPromise/seven-agent/force/internal/app/system/user/facade"
	apperrors "github.com/CaixyPromise/seven-agent/force/internal/common/errors"
)

func TestListUserOptionsNormalizesBoundedScopedQuery(t *testing.T) {
	repo := &fakeRepository{
		selectorRecords: []domain.UserSelectorRecord{{
			ID: 2065424359060983808, AccountName: "alice", NickName: "Alice", Avatar: "/avatar.png", Status: 0,
		}},
	}
	service := newTestService(repo, nil)
	scope := userfacade.DataScopeFilter{Enabled: true, ScopeType: "DEPT", DeptIDs: []int64{7}}

	result, err := service.ListUserOptions(context.Background(), userfacade.UserSelectorQuery{
		Keyword: "  ali  ",
		Limit:   999,
		DeptID:  7,
		Scope:   scope,
	})
	if err != nil {
		t.Fatalf("list user options: %v", err)
	}
	if len(result) != 1 || result[0].ID != 2065424359060983808 || result[0].Username != "alice" || result[0].NickName != "Alice" {
		t.Fatalf("unexpected options: %#v", result)
	}
	if repo.selectorQuery.Keyword != "ali" || repo.selectorQuery.Limit != maxUserSelectorLimit || repo.selectorQuery.DeptID != 7 {
		t.Fatalf("selector query was not normalized: %#v", repo.selectorQuery)
	}
	if !repo.selectorQuery.Scope.Enabled || len(repo.selectorQuery.Scope.DeptIDs) != 1 || repo.selectorQuery.Scope.DeptIDs[0] != 7 {
		t.Fatalf("selector scope was not preserved: %#v", repo.selectorQuery.Scope)
	}
}

func TestGetSimpleUserUsesVisibilityScopeAndHidesMissingUsers(t *testing.T) {
	repo := &fakeRepository{
		selectorRecord: &domain.UserSelectorRecord{
			ID: 2065424359060983808, AccountName: "alice", NickName: "Alice", Status: 1,
		},
	}
	service := newTestService(repo, nil)
	scope := userfacade.DataScopeFilter{Enabled: true, SelfUserID: 2065424359060983808}

	result, err := service.GetSimpleUser(context.Background(), 2065424359060983808, scope)
	if err != nil {
		t.Fatalf("get simple user: %v", err)
	}
	if result == nil || result.ID != 2065424359060983808 || result.Status != 1 {
		t.Fatalf("unexpected simple user: %#v", result)
	}
	if repo.selectorUserID != 2065424359060983808 || repo.selectorScope.SelfUserID != 2065424359060983808 {
		t.Fatalf("visibility scope was not passed to repository: id=%d scope=%#v", repo.selectorUserID, repo.selectorScope)
	}

	repo.selectorRecord = nil
	result, err = service.GetSimpleUser(context.Background(), 2065424359060983808, scope)
	if result != nil {
		t.Fatalf("expected hidden user to return nil, got %#v", result)
	}
	if appErr := apperrors.From(err); appErr == nil || appErr.Code() != apperrors.CodeNotFound {
		t.Fatalf("expected not found for hidden user, got %v", err)
	}
}
