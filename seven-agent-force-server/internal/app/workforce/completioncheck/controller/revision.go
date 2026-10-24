package controller

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"github.com/CaixyPromise/seven-agent/force/internal/app/workforce/completioncheck/facade"
	"github.com/CaixyPromise/seven-agent/force/internal/common/response"
	"github.com/CaixyPromise/seven-agent/force/internal/shared/securitycontext"
	"github.com/cloudwego/hertz/pkg/app"
	"io"
	"strconv"
)

func decodeRevision(body []byte) (facade.RevisionRequest, error) {
	var r facade.RevisionRequest
	if len(body) > 128<<10 {
		return r, facade.ErrInvalid
	}
	d := json.NewDecoder(bytes.NewReader(body))
	token, e := d.Token()
	if e != nil || token != json.Delim('{') {
		return r, facade.ErrInvalid
	}
	seen := map[string]bool{}
	for d.More() {
		t, e := d.Token()
		key, ok := t.(string)
		if e != nil || !ok || seen[key] {
			return r, facade.ErrInvalid
		}
		seen[key] = true
		var raw json.RawMessage
		if d.Decode(&raw) != nil {
			return r, facade.ErrInvalid
		}
	}
	if _, e = d.Token(); e != nil {
		return r, facade.ErrInvalid
	}
	if e = d.Decode(&struct{}{}); !errors.Is(e, io.EOF) {
		return r, facade.ErrInvalid
	}
	d = json.NewDecoder(bytes.NewReader(body))
	d.DisallowUnknownFields()
	if d.Decode(&r) != nil {
		return r, facade.ErrInvalid
	}
	return r, nil
}
func (h *Handler) requestRevision(ctx context.Context, c *app.RequestContext) {
	tenant, task, user, e := scope(c)
	if e != nil {
		problem(c, e)
		return
	}
	if h.revisions == nil {
		problem(c, facade.ErrRevisionUnavailable)
		return
	}
	r, e := decodeRevision(c.Request.Body())
	if e != nil {
		problem(c, e)
		return
	}
	out, e := h.revisions.RequestRevision(ctx, facade.RevisionUser{TenantID: tenant, TaskID: task, UserID: user, CanContinue: securitycontext.HasPermission(c, "workforce:task:continue")}, r)
	if e != nil {
		problem(c, e)
		return
	}
	c.Response.Header.Set("Idempotency-Replayed", strconv.FormatBool(out.Replayed))
	response.Success(c, out)
}
func (h *Handler) pendingRevision(ctx context.Context, c *app.RequestContext) {
	tenant, task, user, e := scope(c)
	if e != nil {
		problem(c, e)
		return
	}
	if h.revisions == nil {
		problem(c, facade.ErrRevisionUnavailable)
		return
	}
	out, e := h.revisions.PendingForUser(ctx, facade.RevisionUser{TenantID: tenant, TaskID: task, UserID: user})
	if e != nil {
		problem(c, e)
		return
	}
	response.Success(c, out)
}

func (h *Handler) revisionView(ctx context.Context, c *app.RequestContext) {
	tenant, task, user, err := scope(c)
	if err != nil {
		problem(c, err)
		return
	}
	if h.revisions == nil {
		problem(c, facade.ErrRevisionUnavailable)
		return
	}
	out, err := h.revisions.RevisionView(ctx, facade.RevisionUser{TenantID: tenant, TaskID: task, UserID: user, CanContinue: securitycontext.HasPermission(c, "workforce:task:continue")})
	if err != nil {
		problem(c, err)
		return
	}
	response.Success(c, out)
}
