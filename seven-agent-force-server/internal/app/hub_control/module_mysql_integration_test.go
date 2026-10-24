package hub_control

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/CaixyPromise/seven-agent/force/internal/app/hub_control/application"
	"github.com/CaixyPromise/seven-agent/force/internal/app/hub_control/domain"
	hubfacade "github.com/CaixyPromise/seven-agent/force/internal/app/hub_control/facade"
	hubinfra "github.com/CaixyPromise/seven-agent/force/internal/app/hub_control/infrastructure"
	"github.com/CaixyPromise/seven-agent/force/internal/infrastructure/datasource/mysql"
	"github.com/CaixyPromise/seven-agent/force/internal/shared/config"
)

func TestMySQLRepositoryCreateAndCopy(t *testing.T) {
	dsn := os.Getenv("TASK6_MYSQL_DSN")
	if dsn == "" {
		t.Skip("TASK6_MYSQL_DSN is not set")
	}

	provider, err := mysql.NewProvider(config.MySQLConfig{Enabled: true, DSN: dsn, MaxOpenConns: 4, MaxIdleConns: 2}, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = provider.Close() })
	repository, err := hubinfra.NewRepository(provider)
	if err != nil {
		t.Fatal(err)
	}

	nextID := int64(9000)
	service := application.NewService(repositoryAdapter{repository}, nil, nil, mysqlProbeSecrets{}, func() int64 {
		nextID++
		return nextID
	})
	service.BindTransactor(provider.Transactor())
	ctx := context.Background()
	_, err = service.SaveNode(ctx, hubfacade.SaveNodeCommand{
		NodeCode:          "scratch-source",
		NodeName:          "Scratch Source",
		Status:            domain.NodeStatusEnabled,
		DiscoveryType:     "STATIC",
		ManagementBaseURL: "https://node.example.com:9443",
		HubIssuer:         "https://hub.example.com",
		ManagementBearer:  "source-bearer",
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = provider.SQLX().ExecContext(ctx, `UPDATE sysFederatedNode SET oidcClientId=?, oidcClientSecretCiphertext=?, oidcClientSecretEdek=?, oidcClientSecretWrapKeyRef=?, connectionStatus=? WHERE nodeCode=?`, "source-client", "source-secret-cipher", "source-secret-edek", "source-secret-key", domain.ConnectionActive, "scratch-source"); err != nil {
		t.Fatal(err)
	}

	_, err = service.CopyNode(ctx, "scratch-source", hubfacade.CopyNodeCommand{NodeCode: "scratch-copy", NodeName: "Scratch Copy"})
	if err != nil {
		t.Fatal(err)
	}

	var rowCount int
	if err = provider.SQLX().GetContext(ctx, &rowCount, `SELECT COUNT(*) FROM sysFederatedNode WHERE nodeCode IN (?, ?) AND isDeleted=0`, "scratch-source", "scratch-copy"); err != nil {
		t.Fatal(err)
	}
	if rowCount != 2 {
		t.Fatalf("created rows=%d, want 2", rowCount)
	}
	copyRecord, err := repository.Find(ctx, "scratch-copy")
	if err != nil {
		t.Fatal(err)
	}
	if copyRecord == nil {
		t.Fatal("scratch copy was not persisted")
	}
	if copyRecord.Status != domain.NodeStatusDisabled || copyRecord.ConnectionStatus != domain.ConnectionPending {
		t.Fatalf("copy status=%d connectionStatus=%s, want disabled/PENDING", copyRecord.Status, copyRecord.ConnectionStatus)
	}
	if copyRecord.OIDCClientID != "" || copyRecord.OIDCClientSecret != (hubinfra.EncryptedValue{}) {
		t.Fatalf("copy retained OIDC client material: clientId=%q secret=%+v", copyRecord.OIDCClientID, copyRecord.OIDCClientSecret)
	}
	if copyRecord.TargetRevision != 1 {
		t.Fatalf("copy targetRevision=%d want 1", copyRecord.TargetRevision)
	}

	now := time.Now().UTC()
	command := &hubinfra.ConnectionCommandRecord{NodeCode: "scratch-source", ConnectionVersion: "v1", RequestHash: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", TargetRevision: 1, State: domain.CommandPending, CreatedAt: now, UpdatedAt: now}
	if err := repository.SaveConnectionCommand(ctx, command); err != nil {
		t.Fatal(err)
	}
	reloadedRepository, err := hubinfra.NewRepository(provider)
	if err != nil {
		t.Fatal(err)
	}
	reloaded, err := reloadedRepository.FindConnectionCommandForUpdate(ctx, command.NodeCode, command.ConnectionVersion)
	if err != nil || reloaded == nil || reloaded.RequestHash != command.RequestHash || reloaded.TargetRevision != 1 || reloaded.State != domain.CommandPending {
		t.Fatalf("reloaded command=%+v err=%v", reloaded, err)
	}
}

type mysqlProbeSecrets struct{}

func (mysqlProbeSecrets) Encrypt(_ context.Context, plaintext string) (domain.EncryptedSecret, error) {
	return domain.EncryptedSecret{Ciphertext: "cipher:" + plaintext, EDEK: "scratch-edek", WrapKeyRef: "scratch-key"}, nil
}

func (mysqlProbeSecrets) Decrypt(_ context.Context, value domain.EncryptedSecret) (string, error) {
	return value.Ciphertext, nil
}
