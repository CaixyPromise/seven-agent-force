package infrastructure

import (
	"context"
	"database/sql"
	"encoding/json"
	"strings"
	"time"

	"github.com/CaixyPromise/seven-agent/force/internal/app/notification/domain"
	dbstore "github.com/CaixyPromise/seven-agent/force/internal/infrastructure/datasource/store"
)

func (r *Repository) InsertGroupRouteCandidate(ctx context.Context, item domain.GroupRouteCandidate) error {
	exec := dbstore.SQLXExecutor(ctx, r.db)
	_, err := exec.ExecContext(ctx, exec.Rebind(`INSERT INTO sys_notification_group_route_candidate (id,token_hash,organization_id,connection_id,connection_version,connection_config_digest,display_name,description,target_ciphertext,target_edek,target_wrap_key_ref,target_digest,expires_at,consumed_at,created_at) VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`), item.ID, item.TokenHash, item.OrganizationID, item.ConnectionID, item.ConnectionVersion, item.ConnectionConfigDigest, item.DisplayName, item.Description, item.TargetCiphertext, item.TargetEDEK, item.TargetWrapKeyRef, item.TargetDigest, item.ExpiresAt, item.ConsumedAt, item.CreatedAt)
	return err
}

func (r *Repository) ConsumeGroupRouteCandidate(ctx context.Context, organizationID int64, tokenHash string, now time.Time) (*domain.GroupRouteCandidate, error) {
	exec := dbstore.SQLXExecutor(ctx, r.db)
	var item domain.GroupRouteCandidate
	err := exec.QueryRowxContext(ctx, exec.Rebind(`SELECT id,token_hash,organization_id,connection_id,connection_version,connection_config_digest,display_name,description,target_ciphertext,target_edek,target_wrap_key_ref,target_digest,expires_at,consumed_at,created_at FROM sys_notification_group_route_candidate WHERE organization_id=? AND token_hash=? FOR UPDATE`), organizationID, tokenHash).Scan(&item.ID, &item.TokenHash, &item.OrganizationID, &item.ConnectionID, &item.ConnectionVersion, &item.ConnectionConfigDigest, &item.DisplayName, &item.Description, &item.TargetCiphertext, &item.TargetEDEK, &item.TargetWrapKeyRef, &item.TargetDigest, &item.ExpiresAt, &item.ConsumedAt, &item.CreatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if item.ConsumedAt != nil || !item.ExpiresAt.After(now) {
		return nil, nil
	}
	if _, err = exec.ExecContext(ctx, exec.Rebind(`UPDATE sys_notification_group_route_candidate SET consumed_at=? WHERE id=? AND consumed_at IS NULL`), now, item.ID); err != nil {
		return nil, err
	}
	item.ConsumedAt = &now
	return &item, nil
}

func (r *Repository) ListEnabledGroupRoutes(ctx context.Context, organizationID, _ int64, _ string) ([]domain.GroupRoute, error) {
	exec := dbstore.SQLXExecutor(ctx, r.db)
	rows, err := exec.QueryxContext(ctx, exec.Rebind(`SELECT id,organization_id,display_name,provider_type,transport_type,delivery_channel_id,required_permission_code,target_ciphertext,target_edek,target_wrap_key_ref,target_digest,status,route_version,channel_version,COALESCE(connection_version,0),config_digest,capabilities_json,last_probe_status,last_probe_reason_code,last_probe_at,creator_id,updater_id,created_at,updated_at FROM sys_notification_group_route WHERE organization_id=? AND status='ENABLED' ORDER BY display_name,id`), organizationID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := make([]domain.GroupRoute, 0)
	for rows.Next() {
		item, scanErr := scanGroupRoute(rows)
		if scanErr != nil {
			return nil, scanErr
		}
		result = append(result, item)
	}
	return result, rows.Err()
}

func (r *Repository) ListGroupRoutes(ctx context.Context, organizationID int64) ([]domain.GroupRoute, error) {
	exec := dbstore.SQLXExecutor(ctx, r.db)
	rows, err := exec.QueryxContext(ctx, exec.Rebind(`SELECT id,organization_id,display_name,provider_type,transport_type,delivery_channel_id,required_permission_code,target_ciphertext,target_edek,target_wrap_key_ref,target_digest,status,route_version,channel_version,COALESCE(connection_version,0),config_digest,capabilities_json,last_probe_status,last_probe_reason_code,last_probe_at,creator_id,updater_id,created_at,updated_at FROM sys_notification_group_route WHERE organization_id=? ORDER BY updated_at DESC,id DESC`), organizationID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := []domain.GroupRoute{}
	for rows.Next() {
		item, scanErr := scanGroupRoute(rows)
		if scanErr != nil {
			return nil, scanErr
		}
		result = append(result, item)
	}
	return result, rows.Err()
}

func (r *Repository) FindGroupRouteByID(ctx context.Context, organizationID, routeID int64) (*domain.GroupRoute, error) {
	exec := dbstore.SQLXExecutor(ctx, r.db)
	row := exec.QueryRowxContext(ctx, exec.Rebind(`SELECT id,organization_id,display_name,provider_type,transport_type,delivery_channel_id,required_permission_code,target_ciphertext,target_edek,target_wrap_key_ref,target_digest,status,route_version,channel_version,COALESCE(connection_version,0),config_digest,capabilities_json,last_probe_status,last_probe_reason_code,last_probe_at,creator_id,updater_id,created_at,updated_at FROM sys_notification_group_route WHERE organization_id=? AND id=?`), organizationID, routeID)
	item, err := scanGroupRoute(row)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	return &item, err
}

func (r *Repository) InsertGroupRoute(ctx context.Context, item domain.GroupRoute) error {
	caps, _ := json.Marshal(item.Capabilities)
	exec := dbstore.SQLXExecutor(ctx, r.db)
	_, err := exec.ExecContext(ctx, exec.Rebind(`INSERT INTO sys_notification_group_route (id,organization_id,display_name,provider_type,transport_type,delivery_channel_id,required_permission_code,target_ciphertext,target_edek,target_wrap_key_ref,target_digest,status,route_version,channel_version,connection_version,config_digest,capabilities_json,last_probe_status,last_probe_reason_code,last_probe_at,creator_id,updater_id,created_at,updated_at) VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`), item.ID, item.OrganizationID, item.DisplayName, item.Provider, item.Transport, item.DeliveryChannelID, item.RequiredPermissionCode, item.TargetCiphertext, item.TargetEDEK, item.TargetWrapKeyRef, item.TargetDigest, item.Status, item.RouteVersion, item.ChannelVersion, nullablePositiveInt64(item.ConnectionVersion), item.ConfigDigest, string(caps), item.LastProbeStatus, item.LastProbeReasonCode, item.LastProbeAt, item.CreatorID, item.UpdaterID, item.CreatedAt, item.UpdatedAt)
	return err
}

func (r *Repository) UpdateGroupRoute(ctx context.Context, item domain.GroupRoute, expectedVersion int64) (bool, error) {
	caps, _ := json.Marshal(item.Capabilities)
	exec := dbstore.SQLXExecutor(ctx, r.db)
	result, err := exec.ExecContext(ctx, exec.Rebind(`UPDATE sys_notification_group_route SET display_name=?,required_permission_code=?,target_ciphertext=?,target_edek=?,target_wrap_key_ref=?,target_digest=?,status=?,route_version=?,channel_version=?,connection_version=?,config_digest=?,capabilities_json=?,last_probe_status=?,last_probe_reason_code=?,last_probe_at=?,updater_id=?,updated_at=? WHERE organization_id=? AND id=? AND route_version=?`), item.DisplayName, item.RequiredPermissionCode, item.TargetCiphertext, item.TargetEDEK, item.TargetWrapKeyRef, item.TargetDigest, item.Status, item.RouteVersion, item.ChannelVersion, nullablePositiveInt64(item.ConnectionVersion), item.ConfigDigest, string(caps), item.LastProbeStatus, item.LastProbeReasonCode, item.LastProbeAt, item.UpdaterID, item.UpdatedAt, item.OrganizationID, item.ID, expectedVersion)
	if err != nil {
		return false, err
	}
	count, err := result.RowsAffected()
	return count == 1, err
}

func (r *Repository) SetChannelOrganizationOwnership(ctx context.Context, channelID, organizationID int64) error {
	exec := dbstore.SQLXExecutor(ctx, r.db)
	_, err := exec.ExecContext(ctx, exec.Rebind(r.inboxSQL(`UPDATE sys_notification_channel SET scopeType='ORGANIZATION',organizationId=? WHERE id=? AND isDeleted=0`)), organizationID, channelID)
	return err
}

func (r *Repository) GroupRouteImpact(ctx context.Context, organizationID, routeID int64) (activeTasks, resumableTasks, subscriptions, pendingRecords, pendingDeliveries, historical int64, err error) {
	queries := []struct {
		dst   *int64
		query string
	}{
		{&activeTasks, `SELECT COUNT(DISTINCT s.task_id) FROM workforce_task_notification_subscription_channels sc JOIN workforce_task_notification_subscriptions s ON s.id=sc.subscription_id JOIN workforce_tasks t ON t.id=s.task_id AND t.tenant_id=s.tenant_id WHERE sc.tenant_id=? AND sc.group_route_id=? AND t.status IN ('QUEUED','PREPARING','RUNNING','VALIDATING','PACKAGING','UPLOADING','RETRY_WAIT','CANCELLING')`},
		{&resumableTasks, `SELECT COUNT(DISTINCT s.task_id) FROM workforce_task_notification_subscription_channels sc JOIN workforce_task_notification_subscriptions s ON s.id=sc.subscription_id JOIN workforce_tasks t ON t.id=s.task_id AND t.tenant_id=s.tenant_id WHERE sc.tenant_id=? AND sc.group_route_id=? AND t.status IN ('REVIEWING','MANUAL_REVIEW')`},
		{&subscriptions, `SELECT COUNT(DISTINCT subscription_id) FROM workforce_task_notification_subscription_channels WHERE tenant_id=? AND group_route_id=?`},
		{&pendingRecords, `SELECT COUNT(DISTINCT rc.record_id) FROM workforce_task_notification_record_channels rc JOIN workforce_task_notification_records r ON r.id=rc.record_id WHERE rc.tenant_id=? AND rc.group_route_id=? AND r.status IN ('PENDING','PROCESSING','RETRY_WAIT')`},
		{&pendingDeliveries, `SELECT COUNT(*) FROM sys_notification_group_route_snapshot s JOIN sys_notification_delivery d ON d.deliveryId=s.delivery_id WHERE s.organization_id=? AND s.route_id=? AND d.status IN ('PENDING','SENDING','RETRY_WAIT','UNKNOWN')`},
		{&historical, `SELECT COUNT(*) FROM sys_notification_group_route_snapshot WHERE organization_id=? AND route_id=?`},
	}
	exec := dbstore.SQLXExecutor(ctx, r.db)
	for _, q := range queries {
		if scanErr := exec.QueryRowxContext(ctx, exec.Rebind(r.inboxSQL(q.query)), organizationID, routeID).Scan(q.dst); scanErr != nil {
			return 0, 0, 0, 0, 0, 0, scanErr
		}
	}
	return
}

func (r *Repository) DeleteGroupRoute(ctx context.Context, organizationID, routeID int64) (bool, error) {
	exec := dbstore.SQLXExecutor(ctx, r.db)
	var channelID int64
	if err := exec.QueryRowxContext(ctx, exec.Rebind(`SELECT delivery_channel_id FROM sys_notification_group_route WHERE organization_id=? AND id=? AND status='DRAFT' FOR UPDATE`), organizationID, routeID).Scan(&channelID); err != nil {
		if err == sql.ErrNoRows {
			return false, nil
		}
		return false, err
	}
	result, err := exec.ExecContext(ctx, exec.Rebind(`DELETE FROM sys_notification_group_route WHERE organization_id=? AND id=? AND status='DRAFT'`), organizationID, routeID)
	if err != nil {
		return false, err
	}
	count, err := result.RowsAffected()
	if err != nil || count != 1 {
		return false, err
	}
	_, err = exec.ExecContext(ctx, exec.Rebind(r.inboxSQL(`UPDATE sys_notification_channel SET isDeleted=TRUE,status=1,updateTime=CURRENT_TIMESTAMP WHERE id=? AND organizationId=? AND NOT EXISTS (SELECT 1 FROM sys_notification_group_route WHERE delivery_channel_id=?)`)), channelID, organizationID, channelID)
	return err == nil, err
}

func (r *Repository) FindGroupRoutesByIDs(ctx context.Context, organizationID int64, ids []int64) ([]domain.GroupRoute, error) {
	if len(ids) == 0 {
		return []domain.GroupRoute{}, nil
	}
	query, args, err := sqlxIn(`SELECT id,organization_id,display_name,provider_type,transport_type,delivery_channel_id,required_permission_code,target_ciphertext,target_edek,target_wrap_key_ref,target_digest,status,route_version,channel_version,COALESCE(connection_version,0),config_digest,capabilities_json,last_probe_status,last_probe_reason_code,last_probe_at,creator_id,updater_id,created_at,updated_at FROM sys_notification_group_route WHERE organization_id=? AND id IN (?)`, organizationID, ids)
	if err != nil {
		return nil, err
	}
	exec := dbstore.SQLXExecutor(ctx, r.db)
	rows, err := exec.QueryxContext(ctx, exec.Rebind(query), args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := make([]domain.GroupRoute, 0, len(ids))
	for rows.Next() {
		item, scanErr := scanGroupRoute(rows)
		if scanErr != nil {
			return nil, scanErr
		}
		result = append(result, item)
	}
	return result, rows.Err()
}

func (r *Repository) FindChannelByID(ctx context.Context, id int64) (*domain.Channel, error) {
	var code string
	exec := dbstore.SQLXExecutor(ctx, r.db)
	if err := exec.QueryRowxContext(ctx, exec.Rebind(r.inboxSQL(`SELECT channelCode FROM sys_notification_channel WHERE id=? AND isDeleted=0`)), id).Scan(&code); err == sql.ErrNoRows {
		return nil, nil
	} else if err != nil {
		return nil, err
	}
	return r.FindChannelByCode(ctx, code)
}

func (r *Repository) ResolvePublicationStatus(ctx context.Context, organizationID int64, receiptIdentity string, viewerUserID int64) ([]domain.PublicationChannelStatus, error) {
	_ = viewerUserID // Callers must enforce viewer authority before crossing the facade.
	exec := dbstore.SQLXExecutor(ctx, r.db)
	rows, err := exec.QueryxContext(ctx, exec.Rebind(r.inboxSQL(`SELECT d.channelType,d.channelName,
		CASE WHEN d.status IN ('SENT','PROVIDER_ACCEPTED') THEN 'DELIVERED' WHEN d.status IN ('PENDING','SENDING') THEN 'QUEUED' WHEN d.status='UNKNOWN' THEN 'OUTCOME_UNKNOWN' WHEN d.status='CANCELED' THEN 'SUPPRESSED' ELSE 'FAILED' END,
		CASE WHEN COALESCE(grs.mention_requested,FALSE)=FALSE THEN 'NOT_REQUESTED' WHEN COALESCE(grs.warning_code,'')='MENTION_IDENTITY_NOT_AVAILABLE' THEN 'NOT_AVAILABLE' ELSE 'MENTIONED' END,
		COALESCE(grs.warning_code,''),d.createTime,d.updateTime
		FROM sys_notification n JOIN sys_notification_delivery d ON d.notificationId=n.notificationId
		LEFT JOIN sys_notification_group_route_snapshot grs ON grs.delivery_id=d.deliveryId
		WHERE n.notificationId=? AND (grs.organization_id=? OR grs.organization_id IS NULL)
		ORDER BY d.createTime,d.deliveryId`)), strings.TrimSpace(receiptIdentity), organizationID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := []domain.PublicationChannelStatus{}
	for rows.Next() {
		var x domain.PublicationChannelStatus
		if err = rows.Scan(&x.ChannelKind, &x.DisplayName, &x.State, &x.MentionState, &x.SafeReasonCode, &x.AcceptedAt, &x.UpdatedAt); err != nil {
			return nil, err
		}
		result = append(result, x)
	}
	return result, rows.Err()
}

func (r *Repository) InsertGroupRouteSnapshots(ctx context.Context, items []domain.GroupRouteSnapshot) error {
	if len(items) == 0 {
		return nil
	}
	query := `INSERT INTO sys_notification_group_route_snapshot (id,notification_id,delivery_id,organization_id,route_id,route_version,provider_type,transport_type,route_config_digest,delivery_channel_id,channel_version,enterprise_connection_version,target_ciphertext,target_edek,target_wrap_key_ref,target_digest,presentation,action_url_snapshot,mention_requested,mention_user_id,mention_identity_id,mention_identity_version,mention_authority_digest,warning_code,rendered_payload_digest,created_at) VALUES `
	marks := make([]string, 0, len(items))
	args := make([]any, 0, len(items)*26)
	for _, item := range items {
		marks = append(marks, "("+strings.TrimSuffix(strings.Repeat("?,", 26), ",")+")")
		args = append(args, item.ID, item.NotificationIdentity, item.DeliveryIdentity, item.OrganizationID, item.RouteID, item.RouteVersion, item.Provider, item.Transport, item.RouteConfigDigest, item.DeliveryChannelID, item.ChannelVersion, nullablePositiveInt64(item.EnterpriseConnectionVersion), item.TargetCiphertext, item.TargetEDEK, item.TargetWrapKeyRef, item.TargetDigest, item.Presentation, item.ActionURLSnapshot, item.MentionRequested, nullablePositiveInt64(item.MentionUserID), nullablePositiveInt64(item.MentionIdentityID), nullablePositiveInt64(item.MentionIdentityVersion), item.MentionAuthorityDigest, item.WarningCode, item.RenderedPayloadDigest, item.CreatedAt)
	}
	exec := dbstore.SQLXExecutor(ctx, r.db)
	_, err := exec.ExecContext(ctx, exec.Rebind(query+strings.Join(marks, ",")), args...)
	return err
}

func (r *Repository) FindGroupRouteSnapshotByDeliveryID(ctx context.Context, deliveryID string) (*domain.GroupRouteSnapshot, error) {
	var item domain.GroupRouteSnapshot
	exec := dbstore.SQLXExecutor(ctx, r.db)
	err := exec.QueryRowxContext(ctx, exec.Rebind(`SELECT id,notification_id,delivery_id,organization_id,route_id,route_version,provider_type,transport_type,route_config_digest,delivery_channel_id,channel_version,COALESCE(enterprise_connection_version,0),target_ciphertext,target_edek,target_wrap_key_ref,target_digest,presentation,action_url_snapshot,mention_requested,COALESCE(mention_user_id,0),COALESCE(mention_identity_id,0),COALESCE(mention_identity_version,0),mention_authority_digest,warning_code,rendered_payload_digest,created_at FROM sys_notification_group_route_snapshot WHERE delivery_id=?`), strings.TrimSpace(deliveryID)).Scan(&item.ID, &item.NotificationIdentity, &item.DeliveryIdentity, &item.OrganizationID, &item.RouteID, &item.RouteVersion, &item.Provider, &item.Transport, &item.RouteConfigDigest, &item.DeliveryChannelID, &item.ChannelVersion, &item.EnterpriseConnectionVersion, &item.TargetCiphertext, &item.TargetEDEK, &item.TargetWrapKeyRef, &item.TargetDigest, &item.Presentation, &item.ActionURLSnapshot, &item.MentionRequested, &item.MentionUserID, &item.MentionIdentityID, &item.MentionIdentityVersion, &item.MentionAuthorityDigest, &item.WarningCode, &item.RenderedPayloadDigest, &item.CreatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	return &item, err
}

func (r *Repository) UpdateGroupRouteSnapshotWarning(ctx context.Context, deliveryID, warningCode string) error {
	exec := dbstore.SQLXExecutor(ctx, r.db)
	_, err := exec.ExecContext(ctx, exec.Rebind(`UPDATE sys_notification_group_route_snapshot SET warning_code=? WHERE delivery_id=?`), strings.TrimSpace(warningCode), strings.TrimSpace(deliveryID))
	return err
}

func nullablePositiveInt64(value int64) any {
	if value <= 0 {
		return nil
	}
	return value
}

type groupRouteScanner interface{ Scan(...any) error }

func scanGroupRoute(row groupRouteScanner) (domain.GroupRoute, error) {
	var x domain.GroupRoute
	var caps []byte
	var conn sql.NullInt64
	err := row.Scan(&x.ID, &x.OrganizationID, &x.DisplayName, &x.Provider, &x.Transport, &x.DeliveryChannelID, &x.RequiredPermissionCode, &x.TargetCiphertext, &x.TargetEDEK, &x.TargetWrapKeyRef, &x.TargetDigest, &x.Status, &x.RouteVersion, &x.ChannelVersion, &conn, &x.ConfigDigest, &caps, &x.LastProbeStatus, &x.LastProbeReasonCode, &x.LastProbeAt, &x.CreatorID, &x.UpdaterID, &x.CreatedAt, &x.UpdatedAt)
	x.ConnectionVersion = conn.Int64
	if err == nil {
		err = json.Unmarshal(caps, &x.Capabilities)
	}
	return x, err
}

func sqlxIn(query string, organizationID int64, ids []int64) (string, []any, error) {
	args := make([]any, 0, len(ids)+1)
	args = append(args, organizationID)
	marks := make([]string, len(ids))
	for i, id := range ids {
		marks[i] = "?"
		args = append(args, id)
	}
	return strings.Replace(query, "(?)", "("+strings.Join(marks, ",")+")", 1), args, nil
}
