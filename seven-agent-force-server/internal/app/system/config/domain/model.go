package domain

import (
	"strings"
	"time"
)

type ConfigSecretValue struct {
	Plain         string `json:"plain,omitempty"`
	CiphertextB64 string `json:"ciphertextB64,omitempty"`
	EDEKB64       string `json:"edekB64,omitempty"`
	WrapKeyRef    string `json:"wrapKeyRef,omitempty"`
}

type ConfigExtJSON struct {
	Enums  []string           `json:"enums,omitempty"`
	Secret *ConfigSecretValue `json:"secret,omitempty"`
}

type ConfigGroup struct {
	ID             int64      `json:"id"`
	GroupCode      string     `json:"groupCode"`
	GroupName      string     `json:"groupName"`
	Module         string     `json:"module,omitempty"`
	PermissionCode string     `json:"permissionCode,omitempty"`
	SortOrder      int        `json:"sortOrder"`
	Status         int        `json:"status"`
	CreateTime     *time.Time `json:"createTime,omitempty"`
	UpdateTime     *time.Time `json:"updateTime,omitempty"`
	IsDeleted      int        `json:"isDeleted"`
	ConfigCount    int64      `json:"configCount"`
}

type Config struct {
	ID             int64             `json:"id"`
	GroupID        int64             `json:"groupId"`
	ConfigKey      string            `json:"configKey"`
	ConfigValue    string            `json:"configValue,omitempty"`
	ValueType      string            `json:"valueType"`
	ConfigDesc     string            `json:"configDesc,omitempty"`
	IsSensitive    int               `json:"isSensitive"`
	IsSystemConfig int               `json:"isSystemConfig"`
	RequiredLogin  int               `json:"requiredLogin"`
	UIWidget       string            `json:"uiWidget"`
	Validation     *ScalarValidation `json:"validation,omitempty"`
	Exposure       string            `json:"exposure"`
	Sensitivity    string            `json:"sensitivity"`
	SchemaVersion  int               `json:"schemaVersion"`
	Version        int64             `json:"version"`
	ExtJSON        *ConfigExtJSON    `json:"extJson,omitempty"`
	IsReadonly     int               `json:"isReadonly"`
	IsEnabled      int               `json:"isEnabled"`
	EffectType     string            `json:"effectType,omitempty"`
	CreatedBy      int64             `json:"createdBy,omitempty"`
	CreateTime     *time.Time        `json:"createTime,omitempty"`
	UpdatedBy      int64             `json:"updatedBy,omitempty"`
	UpdateTime     *time.Time        `json:"updateTime,omitempty"`
	IsDeleted      int               `json:"isDeleted"`
	GroupCode      string            `json:"groupCode,omitempty"`
	GroupName      string            `json:"groupName,omitempty"`
}

func (c Config) FullyQualifiedKey(group *ConfigGroup) string {
	groupCode := strings.TrimSpace(c.GroupCode)
	if groupCode == "" && group != nil {
		groupCode = strings.TrimSpace(group.GroupCode)
	}
	if groupCode == "" || strings.TrimSpace(c.ConfigKey) == "" {
		return ""
	}
	return groupCode + "." + strings.TrimSpace(c.ConfigKey)
}

type ConfigChangeLog struct {
	ID                int64      `json:"id"`
	ConfigID          int64      `json:"configId"`
	ConfigKey         string     `json:"configKey"`
	OperationType     string     `json:"operationType"`
	OldValue          string     `json:"oldValue,omitempty"`
	NewValue          string     `json:"newValue,omitempty"`
	OldValueProtected bool       `json:"oldValueProtected"`
	NewValueProtected bool       `json:"newValueProtected"`
	EffectType        string     `json:"effectType,omitempty"`
	Status            string     `json:"status,omitempty"`
	ParentLogID       *int64     `json:"parentLogId,omitempty"`
	RelatedLogID      *int64     `json:"relatedLogId,omitempty"`
	OperatorID        int64      `json:"operatorId"`
	OperatorName      string     `json:"operatorName,omitempty"`
	OperationTime     *time.Time `json:"operationTime,omitempty"`
	OperationReason   string     `json:"operationReason,omitempty"`
	AppliedBy         *int64     `json:"appliedBy,omitempty"`
	AppliedTime       *time.Time `json:"appliedTime,omitempty"`

	// The serialized asset binding snapshots stay unexported by design. They
	// are persisted by the infrastructure adapter and are accessible only to
	// the rollback use case through PrivateAssetSnapshots.
	oldAssetSnapshotPayload string
	newAssetSnapshotPayload string
}

type ConfigScopeGrant struct {
	ID         int64      `json:"id"`
	RoleID     int64      `json:"roleId"`
	GroupCode  string     `json:"groupCode"`
	ConfigKey  string     `json:"configKey,omitempty"`
	CanRead    int        `json:"canRead"`
	CanWrite   int        `json:"canWrite"`
	CanDelete  int        `json:"canDelete"`
	CreatedBy  int64      `json:"createdBy,omitempty"`
	CreateTime *time.Time `json:"createTime,omitempty"`
	UpdatedBy  int64      `json:"updatedBy,omitempty"`
	UpdateTime *time.Time `json:"updateTime,omitempty"`
	IsDeleted  int        `json:"isDeleted"`
}

type ConfigAccess struct {
	CanRead      bool   `json:"canRead"`
	CanWrite     bool   `json:"canWrite"`
	CanDelete    bool   `json:"canDelete"`
	AccessSource string `json:"accessSource,omitempty"`
}

type ConfigGroupPageQuery struct {
	Current   int64
	PageSize  int64
	GroupCode string
	GroupName string
	Module    string
	Status    *int
}

type ConfigGroupPage struct {
	Current int64
	Size    int64
	Total   int64
	Records []ConfigGroup
}

type ConfigPageQuery struct {
	Current    int64
	PageSize   int64
	GroupID    *int64
	Keyword    string
	SearchText string
	SearchType string
	IsEnabled  *int
}

type ConfigPage struct {
	Current int64
	Size    int64
	Total   int64
	Records []Config
}

type AuditLogQuery struct {
	ConfigID      *int64
	OperationType string
	Status        string
	StartTime     *time.Time
	EndTime       *time.Time
	Limit         int
}
