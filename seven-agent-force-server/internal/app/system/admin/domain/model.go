package domain

import "time"

type OnlineUser struct {
	UserID           int64
	Username         string
	Nickname         string
	Avatar           string
	Email            string
	UserRole         string
	LoginTime        *time.Time
	LastActiveTime   *time.Time
	ExpireTime       *time.Time
	LoginIP          string
	LoginAddress     string
	Browser          string
	OS               string
	DeviceID         string
	UserAgent        string
	TokenID          string
	IsCurrentSession bool
}

type OnlineUserStats struct {
	TotalOnlineUsers int64
	AdminUsers       int64
	NormalUsers      int64
	BrowserStats     map[string]int64
	OSStats          map[string]int64
	TodayLoginUsers  int64
	ActiveUsers      int64
	TotalOnline      int64
	TodayLogin       int64
	PeakOnline       int64
}

type BatchLogoutResult struct {
	SuccessIDs     []int64
	FailedIDs      []int64
	TotalCount     int
	SuccessCount   int
	FailedCount    int
	FailureReasons []string
}

type OperationLog struct {
	ID              int64
	UserID          int64
	UserName        string
	NickName        string
	OperationType   string
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
	OperationTime   *time.Time
	ExecutionTime   int64
	Status          int
	ErrorMsg        string
	CreateTime      *time.Time
	UpdateTime      *time.Time
	IsDeleted       int
}

type OperationLogPageQuery struct {
	Current          int64
	Size             int64
	OperationType    string
	Username         string
	RequestMethod    string
	RequestURL       string
	UserID           int64
	ExecutionTimeMin *int64
	ExecutionTimeMax *int64
	StartTime        *time.Time
	EndTime          *time.Time
}

type RuntimeLogLine struct {
	LineID     string
	LogTime    *time.Time
	Level      string
	ThreadName string
	LoggerName string
	TraceID    string
	Message    string
	Source     map[string]any
	FileName   string
	LineNumber int
}

type RuntimeLogPageQuery struct {
	Current        int64
	Size           int64
	Keyword        string
	ContentKeyword string
	Level          string
	LoggerName     string
	ThreadName     string
	TraceID        string
	StartTime      *time.Time
	EndTime        *time.Time
	UseRegex       bool
}

type RuntimeLogStreamQuery struct {
	Keyword        string
	ContentKeyword string
	Level          string
	LoggerName     string
	ThreadName     string
	TraceID        string
	LastN          int
	UseRegex       bool
}
