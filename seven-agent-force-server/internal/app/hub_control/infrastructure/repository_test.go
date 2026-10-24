package infrastructure

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/CaixyPromise/seven-agent/force/internal/infrastructure/datasource/store"
	"github.com/DATA-DOG/go-sqlmock"
	"github.com/jmoiron/sqlx"
)

func TestRepositoryInsertMatchesColumnsPlaceholdersAndArguments(t *testing.T) {
	rawDB, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = rawDB.Close() })
	db := sqlx.NewDb(rawDB, "mysql")
	repository, err := NewRepository(sqlmockProvider{db: db})
	if err != nil {
		t.Fatal(err)
	}

	now := time.Date(2026, 7, 12, 12, 0, 0, 0, time.UTC)
	node := &NodeRecord{
		ID: 1, NodeCode: "node-a", NodeName: "Node A", Status: 0, DiscoveryType: "STATIC",
		ManagementBaseURL: "https://node.example.com:9443", HubIssuer: "https://hub.example.com",
		ManagementBearer: EncryptedValue{Ciphertext: "bearer-cipher", EDEK: "bearer-edek", WrapKeyRef: "bearer-key"},
		ConnectionStatus: "PENDING", TargetRevision: 1, CreatedAt: now, UpdatedAt: now,
	}

	columns := strings.Split(strings.ReplaceAll(nodeColumns, "\n", ""), ",")
	if len(columns) != 26 {
		t.Fatalf("nodeColumns=%d, want 26", len(columns))
	}
	if got := len(nodeArgs(node)); got != 26 {
		t.Fatalf("nodeArgs=%d, want 26", got)
	}
	placeholders := strings.TrimSuffix(strings.Repeat("?, ", 26), ", ")
	expectedSQL := `INSERT INTO sys_federated_node (` + nodeColumns + `, isDeleted) VALUES (` + placeholders + `, 0)`
	args := nodeArgs(node)
	driverArgs := make([]driver.Value, len(args))
	for index := range args {
		driverArgs[index] = args[index]
	}
	mock.ExpectExec(regexp.QuoteMeta(expectedSQL)).WithArgs(driverArgs...).WillReturnResult(sqlmock.NewResult(1, 1))

	if err := repository.Insert(context.Background(), node); err != nil {
		t.Fatal(err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestRepositoryWritersUpdateOnlyOwnedColumns(t *testing.T) {
	rawDB, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = rawDB.Close() })
	db := sqlx.NewDb(rawDB, "mysql")
	repository, err := NewRepository(sqlmockProvider{db: db})
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	node := &NodeRecord{
		ID: 1, NodeName: "Node A", Status: 1, DiscoveryType: "STATIC", ManagementBaseURL: "https://node.example.com:9443",
		HubIssuer: "https://hub.example.com", CapabilitiesJSON: `{}`,
		ManagementBearer: EncryptedValue{Ciphertext: "bearer-cipher", EDEK: "bearer-edek", WrapKeyRef: "bearer-key"},
		OIDCClientID:     "client-a", OIDCClientSecret: EncryptedValue{Ciphertext: "oidc-cipher", EDEK: "oidc-edek", WrapKeyRef: "oidc-key"},
		ConnectionStatus: "ACTIVE", ConnectionVersion: "v2", ConnectionRequestHash: "hash-v2", IssuerLockedAt: &now,
		LastConnectionError: "", LastConnectionTraceID: "", LastHealthyAt: &now, UpdatedAt: now,
	}

	tests := []struct {
		name  string
		query string
		call  func() error
	}{
		{name: "metadata", query: `UPDATE sys_federated_node SET nodeName=?, discoveryType=?, serviceName=?, managementBaseUrl=?, hubIssuer=?, capabilitiesJson=?, updatedAt=? WHERE id=? AND isDeleted=0`, call: func() error { return repository.UpdateMetadata(context.Background(), node) }},
		{name: "management bearer", query: `UPDATE sys_federated_node SET managementBearerCiphertext=?, managementBearerEdek=?, managementBearerWrapKeyRef=?, updatedAt=? WHERE id=? AND isDeleted=0`, call: func() error { return repository.ReplaceManagementBearer(context.Background(), node) }},
		{name: "status", query: `UPDATE sys_federated_node SET status=?, updatedAt=? WHERE id=? AND isDeleted=0`, call: func() error { return repository.UpdateStatus(context.Background(), node) }},
		{name: "target state", query: `UPDATE sys_federated_node SET targetRevision=?, connectionStatus=?, lastConnectionError=?, lastConnectionTraceId=?, updatedAt=? WHERE id=? AND isDeleted=0`, call: func() error { return repository.UpdateTargetState(context.Background(), node) }},
		{name: "health", query: `UPDATE sys_federated_node SET lastHealthyAt=?, updatedAt=? WHERE id=? AND isDeleted=0`, call: func() error { return repository.UpdateHealth(context.Background(), node) }},
		{name: "connection", query: `UPDATE sys_federated_node SET oidcClientId=?, oidcClientSecretCiphertext=?, oidcClientSecretEdek=?, oidcClientSecretWrapKeyRef=?, connectionStatus=?, connectionVersion=?, connectionRequestHash=?, issuerLockedAt=?, lastConnectionError=?, lastConnectionTraceId=?, updatedAt=? WHERE id=? AND isDeleted=0`, call: func() error { return repository.UpdateConnection(context.Background(), node) }},
	}
	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			mock.ExpectExec(regexp.QuoteMeta(testCase.query)).WillReturnResult(sqlmock.NewResult(0, 1))
			if err := testCase.call(); err != nil {
				t.Fatal(err)
			}
		})
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestRepositoryConnectionCommandLedgerReadAndTerminalUpdate(t *testing.T) {
	rawDB, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = rawDB.Close() })
	repository, err := NewRepository(sqlmockProvider{db: sqlx.NewDb(rawDB, "mysql")})
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	mock.ExpectQuery("SELECT nodeCode, connectionVersion, requestHash, targetRevision, terminalState AS state, createdAt, updatedAt").WithArgs("node-a", "v1").WillReturnRows(sqlmock.NewRows([]string{"nodeCode", "connectionVersion", "requestHash", "targetRevision", "state", "createdAt", "updatedAt"}).AddRow("node-a", "v1", strings.Repeat("a", 64), int64(3), "PENDING", now, now))
	command, err := repository.FindConnectionCommandForUpdate(context.Background(), "node-a", "v1")
	if err != nil || command == nil {
		t.Fatalf("command=%+v err=%v", command, err)
	}
	command.State = "ACTIVE"
	mock.ExpectExec("INSERT INTO sys_federated_node_connection_command").WithArgs("node-a", "v1", strings.Repeat("a", 64), int64(3), "ACTIVE", now, now).WillReturnResult(sqlmock.NewResult(0, 1))
	if err := repository.SaveConnectionCommand(context.Background(), command); err != nil {
		t.Fatal(err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

type sqlmockProvider struct{ db *sqlx.DB }

func (p sqlmockProvider) Driver() string               { return "mysql" }
func (p sqlmockProvider) Dialect() string              { return "mysql" }
func (p sqlmockProvider) DB() *sql.DB                  { return p.db.DB }
func (p sqlmockProvider) SQLX() *sqlx.DB               { return p.db }
func (p sqlmockProvider) Transactor() store.Transactor { return store.NewSQLXTransactor(p.db) }
func (p sqlmockProvider) Configured() bool             { return true }
func (p sqlmockProvider) Close() error                 { return nil }
