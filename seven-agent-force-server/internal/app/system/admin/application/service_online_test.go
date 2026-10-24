package application

import (
	"context"
	"testing"

	ssofacade "github.com/CaixyPromise/seven-agent/force/internal/app/sso/facade"
	adminfacade "github.com/CaixyPromise/seven-agent/force/internal/app/system/admin/facade"
	apperrors "github.com/CaixyPromise/seven-agent/force/internal/common/errors"
	"github.com/CaixyPromise/seven-agent/force/internal/shared/stepup"
)

func TestForceLogoutRequiresServiceProofMetadata(t *testing.T) {
	tests := []struct {
		name  string
		proof stepup.ProofMetadata
	}{
		{name: "missing proof"},
		{name: "wrong action", proof: validAdminStepUpProof("RBAC_ASSIGN_USER_ROLES", "user:1002|force-logout")},
		{name: "wrong binding", proof: validAdminStepUpProof(stepUpActionAdminForceLogout, "user:1003|force-logout")},
		{name: "missing proof id", proof: stepup.ProofMetadata{BusinessAction: stepUpActionAdminForceLogout, OperationBinding: "user:1002|force-logout"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sessions := &fakeAdminSessionFacade{}
			service := NewService(LoginSettings{}, nil, nil, nil, sessions, nil, nil, nil, nil, nil, nil)

			ok, err := service.ForceLogout(context.Background(), adminfacade.ForceLogoutCommand{
				UserID:      1002,
				OperatorID:  9001,
				StepUpProof: tt.proof,
			})

			if ok {
				t.Fatal("expected rejected proof to return ok=false")
			}
			if appErr := apperrors.From(err); appErr == nil || appErr.Code() != apperrors.CodeForbidden {
				t.Fatalf("expected forbidden proof rejection, got %v", err)
			}
			if sessions.revokeUserCalls != 0 {
				t.Fatalf("expected no session revocation on rejected proof, got %d calls", sessions.revokeUserCalls)
			}
		})
	}

	sessions := &fakeAdminSessionFacade{}
	service := NewService(LoginSettings{}, nil, nil, nil, sessions, nil, nil, nil, nil, nil, nil)
	ok, err := service.ForceLogout(context.Background(), adminfacade.ForceLogoutCommand{
		UserID:      1002,
		OperatorID:  9001,
		StepUpProof: validAdminStepUpProof(stepUpActionAdminForceLogout, "user:1002|force-logout"),
	})
	if err != nil || !ok {
		t.Fatalf("expected valid proof to force logout, ok=%v err=%v", ok, err)
	}
	if sessions.revokeUserCalls != 1 || sessions.lastRevokedUserID != 1002 {
		t.Fatalf("expected one revoke for user 1002, calls=%d user=%d", sessions.revokeUserCalls, sessions.lastRevokedUserID)
	}
}

func TestBatchForceLogoutRequiresServiceProofMetadata(t *testing.T) {
	sessions := &fakeAdminSessionFacade{}
	service := NewService(LoginSettings{}, nil, nil, nil, sessions, nil, nil, nil, nil, nil, nil)

	_, err := service.BatchForceLogout(context.Background(), adminfacade.BatchForceLogoutCommand{
		UserIDs:     []int64{1003, 1002, 1002},
		OperatorID:  9001,
		StepUpProof: validAdminStepUpProof(stepUpActionAdminForceLogout, "users:1002|force-logout"),
	})
	if appErr := apperrors.From(err); appErr == nil || appErr.Code() != apperrors.CodeForbidden {
		t.Fatalf("expected forbidden proof rejection for wrong batch binding, got %v", err)
	}
	if sessions.revokeUserCalls != 0 {
		t.Fatalf("expected no batch revocation on rejected proof, got %d calls", sessions.revokeUserCalls)
	}

	result, err := service.BatchForceLogout(context.Background(), adminfacade.BatchForceLogoutCommand{
		UserIDs:     []int64{1003, 1002, 1002},
		OperatorID:  9001,
		StepUpProof: validAdminStepUpProof(stepUpActionAdminForceLogout, "users:1002,1003|force-logout"),
	})
	if err != nil {
		t.Fatalf("expected valid batch proof to force logout: %v", err)
	}
	if result.SuccessCount != 2 || result.FailedCount != 0 {
		t.Fatalf("unexpected batch result: %#v", result)
	}
	if sessions.revokeUserCalls != 2 {
		t.Fatalf("expected two unique user revocations, got %d", sessions.revokeUserCalls)
	}
}

func TestBatchForceLogoutAcceptsCompactProofBindingForLargeBatch(t *testing.T) {
	sessions := &fakeAdminSessionFacade{}
	service := NewService(LoginSettings{}, nil, nil, nil, sessions, nil, nil, nil, nil, nil, nil)
	userIDs := make([]int64, 0, 100)
	for i := int64(0); i < 100; i++ {
		userIDs = append(userIDs, 2065424246313897984+i)
	}
	binding := batchForceLogoutBinding(userIDs)
	if len(binding) > 128 {
		t.Fatalf("expected compact binding, got len=%d binding=%q", len(binding), binding)
	}

	result, err := service.BatchForceLogout(context.Background(), adminfacade.BatchForceLogoutCommand{
		UserIDs:     userIDs,
		OperatorID:  9001,
		StepUpProof: validAdminStepUpProof(stepUpActionAdminForceLogout, binding),
	})
	if err != nil {
		t.Fatalf("expected compact proof binding to pass: %v", err)
	}
	if result.SuccessCount != 100 || result.FailedCount != 0 {
		t.Fatalf("unexpected batch result: %#v", result)
	}
	if sessions.revokeUserCalls != 100 {
		t.Fatalf("expected 100 user revocations, got %d", sessions.revokeUserCalls)
	}
}

func TestForceLogoutRejectsSelfAtServiceLayer(t *testing.T) {
	sessions := &fakeAdminSessionFacade{}
	service := NewService(LoginSettings{}, nil, nil, nil, sessions, nil, nil, nil, nil, nil, nil)

	ok, err := service.ForceLogout(context.Background(), adminfacade.ForceLogoutCommand{
		UserID:      9001,
		OperatorID:  9001,
		StepUpProof: validAdminStepUpProof(stepUpActionAdminForceLogout, "user:9001|force-logout"),
	})
	if ok || err == nil {
		t.Fatalf("expected self force logout to fail, ok=%v err=%v", ok, err)
	}
	if sessions.revokeUserCalls != 0 {
		t.Fatalf("expected no revocation for self force logout, got %d calls", sessions.revokeUserCalls)
	}
}

func validAdminStepUpProof(action, binding string) stepup.ProofMetadata {
	return stepup.ProofMetadata{
		BusinessAction:        action,
		OperationBinding:      binding,
		ProofIdentifier:       "proof-jti",
		ChallengeIdentifier:   "challenge-id",
		AssuranceLevel:        "AAL2",
		AuthenticationMethods: []string{"TIME_BASED_ONE_TIME_PASSWORD"},
	}
}

type fakeAdminSessionFacade struct {
	revokeUserCalls   int
	lastRevokedUserID int64
}

func (f *fakeAdminSessionFacade) ListSessionsByUserID(context.Context, int64) ([]ssofacade.SessionRecord, error) {
	return nil, nil
}

func (f *fakeAdminSessionFacade) ListActiveSessions(context.Context) ([]ssofacade.SessionRecord, error) {
	return nil, nil
}

func (f *fakeAdminSessionFacade) CountActiveSessions(context.Context) (int64, error) {
	return 0, nil
}

func (f *fakeAdminSessionFacade) RevokeSession(context.Context, string) (bool, error) {
	return false, nil
}

func (f *fakeAdminSessionFacade) RevokeSessionsByUserID(_ context.Context, userID int64) (int64, error) {
	f.revokeUserCalls++
	f.lastRevokedUserID = userID
	return 1, nil
}

func (f *fakeAdminSessionFacade) RevokeSessionsByPlatformCode(context.Context, string) (int64, error) {
	return 0, nil
}

func (f *fakeAdminSessionFacade) RevokeSessionsByPlatformLoginMethod(context.Context, string, string, string) (int64, error) {
	return 0, nil
}

func (f *fakeAdminSessionFacade) RevokeSessionsByExternalProvider(context.Context, string) (int64, error) {
	return 0, nil
}

func (f *fakeAdminSessionFacade) RevokeSessionsByExternalIdentity(context.Context, int64) (int64, error) {
	return 0, nil
}

func (f *fakeAdminSessionFacade) ResolveActiveSessionRecord(context.Context, string) (*ssofacade.SessionRecord, error) {
	return nil, nil
}
