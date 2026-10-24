package infrastructure

import (
	"context"
	"database/sql"

	"github.com/CaixyPromise/seven-agent/force/internal/app/notification/facade"
)

func (r *Repository) ListWorkPreferences(ctx context.Context, organizationID, userID int64) ([]facade.WorkPreference, error) {
	rows, err := r.db.QueryxContext(ctx, r.db.Rebind(`SELECT id,organization_id,user_id,event_family,channel_kind,enabled,version FROM sys_notification_user_preferences WHERE organization_id=? AND user_id=? ORDER BY event_family,channel_kind`), organizationID, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := make([]facade.WorkPreference, 0)
	for rows.Next() {
		var item facade.WorkPreference
		if err = rows.Scan(&item.ID, &item.OrganizationID, &item.UserID, &item.EventFamily, &item.ChannelKind, &item.Enabled, &item.Version); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (r *Repository) FindWorkPreference(ctx context.Context, organizationID, userID int64, eventFamily, channelKind string) (*facade.WorkPreference, error) {
	var item facade.WorkPreference
	err := r.db.QueryRowxContext(ctx, r.db.Rebind(`SELECT id,organization_id,user_id,event_family,channel_kind,enabled,version FROM sys_notification_user_preferences WHERE organization_id=? AND user_id=? AND event_family=? AND channel_kind=?`), organizationID, userID, eventFamily, channelKind).Scan(&item.ID, &item.OrganizationID, &item.UserID, &item.EventFamily, &item.ChannelKind, &item.Enabled, &item.Version)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &item, nil
}

func (r *Repository) InsertWorkPreference(ctx context.Context, item facade.WorkPreference) error {
	_, err := r.db.ExecContext(ctx, r.db.Rebind(`INSERT INTO sys_notification_user_preferences (id,organization_id,user_id,event_family,channel_kind,enabled,version,creator_id,updater_id) VALUES (?,?,?,?,?,?,1,?,?)`), item.ID, item.OrganizationID, item.UserID, item.EventFamily, item.ChannelKind, item.Enabled, item.UserID, item.UserID)
	return err
}

func (r *Repository) UpdateWorkPreference(ctx context.Context, item facade.WorkPreference, expectedVersion int64) (bool, error) {
	result, err := r.db.ExecContext(ctx, r.db.Rebind(`UPDATE sys_notification_user_preferences SET enabled=?,version=version+1,updater_id=?,updated_at=CURRENT_TIMESTAMP WHERE id=? AND organization_id=? AND user_id=? AND version=?`), item.Enabled, item.UserID, item.ID, item.OrganizationID, item.UserID, expectedVersion)
	if err != nil {
		return false, err
	}
	count, err := result.RowsAffected()
	return count == 1, err
}
