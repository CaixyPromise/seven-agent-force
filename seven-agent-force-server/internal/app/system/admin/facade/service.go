package facade

import (
	"context"
	"io"

	"github.com/cloudwego/hertz/pkg/app"
)

type LoginFailureFacade interface {
	RecordFailure(ctx context.Context, userAccount, clientIP, deviceID string) error
	ClearFailure(ctx context.Context, userAccount string) error
	NeedCaptcha(ctx context.Context, userAccount string) (bool, error)
	IsAccountLocked(ctx context.Context, userAccount string) (bool, error)
	GetUnlockTime(ctx context.Context, userAccount string) (*int64, error)
	GetFailureCount(ctx context.Context, userAccount string) (int, error)
	GetRiskFailureCount(ctx context.Context, userAccount, clientIP, deviceID string) (int, error)
	UnlockAccount(ctx context.Context, userAccount string) error
	LockAccount(ctx context.Context, userAccount string, lockHours int) error
	RecordCaptchaFailure(ctx context.Context, userAccount, clientIP string) error
	ClearCaptchaFailure(ctx context.Context, userAccount string) error
	GetCaptchaFailureCount(ctx context.Context, userAccount string) (int, error)
}

type OnlineUserFacade interface {
	GetOnlineUsers(ctx context.Context, current, size int64, username, loginIP, browser, os string, currentSessionID string) (*PageResult[OnlineUserVO], error)
	GetOnlineUserStats(ctx context.Context) (*OnlineUserStatsVO, error)
	GetUserSession(ctx context.Context, userID int64, currentSessionID string) (*OnlineUserVO, error)
	ForceLogout(ctx context.Context, command ForceLogoutCommand) (bool, error)
	BatchForceLogout(ctx context.Context, command BatchForceLogoutCommand) (*BatchLogoutResultVO, error)
	GetOnlineUserCount(ctx context.Context) (int64, error)
	IsUserOnline(ctx context.Context, userID int64) (bool, error)
}

type OperationLogFacade interface {
	SaveLogAsync(ctx context.Context, entry OperationLogEntry)
	SaveLog(ctx context.Context, entry OperationLogEntry) error
	GetOperationLogs(ctx context.Context, request OperationLogQueryRequest) (*PageResult[OperationLogVO], error)
	GetOperationLogByID(ctx context.Context, id int64) (*OperationLogVO, error)
	CleanExpiredLogs(ctx context.Context, days int) (int64, error)
	ExportOperationLogs(ctx context.Context, request OperationLogExportRequest, currentUserID int64) ([]OperationLogExportDTO, error)
	DeleteLogsByTimeRange(ctx context.Context, startTime, endTime string) (int64, error)
	GetMyOperationLogs(ctx context.Context, currentUserID int64, request MyOperationLogQueryRequest) (*PageResult[OperationLogVO], error)
	GetOperationTypes(ctx context.Context) []OperationTypeOption
}

type RuntimeLogFacade interface {
	Page(ctx context.Context, request RuntimeLogQueryDTO) (*PageResult[RuntimeLogLineDTO], error)
	Stream(ctx context.Context, request RuntimeLogStreamRequestDTO, userID int64) (io.ReadCloser, error)
}

type OperationLogger interface {
	Wrap(spec OperationLogSpec, handler app.HandlerFunc) app.HandlerFunc
}
