package schemaalignment

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
)

func ensureTaskAnalyticsSchema(ctx context.Context, tx pgx.Tx) error {
	if _, err := tx.Exec(ctx, `
	CREATE INDEX IF NOT EXISTS workforce_tasks_tenant_project_finished_idx ON workforce_tasks (tenant_id,project_id,finished_at DESC,id) WHERE finished_at IS NOT NULL;
	CREATE INDEX IF NOT EXISTS workforce_delivery_requests_tenant_project_created_idx ON workforce_delivery_requests (tenant_id,project_id,created_at DESC,id);
	CREATE INDEX IF NOT EXISTS workforce_completion_check_results_tenant_task_created_idx ON workforce_completion_check_results (tenant_id,task_id,created_at DESC,id);`); err != nil {
		return fmt.Errorf("ensure Task analytics indexes: %w", err)
	}
	if _, err := tx.Exec(ctx, `
	UPDATE sys_menu
	SET name='任务',remark='查看当前组织的 Agent 任务',"updateTime"=CURRENT_TIMESTAMP
	WHERE path='/workforce/tasks' AND "isDeleted"=false;`); err != nil {
		return fmt.Errorf("align Task menu: %w", err)
	}

	var nextMenuID int64
	if err := tx.QueryRow(ctx, `SELECT GREATEST(COALESCE(MAX(id),0),2026082412100000100)+1 FROM sys_menu`).Scan(&nextMenuID); err != nil {
		return fmt.Errorf("allocate Workbench menu identity: %w", err)
	}
	if _, err := tx.Exec(ctx, `
	INSERT INTO sys_menu (id,name,"parentId","sortOrder",path,component,icon,type,permission,"isFrame","isCache",visible,hierarchy,level,status,"creatorId","createTime","updaterId","updateTime","isDeleted",remark)
	SELECT $1::BIGINT,'工作台',parent.id,5,'/','index','HomeOutlined','C','workforce:task:list',true,true,true,
	       CONCAT(COALESCE(NULLIF(parent.hierarchy,''),'/1'),'/', ($1::BIGINT)::text),
	       parent.level+1,0,0,CURRENT_TIMESTAMP,0,CURRENT_TIMESTAMP,false,'查看正在进行的工作、待处理事项和平台运行状态'
	FROM sys_menu parent
	WHERE parent.path='/workbench' AND parent."isDeleted"=false
	  AND NOT EXISTS (SELECT 1 FROM sys_menu WHERE path='/' AND "isDeleted"=false);`, nextMenuID); err != nil {
		return fmt.Errorf("align Workbench menu: %w", err)
	}

	var workbenchMenuID, taskListPermissionID int64
	if err := tx.QueryRow(ctx, `SELECT id FROM sys_menu WHERE path='/' AND "isDeleted"=false`).Scan(&workbenchMenuID); err != nil {
		return fmt.Errorf("resolve Workbench menu: %w", err)
	}
	if err := tx.QueryRow(ctx, `SELECT id FROM sys_permission WHERE code='workforce:task:list' AND "isDeleted"=false`).Scan(&taskListPermissionID); err != nil {
		return fmt.Errorf("resolve Task list permission: %w", err)
	}

	var nextMenuPermissionID int64
	if err := tx.QueryRow(ctx, `SELECT GREATEST(COALESCE(MAX(id),0),2026082412100000200)+1 FROM sys_menu_permission`).Scan(&nextMenuPermissionID); err != nil {
		return fmt.Errorf("allocate Workbench menu permission identity: %w", err)
	}
	if _, err := tx.Exec(ctx, `
	INSERT INTO sys_menu_permission (id,"menuId","permissionId","creatorId","createTime")
	SELECT $1,$2,$3,0,CURRENT_TIMESTAMP
	WHERE NOT EXISTS (SELECT 1 FROM sys_menu_permission current WHERE current."menuId"=$2 AND current."permissionId"=$3);`, nextMenuPermissionID, workbenchMenuID, taskListPermissionID); err != nil {
		return fmt.Errorf("align Workbench menu permission: %w", err)
	}

	var nextRoleMenuID int64
	if err := tx.QueryRow(ctx, `SELECT GREATEST(COALESCE(MAX(id),0),2026082412100000300) FROM sys_role_menu`).Scan(&nextRoleMenuID); err != nil {
		return fmt.Errorf("allocate Workbench role menu identities: %w", err)
	}
	if _, err := tx.Exec(ctx, `
	INSERT INTO sys_role_menu (id,"roleId","menuId","createTime","updateTime")
	SELECT $1+ROW_NUMBER() OVER (ORDER BY role_permission."roleId"),
	       role_permission."roleId",$2,CURRENT_TIMESTAMP,CURRENT_TIMESTAMP
	FROM sys_role_permission role_permission
	WHERE role_permission."permissionId"=$3
	  AND NOT EXISTS (SELECT 1 FROM sys_role_menu current WHERE current."roleId"=role_permission."roleId" AND current."menuId"=$2);`, nextRoleMenuID, workbenchMenuID, taskListPermissionID); err != nil {
		return fmt.Errorf("align Workbench role menus: %w", err)
	}
	return nil
}

func verifyTaskAnalyticsSchema(ctx context.Context, db DB) error {
	var count int
	err := db.QueryRow(ctx, `SELECT count(*) FROM pg_indexes WHERE schemaname='public' AND indexname IN (
		'workforce_tasks_tenant_project_finished_idx',
		'workforce_delivery_requests_tenant_project_created_idx',
		'workforce_completion_check_results_tenant_task_created_idx')`).Scan(&count)
	if err != nil {
		return err
	}
	if count != 3 {
		return fmt.Errorf("Task analytics indexes are incomplete")
	}
	var menuAligned bool
	if err = db.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM sys_menu WHERE path='/workforce/tasks' AND "isDeleted"=false AND name='任务' AND remark='查看当前组织的 Agent 任务')`).Scan(&menuAligned); err != nil {
		return err
	}
	if !menuAligned {
		return fmt.Errorf("Task menu is not aligned")
	}
	var workbenchAligned bool
	if err = db.QueryRow(ctx, `SELECT EXISTS(
		SELECT 1 FROM sys_menu menu
		JOIN sys_menu_permission relation ON relation."menuId"=menu.id
		JOIN sys_permission permission ON permission.id=relation."permissionId"
		WHERE menu.path='/' AND menu.name='工作台' AND menu.icon='HomeOutlined' AND menu."isDeleted"=false AND permission.code='workforce:task:list' AND permission."isDeleted"=false
	)`).Scan(&workbenchAligned); err != nil {
		return err
	}
	if !workbenchAligned {
		return fmt.Errorf("Workbench menu is not aligned")
	}
	return nil
}
