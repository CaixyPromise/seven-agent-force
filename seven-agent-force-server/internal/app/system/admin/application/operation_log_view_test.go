package application

import (
	"testing"

	"github.com/CaixyPromise/seven-agent/force/internal/app/system/admin/domain"
	adminfacade "github.com/CaixyPromise/seven-agent/force/internal/app/system/admin/facade"
)

func TestToOperationLogVOKeepsFilterCategoryAndReturnsUserFacingLabel(t *testing.T) {
	other := toOperationLogVO(domain.OperationLog{
		OperationType: string(adminfacade.OperationTypeOther),
		OperationDesc: "分页查询角色",
	})
	if other.OperationType != string(adminfacade.OperationTypeOther) {
		t.Fatalf("filter category changed: got=%q", other.OperationType)
	}
	if other.OperationTypeDesc != "其他" {
		t.Fatalf("unexpected stable category label: got=%q want=%q", other.OperationTypeDesc, "其他")
	}
	if other.OperationTypeLabel != "分页查询角色" {
		t.Fatalf("unexpected user-facing label: got=%q want=%q", other.OperationTypeLabel, "分页查询角色")
	}

	login := toOperationLogVO(domain.OperationLog{
		OperationType: string(adminfacade.OperationTypeUserLogin),
		OperationDesc: "密码登录",
	})
	if login.OperationTypeLabel != "用户登录" {
		t.Fatalf("known type must use canonical user-facing label: got=%q want=%q", login.OperationTypeLabel, "用户登录")
	}
}
