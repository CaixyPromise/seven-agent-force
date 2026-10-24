package application

import (
	"context"
	"reflect"
	"sync"
	"testing"
	"time"

	"github.com/CaixyPromise/seven-agent/force/internal/app/authorization/domain"
	cachegovernancefacade "github.com/CaixyPromise/seven-agent/force/internal/app/system/cache_governance/facade"
	cacheinfra "github.com/CaixyPromise/seven-agent/force/internal/infrastructure/cache"
	"github.com/CaixyPromise/seven-agent/force/internal/shared/cachepolicy"
	"github.com/CaixyPromise/seven-agent/force/internal/shared/securitycontext"
)

func TestGovernedAuthorizationContextDoesNotCacheSessionAuthority(t *testing.T) {
	repo := newAuthorizationSnapshotRepo()
	service, governed := newGovernedAuthorizationSnapshotService(repo)
	issuedA := time.Now().UTC().Add(-time.Minute)
	expiresA := time.Now().UTC().Add(time.Hour)
	first, err := service.BuildUserContext(context.Background(), 1001, "session-a", &issuedA, &expiresA, "bearer")
	if err != nil {
		t.Fatalf("first context: %v", err)
	}
	issuedB := issuedA.Add(time.Minute)
	expiresB := expiresA.Add(time.Hour)
	second, err := service.BuildUserContext(context.Background(), 1001, "session-b", &issuedB, &expiresB, "cookie")
	if err != nil {
		t.Fatalf("second context: %v", err)
	}
	if repo.contextLoads() != 3 || governed.hits != 1 {
		t.Fatalf("expected governed snapshot hit with only source-side availability preflights, loads=%d hits=%d", repo.contextLoads(), governed.hits)
	}
	if first.SessionID != "session-a" || second.SessionID != "session-b" || second.Source != "cookie" || second.IssuedAtEpoch != issuedB.Unix() || second.ExpireAtEpoch != expiresB.Unix() {
		t.Fatalf("session authority leaked or was not rebound: first=%#v second=%#v", first, second)
	}
	stored, ok := governed.values[cachepolicy.DataClassAuthorizationContext]
	if !ok {
		t.Fatal("expected context snapshot to be admitted")
	}
	snapshot, ok := stored.(*securitycontext.UserContext)
	if !ok || snapshot.SessionID != "" || snapshot.IssuedAtEpoch != 0 || snapshot.ExpireAtEpoch != 0 || snapshot.Source != "" {
		t.Fatalf("reusable authorization snapshot must exclude session authority, got %#v", stored)
	}
}

func TestActiveTemporaryGrantBypassesWarmAuthorizationSnapshots(t *testing.T) {
	repo := newAuthorizationSnapshotRepo()
	service, governed := newGovernedAuthorizationSnapshotService(repo)
	if _, err := service.BuildUserContext(context.Background(), 1001, "", nil, nil, "test"); err != nil {
		t.Fatalf("warm context: %v", err)
	}
	if _, err := service.GetCurrentUserMenus(context.Background(), 1001); err != nil {
		t.Fatalf("warm menus: %v", err)
	}
	repo.setTemporary(domain.TemporaryPermissionRecord{UserID: 1001, Type: 1, ExpireAt: governedSnapshotTimePointer(time.Now().UTC().Add(time.Hour))})
	if _, err := service.BuildUserContext(context.Background(), 1001, "", nil, nil, "test"); err != nil {
		t.Fatalf("temporary context must load authority: %v", err)
	}
	if _, err := service.GetCurrentUserMenus(context.Background(), 1001); err != nil {
		t.Fatalf("temporary menus must load authority: %v", err)
	}
	if governed.hits != 0 {
		t.Fatalf("temporary grant must bypass pre-existing authorization snapshots, hits=%d", governed.hits)
	}
	if governed.preflightDenied != 2 {
		t.Fatalf("expected context and menu preflight bypasses, denied=%d", governed.preflightDenied)
	}
	if governed.admissions != 2 {
		t.Fatalf("temporary grant must not admit replacement snapshots, admissions=%d", governed.admissions)
	}
}

func TestGovernedAuthorizationMenusUseSeparateCataloguedSnapshot(t *testing.T) {
	repo := newAuthorizationSnapshotRepo()
	service, governed := newGovernedAuthorizationSnapshotService(repo)
	first, err := service.GetCurrentUserMenus(context.Background(), 1001)
	if err != nil {
		t.Fatalf("first menus: %v", err)
	}
	second, err := service.GetCurrentUserMenus(context.Background(), 1001)
	if err != nil {
		t.Fatalf("second menus: %v", err)
	}
	if len(first) != 1 || len(second) != 1 || repo.menuLoads() != 1 || governed.hits != 1 {
		t.Fatalf("expected one governed menu source load then hit, first=%#v second=%#v loads=%d hits=%d", first, second, repo.menuLoads(), governed.hits)
	}
	if _, ok := governed.values[cachepolicy.DataClassAuthorizationMenus]; !ok {
		t.Fatal("menus must use their own catalogued data class")
	}
	if _, ok := governed.values[cachepolicy.DataClassAuthorizationContext]; ok {
		t.Fatal("menu snapshot must not share the authorization-context cache entry")
	}
}

func TestDisabledOrLockedUserCannotUseWarmAuthorizationSnapshot(t *testing.T) {
	for _, state := range []struct {
		name    string
		enabled bool
		locked  bool
	}{
		{name: "disabled", enabled: false},
		{name: "locked", enabled: true, locked: true},
	} {
		t.Run(state.name, func(t *testing.T) {
			repo := newAuthorizationSnapshotRepo()
			service, governed := newGovernedAuthorizationSnapshotService(repo)
			if _, err := service.BuildUserContext(context.Background(), 1001, "", nil, nil, "test"); err != nil {
				t.Fatalf("warm context: %v", err)
			}
			if _, err := service.GetCurrentUserMenus(context.Background(), 1001); err != nil {
				t.Fatalf("warm menus: %v", err)
			}
			repo.setAvailability(state.enabled, state.locked)
			if _, err := service.BuildUserContext(context.Background(), 1001, "", nil, nil, "test"); err == nil {
				t.Fatal("non-normal user must not receive a warm authorization context")
			}
			if _, err := service.GetCurrentUserMenus(context.Background(), 1001); err == nil {
				t.Fatal("non-normal user must not receive a warm menu snapshot")
			}
			if governed.hits != 0 {
				t.Fatalf("availability preflight must reject before any L1/L2 hit, hits=%d", governed.hits)
			}
		})
	}
}

func TestLegacyAuthorizationCacheIsNotReadWhenGovernanceIsUnavailable(t *testing.T) {
	repo := newAuthorizationSnapshotRepo()
	manager := cacheinfra.NewManager("legacy-disabled", nil)
	service := NewService(nilConfig(), manager, &serialTestTransactor{}, repo, domain.NewService(), nil, nil, nil, nil, nil)
	// authorization.cache.enabled is deliberately irrelevant without the durable
	// registrar and governed layer. This is the RED regression guard for the
	// former local 300-second authorization snapshot path.
	service.cfg.Cache.Enabled = true
	if _, err := service.BuildUserContext(context.Background(), 1001, "", nil, nil, "test"); err != nil {
		t.Fatalf("source fallback: %v", err)
	}
	if repo.contextLoads() != 1 {
		t.Fatalf("expected authority load without legacy cache, loads=%d", repo.contextLoads())
	}
}

type authorizationSnapshotRepo struct {
	*roleSecurityRepository
	mu          sync.Mutex
	temporary   []domain.TemporaryPermissionRecord
	contextRead int
	menuRead    int
	menus       []domain.MenuRecord
	enabled     bool
	locked      bool
}

func newAuthorizationSnapshotRepo() *authorizationSnapshotRepo {
	return &authorizationSnapshotRepo{
		roleSecurityRepository: newRoleSecurityRepository(),
		menus: []domain.MenuRecord{
			{MenuID: 1, ParentID: 0, Name: "System", Type: "M", Status: 0, Visible: 0},
			{MenuID: 2, ParentID: 1, Name: "Users", Type: "C", Status: 0, Visible: 0, Permission: "system:user:list"},
		},
		enabled: true,
	}
}

func (r *authorizationSnapshotRepo) FindUserAggregate(ctx context.Context, userID int64) (*domain.UserAggregate, error) {
	r.mu.Lock()
	r.contextRead++
	enabled, locked := r.enabled, r.locked
	r.mu.Unlock()
	aggregate, err := r.roleSecurityRepository.FindUserAggregate(ctx, userID)
	if aggregate != nil {
		aggregate.Enabled = enabled
		aggregate.Locked = locked
	}
	return aggregate, err
}

func (r *authorizationSnapshotRepo) ListUserTemporaryPermissions(context.Context, int64) ([]domain.TemporaryPermissionRecord, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]domain.TemporaryPermissionRecord(nil), r.temporary...), nil
}

func (r *authorizationSnapshotRepo) ListUserMenus(context.Context, int64) ([]domain.MenuRecord, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.menuRead++
	return append([]domain.MenuRecord(nil), r.menus...), nil
}

func (r *authorizationSnapshotRepo) setTemporary(items ...domain.TemporaryPermissionRecord) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.temporary = append([]domain.TemporaryPermissionRecord(nil), items...)
}

func (r *authorizationSnapshotRepo) setAvailability(enabled, locked bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.enabled = enabled
	r.locked = locked
}

func (r *authorizationSnapshotRepo) contextLoads() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.contextRead
}

func (r *authorizationSnapshotRepo) menuLoads() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.menuRead
}

func newGovernedAuthorizationSnapshotService(repo *authorizationSnapshotRepo) (*Service, *authorizationSnapshotGovernedCache) {
	governed := &authorizationSnapshotGovernedCache{values: make(map[cachepolicy.DataClass]any)}
	manager := cacheinfra.NewManager("authorization-governed-test", nil, cacheinfra.WithGovernedLayer("governed", governed))
	service := NewService(nilConfig(), manager, &serialTestTransactor{}, repo, domain.NewService(), nil, nil, nil, nil, nil)
	service.BindCacheInvalidations(enabledAuthorizationSnapshotRegistrar{})
	return service, governed
}

type enabledAuthorizationSnapshotRegistrar struct{}

func (enabledAuthorizationSnapshotRegistrar) Enabled() bool { return true }
func (enabledAuthorizationSnapshotRegistrar) Register(context.Context, cachepolicy.DataClass) (cachegovernancefacade.Registration, error) {
	return cachegovernancefacade.Registration{}, nil
}
func (enabledAuthorizationSnapshotRegistrar) AfterCommit(context.Context, ...cachegovernancefacade.Registration) {
}
func (enabledAuthorizationSnapshotRegistrar) AcquireMutationFence(context.Context, cachepolicy.DataClass) (cachepolicy.FreshnessLease, error) {
	return nil, nil
}

type authorizationSnapshotGovernedCache struct {
	mu              sync.Mutex
	values          map[cachepolicy.DataClass]any
	hits            int
	preflightDenied int
	admissions      int
}

func (c *authorizationSnapshotGovernedCache) GetOrLoadClassified(ctx context.Context, request cachepolicy.ReadRequest, dest any, loader cacheinfra.ClassifiedLoader) (bool, error) {
	return c.GetOrLoadClassifiedWithPreflight(ctx, request, dest, nil, loader)
}

func (c *authorizationSnapshotGovernedCache) GetOrLoadClassifiedWithPreflight(ctx context.Context, request cachepolicy.ReadRequest, dest any, preflight cacheinfra.ClassifiedPreflight, loader cacheinfra.ClassifiedLoader) (bool, error) {
	if preflight != nil {
		allowed, err := preflight(ctx)
		if err != nil {
			return false, err
		}
		if !allowed {
			c.mu.Lock()
			c.preflightDenied++
			c.mu.Unlock()
			return c.load(ctx, request, dest, loader, false)
		}
	}
	c.mu.Lock()
	value, hit := c.values[request.Entry.DataClass]
	if hit {
		c.hits++
	}
	c.mu.Unlock()
	if hit {
		assignAuthorizationSnapshot(nil, dest, value)
		return true, nil
	}
	return c.load(ctx, request, dest, loader, true)
}

func (c *authorizationSnapshotGovernedCache) load(ctx context.Context, request cachepolicy.ReadRequest, dest any, loader cacheinfra.ClassifiedLoader, admit bool) (bool, error) {
	loaded, err := loader(ctx)
	if err != nil {
		return false, err
	}
	assignAuthorizationSnapshot(nil, dest, loaded.Value)
	if admit && loaded.Cacheable {
		c.mu.Lock()
		c.values[request.Entry.DataClass] = loaded.Value
		c.admissions++
		c.mu.Unlock()
	}
	return true, nil
}

func assignAuthorizationSnapshot(t *testing.T, destination, value any) {
	if destination == nil || value == nil {
		if t != nil {
			t.Fatal("invalid authorization snapshot test assignment")
		}
		return
	}
	dest := reflect.ValueOf(destination)
	from := reflect.ValueOf(value)
	if dest.Kind() != reflect.Pointer || dest.IsNil() {
		if t != nil {
			t.Fatalf("destination is not a pointer: %T", destination)
		}
		return
	}
	if from.Kind() == reflect.Pointer {
		from = from.Elem()
	}
	dest.Elem().Set(from)
}

func (*authorizationSnapshotGovernedCache) MarkLocalDirty(string, ...cachepolicy.DataClass)       {}
func (*authorizationSnapshotGovernedCache) EvictLocalAndResolve(string, ...cachepolicy.DataClass) {}
func (*authorizationSnapshotGovernedCache) AdvanceGeneration(context.Context, string, cachepolicy.DataClass) (bool, error) {
	return true, nil
}
func (*authorizationSnapshotGovernedCache) SetFanoutHealthy(bool)                      {}
func (*authorizationSnapshotGovernedCache) SetFreshnessGate(cachepolicy.FreshnessGate) {}
func (*authorizationSnapshotGovernedCache) RecordRejectedFanout()                      {}
func (*authorizationSnapshotGovernedCache) GovernedStatus() cacheinfra.GovernedStatus {
	return cacheinfra.GovernedStatus{ReadTrusted: true}
}

func governedSnapshotTimePointer(value time.Time) *time.Time { return &value }
