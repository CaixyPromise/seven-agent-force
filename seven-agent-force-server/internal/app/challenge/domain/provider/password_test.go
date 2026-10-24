package provider

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/CaixyPromise/seven-agent/force/internal/app/challenge/domain"
	passwordinfra "github.com/CaixyPromise/seven-agent/force/internal/infrastructure/security/password"
	"github.com/CaixyPromise/seven-agent/force/internal/shared/config"
)

func TestPasswordPrepareMarksStepRequired(t *testing.T) {
	provider := NewPasswordChallengeStepProvider(nil, nil)
	step := &domain.ChallengeStep{ChallengeType: domain.ChallengeTypePasswordVerification}

	if err := provider.Prepare(context.Background(), &domain.ChallengeSession{}, step); err != nil {
		t.Fatalf("prepare password step: %v", err)
	}
	if step.UserInterfaceHints["required"] != true {
		t.Fatalf("expected password step required hint, got %#v", step.UserInterfaceHints)
	}
}

func TestPasswordVerifyAcceptsMatchingCredentialAndRejectsWrongPassword(t *testing.T) {
	passwordService := newTestPasswordService(t)
	hash, err := passwordService.Hash(context.Background(), "correct horse battery staple")
	if err != nil {
		t.Fatalf("hash password: %v", err)
	}
	otherHash, err := passwordService.Hash(context.Background(), "another subject password")
	if err != nil {
		t.Fatalf("hash other subject password: %v", err)
	}
	store := &passwordSubjectStore{passwordHashBySubject: map[string]string{
		"user:1001": hash,
		"user:2002": otherHash,
	}}
	provider := NewPasswordChallengeStepProvider(passwordService, store)
	session := &domain.ChallengeSession{SubjectIdentifier: "user:1001"}
	step := &domain.ChallengeStep{ChallengeType: domain.ChallengeTypePasswordVerification}

	ok, err := provider.Verify(context.Background(), session, step, map[string]any{"password": "correct horse battery staple"})
	if err != nil {
		t.Fatalf("verify matching password: %v", err)
	}
	if !ok {
		t.Fatal("expected matching password to verify")
	}
	if store.lastSessionID != "user:1001" {
		t.Fatalf("expected provider to resolve credential for current session subject, got %q", store.lastSessionID)
	}

	ok, err = provider.Verify(context.Background(), session, step, map[string]any{"password": "wrong password"})
	if err != nil {
		t.Fatalf("verify wrong password: %v", err)
	}
	if ok {
		t.Fatal("expected wrong password to fail")
	}

	ok, err = provider.Verify(context.Background(), &domain.ChallengeSession{SubjectIdentifier: "user:2002"}, step, map[string]any{"password": "correct horse battery staple"})
	if err != nil {
		t.Fatalf("verify password against other subject credential: %v", err)
	}
	if ok {
		t.Fatal("expected password for a different subject credential to fail")
	}
}

func TestPasswordVerifyRejectsBlankOrMissingCredential(t *testing.T) {
	passwordService := newTestPasswordService(t)
	provider := NewPasswordChallengeStepProvider(passwordService, &passwordSubjectStore{})
	session := &domain.ChallengeSession{SubjectIdentifier: "user:1001"}
	step := &domain.ChallengeStep{ChallengeType: domain.ChallengeTypePasswordVerification}

	ok, err := provider.Verify(context.Background(), session, step, map[string]any{"password": "  "})
	if err != nil {
		t.Fatalf("verify blank password: %v", err)
	}
	if ok {
		t.Fatal("expected blank password to fail")
	}

	ok, err = provider.Verify(context.Background(), session, step, map[string]any{})
	if err != nil {
		t.Fatalf("verify absent password: %v", err)
	}
	if ok {
		t.Fatal("expected absent password to fail")
	}

	ok, err = provider.Verify(context.Background(), session, step, nil)
	if err != nil {
		t.Fatalf("verify nil payload password: %v", err)
	}
	if ok {
		t.Fatal("expected nil payload password to fail")
	}

	ok, err = provider.Verify(context.Background(), session, step, map[string]any{"password": "candidate"})
	if err != nil {
		t.Fatalf("verify missing credential: %v", err)
	}
	if ok {
		t.Fatal("expected missing password credential to fail")
	}
}

func TestPasswordVerifyPropagatesCredentialLookupErrors(t *testing.T) {
	passwordService := newTestPasswordService(t)
	expected := errors.New("credential store unavailable")
	provider := NewPasswordChallengeStepProvider(passwordService, &passwordSubjectStore{err: expected})

	ok, err := provider.Verify(context.Background(), &domain.ChallengeSession{SubjectIdentifier: "user:1001"}, &domain.ChallengeStep{}, map[string]any{"password": "candidate"})
	if !errors.Is(err, expected) {
		t.Fatalf("expected credential lookup error, got %v", err)
	}
	if ok {
		t.Fatal("expected errored credential lookup to fail")
	}
}

func TestPasswordProviderDoesNotPersistSubmittedPasswordOrCredentialHash(t *testing.T) {
	passwordService := newTestPasswordService(t)
	const plainPassword = "correct horse battery staple"
	hash, err := passwordService.Hash(context.Background(), plainPassword)
	if err != nil {
		t.Fatalf("hash password: %v", err)
	}
	provider := NewPasswordChallengeStepProvider(passwordService, &passwordSubjectStore{passwordHashBySubject: map[string]string{
		"user:1001": hash,
	}})
	session := &domain.ChallengeSession{SubjectIdentifier: "user:1001"}
	step := &domain.ChallengeStep{ChallengeType: domain.ChallengeTypePasswordVerification}

	if err := provider.Prepare(context.Background(), session, step); err != nil {
		t.Fatalf("prepare password step: %v", err)
	}
	ok, err := provider.Verify(context.Background(), session, step, map[string]any{"password": plainPassword})
	if err != nil {
		t.Fatalf("verify matching password: %v", err)
	}
	if !ok {
		t.Fatal("expected matching password to verify")
	}
	if mapContainsString(session.SessionContext, plainPassword) || mapContainsString(session.SessionContext, hash) {
		t.Fatalf("password provider must not persist submitted password or hash in session context: %#v", session.SessionContext)
	}
	if mapContainsString(step.UserInterfaceHints, plainPassword) || mapContainsString(step.UserInterfaceHints, hash) {
		t.Fatalf("password provider must not expose submitted password or hash in UI hints: %#v", step.UserInterfaceHints)
	}
}

func newTestPasswordService(t *testing.T) *passwordinfra.Service {
	t.Helper()
	service, err := passwordinfra.New(config.PasswordConfig{
		Algorithm: "bcrypt",
		Bcrypt:    config.BcryptPasswordConfig{Cost: 4},
	})
	if err != nil {
		t.Fatalf("new password service: %v", err)
	}
	return service
}

type passwordSubjectStore struct {
	fakeSubjectStore
	passwordHashBySubject map[string]string
	err                   error
	lastSessionID         string
}

func (s *passwordSubjectStore) FindPasswordCredential(ctx context.Context, session *domain.ChallengeSession) (string, error) {
	_ = ctx
	if session != nil {
		s.lastSessionID = session.SubjectIdentifier
	}
	if s.err != nil {
		return "", s.err
	}
	return s.passwordHashBySubject[session.SubjectIdentifier], nil
}

func mapContainsString(values map[string]any, needle string) bool {
	if needle == "" {
		return false
	}
	for key, value := range values {
		if strings.Contains(key, needle) || valueContainsString(value, needle) {
			return true
		}
	}
	return false
}

func valueContainsString(value any, needle string) bool {
	switch typed := value.(type) {
	case string:
		return strings.Contains(typed, needle)
	case map[string]any:
		return mapContainsString(typed, needle)
	case []any:
		for _, item := range typed {
			if valueContainsString(item, needle) {
				return true
			}
		}
	}
	return false
}
