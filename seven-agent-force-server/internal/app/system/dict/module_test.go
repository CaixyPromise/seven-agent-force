package dict

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"testing"

	adminfacade "github.com/CaixyPromise/seven-agent/force/internal/app/system/admin/facade"
	dictapp "github.com/CaixyPromise/seven-agent/force/internal/app/system/dict/application"
	dictfacade "github.com/CaixyPromise/seven-agent/force/internal/app/system/dict/facade"
	dicthandler "github.com/CaixyPromise/seven-agent/force/internal/app/system/dict/handler"
	apperrors "github.com/CaixyPromise/seven-agent/force/internal/common/errors"
	"github.com/CaixyPromise/seven-agent/force/internal/common/response"
	"github.com/CaixyPromise/seven-agent/force/internal/shared/securitycontext"
	"github.com/cloudwego/hertz/pkg/app"
	"github.com/cloudwego/hertz/pkg/app/server"
	"github.com/cloudwego/hertz/pkg/common/ut"
)

func TestModulePermissionGuardsAndClientRoutes(t *testing.T) {
	engine := server.Default()
	engine.Use(func(ctx context.Context, reqCtx *app.RequestContext) {
		if string(reqCtx.Request.Header.Peek("X-Test-Login")) == "true" {
			securitycontext.Set(reqCtx, &securitycontext.UserContext{
				UserID:      1001,
				Username:    "admin",
				Permissions: splitPermissions(string(reqCtx.Request.Header.Peek("X-Test-Permissions"))),
			})
		}
		reqCtx.Next(ctx)
	})
	module := &Module{
		handler: dicthandler.NewHandler(&fakeManagementService{}, &fakeClientService{}),
	}
	module.Mount(engine.Engine)

	assertBusinessCode(t, ut.PerformRequest(engine.Engine, "GET", "/dict-type/types", nil), apperrors.CodeNotLogin)
	assertBusinessCode(t, ut.PerformRequest(engine.Engine, "GET", "/dict-type/types", nil,
		ut.Header{Key: "X-Test-Login", Value: "true"},
	), apperrors.CodeForbidden)
	assertBusinessCode(t, ut.PerformRequest(engine.Engine, "GET", "/dict-type/types", nil,
		ut.Header{Key: "X-Test-Login", Value: "true"},
		ut.Header{Key: "X-Test-Permissions", Value: "system:dict:query"},
	), apperrors.CodeSuccess)

	assertBusinessCode(t, ut.PerformRequest(engine.Engine, "GET", "/dict-client/public_demo", nil), apperrors.CodeSuccess)
	assertBusinessCode(t, ut.PerformRequest(engine.Engine, "GET", "/dict-client/secure_demo", nil), apperrors.CodeNotLogin)
	assertBusinessCode(t, ut.PerformRequest(engine.Engine, "GET", "/dict-client/secure_demo", nil,
		ut.Header{Key: "X-Test-Login", Value: "true"},
	), apperrors.CodeSuccess)
}

func TestSensitiveDictMutationOperationAuditUsesDigestOnly(t *testing.T) {
	const canaryValue = "secret-item-value-canary"
	const canaryLabel = "secret-item-label-canary"
	const canaryExt = "secret-ext-json-canary"
	reqCtx := app.NewContext(0)
	reqCtx.Request.SetMethod("POST")
	reqCtx.Request.SetRequestURI("/dict/17/items")
	reqCtx.Request.SetBodyString(`{"itemValue":"` + canaryValue + `","itemLabel":"` + canaryLabel + `","extJson":"` + canaryExt + `"}`)
	entry := adminfacade.OperationLogEntry{}
	safeDictMutationAuditEnricher{}.Enrich(context.Background(), reqCtx, &entry)
	if !strings.Contains(entry.RequestParams, `"kind":"sensitive_dict_mutation"`) ||
		!strings.Contains(entry.RequestParams, `"bodySha256":"`) {
		t.Fatalf("expected safe mutation digest, got %s", entry.RequestParams)
	}
	for _, canary := range []string{canaryValue, canaryLabel, canaryExt, "itemValue", "itemLabel", "extJson"} {
		if strings.Contains(entry.RequestParams, canary) {
			t.Fatalf("operation audit leaked %q: %s", canary, entry.RequestParams)
		}
	}
	spec := dictItemMutationSpec("更新字典项")
	if spec.IncludeParams {
		t.Fatal("sensitive dictionary mutation must not capture raw parameters")
	}
}

type fakeManagementService struct{}

func (f *fakeManagementService) AddDictType(context.Context, dictapp.Actor, dictfacade.DictTypeAddRequest) (int64, error) {
	return 1, nil
}
func (f *fakeManagementService) UpdateDictType(context.Context, dictapp.Actor, dictfacade.DictTypeUpdateRequest) error {
	return nil
}
func (f *fakeManagementService) DeleteDictType(context.Context, dictapp.Actor, int64, bool) error {
	return nil
}
func (f *fakeManagementService) GetDictTypeByID(context.Context, int64) (*dictfacade.DictTypeVO, error) {
	return &dictfacade.DictTypeVO{ID: 1, DictCode: "public_demo", DictName: "Public Demo", Status: 1}, nil
}
func (f *fakeManagementService) GetDictTypePage(context.Context, dictfacade.DictTypeQueryRequest) (*dictfacade.PageResult[dictfacade.DictTypeVO], error) {
	return &dictfacade.PageResult[dictfacade.DictTypeVO]{
		Current: 1,
		Size:    10,
		Total:   1,
		Records: []dictfacade.DictTypeVO{{ID: 1, DictCode: "public_demo", DictName: "Public Demo", Status: 1}},
	}, nil
}
func (f *fakeManagementService) ChangeDictTypeStatus(context.Context, dictapp.Actor, int64, int) error {
	return nil
}
func (f *fakeManagementService) MoveDictType(context.Context, dictapp.Actor, int64, *int64, *int64) error {
	return nil
}
func (f *fakeManagementService) AddDictItem(context.Context, dictapp.Actor, int64, dictfacade.DictItemAddRequest) (int64, error) {
	return 1, nil
}
func (f *fakeManagementService) UpdateDictItem(context.Context, dictapp.Actor, dictfacade.DictItemUpdateRequest) error {
	return nil
}
func (f *fakeManagementService) DeleteDictItem(context.Context, dictapp.Actor, int64) error {
	return nil
}
func (f *fakeManagementService) ChangeDictItemStatus(context.Context, dictapp.Actor, int64, int) error {
	return nil
}
func (f *fakeManagementService) GetDictItemList(context.Context, dictfacade.DictItemQueryRequest) ([]dictfacade.DictItemVO, error) {
	return []dictfacade.DictItemVO{{ID: 1, DictTypeID: 1, ItemValue: "A", ItemLabel: "Alpha", Status: 1}}, nil
}
func (f *fakeManagementService) BatchUpdateSort(context.Context, dictapp.Actor, int64, dictfacade.DictItemSortRequest) (int, error) {
	return 1, nil
}
func (f *fakeManagementService) MoveDictItem(context.Context, dictapp.Actor, int64, int64, *int64, *int64) error {
	return nil
}
func (f *fakeManagementService) BatchGetDict(context.Context, dictfacade.DictBatchRequest) (*dictfacade.DictBatchResponse, error) {
	return &dictfacade.DictBatchResponse{
		Record: map[string][]dictfacade.DictItemVO{
			"public_demo": {{ID: 1, DictTypeID: 1, ItemValue: "A", ItemLabel: "Alpha", Status: 1}},
		},
	}, nil
}

type fakeClientService struct{}

func (f *fakeClientService) GetDictByCodeForClient(_ context.Context, actor dictapp.Actor, dictCode string) (*dictfacade.DictBatchResponse, error) {
	if dictCode == "secure_demo" && !actor.Authenticated {
		return nil, apperrors.Unauthorized("字典需要登录后访问：" + dictCode)
	}
	return &dictfacade.DictBatchResponse{
		Record: map[string][]dictfacade.DictItemVO{
			dictCode: {{ID: 1, DictTypeID: 1, ItemValue: "A", ItemLabel: "Alpha", Status: 1}},
		},
	}, nil
}

func (f *fakeClientService) BatchGetDictForClient(_ context.Context, actor dictapp.Actor, request dictfacade.DictBatchRequest) (*dictfacade.DictBatchResponse, error) {
	for _, code := range request.DictCodes {
		if code == "secure_demo" && !actor.Authenticated {
			return nil, apperrors.Unauthorized("字典需要登录后访问：" + code)
		}
	}
	result := &dictfacade.DictBatchResponse{Record: map[string][]dictfacade.DictItemVO{}}
	for _, code := range request.DictCodes {
		result.Record[code] = []dictfacade.DictItemVO{{ID: 1, DictTypeID: 1, ItemValue: "A", ItemLabel: "Alpha", Status: 1}}
	}
	return result, nil
}

func assertBusinessCode(t *testing.T, recorder *ut.ResponseRecorder, expected int) {
	t.Helper()
	if recorder.Code != 200 {
		t.Fatalf("unexpected status: %d body=%s", recorder.Code, recorder.Body.String())
	}
	var result response.Result
	if err := json.Unmarshal(recorder.Body.Bytes(), &result); err != nil {
		t.Fatalf("unmarshal result: %v", err)
	}
	if result.Code != expected {
		t.Fatalf("unexpected business code: got=%d want=%d body=%s", result.Code, expected, recorder.Body.String())
	}
}

func splitPermissions(value string) []string {
	if value == "" {
		return nil
	}
	decoder := json.NewDecoder(bytes.NewBufferString(`["` + value + `"]`))
	var values []string
	if err := decoder.Decode(&values); err == nil && len(values) == 1 {
		return bytesToPermissions(value)
	}
	return bytesToPermissions(value)
}

func bytesToPermissions(value string) []string {
	result := make([]string, 0, 2)
	for _, item := range bytes.Split([]byte(value), []byte(",")) {
		trimmed := bytes.TrimSpace(item)
		if len(trimmed) == 0 {
			continue
		}
		result = append(result, string(trimmed))
	}
	return result
}
