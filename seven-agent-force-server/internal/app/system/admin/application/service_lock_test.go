package application

import (
	"context"
	"testing"
	"time"

	userfacade "github.com/CaixyPromise/seven-agent/force/internal/app/system/user/facade"
)

func TestLockAccountWithNonPositiveHoursUnlocksUserState(t *testing.T) {
	subjects := &fakeAdminSubjectFacade{byAccount: map[string]*userfacade.SubjectRecord{
		"admin": {UserID: 1001, AccountName: "admin", Enabled: true, LockStatus: true, UnsealAt: timePtr(time.Now().UTC().Add(time.Hour))},
	}}
	accounts := &fakeAdminAccountFacade{subjects: subjects.byAccount}
	failures := newFakeAdminLoginFailureStore()
	failures.locks["admin"] = time.Now().UTC().Add(time.Hour).UnixMilli()
	service := NewService(LoginSettings{}, subjects, accounts, nil, nil, nil, failures, nil, nil, nil, nil)

	if err := service.LockAccount(context.Background(), "admin", 0); err != nil {
		t.Fatalf("LockAccount(0) returned error: %v", err)
	}
	if _, ok := failures.locks["admin"]; ok {
		t.Fatalf("expected cached account lock to be deleted, got %#v", failures.locks)
	}
	if accounts.lastLock.UserID != 1001 || accounts.lastLock.Status != 0 || accounts.lastLock.UnsealTime != nil {
		t.Fatalf("expected user lock state to be cleared, got %#v", accounts.lastLock)
	}
}

func TestLockAccountWithNonPositiveHoursUnlocksUserStateWithoutFailureStore(t *testing.T) {
	subjects := &fakeAdminSubjectFacade{byAccount: map[string]*userfacade.SubjectRecord{
		"admin": {UserID: 1001, AccountName: "admin", Enabled: true, LockStatus: true, UnsealAt: timePtr(time.Now().UTC().Add(time.Hour))},
	}}
	accounts := &fakeAdminAccountFacade{subjects: subjects.byAccount}
	service := NewService(LoginSettings{}, subjects, accounts, nil, nil, nil, nil, nil, nil, nil, nil)

	if err := service.LockAccount(context.Background(), "admin", 0); err != nil {
		t.Fatalf("LockAccount(0) without failure store returned error: %v", err)
	}
	if accounts.lastLock.UserID != 1001 || accounts.lastLock.Status != 0 || accounts.lastLock.UnsealTime != nil {
		t.Fatalf("expected user lock state to be cleared without failure store, got %#v", accounts.lastLock)
	}
}

func TestExpiredAccountLockClearsFailureAndUserLockState(t *testing.T) {
	expired := time.Now().UTC().Add(-time.Minute)
	subjects := &fakeAdminSubjectFacade{byAccount: map[string]*userfacade.SubjectRecord{
		"admin": {UserID: 1001, AccountName: "admin", Enabled: true, LockStatus: true, UnsealAt: &expired},
	}}
	accounts := &fakeAdminAccountFacade{subjects: subjects.byAccount}
	failures := newFakeAdminLoginFailureStore()
	failures.failures["admin"] = 9
	failures.captchaFailures["admin"] = 2
	failures.locks["admin"] = expired.UnixMilli()
	service := NewService(LoginSettings{}, subjects, accounts, nil, nil, nil, failures, nil, nil, nil, nil)

	locked, err := service.IsAccountLocked(context.Background(), "admin")
	if err != nil {
		t.Fatalf("IsAccountLocked returned error: %v", err)
	}
	if locked {
		t.Fatalf("expected expired lock to be treated as unlocked")
	}
	if failures.failures["admin"] != 0 || failures.captchaFailures["admin"] != 0 {
		t.Fatalf("expected failure counters to be cleared, got failures=%#v captcha=%#v", failures.failures, failures.captchaFailures)
	}
	if _, ok := failures.locks["admin"]; ok {
		t.Fatalf("expected expired cached lock to be deleted, got %#v", failures.locks)
	}
	if accounts.lastLock.UserID != 1001 || accounts.lastLock.Status != 0 || accounts.lastLock.UnsealTime != nil {
		t.Fatalf("expected user lock state to be cleared, got %#v", accounts.lastLock)
	}
}

func TestRecordFailureLocksCurrentAccountWhenClientIPContextThresholdReached(t *testing.T) {
	subjects := &fakeAdminSubjectFacade{byAccount: map[string]*userfacade.SubjectRecord{
		"alice": {UserID: 1001, AccountName: "alice", Enabled: true},
		"bob":   {UserID: 1002, AccountName: "bob", Enabled: true},
		"carol": {UserID: 1003, AccountName: "carol", Enabled: true},
	}}
	accounts := &fakeAdminAccountFacade{subjects: subjects.byAccount}
	failures := newFakeAdminLoginFailureStore()
	service := NewService(LoginSettings{LockThreshold: 99, ContextLockThreshold: 3, LockDurationHours: 1}, subjects, accounts, nil, nil, nil, failures, nil, nil, nil, nil)

	for _, account := range []string{"alice", "bob", "carol"} {
		if err := service.RecordFailure(context.Background(), account, "203.0.113.10", ""); err != nil {
			t.Fatalf("RecordFailure(%s) returned error: %v", account, err)
		}
	}

	if subjects.byAccount["alice"].LockStatus || subjects.byAccount["bob"].LockStatus {
		t.Fatalf("expected earlier accounts to remain unlocked, got alice=%v bob=%v", subjects.byAccount["alice"].LockStatus, subjects.byAccount["bob"].LockStatus)
	}
	if !subjects.byAccount["carol"].LockStatus || accounts.lastLock.UserID != 1003 || accounts.lastLock.Status != 1 {
		t.Fatalf("expected current account to be locked by IP context threshold, got lastLock=%#v carol=%#v", accounts.lastLock, subjects.byAccount["carol"])
	}
	if got := failures.contextFailures["ip:203.0.113.10"]; got != 3 {
		t.Fatalf("expected IP context failure count 3, got %d", got)
	}
}

func TestRecordFailureLocksCurrentAccountWhenDeviceContextThresholdReached(t *testing.T) {
	subjects := &fakeAdminSubjectFacade{byAccount: map[string]*userfacade.SubjectRecord{
		"alice": {UserID: 1001, AccountName: "alice", Enabled: true},
		"bob":   {UserID: 1002, AccountName: "bob", Enabled: true},
		"carol": {UserID: 1003, AccountName: "carol", Enabled: true},
	}}
	accounts := &fakeAdminAccountFacade{subjects: subjects.byAccount}
	failures := newFakeAdminLoginFailureStore()
	service := NewService(LoginSettings{LockThreshold: 99, ContextLockThreshold: 3, LockDurationHours: 1}, subjects, accounts, nil, nil, nil, failures, nil, nil, nil, nil)

	for _, account := range []string{"alice", "bob", "carol"} {
		if err := service.RecordFailure(context.Background(), account, "", "device-browser-1"); err != nil {
			t.Fatalf("RecordFailure(%s) returned error: %v", account, err)
		}
	}

	if subjects.byAccount["alice"].LockStatus || subjects.byAccount["bob"].LockStatus {
		t.Fatalf("expected earlier accounts to remain unlocked, got alice=%v bob=%v", subjects.byAccount["alice"].LockStatus, subjects.byAccount["bob"].LockStatus)
	}
	if !subjects.byAccount["carol"].LockStatus || accounts.lastLock.UserID != 1003 || accounts.lastLock.Status != 1 {
		t.Fatalf("expected current account to be locked by device context threshold, got lastLock=%#v carol=%#v", accounts.lastLock, subjects.byAccount["carol"])
	}
	if got := failures.contextFailures["device:device-browser-1"]; got != 3 {
		t.Fatalf("expected device context failure count 3, got %d", got)
	}
}

type fakeAdminSubjectFacade struct {
	byAccount map[string]*userfacade.SubjectRecord
}

func (f *fakeAdminSubjectFacade) FindSubjectByID(context.Context, int64) (*userfacade.SubjectRecord, error) {
	return nil, nil
}

func (f *fakeAdminSubjectFacade) FindSubjectByAccount(_ context.Context, account string) (*userfacade.SubjectRecord, error) {
	return f.byAccount[account], nil
}

func (f *fakeAdminSubjectFacade) FindSubjectByEmail(context.Context, string) (*userfacade.SubjectRecord, error) {
	return nil, nil
}

func (f *fakeAdminSubjectFacade) CreateExternalSubject(context.Context, userfacade.CreateExternalSubjectCommand) (*userfacade.SubjectRecord, error) {
	return nil, nil
}

func (f *fakeAdminSubjectFacade) CreateFormSubject(context.Context, userfacade.CreateFormSubjectCommand) (*userfacade.SubjectRecord, error) {
	return nil, nil
}

func (f *fakeAdminSubjectFacade) ExistsByID(context.Context, int64) (bool, error) {
	return false, nil
}

func (f *fakeAdminSubjectFacade) BuildPrincipalSeed(context.Context, int64) (*userfacade.UserPrincipalSeed, error) {
	return nil, nil
}

type fakeAdminAccountFacade struct {
	subjects map[string]*userfacade.SubjectRecord
	lastLock userfacade.UpdateLockStateCommand
}

func (f *fakeAdminAccountFacade) VerifyPassword(context.Context, int64, string) (bool, error) {
	return false, nil
}

func (f *fakeAdminAccountFacade) UpdatePassword(context.Context, userfacade.UpdatePasswordCommand) error {
	return nil
}

func (f *fakeAdminAccountFacade) UpdateLockState(_ context.Context, command userfacade.UpdateLockStateCommand) error {
	f.lastLock = command
	for _, subject := range f.subjects {
		if subject.UserID == command.UserID {
			subject.LockStatus = command.Status == 1
			subject.UnsealAt = command.UnsealTime
			break
		}
	}
	return nil
}

type fakeAdminLoginFailureStore struct {
	failures        map[string]int
	captchaFailures map[string]int
	contextFailures map[string]int
	locks           map[string]int64
}

func newFakeAdminLoginFailureStore() *fakeAdminLoginFailureStore {
	return &fakeAdminLoginFailureStore{
		failures:        map[string]int{},
		captchaFailures: map[string]int{},
		contextFailures: map[string]int{},
		locks:           map[string]int64{},
	}
}

func (f *fakeAdminLoginFailureStore) GetFailureCount(_ context.Context, userAccount string) (int, error) {
	return f.failures[userAccount], nil
}

func (f *fakeAdminLoginFailureStore) SaveFailureCount(_ context.Context, userAccount string, count int) error {
	f.failures[userAccount] = count
	return nil
}

func (f *fakeAdminLoginFailureStore) DeleteFailureCount(_ context.Context, userAccount string) error {
	delete(f.failures, userAccount)
	return nil
}

func (f *fakeAdminLoginFailureStore) GetCaptchaFailureCount(_ context.Context, userAccount string) (int, error) {
	return f.captchaFailures[userAccount], nil
}

func (f *fakeAdminLoginFailureStore) SaveCaptchaFailureCount(_ context.Context, userAccount string, count int) error {
	f.captchaFailures[userAccount] = count
	return nil
}

func (f *fakeAdminLoginFailureStore) DeleteCaptchaFailureCount(_ context.Context, userAccount string) error {
	delete(f.captchaFailures, userAccount)
	return nil
}

func (f *fakeAdminLoginFailureStore) GetContextFailureCount(_ context.Context, scope, value string) (int, error) {
	return f.contextFailures[scope+":"+value], nil
}

func (f *fakeAdminLoginFailureStore) SaveContextFailureCount(_ context.Context, scope, value string, count int) error {
	f.contextFailures[scope+":"+value] = count
	return nil
}

func (f *fakeAdminLoginFailureStore) GetLockUntil(_ context.Context, userAccount string) (*int64, error) {
	value, ok := f.locks[userAccount]
	if !ok {
		return nil, nil
	}
	return &value, nil
}

func (f *fakeAdminLoginFailureStore) SaveLockUntil(_ context.Context, userAccount string, unlockTime int64, _ int) error {
	f.locks[userAccount] = unlockTime
	return nil
}

func (f *fakeAdminLoginFailureStore) DeleteLock(_ context.Context, userAccount string) error {
	delete(f.locks, userAccount)
	return nil
}

func timePtr(value time.Time) *time.Time {
	return &value
}
