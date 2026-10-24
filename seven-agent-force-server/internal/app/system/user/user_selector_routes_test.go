package user

import (
	"context"
	"strings"
	"testing"

	userfacade "github.com/CaixyPromise/seven-agent/force/internal/app/system/user/facade"
	userhandler "github.com/CaixyPromise/seven-agent/force/internal/app/system/user/handler"
	"github.com/CaixyPromise/seven-agent/force/internal/shared/securitycontext"
	"github.com/cloudwego/hertz/pkg/app"
	"github.com/cloudwego/hertz/pkg/app/server"
	"github.com/cloudwego/hertz/pkg/common/ut"
)

func TestUserSelectorRoutesRequireLoginButNotAdminListPermission(t *testing.T) {
	selectors := &fakeUserSelectorFacade{
		options: []userfacade.SimpleUserVO{{
			ID: 2065424359060983808, Username: "alice", NickName: "Alice", Avatar: "/avatar.png", Status: 0,
		}},
		simple: &userfacade.SimpleUserVO{
			ID: 2065424359060983808, Username: "alice", NickName: "Alice", Status: 0,
		},
	}
	engine := server.Default()
	engine.Use(func(ctx context.Context, reqCtx *app.RequestContext) {
		if string(reqCtx.Request.Header.Peek("X-Test-Login")) == "1" {
			securitycontext.Set(reqCtx, &securitycontext.UserContext{
				UserID:           9001,
				Username:         "operator",
				Permissions:      []string{},
				DataScopeType:    securitycontext.DataScopeDept,
				DataScopeDeptIDs: []int64{7},
			})
		}
		reqCtx.Next(ctx)
	})
	module := &Module{
		handler:      userhandler.NewHandler(nil, nil, nil, selectors),
		adminHandler: userhandler.NewAdminHandler(nil, nil, nil, nil, nil),
	}
	module.Mount(engine)

	anonymous := ut.PerformRequest(engine.Engine, "GET", "/user/options", nil)
	if !strings.Contains(anonymous.Body.String(), `"code":40100`) {
		t.Fatalf("anonymous selector request was not rejected: %s", anonymous.Body.String())
	}

	headers := []ut.Header{{Key: "X-Test-Login", Value: "1"}}
	options := ut.PerformRequest(engine.Engine, "GET", "/user/options?limit=20&deptId=7", nil, headers...)
	assertMinimalSelectorResponse(t, options.Body.String())
	search := ut.PerformRequest(engine.Engine, "GET", "/user/search?keyword=ali&limit=10&deptId=7", nil, headers...)
	assertMinimalSelectorResponse(t, search.Body.String())
	simple := ut.PerformRequest(engine.Engine, "GET", "/user/simple/2065424359060983808", nil, headers...)
	assertMinimalSelectorResponse(t, simple.Body.String())

	if selectors.listCalls != 2 || selectors.getCalls != 1 {
		t.Fatalf("unexpected selector facade calls: list=%d get=%d", selectors.listCalls, selectors.getCalls)
	}
	if !selectors.lastQuery.Scope.Enabled || len(selectors.lastQuery.Scope.DeptIDs) != 1 || selectors.lastQuery.Scope.DeptIDs[0] != 7 {
		t.Fatalf("data scope was not forwarded: %#v", selectors.lastQuery.Scope)
	}
	if selectors.lastQuery.Keyword != "ali" || selectors.lastQuery.DeptID != 7 {
		t.Fatalf("search query was not forwarded: %#v", selectors.lastQuery)
	}
}

func assertMinimalSelectorResponse(t *testing.T, body string) {
	t.Helper()
	if !strings.Contains(body, `"code":0`) || !strings.Contains(body, `"id":"2065424359060983808"`) {
		t.Fatalf("selector response did not preserve Int64 string: %s", body)
	}
	for _, forbidden := range []string{"userEmail", "email", "userPhone", "phone"} {
		if strings.Contains(body, `"`+forbidden+`"`) {
			t.Fatalf("selector response leaked %s: %s", forbidden, body)
		}
	}
}

type fakeUserSelectorFacade struct {
	options   []userfacade.SimpleUserVO
	simple    *userfacade.SimpleUserVO
	listCalls int
	getCalls  int
	lastQuery userfacade.UserSelectorQuery
}

func (f *fakeUserSelectorFacade) ListUserOptions(_ context.Context, query userfacade.UserSelectorQuery) ([]userfacade.SimpleUserVO, error) {
	f.listCalls++
	f.lastQuery = query
	return f.options, nil
}

func (f *fakeUserSelectorFacade) GetSimpleUser(context.Context, int64, userfacade.DataScopeFilter) (*userfacade.SimpleUserVO, error) {
	f.getCalls++
	return f.simple, nil
}
