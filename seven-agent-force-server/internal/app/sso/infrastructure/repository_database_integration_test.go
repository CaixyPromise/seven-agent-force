package infrastructure

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/CaixyPromise/seven-agent/force/internal/app/sso/domain"
	dbgovernance "github.com/CaixyPromise/seven-agent/force/internal/infrastructure/datasource/governance"
	"github.com/CaixyPromise/seven-agent/force/internal/infrastructure/datasource/store"
	_ "github.com/go-sql-driver/mysql"
	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/jmoiron/sqlx"
)

type dg2SSOIntegrationProvider struct {
	driver  string
	dialect string
	db      *sql.DB
	sqlxDB  *sqlx.DB
}

func (p *dg2SSOIntegrationProvider) Driver() string               { return p.driver }
func (p *dg2SSOIntegrationProvider) Dialect() string              { return p.dialect }
func (p *dg2SSOIntegrationProvider) DB() *sql.DB                  { return p.db }
func (p *dg2SSOIntegrationProvider) SQLX() *sqlx.DB               { return p.sqlxDB }
func (p *dg2SSOIntegrationProvider) Transactor() store.Transactor { return nil }
func (p *dg2SSOIntegrationProvider) Configured() bool             { return true }
func (p *dg2SSOIntegrationProvider) Close() error                 { return p.db.Close() }

func TestSSORepositoryDatabaseDialectAcceptance(t *testing.T) {
	dialect := strings.TrimSpace(os.Getenv("DG2_TEST_DIALECT"))
	dsn := strings.TrimSpace(os.Getenv("DG2_TEST_DSN"))
	if dialect == "" || dsn == "" {
		t.Skip("set DG2_TEST_DIALECT and DG2_TEST_DSN for the exact isolated governance database")
	}
	driver := "mysql"
	if strings.EqualFold(dialect, "postgres") {
		driver = "pgx"
	}
	db, err := sql.Open(driver, dsn)
	if err != nil {
		t.Fatalf("open %s: %v", dialect, err)
	}
	if err := dbgovernance.AssertConnectedDatabase(context.Background(), db, dialect); err != nil {
		_ = db.Close()
		t.Fatal(err)
	}
	provider := &dg2SSOIntegrationProvider{
		driver: driver, dialect: dialect, db: db, sqlxDB: sqlx.NewDb(db, driver),
	}
	t.Cleanup(func() { _ = provider.Close() })
	repo, err := NewRepository(provider)
	if err != nil {
		t.Fatalf("new sso repository: %v", err)
	}

	ctx := context.Background()
	suffix := time.Now().UTC().UnixNano()
	clientID := fmt.Sprintf("dg2-sso-%d", suffix)
	providerCode := fmt.Sprintf("dg2-provider-%d", suffix)
	userID := suffix
	rollback := errors.New("dg2 sso rollback")
	err = store.NewSQLXTransactor(provider.sqlxDB).WithinTransaction(ctx, func(txCtx context.Context) error {
		if err := repo.InsertClient(txCtx, &domain.Client{
			ClientID:           clientID,
			ClientName:         "DG2 SSO Dialect Fixture",
			ClientType:         "CONFIDENTIAL",
			ClientAuthMethod:   "client_secret_basic",
			GrantTypes:         []string{"authorization_code", "refresh_token"},
			Scopes:             []string{"openid", "profile"},
			RequirePKCE:        true,
			AccessTokenTTLSec:  1800,
			RefreshTokenTTLSec: 2592000,
			Status:             domain.ClientStatusActive,
			MetadataJSON:       `{"fixture":"dg2"}`,
		}, 0); err != nil {
			return err
		}

		redirects := []domain.ClientRedirectURI{
			{
				RedirectURI:           "https://a.example.test/callback",
				PostLogoutRedirectURI: "https://a.example.test/logout",
				Status:                domain.ClientStatusActive,
			},
			{
				RedirectURI:           "https://b.example.test/callback",
				PostLogoutRedirectURI: "https://b.example.test/logout",
				Status:                domain.ClientStatusActive,
			},
		}
		if err := repo.ReplaceClientRedirectURIs(txCtx, clientID, redirects, 0, time.Now().UTC()); err != nil {
			return err
		}
		storedRedirects, err := repo.ListClientRedirectURIs(txCtx, clientID)
		if err != nil || len(storedRedirects) != 2 {
			return fmt.Errorf("redirect readback rows=%#v: %w", storedRedirects, err)
		}
		for index := range redirects {
			if storedRedirects[index].RedirectURI != redirects[index].RedirectURI ||
				storedRedirects[index].PostLogoutRedirectURI != redirects[index].PostLogoutRedirectURI {
				return fmt.Errorf("redirect readback mismatch got=%#v want=%#v", storedRedirects, redirects)
			}
		}

		firstSecretID, secondSecretID := suffix+1, suffix+2
		for _, secret := range []domain.ClientSecret{
			{ID: firstSecretID, ClientID: clientID, SecretHash: "dg2-hash-a", SecretHint: "a", Status: domain.ClientStatusActive},
			{ID: secondSecretID, ClientID: clientID, SecretHash: "dg2-hash-b", SecretHint: "b", Status: domain.ClientStatusActive},
		} {
			item := secret
			if err := repo.InsertClientSecret(txCtx, &item, 0); err != nil {
				return err
			}
		}
		if affected, err := repo.DisableOtherActiveClientSecrets(txCtx, clientID, secondSecretID, 0, time.Now().UTC()); err != nil || affected != 1 {
			return fmt.Errorf("disable previous secrets affected=%d: %w", affected, err)
		}
		secrets, err := repo.ListClientSecrets(txCtx, clientID)
		if err != nil || len(secrets) != 2 {
			return fmt.Errorf("secret readback rows=%#v: %w", secrets, err)
		}

		consent := &domain.ConsentGrant{
			UserID: userID, ClientID: clientID, Scopes: []string{"openid"},
			GrantedAt: time.Now().UTC(), Status: domain.ConsentStatusActive,
		}
		if err := repo.UpsertConsentGrant(txCtx, consent); err != nil {
			return err
		}
		consent.Scopes = []string{"openid", "profile"}
		if err := repo.UpsertConsentGrant(txCtx, consent); err != nil {
			return err
		}
		storedConsent, err := repo.FindConsentGrant(txCtx, userID, clientID)
		if err != nil || storedConsent == nil || len(storedConsent.Scopes) != 2 {
			return fmt.Errorf("consent readback=%#v: %w", storedConsent, err)
		}

		now := time.Now().UTC()
		sessionIDs := []string{fmt.Sprintf("dg2-session-a-%d", suffix), fmt.Sprintf("dg2-session-b-%d", suffix)}
		tokenHashes := make([]string, 0, len(sessionIDs))
		for index, sessionID := range sessionIDs {
			if err := repo.InsertSession(txCtx, &domain.Session{
				SessionID:            sessionID,
				UserID:               userID + int64(index),
				ClientID:             clientID,
				PlatformCode:         "seven-admin",
				LoginMethod:          "EXTERNAL_OAUTH",
				ExternalProviderCode: providerCode,
				LoginAt:              now,
				ExpiresAt:            now.Add(time.Hour),
				Status:               domain.SessionStatusActive,
			}); err != nil {
				return err
			}
			tokenHash := fmt.Sprintf("dg2-token-%d-%d", suffix, index)
			tokenHashes = append(tokenHashes, tokenHash)
			if err := repo.InsertRefreshTokenFamily(txCtx, &domain.RefreshTokenFamily{
				FamilyID:         fmt.Sprintf("dg2-family-%d-%d", suffix, index),
				SessionID:        sessionID,
				ClientID:         clientID,
				UserID:           userID + int64(index),
				CurrentTokenHash: tokenHash,
				ExpiresAt:        now.Add(time.Hour),
				Status:           domain.RefreshFamilyStatusActive,
			}); err != nil {
				return err
			}
		}
		cutoff, err := repo.CaptureManagedSessionCutoff(txCtx)
		if err != nil {
			return err
		}
		page, err := repo.ListActiveSessionsByExternalProviderPage(txCtx, providerCode, cutoff, 0, 100)
		if err != nil || len(page) != 2 {
			return fmt.Errorf("bounded session page=%#v: %w", page, err)
		}
		if err := repo.RevokeRefreshFamiliesByExternalProvider(txCtx, providerCode, cutoff); err != nil {
			return err
		}
		for _, tokenHash := range tokenHashes {
			family, err := repo.FindRefreshFamilyByCurrentHash(txCtx, tokenHash)
			if err != nil || family == nil || family.Status != domain.RefreshFamilyStatusRevoked {
				return fmt.Errorf("revoked refresh family readback=%#v: %w", family, err)
			}
		}
		if affected, err := repo.RevokeSessionsByExternalProvider(txCtx, providerCode, cutoff); err != nil || affected != 2 {
			return fmt.Errorf("revoke sessions affected=%d: %w", affected, err)
		}
		for _, sessionID := range sessionIDs {
			session, err := repo.FindSessionBySessionID(txCtx, sessionID)
			if err != nil || session == nil || session.Status != domain.SessionStatusRevoked {
				return fmt.Errorf("revoked session readback=%#v: %w", session, err)
			}
		}
		return rollback
	})
	if !errors.Is(err, rollback) {
		t.Fatalf("SSO acceptance must reach deliberate rollback, got %v", err)
	}
	if item, err := repo.FindClientDetail(ctx, clientID); err != nil || item != nil {
		t.Fatalf("rollback leaked client item=%#v err=%v", item, err)
	}
}
