package facade

import (
	"bytes"
	"errors"
	"io"
	"time"

	"github.com/bytedance/sonic"
)

// strictScalarValidationJSON keeps request-level validation metadata on the
// same Sonic JSON runtime as the DG5 cache/outbox paths. This is an explicit
// allowlist: case variants and undeclared fields are rejected rather than
// becoming silently accepted request metadata.
var strictScalarValidationJSON = sonic.Config{
	CaseSensitive:         true,
	CopyString:            true,
	DisallowUnknownFields: true,
	UseNumber:             true,
	ValidateString:        true,
}.Froze()

type PageResult[T any] struct {
	Current int64 `json:"current"`
	Size    int64 `json:"size"`
	Total   int64 `json:"total"`
	Records []T   `json:"records"`
}

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

type ScalarValidation struct {
	Required  bool     `json:"required,omitempty"`
	MinLength *int     `json:"minLength,omitempty"`
	MaxLength *int     `json:"maxLength,omitempty"`
	MinValue  *float64 `json:"minValue,omitempty"`
	MaxValue  *float64 `json:"maxValue,omitempty"`
	Options   []string `json:"options,omitempty"`
	MaxItems  *int     `json:"maxItems,omitempty"`
}

func (v *ScalarValidation) UnmarshalJSON(data []byte) error {
	type scalarValidationAlias ScalarValidation
	var decoded scalarValidationAlias
	decoder := strictScalarValidationJSON.NewDecoder(bytes.NewReader(data))
	if err := decoder.Decode(&decoded); err != nil {
		return err
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		if err == nil {
			return errors.New("validation JSON contains multiple values")
		}
		return err
	}
	*v = ScalarValidation(decoded)
	return nil
}

type ConfigAddRequest struct {
	GroupID     int64  `json:"groupId" validate:"required"`
	ConfigKey   string `json:"configKey" validate:"required,max=128"`
	ConfigValue string `json:"configValue,omitempty" validate:"max=2000"`
	// AssetFileID is accepted only for IMAGE/FILE. It is an upload result, not
	// a durable authority or presentation URL; the server binds it atomically.
	AssetFileID    *int64            `json:"assetFileId,omitempty"`
	ValueType      string            `json:"valueType" validate:"required,max=20"`
	ConfigDesc     string            `json:"configDesc,omitempty" validate:"max=500"`
	IsSensitive    *int              `json:"isSensitive,omitempty"`
	IsReadonly     *int              `json:"isReadonly,omitempty"`
	IsEnabled      *int              `json:"isEnabled,omitempty"`
	EffectType     string            `json:"effectType,omitempty" validate:"max=20"`
	IsSystemConfig *int              `json:"isSystemConfig,omitempty"`
	RequiredLogin  *int              `json:"requiredLogin,omitempty"`
	ExtJSON        *ConfigExtJSON    `json:"extJson,omitempty"`
	UIWidget       string            `json:"uiWidget,omitempty" validate:"max=32"`
	Validation     *ScalarValidation `json:"validation,omitempty"`
	Exposure       string            `json:"exposure,omitempty" validate:"max=20"`
	Sensitivity    string            `json:"sensitivity,omitempty" validate:"max=20"`
	SchemaVersion  *int              `json:"schemaVersion,omitempty"`
}

type ConfigUpdateRequest struct {
	ID          int64   `json:"id" validate:"required"`
	ConfigKey   *string `json:"configKey,omitempty" validate:"omitempty,max=100"`
	ConfigValue *string `json:"configValue,omitempty" validate:"omitempty,max=2000"`
	// AssetFileID requests an atomic replacement for IMAGE/FILE. ClearAsset
	// explicitly removes the active reference; neither field accepts URLs.
	AssetFileID    *int64            `json:"assetFileId,omitempty"`
	ClearAsset     *bool             `json:"clearAsset,omitempty"`
	ConfigDesc     *string           `json:"configDesc,omitempty" validate:"omitempty,max=500"`
	IsEnabled      *int              `json:"isEnabled,omitempty"`
	EffectType     *string           `json:"effectType,omitempty" validate:"omitempty,max=20"`
	ValueType      *string           `json:"valueType,omitempty" validate:"omitempty,max=20"`
	IsSensitive    *int              `json:"isSensitive,omitempty"`
	IsReadonly     *int              `json:"isReadonly,omitempty"`
	SortOrder      *int              `json:"sortOrder,omitempty"`
	IsSystemConfig *int              `json:"isSystemConfig,omitempty"`
	RequiredLogin  *int              `json:"requiredLogin,omitempty"`
	ExtJSON        *ConfigExtJSON    `json:"extJson,omitempty"`
	UIWidget       *string           `json:"uiWidget,omitempty" validate:"omitempty,max=32"`
	Validation     *ScalarValidation `json:"validation,omitempty"`
	Exposure       *string           `json:"exposure,omitempty" validate:"omitempty,max=20"`
	Sensitivity    *string           `json:"sensitivity,omitempty" validate:"omitempty,max=20"`
	SchemaVersion  *int              `json:"schemaVersion,omitempty"`
	Version        *int64            `json:"version,omitempty"`
}

type ConfigQueryRequest struct {
	Current    int64  `json:"current,omitempty" query:"current"`
	PageNum    int64  `json:"pageNum,omitempty" query:"pageNum"`
	PageSize   int64  `json:"pageSize,omitempty" query:"pageSize"`
	SortField  string `json:"sortField,omitempty" query:"sortField"`
	SortOrder  string `json:"sortOrder,omitempty" query:"sortOrder"`
	GroupID    *int64 `json:"groupId,omitempty" query:"groupId"`
	Keyword    string `json:"keyword,omitempty" query:"keyword"`
	SearchText string `json:"searchText,omitempty" query:"searchText"`
	SearchType string `json:"searchType,omitempty" query:"searchType"`
	IsEnabled  *int   `json:"isEnabled,omitempty" query:"isEnabled"`
}

type ConfigVO struct {
	ID             int64             `json:"id"`
	GroupID        int64             `json:"groupId"`
	GroupName      string            `json:"groupName,omitempty"`
	ConfigKey      string            `json:"configKey"`
	ConfigValue    string            `json:"configValue,omitempty"`
	ValueType      string            `json:"valueType"`
	ConfigDesc     string            `json:"configDesc,omitempty"`
	IsSensitive    int               `json:"isSensitive"`
	IsReadonly     int               `json:"isReadonly"`
	IsEnabled      int               `json:"isEnabled"`
	EffectType     string            `json:"effectType,omitempty"`
	IsSystemConfig int               `json:"isSystemConfig"`
	RequiredLogin  int               `json:"requiredLogin"`
	ExtJSON        *ConfigExtJSON    `json:"extJson,omitempty"`
	UIWidget       string            `json:"uiWidget"`
	Validation     *ScalarValidation `json:"validation,omitempty"`
	Exposure       string            `json:"exposure"`
	Sensitivity    string            `json:"sensitivity"`
	SchemaVersion  int               `json:"schemaVersion"`
	Version        int64             `json:"version"`
	ValuePresent   bool              `json:"valuePresent"`
	Connected      bool              `json:"connected"`
	ConsumerStatus string            `json:"consumerStatus"`
	CreatedBy      int64             `json:"createdBy,omitempty"`
	CreateTime     *time.Time        `json:"createTime,omitempty"`
	UpdatedBy      int64             `json:"updatedBy,omitempty"`
	UpdateTime     *time.Time        `json:"updateTime,omitempty"`
	Access         ConfigAccessVO    `json:"access"`
}

type ConfigValueDTO struct {
	Key           string `json:"key"`
	Type          string `json:"type"`
	Value         any    `json:"value"`
	GroupCode     string `json:"groupCode,omitempty"`
	GroupName     string `json:"groupName,omitempty"`
	SchemaVersion int    `json:"schemaVersion"`
	Version       int64  `json:"version"`
}

type ConfigSensitiveRevealRequest struct {
	ObfuscatedClientPublicKey string `json:"obfuscatedClientPublicKey" validate:"required"`
}

type ConfigSensitiveRevealResponse struct {
	EncryptedValue string `json:"encryptedValue"`
}

type ConfigEnabledRequest struct {
	IsEnabled *int `json:"isEnabled" validate:"required"`
}

type PendingConfigVO struct {
	LogID         int64      `json:"logId"`
	ConfigID      int64      `json:"configId"`
	ConfigKey     string     `json:"configKey"`
	ConfigDesc    string     `json:"configDesc,omitempty"`
	CurrentValue  string     `json:"currentValue,omitempty"`
	PendingValue  string     `json:"pendingValue,omitempty"`
	CreatedBy     int64      `json:"createdBy,omitempty"`
	CreatedByName string     `json:"createdByName,omitempty"`
	CreateTime    *time.Time `json:"createTime,omitempty"`
}

type ConfigChangeLogVO struct {
	ID              int64      `json:"id"`
	ConfigID        int64      `json:"configId"`
	ConfigKey       string     `json:"configKey"`
	OldValue        string     `json:"oldValue,omitempty"`
	NewValue        string     `json:"newValue,omitempty"`
	EffectType      string     `json:"effectType,omitempty"`
	Status          string     `json:"status,omitempty"`
	CreatedBy       int64      `json:"createdBy,omitempty"`
	CreatedByName   string     `json:"createdByName,omitempty"`
	CreateTime      *time.Time `json:"createTime,omitempty"`
	AppliedBy       int64      `json:"appliedBy,omitempty"`
	AppliedByName   string     `json:"appliedByName,omitempty"`
	AppliedTime     *time.Time `json:"appliedTime,omitempty"`
	RollbackReason  string     `json:"rollbackReason,omitempty"`
	OperationType   string     `json:"operationType,omitempty"`
	OperatorID      int64      `json:"operatorId,omitempty"`
	OperatorName    string     `json:"operatorName,omitempty"`
	OperationTime   *time.Time `json:"operationTime,omitempty"`
	OperationReason string     `json:"operationReason,omitempty"`
	RelatedLogID    *int64     `json:"relatedLogId,omitempty"`
	ParentLogID     *int64     `json:"parentLogId,omitempty"`
}

type ConfigBatchRequest struct {
	ConfigKeys []string `json:"configKeys" validate:"required,min=1,max=50,dive,required"`
}

type ConfigClientListRequest struct {
	GroupCode string `json:"groupCode" query:"groupCode" validate:"required,max=64"`
}

type ConfigInternalReadRequest struct {
	ConsumerID         string `json:"consumerId"`
	FullyQualifiedKey  string `json:"fullyQualifiedKey"`
	ServerScope        string `json:"serverScope"`
	Purpose            string `json:"purpose"`
	AllowedSensitivity string `json:"allowedSensitivity"`
}

type ConfigInternalBatchReadRequest struct {
	ConsumerID         string   `json:"consumerId"`
	FullyQualifiedKeys []string `json:"fullyQualifiedKeys"`
	ServerScope        string   `json:"serverScope"`
	Purpose            string   `json:"purpose"`
	AllowedSensitivity string   `json:"allowedSensitivity"`
}

type ConfigInternalListRequest struct {
	ConsumerID         string `json:"consumerId"`
	ServerScope        string `json:"serverScope"`
	Purpose            string `json:"purpose"`
	AllowedSensitivity string `json:"allowedSensitivity"`
}

type ConfigConsumerRegistration struct {
	ConsumerID         string `json:"consumerId"`
	FullyQualifiedKey  string `json:"fullyQualifiedKey"`
	ServerScope        string `json:"serverScope"`
	Purpose            string `json:"purpose"`
	AllowedSensitivity string `json:"allowedSensitivity"`
	Source             string `json:"source"`
	ActualConsumer     string `json:"actualConsumer"`
	Activation         string `json:"activation"`
	CacheRule          string `json:"cacheRule"`
}

type ConfigConsumerVO struct {
	ConsumerID         string `json:"consumerId"`
	FullyQualifiedKey  string `json:"fullyQualifiedKey"`
	ServerScope        string `json:"serverScope"`
	Purpose            string `json:"purpose"`
	AllowedSensitivity string `json:"allowedSensitivity"`
	Source             string `json:"source"`
	ActualConsumer     string `json:"actualConsumer"`
	Activation         string `json:"activation"`
	CacheRule          string `json:"cacheRule"`
	Connected          bool   `json:"connected"`
}

type ConfigGroupAddRequest struct {
	GroupCode      string `json:"groupCode" validate:"required,max=64"`
	GroupName      string `json:"groupName" validate:"required,max=100"`
	Module         string `json:"module,omitempty" validate:"max=50"`
	PermissionCode string `json:"permissionCode,omitempty" validate:"max=1024"`
	SortOrder      *int   `json:"sortOrder,omitempty"`
	Status         *int   `json:"status,omitempty"`
}

type ConfigGroupUpdateRequest struct {
	ID             int64   `json:"id" validate:"required"`
	GroupName      *string `json:"groupName,omitempty" validate:"omitempty,max=100"`
	GroupCode      *string `json:"groupCode,omitempty" validate:"omitempty,max=50"`
	Module         *string `json:"module,omitempty" validate:"omitempty,max=50"`
	PermissionCode *string `json:"permissionCode,omitempty" validate:"omitempty,max=1024"`
	SortOrder      *int    `json:"sortOrder,omitempty"`
	Status         *int    `json:"status,omitempty"`
}

type ConfigGroupQueryRequest struct {
	Current   int64  `json:"current,omitempty" query:"current"`
	PageNum   int64  `json:"pageNum,omitempty" query:"pageNum"`
	PageSize  int64  `json:"pageSize,omitempty" query:"pageSize"`
	SortField string `json:"sortField,omitempty" query:"sortField"`
	SortOrder string `json:"sortOrder,omitempty" query:"sortOrder"`
	GroupCode string `json:"groupCode,omitempty" query:"groupCode"`
	GroupName string `json:"groupName,omitempty" query:"groupName"`
	Module    string `json:"module,omitempty" query:"module"`
	Status    *int   `json:"status,omitempty" query:"status"`
}

type ConfigGroupVO struct {
	ID             int64          `json:"id"`
	GroupCode      string         `json:"groupCode"`
	GroupName      string         `json:"groupName"`
	Module         string         `json:"module,omitempty"`
	PermissionCode string         `json:"permissionCode,omitempty"`
	SortOrder      int            `json:"sortOrder"`
	Status         int            `json:"status"`
	ConfigCount    int64          `json:"configCount"`
	CreateTime     *time.Time     `json:"createTime,omitempty"`
	UpdateTime     *time.Time     `json:"updateTime,omitempty"`
	Access         ConfigAccessVO `json:"access"`
}

type ConfigAccessVO struct {
	CanRead      bool   `json:"canRead"`
	CanWrite     bool   `json:"canWrite"`
	CanDelete    bool   `json:"canDelete"`
	AccessSource string `json:"accessSource,omitempty"`
}

type ConfigScopeGrantVO struct {
	GroupCode string `json:"groupCode"`
	ConfigKey string `json:"configKey,omitempty"`
	CanRead   int    `json:"canRead"`
	CanWrite  int    `json:"canWrite"`
	CanDelete int    `json:"canDelete"`
}

type AssignRoleConfigScopesRequest struct {
	Grants []ConfigScopeGrantVO `json:"grants"`
}

type MoveRequest struct {
	BeforeID *int64 `json:"beforeId,omitempty"`
	AfterID  *int64 `json:"afterId,omitempty"`
}

type AuditLogQueryRequest struct {
	ConfigID      *int64     `json:"configId,omitempty" query:"configId"`
	OperationType string     `json:"operationType,omitempty" query:"operationType"`
	Status        string     `json:"status,omitempty" query:"status"`
	StartTime     *time.Time `json:"startTime,omitempty" query:"startTime"`
	EndTime       *time.Time `json:"endTime,omitempty" query:"endTime"`
	Limit         int        `json:"limit,omitempty" query:"limit"`
}
