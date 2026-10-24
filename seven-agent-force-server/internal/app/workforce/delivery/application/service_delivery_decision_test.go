package application

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/CaixyPromise/seven-agent/force/internal/app/workforce/delivery/domain"
)

type decisionRepository struct {
	Repository
	restart Restart
	called  bool
}

func (r *decisionRepository) Restart(_ context.Context, request Restart) (RequestView, bool, error) {
	r.restart, r.called = request, true
	return RequestView{ID: 11}, false, nil
}

func TestRestartValidatesCurrentResultIdentityBeforeRepository(t *testing.T) {
	repository := &decisionRepository{}
	service := New(repository, true)
	valid := Restart{TenantID: 1, TaskID: 2, Actor: 3, ResultCommit: "AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA", IdempotencyKey: " restart-1 "}
	view, replay, err := service.Restart(context.Background(), valid)
	if err != nil || replay || view.ID != 11 || !repository.called {
		t.Fatalf("valid restart rejected: view=%+v replay=%v err=%v", view, replay, err)
	}
	if repository.restart.ResultCommit != "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa" || repository.restart.IdempotencyKey != "restart-1" {
		t.Fatalf("restart was not canonicalized: %+v", repository.restart)
	}
	for _, invalid := range []Restart{
		{TenantID: 1, TaskID: 2, Actor: 3, ResultCommit: "not-a-commit", IdempotencyKey: "key"},
		{TenantID: 1, TaskID: 2, Actor: 3, ResultCommit: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"},
		{TenantID: 1, TaskID: 2, ResultCommit: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", IdempotencyKey: "key"},
	} {
		repository.called = false
		_, _, err = service.Restart(context.Background(), invalid)
		if !errors.Is(err, domain.ErrDecisionNotAllowed) || repository.called {
			t.Fatalf("invalid restart reached repository: request=%+v err=%v", invalid, err)
		}
	}
}

func (r *decisionRepository) ReauthorizeRequest(_ context.Context, request Reauthorization) (RequestView, error) {
	r.called = true
	return RequestView{ID: request.RequestID, Status: "WAITING_APPROVAL"}, nil
}
func TestReauthorizationValidatesIdentityAndBoundedReason(t *testing.T) {
	r := &decisionRepository{}
	s := New(r, true)
	valid := Reauthorization{TenantID: 1, RequestID: 2, Actor: 3, Reason: " transport repaired "}
	if _, err := s.ReauthorizeRequest(context.Background(), valid); err != nil || !r.called {
		t.Fatalf("valid reauth rejected: %v", err)
	}
	for _, bad := range []Reauthorization{
		{TenantID: 1, RequestID: 2, Actor: 3, Reason: "repair", ProjectVersionID: -1},
		{RequestID: 2, Actor: 3, Reason: "repair"}, {TenantID: 1, Actor: 3, Reason: "repair"}, {TenantID: 1, RequestID: 2, Reason: "repair"}, {TenantID: 1, RequestID: 2, Actor: 3, Reason: " "}, {TenantID: 1, RequestID: 2, Actor: 3, Reason: strings.Repeat("中", 700)},
	} {
		r.called = false
		if _, err := s.ReauthorizeRequest(context.Background(), bad); !errors.Is(err, domain.ErrApprovalRequired) || r.called {
			t.Fatalf("invalid request reached repository: %v", err)
		}
	}
}
