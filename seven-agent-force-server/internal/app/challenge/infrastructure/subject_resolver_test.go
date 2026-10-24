package infrastructure

import (
	"context"
	"testing"

	"github.com/CaixyPromise/seven-agent/force/internal/app/challenge/domain"
	userfacade "github.com/CaixyPromise/seven-agent/force/internal/app/system/user/facade"
)

func TestSubjectResolverResolvesLoginPrefix(t *testing.T) {
	resolver := NewSubjectResolver(&fakeUserFacade{
		byAccount: map[string]*userfacade.SubjectRecord{
			"alice": {UserID: 1001, AccountName: "alice"},
		},
	})
	subject, err := resolver.Resolve(context.Background(), &domain.ChallengeSession{SubjectIdentifier: "login:alice"})
	if err != nil {
		t.Fatalf("resolve login subject: %v", err)
	}
	if subject == nil || subject.UserID != 1001 || subject.AccountName != "alice" {
		t.Fatalf("unexpected subject: %#v", subject)
	}
}

func TestSubjectResolverReturnsNilForBareUsername(t *testing.T) {
	resolver := NewSubjectResolver(&fakeUserFacade{
		byAccount: map[string]*userfacade.SubjectRecord{
			"bob": {UserID: 1002, AccountName: "bob"},
		},
	})
	subject, err := resolver.Resolve(context.Background(), &domain.ChallengeSession{SubjectIdentifier: "bob"})
	if err != nil {
		t.Fatalf("resolve bare subject: %v", err)
	}
	if subject != nil {
		t.Fatalf("expected bare subject to be unsupported, got %#v", subject)
	}
}

func TestSubjectResolverReturnsNilForBareNumericSubject(t *testing.T) {
	resolver := NewSubjectResolver(&fakeUserFacade{
		byAccount: map[string]*userfacade.SubjectRecord{
			"1001": {UserID: 2002, AccountName: "1001"},
		},
	})
	subject, err := resolver.Resolve(context.Background(), &domain.ChallengeSession{SubjectIdentifier: "1001"})
	if err != nil {
		t.Fatalf("resolve bare numeric subject: %v", err)
	}
	if subject != nil {
		t.Fatalf("expected bare numeric subject to be unsupported, got %#v", subject)
	}
}
