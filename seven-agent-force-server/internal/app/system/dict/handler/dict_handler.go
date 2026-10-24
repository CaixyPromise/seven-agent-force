package handler

import (
	"context"
	"strconv"
	"strings"

	"github.com/CaixyPromise/seven-agent/force/internal/app/system/dict/application"
	dictfacade "github.com/CaixyPromise/seven-agent/force/internal/app/system/dict/facade"
	apperrors "github.com/CaixyPromise/seven-agent/force/internal/common/errors"
	"github.com/CaixyPromise/seven-agent/force/internal/common/httpx"
	"github.com/CaixyPromise/seven-agent/force/internal/common/response"
	"github.com/CaixyPromise/seven-agent/force/internal/shared/securitycontext"
	"github.com/cloudwego/hertz/pkg/app"
)

type ManagementService interface {
	AddDictType(ctx context.Context, actor application.Actor, request dictfacade.DictTypeAddRequest) (int64, error)
	UpdateDictType(ctx context.Context, actor application.Actor, request dictfacade.DictTypeUpdateRequest) error
	DeleteDictType(ctx context.Context, actor application.Actor, id int64, force bool) error
	GetDictTypeByID(ctx context.Context, id int64) (*dictfacade.DictTypeVO, error)
	GetDictTypePage(ctx context.Context, request dictfacade.DictTypeQueryRequest) (*dictfacade.PageResult[dictfacade.DictTypeVO], error)
	ChangeDictTypeStatus(ctx context.Context, actor application.Actor, id int64, status int) error
	MoveDictType(ctx context.Context, actor application.Actor, id int64, beforeID, afterID *int64) error

	AddDictItem(ctx context.Context, actor application.Actor, typeID int64, request dictfacade.DictItemAddRequest) (int64, error)
	UpdateDictItem(ctx context.Context, actor application.Actor, request dictfacade.DictItemUpdateRequest) error
	DeleteDictItem(ctx context.Context, actor application.Actor, id int64) error
	ChangeDictItemStatus(ctx context.Context, actor application.Actor, id int64, status int) error
	GetDictItemList(ctx context.Context, request dictfacade.DictItemQueryRequest) ([]dictfacade.DictItemVO, error)
	BatchUpdateSort(ctx context.Context, actor application.Actor, typeID int64, request dictfacade.DictItemSortRequest) (int, error)
	MoveDictItem(ctx context.Context, actor application.Actor, typeID, itemID int64, beforeID, afterID *int64) error
	BatchGetDict(ctx context.Context, request dictfacade.DictBatchRequest) (*dictfacade.DictBatchResponse, error)
}

type ClientService interface {
	GetDictByCodeForClient(ctx context.Context, actor application.Actor, dictCode string) (*dictfacade.DictBatchResponse, error)
	BatchGetDictForClient(ctx context.Context, actor application.Actor, request dictfacade.DictBatchRequest) (*dictfacade.DictBatchResponse, error)
}

type Handler struct {
	management ManagementService
	client     ClientService
}

func NewHandler(management ManagementService, client ClientService) *Handler {
	return &Handler{management: management, client: client}
}

func (c *Handler) AddDictType(ctx context.Context, reqCtx *app.RequestContext) {
	var request dictfacade.DictTypeAddRequest
	if err := httpx.BindAndValidate(reqCtx, &request); err != nil {
		response.Error(reqCtx, err)
		return
	}
	id, err := c.management.AddDictType(ctx, currentActor(reqCtx), request)
	if err != nil {
		response.Error(reqCtx, err)
		return
	}
	response.Success(reqCtx, id)
}

func (c *Handler) UpdateDictType(ctx context.Context, reqCtx *app.RequestContext) {
	var request dictfacade.DictTypeUpdateRequest
	if err := httpx.BindAndValidate(reqCtx, &request); err != nil {
		response.Error(reqCtx, err)
		return
	}
	if err := c.management.UpdateDictType(ctx, currentActor(reqCtx), request); err != nil {
		response.Error(reqCtx, err)
		return
	}
	response.Success(reqCtx, true)
}

func (c *Handler) DeleteDictType(ctx context.Context, reqCtx *app.RequestContext) {
	id, err := parseQueryInt64(reqCtx, "id")
	if err != nil {
		response.Error(reqCtx, err)
		return
	}
	force := parseQueryBool(reqCtx, "force")
	if err := c.management.DeleteDictType(ctx, currentActor(reqCtx), id, force); err != nil {
		response.Error(reqCtx, err)
		return
	}
	response.Success(reqCtx, true)
}

func (c *Handler) GetDictTypeByID(ctx context.Context, reqCtx *app.RequestContext) {
	id, err := parsePathInt64(reqCtx, "id")
	if err != nil {
		response.Error(reqCtx, err)
		return
	}
	item, err := c.management.GetDictTypeByID(ctx, id)
	if err != nil {
		response.Error(reqCtx, err)
		return
	}
	response.Success(reqCtx, item)
}

func (c *Handler) GetDictTypePage(ctx context.Context, reqCtx *app.RequestContext) {
	var request dictfacade.DictTypeQueryRequest
	if err := httpx.Bind(reqCtx, &request); err != nil {
		response.Error(reqCtx, err)
		return
	}
	page, err := c.management.GetDictTypePage(ctx, request)
	if err != nil {
		response.Error(reqCtx, err)
		return
	}
	response.Success(reqCtx, page)
}

func (c *Handler) ChangeDictTypeStatus(ctx context.Context, reqCtx *app.RequestContext) {
	id, err := parseQueryInt64(reqCtx, "id")
	if err != nil {
		response.Error(reqCtx, err)
		return
	}
	status, err := parseQueryInt(reqCtx, "status")
	if err != nil {
		response.Error(reqCtx, err)
		return
	}
	if err := c.management.ChangeDictTypeStatus(ctx, currentActor(reqCtx), id, status); err != nil {
		response.Error(reqCtx, err)
		return
	}
	response.Success(reqCtx, true)
}

func (c *Handler) MoveDictType(ctx context.Context, reqCtx *app.RequestContext) {
	id, err := parsePathInt64(reqCtx, "id")
	if err != nil {
		response.Error(reqCtx, err)
		return
	}
	var request dictfacade.MoveRequest
	if err := httpx.Bind(reqCtx, &request); err != nil {
		response.Error(reqCtx, err)
		return
	}
	if err := c.management.MoveDictType(ctx, currentActor(reqCtx), id, request.BeforeID, request.AfterID); err != nil {
		response.Error(reqCtx, err)
		return
	}
	response.Success(reqCtx, true)
}

func (c *Handler) AddDictItem(ctx context.Context, reqCtx *app.RequestContext) {
	typeID, err := parsePathInt64(reqCtx, "typeId")
	if err != nil {
		response.Error(reqCtx, err)
		return
	}
	var request dictfacade.DictItemAddRequest
	if err := httpx.BindAndValidate(reqCtx, &request); err != nil {
		response.Error(reqCtx, err)
		return
	}
	id, err := c.management.AddDictItem(ctx, currentActor(reqCtx), typeID, request)
	if err != nil {
		response.Error(reqCtx, err)
		return
	}
	response.Success(reqCtx, id)
}

func (c *Handler) UpdateDictItem(ctx context.Context, reqCtx *app.RequestContext) {
	var request dictfacade.DictItemUpdateRequest
	if err := httpx.BindAndValidate(reqCtx, &request); err != nil {
		response.Error(reqCtx, err)
		return
	}
	if err := c.management.UpdateDictItem(ctx, currentActor(reqCtx), request); err != nil {
		response.Error(reqCtx, err)
		return
	}
	response.Success(reqCtx, true)
}

func (c *Handler) DeleteDictItem(ctx context.Context, reqCtx *app.RequestContext) {
	id, err := parseQueryInt64(reqCtx, "id")
	if err != nil {
		response.Error(reqCtx, err)
		return
	}
	if err := c.management.DeleteDictItem(ctx, currentActor(reqCtx), id); err != nil {
		response.Error(reqCtx, err)
		return
	}
	response.Success(reqCtx, true)
}

func (c *Handler) ChangeDictItemStatus(ctx context.Context, reqCtx *app.RequestContext) {
	id, err := parseQueryInt64(reqCtx, "id")
	if err != nil {
		response.Error(reqCtx, err)
		return
	}
	status, err := parseQueryInt(reqCtx, "status")
	if err != nil {
		response.Error(reqCtx, err)
		return
	}
	if err := c.management.ChangeDictItemStatus(ctx, currentActor(reqCtx), id, status); err != nil {
		response.Error(reqCtx, err)
		return
	}
	response.Success(reqCtx, true)
}

func (c *Handler) GetDictItemList(ctx context.Context, reqCtx *app.RequestContext) {
	typeID, err := parsePathInt64(reqCtx, "typeId")
	if err != nil {
		response.Error(reqCtx, err)
		return
	}
	var request dictfacade.DictItemQueryRequest
	if err := httpx.Bind(reqCtx, &request); err != nil {
		response.Error(reqCtx, err)
		return
	}
	request.DictTypeID = typeID
	items, err := c.management.GetDictItemList(ctx, request)
	if err != nil {
		response.Error(reqCtx, err)
		return
	}
	response.Success(reqCtx, items)
}

func (c *Handler) BatchUpdateSort(ctx context.Context, reqCtx *app.RequestContext) {
	typeID, err := parseQueryInt64(reqCtx, "typeId")
	if err != nil {
		response.Error(reqCtx, err)
		return
	}
	var request dictfacade.DictItemSortRequest
	if err := httpx.BindAndValidate(reqCtx, &request); err != nil {
		response.Error(reqCtx, err)
		return
	}
	count, err := c.management.BatchUpdateSort(ctx, currentActor(reqCtx), typeID, request)
	if err != nil {
		response.Error(reqCtx, err)
		return
	}
	response.Success(reqCtx, count)
}

func (c *Handler) BatchGetDict(ctx context.Context, reqCtx *app.RequestContext) {
	var request dictfacade.DictBatchRequest
	if err := httpx.BindAndValidate(reqCtx, &request); err != nil {
		response.Error(reqCtx, err)
		return
	}
	result, err := c.management.BatchGetDict(ctx, request)
	if err != nil {
		response.Error(reqCtx, err)
		return
	}
	response.Success(reqCtx, result)
}

func (c *Handler) MoveDictItem(ctx context.Context, reqCtx *app.RequestContext) {
	typeID, err := parsePathInt64(reqCtx, "typeId")
	if err != nil {
		response.Error(reqCtx, err)
		return
	}
	itemID, err := parsePathInt64(reqCtx, "itemId")
	if err != nil {
		response.Error(reqCtx, err)
		return
	}
	var request dictfacade.MoveRequest
	if err := httpx.Bind(reqCtx, &request); err != nil {
		response.Error(reqCtx, err)
		return
	}
	if err := c.management.MoveDictItem(ctx, currentActor(reqCtx), typeID, itemID, request.BeforeID, request.AfterID); err != nil {
		response.Error(reqCtx, err)
		return
	}
	response.Success(reqCtx, true)
}

func (c *Handler) GetDictByCodeForClient(ctx context.Context, reqCtx *app.RequestContext) {
	dictCode := strings.TrimSpace(string(reqCtx.Param("dictCode")))
	result, err := c.client.GetDictByCodeForClient(ctx, currentActor(reqCtx), dictCode)
	if err != nil {
		response.Error(reqCtx, err)
		return
	}
	items := []dictfacade.DictItemVO{}
	if result != nil && result.Record != nil {
		items = result.Record[dictCode]
	}
	response.Success(reqCtx, items)
}

func (c *Handler) BatchGetDictForClient(ctx context.Context, reqCtx *app.RequestContext) {
	var request dictfacade.DictBatchRequest
	if err := httpx.BindAndValidate(reqCtx, &request); err != nil {
		response.Error(reqCtx, err)
		return
	}
	result, err := c.client.BatchGetDictForClient(ctx, currentActor(reqCtx), request)
	if err != nil {
		response.Error(reqCtx, err)
		return
	}
	response.Success(reqCtx, result)
}

func currentActor(reqCtx *app.RequestContext) application.Actor {
	user := securitycontext.Require(reqCtx)
	return application.Actor{
		UserID:        user.UserID,
		IsAdmin:       user.IsAdmin,
		Authenticated: securitycontext.IsLogin(reqCtx),
		AccountID:     user.UserID,
		ScopeID:       dictActorScopeID(user.PrimaryOrgID),
		AuthzVersion:  user.AuthVersion,
	}
}

func dictActorScopeID(primaryOrgID int64) string {
	if primaryOrgID > 0 {
		return "org:" + strconv.FormatInt(primaryOrgID, 10)
	}
	return "server:local"
}

func parsePathInt64(reqCtx *app.RequestContext, key string) (int64, error) {
	if reqCtx == nil {
		return 0, apperrors.Params("路径参数错误")
	}
	return parseStringInt64(string(reqCtx.Param(key)))
}

func parseQueryInt64(reqCtx *app.RequestContext, key string) (int64, error) {
	if reqCtx == nil {
		return 0, apperrors.Params("查询参数错误")
	}
	return parseStringInt64(string(reqCtx.Query(key)))
}

func parseQueryInt(reqCtx *app.RequestContext, key string) (int, error) {
	value := strings.TrimSpace(string(reqCtx.Query(key)))
	parsed, err := strconv.Atoi(value)
	if err != nil {
		return 0, apperrors.Params("查询参数错误")
	}
	return parsed, nil
}

func parseQueryBool(reqCtx *app.RequestContext, key string) bool {
	return strings.EqualFold(strings.TrimSpace(string(reqCtx.Query(key))), "true")
}

func parseStringInt64(value string) (int64, error) {
	parsed, err := strconv.ParseInt(strings.TrimSpace(value), 10, 64)
	if err != nil || parsed <= 0 {
		return 0, apperrors.Params("路径参数错误")
	}
	return parsed, nil
}
