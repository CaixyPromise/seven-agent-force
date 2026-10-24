package application

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	authorizationfacade "github.com/CaixyPromise/seven-agent/force/internal/app/authorization/facade"
	credentialfacade "github.com/CaixyPromise/seven-agent/force/internal/app/credential/facade"
	filefacade "github.com/CaixyPromise/seven-agent/force/internal/app/file/facade"
	ssofacade "github.com/CaixyPromise/seven-agent/force/internal/app/sso/facade"
	"github.com/CaixyPromise/seven-agent/force/internal/app/system/user/domain"
	userfacade "github.com/CaixyPromise/seven-agent/force/internal/app/system/user/facade"
	apperrors "github.com/CaixyPromise/seven-agent/force/internal/common/errors"
	"github.com/CaixyPromise/seven-agent/force/internal/infrastructure/datasource/store"
	passwordinfra "github.com/CaixyPromise/seven-agent/force/internal/infrastructure/security/password"
	"github.com/CaixyPromise/seven-agent/force/internal/shared/config"
	"github.com/CaixyPromise/seven-agent/force/internal/shared/securitycontext"
	"github.com/CaixyPromise/seven-agent/force/internal/shared/stepup"
	"github.com/CaixyPromise/seven-agent/force/internal/shared/xid"
)

func TestFindSubjectByIDBuildsIdentityCoreFields(t *testing.T) {
	unsealAt := time.Now().UTC().Add(time.Hour)
	service := newTestService(&fakeRepository{
		byID: map[int64]*domain.SubjectRecord{
			1001: {
				UserID:      1001,
				AccountName: "admin",
				Email:       "admin@example.com",
				Phone:       "13800138000",
				Status:      1,
				UnsealAt:    &unsealAt,
			},
		},
	}, nil)

	record, err := service.FindSubjectByID(context.Background(), 1001)
	if err != nil {
		t.Fatalf("find subject by id: %v", err)
	}
	if record == nil || record.UserID != 1001 || record.Enabled || !record.LockStatus {
		t.Fatalf("unexpected subject record: %#v", record)
	}
}

func TestCreateFirstOrganizationAssignsCreatorAsPrimaryMember(t *testing.T) {
	repo := &fakeRepository{}
	service := newTestService(repo, nil)
	service.transactor = &avatarTestTransactor{}

	status := 0
	if err := service.CreateOrg(context.Background(), userfacade.OrgCommand{
		Code:       "SEVEN_WORKFORCE",
		Name:       "Seven Agent Force",
		Status:     &status,
		OperatorID: 1001,
	}); err != nil {
		t.Fatalf("create first organization: %v", err)
	}
	if repo.createdOrg == nil || repo.createdOrg.ID <= 0 {
		t.Fatalf("organization was not created: %#v", repo.createdOrg)
	}
	if repo.replaceUserOrgsCalls != 1 || repo.replaceUserOrgsUserID != 1001 ||
		len(repo.replaceUserOrgIDs) != 1 || repo.replaceUserOrgIDs[0] != repo.createdOrg.ID ||
		repo.replaceUserOrgPrimaryID != repo.createdOrg.ID {
		t.Fatalf("creator primary organization assignment mismatch: %#v", repo)
	}
}

func TestGetAuthorizationUserAggregateCarriesAuthoritativeEnabledAndLockedGates(t *testing.T) {
	unsealAt := time.Now().UTC().Add(time.Hour)
	service := newTestService(&fakeRepository{byID: map[int64]*domain.SubjectRecord{
		1001: {UserID: 1001, AccountName: "locked", Status: domain.UserStatusDisabled, UnsealAt: &unsealAt},
	}}, nil)

	aggregate, err := service.GetAuthorizationUserAggregate(context.Background(), 1001)
	if err != nil {
		t.Fatalf("get authorization aggregate: %v", err)
	}
	if aggregate == nil || aggregate.Enabled || !aggregate.Locked {
		t.Fatalf("authorization aggregate must carry source status gates, got %#v", aggregate)
	}
}

func TestQueryUsersFailsClosedWithoutSnapshotter(t *testing.T) {
	service := newTestService(&fakeRepository{}, nil)
	if _, err := service.QueryUsers(context.Background(), userfacade.AdminUserQuery{}); err == nil {
		t.Fatal("expected split admin-user query to fail without a consistent snapshot")
	}
}

func TestFindSubjectByEmailReturnsEnabledUniqueUser(t *testing.T) {
	repo := &fakeRepository{
		byEmail: map[string]*domain.SubjectRecord{
			"verified@example.com": {
				UserID:      1001,
				AccountName: "verified",
				Email:       "verified@example.com",
				Status:      0,
			},
		},
		emailCount: 1,
	}
	service := newTestService(repo, nil)

	record, err := service.FindSubjectByEmail(context.Background(), " verified@example.com ")
	if err != nil {
		t.Fatalf("find subject by email: %v", err)
	}
	if record == nil || record.UserID != 1001 || record.AccountName != "verified" || !record.Enabled {
		t.Fatalf("unexpected subject record: %#v", record)
	}
	if repo.countByEmailCalls != 0 || repo.emailLookupCount != 1 || repo.emailLookupArg != "verified@example.com" {
		t.Fatalf("expected one normalized lookup without count, got count=%d lookup=%d arg=%q", repo.countByEmailCalls, repo.emailLookupCount, repo.emailLookupArg)
	}
}

func TestFindSubjectByEmailReturnsNilForBlankEmail(t *testing.T) {
	repo := &fakeRepository{}
	service := newTestService(repo, nil)

	record, err := service.FindSubjectByEmail(context.Background(), "   ")
	if err != nil {
		t.Fatalf("blank email should not error: %v", err)
	}
	if record != nil {
		t.Fatalf("expected nil subject for blank email, got %#v", record)
	}
	if repo.countByEmailCalls != 0 || repo.emailLookupCount != 0 {
		t.Fatalf("blank email should not query repository, got count=%d lookup=%d", repo.countByEmailCalls, repo.emailLookupCount)
	}
}

func TestFindSubjectByEmailReturnsErrorForDuplicateEmail(t *testing.T) {
	repo := &fakeRepository{emailLookupErr: apperrors.Operation("邮箱匹配到多个用户，禁止自动绑定")}
	service := newTestService(repo, nil)

	record, err := service.FindSubjectByEmail(context.Background(), "dup@example.com")
	if record != nil {
		t.Fatalf("expected nil subject for duplicate email, got %#v", record)
	}
	if appErr := apperrors.From(err); appErr == nil || appErr.Message() != "邮箱匹配到多个用户，禁止自动绑定" {
		t.Fatalf("expected duplicate email operation error, got %v", err)
	}
	if repo.countByEmailCalls != 0 || repo.emailLookupCount != 1 {
		t.Fatalf("duplicate email should use repository lookup guard only, got count=%d lookup=%d", repo.countByEmailCalls, repo.emailLookupCount)
	}
}

func TestCreateExternalSubjectPersistsRegisterSourceAndDefaultRoles(t *testing.T) {
	repo := &fakeRepository{}
	service := newTestService(repo, nil)
	idGen, err := xid.New(1)
	if err != nil {
		t.Fatalf("id generator: %v", err)
	}
	service.idGen = idGen

	subject, err := service.CreateExternalSubject(context.Background(), userfacade.CreateExternalSubjectCommand{
		AccountName:          "github-user",
		NickName:             "GitHub User",
		UserEmail:            "github-user@example.com",
		UserAvatar:           "https://example.com/avatar.png",
		RegisterPlatformCode: "seven-admin",
		RegisterProviderCode: "github",
		DefaultOrgID:         pointerInt64(41),
		DefaultDeptID:        pointerInt64(21),
		DefaultPostIDs:       []int64{31, 32, 31},
		DefaultRoleIDs:       []int64{11, 12, 11},
	})
	if err != nil {
		t.Fatalf("create external subject: %v", err)
	}
	if subject == nil || subject.Email != "github-user@example.com" {
		t.Fatalf("unexpected subject: %#v", subject)
	}
	if repo.createdExternal == nil || repo.createdExternal.RegisterPlatformCode != "seven-admin" || repo.createdExternal.RegisterProviderCode != "github" {
		t.Fatalf("registration source not persisted: %#v", repo.createdExternal)
	}
	if repo.replaceUserRolesCalls != 1 || repo.replaceUserRolesUserID != repo.createdExternal.ID {
		t.Fatalf("default roles not assigned, calls=%d userID=%d created=%#v", repo.replaceUserRolesCalls, repo.replaceUserRolesUserID, repo.createdExternal)
	}
	if got := repo.replaceUserRoleIDs; len(got) != 2 || got[0] != 11 || got[1] != 12 {
		t.Fatalf("default roles not normalized: %#v", got)
	}
	if repo.replaceUserOrgsCalls != 1 || repo.replaceUserOrgsUserID != repo.createdExternal.ID || repo.replaceUserOrgPrimaryID != 41 {
		t.Fatalf("default org not assigned, calls=%d userID=%d primary=%d", repo.replaceUserOrgsCalls, repo.replaceUserOrgsUserID, repo.replaceUserOrgPrimaryID)
	}
	if got := repo.replaceUserOrgIDs; len(got) != 1 || got[0] != 41 {
		t.Fatalf("default org ids not normalized: %#v", got)
	}
	if repo.replaceUserDeptsCalls != 1 || repo.replaceUserDeptsUserID != repo.createdExternal.ID || repo.replaceUserDeptPrimaryID != 21 {
		t.Fatalf("default dept not assigned, calls=%d userID=%d primary=%d", repo.replaceUserDeptsCalls, repo.replaceUserDeptsUserID, repo.replaceUserDeptPrimaryID)
	}
	if got := repo.replaceUserDeptIDs; len(got) != 1 || got[0] != 21 {
		t.Fatalf("default dept ids not normalized: %#v", got)
	}
	if repo.replaceUserPostsCalls != 1 || repo.replaceUserPostsUserID != repo.createdExternal.ID {
		t.Fatalf("default posts not assigned, calls=%d userID=%d", repo.replaceUserPostsCalls, repo.replaceUserPostsUserID)
	}
	if got := repo.replaceUserPostIDs; len(got) != 2 || got[0] != 31 || got[1] != 32 {
		t.Fatalf("default post ids not normalized: %#v", got)
	}
}

func TestCreateExternalSubjectCanDisableEmailMergeForFederatedIdentity(t *testing.T) {
	repo := &fakeRepository{
		byEmail:    map[string]*domain.SubjectRecord{"shared@example.com": {UserID: 77, Email: "shared@example.com", Status: 0}},
		emailCount: 1,
	}
	service := newTestService(repo, nil)
	idGen, err := xid.New(1)
	if err != nil {
		t.Fatalf("id generator: %v", err)
	}
	service.idGen = idGen
	subject, err := service.CreateExternalSubject(context.Background(), userfacade.CreateExternalSubjectCommand{
		AccountName: "hub-user", NickName: "Hub User", UserEmail: "shared@example.com",
		RegisterProviderCode: "hub:order-admin", DisableEmailMerge: true,
	})
	if err != nil {
		t.Fatalf("create non-merging federated subject: %v", err)
	}
	if subject == nil || subject.UserID == 77 || repo.createdExternal == nil {
		t.Fatalf("federated subject merged existing email: subject=%#v created=%#v", subject, repo.createdExternal)
	}
	if repo.emailLookupCount != 0 || repo.countByEmailCalls != 0 {
		t.Fatalf("email merge checks ran: lookup=%d count=%d", repo.emailLookupCount, repo.countByEmailCalls)
	}
}

func TestFindSubjectByEmailReturnsNilForDisabledUser(t *testing.T) {
	repo := &fakeRepository{
		byEmail: map[string]*domain.SubjectRecord{
			"disabled@example.com": {
				UserID:      1002,
				AccountName: "disabled",
				Email:       "disabled@example.com",
				Status:      1,
			},
		},
	}
	service := newTestService(repo, nil)

	record, err := service.FindSubjectByEmail(context.Background(), "disabled@example.com")
	if err != nil {
		t.Fatalf("disabled email lookup should not error: %v", err)
	}
	if record != nil {
		t.Fatalf("expected disabled user to be non-bindable, got %#v", record)
	}
}

func TestFindSubjectByEmailReturnsNilForLockedUser(t *testing.T) {
	unsealAt := time.Now().UTC().Add(time.Hour)
	repo := &fakeRepository{
		byEmail: map[string]*domain.SubjectRecord{
			"locked@example.com": {
				UserID:      1003,
				AccountName: "locked",
				Email:       "locked@example.com",
				Status:      1,
				UnsealAt:    &unsealAt,
			},
		},
	}
	service := newTestService(repo, nil)

	record, err := service.FindSubjectByEmail(context.Background(), "locked@example.com")
	if err != nil {
		t.Fatalf("locked email lookup should not error: %v", err)
	}
	if record != nil {
		t.Fatalf("expected locked user to be non-bindable, got %#v", record)
	}
}

func TestFindSubjectByEmailNormalizesCase(t *testing.T) {
	repo := &fakeRepository{
		byEmail: map[string]*domain.SubjectRecord{
			"case@example.com": {
				UserID:      1004,
				AccountName: "case",
				Email:       "case@example.com",
				Status:      0,
			},
		},
	}
	service := newTestService(repo, nil)

	record, err := service.FindSubjectByEmail(context.Background(), " CASE@Example.COM ")
	if err != nil {
		t.Fatalf("find subject by normalized email: %v", err)
	}
	if record == nil || record.UserID != 1004 {
		t.Fatalf("expected normalized email lookup to find user, got %#v", record)
	}
	if repo.emailLookupArg != "case@example.com" {
		t.Fatalf("expected lower-case normalized lookup arg, got %q", repo.emailLookupArg)
	}
}

func TestUpdateSelfProfileValidatesAndUpdatesPhone(t *testing.T) {
	repo := &fakeRepository{
		byID: map[int64]*domain.SubjectRecord{
			1001: {UserID: 1001, AccountName: "admin", Phone: "13800138000", Status: 0},
		},
	}
	service := newTestService(repo, nil)

	err := service.UpdateSelfProfile(context.Background(), userfacade.UpdateSelfProfileCommand{
		UserID:    1001,
		UserPhone: stringPointer("13900139000"),
	})
	if err != nil {
		t.Fatalf("update self profile phone: %v", err)
	}
	if repo.updatedPhone == nil || *repo.updatedPhone != "13900139000" {
		t.Fatalf("expected phone updated, got %#v", repo.updatedPhone)
	}
}

func TestUpdateSelfProfileIgnoresEmptyPhoneWithoutChallenge(t *testing.T) {
	repo := &fakeRepository{
		byID: map[int64]*domain.SubjectRecord{
			1001: {UserID: 1001, AccountName: "admin", Phone: "13800138000", Status: 0},
		},
	}
	service := newTestService(repo, nil)

	err := service.UpdateSelfProfile(context.Background(), userfacade.UpdateSelfProfileCommand{
		UserID:    1001,
		UserPhone: stringPointer(""),
	})
	if err != nil {
		t.Fatalf("ignore empty phone without challenge: %v", err)
	}
	if repo.updatedPhone != nil {
		t.Fatalf("expected empty phone to be ignored, got %#v", repo.updatedPhone)
	}
}

func TestCommitCurrentUserAvatarUsesOneSharedTransactionContext(t *testing.T) {
	repo := &fakeRepository{
		byID: map[int64]*domain.SubjectRecord{
			1001: {UserID: 1001, AccountName: "admin", Status: 0},
		},
	}
	service := newTestService(repo, nil)
	transactor := &avatarTestTransactor{}
	files := &fakeFileAssetBindingFacade{
		result: &filefacade.FileReferenceDTO{
			FileID:        9001,
			UserID:        1001,
			VisitURL:      "/public/avatar/1001/avatar.png",
			VisitStrategy: string(filefacade.VisitPublicStatic),
			AccessScope:   string(filefacade.AccessPublic),
		},
	}
	service.transactor = transactor
	service.BindFileAssets(files)

	avatar, err := service.CommitCurrentUserAvatar(authenticatedUserContext(1001, 22), 1001, 9001)
	if err != nil {
		t.Fatalf("CommitCurrentUserAvatar() error = %v", err)
	}
	if avatar != "/public/avatar/1001/avatar.png" || repo.updatedAvatar == nil || *repo.updatedAvatar != avatar {
		t.Fatalf("avatar result/persistence mismatch: result=%q stored=%v", avatar, repo.updatedAvatar)
	}
	if !transactor.committed || transactor.rolledBack || files.transactionMarker == "" || repo.transactionMarker != files.transactionMarker {
		t.Fatalf("file bind and user update did not share one committed transaction: tx=%+v fileMarker=%q repoMarker=%q", transactor, files.transactionMarker, repo.transactionMarker)
	}
}

func TestCommitCurrentUserAvatarRollsBackWhenBusinessPersistenceFails(t *testing.T) {
	repo := &fakeRepository{
		byID: map[int64]*domain.SubjectRecord{
			1001: {UserID: 1001, AccountName: "admin", Status: 0},
		},
		updateProfileErr: fmt.Errorf("injected user persistence failure"),
	}
	service := newTestService(repo, nil)
	transactor := &avatarTestTransactor{}
	files := &fakeFileAssetBindingFacade{
		result: &filefacade.FileReferenceDTO{
			FileID:        9001,
			UserID:        1001,
			VisitURL:      "/public/avatar/1001/avatar.png",
			VisitStrategy: string(filefacade.VisitPublicStatic),
			AccessScope:   string(filefacade.AccessPublic),
		},
	}
	service.transactor = transactor
	service.BindFileAssets(files)

	if _, err := service.CommitCurrentUserAvatar(authenticatedUserContext(1001, 22), 1001, 9001); err == nil {
		t.Fatal("injected business persistence failure must fail the avatar use case")
	}
	if transactor.committed || !transactor.rolledBack || repo.transactionMarker != files.transactionMarker {
		t.Fatalf("failed avatar use case did not roll back its shared transaction: tx=%+v fileMarker=%q repoMarker=%q", transactor, files.transactionMarker, repo.transactionMarker)
	}
}

func TestCommitCurrentUserAvatarRejectsCallerSelectedUser(t *testing.T) {
	service := newTestService(&fakeRepository{
		byID: map[int64]*domain.SubjectRecord{
			1002: {UserID: 1002, AccountName: "other", Status: 0},
		},
	}, nil)
	service.BindFileAssets(&fakeFileAssetBindingFacade{})
	if _, err := service.CommitCurrentUserAvatar(authenticatedUserContext(1001, 22), 1002, 9001); err == nil {
		t.Fatal("authenticated user must not select another avatar owner")
	}
}

func authenticatedUserContext(userID, orgID int64) context.Context {
	return securitycontext.WithUser(context.Background(), &securitycontext.UserContext{
		UserID:       userID,
		PrimaryOrgID: orgID,
		OrgIDs:       []int64{orgID},
	})
}

func TestUpdateSelfEmailRejectsDuplicate(t *testing.T) {
	repo := &fakeRepository{
		byID: map[int64]*domain.SubjectRecord{
			1001: {UserID: 1001, AccountName: "admin", Email: "old@example.com", Status: 0},
		},
		emailCount: 1,
	}
	service := newTestService(repo, nil)

	err := service.UpdateSelfEmail(context.Background(), userfacade.UpdateSelfEmailCommand{
		UserID:               1001,
		UserEmail:            "dup@example.com",
		ExpectedEmailVersion: 1,
	})
	if appErr := apperrors.From(err); appErr == nil || appErr.Message() != "邮箱已存在" {
		t.Fatalf("expected duplicate email rejection, got %v", err)
	}
}

func TestUpdateSelfEmailUpdatesRequestedValue(t *testing.T) {
	repo := &fakeRepository{
		byID: map[int64]*domain.SubjectRecord{
			1001: {UserID: 1001, AccountName: "admin", Email: "old@example.com", Status: 0},
		},
	}
	service := newTestService(repo, nil)

	err := service.UpdateSelfEmail(context.Background(), userfacade.UpdateSelfEmailCommand{
		UserID:               1001,
		UserEmail:            "new@example.com",
		ExpectedEmailVersion: 1,
	})
	if err != nil {
		t.Fatalf("update self email: %v", err)
	}
	if repo.updatedEmail != "new@example.com" {
		t.Fatalf("expected email updated, got %q", repo.updatedEmail)
	}
}

func TestUpdateSelfEmailRejectsStaleVersionWithoutChangingAddress(t *testing.T) {
	repo := &fakeRepository{
		byID: map[int64]*domain.SubjectRecord{
			1001: {UserID: 1001, AccountName: "admin", Email: "old@example.com", EmailVersion: 2, Status: 0},
		},
	}
	service := newTestService(repo, nil)

	err := service.UpdateSelfEmail(context.Background(), userfacade.UpdateSelfEmailCommand{
		UserID:               1001,
		UserEmail:            "new@example.com",
		ExpectedEmailVersion: 1,
	})
	appErr := apperrors.From(err)
	if appErr == nil {
		t.Fatalf("expected stable email version conflict, got %v", err)
	}
	details, _ := appErr.Details().(map[string]string)
	if details["reasonCode"] != "PROFILE_EMAIL_VERSION_CONFLICT" {
		t.Fatalf("expected stable email version conflict, got %v", err)
	}
	if repo.updatedEmail != "" {
		t.Fatalf("stale email update reached repository: %q", repo.updatedEmail)
	}
}

func TestUpdateSelfEmailRejectsConcurrentCompareAndSetLoss(t *testing.T) {
	repo := &fakeRepository{
		byID: map[int64]*domain.SubjectRecord{
			1001: {UserID: 1001, AccountName: "admin", Email: "old@example.com", EmailVersion: 3, Status: 0},
		},
		emailCompareAndSetSet:    true,
		emailCompareAndSetResult: false,
	}
	service := newTestService(repo, nil)

	err := service.UpdateSelfEmail(context.Background(), userfacade.UpdateSelfEmailCommand{
		UserID:               1001,
		UserEmail:            "new@example.com",
		ExpectedEmailVersion: 3,
	})
	appErr := apperrors.From(err)
	if appErr == nil {
		t.Fatalf("expected compare-and-set conflict, got %v", err)
	}
	details, _ := appErr.Details().(map[string]string)
	if details["reasonCode"] != "PROFILE_EMAIL_VERSION_CONFLICT" {
		t.Fatalf("expected compare-and-set conflict, got %v", err)
	}
	if repo.updatedEmail != "" {
		t.Fatalf("failed compare-and-set changed fake repository state: %q", repo.updatedEmail)
	}
}

func TestAssignUserRolesRequiresServiceProofMetadata(t *testing.T) {
	tests := []struct {
		name  string
		proof stepup.ProofMetadata
	}{
		{name: "missing proof"},
		{name: "wrong action", proof: validUserStepUpProof("CONFIG_APPLY_PENDING", "user:1001|roles:1,2")},
		{name: "wrong binding", proof: validUserStepUpProof(stepUpActionRBACAssignUserRoles, "user:1001|roles:2")},
		{name: "missing proof id", proof: stepup.ProofMetadata{BusinessAction: stepUpActionRBACAssignUserRoles, OperationBinding: "user:1001|roles:1,2"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := &fakeRepository{}
			service := newTestService(repo, nil)

			err := service.AssignUserRoles(context.Background(), userfacade.RelationAssignCommand{
				UserID:      1001,
				IDs:         []int64{2, 1, 2},
				OperatorID:  9001,
				StepUpProof: tt.proof,
			})
			if appErr := apperrors.From(err); appErr == nil || appErr.Code() != apperrors.CodeForbidden {
				t.Fatalf("expected permission denied proof rejection, got %v", err)
			}
			if repo.replaceUserRolesCalls != 0 {
				t.Fatalf("expected no role replacement on rejected proof, got %d calls", repo.replaceUserRolesCalls)
			}
		})
	}

	repo := &fakeRepository{}
	service := newTestService(repo, nil)
	err := service.AssignUserRoles(context.Background(), userfacade.RelationAssignCommand{
		UserID:      1001,
		IDs:         []int64{2, 1, 2},
		OperatorID:  9001,
		StepUpProof: validUserStepUpProof(stepUpActionRBACAssignUserRoles, "user:1001|roles:1,2"),
	})
	if err != nil {
		t.Fatalf("expected valid proof to allow user role assignment: %v", err)
	}
	if repo.replaceUserRolesCalls != 1 || repo.replaceUserRolesUserID != 1001 {
		t.Fatalf("expected one role replacement for user 1001, got calls=%d user=%d", repo.replaceUserRolesCalls, repo.replaceUserRolesUserID)
	}
}

func TestDeleteAdminUserRequiresServiceProofMetadata(t *testing.T) {
	tests := []struct {
		name  string
		proof stepup.ProofMetadata
	}{
		{name: "missing proof"},
		{name: "wrong action", proof: validUserStepUpProof(stepUpActionAdminChangeUserStatus, "user:1001|delete")},
		{name: "wrong binding", proof: validUserStepUpProof(stepUpActionAdminDeleteUser, "user:1002|delete")},
		{name: "missing proof id", proof: stepup.ProofMetadata{BusinessAction: stepUpActionAdminDeleteUser, OperationBinding: "user:1001|delete"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := &fakeRepository{}
			service := newTestService(repo, nil)

			err := service.DeleteAdminUser(context.Background(), userfacade.AdminUserDeleteCommand{
				UserID:      1001,
				OperatorID:  9001,
				StepUpProof: tt.proof,
			})
			if appErr := apperrors.From(err); appErr == nil || appErr.Code() != apperrors.CodeForbidden {
				t.Fatalf("expected permission denied proof rejection, got %v", err)
			}
			if repo.softDeleteCalls != 0 {
				t.Fatalf("expected no user delete on rejected proof, got %d calls", repo.softDeleteCalls)
			}
		})
	}

	repo := &fakeRepository{}
	service := newTestService(repo, nil)
	sessions := &fakeSessionFacade{}
	service.BindSessions(sessions)
	err := service.DeleteAdminUser(context.Background(), userfacade.AdminUserDeleteCommand{
		UserID:      1001,
		OperatorID:  9001,
		StepUpProof: validUserStepUpProof(stepUpActionAdminDeleteUser, "user:1001|delete"),
	})
	if err != nil {
		t.Fatalf("expected valid proof to allow user delete: %v", err)
	}
	if repo.softDeleteCalls != 1 || repo.softDeleteUserID != 1001 {
		t.Fatalf("expected one user delete for 1001, got calls=%d user=%d", repo.softDeleteCalls, repo.softDeleteUserID)
	}
	if sessions.revokedUserID != 1001 {
		t.Fatalf("expected transactional session revoke for deleted user, got %d", sessions.revokedUserID)
	}
}

func TestUpdateAdminUserStatusRequiresServiceProofMetadata(t *testing.T) {
	tests := []struct {
		name  string
		proof stepup.ProofMetadata
	}{
		{name: "missing proof"},
		{name: "wrong action", proof: validUserStepUpProof(stepUpActionAdminDeleteUser, "user:1001|status:1")},
		{name: "wrong binding", proof: validUserStepUpProof(stepUpActionAdminChangeUserStatus, "user:1001|status:0")},
		{name: "missing proof id", proof: stepup.ProofMetadata{BusinessAction: stepUpActionAdminChangeUserStatus, OperationBinding: "user:1001|status:1"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := &fakeRepository{}
			service := newTestService(repo, nil)

			err := service.UpdateAdminUserStatus(context.Background(), userfacade.AdminUserStatusCommand{
				UserID:      1001,
				Status:      1,
				OperatorID:  9001,
				StepUpProof: tt.proof,
			})
			if appErr := apperrors.From(err); appErr == nil || appErr.Code() != apperrors.CodeForbidden {
				t.Fatalf("expected permission denied proof rejection, got %v", err)
			}
			if repo.lockUserID != 0 {
				t.Fatalf("expected no status update on rejected proof, got user=%d status=%d", repo.lockUserID, repo.lockStatus)
			}
		})
	}

	repo := &fakeRepository{}
	service := newTestService(repo, nil)
	sessions := &fakeSessionFacade{}
	service.BindSessions(sessions)
	err := service.UpdateAdminUserStatus(context.Background(), userfacade.AdminUserStatusCommand{
		UserID:      1001,
		Status:      1,
		OperatorID:  9001,
		StepUpProof: validUserStepUpProof(stepUpActionAdminChangeUserStatus, "user:1001|status:1"),
	})
	if err != nil {
		t.Fatalf("expected valid proof to allow user status update: %v", err)
	}
	if repo.lockUserID != 1001 || repo.lockStatus != 1 {
		t.Fatalf("expected status update for user 1001 status 1, got user=%d status=%d", repo.lockUserID, repo.lockStatus)
	}

	err = service.UpdateAdminUserStatus(context.Background(), userfacade.AdminUserStatusCommand{
		UserID:      1002,
		Status:      2,
		OperatorID:  9001,
		StepUpProof: validUserStepUpProof(stepUpActionAdminChangeUserStatus, "user:1002|status:2"),
	})
	if err != nil {
		t.Fatalf("expected pending review status update to be allowed: %v", err)
	}
	if repo.lockUserID != 1002 || repo.lockStatus != 2 {
		t.Fatalf("expected status update for user 1002 status 2, got user=%d status=%d", repo.lockUserID, repo.lockStatus)
	}
	if sessions.revokedUserID != 1002 {
		t.Fatalf("expected transactional session revoke for last disabled user, got %d", sessions.revokedUserID)
	}
}

func TestCreateAdminUserWithRolesRequiresProofBeforeInfrastructureAccess(t *testing.T) {
	service := newTestService(&fakeRepository{}, nil)

	_, err := service.CreateAdminUser(context.Background(), userfacade.AdminUserCreateCommand{
		Username: "created",
		Nickname: "Created User",
		Password: "Secret123",
		RoleIDs:  []int64{2, 1, 2},
	})
	if appErr := apperrors.From(err); appErr == nil || appErr.Code() != apperrors.CodeForbidden {
		t.Fatalf("expected permission denied proof rejection before id generator/unique checks, got %v", err)
	}
}

func TestCreateAdminUserWithRolesRequiresSuperAdminOperator(t *testing.T) {
	repo := &fakeRepository{}
	service := newTestService(repo, nil)
	service.BindRoleAssignments(&fakeRoleAssignmentFacade{writer: repo, validationErr: apperrors.Forbidden("不能分配超级管理员角色")})

	_, err := service.CreateAdminUser(context.Background(), userfacade.AdminUserCreateCommand{
		Username:    "created",
		Nickname:    "Created User",
		Password:    "Secret123",
		OperatorID:  9001,
		RoleIDs:     []int64{2, 1, 2},
		StepUpProof: validUserStepUpProof(stepUpActionRBACAssignUserRoles, "user:create:created|roles:1,2"),
	})

	if appErr := apperrors.From(err); appErr == nil || appErr.Code() != apperrors.CodeForbidden {
		t.Fatalf("expected forbidden when non-super-admin creates user with roles, got %v", err)
	}
	if repo.created != nil {
		t.Fatalf("CreateAdminUser must not create role-bearing user for non-super-admin operator")
	}
}

func TestAssignPostRolesRequiresServiceProofMetadata(t *testing.T) {
	tests := []struct {
		name  string
		proof stepup.ProofMetadata
	}{
		{name: "missing proof"},
		{name: "wrong action", proof: validUserStepUpProof(stepUpActionRBACAssignUserRoles, "post:2001|roles:1,2")},
		{name: "wrong binding", proof: validUserStepUpProof(stepUpActionRBACAssignPostRoles, "post:2001|roles:2")},
		{name: "missing proof id", proof: stepup.ProofMetadata{BusinessAction: stepUpActionRBACAssignPostRoles, OperationBinding: "post:2001|roles:1,2"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := &fakeRepository{}
			service := newTestService(repo, nil)
			service.BindPermissions(fakePermissionFacade{validatePostRoleAssignment: true})

			err := service.AssignPostRoles(context.Background(), userfacade.PostRoleAssignCommand{
				PostID:      2001,
				RoleIDs:     []int64{2, 1, 2},
				OperatorID:  9001,
				StepUpProof: tt.proof,
			})
			if appErr := apperrors.From(err); appErr == nil || appErr.Code() != apperrors.CodeForbidden {
				t.Fatalf("expected permission denied proof rejection, got %v", err)
			}
			if repo.replacePostRolesCalls != 0 {
				t.Fatalf("expected no post role replacement on rejected proof, got %d calls", repo.replacePostRolesCalls)
			}
		})
	}

	repo := &fakeRepository{}
	service := newTestService(repo, nil)
	service.transactor = &avatarTestTransactor{}
	service.BindPermissions(fakePermissionFacade{validatePostRoleAssignment: true})
	err := service.AssignPostRoles(context.Background(), userfacade.PostRoleAssignCommand{
		PostID:      2001,
		RoleIDs:     []int64{2, 1, 2},
		OperatorID:  9001,
		StepUpProof: validUserStepUpProof(stepUpActionRBACAssignPostRoles, "post:2001|roles:1,2"),
	})
	if err != nil {
		t.Fatalf("expected valid proof to allow post role assignment: %v", err)
	}
	if repo.replacePostRolesCalls != 1 || repo.replacePostRolesPostID != 2001 {
		t.Fatalf("expected one post role replacement for post 2001, got calls=%d post=%d", repo.replacePostRolesCalls, repo.replacePostRolesPostID)
	}
}

func TestAssignPostRolesUsesOneBatchAuthorizationCheck(t *testing.T) {
	repo := &fakeRepository{}
	service := newTestService(repo, nil)
	service.transactor = &avatarTestTransactor{}
	calls := 0
	service.BindPermissions(fakePermissionFacade{validatePostRoleAssignment: true, validatePostRoleBatchCalls: &calls})

	err := service.AssignPostRoles(context.Background(), userfacade.PostRoleAssignCommand{
		PostID:      2001,
		RoleIDs:     []int64{3, 1, 2, 3},
		OperatorID:  9001,
		StepUpProof: validUserStepUpProof(stepUpActionRBACAssignPostRoles, "post:2001|roles:1,2,3"),
	})
	if err != nil {
		t.Fatalf("AssignPostRoles(): %v", err)
	}
	if calls != 1 {
		t.Fatalf("batch authorization calls=%d want=1", calls)
	}
}

func TestAssignPostRolesUsesRoleGuardBeforePostRoleWrite(t *testing.T) {
	var events []string
	repo := &fakeRepository{postRoleEvents: &events}
	service := newTestService(repo, nil)
	service.transactor = &avatarTestTransactor{}
	service.BindPermissions(fakePermissionFacade{
		validatePostRoleAssignment: true,
		postRoleEvents:             &events,
	})

	err := service.AssignPostRoles(context.Background(), userfacade.PostRoleAssignCommand{
		PostID:      2001,
		RoleIDs:     []int64{3, 1, 2},
		OperatorID:  9001,
		StepUpProof: validUserStepUpProof(stepUpActionRBACAssignPostRoles, "post:2001|roles:1,2,3"),
	})
	if err != nil {
		t.Fatalf("AssignPostRoles(): %v", err)
	}
	if got, want := fmt.Sprint(events), "[lock-and-validate-role-guard replace-post-roles]"; got != want {
		t.Fatalf("post role mutation order=%s, want %s", got, want)
	}
}

func TestAssignPostRolesRejectsRoleDeletedBeforeGuardAcquisition(t *testing.T) {
	guardStarted := make(chan struct{})
	deleteFinished := make(chan struct{})
	repo := &fakeRepository{}
	service := newTestService(repo, nil)
	service.transactor = &avatarTestTransactor{}
	service.BindPermissions(fakePermissionFacade{
		validatePostRoleAssignment: true,
		roleGuardStarted:           guardStarted,
		roleDeleteFinished:         deleteFinished,
		roleStillAssignable:        false,
	})
	go func() {
		<-guardStarted
		close(deleteFinished)
	}()

	err := service.AssignPostRoles(context.Background(), userfacade.PostRoleAssignCommand{
		PostID:      2001,
		RoleIDs:     []int64{1, 2},
		OperatorID:  9001,
		StepUpProof: validUserStepUpProof(stepUpActionRBACAssignPostRoles, "post:2001|roles:1,2"),
	})
	if err == nil || apperrors.From(err).Code() != apperrors.CodeForbidden {
		t.Fatalf("concurrent deleted role err=%v, want forbidden", err)
	}
	if repo.replacePostRolesCalls != 0 {
		t.Fatalf("post role write calls=%d after role was deleted before guard acquisition", repo.replacePostRolesCalls)
	}
}

func TestAssignPostRolesFailsClosedWithoutConsistentTransaction(t *testing.T) {
	repo := &fakeRepository{}
	service := newTestService(repo, nil)
	calls := 0
	service.BindPermissions(fakePermissionFacade{validatePostRoleAssignment: true, validatePostRoleBatchCalls: &calls})

	err := service.AssignPostRoles(context.Background(), userfacade.PostRoleAssignCommand{
		PostID:      2001,
		RoleIDs:     []int64{1, 2},
		OperatorID:  9001,
		StepUpProof: validUserStepUpProof(stepUpActionRBACAssignPostRoles, "post:2001|roles:1,2"),
	})
	if err == nil {
		t.Fatal("post role assignment unexpectedly ran without a consistent transaction")
	}
	if calls != 0 || repo.replacePostRolesCalls != 0 {
		t.Fatalf("post role assignment reached validation/write before transaction guard: validation=%d replace=%d",
			calls, repo.replacePostRolesCalls)
	}
}

func TestAssignPostRolesInvalidatesClassGenerationWithoutEnumeratingAffectedUsers(t *testing.T) {
	userIDs := make([]int64, 1001)
	for index := range userIDs {
		userIDs[index] = int64(index + 1)
	}
	repo := &fakeRepository{postUserIDs: userIDs}
	tx := &avatarTestTransactor{}
	service := newTestService(repo, nil)
	service.transactor = tx
	service.BindPermissions(fakePermissionFacade{
		validatePostRoleAssignment: true,
	})

	err := service.AssignPostRoles(context.Background(), userfacade.PostRoleAssignCommand{
		PostID:      2001,
		RoleIDs:     []int64{1, 2},
		OperatorID:  9001,
		StepUpProof: validUserStepUpProof(stepUpActionRBACAssignPostRoles, "post:2001|roles:1,2"),
	})
	if err != nil {
		t.Fatalf("AssignPostRoles(): %v", err)
	}
	if !tx.committed || repo.listPostUsersUnboundedCalls != 0 {
		t.Fatalf("post role write committed=%v unboundedReads=%d", tx.committed, repo.listPostUsersUnboundedCalls)
	}
	if repo.listPostUsersPageCalls != 0 {
		t.Fatalf("paged reads=%d, want zero: durable class invalidation must not enumerate users", repo.listPostUsersPageCalls)
	}
}

func TestCreatePostRequiresDepartmentBeforeRepositoryWrite(t *testing.T) {
	repo := &fakeRepository{}
	service := newTestService(repo, nil)

	err := service.CreatePost(context.Background(), userfacade.PostCommand{
		Code: "AUDITOR",
		Name: "审计岗位",
	})

	if appErr := apperrors.From(err); appErr == nil || appErr.Code() != apperrors.CodeParamsError {
		t.Fatalf("expected missing department to return params error, got %v", err)
	}
	if repo.createdPost != nil {
		t.Fatalf("missing department must not reach repository, got %#v", repo.createdPost)
	}
}

func TestUpdatePasswordUpdatesCredentialAndRevokesSessions(t *testing.T) {
	credentials := &fakeCredentialFacade{}
	sessions := &fakeSessionFacade{}
	service := newTestService(&fakeRepository{
		byID: map[int64]*domain.SubjectRecord{
			1001: {UserID: 1001, AccountName: "admin", Status: 0},
		},
	}, credentials)
	service.BindSessions(sessions)

	err := service.UpdatePassword(context.Background(), userfacade.UpdatePasswordCommand{
		UserID:      1001,
		RawPassword: "NewPass123",
		OperatorID:  1001,
	})
	if err != nil {
		t.Fatalf("update password: %v", err)
	}
	if credentials.lastUpsert == nil || credentials.lastUpsert.UserID != 1001 || credentials.lastUpsert.PasswordHash == "" {
		t.Fatalf("unexpected password upsert: %#v", credentials.lastUpsert)
	}
	if sessions.revokedUserID != 1001 {
		t.Fatalf("expected revoke sessions for user 1001, got %d", sessions.revokedUserID)
	}
}

func TestUpdateLockStateDelegatesToRepository(t *testing.T) {
	unsealAt := time.Now().UTC().Add(2 * time.Hour)
	repo := &fakeRepository{}
	service := newTestService(repo, nil)
	sessions := &fakeSessionFacade{}
	service.BindSessions(sessions)
	if err := service.UpdateLockState(context.Background(), userfacade.UpdateLockStateCommand{
		UserID:     1001,
		Status:     1,
		UnsealTime: &unsealAt,
	}); err != nil {
		t.Fatalf("update lock state: %v", err)
	}
	if repo.lockStatus != 1 || repo.lockUserID != 1001 || repo.lockUnsealAt == nil {
		t.Fatalf("unexpected lock update state: %#v %#v %#v", repo.lockUserID, repo.lockStatus, repo.lockUnsealAt)
	}
	if sessions.revokedUserID != 1001 {
		t.Fatalf("expected transactional session revoke for locked user, got %d", sessions.revokedUserID)
	}
}

func TestUserDeactivationPathsHonorSuperAdminInvariantGuard(t *testing.T) {
	guardErr := apperrors.Operation("必须保留至少一个有效超级管理员")
	newGuardedService := func(repo *fakeRepository) *Service {
		service := newTestService(repo, nil)
		service.BindRoleAssignments(&fakeRoleAssignmentFacade{writer: repo, guardErr: guardErr})
		return service
	}
	assertBlocked := func(t *testing.T, err error) {
		t.Helper()
		appErr := apperrors.From(err)
		if appErr == nil || appErr.Code() != apperrors.CodeOperateError {
			t.Fatalf("expected last SUPER_ADMIN operation error, got %v", err)
		}
	}

	t.Run("delete", func(t *testing.T) {
		repo := &fakeRepository{}
		service := newGuardedService(repo)
		err := service.DeleteAdminUser(context.Background(), userfacade.AdminUserDeleteCommand{
			UserID: 1001, OperatorID: 9001,
			StepUpProof: validUserStepUpProof(stepUpActionAdminDeleteUser, "user:1001|delete"),
		})
		assertBlocked(t, err)
		if repo.softDeleteCalls != 0 {
			t.Fatal("guarded user was deleted")
		}
	})

	t.Run("dedicated status command", func(t *testing.T) {
		repo := &fakeRepository{}
		service := newGuardedService(repo)
		err := service.UpdateAdminUserStatus(context.Background(), userfacade.AdminUserStatusCommand{
			UserID: 1001, Status: domain.UserStatusDisabled, OperatorID: 9001,
			StepUpProof: validUserStepUpProof(stepUpActionAdminChangeUserStatus, "user:1001|status:1"),
		})
		assertBlocked(t, err)
		if repo.lockUserID != 0 {
			t.Fatal("guarded user status was updated")
		}
	})

	t.Run("generic user update", func(t *testing.T) {
		disabled := domain.UserStatusDisabled
		repo := &fakeRepository{adminByID: map[int64]*domain.AdminUserRecord{
			1001: {ID: 1001, AccountName: "admin", NickName: "Admin", Status: domain.UserStatusNormal},
		}}
		service := newGuardedService(repo)
		err := service.UpdateAdminUser(context.Background(), userfacade.AdminUserUpdateCommand{
			ID: 1001, Username: "admin", Status: &disabled, OperatorID: 9001,
		})
		assertBlocked(t, err)
		if repo.updateAdminCalls != 0 {
			t.Fatal("generic update bypassed the SUPER_ADMIN guard")
		}
	})

	t.Run("login lock state", func(t *testing.T) {
		repo := &fakeRepository{}
		service := newGuardedService(repo)
		err := service.UpdateLockState(context.Background(), userfacade.UpdateLockStateCommand{
			UserID: 1001, Status: domain.UserStatusDisabled,
		})
		assertBlocked(t, err)
		if repo.lockUserID != 0 {
			t.Fatal("login lock state bypassed the SUPER_ADMIN guard")
		}
	})

	t.Run("managed status command", func(t *testing.T) {
		repo := &fakeRepository{}
		service := newGuardedService(repo)
		_, err := service.SetManagedUserStatus(context.Background(), userfacade.SetManagedUserStatusCommand{
			UserID: 1001, Status: domain.UserStatusDisabled, ExpectedStatus: domain.UserStatusNormal,
			ExpectedVersion: 1, Cutoff: time.Now().UTC(), StatusCommandHash: strings.Repeat("a", 64),
		})
		assertBlocked(t, err)
		if repo.managedStatusCalls != 0 {
			t.Fatal("managed status command bypassed the SUPER_ADMIN guard")
		}
	})
}

func TestVerifyPasswordUsesCredentialPasswordHash(t *testing.T) {
	service := newTestService(&fakeRepository{}, &fakeCredentialFacade{
		password: &credentialfacade.PasswordCredential{UserID: 1001, PasswordHash: mustHash(t, "OldPass123")},
	})

	ok, err := service.VerifyPassword(context.Background(), 1001, "OldPass123")
	if err != nil {
		t.Fatalf("verify password: %v", err)
	}
	if !ok {
		t.Fatal("expected password verify success")
	}
}

func TestCreateOwnerUserCreatesUserAndPasswordCredential(t *testing.T) {
	repo := &fakeRepository{}
	credentials := &fakeCredentialFacade{}
	idGen, err := xid.New(1)
	if err != nil {
		t.Fatalf("new id generator: %v", err)
	}
	passwordService, err := passwordinfra.New(config.PasswordConfig{
		Algorithm: "bcrypt",
		Bcrypt:    config.BcryptPasswordConfig{Cost: 4},
	})
	if err != nil {
		t.Fatalf("new password service: %v", err)
	}
	service := NewService(repo, domain.NewService(), passwordService, credentials, WithIDGenerator(idGen))

	owner, err := service.CreateOwnerUser(context.Background(), userfacade.CreateOwnerUserCommand{
		AccountName: "Owner1",
		NickName:    "",
		RawPassword: "Owner123",
	})
	if err != nil {
		t.Fatalf("create owner user: %v", err)
	}
	if owner == nil || owner.UserID <= 0 || owner.AccountName != "Owner1" || owner.NickName != "Owner1" {
		t.Fatalf("unexpected owner: %#v", owner)
	}
	if repo.created == nil || repo.created.UserID != owner.UserID || repo.created.Status != 0 || repo.created.Gender != 0 {
		t.Fatalf("unexpected created record: %#v", repo.created)
	}
	if credentials.lastUpsert == nil || credentials.lastUpsert.UserID != owner.UserID || credentials.lastUpsert.PasswordHash == "" {
		t.Fatalf("expected password credential upsert, got %#v", credentials.lastUpsert)
	}
}

func newTestService(repo domain.Repository, credentials credentialfacade.UserCredentialFacade) *Service {
	passwordService, err := passwordinfra.New(config.PasswordConfig{
		Algorithm: "bcrypt",
		Bcrypt:    config.BcryptPasswordConfig{Cost: 4},
	})
	if err != nil {
		panic(err)
	}
	service := NewService(repo, domain.NewService(), passwordService, credentials)
	if writer, ok := repo.(fakeUserRoleWriter); ok {
		service.BindRoleAssignments(&fakeRoleAssignmentFacade{writer: writer})
	}
	return service
}

func mustHash(t *testing.T, raw string) string {
	t.Helper()
	passwordService, err := passwordinfra.New(config.PasswordConfig{
		Algorithm: "bcrypt",
		Bcrypt:    config.BcryptPasswordConfig{Cost: 4},
	})
	if err != nil {
		t.Fatalf("new password service: %v", err)
	}
	value, err := passwordService.Hash(context.Background(), raw)
	if err != nil {
		t.Fatalf("hash password: %v", err)
	}
	return value
}

func stringPointer(value string) *string {
	return &value
}

func validUserStepUpProof(action, binding string) stepup.ProofMetadata {
	return stepup.ProofMetadata{
		BusinessAction:        action,
		OperationBinding:      binding,
		ProofIdentifier:       "proof-jti",
		ChallengeIdentifier:   "challenge-id",
		AssuranceLevel:        "AAL2",
		AuthenticationMethods: []string{"TOTP"},
	}
}

func TestBatchDeletePostsFailsClosedWithoutTransaction(t *testing.T) {
	repo := &fakeRepository{}
	service := NewService(repo, domain.NewService(), nil, nil)
	err := service.BatchDeletePosts(context.Background(), []int64{10, 20})
	if err == nil {
		t.Fatal("expected batch post delete to reject missing transaction")
	}
	if repo.listReferencedPostCalls != 0 || repo.deletePostsCalls != 0 {
		t.Fatalf("batch post delete ran without transaction: refs=%d deletes=%d", repo.listReferencedPostCalls, repo.deletePostsCalls)
	}
}

func TestGetAdminUserRequiresSnapshotAndPropagatesRelationErrors(t *testing.T) {
	repo := &fakeRepository{
		adminByID:       map[int64]*domain.AdminUserRecord{7: {ID: 7}},
		listUserRoleErr: errors.New("role relation read failed"),
	}
	if _, err := NewService(repo, domain.NewService(), nil, nil).GetAdminUser(context.Background(), 7); err == nil {
		t.Fatal("admin user aggregate unexpectedly ran without snapshot")
	}
	service := NewService(repo, domain.NewService(), nil, nil, WithTransactor(&avatarTestTransactor{}))
	if _, err := service.GetAdminUser(context.Background(), 7); !errors.Is(err, repo.listUserRoleErr) {
		t.Fatalf("relation error was not propagated: %v", err)
	}
}

type fakeRepository struct {
	byID                        map[int64]*domain.SubjectRecord
	byAccount                   map[string]*domain.SubjectRecord
	byEmail                     map[string]*domain.SubjectRecord
	emailCount                  int
	emailLookupErr              error
	emailLookupArg              string
	countByEmailCalls           int
	emailLookupCount            int
	phoneCount                  int
	updatedNick                 *string
	updatedPhone                *string
	updatedAvatar               *string
	updateProfileErr            error
	transactionMarker           string
	updatedEmail                string
	emailCompareAndSetSet       bool
	emailCompareAndSetResult    bool
	lockUserID                  int64
	lockStatus                  int
	lockUnsealAt                *time.Time
	created                     *domain.OwnerUserRecord
	createdExternal             *domain.ExternalSubjectCreateRecord
	createdForm                 *domain.FormSubjectCreateRecord
	softDeleteCalls             int
	listReferencedPostCalls     int
	deletePostsCalls            int
	softDeleteUserID            int64
	replaceUserRolesCalls       int
	replaceUserRolesUserID      int64
	replaceUserRoleIDs          []int64
	replaceUserOrgsCalls        int
	replaceUserOrgsUserID       int64
	replaceUserOrgIDs           []int64
	replaceUserOrgPrimaryID     int64
	listUserOrgIDs              []int64
	createdOrg                  *domain.OrgRecord
	replaceUserDeptsCalls       int
	replaceUserDeptsUserID      int64
	replaceUserDeptIDs          []int64
	replaceUserDeptPrimaryID    int64
	replaceUserPostsCalls       int
	replaceUserPostsUserID      int64
	replaceUserPostIDs          []int64
	replacePostRolesCalls       int
	replacePostRolesPostID      int64
	adminByID                   map[int64]*domain.AdminUserRecord
	updateAdminCalls            int
	managedStatusCalls          int
	createdPost                 *domain.PostRecord
	selectorRecords             []domain.UserSelectorRecord
	selectorRecord              *domain.UserSelectorRecord
	selectorQuery               domain.UserSelectorQuery
	selectorUserID              int64
	selectorScope               domain.DataScopeFilter
	listUserRoleErr             error
	postUserIDs                 []int64
	listPostUsersUnboundedCalls int
	listPostUsersPageCalls      int
	postRoleEvents              *[]string
}

func (f *fakeRepository) ListReferencedPostIDs(context.Context, []int64) ([]int64, error) {
	f.listReferencedPostCalls++
	return []int64{}, nil
}

func (f *fakeRepository) DeletePosts(context.Context, []int64) error {
	f.deletePostsCalls++
	return nil
}

func (f *fakeRepository) FindSubjectByID(_ context.Context, userID int64) (*domain.SubjectRecord, error) {
	if f.createdExternal != nil && f.createdExternal.ID == userID {
		return &domain.SubjectRecord{
			UserID:      f.createdExternal.ID,
			AccountName: f.createdExternal.AccountName,
			Email:       f.createdExternal.Email,
			Status:      f.createdExternal.Status,
		}, nil
	}
	return f.byID[userID], nil
}

func (f *fakeRepository) FindSubjectByAccount(_ context.Context, account string) (*domain.SubjectRecord, error) {
	if f.byAccount == nil {
		return nil, nil
	}
	return f.byAccount[account], nil
}

func (f *fakeRepository) FindSubjectByEmail(_ context.Context, email string) (*domain.SubjectRecord, error) {
	f.emailLookupCount++
	f.emailLookupArg = email
	if f.emailLookupErr != nil {
		return nil, f.emailLookupErr
	}
	if f.byEmail == nil {
		return nil, nil
	}
	return f.byEmail[email], nil
}

func (f *fakeRepository) ExistsByID(_ context.Context, userID int64) (bool, error) {
	_, ok := f.byID[userID]
	return ok, nil
}

func (f *fakeRepository) CountByPhoneExcludingUserID(context.Context, int64, string) (int, error) {
	return f.phoneCount, nil
}

func (f *fakeRepository) CountByEmailExcludingUserID(context.Context, int64, string) (int, error) {
	return f.emailCount, nil
}

func (f *fakeRepository) CountByEmail(context.Context, string) (int, error) {
	f.countByEmailCalls++
	return f.emailCount, nil
}

func (f *fakeRepository) CountByAccountExcludingUserID(context.Context, int64, string) (int, error) {
	return 0, nil
}

func (f *fakeRepository) CreateOwnerUser(_ context.Context, record *domain.OwnerUserRecord) error {
	copied := *record
	f.created = &copied
	return nil
}

func (f *fakeRepository) CreateFormSubject(_ context.Context, record domain.FormSubjectCreateRecord) error {
	copied := record
	if copied.ID == 0 {
		copied.ID = 3001
	}
	f.createdForm = &copied
	return nil
}

func (f *fakeRepository) CreateExternalSubject(_ context.Context, record domain.ExternalSubjectCreateRecord) error {
	copied := record
	f.createdExternal = &copied
	return nil
}

func (f *fakeRepository) UpdateProfile(ctx context.Context, _ int64, nickName, phone, avatar, _ *string) error {
	f.updatedNick = nickName
	f.updatedPhone = phone
	f.updatedAvatar = avatar
	f.transactionMarker, _ = ctx.Value(avatarTransactionKey{}).(string)
	return f.updateProfileErr
}

func (f *fakeRepository) UpdateEmail(_ context.Context, _ int64, email string) error {
	f.updatedEmail = email
	return nil
}

func (f *fakeRepository) CompareAndSetVerifiedEmail(_ context.Context, _ int64, email string, expectedVersion int64, _ string, _ time.Time) (bool, error) {
	if expectedVersion <= 0 {
		return false, nil
	}
	if f.emailCompareAndSetSet {
		if !f.emailCompareAndSetResult {
			return false, nil
		}
	}
	f.updatedEmail = email
	return true, nil
}

func (f *fakeRepository) UpdateLockState(_ context.Context, userID int64, status int, unsealAt *time.Time) error {
	f.lockUserID = userID
	f.lockStatus = status
	f.lockUnsealAt = unsealAt
	return nil
}

func (f *fakeRepository) CompareAndSetManagedUserStatus(context.Context, int64, int, uint64, int, *time.Time, string) (bool, error) {
	f.managedStatusCalls++
	return true, nil
}

func (f *fakeRepository) QueryAdminUsers(context.Context, domain.AdminUserQuery) ([]domain.AdminUserRecord, int64, error) {
	return nil, 0, nil
}
func (f *fakeRepository) ListUserOptions(_ context.Context, query domain.UserSelectorQuery) ([]domain.UserSelectorRecord, error) {
	f.selectorQuery = query
	return f.selectorRecords, nil
}
func (f *fakeRepository) FindVisibleUserOptionByID(_ context.Context, userID int64, scope domain.DataScopeFilter) (*domain.UserSelectorRecord, error) {
	f.selectorUserID = userID
	f.selectorScope = scope
	return f.selectorRecord, nil
}
func (f *fakeRepository) FindAdminUserByID(_ context.Context, userID int64) (*domain.AdminUserRecord, error) {
	return f.adminByID[userID], nil
}
func (f *fakeRepository) CreateAdminUser(context.Context, domain.AdminUserCreateRecord) error {
	return nil
}

func (f *fakeRepository) UpdateAdminUser(context.Context, domain.AdminUserUpdateRecord) error {
	f.updateAdminCalls++
	return nil
}
func (f *fakeRepository) SoftDeleteUser(_ context.Context, userID int64, _ int64) error {
	f.softDeleteCalls++
	f.softDeleteUserID = userID
	return nil
}
func (f *fakeRepository) ReplaceUserRoles(_ context.Context, userID int64, roleIDs []int64, _ int64) error {
	f.replaceUserRolesCalls++
	f.replaceUserRolesUserID = userID
	f.replaceUserRoleIDs = append([]int64(nil), roleIDs...)
	return nil
}
func (f *fakeRepository) ReplaceUserOrgs(_ context.Context, userID int64, orgIDs []int64, primaryOrgID int64, _ int64) error {
	f.replaceUserOrgsCalls++
	f.replaceUserOrgsUserID = userID
	f.replaceUserOrgIDs = append([]int64(nil), orgIDs...)
	f.replaceUserOrgPrimaryID = primaryOrgID
	return nil
}
func (f *fakeRepository) ReplaceUserDepts(_ context.Context, userID int64, deptIDs []int64, primaryDeptID int64, _ int64) error {
	f.replaceUserDeptsCalls++
	f.replaceUserDeptsUserID = userID
	f.replaceUserDeptIDs = append([]int64(nil), deptIDs...)
	f.replaceUserDeptPrimaryID = primaryDeptID
	return nil
}
func (f *fakeRepository) ReplaceUserPosts(_ context.Context, userID int64, postIDs []int64, _ int64, _ int64) error {
	f.replaceUserPostsCalls++
	f.replaceUserPostsUserID = userID
	f.replaceUserPostIDs = append([]int64(nil), postIDs...)
	return nil
}
func (f *fakeRepository) ListUserRoleIDs(context.Context, int64) ([]int64, error) {
	return nil, f.listUserRoleErr
}
func (f *fakeRepository) ListUserOrgIDs(context.Context, int64) ([]int64, error) {
	return append([]int64(nil), f.listUserOrgIDs...), nil
}
func (f *fakeRepository) ListUserDeptIDs(context.Context, int64) ([]int64, error) { return nil, nil }
func (f *fakeRepository) ListUserPostIDs(context.Context, int64) ([]int64, error) { return nil, nil }
func (f *fakeRepository) ListActiveUserIDsByRoleID(context.Context, int64) ([]int64, error) {
	return nil, nil
}
func (f *fakeRepository) ListActiveUserIDsByRoleIDPage(context.Context, int64, int64, int) ([]int64, error) {
	return nil, nil
}
func (f *fakeRepository) CreateOrg(_ context.Context, record domain.OrgRecord, _ int64) error {
	f.createdOrg = &record
	return nil
}
func (f *fakeRepository) UpdateOrg(context.Context, domain.OrgRecord, int64) error { return nil }
func (f *fakeRepository) DeleteOrg(context.Context, int64) error                   { return nil }
func (f *fakeRepository) UpdateOrgStatus(context.Context, int64, int, int64) error { return nil }
func (f *fakeRepository) UpdateOrgParent(context.Context, int64, int64, int64) error {
	return nil
}
func (f *fakeRepository) FindOrgByID(context.Context, int64) (*domain.OrgRecord, error) {
	return nil, nil
}
func (f *fakeRepository) FindOrgsByIDs(context.Context, []int64) ([]domain.OrgRecord, error) {
	return nil, nil
}
func (f *fakeRepository) FindOrgByCode(context.Context, string) (*domain.OrgRecord, error) {
	return nil, nil
}
func (f *fakeRepository) FindOrgByUserID(context.Context, int64) (*domain.OrgRecord, error) {
	return nil, nil
}
func (f *fakeRepository) ListOrgs(context.Context, bool) ([]domain.OrgRecord, error) {
	return nil, nil
}
func (f *fakeRepository) ListOrgChildren(context.Context, int64) ([]domain.OrgRecord, error) {
	return nil, nil
}
func (f *fakeRepository) CountOrgCodeExcludingID(context.Context, int64, string) (int, error) {
	return 0, nil
}
func (f *fakeRepository) CountOrgChildren(context.Context, int64) (int, error)       { return 0, nil }
func (f *fakeRepository) CountDeptByOrgID(context.Context, int64) (int, error)       { return 0, nil }
func (f *fakeRepository) CountUserOrgByOrgID(context.Context, int64) (int, error)    { return 0, nil }
func (f *fakeRepository) CreateDept(context.Context, domain.DeptRecord, int64) error { return nil }
func (f *fakeRepository) UpdateDept(context.Context, domain.DeptRecord, int64) error { return nil }
func (f *fakeRepository) DeleteDept(context.Context, int64) error                    { return nil }
func (f *fakeRepository) FindDeptByID(context.Context, int64) (*domain.DeptRecord, error) {
	return nil, nil
}
func (f *fakeRepository) FindDeptsByIDs(context.Context, []int64) ([]domain.DeptRecord, error) {
	return nil, nil
}
func (f *fakeRepository) ListDepts(context.Context, bool, string, int64, *int, int) ([]domain.DeptRecord, error) {
	return nil, nil
}
func (f *fakeRepository) ListChildDeptIDs(context.Context, int64) ([]int64, error) { return nil, nil }
func (f *fakeRepository) CountDeptNameUnderParent(context.Context, int64, int64, string) (int, error) {
	return 0, nil
}
func (f *fakeRepository) CountDeptChildren(context.Context, int64) (int, error)     { return 0, nil }
func (f *fakeRepository) CountUserDeptByDeptID(context.Context, int64) (int, error) { return 0, nil }
func (f *fakeRepository) QueryPosts(context.Context, domain.PostQuery) ([]domain.PostRecord, int64, error) {
	return nil, 0, nil
}
func (f *fakeRepository) ListEnabledPosts(context.Context) ([]domain.PostRecord, error) {
	return nil, nil
}
func (f *fakeRepository) FindPostByID(context.Context, int64) (*domain.PostRecord, error) {
	return nil, nil
}
func (f *fakeRepository) FindPostsByIDs(context.Context, []int64) ([]domain.PostRecord, error) {
	return nil, nil
}
func (f *fakeRepository) CreatePost(_ context.Context, record domain.PostRecord, _ int64) error {
	copy := record
	f.createdPost = &copy
	return nil
}
func (f *fakeRepository) UpdatePost(context.Context, domain.PostRecord, int64) error { return nil }
func (f *fakeRepository) DeletePost(context.Context, int64) error                    { return nil }
func (f *fakeRepository) UpdatePostStatus(context.Context, int64, int, int64) error  { return nil }
func (f *fakeRepository) CountPostCodeExcludingID(context.Context, int64, string) (int, error) {
	return 0, nil
}
func (f *fakeRepository) CountPostNameExcludingID(context.Context, int64, string) (int, error) {
	return 0, nil
}
func (f *fakeRepository) CountUserPostByPostID(context.Context, int64) (int, error) { return 0, nil }
func (f *fakeRepository) ListPostRoleIDs(context.Context, int64) ([]int64, error)   { return nil, nil }
func (f *fakeRepository) ReplacePostRoles(_ context.Context, postID int64, _ []int64, _ int64) error {
	f.replacePostRolesCalls++
	f.replacePostRolesPostID = postID
	if f.postRoleEvents != nil {
		*f.postRoleEvents = append(*f.postRoleEvents, "replace-post-roles")
	}
	return nil
}
func (f *fakeRepository) ListPostIDsByRoleID(context.Context, int64) ([]int64, error) {
	return nil, nil
}

type fakePermissionFacade struct {
	authorizationfacade.PermissionFacade
	hasSuperAdmin              bool
	validatePostRoleAssignment bool
	validatePostRoleBatchCalls *int
	refreshedUserBatches       *[][]int64
	postRoleEvents             *[]string
	roleGuardStarted           chan struct{}
	roleDeleteFinished         chan struct{}
	roleStillAssignable        bool
}

type avatarTransactionKey struct{}

type avatarTestTransactor struct {
	committed  bool
	rolledBack bool
}

var _ store.Transactor = (*avatarTestTransactor)(nil)

func (t *avatarTestTransactor) Enabled() bool { return true }

func (t *avatarTestTransactor) WithinTransaction(ctx context.Context, fn func(context.Context) error) error {
	txCtx := context.WithValue(ctx, avatarTransactionKey{}, "avatar-transaction")
	if err := fn(txCtx); err != nil {
		t.rolledBack = true
		return err
	}
	t.committed = true
	return nil
}

func (t *avatarTestTransactor) WithinConsistentTransaction(ctx context.Context, fn func(context.Context) error) error {
	return t.WithinTransaction(ctx, fn)
}

func (t *avatarTestTransactor) WithinReadOnlySnapshot(ctx context.Context, fn func(context.Context) error) error {
	return t.WithinTransaction(ctx, fn)
}

type fakeFileAssetBindingFacade struct {
	result            *filefacade.FileReferenceDTO
	err               error
	transactionMarker string
}

func (f *fakeFileAssetBindingFacade) BindUploadedFile(ctx context.Context, _ filefacade.BindUploadedFileCommand) (*filefacade.FileReferenceDTO, error) {
	f.transactionMarker, _ = ctx.Value(avatarTransactionKey{}).(string)
	return f.result, f.err
}

type fakeUserRoleWriter interface {
	ReplaceUserRoles(ctx context.Context, userID int64, roleIDs []int64, operatorID int64) error
}

type fakeRoleAssignmentFacade struct {
	writer        fakeUserRoleWriter
	validationErr error
	guardErr      error
}

func (f *fakeRoleAssignmentFacade) AssignUserRoles(ctx context.Context, command authorizationfacade.AssignUserRolesCommand) error {
	roleIDs := normalizeIDs([]int64(command.RoleIDs))
	if err := stepup.Require(command.StepUpProof, stepUpActionRBACAssignUserRoles, fmt.Sprintf("user:%d|roles:%s", command.UserID, joinSortedIDs(roleIDs))); err != nil {
		return err
	}
	if f.validationErr != nil {
		return f.validationErr
	}
	return f.writer.ReplaceUserRoles(ctx, command.UserID, roleIDs, command.OperatorID)
}

func (f *fakeRoleAssignmentFacade) ValidateCreatedUserRoles(_ context.Context, command authorizationfacade.AssignCreatedUserRolesCommand) error {
	roleIDs := normalizeIDs([]int64(command.RoleIDs))
	if err := stepup.Require(command.StepUpProof, stepUpActionRBACAssignUserRoles, fmt.Sprintf("user:create:%s|roles:%s", command.Username, joinSortedIDs(roleIDs))); err != nil {
		return err
	}
	return f.validationErr
}

func (f *fakeRoleAssignmentFacade) AssignCreatedUserRoles(ctx context.Context, command authorizationfacade.AssignCreatedUserRolesCommand) error {
	if err := f.ValidateCreatedUserRoles(ctx, command); err != nil {
		return err
	}
	return f.writer.ReplaceUserRoles(ctx, command.UserID, normalizeIDs([]int64(command.RoleIDs)), command.OperatorID)
}

func (f *fakeRoleAssignmentFacade) AssignProvisionedUserRoles(ctx context.Context, command authorizationfacade.AssignProvisionedUserRolesCommand) error {
	if f.validationErr != nil {
		return f.validationErr
	}
	return f.writer.ReplaceUserRoles(ctx, command.UserID, normalizeIDs([]int64(command.RoleIDs)), 0)
}

func (f *fakeRoleAssignmentFacade) BootstrapOwnerRoles(ctx context.Context, command authorizationfacade.BootstrapOwnerRolesCommand) error {
	return f.writer.ReplaceUserRoles(ctx, command.UserID, normalizeIDs([]int64(command.RoleIDs)), command.OperatorID)
}

func (f *fakeRoleAssignmentFacade) GuardUserDeactivation(context.Context, int64) error {
	return f.guardErr
}

func (f *fakeRoleAssignmentFacade) IsAuthorizationRootUser(context.Context, int64) (bool, error) {
	return f.validationErr == nil, nil
}

func (f fakePermissionFacade) HasRole(context.Context, int64, string) (bool, error) {
	return f.hasSuperAdmin, nil
}

func (f fakePermissionFacade) ValidatePostRoleAssignment(context.Context, int64, int64, int64) (bool, error) {
	return f.validatePostRoleAssignment, nil
}
func (f fakePermissionFacade) ValidatePostRoleAssignments(context.Context, int64, int64, []int64) (bool, error) {
	if f.validatePostRoleBatchCalls != nil {
		*f.validatePostRoleBatchCalls++
	}
	if f.roleGuardStarted != nil {
		close(f.roleGuardStarted)
		<-f.roleDeleteFinished
	}
	return f.validatePostRoleAssignment, nil
}
func (f fakePermissionFacade) LockAndValidatePostRoleAssignments(context.Context, int64, int64, []int64) (bool, error) {
	if f.validatePostRoleBatchCalls != nil {
		*f.validatePostRoleBatchCalls++
	}
	if f.postRoleEvents != nil {
		*f.postRoleEvents = append(*f.postRoleEvents, "lock-and-validate-role-guard")
	}
	if f.roleGuardStarted != nil {
		close(f.roleGuardStarted)
		<-f.roleDeleteFinished
		return f.roleStillAssignable, nil
	}
	return f.validatePostRoleAssignment, nil
}
func (f fakePermissionFacade) RefreshUsersPermissionCache(_ context.Context, userIDs []int64) error {
	if f.refreshedUserBatches != nil {
		*f.refreshedUserBatches = append(*f.refreshedUserBatches, append([]int64(nil), userIDs...))
	}
	return nil
}
func (f *fakeRepository) ListUserIDsByPostID(context.Context, int64) ([]int64, error) {
	f.listPostUsersUnboundedCalls++
	return append([]int64(nil), f.postUserIDs...), nil
}
func (f *fakeRepository) ListUserIDsByPostIDPage(_ context.Context, _ int64, afterUserID int64, limit int) ([]int64, error) {
	f.listPostUsersPageCalls++
	result := make([]int64, 0, limit)
	for _, userID := range f.postUserIDs {
		if userID <= afterUserID {
			continue
		}
		result = append(result, userID)
		if len(result) == limit {
			break
		}
	}
	return result, nil
}

type fakeSessionFacade struct {
	revokedUserID int64
	revokeErr     error
}

func (f *fakeSessionFacade) ListSessionsByUserID(context.Context, int64) ([]ssofacade.SessionRecord, error) {
	return nil, nil
}

func (f *fakeSessionFacade) ListActiveSessions(context.Context) ([]ssofacade.SessionRecord, error) {
	return nil, nil
}

func (f *fakeSessionFacade) CountActiveSessions(context.Context) (int64, error) {
	return 0, nil
}

func (f *fakeSessionFacade) RevokeSession(context.Context, string) (bool, error) {
	return false, nil
}

func (f *fakeSessionFacade) RevokeSessionsByUserID(_ context.Context, userID int64) (int64, error) {
	f.revokedUserID = userID
	return 0, f.revokeErr
}

func (f *fakeSessionFacade) RevokeSessionsByPlatformCode(context.Context, string) (int64, error) {
	return 0, nil
}

func (f *fakeSessionFacade) RevokeSessionsByPlatformLoginMethod(context.Context, string, string, string) (int64, error) {
	return 0, nil
}

func (f *fakeSessionFacade) RevokeSessionsByExternalProvider(context.Context, string) (int64, error) {
	return 0, nil
}

func (f *fakeSessionFacade) RevokeSessionsByExternalIdentity(context.Context, int64) (int64, error) {
	return 0, nil
}

func (f *fakeSessionFacade) ResolveActiveSessionRecord(context.Context, string) (*ssofacade.SessionRecord, error) {
	return nil, nil
}

func (f *fakeSessionFacade) ValidateActiveSession(context.Context, int64, int64) error {
	return nil
}

type fakeCredentialFacade struct {
	password   *credentialfacade.PasswordCredential
	lastUpsert *credentialfacade.UpsertPasswordCredentialCommand
}

func (f *fakeCredentialFacade) FindActivePasswordByUserID(context.Context, int64) (*credentialfacade.PasswordCredential, error) {
	return f.password, nil
}
func (f *fakeCredentialFacade) UpsertPasswordCredential(_ context.Context, command credentialfacade.UpsertPasswordCredentialCommand) error {
	copied := command
	f.lastUpsert = &copied
	return nil
}
func (f *fakeCredentialFacade) MarkPasswordUsed(context.Context, int64, time.Time) error {
	return nil
}
func (f *fakeCredentialFacade) FindActiveTotpByUserID(context.Context, int64) (*credentialfacade.TotpCredential, error) {
	return nil, nil
}
func (f *fakeCredentialFacade) FindActiveTotpSecretByUserID(context.Context, int64) (*credentialfacade.TotpSecret, error) {
	return nil, nil
}
func (f *fakeCredentialFacade) UpsertTotpCredential(context.Context, credentialfacade.UpsertTotpCredentialCommand) error {
	return nil
}
func (f *fakeCredentialFacade) CompleteTotpBinding(context.Context, credentialfacade.CompleteTotpBindingCommand) error {
	return nil
}
func (f *fakeCredentialFacade) DisableTotpCredential(context.Context, int64) (bool, error) {
	return false, nil
}
func (f *fakeCredentialFacade) MarkTotpUsed(context.Context, int64, time.Time) error {
	return nil
}
func (f *fakeCredentialFacade) ListActivePasskeys(context.Context, int64) ([]credentialfacade.PasskeyCredential, error) {
	return nil, nil
}
func (f *fakeCredentialFacade) FindActivePasskeyByCredentialKey(context.Context, string) (*credentialfacade.PasskeyCredential, error) {
	return nil, nil
}
func (f *fakeCredentialFacade) SavePasskeyCredential(context.Context, credentialfacade.SavePasskeyCredentialCommand) error {
	return nil
}
func (f *fakeCredentialFacade) CompletePasskeyBinding(context.Context, credentialfacade.CompletePasskeyBindingCommand) error {
	return nil
}
func (f *fakeCredentialFacade) DisablePasskeyCredential(context.Context, int64, string) (bool, error) {
	return false, nil
}
func (f *fakeCredentialFacade) UpdatePasskeyUsage(context.Context, string, int64, time.Time) error {
	return nil
}
func (f *fakeCredentialFacade) CountAvailableRecoveryCodes(context.Context, int64) (int, error) {
	return 0, nil
}
func (f *fakeCredentialFacade) RegenerateRecoveryCodes(context.Context, int64, int) (*credentialfacade.RegeneratedRecoveryCodes, error) {
	return nil, nil
}
func (f *fakeCredentialFacade) ConsumeRecoveryCode(context.Context, int64, string, time.Time) (bool, error) {
	return false, nil
}

var _ ssofacade.SessionFacade = (*fakeSessionFacade)(nil)
