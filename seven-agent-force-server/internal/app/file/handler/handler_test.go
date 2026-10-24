package handler

import (
	"testing"

	"github.com/CaixyPromise/seven-agent/force/internal/shared/securitycontext"
	"github.com/cloudwego/hertz/pkg/app"
)

func TestDownloadURLPayloadIncludesUIAndLegacyKeys(t *testing.T) {
	payload := downloadURLPayload("/uploads/files/1/download")
	if payload["url"] != "/uploads/files/1/download" {
		t.Fatalf("expected url field for UI contract, got %#v", payload["url"])
	}
	if payload["downloadUrl"] != "/uploads/files/1/download" {
		t.Fatalf("expected legacy downloadUrl field, got %#v", payload["downloadUrl"])
	}
}

func TestActorScopeComesOnlyFromAuthenticatedOrganizationContext(t *testing.T) {
	reqCtx := app.NewContext(0)
	reqCtx.Request.Header.Set("X-Scope-Id", "org:999")
	securitycontext.Set(reqCtx, &securitycontext.UserContext{
		UserID:       101,
		PrimaryOrgID: 22,
		OrgIDs:       []int64{22, 33},
	})
	got := actor(reqCtx)
	if got.ScopeID != "org:22" || got.ScopeSource != "primary-org" {
		t.Fatalf("actor accepted a non-authenticated organization source: %+v", got)
	}

	fallbackCtx := app.NewContext(0)
	securitycontext.Set(fallbackCtx, &securitycontext.UserContext{
		UserID: 101,
		OrgIDs: []int64{33},
	})
	fallback := actor(fallbackCtx)
	if fallback.ScopeID != "org:33" || fallback.ScopeSource != "single-org-fallback" {
		t.Fatalf("single organization fallback is not auditable: %+v", fallback)
	}
}

func TestFormOrQueryInt64ReadsMultipartPartNumber(t *testing.T) {
	reqCtx := app.NewContext(0)
	reqCtx.SetFormValueFunc(func(_ *app.RequestContext, key string) []byte {
		if key == "partNumber" {
			return []byte("2")
		}
		return nil
	})
	if got := formOrQueryInt64(reqCtx, "partNumber", 0); got != 2 {
		t.Fatalf("partNumber = %d, want 2", got)
	}
}
