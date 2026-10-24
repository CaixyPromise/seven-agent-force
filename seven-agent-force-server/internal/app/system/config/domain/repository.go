package domain

import (
	"context"
	"time"
)

type Repository interface {
	FindGroupByID(ctx context.Context, id int64) (*ConfigGroup, error)
	FindGroupByCode(ctx context.Context, groupCode string) (*ConfigGroup, error)
	ListGroupsByCodes(ctx context.Context, groupCodes []string) ([]ConfigGroup, error)
	CountGroupByCode(ctx context.Context, groupCode string, excludeID int64) (int64, error)
	InsertGroup(ctx context.Context, item *ConfigGroup) (int64, error)
	UpdateGroup(ctx context.Context, item *ConfigGroup) error
	QueryGroups(ctx context.Context, query ConfigGroupPageQuery) (*ConfigGroupPage, error)
	CountConfigsByGroupID(ctx context.Context, groupID int64) (int64, error)
	ShiftGroupSort(ctx context.Context, targetID int64, oldOrder, newOrder int) error

	FindConfigByID(ctx context.Context, id int64) (*Config, error)
	FindConfigByGroupAndKey(ctx context.Context, groupID int64, configKey string, includeDisabled bool) (*Config, error)
	FindConfigsByRawKey(ctx context.Context, configKey string, includeDisabled bool) ([]Config, error)
	CountConfigByGroupAndKey(ctx context.Context, groupID int64, configKey string, excludeID int64) (int64, error)
	InsertConfig(ctx context.Context, item *Config) (int64, error)
	UpdateConfig(ctx context.Context, item *Config) error
	QueryConfigs(ctx context.Context, query ConfigPageQuery) (*ConfigPage, error)
	ListConfigsByIDs(ctx context.Context, ids []int64) ([]Config, error)
	ListConfigsByGroupAndKeys(ctx context.Context, refs []ConfigKeyRef) ([]Config, error)

	InsertChangeLog(ctx context.Context, item *ConfigChangeLog) (int64, error)
	UpdateChangeLog(ctx context.Context, item *ConfigChangeLog) error
	ClaimPendingChangeLog(ctx context.Context, id int64, appliedBy int64, appliedTime time.Time, operatorName string) (bool, error)
	ApplyPendingConfigBatch(ctx context.Context, items []PendingConfigApply) ([]int64, error)
	FindChangeLogByID(ctx context.Context, id int64) (*ConfigChangeLog, error)
	ListPendingLogs(ctx context.Context) ([]ConfigChangeLog, error)
	ListHistoryByConfigID(ctx context.Context, configID int64, limit int) ([]ConfigChangeLog, error)
	ListAuditLogs(ctx context.Context, query AuditLogQuery) ([]ConfigChangeLog, error)
	ListChangeLogsByIDs(ctx context.Context, ids []int64) ([]ConfigChangeLog, error)
	ListChangeLogsReferencing(ctx context.Context, ids []int64) ([]ConfigChangeLog, error)

	ListConfigScopeGrantsByRoleIDs(ctx context.Context, roleIDs []int64) ([]ConfigScopeGrant, error)
	ListConfigScopeGrantsByRoleID(ctx context.Context, roleID int64) ([]ConfigScopeGrant, error)
	ReplaceRoleConfigScopes(ctx context.Context, roleID int64, grants []ConfigScopeGrant, operatorID int64, nextID func() int64) error
}

type ConfigKeyRef struct {
	GroupID   int64
	ConfigKey string
}

// PendingConfigApply contains one prepared, version-checked config mutation and
// its audit row. Repositories apply a bounded slice with a fixed query shape.
type PendingConfigApply struct {
	PendingLogID int64
	Config       Config
	ApplyLog     ConfigChangeLog
}

type SecretCipher interface {
	EncryptString(ctx context.Context, plain string) (ConfigSecretValue, error)
	DecryptString(ctx context.Context, value ConfigSecretValue) (string, error)
}

type UserLookup interface {
	FindNicknames(ctx context.Context, userIDs []int64) (map[int64]string, error)
}

type StartupApplyRecorder interface {
	RunOnce(ctx context.Context, fn func(context.Context) error) error
}

type ConfigChangeOperationType string

const (
	ConfigOperationCreate   ConfigChangeOperationType = "CREATE"
	ConfigOperationUpdate   ConfigChangeOperationType = "UPDATE"
	ConfigOperationDelete   ConfigChangeOperationType = "DELETE"
	ConfigOperationRollback ConfigChangeOperationType = "ROLLBACK"
	ConfigOperationApply    ConfigChangeOperationType = "APPLY"
)

type ConfigChangeStatus string

const (
	ConfigStatusPending    ConfigChangeStatus = "pending"
	ConfigStatusApplied    ConfigChangeStatus = "applied"
	ConfigStatusRolledBack ConfigChangeStatus = "rolled_back"
)

type ConfigEffectType string

const (
	ConfigEffectRealtime ConfigEffectType = "realtime"
	ConfigEffectRestart  ConfigEffectType = "restart"
)

type CreateChangeLogInput struct {
	ConfigID         int64
	ConfigKey        string
	OperationType    ConfigChangeOperationType
	OldValue         string
	NewValue         string
	EffectType       string
	ParentLogID      *int64
	RelatedLogID     *int64
	OperatorID       int64
	OperationReason  string
	OldAssetSnapshot *ConfigAssetBindingSnapshot
	NewAssetSnapshot *ConfigAssetBindingSnapshot
	IsStartup        bool
	Now              time.Time
}
