package application

import (
	"context"
	"errors"
	"time"
)

var (
	ErrRangeInvalid    = errors.New("ANALYTICS_RANGE_INVALID")
	ErrProjectNotFound = errors.New("PROJECT_NOT_FOUND")
)

type Window struct {
	From time.Time
	To   time.Time
	Days int
}

func ParseWindow(value string, allowed map[string]int, now time.Time) (Window, error) {
	days, ok := allowed[value]
	if !ok {
		return Window{}, ErrRangeInvalid
	}
	to := now.UTC().Truncate(24 * time.Hour).Add(24 * time.Hour)
	return Window{From: to.AddDate(0, 0, -days), To: to, Days: days}, nil
}

type TaskTrendPoint struct {
	At        time.Time `json:"at"`
	Created   int64     `json:"created"`
	Succeeded int64     `json:"succeeded"`
	Failed    int64     `json:"failed"`
	Cancelled int64     `json:"cancelled"`
}

type DurationPoint struct {
	At          time.Time `json:"at"`
	SampleCount int64     `json:"sampleCount"`
	P50Seconds  *float64  `json:"p50Seconds"`
	P90Seconds  *float64  `json:"p90Seconds"`
}

type Count struct {
	Code  string `json:"code"`
	Count int64  `json:"count"`
}

type ProjectListSummary struct {
	ActiveProjectCount    int64 `json:"activeProjectCount"`
	CreatedTaskCount      int64 `json:"createdTaskCount"`
	SucceededTaskCount    int64 `json:"succeededTaskCount"`
	AttentionProjectCount int64 `json:"attentionProjectCount"`
}

type ProjectHealth struct {
	ProjectID      int64     `json:"projectId,string"`
	Name           string    `json:"name"`
	Succeeded      int64     `json:"succeeded"`
	Failed         int64     `json:"failed"`
	Active         int64     `json:"active"`
	Attention      int64     `json:"attention"`
	LatestActivity time.Time `json:"latestActivity"`
}

type ProjectListAnalytics struct {
	From          time.Time          `json:"from"`
	To            time.Time          `json:"to"`
	Summary       ProjectListSummary `json:"summary"`
	TaskTrend     []TaskTrendPoint   `json:"taskTrend"`
	ProjectHealth []ProjectHealth    `json:"projectHealth"`
}

type ProjectSummary struct {
	CreatedTaskCount        int64    `json:"createdTaskCount"`
	SuccessRate             *float64 `json:"successRate"`
	SuccessfulDeliveryCount int64    `json:"successfulDeliveryCount"`
	AttentionTaskCount      int64    `json:"attentionTaskCount"`
}

type StageHealth struct {
	Code          string `json:"code"`
	EligibleCount int64  `json:"eligibleCount"`
	ReachedCount  int64  `json:"reachedCount"`
	FailedCount   int64  `json:"failedCount"`
}

type ReviewOutcomes struct {
	Disabled            int64 `json:"disabled"`
	DirectPassed        int64 `json:"directPassed"`
	PassedAfterRevision int64 `json:"passedAfterRevision"`
	AcceptedRisk        int64 `json:"acceptedRisk"`
	Rejected            int64 `json:"rejected"`
	Processing          int64 `json:"processing"`
}

type ModelUsagePoint struct {
	At                       time.Time `json:"at"`
	InvocationCount          int64     `json:"invocationCount"`
	InputTokens              *int64    `json:"inputTokens"`
	CachedInputTokens        *int64    `json:"cachedInputTokens"`
	CacheCreationInputTokens *int64    `json:"cacheCreationInputTokens"`
	OutputTokens             *int64    `json:"outputTokens"`
	ReasoningOutputTokens    *int64    `json:"reasoningOutputTokens"`
	TotalTokens              *int64    `json:"totalTokens"`
	CacheHitRate             *float64  `json:"cacheHitRate"`
}

type ProjectAnalytics struct {
	From            time.Time         `json:"from"`
	To              time.Time         `json:"to"`
	Summary         ProjectSummary    `json:"summary"`
	TaskTrend       []TaskTrendPoint  `json:"taskTrend"`
	StageHealth     []StageHealth     `json:"stageHealth"`
	DurationTrend   []DurationPoint   `json:"durationTrend"`
	ReviewOutcomes  ReviewOutcomes    `json:"reviewOutcomes"`
	WaitingReasons  []Count           `json:"waitingReasons"`
	ModelUsageTrend []ModelUsagePoint `json:"modelUsageTrend"`
}

type TaskSummary struct {
	Attention int64 `json:"attention"`
	Running   int64 `json:"running"`
	Waiting   int64 `json:"waiting"`
	Succeeded int64 `json:"succeeded"`
}

type ActivityPoint struct {
	At        time.Time `json:"at"`
	Created   int64     `json:"created"`
	Terminal  int64     `json:"terminal"`
	Succeeded int64     `json:"succeeded"`
	Failed    int64     `json:"failed"`
	Backlog   int64     `json:"backlog"`
}

type TaskAnalytics struct {
	From              time.Time       `json:"from"`
	To                time.Time       `json:"to"`
	Summary           TaskSummary     `json:"summary"`
	ActivityTrend     []ActivityPoint `json:"activityTrend"`
	StatusCounts      []Count         `json:"statusCounts"`
	WaitingAgeBuckets []Count         `json:"waitingAgeBuckets"`
	WaitingReasons    []Count         `json:"waitingReasons"`
	DurationTrend     []DurationPoint `json:"durationTrend"`
}

type WorkbenchSummary struct {
	AttentionCount      int64    `json:"attentionCount"`
	RunningTaskCount    int64    `json:"runningTaskCount"`
	QueuedTaskCount     int64    `json:"queuedTaskCount"`
	CompletedTodayCount int64    `json:"completedTodayCount"`
	TodaySuccessRate    *float64 `json:"todaySuccessRate"`
}

type WorkbenchTask struct {
	TaskID          int64      `json:"taskId,string"`
	ProjectID       int64      `json:"projectId,string"`
	Name            string     `json:"name"`
	ProjectName     string     `json:"projectName"`
	Status          string     `json:"status"`
	RuntimeName     string     `json:"runtimeName"`
	ProgressSummary string     `json:"progressSummary"`
	WaitReasonCode  string     `json:"waitReasonCode,omitempty"`
	UpdatedAt       time.Time  `json:"updatedAt"`
	StartedAt       *time.Time `json:"startedAt,omitempty"`
}

type WorkbenchAttentionItem struct {
	TaskID      int64     `json:"taskId,string"`
	ProjectID   int64     `json:"projectId,string"`
	ObjectID    int64     `json:"objectId,string,omitempty"`
	TaskName    string    `json:"taskName"`
	ProjectName string    `json:"projectName"`
	ReasonCode  string    `json:"reasonCode"`
	ObjectKind  string    `json:"objectKind"`
	OccurredAt  time.Time `json:"occurredAt"`
}

type WorkbenchActivityPoint struct {
	At        time.Time `json:"at"`
	Created   int64     `json:"created"`
	Completed int64     `json:"completed"`
	Backlog   int64     `json:"backlog"`
}

type WorkbenchWorker struct {
	WorkerID     int64  `json:"workerId,string"`
	Name         string `json:"name"`
	RunningSlots int64  `json:"runningSlots"`
	MaximumSlots int64  `json:"maximumSlots"`
}

type WorkbenchResources struct {
	OnlineWorkerCount int64             `json:"onlineWorkerCount"`
	RunningSlots      int64             `json:"runningSlots"`
	MaximumSlots      int64             `json:"maximumSlots"`
	QueuedTasks       int64             `json:"queuedTasks"`
	Workers           []WorkbenchWorker `json:"workers"`
}

type WorkbenchModelUsagePoint struct {
	At          time.Time `json:"at"`
	TotalTokens *int64    `json:"totalTokens"`
}

type WorkbenchModelUsage struct {
	InvocationCount int64                      `json:"invocationCount"`
	TotalTokens     *int64                     `json:"totalTokens"`
	CacheHitRate    *float64                   `json:"cacheHitRate"`
	Trend           []WorkbenchModelUsagePoint `json:"trend"`
}

type WorkbenchDelivery struct {
	DeliveryID  int64     `json:"deliveryId,string"`
	TaskID      int64     `json:"taskId,string"`
	ProjectID   int64     `json:"projectId,string"`
	TaskName    string    `json:"taskName"`
	ProjectName string    `json:"projectName"`
	Status      string    `json:"status"`
	UpdatedAt   time.Time `json:"updatedAt"`
}

type Workbench struct {
	From             time.Time                `json:"from"`
	To               time.Time                `json:"to"`
	Summary          WorkbenchSummary         `json:"summary"`
	ActiveTasks      []WorkbenchTask          `json:"activeTasks"`
	AttentionItems   []WorkbenchAttentionItem `json:"attentionItems"`
	ActivityTrend    []WorkbenchActivityPoint `json:"activityTrend"`
	Resources        WorkbenchResources       `json:"resources"`
	ModelUsage       *WorkbenchModelUsage     `json:"modelUsage"`
	RecentDeliveries []WorkbenchDelivery      `json:"recentDeliveries"`
}

type Repository interface {
	ProjectList(context.Context, int64, Window) (ProjectListAnalytics, error)
	Project(context.Context, int64, int64, Window) (ProjectAnalytics, error)
	Tasks(context.Context, int64, Window) (TaskAnalytics, error)
	Workbench(context.Context, int64, Window, bool) (Workbench, error)
}

func (s *Service) Workbench(ctx context.Context, tenantID int64, value string, includeModelUsage bool) (Workbench, error) {
	w, err := ParseWindow(value, map[string]int{"7d": 7, "14d": 14, "30d": 30}, s.now())
	if err != nil {
		return Workbench{}, err
	}
	return s.repository.Workbench(ctx, tenantID, w, includeModelUsage)
}

type Service struct {
	repository Repository
	now        func() time.Time
}

func New(repository Repository) *Service { return &Service{repository: repository, now: time.Now} }

func (s *Service) ProjectList(ctx context.Context, tenantID int64, value string) (ProjectListAnalytics, error) {
	w, err := ParseWindow(value, map[string]int{"7d": 7, "30d": 30, "90d": 90}, s.now())
	if err != nil {
		return ProjectListAnalytics{}, err
	}
	return s.repository.ProjectList(ctx, tenantID, w)
}

func (s *Service) Project(ctx context.Context, tenantID, projectID int64, value string) (ProjectAnalytics, error) {
	w, err := ParseWindow(value, map[string]int{"7d": 7, "30d": 30, "90d": 90}, s.now())
	if err != nil {
		return ProjectAnalytics{}, err
	}
	return s.repository.Project(ctx, tenantID, projectID, w)
}

func (s *Service) Tasks(ctx context.Context, tenantID int64, value string) (TaskAnalytics, error) {
	w, err := ParseWindow(value, map[string]int{"7d": 7, "14d": 14, "30d": 30}, s.now())
	if err != nil {
		return TaskAnalytics{}, err
	}
	return s.repository.Tasks(ctx, tenantID, w)
}
