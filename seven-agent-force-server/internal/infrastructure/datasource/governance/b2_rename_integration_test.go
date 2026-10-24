package governance

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	externallogindomain "github.com/CaixyPromise/seven-agent/force/internal/app/external_login/domain"
	externallogininfra "github.com/CaixyPromise/seven-agent/force/internal/app/external_login/infrastructure"
	hubinfra "github.com/CaixyPromise/seven-agent/force/internal/app/hub_control/infrastructure"
	platformdomain "github.com/CaixyPromise/seven-agent/force/internal/app/platform/domain"
	platforminfra "github.com/CaixyPromise/seven-agent/force/internal/app/platform/infrastructure"
	"github.com/CaixyPromise/seven-agent/force/internal/infrastructure/datasource/store"
	_ "github.com/go-sql-driver/mysql"
	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/jmoiron/sqlx"
	"github.com/pressly/goose/v3"
)

const (
	dg4B2RenameAcceptanceEnv = "DG4_B2_RENAME_ACCEPTANCE"
	dg4B2TestDialectEnv      = "DG4_B2_TEST_DIALECT"
	dg4B2TestDSNEnv          = "DG4_B2_TEST_DSN"
	dg4B2MigrationVersion    = int64(20260731110000)
	dg4B2FixtureProviderCode = "dg4-b2-provider"
	dg4B2FixtureNodeCode     = "dg4-b2-node"
	dg4B2FixtureUserID       = int64(9202607312001)
	dg4B2FixtureNodeID       = int64(9202607312002)
)

type b2TableMapping struct {
	legacy string
	target string
}

var b2TableMappings = []b2TableMapping{
	{legacy: "sysExternalLoginProvider", target: "sys_external_login_provider"},
	{legacy: "sysExternalManagedProviderCommand", target: "sys_external_managed_provider_command"},
	{legacy: "sysExternalOAuthLoginState", target: "sys_external_oauth_login_state"},
	{legacy: "sysExternalOAuthToken", target: "sys_external_oauth_token"},
	{legacy: "sysExternalProviderMethod", target: "sys_external_provider_method"},
	{legacy: "sysExternalUserIdentity", target: "sys_external_user_identity"},
	{legacy: "sysFederatedNode", target: "sys_federated_node"},
	{legacy: "sysFederatedNodeConnectionCommand", target: "sys_federated_node_connection_command"},
}

// TestB2ExternalIdentityFederationRenameAcceptance is a controlled restart
// acceptance test. "before" runs from the B1-stage source snapshot (B1 names
// are renamed; B2/B3 names are legacy). "after" runs from the B2-stage source
// snapshot (B1/B2 names are renamed; B3 names are legacy). "migrate" and
// "forward" only exercise the migration harness.
func TestB2ExternalIdentityFederationRenameAcceptance(t *testing.T) {
	mode := strings.ToLower(strings.TrimSpace(os.Getenv(dg4B2RenameAcceptanceEnv)))
	if mode != "before" && mode != "migrate" && mode != "after" && mode != "forward" {
		t.Skip("set DG4_B2_RENAME_ACCEPTANCE=before|migrate|after|forward with the exact isolated database")
	}
	dialect := strings.ToLower(strings.TrimSpace(os.Getenv(dg4B2TestDialectEnv)))
	dsn := strings.TrimSpace(os.Getenv(dg4B2TestDSNEnv))
	if dialect == "" || dsn == "" {
		t.Skip("set DG4_B2_TEST_DIALECT and DG4_B2_TEST_DSN for the exact isolated database")
	}
	if dialect != "mysql" && dialect != "postgres" {
		t.Fatalf("unsupported B2 test dialect %q", dialect)
	}
	driver := "mysql"
	if dialect == "postgres" {
		driver = "pgx"
	}
	db, err := sql.Open(driver, dsn)
	if err != nil {
		t.Fatalf("open B2 %s database: %v", dialect, err)
	}
	if err := AssertConnectedDatabase(context.Background(), db, dialect); err != nil {
		_ = db.Close()
		t.Fatal(err)
	}
	provider := &b1IntegrationProvider{driver: driver, dialect: dialect, db: db, sqlxDB: sqlx.NewDb(db, driver)}
	t.Cleanup(func() { _ = provider.Close() })

	version := b1GooseVersion(t, context.Background(), provider.db)
	if mode == "before" && version != dg4B1MigrationVersion {
		t.Fatalf("B2 pre-rename version=%d, want=%d", version, dg4B1MigrationVersion)
	}
	if mode == "migrate" {
		if version != dg4B1MigrationVersion {
			t.Fatalf("B2 migration start version=%d, want=%d", version, dg4B1MigrationVersion)
		}
		b2ApplyMigration(t, context.Background(), provider.db, dialect)
		if version = b1GooseVersion(t, context.Background(), provider.db); version != dg4B2MigrationVersion {
			t.Fatalf("B2 migration finish version=%d, want=%d", version, dg4B2MigrationVersion)
		}
		return
	}
	if mode == "forward" {
		if version != dg4B2MigrationVersion {
			t.Fatalf("B2 forward-recovery version=%d, want=%d", version, dg4B2MigrationVersion)
		}
		b2AssertForwardOnlyDownRejected(t, context.Background(), provider.db, dialect)
		return
	}
	if mode == "after" && version < dg4B2MigrationVersion {
		t.Fatalf("B2 post-rename version=%d, require at least %d", version, dg4B2MigrationVersion)
	}

	externalRepository, err := externallogininfra.NewRepository(provider)
	if err != nil {
		t.Fatalf("new B2 external login repository: %v", err)
	}
	hubRepository, err := hubinfra.NewRepository(provider)
	if err != nil {
		t.Fatalf("new B2 hub repository: %v", err)
	}
	platformRepository, err := platforminfra.NewRepository(provider)
	if err != nil {
		t.Fatalf("new B2 platform repository: %v", err)
	}

	ctx := context.Background()
	if mode == "before" {
		b2CreateFixture(t, ctx, provider, externalRepository, hubRepository, platformRepository)
		b2AssertBusinessFixture(t, ctx, externalRepository, hubRepository, platformRepository)
		b2AssertTableState(t, ctx, provider.db, dialect, true)
		dg4CapturePhysicalSignatures(t, ctx, provider.db, dialect, "b2", dg4B2PhysicalMappings())
		return
	}

	b2AssertBusinessFixture(t, ctx, externalRepository, hubRepository, platformRepository)
	dg4AssertPhysicalSignatures(t, ctx, provider.db, dialect, "b2", dg4B2PhysicalMappings())
	b2UpdateAndCreateAfterRename(t, ctx, provider, externalRepository, hubRepository, platformRepository)
	b2AssertTableState(t, ctx, provider.db, dialect, false)
}

func b2ApplyMigration(t *testing.T, ctx context.Context, db *sql.DB, dialect string) {
	t.Helper()
	root, err := filepath.Abs(filepath.Join("..", "..", "..", ".."))
	if err != nil {
		t.Fatalf("resolve B2 repository root: %v", err)
	}
	dir := filepath.Join(root, "migrations", dialect)
	goose.SetTableName(goose.DefaultTablename)
	if err := goose.SetDialect(dialect); err != nil {
		t.Fatalf("set B2 %s goose dialect: %v", dialect, err)
	}
	if err := goose.UpToContext(ctx, db, dir, dg4B2MigrationVersion); err != nil {
		t.Fatalf("apply B2 %s in-place table rename: %v", dialect, err)
	}
}

func b2AssertForwardOnlyDownRejected(t *testing.T, ctx context.Context, db *sql.DB, dialect string) {
	t.Helper()
	root, err := filepath.Abs(filepath.Join("..", "..", "..", ".."))
	if err != nil {
		t.Fatalf("resolve B2 repository root for rejected Down: %v", err)
	}
	dir := filepath.Join(root, "migrations", dialect)
	goose.SetTableName(goose.DefaultTablename)
	if err := goose.SetDialect(dialect); err != nil {
		t.Fatalf("set B2 %s goose dialect for rejected Down: %v", dialect, err)
	}
	assertForwardOnlyDownRejected(t, ctx, db, dir)
}

func b2CreateFixture(
	t *testing.T,
	ctx context.Context,
	provider *b1IntegrationProvider,
	externalRepository *externallogininfra.Repository,
	hubRepository *hubinfra.Repository,
	platformRepository *platforminfra.Repository,
) {
	t.Helper()
	now := time.Now().UTC().Truncate(time.Microsecond)
	if err := store.NewSQLXTransactor(provider.sqlxDB).WithinTransaction(ctx, func(txCtx context.Context) error {
		if err := externalRepository.InsertProvider(txCtx, &externallogindomain.Provider{
			ProviderCode:             dg4B2FixtureProviderCode,
			ProviderName:             "DG4 B2 Legacy Provider",
			ProtocolType:             externallogindomain.ProtocolTypeOIDC,
			Issuer:                   "https://dg4-b2.example.test",
			AuthorizationEndpoint:    "https://dg4-b2.example.test/authorize",
			TokenEndpoint:            "https://dg4-b2.example.test/token",
			ClientID:                 "dg4-b2-client",
			Scopes:                   []string{"openid", "profile"},
			RedirectURI:              "https://app.example.test/callback",
			DisplayName:              "DG4 B2",
			SortOrder:                1,
			DisplayEnabled:           true,
			LoginEnabled:             true,
			BindEnabled:              true,
			EmailAutoBindEnabled:     false,
			AccountAutoCreateEnabled: false,
			Status:                   externallogindomain.ProviderStatusActive,
		}, dg4B2FixtureUserID); err != nil {
			return err
		}
		if err := externalRepository.ReplaceProviderMethods(txCtx, dg4B2FixtureProviderCode, []externallogindomain.ProviderMethod{{
			ProviderCode:   dg4B2FixtureProviderCode,
			MethodKey:      "oidc",
			CapabilityCode: externallogindomain.CapabilityOIDCLogin,
			Status:         externallogindomain.ProviderMethodStatusActive,
		}}); err != nil {
			return err
		}
		identity := &externallogindomain.ExternalIdentity{
			ProviderCode:    dg4B2FixtureProviderCode,
			ExternalIssuer:  "https://dg4-b2.example.test",
			ExternalSubject: "dg4-b2-subject",
			UserID:          dg4B2FixtureUserID,
			ExternalLogin:   "dg4-b2-login",
			ExternalEmail:   "dg4-b2@example.test",
			EmailVerified:   true,
			Status:          externallogindomain.IdentityStatusActive,
			FirstLinkedAt:   now,
		}
		if err := externalRepository.InsertIdentity(txCtx, identity, dg4B2FixtureUserID); err != nil {
			return err
		}
		storedIdentity, err := externalRepository.FindIdentityBySubject(txCtx, identity.ProviderCode, identity.ExternalIssuer, identity.ExternalSubject)
		if err != nil || storedIdentity == nil {
			return fmt.Errorf("read B2 inserted identity: identity=%#v err=%w", storedIdentity, err)
		}
		if err := externalRepository.InsertManagedProviderCommand(txCtx, &externallogindomain.ManagedProviderCommand{
			ProviderCode:      dg4B2FixtureProviderCode,
			ConnectionVersion: "v1",
			RequestHash:       strings.Repeat("a", 64),
			CreateTime:        now,
		}); err != nil {
			return err
		}
		if err := externalRepository.InsertLoginState(txCtx, &externallogindomain.LoginState{
			StateID:              "dg4-b2-state",
			ProviderCode:         dg4B2FixtureProviderCode,
			StateHash:            "dg4-b2-state-hash",
			Issuer:               identity.ExternalIssuer,
			ProviderConfigDigest: strings.Repeat("b", 64),
			RedirectURI:          "https://app.example.test/callback",
			ExpiresAt:            now.Add(time.Hour),
			Status:               externallogindomain.LoginStateStatusActive,
			LoginIP:              "127.0.0.1",
			UserAgent:            "DG4 B2",
			TraceID:              "dg4-b2-trace",
		}); err != nil {
			return err
		}
		if err := externalRepository.InsertToken(txCtx, &externallogindomain.OAuthToken{
			ProviderCode:       dg4B2FixtureProviderCode,
			IdentityID:         storedIdentity.ID,
			UserID:             dg4B2FixtureUserID,
			TokenPurpose:       externallogindomain.TokenPurposeLogin,
			Scopes:             []string{"openid"},
			ScopeHash:          strings.Repeat("c", 64),
			TokenSetCiphertext: "dg4-b2-token-cipher",
			TokenSetEDEK:       "dg4-b2-token-edek",
			TokenSetWrapKeyRef: "dg4-b2-key",
			Status:             externallogindomain.TokenStatusActive,
			Version:            1,
		}); err != nil {
			return err
		}

		if err := hubRepository.Insert(txCtx, &hubinfra.NodeRecord{
			ID:                dg4B2FixtureNodeID,
			NodeCode:          dg4B2FixtureNodeCode,
			NodeName:          "DG4 B2 Legacy Node",
			Status:            0,
			DiscoveryType:     "STATIC",
			ManagementBaseURL: "https://node.example.test",
			HubIssuer:         "https://hub.example.test",
			ConnectionStatus:  "PENDING",
			TargetRevision:    1,
			CreatedAt:         now,
			UpdatedAt:         now,
		}); err != nil {
			return err
		}
		if err := hubRepository.SaveConnectionCommand(txCtx, &hubinfra.ConnectionCommandRecord{
			NodeCode:          dg4B2FixtureNodeCode,
			ConnectionVersion: "v1",
			RequestHash:       strings.Repeat("d", 64),
			TargetRevision:    1,
			State:             "PENDING",
			CreatedAt:         now,
			UpdatedAt:         now,
		}); err != nil {
			return err
		}

		return platformRepository.ReplaceLoginMethods(txCtx, dg4B1FixturePlatformCode, []platformdomain.LoginMethod{{
			MethodType:     platformdomain.MethodExternalOAuth,
			ProviderCode:   dg4B2FixtureProviderCode,
			DisplayName:    "DG4 B2",
			SortOrder:      1,
			DisplayEnabled: true,
			LoginEnabled:   true,
		}}, 0)
	}); err != nil {
		t.Fatalf("create B2 pre-rename fixture: %v", err)
	}
}

func b2AssertBusinessFixture(
	t *testing.T,
	ctx context.Context,
	externalRepository *externallogininfra.Repository,
	hubRepository *hubinfra.Repository,
	platformRepository *platforminfra.Repository,
) {
	t.Helper()
	provider, err := externalRepository.FindProvider(ctx, dg4B2FixtureProviderCode)
	if err != nil || provider == nil || provider.ProviderCode != dg4B2FixtureProviderCode {
		t.Fatalf("read B2 provider through repository: provider=%#v err=%v", provider, err)
	}
	methods, err := externalRepository.ListProviderMethods(ctx, dg4B2FixtureProviderCode)
	if err != nil || len(methods) != 1 {
		t.Fatalf("read B2 provider methods through repository: methods=%#v err=%v", methods, err)
	}
	identity, err := externalRepository.FindIdentityBySubject(ctx, dg4B2FixtureProviderCode, "https://dg4-b2.example.test", "dg4-b2-subject")
	if err != nil || identity == nil || identity.UserID != dg4B2FixtureUserID {
		t.Fatalf("read B2 identity through repository: identity=%#v err=%v", identity, err)
	}
	command, err := externalRepository.FindManagedProviderCommand(ctx, dg4B2FixtureProviderCode, "v1")
	if err != nil || command == nil || command.RequestHash != strings.Repeat("a", 64) {
		t.Fatalf("read B2 provider command through repository: command=%#v err=%v", command, err)
	}
	token, err := externalRepository.FindActiveToken(ctx, dg4B2FixtureProviderCode, identity.ID, dg4B2FixtureUserID, externallogindomain.TokenPurposeLogin, strings.Repeat("c", 64))
	if err != nil || token == nil || token.TokenSetCiphertext != "dg4-b2-token-cipher" {
		t.Fatalf("read B2 token through repository: token=%#v err=%v", token, err)
	}
	node, err := hubRepository.Find(ctx, dg4B2FixtureNodeCode)
	if err != nil || node == nil || node.NodeCode != dg4B2FixtureNodeCode {
		t.Fatalf("read B2 node through repository: node=%#v err=%v", node, err)
	}
	methodsForPlatform, err := platformRepository.ListLoginMethods(ctx, dg4B1FixturePlatformCode)
	if err != nil || len(methodsForPlatform) != 1 || methodsForPlatform[0].ProviderCode != dg4B2FixtureProviderCode {
		t.Fatalf("read B2 provider through platform repository: methods=%#v err=%v", methodsForPlatform, err)
	}
}

func b2UpdateAndCreateAfterRename(
	t *testing.T,
	ctx context.Context,
	provider *b1IntegrationProvider,
	externalRepository *externallogininfra.Repository,
	hubRepository *hubinfra.Repository,
	platformRepository *platforminfra.Repository,
) {
	t.Helper()
	now := time.Now().UTC().Truncate(time.Microsecond)
	if err := store.NewSQLXTransactor(provider.sqlxDB).WithinTransaction(ctx, func(txCtx context.Context) error {
		storedIdentity, err := externalRepository.FindIdentityBySubject(txCtx, dg4B2FixtureProviderCode, "https://dg4-b2.example.test", "dg4-b2-subject")
		if err != nil || storedIdentity == nil {
			return fmt.Errorf("read B2 identity after rename: identity=%#v err=%w", storedIdentity, err)
		}
		if err := externalRepository.TouchIdentityLogin(txCtx, storedIdentity.ID, externallogindomain.ExternalProfile{Login: "dg4-b2-after", Email: "after@example.test", EmailVerified: true}, now); err != nil {
			return err
		}
		state, err := externalRepository.ConsumeLoginState(txCtx, "dg4-b2-state-hash", now)
		if err != nil || state == nil || state.Status != externallogindomain.LoginStateStatusConsumed {
			return fmt.Errorf("consume B2 pre-rename login state: state=%#v err=%w", state, err)
		}
		token, err := externalRepository.FindActiveToken(txCtx, dg4B2FixtureProviderCode, storedIdentity.ID, dg4B2FixtureUserID, externallogindomain.TokenPurposeLogin, strings.Repeat("c", 64))
		if err != nil || token == nil {
			return fmt.Errorf("read B2 token after rename: token=%#v err=%w", token, err)
		}
		token.TokenSetCiphertext = "dg4-b2-token-after"
		token.Status = externallogindomain.TokenStatusActive
		if updated, err := externalRepository.UpdateTokenSet(txCtx, token, token.Version); err != nil || !updated {
			return fmt.Errorf("update B2 token after rename: updated=%t err=%w", updated, err)
		}
		if err := externalRepository.InsertToken(txCtx, &externallogindomain.OAuthToken{
			ProviderCode:       dg4B2FixtureProviderCode,
			IdentityID:         storedIdentity.ID,
			UserID:             dg4B2FixtureUserID,
			TokenPurpose:       externallogindomain.TokenPurposeAPI,
			Scopes:             []string{"profile"},
			ScopeHash:          strings.Repeat("e", 64),
			TokenSetCiphertext: "dg4-b2-api-cipher",
			TokenSetEDEK:       "dg4-b2-api-edek",
			TokenSetWrapKeyRef: "dg4-b2-key",
			Status:             externallogindomain.TokenStatusActive,
			Version:            1,
		}); err != nil {
			return err
		}
		node, err := hubRepository.Find(txCtx, dg4B2FixtureNodeCode)
		if err != nil || node == nil {
			return fmt.Errorf("read B2 node after rename: node=%#v err=%w", node, err)
		}
		node.NodeName = "DG4 B2 Renamed Node"
		node.UpdatedAt = now
		if err := hubRepository.UpdateMetadata(txCtx, node); err != nil {
			return err
		}
		if err := hubRepository.SaveConnectionCommand(txCtx, &hubinfra.ConnectionCommandRecord{
			NodeCode:          dg4B2FixtureNodeCode,
			ConnectionVersion: "v1",
			RequestHash:       strings.Repeat("d", 64),
			TargetRevision:    1,
			State:             "ACTIVE",
			CreatedAt:         now,
			UpdatedAt:         now,
		}); err != nil {
			return err
		}
		return externalRepository.InsertProvider(txCtx, &externallogindomain.Provider{
			ProviderCode:          dg4B2FixtureProviderCode + "-after",
			ProviderName:          "DG4 B2 New Provider",
			ProtocolType:          externallogindomain.ProtocolTypeOAuth2,
			AuthorizationEndpoint: "https://after.example.test/authorize",
			TokenEndpoint:         "https://after.example.test/token",
			ClientID:              "dg4-b2-after-client",
			Scopes:                []string{"openid"},
			RedirectURI:           "https://app.example.test/after",
			DisplayName:           "DG4 B2 After",
			Status:                externallogindomain.ProviderStatusActive,
		}, dg4B2FixtureUserID)
	}); err != nil {
		t.Fatalf("update/create B2 records after rename: %v", err)
	}
	if provider, err := externalRepository.FindProvider(ctx, dg4B2FixtureProviderCode+"-after"); err != nil || provider == nil {
		t.Fatalf("read newly created B2 provider after rename: provider=%#v err=%v", provider, err)
	}
	if token, err := externalRepository.FindActiveToken(ctx, dg4B2FixtureProviderCode, mustB2IdentityID(t, ctx, externalRepository), dg4B2FixtureUserID, externallogindomain.TokenPurposeAPI, strings.Repeat("e", 64)); err != nil || token == nil {
		t.Fatalf("read newly created B2 token after rename: token=%#v err=%v", token, err)
	}
	if node, err := hubRepository.Find(ctx, dg4B2FixtureNodeCode); err != nil || node == nil || node.NodeName != "DG4 B2 Renamed Node" {
		t.Fatalf("read updated B2 node after rename: node=%#v err=%v", node, err)
	}
	if commands, err := platformRepository.ListAvailableExternalProviderCodes(ctx, []string{dg4B2FixtureProviderCode}); err != nil || len(commands) != 1 {
		t.Fatalf("read B2 provider through platform repository after rename: providers=%#v err=%v", commands, err)
	}
}

func mustB2IdentityID(t *testing.T, ctx context.Context, repository *externallogininfra.Repository) int64 {
	t.Helper()
	identity, err := repository.FindIdentityBySubject(ctx, dg4B2FixtureProviderCode, "https://dg4-b2.example.test", "dg4-b2-subject")
	if err != nil || identity == nil {
		t.Fatalf("read B2 identity ID: identity=%#v err=%v", identity, err)
	}
	return identity.ID
}

func b2AssertTableState(t *testing.T, ctx context.Context, db *sql.DB, dialect string, legacy bool) {
	t.Helper()
	for _, mapping := range b2TableMappings {
		legacyExists := b1TableExists(t, ctx, db, dialect, mapping.legacy)
		targetExists := b1TableExists(t, ctx, db, dialect, mapping.target)
		if legacy {
			if !legacyExists || targetExists {
				t.Fatalf("B2 pre-rename table state %s -> %s: legacy=%t target=%t", mapping.legacy, mapping.target, legacyExists, targetExists)
			}
			continue
		}
		if legacyExists || !targetExists {
			t.Fatalf("B2 post-rename table state %s -> %s: legacy=%t target=%t", mapping.legacy, mapping.target, legacyExists, targetExists)
		}
		var rows int64
		if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM `+mapping.target).Scan(&rows); err != nil {
			t.Fatalf("count B2 target table %s: %v", mapping.target, err)
		}
		if rows < 1 {
			t.Fatalf("B2 target table %s lost all rows", mapping.target)
		}
	}
}
