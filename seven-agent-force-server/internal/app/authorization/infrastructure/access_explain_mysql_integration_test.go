package infrastructure

import (
	"context"
	"os"
	"testing"

	"github.com/CaixyPromise/seven-agent/force/internal/infrastructure/datasource/mysql"
	"github.com/CaixyPromise/seven-agent/force/internal/shared/config"
)

func TestMySQLAccessExplainQueriesPreserveSources(t *testing.T) {
	dsn := os.Getenv("RBAC_MYSQL_DSN")
	if dsn == "" || os.Getenv("RBAC_MYSQL_ALLOW_MUTATION") != "1" {
		t.Skip("RBAC_MYSQL_DSN and RBAC_MYSQL_ALLOW_MUTATION=1 are required")
	}
	provider, err := mysql.NewProvider(config.MySQLConfig{Enabled: true, DSN: dsn, MaxOpenConns: 2, MaxIdleConns: 1}, nil)
	if err != nil {
		t.Fatalf("open MySQL: %v", err)
	}
	t.Cleanup(func() { _ = provider.Close() })

	const (
		userID       int64 = 9190719001
		roleID       int64 = 9190719002
		permissionID int64 = 9190719003
		menuID       int64 = 9190719004
		postID       int64 = 9190719005
		orgID        int64 = 9190719006
		deptID       int64 = 9190719007
	)
	ctx := context.Background()
	db := provider.DB()
	cleanup := func() {
		statements := []string{
			"DELETE FROM sys_role_dept WHERE roleId = 9190719002",
			"DELETE FROM sys_user_permission WHERE userId = 9190719001",
			"DELETE FROM sys_role_permission WHERE roleId = 9190719002",
			"DELETE FROM sys_role_menu WHERE roleId = 9190719002",
			"DELETE FROM sys_post_role WHERE roleId = 9190719002",
			"DELETE FROM sys_user_position WHERE userId = 9190719001",
			"DELETE FROM sys_user_dept WHERE userId = 9190719001",
			"DELETE FROM sys_user_org WHERE userId = 9190719001",
			"DELETE FROM sys_user_role WHERE userId = 9190719001",
			"DELETE FROM sys_menu WHERE id = 9190719004",
			"DELETE FROM sys_permission WHERE id = 9190719003",
			"DELETE FROM sys_post WHERE id = 9190719005",
			"DELETE FROM sys_dept WHERE id = 9190719007",
			"DELETE FROM sys_org WHERE id = 9190719006",
			"DELETE FROM sys_role WHERE id = 9190719002",
			"DELETE FROM sys_user WHERE id = 9190719001",
		}
		for _, statement := range statements {
			_, _ = db.ExecContext(context.Background(), statement)
		}
	}
	cleanup()
	t.Cleanup(cleanup)

	seed := []string{
		"INSERT INTO sys_user (id,userAccount,nickName,status,userEmail,userGender,isDeleted) VALUES (9190719001,'batch_b_query_user','Batch B Query User',0,'batch-b@example.test',0,0)",
		"INSERT INTO sys_role (id,name,code,dataScope,status,type,sortOrder,isDeleted) VALUES (9190719002,'Batch B Role','BATCH_B_QUERY_ROLE',2,0,3,0,0)",
		"INSERT INTO sys_org (id,code,name,parentId,status,sortOrder,isDeleted) VALUES (9190719006,'BATCH_B_ORG','Batch B Org',0,0,0,0)",
		"INSERT INTO sys_dept (id,name,code,orgId,parentId,status,sortOrder,hierarchy,level,isDeleted) VALUES (9190719007,'Batch B Dept','BATCH_B_DEPT',9190719006,0,0,0,'9190719007',1,0)",
		"INSERT INTO sys_post (id,code,name,deptId,orgId,sortOrder,status,isDeleted) VALUES (9190719005,'BATCH_B_POST','Batch B Post',9190719007,9190719006,0,0,0)",
		"INSERT INTO sys_permission (id,code,name,resourceType,status,isDeleted) VALUES (9190719003,'batch:b:query','Batch B Query','API',0,0)",
		"INSERT INTO sys_menu (id,name,parentId,sortOrder,path,type,permission,status,visible,isDeleted) VALUES (9190719004,'Batch B Menu',0,0,'/batch-b','C','batch:b:menu',0,1,0)",
		"INSERT INTO sys_user_role (userId,roleId,isDeleted) VALUES (9190719001,9190719002,0)",
		"INSERT INTO sys_user_position (userId,postId,isPrimary,isDeleted) VALUES (9190719001,9190719005,1,0)",
		"INSERT INTO sys_post_role (postId,roleId) VALUES (9190719005,9190719002)",
		"INSERT INTO sys_role_permission (id,roleId,permissionId,source) VALUES (9190719101,9190719002,9190719003,'DIRECT')",
		"INSERT INTO sys_role_menu (id,roleId,menuId) VALUES (9190719102,9190719002,9190719004)",
		"INSERT INTO sys_role_dept (id,roleId,deptId) VALUES (9190719103,9190719002,9190719007)",
		"INSERT INTO sys_user_org (userId,orgId,isPrimary,isDeleted) VALUES (9190719001,9190719006,1,0)",
		"INSERT INTO sys_user_dept (userId,deptId,isPrimary) VALUES (9190719001,9190719007,1)",
	}
	for _, statement := range seed {
		if _, err := db.ExecContext(ctx, statement); err != nil {
			t.Fatalf("seed access explain fixture: %v\nSQL: %s", err, statement)
		}
	}

	repository := &Repository{db: provider.SQLX()}
	user, err := repository.FindAccessUser(ctx, userID)
	if err != nil || user == nil || user.Username != "batch_b_query_user" {
		t.Fatalf("FindAccessUser() user=%#v err=%v", user, err)
	}
	roles, err := repository.ListAccessRoleSources(ctx, userID)
	if err != nil || len(roles) != 2 {
		t.Fatalf("ListAccessRoleSources() len=%d err=%v records=%#v", len(roles), err, roles)
	}
	grants, err := repository.ListAccessGrantRecords(ctx, userID)
	if err != nil || len(grants) != 4 {
		t.Fatalf("ListAccessGrantRecords() len=%d err=%v records=%#v", len(grants), err, grants)
	}
	roleDepts, err := repository.ListAccessRoleDeptRecords(ctx, []int64{roleID})
	if err != nil || len(roleDepts) != 1 || roleDepts[0].DeptID != deptID {
		t.Fatalf("ListAccessRoleDeptRecords() records=%#v err=%v", roleDepts, err)
	}
	memberships, err := repository.ListAccessMemberships(ctx, userID)
	if err != nil || len(memberships) != 2 {
		t.Fatalf("ListAccessMemberships() records=%#v err=%v", memberships, err)
	}
}
