package infrastructure

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"

	"github.com/CaixyPromise/seven-agent/force/internal/infrastructure/datasource/store"
	"github.com/jmoiron/sqlx"
)

type Repository struct {
	db       store.SQLX
	postgres bool
}

// EncryptedValue is the adapter-local persisted envelope representation.
type EncryptedValue struct {
	Ciphertext string
	EDEK       string
	WrapKeyRef string
}

// NodeRecord is the adapter-local persistence record for sys_federated_node.
type NodeRecord struct {
	ID                    int64
	NodeCode              string
	NodeName              string
	Status                int
	DiscoveryType         string
	ServiceName           string
	ManagementBaseURL     string
	HubIssuer             string
	OIDCClientID          string
	OIDCClientSecret      EncryptedValue
	ManagementBearer      EncryptedValue
	CapabilitiesJSON      string
	ConnectionStatus      string
	ConnectionVersion     string
	ConnectionRequestHash string
	TargetRevision        int64
	IssuerLockedAt        *time.Time
	LastConnectionError   string
	LastConnectionTraceID string
	LastHealthyAt         *time.Time
	CreatedAt             time.Time
	UpdatedAt             time.Time
}

type NodePageQuery struct {
	Current int
	Size    int
	Keyword string
	Status  *int
}

func NewRepository(provider store.Provider) (*Repository, error) {
	if provider == nil {
		return nil, fmt.Errorf("hub control repository requires datasource provider")
	}
	dialect := strings.ToLower(strings.TrimSpace(provider.Dialect()))
	return &Repository{
		db:       provider.SQLX(),
		postgres: dialect == "postgres" || dialect == "postgresql" || dialect == "pgx",
	}, nil
}

func (r *Repository) rebind(exec store.SQLX, query string) string {
	if r.postgres {
		query = hubControlPostgresRenderer.RenderPostgres(query)
	}
	return exec.Rebind(query)
}

type nodeRow struct {
	ID                    int64          `db:"id"`
	NodeCode              string         `db:"nodeCode"`
	NodeName              string         `db:"nodeName"`
	Status                int            `db:"status"`
	DiscoveryType         string         `db:"discoveryType"`
	ServiceName           sql.NullString `db:"serviceName"`
	ManagementBaseURL     sql.NullString `db:"managementBaseUrl"`
	HubIssuer             string         `db:"hubIssuer"`
	OIDCClientID          sql.NullString `db:"oidcClientId"`
	OIDCCiphertext        sql.NullString `db:"oidcClientSecretCiphertext"`
	OIDCEDEK              sql.NullString `db:"oidcClientSecretEdek"`
	OIDCWrapKeyRef        sql.NullString `db:"oidcClientSecretWrapKeyRef"`
	BearerCiphertext      sql.NullString `db:"managementBearerCiphertext"`
	BearerEDEK            sql.NullString `db:"managementBearerEdek"`
	BearerWrapKeyRef      sql.NullString `db:"managementBearerWrapKeyRef"`
	CapabilitiesJSON      sql.NullString `db:"capabilitiesJson"`
	ConnectionStatus      string         `db:"connectionStatus"`
	ConnectionVersion     sql.NullString `db:"connectionVersion"`
	ConnectionRequestHash sql.NullString `db:"connectionRequestHash"`
	TargetRevision        int64          `db:"targetRevision"`
	IssuerLockedAt        sql.NullTime   `db:"issuerLockedAt"`
	LastConnectionError   sql.NullString `db:"lastConnectionError"`
	LastConnectionTraceID sql.NullString `db:"lastConnectionTraceId"`
	LastHealthyAt         sql.NullTime   `db:"lastHealthyAt"`
	CreatedAt             time.Time      `db:"createdAt"`
	UpdatedAt             time.Time      `db:"updatedAt"`
}

const nodeColumns = `id, nodeCode, nodeName, status, discoveryType, serviceName, managementBaseUrl, hubIssuer, oidcClientId,
oidcClientSecretCiphertext, oidcClientSecretEdek, oidcClientSecretWrapKeyRef,
managementBearerCiphertext, managementBearerEdek, managementBearerWrapKeyRef,
capabilitiesJson, connectionStatus, connectionVersion, connectionRequestHash, targetRevision, issuerLockedAt, lastConnectionError, lastConnectionTraceId, lastHealthyAt, createdAt, updatedAt`

func (r *Repository) executor(ctx context.Context) (store.SQLX, error) {
	exec := store.SQLXExecutor(ctx, r.db)
	if exec == nil {
		return nil, fmt.Errorf("hub control datasource is not configured")
	}
	return exec, nil
}

func (r *Repository) Page(ctx context.Context, query NodePageQuery) ([]NodeRecord, int64, error) {
	exec, err := r.executor(ctx)
	if err != nil {
		return nil, 0, err
	}
	where := `WHERE isDeleted = 0`
	args := make([]any, 0, 4)
	if keyword := strings.TrimSpace(query.Keyword); keyword != "" {
		where += ` AND (nodeCode LIKE ? OR nodeName LIKE ?)`
		like := "%" + keyword + "%"
		args = append(args, like, like)
	}
	if query.Status != nil {
		where += ` AND status = ?`
		args = append(args, *query.Status)
	}
	var total int64
	if err := sqlx.GetContext(ctx, exec, &total, r.rebind(exec, `SELECT COUNT(1) FROM sys_federated_node `+where), args...); err != nil {
		return nil, 0, fmt.Errorf("count federated nodes: %w", err)
	}
	selectArgs := append(append([]any{}, args...), query.Size, (query.Current-1)*query.Size)
	var rows []nodeRow
	if err := sqlx.SelectContext(ctx, exec, &rows, r.rebind(exec, `SELECT `+nodeColumns+` FROM sys_federated_node `+where+` ORDER BY updatedAt DESC, id DESC LIMIT ? OFFSET ?`), selectArgs...); err != nil {
		return nil, 0, fmt.Errorf("page federated nodes: %w", err)
	}
	items := make([]NodeRecord, 0, len(rows))
	for _, row := range rows {
		items = append(items, mapNode(row))
	}
	return items, total, nil
}

func (r *Repository) Find(ctx context.Context, nodeCode string) (*NodeRecord, error) {
	return r.find(ctx, nodeCode, false)
}

func (r *Repository) FindForUpdate(ctx context.Context, nodeCode string) (*NodeRecord, error) {
	return r.find(ctx, nodeCode, true)
}

func (r *Repository) find(ctx context.Context, nodeCode string, forUpdate bool) (*NodeRecord, error) {
	exec, err := r.executor(ctx)
	if err != nil {
		return nil, err
	}
	var row nodeRow
	query := `SELECT ` + nodeColumns + ` FROM sys_federated_node WHERE nodeCode = ? AND isDeleted = 0 LIMIT 1`
	if forUpdate {
		query += ` FOR UPDATE`
	}
	if err := sqlx.GetContext(ctx, exec, &row, r.rebind(exec, query), strings.TrimSpace(nodeCode)); err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, fmt.Errorf("find federated node: %w", err)
	}
	node := mapNode(row)
	return &node, nil
}

func (r *Repository) Insert(ctx context.Context, node *NodeRecord) error {
	exec, err := r.executor(ctx)
	if err != nil {
		return err
	}
	_, err = exec.ExecContext(ctx, r.rebind(exec, `INSERT INTO sys_federated_node (`+nodeColumns+`, isDeleted) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, 0)`), nodeArgs(node)...)
	if err != nil {
		return fmt.Errorf("insert federated node: %w", err)
	}
	return nil
}

func (r *Repository) UpdateMetadata(ctx context.Context, node *NodeRecord) error {
	exec, err := r.executor(ctx)
	if err != nil {
		return err
	}
	result, err := exec.ExecContext(ctx, r.rebind(exec, `UPDATE sys_federated_node SET nodeName=?, discoveryType=?, serviceName=?, managementBaseUrl=?, hubIssuer=?, capabilitiesJson=?, updatedAt=? WHERE id=? AND isDeleted=0`), node.NodeName, node.DiscoveryType, nullString(node.ServiceName), nullString(node.ManagementBaseURL), node.HubIssuer, nullString(node.CapabilitiesJSON), node.UpdatedAt, node.ID)
	return ownedUpdateResult(result, err, "update federated node metadata")
}

func (r *Repository) ReplaceManagementBearer(ctx context.Context, node *NodeRecord) error {
	exec, err := r.executor(ctx)
	if err != nil {
		return err
	}
	result, err := exec.ExecContext(ctx, r.rebind(exec, `UPDATE sys_federated_node SET managementBearerCiphertext=?, managementBearerEdek=?, managementBearerWrapKeyRef=?, updatedAt=? WHERE id=? AND isDeleted=0`), nullString(node.ManagementBearer.Ciphertext), nullString(node.ManagementBearer.EDEK), nullString(node.ManagementBearer.WrapKeyRef), node.UpdatedAt, node.ID)
	return ownedUpdateResult(result, err, "replace federated node management bearer")
}

func (r *Repository) UpdateStatus(ctx context.Context, node *NodeRecord) error {
	exec, err := r.executor(ctx)
	if err != nil {
		return err
	}
	result, err := exec.ExecContext(ctx, r.rebind(exec, `UPDATE sys_federated_node SET status=?, updatedAt=? WHERE id=? AND isDeleted=0`), node.Status, node.UpdatedAt, node.ID)
	return ownedUpdateResult(result, err, "update federated node status")
}

func (r *Repository) UpdateTargetState(ctx context.Context, node *NodeRecord) error {
	exec, err := r.executor(ctx)
	if err != nil {
		return err
	}
	result, err := exec.ExecContext(ctx, r.rebind(exec, `UPDATE sys_federated_node SET targetRevision=?, connectionStatus=?, lastConnectionError=?, lastConnectionTraceId=?, updatedAt=? WHERE id=? AND isDeleted=0`), node.TargetRevision, node.ConnectionStatus, nullString(node.LastConnectionError), nullString(node.LastConnectionTraceID), node.UpdatedAt, node.ID)
	return ownedUpdateResult(result, err, "update federated node target state")
}

func (r *Repository) UpdateHealth(ctx context.Context, node *NodeRecord) error {
	exec, err := r.executor(ctx)
	if err != nil {
		return err
	}
	result, err := exec.ExecContext(ctx, r.rebind(exec, `UPDATE sys_federated_node SET lastHealthyAt=?, updatedAt=? WHERE id=? AND isDeleted=0`), nullTime(node.LastHealthyAt), node.UpdatedAt, node.ID)
	return ownedUpdateResult(result, err, "update federated node health")
}

func (r *Repository) UpdateConnection(ctx context.Context, node *NodeRecord) error {
	exec, err := r.executor(ctx)
	if err != nil {
		return err
	}
	result, err := exec.ExecContext(ctx, r.rebind(exec, `UPDATE sys_federated_node SET oidcClientId=?, oidcClientSecretCiphertext=?, oidcClientSecretEdek=?, oidcClientSecretWrapKeyRef=?, connectionStatus=?, connectionVersion=?, connectionRequestHash=?, issuerLockedAt=?, lastConnectionError=?, lastConnectionTraceId=?, updatedAt=? WHERE id=? AND isDeleted=0`), nullString(node.OIDCClientID), nullString(node.OIDCClientSecret.Ciphertext), nullString(node.OIDCClientSecret.EDEK), nullString(node.OIDCClientSecret.WrapKeyRef), node.ConnectionStatus, nullString(node.ConnectionVersion), nullString(node.ConnectionRequestHash), nullTime(node.IssuerLockedAt), nullString(node.LastConnectionError), nullString(node.LastConnectionTraceID), node.UpdatedAt, node.ID)
	return ownedUpdateResult(result, err, "update federated node connection")
}

// ConnectionCommandRecord is metadata-only durable replay state.
type ConnectionCommandRecord struct {
	NodeCode          string    `db:"nodeCode"`
	ConnectionVersion string    `db:"connectionVersion"`
	RequestHash       string    `db:"requestHash"`
	TargetRevision    int64     `db:"targetRevision"`
	State             string    `db:"state"`
	CreatedAt         time.Time `db:"createdAt"`
	UpdatedAt         time.Time `db:"updatedAt"`
}

func (r *Repository) FindConnectionCommandForUpdate(ctx context.Context, nodeCode, version string) (*ConnectionCommandRecord, error) {
	exec, err := r.executor(ctx)
	if err != nil {
		return nil, err
	}
	var record ConnectionCommandRecord
	query := r.rebind(exec, `SELECT nodeCode, connectionVersion, requestHash, targetRevision, terminalState AS state, createdAt, updatedAt FROM sys_federated_node_connection_command WHERE nodeCode=? AND connectionVersion=? FOR UPDATE`)
	if err := sqlx.GetContext(ctx, exec, &record, query, strings.TrimSpace(nodeCode), strings.TrimSpace(version)); err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, fmt.Errorf("find federated node connection command: %w", err)
	}
	return &record, nil
}

func (r *Repository) SaveConnectionCommand(ctx context.Context, command *ConnectionCommandRecord) error {
	if command == nil {
		return fmt.Errorf("connection command is nil")
	}
	exec, err := r.executor(ctx)
	if err != nil {
		return err
	}
	query := `INSERT INTO sys_federated_node_connection_command (nodeCode, connectionVersion, requestHash, targetRevision, terminalState, createdAt, updatedAt) VALUES (?, ?, ?, ?, ?, ?, ?) ON DUPLICATE KEY UPDATE terminalState=VALUES(terminalState), updatedAt=VALUES(updatedAt)`
	if r.postgres {
		query = `INSERT INTO sys_federated_node_connection_command (nodeCode, connectionVersion, requestHash, targetRevision, terminalState, createdAt, updatedAt) VALUES (?, ?, ?, ?, ?, ?, ?) ON CONFLICT (nodeCode, connectionVersion) DO UPDATE SET terminalState=EXCLUDED.terminalState, updatedAt=EXCLUDED.updatedAt`
	}
	_, err = exec.ExecContext(ctx, r.rebind(exec, query), command.NodeCode, command.ConnectionVersion, command.RequestHash, command.TargetRevision, command.State, command.CreatedAt, command.UpdatedAt)
	if err != nil {
		return fmt.Errorf("save federated node connection command: %w", err)
	}
	return nil
}

func ownedUpdateResult(result sql.Result, err error, action string) error {
	if err != nil {
		return fmt.Errorf("%s: %w", action, err)
	}
	changed, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if changed == 0 {
		return sql.ErrNoRows
	}
	return nil
}

func nodeArgs(n *NodeRecord) []any {
	return []any{n.ID, n.NodeCode, n.NodeName, n.Status, n.DiscoveryType, nullString(n.ServiceName), nullString(n.ManagementBaseURL), n.HubIssuer, nullString(n.OIDCClientID), nullString(n.OIDCClientSecret.Ciphertext), nullString(n.OIDCClientSecret.EDEK), nullString(n.OIDCClientSecret.WrapKeyRef), nullString(n.ManagementBearer.Ciphertext), nullString(n.ManagementBearer.EDEK), nullString(n.ManagementBearer.WrapKeyRef), nullString(n.CapabilitiesJSON), n.ConnectionStatus, nullString(n.ConnectionVersion), nullString(n.ConnectionRequestHash), n.TargetRevision, nullTime(n.IssuerLockedAt), nullString(n.LastConnectionError), nullString(n.LastConnectionTraceID), nullTime(n.LastHealthyAt), n.CreatedAt, n.UpdatedAt}
}
func mapNode(r nodeRow) NodeRecord {
	var healthy *time.Time
	if r.LastHealthyAt.Valid {
		value := r.LastHealthyAt.Time
		healthy = &value
	}
	var issuerLockedAt *time.Time
	if r.IssuerLockedAt.Valid {
		value := r.IssuerLockedAt.Time
		issuerLockedAt = &value
	}
	return NodeRecord{ID: r.ID, NodeCode: r.NodeCode, NodeName: r.NodeName, Status: r.Status, DiscoveryType: r.DiscoveryType, ServiceName: r.ServiceName.String, ManagementBaseURL: r.ManagementBaseURL.String, HubIssuer: r.HubIssuer, OIDCClientID: r.OIDCClientID.String, OIDCClientSecret: EncryptedValue{Ciphertext: r.OIDCCiphertext.String, EDEK: r.OIDCEDEK.String, WrapKeyRef: r.OIDCWrapKeyRef.String}, ManagementBearer: EncryptedValue{Ciphertext: r.BearerCiphertext.String, EDEK: r.BearerEDEK.String, WrapKeyRef: r.BearerWrapKeyRef.String}, CapabilitiesJSON: r.CapabilitiesJSON.String, ConnectionStatus: r.ConnectionStatus, ConnectionVersion: r.ConnectionVersion.String, ConnectionRequestHash: r.ConnectionRequestHash.String, TargetRevision: r.TargetRevision, IssuerLockedAt: issuerLockedAt, LastConnectionError: r.LastConnectionError.String, LastConnectionTraceID: r.LastConnectionTraceID.String, LastHealthyAt: healthy, CreatedAt: r.CreatedAt, UpdatedAt: r.UpdatedAt}
}
func nullString(value string) any {
	if value == "" {
		return nil
	}
	return value
}
func nullTime(value *time.Time) any {
	if value == nil {
		return nil
	}
	return *value
}
