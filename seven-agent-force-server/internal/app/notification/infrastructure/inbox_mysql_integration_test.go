package infrastructure

import (
	"context"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/CaixyPromise/seven-agent/force/internal/app/notification/domain"
	dbgovernance "github.com/CaixyPromise/seven-agent/force/internal/infrastructure/datasource/governance"
	"github.com/CaixyPromise/seven-agent/force/internal/infrastructure/datasource/mysql"
	"github.com/CaixyPromise/seven-agent/force/internal/infrastructure/datasource/postgres"
	"github.com/CaixyPromise/seven-agent/force/internal/shared/config"
	mysqldriver "github.com/go-sql-driver/mysql"
)

// TestMailboxSequencerMySQLIntegration exercises the actual row lock and
// transaction boundary. It is opt-in because it writes only to an explicitly
// named isolated database prepared by the caller.
func TestMailboxSequencerMySQLIntegration(t *testing.T) {
	dsn := strings.TrimSpace(os.Getenv("NOTIFICATION_MYSQL_DSN"))
	if dsn == "" || os.Getenv("NOTIFICATION_MYSQL_ALLOW_MUTATION") != "1" {
		t.Skip("NOTIFICATION_MYSQL_DSN and NOTIFICATION_MYSQL_ALLOW_MUTATION=1 are required")
	}
	parsed, err := mysqldriver.ParseDSN(dsn)
	if err != nil {
		t.Fatalf("parse MySQL DSN: %v", err)
	}
	if err := dbgovernance.ValidateIsolatedDatabaseName("mysql", parsed.DBName); err != nil {
		t.Fatal(err)
	}
	provider, err := mysql.NewProvider(config.MySQLConfig{
		Enabled: true, DSN: dsn, MaxOpenConns: 4, MaxIdleConns: 2,
	}, nil)
	if err != nil {
		t.Fatalf("open MySQL provider: %v", err)
	}
	t.Cleanup(func() { _ = provider.Close() })
	if err := dbgovernance.AssertConnectedDatabase(context.Background(), provider.DB(), "mysql"); err != nil {
		t.Fatal(err)
	}

	const (
		scopeID = "g3-integration"
		userID  = int64(920042)
	)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	db := provider.DB()
	if _, err := db.ExecContext(ctx, `DELETE FROM sys_notification_recipient WHERE scopeId=? AND userId=?`, scopeID, userID); err != nil {
		t.Fatalf("clear recipients: %v", err)
	}
	if _, err := db.ExecContext(ctx, `DELETE FROM sys_notification_mailbox WHERE scopeId=? AND userId=?`, scopeID, userID); err != nil {
		t.Fatalf("clear mailbox: %v", err)
	}
	t.Cleanup(func() {
		_, _ = db.ExecContext(context.Background(), `DELETE FROM sys_notification_recipient WHERE scopeId=? AND userId=?`, scopeID, userID)
		_, _ = db.ExecContext(context.Background(), `DELETE FROM sys_notification_mailbox WHERE scopeId=? AND userId=?`, scopeID, userID)
	})

	repository := NewRepository(provider.SQLX())
	transactor := provider.Transactor()
	recipients := []domain.Recipient{
		integrationRecipient(940001, "nrc_g3_1", 930001, scopeID, userID),
		integrationRecipient(940002, "nrc_g3_2", 930002, scopeID, userID),
	}
	if err := transactor.WithinTransaction(ctx, func(txCtx context.Context) error {
		created, insertErr := repository.InsertInboxRecipients(txCtx, recipients)
		if insertErr != nil {
			return insertErr
		}
		if len(created) != 2 || created[0].MailboxVersion != 1 || created[1].MailboxVersion != 2 {
			return fmt.Errorf("new recipient sequences=%#v", created)
		}
		return nil
	}); err != nil {
		t.Fatalf("insert serialized recipients: %v", err)
	}

	if err := transactor.WithinTransaction(ctx, func(txCtx context.Context) error {
		recipient, findErr := repository.FindInboxRecipient(txCtx, scopeID, userID, recipients[0].RecipientID)
		if findErr != nil || recipient == nil {
			return fmt.Errorf("find recipient: %w", findErr)
		}
		prior := recipient.MailboxVersion
		changed, actionErr := recipient.ApplyInboxAction(domain.InboxActionRead, time.Now().UTC())
		if actionErr != nil || !changed {
			return fmt.Errorf("read action changed=%t err=%w", changed, actionErr)
		}
		mailbox, advanceErr := repository.AdvanceMailboxChange(txCtx, scopeID, userID)
		if advanceErr != nil {
			return advanceErr
		}
		if err := recipient.SetMailboxVersion(mailbox.ChangeSequence); err != nil {
			return err
		}
		updated, updateErr := repository.CompareAndSetInboxRecipient(txCtx, recipient, prior)
		if updateErr != nil || !updated {
			return fmt.Errorf("compare-and-set updated=%t err=%w", updated, updateErr)
		}
		if mailbox.ChangeSequence != 3 {
			return fmt.Errorf("read sequence=%d, want 3", mailbox.ChangeSequence)
		}
		return nil
	}); err != nil {
		t.Fatalf("mutate recipient with serialized sequence: %v", err)
	}

	firstLocked := make(chan struct{})
	releaseFirst := make(chan struct{})
	firstResult := make(chan int64, 1)
	secondResult := make(chan int64, 1)
	errs := make(chan error, 2)
	go func() {
		errs <- transactor.WithinTransaction(ctx, func(txCtx context.Context) error {
			mailbox, advanceErr := repository.AdvanceMailboxChange(txCtx, scopeID, userID)
			if advanceErr != nil {
				return advanceErr
			}
			firstResult <- mailbox.ChangeSequence
			close(firstLocked)
			select {
			case <-releaseFirst:
				return nil
			case <-txCtx.Done():
				return txCtx.Err()
			}
		})
	}()
	select {
	case <-firstLocked:
	case <-ctx.Done():
		t.Fatal("first sequencer transaction did not acquire mailbox lock")
	}
	go func() {
		errs <- transactor.WithinTransaction(ctx, func(txCtx context.Context) error {
			mailbox, advanceErr := repository.AdvanceMailboxChange(txCtx, scopeID, userID)
			if advanceErr == nil {
				secondResult <- mailbox.ChangeSequence
			}
			return advanceErr
		})
	}()
	select {
	case sequence := <-secondResult:
		t.Fatalf("second transaction advanced to %d before first committed", sequence)
	case <-time.After(75 * time.Millisecond):
	}
	close(releaseFirst)
	for range 2 {
		if err := <-errs; err != nil {
			t.Fatalf("concurrent mailbox sequence transaction: %v", err)
		}
	}
	if first := <-firstResult; first != 4 {
		t.Fatalf("first concurrent sequence=%d, want 4", first)
	}
	if second := <-secondResult; second != 5 {
		t.Fatalf("second concurrent sequence=%d, want 5", second)
	}

	if err := transactor.WithinTransaction(ctx, func(txCtx context.Context) error {
		mailbox, lockErr := repository.LockMailbox(txCtx, scopeID, userID, "unused-key")
		if lockErr != nil {
			return lockErr
		}
		if mailbox.ChangeSequence != 5 {
			return fmt.Errorf("mailbox head=%d, want 5", mailbox.ChangeSequence)
		}
		return nil
	}); err != nil {
		t.Fatalf("read mailbox head: %v", err)
	}
}

func integrationRecipient(id int64, recipientID string, notificationID int64, scopeID string, userID int64) domain.Recipient {
	return domain.Recipient{
		ID:             id,
		RecipientID:    recipientID,
		NotificationID: notificationID,
		ScopeID:        scopeID,
		UserID:         userID,
		EventKey:       "account.security.changed",
		Category:       "ACCOUNT",
		Priority:       "NORMAL",
		Title:          "账号安全设置已更新",
		Content:        "完整正文",
	}
}

// TestMailboxSequencerPostgresIntegration proves that the quoted camelCase
// mailbox queries execute against PostgreSQL after the additive migration.
func TestMailboxSequencerPostgresIntegration(t *testing.T) {
	dsn := strings.TrimSpace(os.Getenv("NOTIFICATION_POSTGRES_DSN"))
	if dsn == "" || os.Getenv("NOTIFICATION_POSTGRES_ALLOW_MUTATION") != "1" {
		t.Skip("NOTIFICATION_POSTGRES_DSN and NOTIFICATION_POSTGRES_ALLOW_MUTATION=1 are required")
	}
	provider, err := postgres.NewProvider(config.PostgresConfig{
		Enabled: true, DSN: dsn, MaxOpenConns: 4, MaxIdleConns: 2,
	}, nil)
	if err != nil {
		t.Fatalf("open PostgreSQL provider: %v", err)
	}
	t.Cleanup(func() { _ = provider.Close() })
	if err := dbgovernance.AssertConnectedDatabase(context.Background(), provider.DB(), "postgres"); err != nil {
		t.Fatal(err)
	}

	const (
		scopeID = "g3-postgres-integration"
		userID  = int64(920043)
	)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	db := provider.DB()
	if _, err := db.ExecContext(ctx, `DELETE FROM "sys_notification_recipient" WHERE "scopeId"=$1 AND "userId"=$2`, scopeID, userID); err != nil {
		t.Fatalf("clear PostgreSQL recipients: %v", err)
	}
	if _, err := db.ExecContext(ctx, `DELETE FROM "sys_notification_mailbox" WHERE "scopeId"=$1 AND "userId"=$2`, scopeID, userID); err != nil {
		t.Fatalf("clear PostgreSQL mailbox: %v", err)
	}
	if _, err := db.ExecContext(ctx, `DELETE FROM "sys_notification" WHERE "scopeId"=$1 AND "idempotencyKey"=$2`, scopeID, "g3-postgres-integration"); err != nil {
		t.Fatalf("clear PostgreSQL logical notification: %v", err)
	}
	t.Cleanup(func() {
		_, _ = db.ExecContext(context.Background(), `DELETE FROM "sys_notification_recipient" WHERE "scopeId"=$1 AND "userId"=$2`, scopeID, userID)
		_, _ = db.ExecContext(context.Background(), `DELETE FROM "sys_notification_mailbox" WHERE "scopeId"=$1 AND "userId"=$2`, scopeID, userID)
		_, _ = db.ExecContext(context.Background(), `DELETE FROM "sys_notification" WHERE "scopeId"=$1 AND "idempotencyKey"=$2`, scopeID, "g3-postgres-integration")
	})

	repository := NewRepository(provider.SQLX())
	if err := provider.Transactor().WithinTransaction(ctx, func(txCtx context.Context) error {
		logical := &domain.LogicalNotification{
			ID:                 950101,
			NotificationID:     "ntf_g3_pg_1",
			ScopeID:            scopeID,
			EventKey:           "account.security.changed",
			IdempotencyKey:     "g3-postgres-integration",
			RequestFingerprint: strings.Repeat("a", 64),
			AudienceJSON:       `{"userIds":[920043]}`,
			Category:           "ACCOUNT",
			Priority:           "NORMAL",
			Title:              "账号安全设置已更新",
			Content:            "完整正文",
			Status:             domain.NotificationStatusMaterialized,
		}
		logicalCreated, logicalErr := repository.CreateLogicalNotification(txCtx, logical)
		if logicalErr != nil || !logicalCreated {
			return fmt.Errorf("PostgreSQL logical notification created=%t err=%w", logicalCreated, logicalErr)
		}
		item := integrationRecipient(950001, "nrc_g3_pg_1", 950101, scopeID, userID)
		created, insertErr := repository.InsertInboxRecipients(txCtx, []domain.Recipient{item})
		if insertErr != nil {
			return insertErr
		}
		if len(created) != 1 || created[0].MailboxVersion != 1 {
			return fmt.Errorf("PostgreSQL inserted recipient=%#v", created)
		}
		page, pageErr := repository.ListInboxRecipients(txCtx, domain.InboxQuery{ScopeID: scopeID, UserID: userID, Limit: 20})
		if pageErr != nil || len(page) != 1 || page[0].RecipientID != item.RecipientID || page[0].MailboxVersion != 1 {
			return fmt.Errorf("PostgreSQL inbox page=%#v err=%w", page, pageErr)
		}
		count, countErr := repository.CountUnreadInboxRecipients(txCtx, scopeID, userID)
		if countErr != nil || count != 1 {
			return fmt.Errorf("PostgreSQL unread count=%d err=%w", count, countErr)
		}
		changes, changesErr := repository.ListInboxRecipientChanges(txCtx, domain.InboxChangeQuery{
			ScopeID: scopeID, UserID: userID, AfterSequence: 0, UntilSequence: 1, Limit: 20,
		})
		if changesErr != nil || len(changes) != 1 || changes[0].RecipientID != item.RecipientID {
			return fmt.Errorf("PostgreSQL inbox changes=%#v err=%w", changes, changesErr)
		}
		advanced, advanceErr := repository.AdvanceMailboxChange(txCtx, scopeID, userID)
		if advanceErr != nil {
			return advanceErr
		}
		if advanced.ChangeSequence != 2 || advanced.MailboxKey == "" {
			return fmt.Errorf("PostgreSQL mailbox advance=%#v", advanced)
		}
		locked, lockErr := repository.LockMailbox(txCtx, scopeID, userID, "unused-key")
		if lockErr != nil {
			return lockErr
		}
		if locked.ChangeSequence != 2 || locked.MailboxKey != advanced.MailboxKey {
			return fmt.Errorf("PostgreSQL mailbox lock=%#v", locked)
		}
		return nil
	}); err != nil {
		t.Fatalf("PostgreSQL mailbox sequencer: %v", err)
	}
}
