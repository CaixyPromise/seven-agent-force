package facade

import (
	"context"
	"strings"
	"time"

	"github.com/CaixyPromise/seven-agent/force/internal/shared/stepup"
	"github.com/cloudwego/hertz/pkg/app"
)

type PageResult[T any] struct {
	Current int64 `json:"current"`
	Size    int64 `json:"size"`
	Total   int64 `json:"total"`
	Records []T   `json:"records"`
}

type OnlineUserVO struct {
	UserID           int64  `json:"userId"`
	Username         string `json:"username,omitempty"`
	Nickname         string `json:"nickname,omitempty"`
	Avatar           string `json:"avatar,omitempty"`
	Email            string `json:"email,omitempty"`
	UserRole         string `json:"userRole,omitempty"`
	LoginTime        *int64 `json:"loginTime,omitempty"`
	LastActiveTime   *int64 `json:"lastActiveTime,omitempty"`
	ExpireTime       *int64 `json:"expireTime,omitempty"`
	LoginIP          string `json:"loginIp,omitempty"`
	LoginAddress     string `json:"loginAddress,omitempty"`
	Browser          string `json:"browser,omitempty"`
	OS               string `json:"os,omitempty"`
	DeviceID         string `json:"deviceId,omitempty"`
	UserAgent        string `json:"userAgent,omitempty"`
	TokenID          string `json:"tokenId,omitempty"`
	IsCurrentSession bool   `json:"isCurrentSession"`
}

type OnlineUserStatsVO struct {
	TotalOnlineUsers int64            `json:"totalOnlineUsers"`
	AdminUsers       int64            `json:"adminUsers"`
	NormalUsers      int64            `json:"normalUsers"`
	BrowserStats     map[string]int64 `json:"browserStats,omitempty"`
	OSStats          map[string]int64 `json:"osStats,omitempty"`
	TodayLoginUsers  int64            `json:"todayLoginUsers"`
	ActiveUsers      int64            `json:"activeUsers"`
	TotalOnline      int64            `json:"totalOnline"`
	TodayLogin       int64            `json:"todayLogin"`
	PeakOnline       int64            `json:"peakOnline"`
}

type BatchLogoutResultVO struct {
	SuccessIDs     []int64  `json:"successIds,omitempty"`
	FailedIDs      []int64  `json:"failedIds,omitempty"`
	TotalCount     int      `json:"totalCount"`
	SuccessCount   int      `json:"successCount"`
	FailedCount    int      `json:"failedCount"`
	FailureReasons []string `json:"failureReasons,omitempty"`
}

type ForceLogoutCommand struct {
	UserID      int64                `json:"userId"`
	OperatorID  int64                `json:"operatorId,omitempty"`
	StepUpProof stepup.ProofMetadata `json:"-"`
}

type BatchForceLogoutCommand struct {
	UserIDs     []int64              `json:"userIds"`
	OperatorID  int64                `json:"operatorId,omitempty"`
	StepUpProof stepup.ProofMetadata `json:"-"`
}

type OperationTypeEnum string

const (
	OperationTypeUserLogin                 OperationTypeEnum = "USER_LOGIN"
	OperationTypeUserLogout                OperationTypeEnum = "USER_LOGOUT"
	OperationTypeUserRefreshToken          OperationTypeEnum = "USER_REFRESH_TOKEN"
	OperationTypeUserCreate                OperationTypeEnum = "USER_CREATE"
	OperationTypeUserUpdate                OperationTypeEnum = "USER_UPDATE"
	OperationTypeUserDelete                OperationTypeEnum = "USER_DELETE"
	OperationTypeUserResetPassword         OperationTypeEnum = "USER_RESET_PASSWORD"
	OperationTypeUserUpdateStatus          OperationTypeEnum = "USER_UPDATE_STATUS"
	OperationTypeRoleCreate                OperationTypeEnum = "ROLE_CREATE"
	OperationTypeRoleUpdate                OperationTypeEnum = "ROLE_UPDATE"
	OperationTypeRoleDelete                OperationTypeEnum = "ROLE_DELETE"
	OperationTypeRoleAssignPermission      OperationTypeEnum = "ROLE_ASSIGN_PERMISSION"
	OperationTypeAdminForceLogout          OperationTypeEnum = "ADMIN_FORCE_LOGOUT"
	OperationTypeAdminBanUser              OperationTypeEnum = "ADMIN_BAN_USER"
	OperationTypeAdminUnbanUser            OperationTypeEnum = "ADMIN_UNBAN_USER"
	OperationTypeAdminUnlockAccount        OperationTypeEnum = "ADMIN_UNLOCK_ACCOUNT"
	OperationTypeSystemConfigUpdate        OperationTypeEnum = "SYSTEM_CONFIG_UPDATE"
	OperationTypeSystemCacheClear          OperationTypeEnum = "SYSTEM_CACHE_CLEAR"
	OperationTypeSystemLogClear            OperationTypeEnum = "SYSTEM_LOG_CLEAR"
	OperationTypeRuntimeLogQuery           OperationTypeEnum = "RUNTIME_LOG_QUERY"
	OperationTypeRuntimeLogStreamSubscribe OperationTypeEnum = "RUNTIME_LOG_STREAM_SUBSCRIBE"
	OperationTypeFileUpload                OperationTypeEnum = "FILE_UPLOAD"
	OperationTypeFileDelete                OperationTypeEnum = "FILE_DELETE"
	OperationTypeDataExport                OperationTypeEnum = "DATA_EXPORT"
	OperationTypeDataImport                OperationTypeEnum = "DATA_IMPORT"
	OperationTypeConfigGroupCreate         OperationTypeEnum = "CONFIG_GROUP_CREATE"
	OperationTypeConfigGroupUpdate         OperationTypeEnum = "CONFIG_GROUP_UPDATE"
	OperationTypeConfigGroupDelete         OperationTypeEnum = "CONFIG_GROUP_DELETE"
	OperationTypeConfigCreate              OperationTypeEnum = "CONFIG_CREATE"
	OperationTypeConfigUpdate              OperationTypeEnum = "CONFIG_UPDATE"
	OperationTypeConfigDelete              OperationTypeEnum = "CONFIG_DELETE"
	OperationTypeConfigEnabledChange       OperationTypeEnum = "CONFIG_ENABLED_CHANGE"
	OperationTypeConfigApply               OperationTypeEnum = "CONFIG_APPLY"
	OperationTypeConfigRollback            OperationTypeEnum = "CONFIG_ROLLBACK"
	OperationTypeDockerContainerStart      OperationTypeEnum = "DOCKER_CONTAINER_START"
	OperationTypeDockerContainerStop       OperationTypeEnum = "DOCKER_CONTAINER_STOP"
	OperationTypeDockerContainerRestart    OperationTypeEnum = "DOCKER_CONTAINER_RESTART"
	OperationTypeDockerContainerDelete     OperationTypeEnum = "DOCKER_CONTAINER_DELETE"
	OperationTypeDockerContainerCreate     OperationTypeEnum = "DOCKER_CONTAINER_CREATE"
	OperationTypeDockerImagePull           OperationTypeEnum = "DOCKER_IMAGE_PULL"
	OperationTypeDockerImageTag            OperationTypeEnum = "DOCKER_IMAGE_TAG"
	OperationTypeDockerImagePush           OperationTypeEnum = "DOCKER_IMAGE_PUSH"
	OperationTypeDockerImageDelete         OperationTypeEnum = "DOCKER_IMAGE_DELETE"
	OperationTypeDockerRegistryCreate      OperationTypeEnum = "DOCKER_REGISTRY_CREATE"
	OperationTypeDockerRegistryUpdate      OperationTypeEnum = "DOCKER_REGISTRY_UPDATE"
	OperationTypeDockerRegistryTest        OperationTypeEnum = "DOCKER_REGISTRY_TEST"
	OperationTypeDockerComposeValidate     OperationTypeEnum = "DOCKER_COMPOSE_VALIDATE"
	OperationTypeDockerComposeUp           OperationTypeEnum = "DOCKER_COMPOSE_UP"
	OperationTypeOther                     OperationTypeEnum = "OTHER"
)

var operationTypeDescriptions = map[OperationTypeEnum]string{
	OperationTypeUserLogin:                 "用户登录",
	OperationTypeUserLogout:                "用户登出",
	OperationTypeUserRefreshToken:          "刷新令牌",
	OperationTypeUserCreate:                "创建用户",
	OperationTypeUserUpdate:                "更新用户",
	OperationTypeUserDelete:                "删除用户",
	OperationTypeUserResetPassword:         "重置密码",
	OperationTypeUserUpdateStatus:          "更新用户状态",
	OperationTypeRoleCreate:                "创建角色",
	OperationTypeRoleUpdate:                "更新角色",
	OperationTypeRoleDelete:                "删除角色",
	OperationTypeRoleAssignPermission:      "分配角色权限",
	OperationTypeAdminForceLogout:          "管理员强制下线",
	OperationTypeAdminBanUser:              "管理员禁用用户",
	OperationTypeAdminUnbanUser:            "管理员启用用户",
	OperationTypeAdminUnlockAccount:        "管理员解锁账号",
	OperationTypeSystemConfigUpdate:        "系统配置更新",
	OperationTypeSystemCacheClear:          "系统缓存清理",
	OperationTypeSystemLogClear:            "系统日志清理",
	OperationTypeRuntimeLogQuery:           "查询运行日志",
	OperationTypeRuntimeLogStreamSubscribe: "订阅运行日志",
	OperationTypeFileUpload:                "文件上传",
	OperationTypeFileDelete:                "文件删除",
	OperationTypeDataExport:                "数据导出",
	OperationTypeDataImport:                "数据导入",
	OperationTypeConfigGroupCreate:         "创建配置分组",
	OperationTypeConfigGroupUpdate:         "更新配置分组",
	OperationTypeConfigGroupDelete:         "删除配置分组",
	OperationTypeConfigCreate:              "创建配置",
	OperationTypeConfigUpdate:              "更新配置",
	OperationTypeConfigDelete:              "删除配置",
	OperationTypeConfigEnabledChange:       "修改配置启用状态",
	OperationTypeConfigApply:               "应用待生效配置",
	OperationTypeConfigRollback:            "回滚配置",
	OperationTypeDockerContainerStart:      "启动 Docker 容器",
	OperationTypeDockerContainerStop:       "停止 Docker 容器",
	OperationTypeDockerContainerRestart:    "重启 Docker 容器",
	OperationTypeDockerContainerDelete:     "删除 Docker 容器",
	OperationTypeDockerContainerCreate:     "从镜像创建 Docker 容器",
	OperationTypeDockerImagePull:           "拉取 Docker 镜像",
	OperationTypeDockerImageTag:            "标记 Docker 镜像",
	OperationTypeDockerImagePush:           "推送 Docker 镜像",
	OperationTypeDockerImageDelete:         "删除 Docker 镜像",
	OperationTypeDockerRegistryCreate:      "创建 Docker Registry",
	OperationTypeDockerRegistryUpdate:      "更新 Docker Registry",
	OperationTypeDockerRegistryTest:        "测试 Docker Registry",
	OperationTypeDockerComposeValidate:     "校验 Docker Compose",
	OperationTypeDockerComposeUp:           "执行 Docker Compose Up",
	OperationTypeOther:                     "其他",
}

func (o OperationTypeEnum) Description() string {
	if desc, ok := operationTypeDescriptions[o]; ok {
		return desc
	}
	return string(o)
}

// DisplayLabel returns the label intended for a person reading a log record.
// OTHER remains a stable filter category, but a concrete audit description is
// more useful than the generic category name when it is available.
func (o OperationTypeEnum) DisplayLabel(operationDesc string) string {
	if o == OperationTypeOther {
		if description := strings.TrimSpace(operationDesc); description != "" {
			return description
		}
	}
	return o.Description()
}

func OperationTypes() []OperationTypeEnum {
	return []OperationTypeEnum{
		OperationTypeUserLogin,
		OperationTypeUserLogout,
		OperationTypeUserRefreshToken,
		OperationTypeUserCreate,
		OperationTypeUserUpdate,
		OperationTypeUserDelete,
		OperationTypeUserResetPassword,
		OperationTypeUserUpdateStatus,
		OperationTypeRoleCreate,
		OperationTypeRoleUpdate,
		OperationTypeRoleDelete,
		OperationTypeRoleAssignPermission,
		OperationTypeAdminForceLogout,
		OperationTypeAdminBanUser,
		OperationTypeAdminUnbanUser,
		OperationTypeAdminUnlockAccount,
		OperationTypeSystemConfigUpdate,
		OperationTypeSystemCacheClear,
		OperationTypeSystemLogClear,
		OperationTypeRuntimeLogQuery,
		OperationTypeRuntimeLogStreamSubscribe,
		OperationTypeFileUpload,
		OperationTypeFileDelete,
		OperationTypeDataExport,
		OperationTypeDataImport,
		OperationTypeConfigGroupCreate,
		OperationTypeConfigGroupUpdate,
		OperationTypeConfigGroupDelete,
		OperationTypeConfigCreate,
		OperationTypeConfigUpdate,
		OperationTypeConfigDelete,
		OperationTypeConfigEnabledChange,
		OperationTypeConfigApply,
		OperationTypeConfigRollback,
		OperationTypeDockerContainerStart,
		OperationTypeDockerContainerStop,
		OperationTypeDockerContainerRestart,
		OperationTypeDockerContainerDelete,
		OperationTypeDockerContainerCreate,
		OperationTypeDockerImagePull,
		OperationTypeDockerImageTag,
		OperationTypeDockerImagePush,
		OperationTypeDockerImageDelete,
		OperationTypeDockerRegistryCreate,
		OperationTypeDockerRegistryUpdate,
		OperationTypeDockerRegistryTest,
		OperationTypeDockerComposeValidate,
		OperationTypeDockerComposeUp,
		OperationTypeOther,
	}
}

// OperationTypeOption is a display-safe operation type choice returned to API clients.
// Value remains the stable storage and filter code, while Label is owned by the server.
type OperationTypeOption struct {
	Value string `json:"value"`
	Label string `json:"label"`
}

// OperationTypeOptions returns the current operation type catalog with server-owned labels.
func OperationTypeOptions() []OperationTypeOption {
	types := OperationTypes()
	options := make([]OperationTypeOption, 0, len(types))
	for _, operationType := range types {
		options = append(options, OperationTypeOption{
			Value: string(operationType),
			Label: operationType.Description(),
		})
	}
	return options
}

type OperationLogVO struct {
	ID                 int64      `json:"id"`
	UserID             int64      `json:"userId,omitempty"`
	UserName           string     `json:"userName,omitempty"`
	NickName           string     `json:"nickName,omitempty"`
	OperationType      string     `json:"operationType,omitempty"`
	OperationTypeDesc  string     `json:"operationTypeDesc,omitempty"`
	OperationTypeLabel string     `json:"operationTypeLabel,omitempty"`
	OperationDesc      string     `json:"operationDesc,omitempty"`
	MethodName         string     `json:"methodName,omitempty"`
	RequestMethod      string     `json:"requestMethod,omitempty"`
	RequestURL         string     `json:"requestUrl,omitempty"`
	TraceID            string     `json:"traceId,omitempty"`
	RequestParams      string     `json:"requestParams,omitempty"`
	ResponseResult     string     `json:"responseResult,omitempty"`
	RequestIP          string     `json:"requestIp,omitempty"`
	RequestLocation    string     `json:"requestLocation,omitempty"`
	UserAgent          string     `json:"userAgent,omitempty"`
	Browser            string     `json:"browser,omitempty"`
	OS                 string     `json:"os,omitempty"`
	OperationTime      *time.Time `json:"operationTime,omitempty"`
	ExecutionTime      int64      `json:"executionTime,omitempty"`
	Status             int        `json:"status"`
	ErrorMsg           string     `json:"errorMsg,omitempty"`
	CreateTime         *time.Time `json:"createTime,omitempty"`
}

type OperationLogExportDTO struct {
	ID                 int64      `json:"id"`
	OperationTime      *time.Time `json:"operationTime,omitempty"`
	UserName           string     `json:"userName,omitempty"`
	NickName           string     `json:"nickName,omitempty"`
	OperationType      string     `json:"operationType,omitempty"`
	OperationTypeDesc  string     `json:"operationTypeDesc,omitempty"`
	OperationTypeLabel string     `json:"operationTypeLabel,omitempty"`
	OperationDesc      string     `json:"operationDesc,omitempty"`
	MethodName         string     `json:"methodName,omitempty"`
	RequestMethod      string     `json:"requestMethod,omitempty"`
	RequestURL         string     `json:"requestUrl,omitempty"`
	TraceID            string     `json:"traceId,omitempty"`
	RequestIP          string     `json:"requestIp,omitempty"`
	RequestLocation    string     `json:"requestLocation,omitempty"`
	Browser            string     `json:"browser,omitempty"`
	OS                 string     `json:"os,omitempty"`
	ExecutionTime      int64      `json:"executionTime,omitempty"`
	Status             string     `json:"status,omitempty"`
	ErrorMsg           string     `json:"errorMsg,omitempty"`
	CreateTime         *time.Time `json:"createTime,omitempty"`
}

type RuntimeLogLineDTO struct {
	LineID     string         `json:"lineId"`
	LogTime    *time.Time     `json:"logTime,omitempty"`
	Level      string         `json:"level,omitempty"`
	ThreadName string         `json:"threadName,omitempty"`
	LoggerName string         `json:"loggerName,omitempty"`
	TraceID    string         `json:"traceId,omitempty"`
	Message    string         `json:"message,omitempty"`
	Source     map[string]any `json:"source,omitempty"`
	FileName   string         `json:"fileName,omitempty"`
	LineNumber int            `json:"lineNumber,omitempty"`
}

type RuntimeLogQueryDTO struct {
	Current        int64   `query:"current" json:"current"`
	Size           int64   `query:"size" json:"size"`
	Keyword        string  `query:"keyword" json:"keyword,omitempty"`
	ContentKeyword string  `query:"contentKeyword" json:"contentKeyword,omitempty"`
	Level          string  `query:"level" json:"level,omitempty"`
	LoggerName     string  `query:"loggerName" json:"loggerName,omitempty"`
	ThreadName     string  `query:"threadName" json:"threadName,omitempty"`
	TraceID        string  `query:"traceId" json:"traceId,omitempty"`
	StartTime      *string `query:"startTime" json:"startTime,omitempty"`
	EndTime        *string `query:"endTime" json:"endTime,omitempty"`
	UseRegex       bool    `query:"useRegex" json:"useRegex,omitempty"`
}

type RuntimeLogStreamRequestDTO struct {
	Keyword        string `query:"keyword" json:"keyword,omitempty"`
	ContentKeyword string `query:"contentKeyword" json:"contentKeyword,omitempty"`
	Level          string `query:"level" json:"level,omitempty"`
	LoggerName     string `query:"loggerName" json:"loggerName,omitempty"`
	ThreadName     string `query:"threadName" json:"threadName,omitempty"`
	TraceID        string `query:"traceId" json:"traceId,omitempty"`
	LastN          int    `query:"lastN" json:"lastN,omitempty"`
	UseRegex       bool   `query:"useRegex" json:"useRegex,omitempty"`
}

type OperationLogQueryRequest struct {
	Current       int64   `query:"current" json:"current"`
	Size          int64   `query:"size" json:"size"`
	OperationType *string `query:"operationType" json:"operationType,omitempty"`
	Username      string  `query:"username" json:"username,omitempty"`
	StartTime     *string `query:"startTime" json:"startTime,omitempty"`
	EndTime       *string `query:"endTime" json:"endTime,omitempty"`
}

type MyOperationLogQueryRequest struct {
	Current          int64   `query:"current" json:"current"`
	Size             int64   `query:"size" json:"size"`
	OperationType    string  `query:"operationType" json:"operationType,omitempty"`
	RequestMethod    string  `query:"requestMethod" json:"requestMethod,omitempty"`
	RequestURL       string  `query:"requestUrl" json:"requestUrl,omitempty"`
	ExecutionTimeMin *int64  `query:"executionTimeMin" json:"executionTimeMin,omitempty"`
	ExecutionTimeMax *int64  `query:"executionTimeMax" json:"executionTimeMax,omitempty"`
	StartTime        *string `query:"startTime" json:"startTime,omitempty"`
	EndTime          *string `query:"endTime" json:"endTime,omitempty"`
}

type OperationLogDeleteByTimeRangeRequest struct {
	StartTime string `query:"startTime" json:"startTime"`
	EndTime   string `query:"endTime" json:"endTime"`
}

type OperationLogExportRequest struct {
	OperationType *string `query:"operationType" json:"operationType,omitempty"`
	StartTime     *string `query:"startTime" json:"startTime,omitempty"`
	EndTime       *string `query:"endTime" json:"endTime,omitempty"`
}

type RuntimeLogStreamEvent struct {
	Event string `json:"event"`
	Data  any    `json:"data,omitempty"`
}

type OperationLogEntry struct {
	UserID          int64
	UserName        string
	NickName        string
	OperationType   OperationTypeEnum
	OperationDesc   string
	MethodName      string
	RequestMethod   string
	RequestURL      string
	TraceID         string
	RequestParams   string
	ResponseResult  string
	RequestIP       string
	RequestLocation string
	UserAgent       string
	Browser         string
	OS              string
	OperationTime   time.Time
	ExecutionTime   int64
	Status          int
	ErrorMsg        string
}

type OperationLogEnricher interface {
	Enrich(ctx context.Context, reqCtx *app.RequestContext, entry *OperationLogEntry)
}

type OperationLogSpec struct {
	Operation           OperationTypeEnum
	Description         string
	IncludeParams       bool
	IncludeResult       bool
	OmitQuery           bool
	Enrichers           []OperationLogEnricher
	CompletionEnrichers []OperationLogEnricher
}
