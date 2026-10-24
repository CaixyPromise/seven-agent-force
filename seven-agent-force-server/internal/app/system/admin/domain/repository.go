package domain

import "context"

type OperationLogRepository interface {
	InsertOperationLog(ctx context.Context, item *OperationLog) (int64, error)
	FindOperationLogByID(ctx context.Context, id int64) (*OperationLog, error)
	QueryOperationLogs(ctx context.Context, query OperationLogPageQuery) ([]OperationLog, int64, error)
	DeleteOperationLogsBeforeDays(ctx context.Context, days int) (int64, error)
	DeleteOperationLogsByTimeRange(ctx context.Context, start, end string) (int64, error)
}

type OnlineUserStateStore interface {
	GetByUserID(ctx context.Context, userID int64) (*OnlineUser, bool, error)
	Save(ctx context.Context, userID int64, item *OnlineUser) error
	Delete(ctx context.Context, userID int64) error
}

type LoginFailureStateStore interface {
	GetFailureCount(ctx context.Context, userAccount string) (int, error)
	SaveFailureCount(ctx context.Context, userAccount string, count int) error
	DeleteFailureCount(ctx context.Context, userAccount string) error
	GetContextFailureCount(ctx context.Context, scope, value string) (int, error)
	SaveContextFailureCount(ctx context.Context, scope, value string, count int) error
	GetCaptchaFailureCount(ctx context.Context, userAccount string) (int, error)
	SaveCaptchaFailureCount(ctx context.Context, userAccount string, count int) error
	DeleteCaptchaFailureCount(ctx context.Context, userAccount string) error
	GetLockUntil(ctx context.Context, userAccount string) (*int64, error)
	SaveLockUntil(ctx context.Context, userAccount string, unlockTime int64, ttlHours int) error
	DeleteLock(ctx context.Context, userAccount string) error
}
