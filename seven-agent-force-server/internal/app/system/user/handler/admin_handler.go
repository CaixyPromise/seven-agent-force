package handler

import (
	"context"
	"fmt"
	"sort"
	"strconv"
	"strings"

	authorizationfacade "github.com/CaixyPromise/seven-agent/force/internal/app/authorization/facade"
	challengedomain "github.com/CaixyPromise/seven-agent/force/internal/app/challenge/domain"
	userfacade "github.com/CaixyPromise/seven-agent/force/internal/app/system/user/facade"
	apperrors "github.com/CaixyPromise/seven-agent/force/internal/common/errors"
	"github.com/CaixyPromise/seven-agent/force/internal/common/httpx"
	"github.com/CaixyPromise/seven-agent/force/internal/common/response"
	"github.com/CaixyPromise/seven-agent/force/internal/shared/securitycontext"
	"github.com/CaixyPromise/seven-agent/force/internal/shared/stepup"
	"github.com/cloudwego/hertz/pkg/app"
)

type AdminHandler struct {
	users     userfacade.AdminUserFacade
	relations userfacade.UserRelationFacade
	orgs      userfacade.OrgFacade
	depts     userfacade.DeptFacade
	posts     userfacade.PostFacade
	auth      authorizationfacade.AuthFacade
	access    authorizationfacade.AccessExplainFacade
}

func (c *AdminHandler) BindAccessExplain(access authorizationfacade.AccessExplainFacade) {
	if c == nil {
		return
	}
	c.access = access
}

func NewAdminHandler(users userfacade.AdminUserFacade, relations userfacade.UserRelationFacade, orgs userfacade.OrgFacade, depts userfacade.DeptFacade, posts userfacade.PostFacade) *AdminHandler {
	return &AdminHandler{users: users, relations: relations, orgs: orgs, depts: depts, posts: posts}
}

func (c *AdminHandler) BindAuthorization(auth authorizationfacade.AuthFacade) {
	if c == nil {
		return
	}
	c.auth = auth
}

func (c *AdminHandler) QueryUsers(ctx context.Context, reqCtx *app.RequestContext) {
	result, err := c.users.QueryUsers(ctx, userfacade.AdminUserQuery{Current: queryInt64(reqCtx, "current", 1), Size: queryInt64(reqCtx, "size", 10), Username: queryString(reqCtx, "username"), Nickname: queryString(reqCtx, "nickname"), Status: queryIntPtr(reqCtx, "status"), OrgID: queryInt64(reqCtx, "orgId", 0), DeptID: queryInt64(reqCtx, "deptId", 0), PostID: queryInt64(reqCtx, "postId", 0), Scope: dataScopeFilter(reqCtx)})
	write(reqCtx, result, err)
}

func (c *AdminHandler) GetUser(ctx context.Context, reqCtx *app.RequestContext) {
	id, err := pathInt64(reqCtx, "id")
	if err != nil {
		response.Error(reqCtx, err)
		return
	}
	result, err := c.users.GetAdminUser(ctx, id)
	if err == nil && result != nil && !dataScopeAllowsUser(ctx, c.relations, reqCtx, id) {
		err = apperrors.DataScopeDenied("数据范围不足")
		result = nil
	}
	write(reqCtx, result, err)
}

func (c *AdminHandler) CreateUser(ctx context.Context, reqCtx *app.RequestContext) {
	var command userfacade.AdminUserCreateCommand
	if err := httpx.Bind(reqCtx, &command); err != nil {
		response.Error(reqCtx, err)
		return
	}
	if !c.ensureUserCreateTargetsInScope(ctx, reqCtx, command) {
		return
	}
	if len(command.RoleIDs) > 0 {
		binding := createUserRoleAssignmentBinding(command.Username, command.RoleIDs)
		proof, err := c.ensureProtectedMutation(ctx, reqCtx, string(challengedomain.BusinessActionRBACAssignUserRoles), binding)
		if err != nil {
			response.Error(reqCtx, err)
			return
		}
		command.StepUpProof = proof
	}
	command.OperatorID = currentUserID(reqCtx)
	id, err := c.users.CreateAdminUser(ctx, command)
	write(reqCtx, id > 0, err)
}

func (c *AdminHandler) UpdateUser(ctx context.Context, reqCtx *app.RequestContext) {
	var command userfacade.AdminUserUpdateCommand
	if err := httpx.Bind(reqCtx, &command); err != nil {
		response.Error(reqCtx, err)
		return
	}
	if !c.ensureUserInScope(ctx, reqCtx, command.ID) {
		return
	}
	command.OperatorID = currentUserID(reqCtx)
	write(reqCtx, true, c.users.UpdateAdminUser(ctx, command))
}

func (c *AdminHandler) DeleteUser(ctx context.Context, reqCtx *app.RequestContext) {
	id, err := pathInt64(reqCtx, "id")
	if err != nil {
		response.Error(reqCtx, err)
		return
	}
	if !c.ensureUserInScope(ctx, reqCtx, id) {
		return
	}
	binding := adminDeleteUserBinding(id)
	proof, err := c.ensureProtectedMutation(ctx, reqCtx, string(challengedomain.BusinessActionAdminDeleteUser), binding)
	if err != nil {
		response.Error(reqCtx, err)
		return
	}
	write(reqCtx, true, c.users.DeleteAdminUser(ctx, userfacade.AdminUserDeleteCommand{UserID: id, OperatorID: currentUserID(reqCtx), StepUpProof: proof}))
}

func (c *AdminHandler) ChangeUserStatus(ctx context.Context, reqCtx *app.RequestContext) {
	id, err := pathInt64(reqCtx, "id")
	if err != nil {
		response.Error(reqCtx, err)
		return
	}
	if !c.ensureUserInScope(ctx, reqCtx, id) {
		return
	}
	status := int(queryInt64(reqCtx, "status", -1))
	if status != 0 && status != 1 && status != 2 {
		response.Error(reqCtx, apperrors.Params("用户状态错误"))
		return
	}
	binding := adminChangeUserStatusBinding(id, status)
	proof, err := c.ensureProtectedMutation(ctx, reqCtx, string(challengedomain.BusinessActionAdminChangeUserStatus), binding)
	if err != nil {
		response.Error(reqCtx, err)
		return
	}
	write(reqCtx, true, c.users.UpdateAdminUserStatus(ctx, userfacade.AdminUserStatusCommand{UserID: id, Status: status, OperatorID: currentUserID(reqCtx), StepUpProof: proof}))
}

func (c *AdminHandler) ResetPassword(ctx context.Context, reqCtx *app.RequestContext) {
	id, err := pathInt64(reqCtx, "id")
	if err != nil {
		response.Error(reqCtx, err)
		return
	}
	if !c.ensureUserInScope(ctx, reqCtx, id) {
		return
	}
	var body struct {
		Password string `json:"password"`
	}
	_ = reqCtx.Bind(&body)
	binding := adminResetPasswordBinding(id)
	if _, err := c.ensureProtectedMutation(ctx, reqCtx, string(challengedomain.BusinessActionAdminResetPassword), binding); err != nil {
		response.Error(reqCtx, err)
		return
	}
	write(reqCtx, true, c.users.ResetAdminUserPassword(ctx, userfacade.AdminPasswordResetCommand{UserID: id, RawPassword: body.Password, OperatorID: currentUserID(reqCtx)}))
}

func (c *AdminHandler) ListUserRoles(ctx context.Context, reqCtx *app.RequestContext) {
	c.listRelation(ctx, reqCtx, c.relations.ListUserRoleIDs)
}
func (c *AdminHandler) ListUserOrgs(ctx context.Context, reqCtx *app.RequestContext) {
	c.listRelation(ctx, reqCtx, c.relations.ListUserOrgIDs)
}
func (c *AdminHandler) ListUserDepts(ctx context.Context, reqCtx *app.RequestContext) {
	c.listRelation(ctx, reqCtx, c.relations.ListUserDeptIDs)
}
func (c *AdminHandler) ListUserPosts(ctx context.Context, reqCtx *app.RequestContext) {
	c.listRelation(ctx, reqCtx, c.relations.ListUserPostIDs)
}

func (c *AdminHandler) AssignUserRoles(ctx context.Context, reqCtx *app.RequestContext) {
	id, err := pathInt64(reqCtx, "id")
	if err != nil {
		response.Error(reqCtx, err)
		return
	}
	if !c.ensureUserInScope(ctx, reqCtx, id) {
		return
	}
	ids, primaryID, err := bindIDs(reqCtx)
	if err != nil {
		response.Error(reqCtx, err)
		return
	}
	binding := userRoleAssignmentBinding(id, ids)
	proof, err := c.ensureProtectedMutation(ctx, reqCtx, string(challengedomain.BusinessActionRBACAssignUserRoles), binding)
	if err != nil {
		response.Error(reqCtx, err)
		return
	}
	write(reqCtx, true, c.relations.AssignUserRoles(ctx, userfacade.RelationAssignCommand{UserID: id, IDs: ids, PrimaryID: primaryID, OperatorID: currentUserID(reqCtx), StepUpProof: proof}))
}
func (c *AdminHandler) AssignUserOrgs(ctx context.Context, reqCtx *app.RequestContext) {
	c.assignRelation(ctx, reqCtx, c.relations.AssignUserOrgs)
}
func (c *AdminHandler) AssignUserDepts(ctx context.Context, reqCtx *app.RequestContext) {
	c.assignRelation(ctx, reqCtx, c.relations.AssignUserDepts)
}
func (c *AdminHandler) AssignUserPosts(ctx context.Context, reqCtx *app.RequestContext) {
	c.assignRelation(ctx, reqCtx, c.relations.AssignUserPosts)
}

func (c *AdminHandler) CreateOrg(ctx context.Context, reqCtx *app.RequestContext) {
	var cmd userfacade.OrgCommand
	if err := bindActor(reqCtx, &cmd); err != nil {
		response.Error(reqCtx, err)
		return
	}
	if !c.ensureOrgCreateParentInScope(ctx, reqCtx, cmd.ParentID) {
		return
	}
	write(reqCtx, true, c.orgs.CreateOrg(ctx, cmd))
}
func (c *AdminHandler) UpdateOrg(ctx context.Context, reqCtx *app.RequestContext) {
	var cmd userfacade.OrgCommand
	if err := bindActor(reqCtx, &cmd); err != nil {
		response.Error(reqCtx, err)
		return
	}
	if !c.ensureOrgInScope(ctx, reqCtx, cmd.ID) {
		return
	}
	if cmd.ParentID > 0 && !c.ensureOrgInScope(ctx, reqCtx, cmd.ParentID) {
		return
	}
	write(reqCtx, true, c.orgs.UpdateOrg(ctx, cmd))
}
func (c *AdminHandler) DeleteOrg(ctx context.Context, reqCtx *app.RequestContext) {
	id, err := pathInt64(reqCtx, "id")
	if err != nil {
		response.Error(reqCtx, err)
		return
	}
	if !c.ensureOrgInScope(ctx, reqCtx, id) {
		return
	}
	write(reqCtx, true, c.orgs.DeleteOrg(ctx, id))
}
func (c *AdminHandler) GetOrg(ctx context.Context, reqCtx *app.RequestContext) {
	id, err := pathInt64(reqCtx, "id")
	if err != nil {
		response.Error(reqCtx, err)
		return
	}
	result, err := c.orgs.GetOrgByID(ctx, id)
	if err == nil && result != nil && !dataScopeAllowsOrg(reqCtx, result.ID) {
		err = apperrors.DataScopeDenied("数据范围不足")
		result = nil
	}
	write(reqCtx, result, err)
}
func (c *AdminHandler) GetOrgByCode(ctx context.Context, reqCtx *app.RequestContext) {
	result, err := c.orgs.GetOrgByCode(ctx, strings.TrimSpace(string(reqCtx.Param("code"))))
	if err == nil && result != nil && !dataScopeAllowsOrg(reqCtx, result.ID) {
		err = apperrors.DataScopeDenied("数据范围不足")
		result = nil
	}
	write(reqCtx, result, err)
}
func (c *AdminHandler) GetOrgByUserID(ctx context.Context, reqCtx *app.RequestContext) {
	id, err := pathInt64(reqCtx, "userId")
	if err != nil {
		response.Error(reqCtx, err)
		return
	}
	if _, err := c.users.GetAdminUser(ctx, id); err != nil {
		response.Error(reqCtx, err)
		return
	}
	if !dataScopeAllowsUser(ctx, c.relations, reqCtx, id) {
		response.Error(reqCtx, apperrors.DataScopeDenied("数据范围不足"))
		return
	}
	result, err := c.orgs.GetOrgByUserID(ctx, id)
	if err == nil && result != nil && !dataScopeAllowsOrg(reqCtx, result.ID) {
		err = apperrors.DataScopeDenied("数据范围不足")
		result = nil
	}
	write(reqCtx, result, err)
}
func (c *AdminHandler) OrgTree(ctx context.Context, reqCtx *app.RequestContext) {
	result, err := c.orgs.GetOrgTree(ctx)
	if err == nil {
		result = filterOrgTree(reqCtx, result)
	}
	write(reqCtx, result, err)
}
func (c *AdminHandler) OrgChildren(ctx context.Context, reqCtx *app.RequestContext) {
	id, err := pathInt64(reqCtx, "parentId")
	if err != nil {
		response.Error(reqCtx, err)
		return
	}
	result, err := c.orgs.ListOrgChildren(ctx, id)
	if err == nil {
		result = filterOrgList(reqCtx, result)
	}
	write(reqCtx, result, err)
}
func (c *AdminHandler) ActiveOrgs(ctx context.Context, reqCtx *app.RequestContext) {
	result, err := c.orgs.ListActiveOrgs(ctx)
	if err == nil {
		result = filterOrgList(reqCtx, result)
	}
	write(reqCtx, result, err)
}
func (c *AdminHandler) CheckOrgCode(ctx context.Context, reqCtx *app.RequestContext) {
	ok, err := c.orgs.CheckOrgCode(ctx, queryString(reqCtx, "code"), queryInt64(reqCtx, "excludeId", 0))
	write(reqCtx, !ok, err)
}
func (c *AdminHandler) ChangeOrgStatus(ctx context.Context, reqCtx *app.RequestContext) {
	id := queryInt64(reqCtx, "id", 0)
	if !c.ensureOrgInScope(ctx, reqCtx, id) {
		return
	}
	write(reqCtx, true, c.orgs.ChangeOrgStatus(ctx, id, int(queryInt64(reqCtx, "status", -1)), currentUserID(reqCtx)))
}
func (c *AdminHandler) MoveOrg(ctx context.Context, reqCtx *app.RequestContext) {
	id := queryInt64(reqCtx, "id", 0)
	if !c.ensureOrgInScope(ctx, reqCtx, id) {
		return
	}
	newParentID := queryInt64(reqCtx, "newParentId", 0)
	if newParentID > 0 && !c.ensureOrgInScope(ctx, reqCtx, newParentID) {
		return
	}
	write(reqCtx, true, c.orgs.MoveOrg(ctx, id, newParentID, currentUserID(reqCtx)))
}

func (c *AdminHandler) DeptTree(ctx context.Context, reqCtx *app.RequestContext) {
	result, err := c.depts.GetDeptTree(ctx, false)
	if err == nil {
		result = filterDeptTree(reqCtx, result)
	}
	write(reqCtx, result, err)
}
func (c *AdminHandler) EnabledDeptTree(ctx context.Context, reqCtx *app.RequestContext) {
	result, err := c.depts.GetDeptTree(ctx, true)
	if err == nil {
		result = filterDeptTree(reqCtx, result)
	}
	write(reqCtx, result, err)
}
func (c *AdminHandler) DeptOptions(ctx context.Context, reqCtx *app.RequestContext) {
	result, err := c.depts.SearchDepts(ctx, queryString(reqCtx, "keyword"), queryInt64(reqCtx, "orgId", 0), queryIntPtr(reqCtx, "status"), int(queryInt64(reqCtx, "limit", 50)))
	if err == nil {
		result = filterDeptList(reqCtx, result)
	}
	write(reqCtx, result, err)
}
func (c *AdminHandler) GetDept(ctx context.Context, reqCtx *app.RequestContext) {
	id, err := pathInt64(reqCtx, "id")
	if err != nil {
		response.Error(reqCtx, err)
		return
	}
	result, err := c.depts.GetDeptByID(ctx, id)
	if err == nil && result != nil && !dataScopeAllowsDept(reqCtx, result.ID, result.OrgID) {
		err = apperrors.DataScopeDenied("数据范围不足")
		result = nil
	}
	write(reqCtx, result, err)
}
func (c *AdminHandler) CreateDept(ctx context.Context, reqCtx *app.RequestContext) {
	var cmd userfacade.DeptCommand
	if err := bindActor(reqCtx, &cmd); err != nil {
		response.Error(reqCtx, err)
		return
	}
	if !c.ensureDeptTargetInScope(ctx, reqCtx, cmd.OrgID, cmd.ParentID) {
		return
	}
	write(reqCtx, true, c.depts.CreateDept(ctx, cmd))
}
func (c *AdminHandler) UpdateDept(ctx context.Context, reqCtx *app.RequestContext) {
	var cmd userfacade.DeptCommand
	if err := bindActor(reqCtx, &cmd); err != nil {
		response.Error(reqCtx, err)
		return
	}
	if !c.ensureDeptInScope(ctx, reqCtx, cmd.ID) {
		return
	}
	if !c.ensureDeptTargetInScope(ctx, reqCtx, cmd.OrgID, cmd.ParentID) {
		return
	}
	write(reqCtx, true, c.depts.UpdateDept(ctx, cmd))
}
func (c *AdminHandler) DeleteDept(ctx context.Context, reqCtx *app.RequestContext) {
	id, err := pathInt64(reqCtx, "id")
	if err != nil {
		response.Error(reqCtx, err)
		return
	}
	if !c.ensureDeptInScope(ctx, reqCtx, id) {
		return
	}
	write(reqCtx, true, c.depts.DeleteDept(ctx, id))
}
func (c *AdminHandler) ChildDeptIDs(ctx context.Context, reqCtx *app.RequestContext) {
	id, err := pathInt64(reqCtx, "deptId")
	if err != nil {
		response.Error(reqCtx, err)
		return
	}
	result, err := c.depts.GetChildDeptIDs(ctx, id)
	write(reqCtx, result, err)
}

func (c *AdminHandler) QueryPosts(ctx context.Context, reqCtx *app.RequestContext) {
	result, err := c.posts.QueryPosts(ctx, userfacade.PostQuery{Current: queryInt64(reqCtx, "current", 1), Size: queryInt64(reqCtx, "size", 10), Name: queryString(reqCtx, "name"), Code: queryString(reqCtx, "code"), Status: queryIntPtr(reqCtx, "status"), Scope: dataScopeFilter(reqCtx)})
	write(reqCtx, result, err)
}
func (c *AdminHandler) ListPosts(ctx context.Context, reqCtx *app.RequestContext) {
	result, err := c.posts.ListEnabledPosts(ctx)
	if err == nil {
		result = filterPostList(reqCtx, result)
	}
	write(reqCtx, result, err)
}
func (c *AdminHandler) GetPost(ctx context.Context, reqCtx *app.RequestContext) {
	id, err := pathInt64(reqCtx, "id")
	if err != nil {
		response.Error(reqCtx, err)
		return
	}
	result, err := c.posts.GetPostByID(ctx, id)
	if err == nil && result != nil && !dataScopeAllowsDept(reqCtx, result.DeptID, result.OrgID) {
		err = apperrors.DataScopeDenied("数据范围不足")
		result = nil
	}
	write(reqCtx, result, err)
}
func (c *AdminHandler) CreatePost(ctx context.Context, reqCtx *app.RequestContext) {
	var cmd userfacade.PostCommand
	if err := bindActor(reqCtx, &cmd); err != nil {
		response.Error(reqCtx, err)
		return
	}
	if !c.ensurePostTargetInScope(ctx, reqCtx, cmd.OrgID, cmd.DeptID) {
		return
	}
	write(reqCtx, true, c.posts.CreatePost(ctx, cmd))
}
func (c *AdminHandler) UpdatePost(ctx context.Context, reqCtx *app.RequestContext) {
	var cmd userfacade.PostCommand
	if err := bindActor(reqCtx, &cmd); err != nil {
		response.Error(reqCtx, err)
		return
	}
	if !c.ensurePostInScope(ctx, reqCtx, cmd.ID) {
		return
	}
	if !c.ensurePostTargetInScope(ctx, reqCtx, cmd.OrgID, cmd.DeptID) {
		return
	}
	write(reqCtx, true, c.posts.UpdatePost(ctx, cmd))
}
func (c *AdminHandler) DeletePost(ctx context.Context, reqCtx *app.RequestContext) {
	id, err := pathInt64(reqCtx, "id")
	if err != nil {
		response.Error(reqCtx, err)
		return
	}
	if !c.ensurePostInScope(ctx, reqCtx, id) {
		return
	}
	write(reqCtx, true, c.posts.DeletePost(ctx, id))
}
func (c *AdminHandler) BatchDeletePosts(ctx context.Context, reqCtx *app.RequestContext) {
	ids, _, err := bindIDs(reqCtx)
	if err != nil {
		response.Error(reqCtx, err)
		return
	}
	if len(ids) > 100 {
		response.Error(reqCtx, apperrors.Params("岗位数量超过单次批量上限"))
		return
	}
	if !c.ensurePostsInScope(ctx, reqCtx, ids) {
		return
	}
	write(reqCtx, true, c.posts.BatchDeletePosts(ctx, ids))
}
func (c *AdminHandler) ChangePostStatus(ctx context.Context, reqCtx *app.RequestContext) {
	id, err := pathInt64(reqCtx, "id")
	if err != nil {
		response.Error(reqCtx, err)
		return
	}
	if !c.ensurePostInScope(ctx, reqCtx, id) {
		return
	}
	write(reqCtx, true, c.posts.ChangePostStatus(ctx, id, int(queryInt64(reqCtx, "status", -1)), currentUserID(reqCtx)))
}
func (c *AdminHandler) GetPostRoles(ctx context.Context, reqCtx *app.RequestContext) {
	id, err := pathInt64(reqCtx, "postId")
	if err != nil {
		response.Error(reqCtx, err)
		return
	}
	if !c.ensurePostInScope(ctx, reqCtx, id) {
		return
	}
	result, err := c.posts.ListPostRoleIDs(ctx, id)
	write(reqCtx, result, err)
}
func (c *AdminHandler) AssignPostRoles(ctx context.Context, reqCtx *app.RequestContext) {
	id, err := pathInt64(reqCtx, "postId")
	if err != nil {
		response.Error(reqCtx, err)
		return
	}
	if !c.ensurePostInScope(ctx, reqCtx, id) {
		return
	}
	ids, _, err := bindIDs(reqCtx)
	if err != nil {
		response.Error(reqCtx, err)
		return
	}
	if len(ids) > 100 {
		response.Error(reqCtx, apperrors.Params("角色数量超过单次批量上限"))
		return
	}
	binding := postRoleAssignmentBinding(id, ids)
	proof, err := c.ensureProtectedMutation(ctx, reqCtx, string(challengedomain.BusinessActionRBACAssignPostRoles), binding)
	if err != nil {
		response.Error(reqCtx, err)
		return
	}
	write(reqCtx, true, c.posts.AssignPostRoles(ctx, userfacade.PostRoleAssignCommand{PostID: id, RoleIDs: ids, OperatorID: currentUserID(reqCtx), StepUpProof: proof}))
}
func (c *AdminHandler) RemovePostRoles(ctx context.Context, reqCtx *app.RequestContext) {
	id, err := pathInt64(reqCtx, "postId")
	if err != nil {
		response.Error(reqCtx, err)
		return
	}
	if !c.ensurePostInScope(ctx, reqCtx, id) {
		return
	}
	binding := postRoleAssignmentBinding(id, nil)
	proof, err := c.ensureProtectedMutation(ctx, reqCtx, string(challengedomain.BusinessActionRBACAssignPostRoles), binding)
	if err != nil {
		response.Error(reqCtx, err)
		return
	}
	write(reqCtx, true, c.posts.AssignPostRoles(ctx, userfacade.PostRoleAssignCommand{PostID: id, RoleIDs: []int64{}, OperatorID: currentUserID(reqCtx), StepUpProof: proof}))
}
func (c *AdminHandler) GetPostsByRole(ctx context.Context, reqCtx *app.RequestContext) {
	id, err := pathInt64(reqCtx, "roleId")
	if err != nil {
		response.Error(reqCtx, err)
		return
	}
	result, err := c.posts.ListPostIDsByRoleID(ctx, id)
	write(reqCtx, result, err)
}

func (c *AdminHandler) listRelation(ctx context.Context, reqCtx *app.RequestContext, fn func(context.Context, int64) ([]int64, error)) {
	id, err := pathInt64(reqCtx, "id")
	if err != nil {
		response.Error(reqCtx, err)
		return
	}
	if _, err := c.users.GetAdminUser(ctx, id); err != nil {
		response.Error(reqCtx, err)
		return
	}
	if !dataScopeAllowsUser(ctx, c.relations, reqCtx, id) {
		response.Error(reqCtx, apperrors.DataScopeDenied("数据范围不足"))
		return
	}
	result, err := fn(ctx, id)
	write(reqCtx, result, err)
}

func (c *AdminHandler) assignRelation(ctx context.Context, reqCtx *app.RequestContext, fn func(context.Context, userfacade.RelationAssignCommand) error) {
	id, err := pathInt64(reqCtx, "id")
	if err != nil {
		response.Error(reqCtx, err)
		return
	}
	if !c.ensureUserInScope(ctx, reqCtx, id) {
		return
	}
	ids, primaryID, err := bindIDs(reqCtx)
	if err != nil {
		response.Error(reqCtx, err)
		return
	}
	write(reqCtx, true, fn(ctx, userfacade.RelationAssignCommand{UserID: id, IDs: ids, PrimaryID: primaryID, OperatorID: currentUserID(reqCtx)}))
}

func (c *AdminHandler) ensureProtectedMutation(ctx context.Context, reqCtx *app.RequestContext, businessAction, operationBinding string) (stepup.ProofMetadata, error) {
	if c.auth == nil {
		return stepup.ProofMetadata{}, apperrors.System("authorization auth facade未配置")
	}
	scope, err := buildRequestScope(reqCtx)
	if err != nil {
		return stepup.ProofMetadata{}, err
	}
	proofToken := strings.TrimSpace(string(reqCtx.Request.Header.Peek("Proof-Token")))
	flowNonce := chooseFlowNonce(strings.TrimSpace(string(reqCtx.Request.Header.Peek("Flow-Nonce"))), businessAction)
	if proofToken != "" {
		token, err := c.auth.VerifyStepUp(ctx, scope, authorizationfacade.StepUpVerifyRequest{
			ProofToken:       proofToken,
			BusinessAction:   businessAction,
			FlowNonce:        flowNonce,
			OperationBinding: operationBinding,
			ConsumeOnce:      true,
		})
		if err != nil {
			return stepup.ProofMetadata{}, err
		}
		if token == nil {
			return stepup.ProofMetadata{}, apperrors.Forbidden("step-up proof验证失败")
		}
		securitycontext.SetStepUpProofAudit(reqCtx, stepUpProofAuditFromToken(token, businessAction, operationBinding))
		return stepUpProofMetadataFromToken(token, businessAction, operationBinding), nil
	}
	challenge, err := c.auth.CreateStepUpChallenge(ctx, scope, authorizationfacade.StepUpChallengeRequest{
		BusinessAction:   businessAction,
		FlowNonce:        flowNonce,
		OperationBinding: operationBinding,
	})
	if err != nil {
		return stepup.ProofMetadata{}, err
	}
	return stepup.ProofMetadata{}, apperrors.ChallengeRequired("", map[string]any{
		"challengeIdentifier":        challenge.ChallengeIdentifier,
		"challengeState":             challenge.ChallengeState,
		"effectiveTimeToLiveSeconds": challenge.EffectiveTimeToLiveSeconds,
		"requiredAssuranceLevel":     challenge.RequiredAssuranceLevel,
		"resolvedAssuranceLevel":     challenge.ResolvedAssuranceLevel,
		"recommendedStepIdentifier":  challenge.RecommendedStepIdentifier,
		"actualChallengeTypeNames":   challenge.ActualChallengeTypeNames,
		"flowNonce":                  flowNonce,
		"steps":                      challenge.Steps,
		"operationBinding":           operationBinding,
	})
}

func stepUpProofAuditFromToken(token *authorizationfacade.StepUpTokenVO, businessAction, operationBinding string) securitycontext.StepUpProofAudit {
	if token == nil {
		return securitycontext.StepUpProofAudit{}
	}
	return securitycontext.StepUpProofAudit{
		BusinessAction:        firstNonBlank(token.BusinessAction, businessAction),
		OperationBinding:      firstNonBlank(token.OperationBinding, operationBinding),
		ProofIdentifier:       token.TokenUniqueIdentifier,
		ChallengeIdentifier:   token.ChallengeID,
		AssuranceLevel:        "AAL2",
		AuthenticationMethods: append([]string(nil), token.AuthenticationMethodNames...),
	}
}

func stepUpProofMetadataFromToken(token *authorizationfacade.StepUpTokenVO, businessAction, operationBinding string) stepup.ProofMetadata {
	if token == nil {
		return stepup.ProofMetadata{}
	}
	return stepup.ProofMetadata{
		BusinessAction:        firstNonBlank(token.BusinessAction, businessAction),
		OperationBinding:      firstNonBlank(token.OperationBinding, operationBinding),
		ProofIdentifier:       token.TokenUniqueIdentifier,
		ChallengeIdentifier:   token.ChallengeID,
		AssuranceLevel:        "AAL2",
		AuthenticationMethods: append([]string(nil), token.AuthenticationMethodNames...),
	}
}

func firstNonBlank(values ...string) string {
	for _, item := range values {
		if value := strings.TrimSpace(item); value != "" {
			return value
		}
	}
	return ""
}

func userRoleAssignmentBinding(userID int64, roleIDs []int64) string {
	return fmt.Sprintf("user:%d|roles:%s", userID, joinSortedRoleIDs(roleIDs))
}

func createUserRoleAssignmentBinding(username string, roleIDs []int64) string {
	return fmt.Sprintf("user:create:%s|roles:%s", strings.TrimSpace(username), joinSortedRoleIDs(roleIDs))
}

func postRoleAssignmentBinding(postID int64, roleIDs []int64) string {
	return fmt.Sprintf("post:%d|roles:%s", postID, joinSortedRoleIDs(roleIDs))
}

func adminResetPasswordBinding(userID int64) string {
	return fmt.Sprintf("user:%d|reset-password", userID)
}

func adminDeleteUserBinding(userID int64) string {
	return fmt.Sprintf("user:%d|delete", userID)
}

func adminChangeUserStatusBinding(userID int64, status int) string {
	return fmt.Sprintf("user:%d|status:%d", userID, status)
}

func joinSortedRoleIDs(ids []int64) string {
	normalized := make([]int64, 0, len(ids))
	seen := make(map[int64]struct{}, len(ids))
	for _, id := range ids {
		if id <= 0 {
			continue
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		normalized = append(normalized, id)
	}
	sort.Slice(normalized, func(i, j int) bool { return normalized[i] < normalized[j] })
	parts := make([]string, 0, len(normalized))
	for _, id := range normalized {
		parts = append(parts, strconv.FormatInt(id, 10))
	}
	return strings.Join(parts, ",")
}

func (c *AdminHandler) ensureUserInScope(ctx context.Context, reqCtx *app.RequestContext, id int64) bool {
	if _, err := c.users.GetAdminUser(ctx, id); err != nil {
		response.Error(reqCtx, err)
		return false
	}
	if !dataScopeAllowsUser(ctx, c.relations, reqCtx, id) {
		response.Error(reqCtx, apperrors.DataScopeDenied("数据范围不足"))
		return false
	}
	return true
}

func (c *AdminHandler) ensureOrgInScope(ctx context.Context, reqCtx *app.RequestContext, id int64) bool {
	result, err := c.orgs.GetOrgByID(ctx, id)
	if err != nil {
		response.Error(reqCtx, err)
		return false
	}
	if result == nil {
		response.Error(reqCtx, apperrors.NotFound("组织不存在"))
		return false
	}
	if !dataScopeAllowsOrg(reqCtx, result.ID) {
		response.Error(reqCtx, apperrors.DataScopeDenied("数据范围不足"))
		return false
	}
	return true
}

func (c *AdminHandler) ensureDeptInScope(ctx context.Context, reqCtx *app.RequestContext, id int64) bool {
	result, err := c.depts.GetDeptByID(ctx, id)
	if err != nil {
		response.Error(reqCtx, err)
		return false
	}
	if result == nil {
		response.Error(reqCtx, apperrors.NotFound("部门不存在"))
		return false
	}
	if !dataScopeAllowsDept(reqCtx, result.ID, result.OrgID) {
		response.Error(reqCtx, apperrors.DataScopeDenied("数据范围不足"))
		return false
	}
	return true
}

func (c *AdminHandler) ensurePostInScope(ctx context.Context, reqCtx *app.RequestContext, id int64) bool {
	result, err := c.posts.GetPostByID(ctx, id)
	if err != nil {
		response.Error(reqCtx, err)
		return false
	}
	if result == nil {
		response.Error(reqCtx, apperrors.NotFound("岗位不存在"))
		return false
	}
	if !dataScopeAllowsDept(reqCtx, result.DeptID, result.OrgID) {
		response.Error(reqCtx, apperrors.DataScopeDenied("数据范围不足"))
		return false
	}
	return true
}

func (c *AdminHandler) ensurePostsInScope(ctx context.Context, reqCtx *app.RequestContext, ids []int64) bool {
	results, err := c.posts.ListPostsByIDs(ctx, ids)
	if err != nil {
		response.Error(reqCtx, err)
		return false
	}
	expected := make(map[int64]struct{}, len(ids))
	for _, id := range ids {
		if id > 0 {
			expected[id] = struct{}{}
		}
	}
	if len(results) != len(expected) {
		response.Error(reqCtx, apperrors.NotFound("岗位不存在"))
		return false
	}
	for _, result := range results {
		if !dataScopeAllowsDept(reqCtx, result.DeptID, result.OrgID) {
			response.Error(reqCtx, apperrors.DataScopeDenied("数据范围不足"))
			return false
		}
	}
	return true
}

func (c *AdminHandler) ensureUserCreateTargetsInScope(ctx context.Context, reqCtx *app.RequestContext, command userfacade.AdminUserCreateCommand) bool {
	for _, orgID := range command.OrgIDs {
		if !c.ensureOrgInScope(ctx, reqCtx, orgID) {
			return false
		}
	}
	for _, deptID := range command.DeptIDs {
		if !c.ensureDeptInScope(ctx, reqCtx, deptID) {
			return false
		}
	}
	for _, postID := range command.PostIDs {
		if !c.ensurePostInScope(ctx, reqCtx, postID) {
			return false
		}
	}
	return true
}

func (c *AdminHandler) ensureOrgCreateParentInScope(ctx context.Context, reqCtx *app.RequestContext, parentID int64) bool {
	scope := dataScopeFilter(reqCtx)
	if !scope.Enabled {
		return true
	}
	if parentID <= 0 {
		response.Error(reqCtx, apperrors.DataScopeDenied("数据范围不足"))
		return false
	}
	return c.ensureOrgInScope(ctx, reqCtx, parentID)
}

func (c *AdminHandler) ensureDeptTargetInScope(ctx context.Context, reqCtx *app.RequestContext, orgID, parentID int64) bool {
	if orgID > 0 && !c.ensureOrgInScope(ctx, reqCtx, orgID) {
		return false
	}
	if parentID > 0 && !c.ensureDeptInScope(ctx, reqCtx, parentID) {
		return false
	}
	return true
}

func (c *AdminHandler) ensurePostTargetInScope(ctx context.Context, reqCtx *app.RequestContext, orgID, deptID int64) bool {
	if orgID > 0 && !c.ensureOrgInScope(ctx, reqCtx, orgID) {
		return false
	}
	if deptID > 0 && !c.ensureDeptInScope(ctx, reqCtx, deptID) {
		return false
	}
	return true
}

func bindIDs(reqCtx *app.RequestContext) ([]int64, int64, error) {
	var ids []int64
	if err := reqCtx.Bind(&ids); err == nil && ids != nil {
		return ids, 0, nil
	}
	var body struct {
		IDs       []int64 `json:"ids"`
		RoleIDs   []int64 `json:"roleIds"`
		OrgIDs    []int64 `json:"orgIds"`
		DeptIDs   []int64 `json:"deptIds"`
		PostIDs   []int64 `json:"postIds"`
		PrimaryID int64   `json:"primaryId"`
	}
	if err := reqCtx.Bind(&body); err != nil {
		return nil, 0, apperrors.Params("参数错误")
	}
	switch {
	case body.IDs != nil:
		return body.IDs, body.PrimaryID, nil
	case body.RoleIDs != nil:
		return body.RoleIDs, body.PrimaryID, nil
	case body.OrgIDs != nil:
		return body.OrgIDs, body.PrimaryID, nil
	case body.DeptIDs != nil:
		return body.DeptIDs, body.PrimaryID, nil
	case body.PostIDs != nil:
		return body.PostIDs, body.PrimaryID, nil
	}
	return body.IDs, body.PrimaryID, nil
}

func bindActor[T interface {
	*userfacade.OrgCommand | *userfacade.DeptCommand | *userfacade.PostCommand
}](reqCtx *app.RequestContext, command T) error {
	if err := httpx.Bind(reqCtx, command); err != nil {
		return err
	}
	switch value := any(command).(type) {
	case *userfacade.OrgCommand:
		value.OperatorID = currentUserID(reqCtx)
	case *userfacade.DeptCommand:
		value.OperatorID = currentUserID(reqCtx)
	case *userfacade.PostCommand:
		value.OperatorID = currentUserID(reqCtx)
	}
	return nil
}

func write(reqCtx *app.RequestContext, data any, err error) {
	if err != nil {
		response.Error(reqCtx, err)
		return
	}
	response.Success(reqCtx, data)
}

func currentUserID(reqCtx *app.RequestContext) int64 {
	userID, _ := securitycontext.CurrentUserID(reqCtx)
	return userID
}

func dataScopeFilter(reqCtx *app.RequestContext) userfacade.DataScopeFilter {
	user := securitycontext.Require(reqCtx)
	if user.IsAdmin || user.DataScopeType == securitycontext.DataScopeAll {
		return userfacade.DataScopeFilter{}
	}
	filter := userfacade.DataScopeFilter{
		Enabled:   true,
		ScopeType: string(user.DataScopeType),
		DeptIDs:   append([]int64{}, user.DataScopeDeptIDs...),
		OrgIDs:    append([]int64{}, user.DataScopeOrgIDs...),
	}
	switch user.DataScopeType {
	case securitycontext.DataScopeCustom, securitycontext.DataScopeDept, securitycontext.DataScopeDeptAndChild:
		return filter
	case securitycontext.DataScopeSelf:
		filter.SelfUserID = user.UserID
		if len(filter.DeptIDs) == 0 {
			filter.DeptIDs = append([]int64{}, user.DeptIDs...)
		}
		if len(filter.OrgIDs) == 0 {
			filter.OrgIDs = append([]int64{}, user.OrgIDs...)
		}
		return filter
	default:
		filter.None = true
		return filter
	}
}

func dataScopeAllowsUser(ctx context.Context, relations userfacade.UserRelationFacade, reqCtx *app.RequestContext, userID int64) bool {
	scope := dataScopeFilter(reqCtx)
	if !scope.Enabled {
		return true
	}
	if scope.None || userID <= 0 {
		return false
	}
	if scope.SelfUserID > 0 {
		return userID == scope.SelfUserID
	}
	if relations == nil {
		return false
	}
	if len(scope.DeptIDs) > 0 {
		deptIDs, err := relations.ListUserDeptIDs(ctx, userID)
		if err != nil {
			return false
		}
		if intersectsInt64(scope.DeptIDs, deptIDs) {
			return true
		}
	}
	return false
}

func dataScopeAllowsOrg(reqCtx *app.RequestContext, orgID int64) bool {
	scope := dataScopeFilter(reqCtx)
	if !scope.Enabled {
		return true
	}
	if scope.None || orgID <= 0 {
		return false
	}
	return containsInt64(scope.OrgIDs, orgID)
}

func dataScopeAllowsDept(reqCtx *app.RequestContext, deptID int64, orgID int64) bool {
	scope := dataScopeFilter(reqCtx)
	if !scope.Enabled {
		return true
	}
	if scope.None {
		return false
	}
	if deptID > 0 && containsInt64(scope.DeptIDs, deptID) {
		return true
	}
	return false
}

func filterOrgTree(reqCtx *app.RequestContext, items []userfacade.OrgVO) []userfacade.OrgVO {
	result := make([]userfacade.OrgVO, 0, len(items))
	for _, item := range items {
		item.Children = filterOrgTree(reqCtx, item.Children)
		if dataScopeAllowsOrg(reqCtx, item.ID) {
			result = append(result, item)
			continue
		}
		result = append(result, item.Children...)
	}
	return result
}

func filterOrgList(reqCtx *app.RequestContext, items []userfacade.OrgVO) []userfacade.OrgVO {
	result := make([]userfacade.OrgVO, 0, len(items))
	for _, item := range items {
		if dataScopeAllowsOrg(reqCtx, item.ID) {
			result = append(result, item)
		}
	}
	return result
}

func filterDeptTree(reqCtx *app.RequestContext, items []userfacade.DeptVO) []userfacade.DeptVO {
	result := make([]userfacade.DeptVO, 0, len(items))
	for _, item := range items {
		item.Children = filterDeptTree(reqCtx, item.Children)
		if dataScopeAllowsDept(reqCtx, item.ID, item.OrgID) {
			result = append(result, item)
			continue
		}
		result = append(result, item.Children...)
	}
	return result
}

func filterDeptList(reqCtx *app.RequestContext, items []userfacade.DeptVO) []userfacade.DeptVO {
	result := make([]userfacade.DeptVO, 0, len(items))
	for _, item := range items {
		if dataScopeAllowsDept(reqCtx, item.ID, item.OrgID) {
			result = append(result, item)
		}
	}
	return result
}

func filterPostList(reqCtx *app.RequestContext, items []userfacade.PostVO) []userfacade.PostVO {
	result := make([]userfacade.PostVO, 0, len(items))
	for _, item := range items {
		if dataScopeAllowsDept(reqCtx, item.DeptID, item.OrgID) {
			result = append(result, item)
		}
	}
	return result
}

func containsInt64(values []int64, target int64) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}

func intersectsInt64(left []int64, right []int64) bool {
	for _, value := range right {
		if containsInt64(left, value) {
			return true
		}
	}
	return false
}

func pathInt64(reqCtx *app.RequestContext, key string) (int64, error) {
	value, err := strconv.ParseInt(strings.TrimSpace(string(reqCtx.Param(key))), 10, 64)
	if err != nil || value <= 0 {
		return 0, apperrors.Params("路径参数错误")
	}
	return value, nil
}

func queryString(reqCtx *app.RequestContext, key string) string {
	return strings.TrimSpace(string(reqCtx.QueryArgs().Peek(key)))
}

func queryInt64(reqCtx *app.RequestContext, key string, fallback int64) int64 {
	raw := queryString(reqCtx, key)
	if raw == "" {
		return fallback
	}
	value, err := strconv.ParseInt(raw, 10, 64)
	if err != nil {
		return fallback
	}
	return value
}

func queryIntPtr(reqCtx *app.RequestContext, key string) *int {
	raw := queryString(reqCtx, key)
	if raw == "" {
		return nil
	}
	value, err := strconv.Atoi(raw)
	if err != nil {
		return nil
	}
	return &value
}
