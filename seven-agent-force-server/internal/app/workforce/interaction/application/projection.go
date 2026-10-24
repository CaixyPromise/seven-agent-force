package application

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/CaixyPromise/seven-agent/force/internal/app/workforce/interaction/domain"
	"github.com/CaixyPromise/seven-agent/force/internal/app/workforce/interaction/facade"
	"github.com/jackc/pgx/v5"
	"time"
)

func (s *Service) expire(ctx context.Context, tx pgx.Tx, tenant, task int64) error {
	items, e := s.repo.OpenQuestions(ctx, tx, tenant, task)
	if e != nil {
		return e
	}
	now := time.Now()
	for _, i := range items {
		if i.Expire(now) {
			if e = s.persist(ctx, tx, i, false); e != nil {
				return e
			}
		}
	}
	return nil
}

// CloseAttemptTx is a trusted lifecycle port for parent-owned cancel/supersede
// orchestration. Caller must hold the authorized Task lock; no machine/browser
// request is allowed to choose a lifecycle state. Transaction stays caller-owned.
func (s *Service) CloseAttemptTx(ctx context.Context, tx pgx.Tx, tenant, task, attempt int64, state string) error {
	if tx == nil || tenant <= 0 || task <= 0 || attempt <= 0 || (state != domain.Cancelled && state != domain.Superseded) {
		return domain.ErrInvalid
	}
	items, e := s.repo.AttemptInteractions(ctx, tx, tenant, task, attempt)
	if e != nil {
		return safe(e)
	}
	for _, i := range items {
		if i.AttemptID != attempt || (i.State != domain.Open && !(i.State == domain.Resolved && i.DeliveryState == domain.Saved && i.ResumeTaskTurnID == 0)) {
			continue
		}
		if e = i.Close(state); e != nil {
			return e
		}
		if e = s.persist(ctx, tx, i, false); e != nil {
			return safe(e)
		}
	}
	return nil
}
func (s *Service) ConversationItem(ctx context.Context, u facade.User, id int64) (out domain.Item, err error) {
	if id <= 0 {
		return out, domain.ErrInvalid
	}
	err = s.transaction(ctx, func(tx pgx.Tx) error {
		if _, e := s.user(ctx, tx, u, facade.Read); e != nil {
			return e
		}
		if e := s.expire(ctx, tx, u.TenantID, u.TaskID); e != nil {
			return e
		}
		var e error
		out, e = s.repo.Item(ctx, tx, u.TenantID, u.TaskID, id)
		return e
	})
	return
}

// PublishEvent admits display-only activity. It never interprets an event as
// an operation, grant, completion, delivery acknowledgement or execution proof.
func (s *Service) PublishEvent(ctx context.Context, m facade.Machine, r facade.PublishEventRequest) (out domain.Item, err error) {
	if (r.Kind != "ACTIVITY" && r.Kind != "ASSISTANT_MESSAGE") || domain.ValidateMessage(r.Text, r.LocalRequestKey) != nil {
		return out, domain.ErrInvalid
	}
	err = s.transaction(ctx, func(tx pgx.Tx) error {
		t, e := s.machine(ctx, tx, m)
		if e != nil {
			return e
		}
		if !capable(t, "activity") {
			return domain.ErrNotAllowed
		}
		key := fmt.Sprintf("%d:%d:%s", t.AttemptID, t.AdapterSessionGeneration, r.LocalRequestKey)
		digest := domain.Digest(r)
		old, e := s.repo.FindItem(ctx, tx, t.TenantID, t.TaskID, "ADAPTER_EVENT", key)
		if e != nil {
			return e
		}
		if old != nil {
			if old.Digest != digest {
				return domain.ErrConflict
			}
			out = *old
			return nil
		}
		body, _ := json.Marshal(struct {
			Text string `json:"text"`
		}{r.Text})
		out = domain.Item{ID: s.ids.NextID(), TenantID: t.TenantID, TaskID: t.TaskID, Version: 1, Type: r.Kind, Body: body, CreatedAt: time.Now().UTC(), SourceKind: "ADAPTER_EVENT", SourceKey: key, Digest: digest}
		return s.project(ctx, tx, out, t.AttemptID, t.TaskTurnID)
	})
	return
}
