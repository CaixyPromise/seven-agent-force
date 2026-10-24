package user

import (
	"context"
	"testing"

	adminfacade "github.com/CaixyPromise/seven-agent/force/internal/app/system/admin/facade"
	"github.com/cloudwego/hertz/pkg/app"
)

func TestUserStatusOperationEnricherClassifiesAdminUnlock(t *testing.T) {
	reqCtx := &app.RequestContext{}
	reqCtx.Request.SetRequestURI("/user/status/1001?status=0")
	entry := adminfacade.OperationLogEntry{
		OperationType: adminfacade.OperationTypeUserUpdateStatus,
		OperationDesc: "修改用户状态",
	}

	userStatusOperationEnricher{}.Enrich(context.Background(), reqCtx, &entry)

	if entry.OperationType != adminfacade.OperationTypeAdminUnlockAccount {
		t.Fatalf("expected admin unlock operation, got %s", entry.OperationType)
	}
	if entry.OperationDesc != adminfacade.OperationTypeAdminUnlockAccount.Description() {
		t.Fatalf("unexpected operation description: %q", entry.OperationDesc)
	}
}

func TestUserStatusOperationEnricherClassifiesAdminBan(t *testing.T) {
	reqCtx := &app.RequestContext{}
	reqCtx.Request.SetRequestURI("/user/status/1001?status=1")
	entry := adminfacade.OperationLogEntry{
		OperationType: adminfacade.OperationTypeUserUpdateStatus,
		OperationDesc: "修改用户状态",
	}

	userStatusOperationEnricher{}.Enrich(context.Background(), reqCtx, &entry)

	if entry.OperationType != adminfacade.OperationTypeAdminBanUser {
		t.Fatalf("expected admin ban operation, got %s", entry.OperationType)
	}
	if entry.OperationDesc != adminfacade.OperationTypeAdminBanUser.Description() {
		t.Fatalf("unexpected operation description: %q", entry.OperationDesc)
	}
}

func TestUserStatusOperationEnricherKeepsGenericStatusForUnknownValue(t *testing.T) {
	reqCtx := &app.RequestContext{}
	reqCtx.Request.SetRequestURI("/user/status/1001?status=9")
	entry := adminfacade.OperationLogEntry{
		OperationType: adminfacade.OperationTypeAdminUnlockAccount,
		OperationDesc: "管理员解锁账号",
	}

	userStatusOperationEnricher{}.Enrich(context.Background(), reqCtx, &entry)

	if entry.OperationType != adminfacade.OperationTypeUserUpdateStatus {
		t.Fatalf("expected generic user status operation, got %s", entry.OperationType)
	}
	if entry.OperationDesc != adminfacade.OperationTypeUserUpdateStatus.Description() {
		t.Fatalf("unexpected operation description: %q", entry.OperationDesc)
	}
}
