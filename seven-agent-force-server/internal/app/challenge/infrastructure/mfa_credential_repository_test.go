package infrastructure

import (
	"context"
	"testing"
	"time"

	"github.com/CaixyPromise/seven-agent/force/internal/app/challenge/domain"
	credentialfacade "github.com/CaixyPromise/seven-agent/force/internal/app/credential/facade"
	userfacade "github.com/CaixyPromise/seven-agent/force/internal/app/system/user/facade"
)

func TestMfaCredentialStoreUsesUserPrefixSubjectIdentifier(t *testing.T) {
	store := NewMfaCredentialStore(&fakeCredentialFacade{
		totp: &credentialfacade.TotpCredential{
			UserID:           1001,
			SecretCiphertext: "{\"kid\":\"v1\"}",
		},
	}, NewSubjectResolver(&fakeUserFacade{
		byID: map[int64]*userfacade.SubjectRecord{
			1001: {UserID: 1001, AccountName: "alice"},
		},
	}))

	record, err := store.FindEnabledOtpBinding(context.Background(), &domain.ChallengeSession{SubjectIdentifier: "user:1001"})
	if err != nil {
		t.Fatalf("find enabled otp binding: %v", err)
	}
	if record == nil || record.UserID != 1001 {
		t.Fatalf("unexpected record: %#v", record)
	}
}

func TestMfaCredentialStoreConsumeRecoveryCodeUsesUserPrefixSubjectIdentifier(t *testing.T) {
	store := NewMfaCredentialStore(&fakeCredentialFacade{consumeOK: true}, NewSubjectResolver(&fakeUserFacade{
		byID: map[int64]*userfacade.SubjectRecord{
			1001: {UserID: 1001, AccountName: "alice"},
		},
	}))

	ok, err := store.ConsumeRecoveryCode(context.Background(), &domain.ChallengeSession{SubjectIdentifier: "user:1001"}, "ABCD-EFGH-IJKL", time.Now())
	if err != nil {
		t.Fatalf("consume recovery code: %v", err)
	}
	if !ok {
		t.Fatal("expected recovery code consume to succeed")
	}
}

func TestMfaCredentialStoreCountAvailableRecoveryCodesUsesUserPrefixSubjectIdentifier(t *testing.T) {
	facade := &fakeCredentialFacade{recoveryCount: 3}
	store := NewMfaCredentialStore(facade, NewSubjectResolver(&fakeUserFacade{
		byID: map[int64]*userfacade.SubjectRecord{
			1001: {UserID: 1001, AccountName: "alice"},
		},
	}))

	count, err := store.CountAvailableRecoveryCodes(context.Background(), &domain.ChallengeSession{SubjectIdentifier: "user:1001"})
	if err != nil {
		t.Fatalf("count recovery codes: %v", err)
	}
	if count != 3 {
		t.Fatalf("unexpected recovery code count: %d", count)
	}
	if facade.countRecoveryUserID != 1001 {
		t.Fatalf("unexpected counted user id: %d", facade.countRecoveryUserID)
	}
}

func TestMfaCredentialStoreFindEnabledOtpBindingReturnsNilForUnknownLoginSubject(t *testing.T) {
	store := NewMfaCredentialStore(&fakeCredentialFacade{}, NewSubjectResolver(&fakeUserFacade{}))
	record, err := store.FindEnabledOtpBinding(context.Background(), &domain.ChallengeSession{SubjectIdentifier: "login:missing"})
	if err != nil {
		t.Fatalf("find enabled otp binding: %v", err)
	}
	if record != nil {
		t.Fatalf("expected nil record for unknown subject, got %#v", record)
	}
}

func TestMfaCredentialStoreConsumeRecoveryCodeDegradesWhenSubjectNotFound(t *testing.T) {
	store := NewMfaCredentialStore(&fakeCredentialFacade{}, NewSubjectResolver(&fakeUserFacade{}))
	ok, err := store.ConsumeRecoveryCode(context.Background(), &domain.ChallengeSession{SubjectIdentifier: "missing"}, "ABCD-EFGH-IJKL", time.Now())
	if err != nil {
		t.Fatalf("consume recovery code: %v", err)
	}
	if ok {
		t.Fatal("expected consume to fail for unknown subject")
	}
}

func TestMfaCredentialStoreReturnsSystemErrorWhenResolverMissingForNamedSubject(t *testing.T) {
	store := NewMfaCredentialStore(&fakeCredentialFacade{}, nil)

	_, err := store.FindEnabledOtpBinding(context.Background(), &domain.ChallengeSession{SubjectIdentifier: "login:alice"})
	if err == nil {
		t.Fatal("expected missing subject resolver to return error")
	}
}

func TestMfaCredentialStoreCompleteTotpBindingSetsVerifiedAt(t *testing.T) {
	facade := &fakeCredentialFacade{}
	store := NewMfaCredentialStore(facade, NewSubjectResolver(&fakeUserFacade{
		byAccount: map[string]*userfacade.SubjectRecord{
			"alice": {UserID: 1001, AccountName: "alice"},
		},
	}))
	now := time.Now()
	if err := store.CompleteTotpBinding(context.Background(), &domain.ChallengeSession{SubjectIdentifier: "login:alice"}, "JBSWY3DPEHPK3PXP", now, 8); err != nil {
		t.Fatalf("complete totp binding: %v", err)
	}
	if facade.completeTotp == nil || facade.completeTotp.UserID != 1001 || facade.completeTotp.RecoveryBatchSize != 8 {
		t.Fatalf("unexpected complete totp command: %#v", facade.completeTotp)
	}
	if facade.completeTotp.VerifiedAt == nil || facade.completeTotp.VerifiedAt.IsZero() {
		t.Fatal("expected verifiedAt to be populated")
	}
}

type fakeCredentialFacade struct {
	totp                *credentialfacade.TotpCredential
	totpSecret          *credentialfacade.TotpSecret
	consumeOK           bool
	completeTotp        *credentialfacade.CompleteTotpBindingCommand
	recoveryCount       int
	countRecoveryUserID int64
}

func (f *fakeCredentialFacade) FindActivePasswordByUserID(ctx context.Context, userID int64) (*credentialfacade.PasswordCredential, error) {
	return nil, nil
}
func (f *fakeCredentialFacade) UpsertPasswordCredential(ctx context.Context, command credentialfacade.UpsertPasswordCredentialCommand) error {
	return nil
}
func (f *fakeCredentialFacade) MarkPasswordUsed(ctx context.Context, userID int64, usedAt time.Time) error {
	return nil
}
func (f *fakeCredentialFacade) FindActiveTotpByUserID(ctx context.Context, userID int64) (*credentialfacade.TotpCredential, error) {
	return f.totp, nil
}
func (f *fakeCredentialFacade) FindActiveTotpSecretByUserID(ctx context.Context, userID int64) (*credentialfacade.TotpSecret, error) {
	return f.totpSecret, nil
}
func (f *fakeCredentialFacade) UpsertTotpCredential(ctx context.Context, command credentialfacade.UpsertTotpCredentialCommand) error {
	return nil
}
func (f *fakeCredentialFacade) CompleteTotpBinding(ctx context.Context, command credentialfacade.CompleteTotpBindingCommand) error {
	copied := command
	f.completeTotp = &copied
	return nil
}
func (f *fakeCredentialFacade) DisableTotpCredential(ctx context.Context, userID int64) (bool, error) {
	return false, nil
}
func (f *fakeCredentialFacade) MarkTotpUsed(ctx context.Context, userID int64, usedAt time.Time) error {
	return nil
}
func (f *fakeCredentialFacade) ListActivePasskeys(ctx context.Context, userID int64) ([]credentialfacade.PasskeyCredential, error) {
	return nil, nil
}
func (f *fakeCredentialFacade) FindActivePasskeyByCredentialKey(ctx context.Context, credentialKey string) (*credentialfacade.PasskeyCredential, error) {
	return nil, nil
}
func (f *fakeCredentialFacade) SavePasskeyCredential(ctx context.Context, command credentialfacade.SavePasskeyCredentialCommand) error {
	return nil
}
func (f *fakeCredentialFacade) CompletePasskeyBinding(ctx context.Context, command credentialfacade.CompletePasskeyBindingCommand) error {
	return nil
}
func (f *fakeCredentialFacade) DisablePasskeyCredential(ctx context.Context, userID int64, credentialKey string) (bool, error) {
	return false, nil
}
func (f *fakeCredentialFacade) UpdatePasskeyUsage(ctx context.Context, credentialKey string, signCount int64, usedAt time.Time) error {
	return nil
}
func (f *fakeCredentialFacade) CountAvailableRecoveryCodes(ctx context.Context, userID int64) (int, error) {
	f.countRecoveryUserID = userID
	return f.recoveryCount, nil
}
func (f *fakeCredentialFacade) RegenerateRecoveryCodes(ctx context.Context, userID int64, batchSize int) (*credentialfacade.RegeneratedRecoveryCodes, error) {
	return nil, nil
}
func (f *fakeCredentialFacade) ConsumeRecoveryCode(ctx context.Context, userID int64, recoveryCode string, usedAt time.Time) (bool, error) {
	return f.consumeOK, nil
}

type fakeUserFacade struct {
	byID      map[int64]*userfacade.SubjectRecord
	byAccount map[string]*userfacade.SubjectRecord
}

func (f *fakeUserFacade) FindSubjectByID(ctx context.Context, userID int64) (*userfacade.SubjectRecord, error) {
	if f == nil || f.byID == nil {
		return nil, nil
	}
	return f.byID[userID], nil
}

func (f *fakeUserFacade) ExistsByID(context.Context, int64) (bool, error) {
	return false, nil
}

func (f *fakeUserFacade) BuildPrincipalSeed(context.Context, int64) (*userfacade.UserPrincipalSeed, error) {
	return nil, nil
}

func (f *fakeUserFacade) FindSubjectByAccount(ctx context.Context, account string) (*userfacade.SubjectRecord, error) {
	if f == nil || f.byAccount == nil {
		return nil, nil
	}
	return f.byAccount[account], nil
}

func (f *fakeUserFacade) FindSubjectByEmail(context.Context, string) (*userfacade.SubjectRecord, error) {
	return nil, nil
}

func (f *fakeUserFacade) CreateExternalSubject(context.Context, userfacade.CreateExternalSubjectCommand) (*userfacade.SubjectRecord, error) {
	return nil, nil
}

func (f *fakeUserFacade) CreateFormSubject(context.Context, userfacade.CreateFormSubjectCommand) (*userfacade.SubjectRecord, error) {
	return nil, nil
}
