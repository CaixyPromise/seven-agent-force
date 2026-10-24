package infrastructure

import (
	"context"
	"database/sql"

	"github.com/jackc/pgx/v5/pgxpool"

	analytics "github.com/CaixyPromise/seven-agent/force/internal/app/workforce/taskanalytics/application"
)

type Store struct{ db *pgxpool.Pool }

func New(db *pgxpool.Pool) *Store { return &Store{db: db} }

func (s *Store) Workbench(ctx context.Context, tenantID int64, window analytics.Window, includeModelUsage bool) (analytics.Workbench, error) {
	result := analytics.Workbench{
		From: window.From, To: window.To,
		ActiveTasks: []analytics.WorkbenchTask{}, AttentionItems: []analytics.WorkbenchAttentionItem{},
		ActivityTrend: []analytics.WorkbenchActivityPoint{}, RecentDeliveries: []analytics.WorkbenchDelivery{},
		Resources: analytics.WorkbenchResources{Workers: []analytics.WorkbenchWorker{}},
	}
	var succeededToday, failedToday int64
	err := s.db.QueryRow(ctx, `
WITH scoped AS (
 SELECT t.*,
  CASE
   WHEN EXISTS (SELECT 1 FROM workforce_agent_interactions i WHERE i.tenant_id=t.tenant_id AND i.task_id=t.id AND i.kind='QUESTION' AND i.is_blocking AND i.state='OPEN' AND i.expires_at>CURRENT_TIMESTAMP) THEN 'WAITING_USER_INPUT'
   WHEN (SELECT d.status FROM workforce_delivery_requests d WHERE d.tenant_id=t.tenant_id AND d.task_id=t.id ORDER BY d.created_at DESC,d.id DESC LIMIT 1)='WAITING_APPROVAL' THEN 'WAITING_DELIVERY_DECISION'
   WHEN t.status='MANUAL_REVIEW' THEN 'MANUAL_REVIEW'
   WHEN (SELECT d.status FROM workforce_delivery_requests d WHERE d.tenant_id=t.tenant_id AND d.task_id=t.id ORDER BY d.created_at DESC,d.id DESC LIMIT 1) IN ('FAILED','MANUAL_REVIEW') THEN 'DELIVERY_FAILED'
   WHEN t.status IN ('FAILED','TIMED_OUT') THEN 'EXECUTION_FAILED'
   ELSE '' END AS attention_reason
 FROM workforce_tasks t WHERE t.tenant_id=$1
)
SELECT
 count(*) FILTER (WHERE attention_reason<>''),
 count(*) FILTER (WHERE status IN ('RUNNING','VALIDATING','REVIEWING','PACKAGING','UPLOADING','CANCELLING')),
 count(*) FILTER (WHERE status IN ('CREATED','QUEUED','PREPARING') AND NOT EXISTS (SELECT 1 FROM workforce_task_assignments a WHERE a.task_id=scoped.id)),
 count(*) FILTER (WHERE finished_at>=date_trunc('day',CURRENT_TIMESTAMP AT TIME ZONE 'UTC') AT TIME ZONE 'UTC' AND status IN ('SUCCEEDED','FAILED','TIMED_OUT','CANCELLED')),
 count(*) FILTER (WHERE finished_at>=date_trunc('day',CURRENT_TIMESTAMP AT TIME ZONE 'UTC') AT TIME ZONE 'UTC' AND status='SUCCEEDED'),
 count(*) FILTER (WHERE finished_at>=date_trunc('day',CURRENT_TIMESTAMP AT TIME ZONE 'UTC') AT TIME ZONE 'UTC' AND status IN ('FAILED','TIMED_OUT'))
FROM scoped`, tenantID).Scan(&result.Summary.AttentionCount, &result.Summary.RunningTaskCount, &result.Summary.QueuedTaskCount, &result.Summary.CompletedTodayCount, &succeededToday, &failedToday)
	if err != nil {
		return result, err
	}
	if succeededToday+failedToday > 0 {
		value := float64(succeededToday) / float64(succeededToday+failedToday)
		result.Summary.TodaySuccessRate = &value
	}

	rows, err := s.db.Query(ctx, `
WITH ranked AS (
 SELECT t.id,t.project_id,t.name,pv.name project_name,t.status,COALESCE(hr.alias,'') runtime_name,COALESCE(NULLIF(t.result_summary,''),NULLIF(t.task_instruction,''),'') progress_summary,
  `+attentionSQL+` wait_reason,
  GREATEST(t.created_at,COALESCE(t.started_at,t.created_at),COALESCE((SELECT max(n.updated_at) FROM workforce_execution_nodes n WHERE n.task_id=t.id),t.created_at)) updated_at,
  t.started_at,
  CASE WHEN `+attentionSQL+`<>'' THEN 0 ELSE 1 END attention_rank
 FROM workforce_tasks t
 JOIN workforce_project_versions pv ON pv.tenant_id=t.tenant_id AND pv.id=t.project_version_id
 LEFT JOIN workforce_harness_releases hr ON hr.id=t.selected_runtime_release_id
 WHERE t.tenant_id=$1 AND t.status NOT IN ('SUCCEEDED','FAILED','TIMED_OUT','CANCELLED')
)
SELECT id,project_id,name,project_name,status,runtime_name,LEFT(progress_summary,240),wait_reason,updated_at,started_at
FROM ranked ORDER BY attention_rank,updated_at DESC,id DESC LIMIT 4`, tenantID)
	if err != nil {
		return result, err
	}
	for rows.Next() {
		var item analytics.WorkbenchTask
		if err = rows.Scan(&item.TaskID, &item.ProjectID, &item.Name, &item.ProjectName, &item.Status, &item.RuntimeName, &item.ProgressSummary, &item.WaitReasonCode, &item.UpdatedAt, &item.StartedAt); err != nil {
			rows.Close()
			return result, err
		}
		result.ActiveTasks = append(result.ActiveTasks, item)
	}
	rows.Close()
	if err = rows.Err(); err != nil {
		return result, err
	}

	rows, err = s.db.Query(ctx, `
WITH facts AS (
 SELECT t.id task_id,t.project_id,t.name task_name,pv.name project_name,
  CASE
   WHEN q.id IS NOT NULL THEN 'WAITING_USER_INPUT'
   WHEN d.status='WAITING_APPROVAL' THEN 'WAITING_DELIVERY_DECISION'
   WHEN t.status='MANUAL_REVIEW' THEN 'MANUAL_REVIEW'
   WHEN d.status IN ('FAILED','MANUAL_REVIEW') THEN 'DELIVERY_FAILED'
   WHEN t.status IN ('FAILED','TIMED_OUT') THEN 'EXECUTION_FAILED'
   ELSE '' END reason_code,
  CASE WHEN q.id IS NOT NULL THEN 'CONVERSATION_ITEM' WHEN d.status<>'' THEN 'DELIVERY_REQUEST' WHEN t.status='MANUAL_REVIEW' THEN 'REVIEW' ELSE 'TASK' END object_kind,
  COALESCE(q.id,d.id,0) object_id,
  COALESCE(q.created_at,d.updated_at,t.finished_at,t.created_at) occurred_at
 FROM workforce_tasks t
 JOIN workforce_project_versions pv ON pv.tenant_id=t.tenant_id AND pv.id=t.project_version_id
 LEFT JOIN LATERAL (SELECT i.id,i.created_at FROM workforce_agent_interactions i WHERE i.tenant_id=t.tenant_id AND i.task_id=t.id AND i.kind='QUESTION' AND i.is_blocking AND i.state='OPEN' AND i.expires_at>CURRENT_TIMESTAMP ORDER BY i.created_at DESC,i.id DESC LIMIT 1) q ON true
 LEFT JOIN LATERAL (SELECT x.id,x.status,x.updated_at FROM workforce_delivery_requests x WHERE x.tenant_id=t.tenant_id AND x.task_id=t.id ORDER BY x.created_at DESC,x.id DESC LIMIT 1) d ON true
 WHERE t.tenant_id=$1
)
SELECT task_id,project_id,object_id,task_name,project_name,reason_code,object_kind,occurred_at
FROM facts WHERE reason_code<>'' ORDER BY occurred_at DESC,task_id DESC LIMIT 3`, tenantID)
	if err != nil {
		return result, err
	}
	for rows.Next() {
		var item analytics.WorkbenchAttentionItem
		if err = rows.Scan(&item.TaskID, &item.ProjectID, &item.ObjectID, &item.TaskName, &item.ProjectName, &item.ReasonCode, &item.ObjectKind, &item.OccurredAt); err != nil {
			rows.Close()
			return result, err
		}
		result.AttentionItems = append(result.AttentionItems, item)
	}
	rows.Close()

	rows, err = s.db.Query(ctx, `
WITH days AS (SELECT generate_series($2::timestamptz,$3::timestamptz-INTERVAL '1 day',INTERVAL '1 day') AS bucket_at),
baseline AS (SELECT count(*) value FROM workforce_tasks WHERE tenant_id=$1 AND created_at<$2 AND (finished_at IS NULL OR finished_at>=$2)),
daily AS (
 SELECT d.bucket_at,
	 count(t.id) FILTER (WHERE t.created_at>=d.bucket_at AND t.created_at<d.bucket_at+INTERVAL '1 day') created,
	 count(t.id) FILTER (WHERE t.finished_at>=d.bucket_at AND t.finished_at<d.bucket_at+INTERVAL '1 day') completed
	 FROM days d LEFT JOIN workforce_tasks t ON t.tenant_id=$1 AND ((t.created_at>=d.bucket_at AND t.created_at<d.bucket_at+INTERVAL '1 day') OR (t.finished_at>=d.bucket_at AND t.finished_at<d.bucket_at+INTERVAL '1 day')) GROUP BY d.bucket_at
)
SELECT bucket_at,created,completed,(SELECT value FROM baseline)+sum(created-completed) OVER (ORDER BY bucket_at) backlog FROM daily ORDER BY bucket_at`, tenantID, window.From, window.To)
	if err != nil {
		return result, err
	}
	for rows.Next() {
		var item analytics.WorkbenchActivityPoint
		if err = rows.Scan(&item.At, &item.Created, &item.Completed, &item.Backlog); err != nil {
			rows.Close()
			return result, err
		}
		result.ActivityTrend = append(result.ActivityTrend, item)
	}
	rows.Close()

	err = s.db.QueryRow(ctx, `
WITH online AS (
 SELECT w.id,w.name,s.active_attempts,s.max_active_attempts
 FROM workforce_workers w JOIN workforce_worker_sessions s ON s.instance_id=w.current_instance_id
 WHERE w.desired_state='ENABLED' AND s.ended_at IS NULL AND s.consumer_state='CONSUMING' AND s.last_heartbeat_at>=CURRENT_TIMESTAMP-INTERVAL '35 seconds'
)
SELECT count(*),COALESCE(sum(active_attempts),0),COALESCE(sum(max_active_attempts),0),
 (SELECT count(*) FROM workforce_tasks t WHERE t.tenant_id=$1 AND t.status IN ('CREATED','QUEUED','PREPARING') AND NOT EXISTS (SELECT 1 FROM workforce_task_assignments a WHERE a.task_id=t.id))
FROM online`, tenantID).Scan(&result.Resources.OnlineWorkerCount, &result.Resources.RunningSlots, &result.Resources.MaximumSlots, &result.Resources.QueuedTasks)
	if err != nil {
		return result, err
	}
	rows, err = s.db.Query(ctx, `SELECT w.id,w.name,s.active_attempts,s.max_active_attempts FROM workforce_workers w JOIN workforce_worker_sessions s ON s.instance_id=w.current_instance_id WHERE w.desired_state='ENABLED' AND s.ended_at IS NULL AND s.consumer_state='CONSUMING' AND s.last_heartbeat_at>=CURRENT_TIMESTAMP-INTERVAL '35 seconds' ORDER BY s.active_attempts::numeric/GREATEST(s.max_active_attempts,1) DESC,w.id LIMIT 2`)
	if err != nil {
		return result, err
	}
	for rows.Next() {
		var item analytics.WorkbenchWorker
		if err = rows.Scan(&item.WorkerID, &item.Name, &item.RunningSlots, &item.MaximumSlots); err != nil {
			rows.Close()
			return result, err
		}
		result.Resources.Workers = append(result.Resources.Workers, item)
	}
	rows.Close()

	if includeModelUsage {
		usage := &analytics.WorkbenchModelUsage{Trend: []analytics.WorkbenchModelUsagePoint{}}
		var totalTokens, inputTokens, cachedTokens sql.NullInt64
		err = s.db.QueryRow(ctx, `SELECT count(*),sum(total_tokens),sum(input_tokens),sum(cached_input_tokens) FROM workforce_model_invocations WHERE tenant_id=$1 AND started_at>=date_trunc('day',CURRENT_TIMESTAMP AT TIME ZONE 'UTC') AT TIME ZONE 'UTC'`, tenantID).Scan(&usage.InvocationCount, &totalTokens, &inputTokens, &cachedTokens)
		if err != nil {
			return result, err
		}
		if totalTokens.Valid {
			usage.TotalTokens = &totalTokens.Int64
		}
		if inputTokens.Valid && inputTokens.Int64 > 0 && cachedTokens.Valid {
			value := float64(cachedTokens.Int64) / float64(inputTokens.Int64)
			usage.CacheHitRate = &value
		}
		rows, err = s.db.Query(ctx, `WITH days AS (SELECT generate_series($2::timestamptz,$3::timestamptz-INTERVAL '1 day',INTERVAL '1 day') AS bucket_at) SELECT d.bucket_at,sum(m.total_tokens) FROM days d LEFT JOIN workforce_model_invocations m ON m.tenant_id=$1 AND m.started_at>=d.bucket_at AND m.started_at<d.bucket_at+INTERVAL '1 day' GROUP BY d.bucket_at ORDER BY d.bucket_at`, tenantID, window.From, window.To)
		if err != nil {
			return result, err
		}
		for rows.Next() {
			var point analytics.WorkbenchModelUsagePoint
			var tokens sql.NullInt64
			if err = rows.Scan(&point.At, &tokens); err != nil {
				rows.Close()
				return result, err
			}
			if tokens.Valid {
				point.TotalTokens = &tokens.Int64
			}
			usage.Trend = append(usage.Trend, point)
		}
		rows.Close()
		result.ModelUsage = usage
	}

	rows, err = s.db.Query(ctx, `SELECT d.id,t.id,t.project_id,t.name,pv.name,d.status,d.updated_at FROM workforce_delivery_requests d JOIN workforce_tasks t ON t.tenant_id=d.tenant_id AND t.id=d.task_id JOIN workforce_project_versions pv ON pv.tenant_id=t.tenant_id AND pv.id=t.project_version_id WHERE d.tenant_id=$1 ORDER BY d.updated_at DESC,d.id DESC LIMIT 2`, tenantID)
	if err != nil {
		return result, err
	}
	for rows.Next() {
		var item analytics.WorkbenchDelivery
		if err = rows.Scan(&item.DeliveryID, &item.TaskID, &item.ProjectID, &item.TaskName, &item.ProjectName, &item.Status, &item.UpdatedAt); err != nil {
			rows.Close()
			return result, err
		}
		result.RecentDeliveries = append(result.RecentDeliveries, item)
	}
	rows.Close()
	return result, rows.Err()
}

const attentionSQL = `
CASE
 WHEN EXISTS (SELECT 1 FROM workforce_agent_interactions i WHERE i.tenant_id=t.tenant_id AND i.task_id=t.id AND i.kind='QUESTION' AND i.is_blocking AND i.state='OPEN' AND i.expires_at>CURRENT_TIMESTAMP) THEN 'WAITING_USER_INPUT'
 WHEN (SELECT d.status FROM workforce_delivery_requests d WHERE d.tenant_id=t.tenant_id AND d.task_id=t.id ORDER BY d.created_at DESC,d.id DESC LIMIT 1)='WAITING_APPROVAL' THEN 'WAITING_DELIVERY_DECISION'
 WHEN (SELECT d.status FROM workforce_delivery_requests d WHERE d.tenant_id=t.tenant_id AND d.task_id=t.id ORDER BY d.created_at DESC,d.id DESC LIMIT 1) IN ('FAILED','MANUAL_REVIEW') THEN 'DELIVERY_FAILED'
 WHEN t.status='MANUAL_REVIEW' THEN 'MANUAL_REVIEW'
 WHEN COALESCE((SELECT NULLIF(n.wait_reason_code,'') FROM workforce_execution_nodes n WHERE n.task_id=t.id ORDER BY n.updated_at DESC,n.id DESC LIMIT 1),'')<>'' THEN COALESCE((SELECT NULLIF(n.wait_reason_code,'') FROM workforce_execution_nodes n WHERE n.task_id=t.id ORDER BY n.updated_at DESC,n.id DESC LIMIT 1),'WAITING_AGENT_RUNTIME')
 WHEN t.status='REVIEWING' THEN 'WAITING_REVIEW'
 WHEN t.status='RETRY_WAIT' THEN 'RETRY_WAIT'
 ELSE '' END`

func (s *Store) ProjectList(ctx context.Context, tenantID int64, window analytics.Window) (analytics.ProjectListAnalytics, error) {
	result := analytics.ProjectListAnalytics{From: window.From, To: window.To, TaskTrend: []analytics.TaskTrendPoint{}, ProjectHealth: []analytics.ProjectHealth{}}
	err := s.db.QueryRow(ctx, `
WITH attention AS (
 SELECT DISTINCT t.project_id FROM workforce_tasks t
 WHERE t.tenant_id=$1 AND t.project_id IS NOT NULL AND (`+attentionSQL+`<>'' OR t.status IN ('FAILED','TIMED_OUT'))
)
SELECT
 (SELECT count(*) FROM workforce_projects p WHERE p.tenant_id=$1 AND p.status='ACTIVE'),
 count(*) FILTER (WHERE t.created_at>=$2 AND t.created_at<$3),
 count(*) FILTER (WHERE t.status='SUCCEEDED' AND t.finished_at>=$2 AND t.finished_at<$3),
 (SELECT count(*) FROM attention)
FROM workforce_tasks t WHERE t.tenant_id=$1`, tenantID, window.From, window.To).Scan(
		&result.Summary.ActiveProjectCount, &result.Summary.CreatedTaskCount, &result.Summary.SucceededTaskCount, &result.Summary.AttentionProjectCount)
	if err != nil {
		return result, err
	}
	result.TaskTrend, err = s.taskTrend(ctx, tenantID, 0, window)
	if err != nil {
		return result, err
	}
	rows, err := s.db.Query(ctx, `
WITH active_tasks AS (
 SELECT t.*, `+attentionSQL+` AS attention_reason
 FROM workforce_tasks t
 JOIN workforce_projects p ON p.id=t.project_id AND p.tenant_id=t.tenant_id AND p.status='ACTIVE'
 WHERE t.tenant_id=$1 AND (t.created_at>=$2 OR t.status NOT IN ('SUCCEEDED','FAILED','TIMED_OUT','CANCELLED'))
), grouped AS (
 SELECT p.id,pv.name,
  count(*) FILTER (WHERE t.status='SUCCEEDED' AND t.finished_at>=$2 AND t.finished_at<$3) succeeded,
  count(*) FILTER (WHERE t.status IN ('FAILED','TIMED_OUT') AND t.finished_at>=$2 AND t.finished_at<$3) failed,
  count(*) FILTER (WHERE t.status NOT IN ('SUCCEEDED','FAILED','TIMED_OUT','CANCELLED')) active,
  count(*) FILTER (WHERE t.attention_reason<>'' OR t.status IN ('FAILED','TIMED_OUT')) attention,
  max(COALESCE(t.finished_at,t.started_at,t.created_at)) latest_activity
 FROM workforce_projects p JOIN workforce_project_versions pv ON pv.id=p.current_version_id AND pv.tenant_id=p.tenant_id
 JOIN active_tasks t ON t.project_id=p.id
 WHERE p.tenant_id=$1 AND p.status='ACTIVE'
 GROUP BY p.id,pv.name
)
SELECT id,name,succeeded,failed,active,attention,latest_activity FROM grouped
ORDER BY attention DESC,failed DESC,latest_activity DESC,id LIMIT 10`, tenantID, window.From, window.To)
	if err != nil {
		return result, err
	}
	defer rows.Close()
	for rows.Next() {
		var item analytics.ProjectHealth
		if err = rows.Scan(&item.ProjectID, &item.Name, &item.Succeeded, &item.Failed, &item.Active, &item.Attention, &item.LatestActivity); err != nil {
			return result, err
		}
		result.ProjectHealth = append(result.ProjectHealth, item)
	}
	return result, rows.Err()
}

func (s *Store) Project(ctx context.Context, tenantID, projectID int64, window analytics.Window) (analytics.ProjectAnalytics, error) {
	result := analytics.ProjectAnalytics{From: window.From, To: window.To, TaskTrend: []analytics.TaskTrendPoint{}, StageHealth: []analytics.StageHealth{}, DurationTrend: []analytics.DurationPoint{}, WaitingReasons: []analytics.Count{}, ModelUsageTrend: []analytics.ModelUsagePoint{}}
	var exists bool
	if err := s.db.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM workforce_projects WHERE tenant_id=$1 AND id=$2)`, tenantID, projectID).Scan(&exists); err != nil {
		return result, err
	}
	if !exists {
		return result, analytics.ErrProjectNotFound
	}
	var successDenominator int64
	err := s.db.QueryRow(ctx, `
WITH latest_delivery AS (
 SELECT DISTINCT ON (task_id) task_id,status FROM workforce_delivery_requests WHERE tenant_id=$1 AND project_id=$2 ORDER BY task_id,created_at DESC,id DESC
), scoped AS (
 SELECT t.*, `+attentionSQL+` AS attention_reason FROM workforce_tasks t WHERE t.tenant_id=$1 AND t.project_id=$2
)
SELECT count(*) FILTER (WHERE created_at>=$3 AND created_at<$4),
 count(*) FILTER (WHERE status='SUCCEEDED' AND finished_at>=$3 AND finished_at<$4),
 count(*) FILTER (WHERE status IN ('SUCCEEDED','FAILED','TIMED_OUT') AND finished_at>=$3 AND finished_at<$4),
 (SELECT count(*) FROM latest_delivery WHERE status='SUCCEEDED'),
 count(*) FILTER (WHERE attention_reason<>'' OR status IN ('FAILED','TIMED_OUT'))
FROM scoped`, tenantID, projectID, window.From, window.To).Scan(&result.Summary.CreatedTaskCount, new(int64), &successDenominator, &result.Summary.SuccessfulDeliveryCount, &result.Summary.AttentionTaskCount)
	if err != nil {
		return result, err
	}
	var successCount int64
	if err = s.db.QueryRow(ctx, `SELECT count(*) FROM workforce_tasks WHERE tenant_id=$1 AND project_id=$2 AND status='SUCCEEDED' AND finished_at>=$3 AND finished_at<$4`, tenantID, projectID, window.From, window.To).Scan(&successCount); err != nil {
		return result, err
	}
	if successDenominator > 0 {
		value := float64(successCount) / float64(successDenominator)
		result.Summary.SuccessRate = &value
	}
	result.TaskTrend, err = s.taskTrend(ctx, tenantID, projectID, window)
	if err != nil {
		return result, err
	}
	result.DurationTrend, err = s.durationTrend(ctx, tenantID, projectID, window)
	if err != nil {
		return result, err
	}
	result.WaitingReasons, err = s.waitingReasons(ctx, tenantID, projectID)
	if err != nil {
		return result, err
	}
	if err = s.projectStages(ctx, tenantID, projectID, window, &result); err != nil {
		return result, err
	}
	if err = s.modelUsage(ctx, tenantID, projectID, window, &result); err != nil {
		return result, err
	}
	return result, nil
}

func (s *Store) Tasks(ctx context.Context, tenantID int64, window analytics.Window) (analytics.TaskAnalytics, error) {
	result := analytics.TaskAnalytics{From: window.From, To: window.To, ActivityTrend: []analytics.ActivityPoint{}, StatusCounts: []analytics.Count{}, WaitingAgeBuckets: []analytics.Count{}, WaitingReasons: []analytics.Count{}, DurationTrend: []analytics.DurationPoint{}}
	rows, err := s.db.Query(ctx, `WITH scoped AS (SELECT t.*,`+attentionSQL+` reason FROM workforce_tasks t WHERE t.tenant_id=$1)
SELECT reason,count(*) FROM scoped WHERE reason<>'' GROUP BY reason ORDER BY count(*) DESC,reason`, tenantID)
	if err != nil {
		return result, err
	}
	for rows.Next() {
		var item analytics.Count
		if err = rows.Scan(&item.Code, &item.Count); err != nil {
			rows.Close()
			return result, err
		}
		result.WaitingReasons = append(result.WaitingReasons, item)
	}
	rows.Close()
	for _, group := range []struct{ code, predicate string }{
		{"PENDING", "status IN ('CREATED','QUEUED','PREPARING')"},
		{"RUNNING", "status IN ('RUNNING','VALIDATING','REVIEWING','PACKAGING','UPLOADING','CANCELLING')"},
		{"WAITING", "status IN ('INTERACTION_SUSPENDED','RETRY_WAIT','MANUAL_REVIEW')"},
		{"SUCCEEDED", "status='SUCCEEDED'"}, {"FAILED", "status IN ('FAILED','TIMED_OUT')"}, {"CANCELLED", "status='CANCELLED'"},
	} {
		var count int64
		if err = s.db.QueryRow(ctx, `SELECT count(*) FROM workforce_tasks WHERE tenant_id=$1 AND `+group.predicate, tenantID).Scan(&count); err != nil {
			return result, err
		}
		result.StatusCounts = append(result.StatusCounts, analytics.Count{Code: group.code, Count: count})
	}
	countBy := func(predicate string) (int64, error) {
		var n int64
		e := s.db.QueryRow(ctx, `SELECT count(*) FROM workforce_tasks WHERE tenant_id=$1 AND `+predicate, tenantID).Scan(&n)
		return n, e
	}
	if result.Summary.Attention, err = countBy(`(status IN ('INTERACTION_SUSPENDED','MANUAL_REVIEW','FAILED','TIMED_OUT'))`); err != nil {
		return result, err
	}
	if result.Summary.Running, err = countBy(`status IN ('RUNNING','VALIDATING','REVIEWING','PACKAGING','UPLOADING','CANCELLING')`); err != nil {
		return result, err
	}
	if result.Summary.Waiting, err = countBy(`status IN ('CREATED','QUEUED','PREPARING','INTERACTION_SUSPENDED','RETRY_WAIT','MANUAL_REVIEW')`); err != nil {
		return result, err
	}
	if err = s.db.QueryRow(ctx, `SELECT count(*) FROM workforce_tasks WHERE tenant_id=$1 AND status='SUCCEEDED' AND finished_at>=$2 AND finished_at<$3`, tenantID, window.From, window.To).Scan(&result.Summary.Succeeded); err != nil {
		return result, err
	}
	result.DurationTrend, err = s.durationTrend(ctx, tenantID, 0, window)
	if err != nil {
		return result, err
	}
	if err = s.taskActivity(ctx, tenantID, window, &result); err != nil {
		return result, err
	}
	if err = s.waitingAges(ctx, tenantID, &result); err != nil {
		return result, err
	}
	return result, nil
}

func (s *Store) taskTrend(ctx context.Context, tenantID, projectID int64, window analytics.Window) ([]analytics.TaskTrendPoint, error) {
	projectClause := ""
	args := []any{tenantID, window.From, window.To}
	if projectID > 0 {
		projectClause = " AND project_id=$4"
		args = append(args, projectID)
	}
	rows, err := s.db.Query(ctx, `WITH facts AS (
 SELECT date_trunc('day',created_at) AS bucket_at,'created' AS kind FROM workforce_tasks WHERE tenant_id=$1 AND created_at>=$2 AND created_at<$3`+projectClause+`
 UNION ALL SELECT date_trunc('day',finished_at),'succeeded' FROM workforce_tasks WHERE tenant_id=$1 AND status='SUCCEEDED' AND finished_at>=$2 AND finished_at<$3`+projectClause+`
 UNION ALL SELECT date_trunc('day',finished_at),'failed' FROM workforce_tasks WHERE tenant_id=$1 AND status IN ('FAILED','TIMED_OUT') AND finished_at>=$2 AND finished_at<$3`+projectClause+`
 UNION ALL SELECT date_trunc('day',finished_at),'cancelled' FROM workforce_tasks WHERE tenant_id=$1 AND status='CANCELLED' AND finished_at>=$2 AND finished_at<$3`+projectClause+`
), bounds AS (SELECT min(bucket_at) AS first_at,max(bucket_at) AS last_at FROM facts), buckets AS (SELECT generate_series(first_at,last_at,interval '1 day') AS bucket_at FROM bounds WHERE first_at IS NOT NULL)
SELECT b.bucket_at,count(*) FILTER(WHERE f.kind='created'),count(*) FILTER(WHERE f.kind='succeeded'),count(*) FILTER(WHERE f.kind='failed'),count(*) FILTER(WHERE f.kind='cancelled') FROM buckets b LEFT JOIN facts f ON f.bucket_at=b.bucket_at GROUP BY b.bucket_at ORDER BY b.bucket_at`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := []analytics.TaskTrendPoint{}
	for rows.Next() {
		var x analytics.TaskTrendPoint
		if err = rows.Scan(&x.At, &x.Created, &x.Succeeded, &x.Failed, &x.Cancelled); err != nil {
			return nil, err
		}
		items = append(items, x)
	}
	return items, rows.Err()
}

func (s *Store) durationTrend(ctx context.Context, tenantID, projectID int64, window analytics.Window) ([]analytics.DurationPoint, error) {
	clause := ""
	args := []any{tenantID, window.From, window.To}
	if projectID > 0 {
		clause = " AND project_id=$4"
		args = append(args, projectID)
	}
	rows, err := s.db.Query(ctx, `SELECT date_trunc('day',finished_at),count(*),percentile_cont(.5) WITHIN GROUP(ORDER BY extract(epoch FROM finished_at-started_at)),percentile_cont(.9) WITHIN GROUP(ORDER BY extract(epoch FROM finished_at-started_at)) FROM workforce_tasks WHERE tenant_id=$1 AND status='SUCCEEDED' AND started_at IS NOT NULL AND finished_at>=$2 AND finished_at<$3`+clause+` GROUP BY 1 ORDER BY 1`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := []analytics.DurationPoint{}
	for rows.Next() {
		var x analytics.DurationPoint
		var p50, p90 sql.NullFloat64
		if err = rows.Scan(&x.At, &x.SampleCount, &p50, &p90); err != nil {
			return nil, err
		}
		if p50.Valid {
			x.P50Seconds = &p50.Float64
		}
		if p90.Valid {
			x.P90Seconds = &p90.Float64
		}
		items = append(items, x)
	}
	return items, rows.Err()
}

func (s *Store) waitingReasons(ctx context.Context, tenantID, projectID int64) ([]analytics.Count, error) {
	rows, err := s.db.Query(ctx, `WITH scoped AS (SELECT t.*,`+attentionSQL+` reason FROM workforce_tasks t WHERE t.tenant_id=$1 AND t.project_id=$2 AND t.status NOT IN ('SUCCEEDED','FAILED','TIMED_OUT','CANCELLED')) SELECT reason,count(*) FROM scoped WHERE reason<>'' GROUP BY reason ORDER BY count(*) DESC,reason`, tenantID, projectID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := []analytics.Count{}
	for rows.Next() {
		var x analytics.Count
		if err = rows.Scan(&x.Code, &x.Count); err != nil {
			return nil, err
		}
		items = append(items, x)
	}
	return items, rows.Err()
}

func (s *Store) projectStages(ctx context.Context, tenantID, projectID int64, window analytics.Window, result *analytics.ProjectAnalytics) error {
	query := `WITH tasks AS (SELECT * FROM workforce_tasks WHERE tenant_id=$1 AND project_id=$2 AND created_at>=$3 AND created_at<$4),
 latest_check AS (SELECT DISTINCT ON(task_id) task_id,status FROM workforce_completion_check_results WHERE tenant_id=$1 AND phase='CURRENT' ORDER BY task_id,created_at DESC,id DESC),
 latest_review AS (SELECT DISTINCT ON(task_id) id,task_id,status,automatic_revision_number FROM workforce_review_runs WHERE tenant_id=$1 ORDER BY task_id,id DESC),
 latest_delivery AS (SELECT DISTINCT ON(task_id) task_id,status FROM workforce_delivery_requests WHERE tenant_id=$1 AND project_id=$2 ORDER BY task_id,created_at DESC,id DESC)
SELECT
 count(*),count(*) FILTER(WHERE EXISTS(SELECT 1 FROM workforce_finalization_runs f WHERE f.task_id=tasks.id AND f.status='FROZEN')),count(*) FILTER(WHERE EXISTS(SELECT 1 FROM workforce_finalization_runs f WHERE f.task_id=tasks.id AND f.status IN ('FAILED','TIMED_OUT'))),
 count(*) FILTER(WHERE EXISTS(SELECT 1 FROM workforce_task_turns tt WHERE tt.id=tasks.current_task_turn_id AND COALESCE(tt.completion_check_json->>'mode','NONE')<>'NONE')),count(*) FILTER(WHERE lc.status IN ('PASSED','NO_TESTS','NOT_CONFIGURED')),count(*) FILTER(WHERE lc.status IN ('FAILED','UNAVAILABLE','TIMED_OUT')),
 count(*) FILTER(WHERE tasks.review_enabled),count(*) FILTER(WHERE tasks.review_enabled AND (lr.status='PASSED' OR EXISTS(SELECT 1 FROM workforce_human_review_decisions hd WHERE hd.review_run_id=lr.id AND hd.status='APPLIED' AND hd.action='ACCEPT_RISK'))),count(*) FILTER(WHERE tasks.review_enabled AND lr.status='REJECTED'),
 count(ld.task_id),count(*) FILTER(WHERE ld.status='SUCCEEDED'),count(*) FILTER(WHERE ld.status IN ('FAILED','MANUAL_REVIEW')),
 count(*) FILTER(WHERE NOT tasks.review_enabled),count(*) FILTER(WHERE tasks.review_enabled AND lr.status='PASSED' AND lr.automatic_revision_number=0),count(*) FILTER(WHERE tasks.review_enabled AND lr.status='PASSED' AND lr.automatic_revision_number>0),count(*) FILTER(WHERE EXISTS(SELECT 1 FROM workforce_human_review_decisions hd WHERE hd.review_run_id=lr.id AND hd.status='APPLIED' AND hd.action='ACCEPT_RISK')),count(*) FILTER(WHERE tasks.review_enabled AND lr.status='REJECTED'),count(*) FILTER(WHERE tasks.review_enabled AND COALESCE(lr.status,'PENDING') NOT IN ('PASSED','REJECTED') AND NOT EXISTS(SELECT 1 FROM workforce_human_review_decisions hd WHERE hd.review_run_id=lr.id AND hd.status='APPLIED' AND hd.action='ACCEPT_RISK'))
FROM tasks LEFT JOIN latest_check lc ON lc.task_id=tasks.id LEFT JOIN latest_review lr ON lr.task_id=tasks.id LEFT JOIN latest_delivery ld ON ld.task_id=tasks.id`
	var freeze, check, review, delivery analytics.StageHealth
	freeze.Code = "RESULT_FROZEN"
	check.Code = "COMPLETION_CHECK"
	review.Code = "INDEPENDENT_REVIEW"
	delivery.Code = "GIT_DELIVERY"
	err := s.db.QueryRow(ctx, query, tenantID, projectID, window.From, window.To).Scan(&freeze.EligibleCount, &freeze.ReachedCount, &freeze.FailedCount, &check.EligibleCount, &check.ReachedCount, &check.FailedCount, &review.EligibleCount, &review.ReachedCount, &review.FailedCount, &delivery.EligibleCount, &delivery.ReachedCount, &delivery.FailedCount, &result.ReviewOutcomes.Disabled, &result.ReviewOutcomes.DirectPassed, &result.ReviewOutcomes.PassedAfterRevision, &result.ReviewOutcomes.AcceptedRisk, &result.ReviewOutcomes.Rejected, &result.ReviewOutcomes.Processing)
	result.StageHealth = []analytics.StageHealth{freeze, check, review, delivery}
	return err
}

func (s *Store) modelUsage(ctx context.Context, tenantID, projectID int64, window analytics.Window, result *analytics.ProjectAnalytics) error {
	rows, err := s.db.Query(ctx, `SELECT date_trunc('day',m.started_at),count(*),sum(m.input_tokens),sum(m.cached_input_tokens),sum(m.cache_creation_input_tokens),sum(m.output_tokens),sum(m.reasoning_output_tokens),sum(m.total_tokens) FROM workforce_model_invocations m JOIN workforce_tasks t ON t.id=m.task_id AND t.tenant_id=m.tenant_id WHERE m.tenant_id=$1 AND t.project_id=$2 AND m.started_at>=$3 AND m.started_at<$4 GROUP BY 1 ORDER BY 1`, tenantID, projectID, window.From, window.To)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var x analytics.ModelUsagePoint
		var values [6]sql.NullInt64
		if err = rows.Scan(&x.At, &x.InvocationCount, &values[0], &values[1], &values[2], &values[3], &values[4], &values[5]); err != nil {
			return err
		}
		targets := []**int64{&x.InputTokens, &x.CachedInputTokens, &x.CacheCreationInputTokens, &x.OutputTokens, &x.ReasoningOutputTokens, &x.TotalTokens}
		for i, v := range values {
			if v.Valid {
				n := v.Int64
				*targets[i] = &n
			}
		}
		if x.InputTokens != nil && *x.InputTokens > 0 && x.CachedInputTokens != nil {
			rate := float64(*x.CachedInputTokens) / float64(*x.InputTokens)
			x.CacheHitRate = &rate
		}
		result.ModelUsageTrend = append(result.ModelUsageTrend, x)
	}
	return rows.Err()
}

func (s *Store) taskActivity(ctx context.Context, tenantID int64, window analytics.Window, result *analytics.TaskAnalytics) error {
	rows, err := s.db.Query(ctx, `WITH facts AS (
 SELECT date_trunc('day',created_at) AS bucket_at FROM workforce_tasks WHERE tenant_id=$1 AND created_at>=$2 AND created_at<$3
 UNION ALL SELECT date_trunc('day',finished_at) FROM workforce_tasks WHERE tenant_id=$1 AND finished_at>=$2 AND finished_at<$3
), bounds AS (SELECT min(bucket_at) AS first_at,max(bucket_at) AS last_at FROM facts), buckets AS (SELECT generate_series(first_at,last_at,interval '1 day') AS bucket_at FROM bounds WHERE first_at IS NOT NULL), baseline AS (SELECT count(*) AS n FROM workforce_tasks,bounds WHERE tenant_id=$1 AND created_at<first_at AND (finished_at IS NULL OR finished_at>=first_at)), daily AS (SELECT b.bucket_at,count(t.id) FILTER(WHERE date_trunc('day',t.created_at)=b.bucket_at) AS created,count(t.id) FILTER(WHERE t.finished_at IS NOT NULL AND date_trunc('day',t.finished_at)=b.bucket_at) AS terminal,count(t.id) FILTER(WHERE t.status='SUCCEEDED' AND date_trunc('day',t.finished_at)=b.bucket_at) AS succeeded,count(t.id) FILTER(WHERE t.status IN ('FAILED','TIMED_OUT') AND date_trunc('day',t.finished_at)=b.bucket_at) AS failed FROM buckets b LEFT JOIN workforce_tasks t ON t.tenant_id=$1 AND ((t.created_at>=b.bucket_at AND t.created_at<b.bucket_at+interval '1 day') OR (t.finished_at>=b.bucket_at AND t.finished_at<b.bucket_at+interval '1 day')) GROUP BY b.bucket_at) SELECT bucket_at,created,terminal,succeeded,failed,(SELECT n FROM baseline)+sum(created-terminal) OVER(ORDER BY bucket_at) AS backlog FROM daily ORDER BY bucket_at`, tenantID, window.From, window.To)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var x analytics.ActivityPoint
		if err = rows.Scan(&x.At, &x.Created, &x.Terminal, &x.Succeeded, &x.Failed, &x.Backlog); err != nil {
			return err
		}
		result.ActivityTrend = append(result.ActivityTrend, x)
	}
	return rows.Err()
}

func (s *Store) waitingAges(ctx context.Context, tenantID int64, result *analytics.TaskAnalytics) error {
	rows, err := s.db.Query(ctx, `WITH waiting AS (SELECT t.id,COALESCE((SELECT min(i.created_at) FROM workforce_agent_interactions i WHERE i.tenant_id=t.tenant_id AND i.task_id=t.id AND i.kind='QUESTION' AND i.is_blocking AND i.state='OPEN'),(SELECT d.updated_at FROM workforce_delivery_requests d WHERE d.tenant_id=t.tenant_id AND d.task_id=t.id AND d.status='WAITING_APPROVAL' ORDER BY d.created_at DESC,d.id DESC LIMIT 1),(SELECT n.updated_at FROM workforce_execution_nodes n WHERE n.task_id=t.id AND n.wait_reason_code<>'' ORDER BY n.updated_at DESC,n.id DESC LIMIT 1),(SELECT r.updated_at FROM workforce_review_runs r WHERE r.tenant_id=t.tenant_id AND r.task_id=t.id AND r.status NOT IN ('PASSED','REJECTED','CANCELLED') ORDER BY r.id DESC LIMIT 1),t.queued_at) since FROM workforce_tasks t WHERE t.tenant_id=$1 AND t.status NOT IN ('SUCCEEDED','FAILED','TIMED_OUT','CANCELLED')) SELECT CASE WHEN CURRENT_TIMESTAMP-since<interval '10 minutes' THEN 'LT_10M' WHEN CURRENT_TIMESTAMP-since<interval '30 minutes' THEN 'M10_30' WHEN CURRENT_TIMESTAMP-since<interval '2 hours' THEN 'M30_2H' ELSE 'GT_2H' END bucket,count(*) FROM waiting WHERE since IS NOT NULL GROUP BY bucket`, tenantID)
	if err != nil {
		return err
	}
	defer rows.Close()
	counts := map[string]int64{}
	for rows.Next() {
		var code string
		var count int64
		if err = rows.Scan(&code, &count); err != nil {
			return err
		}
		counts[code] = count
	}
	for _, code := range []string{"LT_10M", "M10_30", "M30_2H", "GT_2H"} {
		result.WaitingAgeBuckets = append(result.WaitingAgeBuckets, analytics.Count{Code: code, Count: counts[code]})
	}
	return rows.Err()
}

var _ analytics.Repository = (*Store)(nil)
