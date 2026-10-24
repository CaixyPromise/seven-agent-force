package infrastructure_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	continuation "github.com/CaixyPromise/seven-agent/force/internal/app/workforce/continuation/facade"
	app "github.com/CaixyPromise/seven-agent/force/internal/app/workforce/interaction/application"
	d "github.com/CaixyPromise/seven-agent/force/internal/app/workforce/interaction/domain"
	f "github.com/CaixyPromise/seven-agent/force/internal/app/workforce/interaction/facade"
	infra "github.com/CaixyPromise/seven-agent/force/internal/app/workforce/interaction/infrastructure"
	"github.com/CaixyPromise/seven-agent/force/internal/app/workforce/shared/eventlog"
	"github.com/bwmarrin/snowflake"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"os"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"
)

type ids struct{ node *snowflake.Node }

func (i ids) NextID() int64 { return i.node.Generate().Int64() }

// This fixture is a SQL-backed authority port test double, NOT the interaction
// backend. Production authority belongs to dispatch; tests exercise application
// transaction ownership against the real PostgreSQL repository and eventlog.
type authority struct {
	noCapabilities     bool
	nativeClosed       bool
	deliveryActions    bool
	closedAttemptState string
}

func (a authority) capabilities() []string {
	if a.noCapabilities {
		return nil
	}
	return []string{"interaction.request", "interaction.respond", "interaction.received", "interaction.closed", "user.message", "message.received", "activity"}
}

func (a authority) AuthorizeUser(ctx context.Context, tx pgx.Tx, u f.User, action string) (f.Target, error) {
	var target f.Target
	var cancelled bool
	e := tx.QueryRow(ctx, `SELECT tenant,task,turn,attempt,session,cancelled FROM interaction_test_authority WHERE tenant=$1 AND task=$2 FOR UPDATE`, u.TenantID, u.TaskID).Scan(&target.TenantID, &target.TaskID, &target.TaskTurnID, &target.AttemptID, &target.AdapterSessionGeneration, &cancelled)
	if e != nil || u.UserID != 7 {
		return target, d.ErrNotFound
	}
	target.PrincipalID = 70
	target.ClosedAttemptState = a.closedAttemptState
	target.Capabilities = a.capabilities()
	if u.CanContinue && !cancelled {
		target.AllowedActions = []string{f.Answer, f.SendMessage}
	}
	if u.CanContinue && a.deliveryActions {
		// Like production, delivery presentation does not require an active Agent.
		target.AllowedActions = append(target.AllowedActions, f.DecideDelivery, f.RestartDelivery)
	}
	if action != f.Read && (!u.CanContinue || cancelled) {
		return target, d.ErrNotAllowed
	}
	return target, nil
}
func (a authority) Validate(ctx context.Context, tx pgx.Tx, m f.Machine) (f.Target, error) {
	var target f.Target
	var cancelled bool
	var generation, owner int64
	e := tx.QueryRow(ctx, `SELECT tenant,task,turn,attempt,session,cancelled,generation,owner FROM interaction_test_authority WHERE attempt=$1 FOR UPDATE`, m.AttemptID).Scan(&target.TenantID, &target.TaskID, &target.TaskTurnID, &target.AttemptID, &target.AdapterSessionGeneration, &cancelled, &generation, &owner)
	if e != nil || cancelled || m.Generation != generation || m.OwnerEpoch != owner || m.AdapterSessionGeneration != target.AdapterSessionGeneration || m.WorkerID != 8 || m.InstanceID != "instance" || m.HostBootID != "boot" || m.ExecutionLease != "lease" {
		return target, d.ErrFenced
	}
	target.PrincipalID = 80
	target.AcceptsNativeInput = !a.nativeClosed
	target.Capabilities = a.capabilities()
	return target, nil
}
func (authority) AuthorizeSummary(ctx context.Context, tx pgx.Tx, tenant, user int64, tasks []int64) (f.SummaryScope, error) {
	out := f.SummaryScope{PrincipalID: 70, TaskIDs: []int64{}, ActiveTaskIDs: []int64{}}
	if user != 7 {
		return out, d.ErrNotFound
	}
	rows, e := tx.Query(ctx, `SELECT task,cancelled FROM interaction_test_authority WHERE tenant=$1 AND task=ANY($2::bigint[])`, tenant, tasks)
	if e != nil {
		return out, e
	}
	defer rows.Close()
	for rows.Next() {
		var id int64
		var cancelled bool
		if e = rows.Scan(&id, &cancelled); e != nil {
			return out, e
		}
		out.TaskIDs = append(out.TaskIDs, id)
		if !cancelled {
			out.ActiveTaskIDs = append(out.ActiveTaskIDs, id)
		}
	}
	return out, rows.Err()
}

type events struct{ fail bool }

type attentionEvents struct{ occurrences int }

func (e *attentionEvents) RecordInteractionEvent(ctx context.Context, tx pgx.Tx, v f.Event) (int64, error) {
	if v.AttentionRequired {
		e.occurrences++
	}
	return (events{}).RecordInteractionEvent(ctx, tx, v)
}

func (e events) RecordInteractionEvent(ctx context.Context, tx pgx.Tx, v f.Event) (int64, error) {
	if e.fail {
		return 0, errors.New("private provider body secret")
	}
	return eventlog.AppendWithSequence(ctx, tx, v.TaskID, v.AttemptID, "conversation.updated", map[string]string{"itemId": strconv.FormatInt(v.ItemID, 10), "version": strconv.FormatInt(v.Version, 10), "kind": v.Type})
}

type resume struct{}

func (resume) FindInteractionResumeTx(ctx context.Context, tx pgx.Tx, r continuation.InteractionResumeLookup) (*continuation.InteractionResumeTarget, error) {
	var mode string
	e := tx.QueryRow(ctx, `SELECT mode FROM interaction_test_authority WHERE task=$1`, r.TaskID).Scan(&mode)
	if e != nil {
		return nil, e
	}
	if mode == "CAPTURING" {
		return nil, continuation.ErrInteractionSuspendInProgress
	}
	if mode == "COLD" {
		return &continuation.InteractionResumeTarget{SourceTaskTurnID: 30, CheckpointID: 300}, nil
	}
	return nil, nil
}
func (resume) ResumeInteractionTx(ctx context.Context, tx pgx.Tx, r continuation.InteractionResumeRequest) (continuation.InteractionResumeResult, error) {
	if !strings.Contains(string(r.InteractionResponse), `"interactionKey":"cold"`) || !strings.Contains(string(r.InteractionResponse), `"responseId":"`+strconv.FormatInt(r.AnswerID, 10)+`"`) {
		return continuation.InteractionResumeResult{}, errors.New("missing private native answer binding")
	}
	var count int
	e := tx.QueryRow(ctx, `SELECT count(*) FROM workforce_agent_interaction_responses WHERE id=$1 AND interaction_id=$2`, r.AnswerID, r.InteractionID).Scan(&count)
	if e != nil || count != 1 {
		return continuation.InteractionResumeResult{}, errors.New("answer not saved in same tx")
	}
	_, e = tx.Exec(ctx, `UPDATE interaction_test_authority SET resumes=resumes+1 WHERE task=$1`, r.TaskID)
	return continuation.InteractionResumeResult{TaskTurnID: 31, SourceCheckpointID: 300}, e
}

type fixture struct {
	ctx     context.Context
	pool    *pgxpool.Pool
	service *app.Service
	ids     ids
}

func (fx fixture) exec(t *testing.T, sql string, args ...any) {
	t.Helper()
	if _, e := fx.pool.Exec(fx.ctx, sql, args...); e != nil {
		t.Fatalf("fixture SQL failed: %v", e)
	}
}
func (fx fixture) count(t *testing.T, sql string, args ...any) int {
	t.Helper()
	var n int
	if e := fx.pool.QueryRow(fx.ctx, sql, args...).Scan(&n); e != nil {
		t.Fatal(e)
	}
	return n
}
func (fx fixture) reset(t *testing.T) {
	fx.exec(t, `TRUNCATE workforce_agent_interaction_responses,workforce_agent_interactions,workforce_task_conversation_items,workforce_task_conversation_read_states,workforce_task_events,workforce_continuation_checkpoints,workforce_result_evaluation_cycles,workforce_task_turns,workforce_tasks,interaction_test_authority`)
	fx.exec(t, `INSERT INTO interaction_test_authority VALUES(10,20,30,40,1,false,1,1,'SHORT',0)`)
	fx.exec(t, `INSERT INTO workforce_tasks VALUES(20,10,30,'RUNNING'); INSERT INTO workforce_task_turns VALUES(30,20,10,'RUNNING')`)
}
func question(key string) f.PublishRequest {
	return f.PublishRequest{LocalRequestKey: key, RoundKey: "round", IsBlocking: true, Questions: []d.Question{{Key: "q", Title: "Choose", Description: "", Kind: "SINGLE_CHOICE", Required: true, Options: []d.Option{{Key: "a", Label: "A"}, {Key: "b", Label: "B"}}}}}
}
func answer(key string) f.ResponseRequest {
	return f.ResponseRequest{IdempotencyKey: key, ExpectedVersion: 1, Answers: []d.Answer{{QuestionKey: "q", SelectedOptionKeys: []string{"a"}}}}
}
func machine() f.Machine {
	return f.Machine{AttemptID: 40, WorkerID: 8, InstanceID: "instance", HostBootID: "boot", Generation: 1, OwnerEpoch: 1, AdapterSessionGeneration: 1, ExecutionLease: "lease"}
}
func user() f.User { return f.User{TenantID: 10, TaskID: 20, UserID: 7, CanContinue: true} }
func (fx fixture) publish(t *testing.T, key string) f.Receipt {
	t.Helper()
	r, e := fx.service.Publish(fx.ctx, machine(), question(key))
	if e != nil {
		t.Fatal(e)
	}
	return r
}

func TestPostgreSQLInteraction(t *testing.T) {
	dsn := os.Getenv("INTERACTION_TEST_POSTGRES_DSN")
	if dsn == "" {
		t.Skip("NOT RUN: set INTERACTION_TEST_POSTGRES_DSN to a disposable migrated PostgreSQL database; one explicit integration gate")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	admin, e := pgxpool.New(ctx, dsn)
	if e != nil {
		t.Fatal("PostgreSQL configuration failed")
	}
	defer admin.Close()
	if e = admin.Ping(ctx); e != nil {
		t.Fatal("PostgreSQL connection failed")
	}
	schema := fmt.Sprintf("interaction_test_%d", time.Now().UnixNano())
	quoted := pgx.Identifier{schema}.Sanitize()
	if _, e = admin.Exec(ctx, "CREATE SCHEMA "+quoted); e != nil {
		t.Fatal(e)
	}
	defer func() {
		cleanup, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if _, e := admin.Exec(cleanup, "DROP SCHEMA "+quoted+" CASCADE"); e != nil {
			t.Error("test schema cleanup failed")
		}
	}()
	cfg, e := pgxpool.ParseConfig(dsn)
	if e != nil {
		t.Fatal("PostgreSQL configuration failed")
	}
	cfg.ConnConfig.RuntimeParams["search_path"] = schema
	pool, e := pgxpool.NewWithConfig(ctx, cfg)
	if e != nil {
		t.Fatal(e)
	}
	defer pool.Close()
	node, e := snowflake.NewNode(901)
	if e != nil {
		t.Fatal(e)
	}
	gen := ids{node}
	fx := fixture{ctx: ctx, pool: pool, ids: gen}
	fx.service = app.New(pool, gen, infra.New(), authority{}, authority{}).WithEventRecorder(events{}).WithContinuation(resume{}, resume{}).WithSummaryAuthority(authority{})
	fx.exec(t, infra.Schema)
	fx.exec(t, infra.Schema)
	// Use the migration-owned full table shape for the summary dependency.
	// The disposable database must be migrated; never substitute a permissive
	// hand-written delivery schema for the production contract.
	fx.exec(t, `CREATE TABLE workforce_delivery_requests (LIKE public.workforce_delivery_requests INCLUDING ALL)`)
	fx.exec(t, `CREATE TABLE workforce_task_events(task_id BIGINT,attempt_id BIGINT,event_type TEXT,sequence BIGINT,payload_json JSONB,UNIQUE(task_id,sequence));
CREATE TABLE workforce_tasks(id BIGINT PRIMARY KEY,tenant_id BIGINT,current_task_turn_id BIGINT,status TEXT);
CREATE TABLE workforce_task_turns(id BIGINT PRIMARY KEY,task_id BIGINT,tenant_id BIGINT,status TEXT);
CREATE TABLE workforce_result_evaluation_cycles(id BIGINT PRIMARY KEY,task_id BIGINT,task_turn_id BIGINT,attempt_id BIGINT,decision TEXT,status TEXT);
CREATE TABLE workforce_continuation_checkpoints(id BIGINT PRIMARY KEY,task_id BIGINT,task_turn_id BIGINT,attempt_id BIGINT,evaluation_cycle_id BIGINT,purpose TEXT,decision TEXT,status TEXT,format_version INT,consumed_by_turn_id BIGINT,expires_at TIMESTAMPTZ);
CREATE TABLE interaction_test_authority(tenant BIGINT,task BIGINT PRIMARY KEY,turn BIGINT,attempt BIGINT,session BIGINT,cancelled BOOLEAN,generation BIGINT,owner BIGINT,mode TEXT,resumes INT)`)
	t.Run("terminal read closes residual input and rejects late answer", func(t *testing.T) {
		fx.reset(t)
		open := fx.publish(t, "terminal-read")
		fx.exec(t, `UPDATE interaction_test_authority SET cancelled=true`)
		a := authority{noCapabilities: true, closedAttemptState: d.Superseded}
		s := app.New(pool, gen, infra.New(), a, a).WithEventRecorder(events{})
		for n := 0; n < 2; n++ {
			if _, err := s.Conversation(ctx, user(), app.Query{}); err != nil {
				t.Fatal(err)
			}
		}
		if got := fx.count(t, `SELECT count(*) FROM workforce_agent_interactions WHERE id=$1 AND state='SUPERSEDED' AND version=2`, open.InteractionID); got != 1 {
			t.Fatal("terminal read did not close the question exactly once")
		}
		if _, err := s.Respond(ctx, user(), open.InteractionID, answer("late-terminal")); err != d.ErrNotAllowed {
			t.Fatalf("late answer accepted: %v", err)
		}
		if got := fx.count(t, `SELECT count(*) FROM workforce_agent_interaction_responses`); got != 0 {
			t.Fatal("late answer was persisted")
		}
	})
	t.Run("terminal delivery actions survive without native capabilities", func(t *testing.T) {
		fx.reset(t)
		fx.exec(t, `UPDATE workforce_tasks SET status='SUCCEEDED'; UPDATE interaction_test_authority SET cancelled=true,session=0`)
		a := authority{noCapabilities: true, deliveryActions: true}
		s := app.New(pool, gen, infra.New(), a, a).WithEventRecorder(events{})
		view, err := s.Conversation(ctx, user(), app.Query{})
		if err != nil || len(view.AllowedActions) != 2 || view.AllowedActions[0] != f.DecideDelivery || view.AllowedActions[1] != f.RestartDelivery {
			t.Fatalf("authorized control-plane delivery actions lost: %v, %v", view.AllowedActions, err)
		}
		readOnly := user()
		readOnly.CanContinue = false
		view, err = s.Conversation(ctx, readOnly, app.Query{})
		if err != nil || len(view.AllowedActions) != 0 {
			t.Fatalf("read-only user received delivery actions: %v, %v", view.AllowedActions, err)
		}
		if _, err = s.PostMessage(ctx, user(), f.MessageRequest{Text: "must stay fenced", IdempotencyKey: "terminal"}); err != d.ErrNotAllowed {
			t.Fatalf("terminal native input was not fenced: %v", err)
		}
		wrongTenant := user()
		wrongTenant.TenantID++
		if _, err = s.Conversation(ctx, wrongTenant, app.Query{}); err != d.ErrNotFound {
			t.Fatalf("cross-tenant conversation was not fenced: %v", err)
		}
	})
	t.Run("attention once cancellation fences saved response", func(t *testing.T) {
		fx.reset(t)
		recorder := &attentionEvents{}
		svc := app.New(pool, gen, infra.New(), authority{}, authority{}).WithEventRecorder(recorder).WithContinuation(resume{}, resume{})
		p, err := svc.Publish(ctx, machine(), question("attention"))
		if err != nil {
			t.Fatal(err)
		}
		if _, err = svc.Publish(ctx, machine(), question("attention")); err != nil {
			t.Fatal(err)
		}
		if _, err = svc.Respond(ctx, user(), p.InteractionID, answer("saved")); err != nil {
			t.Fatal(err)
		}
		tx, err := pool.Begin(ctx)
		if err != nil {
			t.Fatal(err)
		}
		defer tx.Rollback(ctx)
		if _, err = (authority{}).AuthorizeUser(ctx, tx, user(), f.Read); err != nil {
			t.Fatal(err)
		}
		if err = svc.CloseAttemptTx(ctx, tx, 10, 20, 40, d.Cancelled); err != nil {
			t.Fatal(err)
		}
		if err = tx.Commit(ctx); err != nil {
			t.Fatal(err)
		}
		item, err := svc.ConversationItem(ctx, user(), p.InteractionID)
		if err != nil {
			t.Fatal(err)
		}
		body := string(item.Body)
		var projection struct {
			Status         string `json:"status"`
			DeliveryStatus string `json:"deliveryStatus"`
		}
		if json.Unmarshal(item.Body, &projection) != nil || projection.Status != d.Cancelled || projection.DeliveryStatus != d.Saved || strings.Contains(body, "localRequestKey") {
			t.Fatal(body)
		}
		deliveries, err := svc.Pull(ctx, machine(), 20)
		if err != nil || len(deliveries) != 0 {
			t.Fatal(deliveries, err)
		}
		if recorder.occurrences != 1 {
			t.Fatal("attention must only occur once", recorder.occurrences)
		}
	})
	t.Run("question page filters safe newest history", func(t *testing.T) {
		fx.reset(t)
		first := fx.publish(t, "first-page")
		for n := 0; n < 7; n++ {
			fx.publish(t, fmt.Sprintf("open-%d", n))
		}
		if _, e := fx.service.Publish(ctx, machine(), question("busy-page")); e != d.ErrBusy {
			t.Fatal(e)
		}
		fx.reset(t)
		first = fx.publish(t, "first-page")
		if replay, e := fx.service.Publish(ctx, machine(), question("first-page")); e != nil || !replay.Replayed {
			t.Fatal(replay, e)
		}
		if _, e := fx.service.Respond(ctx, user(), first.InteractionID, answer("close-first")); e != nil {
			t.Fatal(e)
		}
		last := fx.publish(t, "last-page")
		if _, e := fx.service.PublishEvent(ctx, machine(), f.PublishEventRequest{Kind: "ACTIVITY", LocalRequestKey: "activity-page", Text: "safe"}); e != nil {
			t.Fatal(e)
		}
		page, e := fx.service.Questions(ctx, user(), 0, 1)
		if e != nil || len(page.Items) != 1 || page.Items[0].ID != last.InteractionID || page.NextCursor == nil {
			t.Fatal(page, e)
		}
		page, e = fx.service.Questions(ctx, user(), last.InteractionID, 1)
		if e != nil || len(page.Items) != 1 || page.Items[0].ID != first.InteractionID || page.NextCursor != nil {
			t.Fatal(page, e)
		}
		if strings.Contains(string(page.Items[0].Body), "localRequestKey") {
			t.Fatal("private identity leaked")
		}
		denied := user()
		denied.TenantID = 99
		if _, e = fx.service.Questions(ctx, denied, 0, 20); e == nil {
			t.Fatal("tenant bypass")
		}
	})
	t.Run("nonblocking projection and native close are authoritative", func(t *testing.T) {
		fx.reset(t)
		nonblocking := question("nonblocking")
		nonblocking.IsBlocking = false
		published, e := fx.service.Publish(ctx, machine(), nonblocking)
		if e != nil {
			t.Fatal(e)
		}
		item, e := fx.service.ConversationItem(ctx, user(), published.InteractionID)
		var projection struct {
			IsBlocking bool `json:"isBlocking"`
		}
		if e != nil || json.Unmarshal(item.Body, &projection) != nil || projection.IsBlocking {
			t.Fatal(string(item.Body), e)
		}
		closed, e := fx.service.Close(ctx, machine(), f.CloseRequest{RoundKey: "round", LocalRequestKey: "nonblocking", Reason: "TURN_COMPLETED"})
		if e != nil || closed.Replayed || closed.Version != 2 {
			t.Fatal(closed, e)
		}
		replay, e := fx.service.Close(ctx, machine(), f.CloseRequest{RoundKey: "round", LocalRequestKey: "nonblocking", Reason: "TURN_COMPLETED"})
		if e != nil || !replay.Replayed || replay.Version != 2 {
			t.Fatal(replay, e)
		}
		item, e = fx.service.ConversationItem(ctx, user(), published.InteractionID)
		var closedProjection struct {
			Status string `json:"status"`
		}
		if e != nil || json.Unmarshal(item.Body, &closedProjection) != nil || closedProjection.Status != string(d.Superseded) {
			t.Fatal(string(item.Body), e)
		}
		late := answer("too-late")
		late.ExpectedVersion = 2
		if _, e = fx.service.Respond(ctx, user(), published.InteractionID, late); e != d.ErrNotAllowed {
			t.Fatal(e)
		}
	})
	t.Run("publish answer pull ack receipt replay", func(t *testing.T) {
		fx.reset(t)
		p := fx.publish(t, "request")
		again := fx.publish(t, "request")
		if !again.Replayed || p.InteractionID != again.InteractionID {
			t.Fatal("occurrence not idempotent")
		}
		changed := question("request")
		changed.Questions[0].Title = "changed"
		if _, e := fx.service.Publish(ctx, machine(), changed); e != d.ErrConflict {
			t.Fatal(e)
		}
		saved, e := fx.service.Respond(ctx, user(), p.InteractionID, answer("answer"))
		if e != nil || !saved.Saved || saved.Delivered || saved.ExecutionContinued {
			t.Fatal(saved, e)
		}
		replay, e := fx.service.Respond(ctx, user(), p.InteractionID, answer("answer"))
		if e != nil || !replay.Replayed || replay.ResponseID != saved.ResponseID {
			t.Fatal(replay, e)
		}
		deliveries, e := fx.service.Pull(ctx, machine(), 0)
		if e != nil || len(deliveries) != 1 || deliveries[0].RoundKey != "round" || deliveries[0].LocalRequestKey != "request" {
			t.Fatal(deliveries, e)
		}
		ack, e := fx.service.Ack(ctx, machine(), f.AckRequest{InteractionID: p.InteractionID, ResponseID: saved.ResponseID})
		if e != nil || !ack.Delivered || ack.ExecutionContinued {
			t.Fatal(ack, e)
		}
		ack, e = fx.service.Ack(ctx, machine(), f.AckRequest{InteractionID: p.InteractionID, ResponseID: saved.ResponseID})
		if e != nil || !ack.Replayed {
			t.Fatal(e)
		}
		recovered, e := fx.service.PublishReceipt(ctx, machine(), f.PublishReceiptRequest{Kind: d.QuestionKind, LocalRequestKey: "request", RoundKey: "round", ResponseID: saved.ResponseID, Status: "RECEIVED"})
		if e != nil || recovered.ResponseID != saved.ResponseID || !recovered.Delivered {
			t.Fatal(recovered, e)
		}
		deliveries, e = fx.service.Pull(ctx, machine(), 20)
		if e != nil || len(deliveries) != 0 {
			t.Fatal(deliveries, e)
		}
		if fx.count(t, `SELECT count(*) FROM workforce_agent_interaction_responses`) != 1 || fx.count(t, `SELECT count(*) FROM workforce_task_conversation_items`) != 1 || fx.count(t, `SELECT count(*) FROM workforce_task_events`) != 3 {
			t.Fatal("duplicate facts")
		}
	})
	t.Run("concurrent unique answer", func(t *testing.T) {
		fx.reset(t)
		p := fx.publish(t, "race")
		var wg sync.WaitGroup
		errs := make(chan error, 2)
		for _, key := range []string{"one", "two"} {
			wg.Add(1)
			go func(key string) {
				defer wg.Done()
				_, e := fx.service.Respond(ctx, user(), p.InteractionID, answer(key))
				errs <- e
			}(key)
		}
		wg.Wait()
		close(errs)
		success := 0
		for e := range errs {
			if e == nil {
				success++
			} else if e != d.ErrStale {
				t.Fatal(e)
			}
		}
		if success != 1 || fx.count(t, `SELECT count(*) FROM workforce_agent_interaction_responses`) != 1 {
			t.Fatal("answer race")
		}
	})
	t.Run("fencing tenant permission and rollback", func(t *testing.T) {
		fx.reset(t)
		p := fx.publish(t, "fence")
		bad := machine()
		bad.OwnerEpoch = 2
		if _, e := fx.service.Pull(ctx, bad, 20); e != d.ErrFenced {
			t.Fatal(e)
		}
		bad = machine()
		bad.TenantID = 99
		if _, e := fx.service.Publish(ctx, bad, question("cross")); e != d.ErrFenced {
			t.Fatal(e)
		}
		u := user()
		u.CanContinue = false
		if _, e := fx.service.Respond(ctx, u, p.InteractionID, answer("no")); e != d.ErrNotAllowed {
			t.Fatal(e)
		}
		view, e := fx.service.Conversation(ctx, u, app.Query{})
		if e != nil || len(view.AllowedActions) != 0 {
			t.Fatal(view, e)
		}
		u.TenantID = 99
		if _, e = fx.service.Conversation(ctx, u, app.Query{}); e != d.ErrNotFound {
			t.Fatal(e)
		}
		failing := app.New(pool, gen, infra.New(), authority{}, authority{}).WithEventRecorder(events{fail: true})
		if _, e = fx.service.Respond(ctx, user(), p.InteractionID, answer("before-rollback")); e != nil {
			t.Fatal(e)
		}
		if _, e = failing.Publish(ctx, machine(), question("rollback")); e != d.ErrUnavailable || strings.Contains(e.Error(), "secret") {
			t.Fatal(e)
		}
		if fx.count(t, `SELECT count(*) FROM workforce_agent_interactions`) != 1 {
			t.Fatal("failed event did not rollback aggregate")
		}
		fx.exec(t, `UPDATE interaction_test_authority SET cancelled=true`)
		if _, e = fx.service.Publish(ctx, machine(), question("cancelled")); e != d.ErrFenced {
			t.Fatal(e)
		}
	})
	t.Run("stable history and after updates", func(t *testing.T) {
		fx.reset(t)
		p := fx.publish(t, "old-question")
		for n := 0; n < 24; n++ {
			if _, e := fx.service.PublishEvent(ctx, machine(), f.PublishEventRequest{LocalRequestKey: fmt.Sprint("event", n), Kind: "ACTIVITY", Text: fmt.Sprint("progress ", n)}); e != nil {
				t.Fatal(e)
			}
		}
		latest, e := fx.service.Conversation(ctx, user(), app.Query{})
		if e != nil || len(latest.Items) != 20 || latest.NextCursor == nil {
			t.Fatal(latest, e)
		}
		before, _ := strconv.ParseInt(*latest.NextCursor, 10, 64)
		old, e := fx.service.Conversation(ctx, user(), app.Query{Before: before})
		if e != nil || len(old.Items) != 5 || old.NextCursor != nil {
			t.Fatal(old, e)
		}
		_, e = fx.service.Respond(ctx, user(), p.InteractionID, answer("update"))
		if e != nil {
			t.Fatal(e)
		}
		changes, e := fx.service.Conversation(ctx, user(), app.Query{After: latest.Watermark, Changes: true})
		if e != nil || len(changes.Items) != 1 || changes.Items[0].ID != p.InteractionID || changes.Items[0].Version != 2 {
			t.Fatal(changes, e)
		}
		history, e := fx.service.Conversation(ctx, user(), app.Query{})
		if e != nil || history.Items[0].ID != latest.Items[0].ID || *history.NextCursor != *latest.NextCursor {
			t.Fatal("update moved history boundary")
		}
		for _, item := range changes.Items {
			if strings.Contains(string(item.Body), "attemptId") || strings.Contains(string(item.Body), "roundKey") {
				t.Fatal("private identity in projection")
			}
		}
	})
	t.Run("expiry readstate and batch summary", func(t *testing.T) {
		fx.reset(t)
		p := fx.publish(t, "pending")
		summaries, e := fx.service.TaskSummaries(ctx, 10, 7, []int64{20}, true)
		if e != nil {
			tx, beginErr := pool.Begin(ctx)
			if beginErr != nil {
				t.Fatal(beginErr)
			}
			_, rawErr := infra.New().Summaries(ctx, tx, 10, 70, []int64{20}, []int64{20})
			_ = tx.Rollback(ctx)
			t.Fatalf("summary service=%v store=%v", e, rawErr)
		}
		if summaries[20].PendingActionCount != 1 || !summaries[20].HasUnreadUpdates || summaries[20].FirstPendingActionKind != f.PendingConversationItem || summaries[20].FirstPendingActionID != p.InteractionID {
			t.Fatal(summaries, e)
		}
		page, e := fx.service.Conversation(ctx, user(), app.Query{})
		if e != nil {
			t.Fatal(e)
		}
		if e = fx.service.MarkRead(ctx, user(), page.Watermark); e != nil {
			t.Fatal(e)
		}
		summaries, e = fx.service.TaskSummaries(ctx, 10, 7, []int64{20}, true)
		if e != nil || summaries[20].PendingActionCount != 1 || summaries[20].HasUnreadUpdates {
			t.Fatal(summaries, e)
		}
		summaries, e = fx.service.TaskSummaries(ctx, 10, 7, []int64{20}, false)
		if e != nil || summaries[20].PendingActionCount != 0 {
			t.Fatal(summaries, e)
		}
		fx.exec(t, `UPDATE workforce_tasks SET status='REVIEWING'; UPDATE workforce_task_turns SET status='REVIEWING';
INSERT INTO workforce_result_evaluation_cycles VALUES(50,20,30,40,'WAIT_USER','PERSISTED');
INSERT INTO workforce_continuation_checkpoints VALUES(60,20,30,40,50,'RESULT','WAIT_USER','AVAILABLE',2,NULL,CURRENT_TIMESTAMP+INTERVAL '1 hour')`)
		fx.exec(t, `INSERT INTO workforce_task_conversation_items(id,tenant_id,task_id,version,type,source_kind,source_key,request_digest,body_json,created_at,change_sequence)
VALUES(99,10,20,1,'OPERATION','EXECUTION_OPERATION','99',$1,'{"status":"WAITING_APPROVAL"}'::jsonb,CURRENT_TIMESTAMP,(SELECT COALESCE(max(sequence),0)+2 FROM workforce_task_events WHERE task_id=20))`, strings.Repeat("a", 64))
		summaries, e = fx.service.TaskSummaries(ctx, 10, 7, []int64{20}, true)
		if e != nil || summaries[20].PendingActionCount != 3 || summaries[20].FirstPendingActionKind != f.PendingCompletionCheck || summaries[20].FirstPendingActionID != 60 {
			t.Fatal("mixed pending actions were not counted with a stable conversation anchor", summaries, e)
		}
		summaries, e = fx.service.TaskSummaries(ctx, 10, 7, []int64{20}, false)
		if e != nil || summaries[20].PendingActionCount != 0 || summaries[20].FirstPendingActionKind != "" || summaries[20].FirstPendingActionID != 0 {
			t.Fatal("read-only user received actionable pending projection", summaries, e)
		}
		past := time.Now().Add(-time.Hour)
		fx.exec(t, `UPDATE workforce_agent_interactions SET created_at=$2::timestamptz-INTERVAL '24 hours',expires_at=$2,body_json=jsonb_set(body_json,'{expiresAt}',to_jsonb($3::text)) WHERE id=$1`, p.InteractionID, past, past.UTC().Format(time.RFC3339Nano))
		if _, e = fx.service.Respond(ctx, user(), p.InteractionID, answer("late")); e != d.ErrNotAllowed {
			t.Fatal(e)
		}
		if fx.count(t, `SELECT count(*) FROM workforce_agent_interactions WHERE state='EXPIRED'`) != 1 || fx.count(t, `SELECT count(*) FROM workforce_agent_interaction_responses`) != 0 {
			t.Fatal("expiry not committed")
		}
		summaries, e = fx.service.TaskSummaries(ctx, 10, 7, []int64{20}, true)
		if e != nil || summaries[20].PendingActionCount != 2 || summaries[20].FirstPendingActionKind != f.PendingCompletionCheck || summaries[20].FirstPendingActionID != 60 {
			t.Fatal("expired question changed unrelated pending facts", summaries, e)
		}
		fx.exec(t, `DELETE FROM workforce_task_conversation_items WHERE id=99`)
		summaries, e = fx.service.TaskSummaries(ctx, 10, 7, []int64{20}, true)
		if e != nil || summaries[20].PendingActionCount != 1 || summaries[20].FirstPendingActionKind != f.PendingCompletionCheck || summaries[20].FirstPendingActionID != 60 {
			t.Fatal("completion-check pending decision was not counted safely", summaries, e)
		}
	})
	t.Run("queued message saved bind delivery", func(t *testing.T) {
		fx.reset(t)
		fx.exec(t, `UPDATE interaction_test_authority SET attempt=0,session=0`)
		saved, e := fx.service.PostMessage(ctx, user(), f.MessageRequest{Text: "Please check", IdempotencyKey: "message"})
		if e != nil || !saved.Saved || saved.Delivered {
			t.Fatal(saved, e)
		}
		if fx.count(t, `SELECT count(*) FROM workforce_agent_interactions WHERE kind='USER_MESSAGE' AND attempt_id IS NULL`) != 1 || fx.count(t, `SELECT count(*) FROM workforce_task_conversation_items WHERE interaction_id IS NULL`) != 1 {
			t.Fatal("invented attempt or question")
		}
		fx.exec(t, `UPDATE interaction_test_authority SET attempt=40,session=1`)
		pending, e := fx.service.Pull(ctx, machine(), 20)
		if e != nil || len(pending) != 1 || pending[0].AdapterSessionGeneration != 1 || pending[0].ResponseID != 0 {
			t.Fatal(pending, e)
		}
		delivered, e := fx.service.Ack(ctx, machine(), f.AckRequest{InteractionID: saved.InteractionID})
		if e != nil || !delivered.Delivered || delivered.ExecutionContinued {
			t.Fatal(delivered, e)
		}
		replay, e := fx.service.PostMessage(ctx, user(), f.MessageRequest{Text: "Please check", IdempotencyKey: "message"})
		if e != nil || !replay.Replayed || replay.InteractionID != saved.InteractionID {
			t.Fatal(replay, e)
		}
	})
	t.Run("cold resume atomic and capturing refusal", func(t *testing.T) {
		fx.reset(t)
		p := fx.publish(t, "cold")
		queued, e := fx.service.PostMessage(ctx, user(), f.MessageRequest{Text: "carry me", IdempotencyKey: "carry"})
		if e != nil {
			t.Fatal(e)
		}
		delivered, e := fx.service.PostMessage(ctx, user(), f.MessageRequest{Text: "already delivered", IdempotencyKey: "delivered"})
		if e != nil {
			t.Fatal(e)
		}
		if _, e = fx.service.Ack(ctx, machine(), f.AckRequest{InteractionID: delivered.InteractionID}); e != nil {
			t.Fatal(e)
		}
		fx.exec(t, `UPDATE interaction_test_authority SET mode='CAPTURING'`)
		if _, e := fx.service.Respond(ctx, user(), p.InteractionID, answer("cold-answer")); e != continuation.ErrInteractionSuspendInProgress {
			t.Fatal(e)
		}
		if fx.count(t, `SELECT count(*) FROM workforce_agent_interaction_responses`) != 0 {
			t.Fatal("capturing answer persisted")
		}
		fx.exec(t, `UPDATE interaction_test_authority SET mode='COLD'`)
		failing := app.New(pool, gen, infra.New(), authority{}, authority{}).WithContinuation(resume{}, resume{}).WithEventRecorder(events{fail: true})
		if _, e := failing.Respond(ctx, user(), p.InteractionID, answer("cold-answer")); e != d.ErrUnavailable {
			t.Fatal(e)
		}
		if fx.count(t, `SELECT resumes FROM interaction_test_authority`) != 0 || fx.count(t, `SELECT count(*) FROM workforce_agent_interaction_responses`) != 0 {
			t.Fatal("cold response and continuation did not roll back together")
		}
		if fx.count(t, `SELECT count(*) FROM workforce_agent_interactions WHERE id=$1 AND task_turn_id=30 AND attempt_id=40`, queued.InteractionID) != 1 {
			t.Fatal("message carry escaped rollback")
		}
		saved, e := fx.service.Respond(ctx, user(), p.InteractionID, answer("cold-answer"))
		if e != nil || saved.TaskTurnID != 31 || !saved.Saved || saved.Delivered || saved.ExecutionContinued {
			t.Fatal(saved, e)
		}
		pending, e := fx.service.Pull(ctx, machine(), 20)
		if e != nil || len(pending) != 0 {
			t.Fatal("cold answer delivered to old runtime")
		}
		if fx.count(t, `SELECT resumes FROM interaction_test_authority`) != 1 {
			t.Fatal("resume not atomic")
		}
		if fx.count(t, `SELECT count(*) FROM workforce_agent_interactions WHERE id=$1 AND task_turn_id=31 AND attempt_id IS NULL AND adapter_session_generation IS NULL`, queued.InteractionID) != 1 {
			t.Fatal("saved message not carried")
		}
		fx.exec(t, `UPDATE interaction_test_authority SET turn=31,attempt=41,session=2,mode=''`)
		newMachine := machine()
		newMachine.AttemptID = 41
		newMachine.AdapterSessionGeneration = 2
		pending, e = fx.service.Pull(ctx, newMachine, 20)
		if e != nil || len(pending) != 2 {
			t.Fatal(pending, e)
		}
		for _, delivery := range pending {
			if delivery.InteractionID == delivered.InteractionID {
				t.Fatal("delivered message replayed")
			}
			if delivery.InteractionID == p.InteractionID && (!delivery.ColdResume || delivery.RoundKey != "" || delivery.AdapterSessionGeneration != 2) {
				t.Fatal(delivery)
			}
		}
		coldReceipt := f.PublishReceiptRequest{Kind: d.QuestionKind, LocalRequestKey: "cold", RoundKey: "new-native-round", ResponseID: saved.ResponseID, Status: "RECEIVED"}
		for _, deliveredAlready := range []bool{false, true} {
			if deliveredAlready {
				if _, e = fx.service.Ack(ctx, newMachine, f.AckRequest{InteractionID: p.InteractionID, ResponseID: saved.ResponseID}); e != nil {
					t.Fatal(e)
				}
			}
			tx, e := pool.Begin(ctx)
			if e != nil {
				t.Fatal(e)
			}
			if _, e = (authority{}).Validate(ctx, tx, newMachine); e != nil {
				t.Fatal(e)
			}
			restored, e := fx.service.RestoredQuestionTx(ctx, tx, 10, 20, 31, saved.ResponseID)
			if e != nil || restored.LocalRequestKey != "cold" || restored.RoundKey != "round" || len(restored.Questions) != 1 || len(restored.Questions[0].Options) != 2 {
				t.Fatal(restored, e)
			}
			if _, e = fx.service.RestoredQuestionTx(ctx, tx, 10, 20, 32, saved.ResponseID); e != d.ErrFenced {
				t.Fatal("wrong target", e)
			}
			if _, e = fx.service.RestoredQuestionTx(ctx, tx, 10, 21, 31, saved.ResponseID); e != d.ErrFenced {
				t.Fatal("wrong task", e)
			}
			if _, e = fx.service.RestoredQuestionTx(ctx, tx, 99, 20, 31, saved.ResponseID); e == nil {
				t.Fatal("wrong tenant")
			}
			_ = tx.Rollback(ctx)
		}
		if _, e = fx.service.PublishReceipt(ctx, newMachine, coldReceipt); e != nil {
			t.Fatal(e)
		}
		wrong := coldReceipt
		wrong.ResponseID++
		if _, e = fx.service.PublishReceipt(ctx, newMachine, wrong); e == nil {
			t.Fatal("unrelated old response accepted")
		}
		if _, e = fx.service.Ack(ctx, newMachine, f.AckRequest{InteractionID: p.InteractionID, ResponseID: saved.ResponseID}); e != nil {
			t.Fatal(e)
		}
		if fx.count(t, `SELECT count(*) FROM workforce_agent_interactions WHERE id=$1 AND task_turn_id=30 AND attempt_id=40 AND adapter_session_generation=1 AND delivery_state='DELIVERED'`, p.InteractionID) != 1 {
			t.Fatal("cold question occurrence overwritten")
		}
		if _, e = fx.service.PublishReceipt(ctx, machine(), coldReceipt); e == nil {
			t.Fatal("old worker not fenced")
		}
	})
	t.Run("session occurrence and suspend gate", func(t *testing.T) {
		fx.reset(t)
		p := fx.publish(t, "same-key")
		tx, e := pool.Begin(ctx)
		if e != nil {
			t.Fatal(e)
		}
		gate := continuation.InteractionSuspendOccurrence{TenantID: 10, TaskID: 20, TaskTurnID: 30, AttemptID: 40, InteractionID: p.InteractionID, AdapterSessionGeneration: 1}
		if e = fx.service.AuthorizeInteractionSuspendTx(ctx, tx, gate); e != nil {
			t.Fatal(e)
		}
		gate.AdapterSessionGeneration = 2
		if e = fx.service.AuthorizeInteractionSuspendTx(ctx, tx, gate); e != d.ErrFenced {
			t.Fatal(e)
		}
		_ = tx.Rollback(ctx)
		if _, e = fx.service.Respond(ctx, user(), p.InteractionID, answer("before-session-change")); e != nil {
			t.Fatal(e)
		}
		fx.exec(t, `UPDATE interaction_test_authority SET session=2`)
		m := machine()
		m.AdapterSessionGeneration = 2
		p2, e := fx.service.Publish(ctx, m, question("same-key"))
		if e != nil || p2.InteractionID == p.InteractionID {
			t.Fatal(p2, e)
		}
		if _, e = fx.service.Respond(ctx, user(), p.InteractionID, answer("stale-session")); e != d.ErrFenced {
			t.Fatal(e)
		}
		if _, e = fx.service.PublishReceipt(ctx, m, f.PublishReceiptRequest{Kind: d.QuestionKind, LocalRequestKey: "same-key", RoundKey: "wrong", ResponseID: 1, Status: "RECEIVED"}); e != d.ErrFenced {
			t.Fatal(e)
		}
	})
	t.Run("same idempotency concurrent replay and pagination caps", func(t *testing.T) {
		fx.reset(t)
		p := fx.publish(t, "same-answer")
		results := make(chan f.Receipt, 2)
		errs := make(chan error, 2)
		var wg sync.WaitGroup
		for n := 0; n < 2; n++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				r, e := fx.service.Respond(ctx, user(), p.InteractionID, answer("same-key"))
				results <- r
				errs <- e
			}()
		}
		wg.Wait()
		close(results)
		close(errs)
		for e := range errs {
			if e != nil {
				t.Fatal(e)
			}
		}
		var id int64
		replayed := 0
		for r := range results {
			if id != 0 && r.ResponseID != id {
				t.Fatal("different replay IDs")
			}
			id = r.ResponseID
			if r.Replayed {
				replayed++
			}
		}
		if replayed != 1 {
			t.Fatal("concurrent replay not recognized")
		}
		for _, q := range []app.Query{{Limit: 51}, {Limit: -1}, {Before: 1, After: 1}, {Cursor: "01"}} {
			if _, e := fx.service.Conversation(ctx, user(), q); e != d.ErrInvalid {
				t.Fatal(q, e)
			}
		}
		tasks := make([]int64, 201)
		if _, e := fx.service.TaskSummaries(ctx, 10, 7, tasks, true); e != d.ErrInvalid {
			t.Fatal(e)
		}
	})
	t.Run("missing admitted capabilities deny all mutation surfaces", func(t *testing.T) {
		fx.reset(t)
		p := fx.publish(t, "caps")
		no := authority{noCapabilities: true}
		s := app.New(pool, gen, infra.New(), no, no).WithEventRecorder(events{}).WithContinuation(resume{}, resume{})
		if _, e := s.Publish(ctx, machine(), question("denied")); e != d.ErrNotAllowed {
			t.Fatal(e)
		}
		if _, e := s.Respond(ctx, user(), p.InteractionID, answer("denied")); e != d.ErrNotAllowed {
			t.Fatal(e)
		}
		if _, e := s.PostMessage(ctx, user(), f.MessageRequest{Text: "denied", IdempotencyKey: "denied"}); e != d.ErrNotAllowed {
			t.Fatal(e)
		}
		if _, e := s.PublishEvent(ctx, machine(), f.PublishEventRequest{LocalRequestKey: "denied", Kind: "ACTIVITY", Text: "denied"}); e != d.ErrNotAllowed {
			t.Fatal(e)
		}
		view, e := s.Conversation(ctx, user(), app.Query{})
		if e != nil || len(view.AllowedActions) != 0 {
			t.Fatal(view, e)
		}
		saved, e := fx.service.Respond(ctx, user(), p.InteractionID, answer("yes"))
		if e != nil {
			t.Fatal(e)
		}
		if _, e = s.Pull(ctx, machine(), 20); e != d.ErrNotAllowed {
			t.Fatal(e)
		}
		if _, e = s.Ack(ctx, machine(), f.AckRequest{InteractionID: p.InteractionID, ResponseID: saved.ResponseID}); e != d.ErrNotAllowed {
			t.Fatal(e)
		}
		if _, e = s.PublishReceipt(ctx, machine(), f.PublishReceiptRequest{Kind: d.QuestionKind, LocalRequestKey: "caps", RoundKey: "round", ResponseID: saved.ResponseID, Status: "RECEIVED"}); e != d.ErrNotAllowed {
			t.Fatal(e)
		}
	})
	t.Run("late native input fenced but receipt and observation retained", func(t *testing.T) {
		fx.reset(t)
		p := fx.publish(t, "late")
		saved, e := fx.service.Respond(ctx, user(), p.InteractionID, answer("late-answer"))
		if e != nil {
			t.Fatal(e)
		}
		closed := authority{nativeClosed: true}
		s := app.New(pool, gen, infra.New(), closed, closed).WithEventRecorder(events{}).WithContinuation(resume{}, resume{})
		if _, e = s.Publish(ctx, machine(), question("new-late")); e != d.ErrNotAllowed {
			t.Fatal(e)
		}
		replay, e := s.Publish(ctx, machine(), question("late"))
		if e != nil || !replay.Replayed {
			t.Fatal(replay, e)
		}
		if _, e = s.Pull(ctx, machine(), 20); e != d.ErrNotAllowed {
			t.Fatal(e)
		}
		r, e := s.PublishReceipt(ctx, machine(), f.PublishReceiptRequest{Kind: d.QuestionKind, LocalRequestKey: "late", RoundKey: "round", ResponseID: saved.ResponseID, Status: "RECEIVED"})
		if e != nil || r.Delivered {
			t.Fatal(r, e)
		}
		if _, e = s.PublishEvent(ctx, machine(), f.PublishEventRequest{LocalRequestKey: "late-activity", Kind: "ACTIVITY", Text: "Finished observation"}); e != nil {
			t.Fatal(e)
		}
		ack, e := s.Ack(ctx, machine(), f.AckRequest{InteractionID: p.InteractionID, ResponseID: saved.ResponseID})
		if e != nil || !ack.Delivered || ack.ExecutionContinued {
			t.Fatal(ack, e)
		}
	})
	t.Run("server open question identity and answer wins reserve", func(t *testing.T) {
		fx.reset(t)
		p := fx.publish(t, "server-id")
		open, e := fx.service.OpenQuestions(ctx, machine())
		if e != nil || len(open) != 1 || open[0].InteractionID != p.InteractionID || open[0].LocalRequestKey != "server-id" || open[0].RoundKey != "round" {
			t.Fatal(open, e)
		}
		if _, e = fx.service.Respond(ctx, user(), p.InteractionID, answer("answer-first")); e != nil {
			t.Fatal(e)
		}
		open, e = fx.service.OpenQuestions(ctx, machine())
		if e != nil || len(open) != 0 {
			t.Fatal(open, e)
		}
		tx, e := pool.Begin(ctx)
		if e != nil {
			t.Fatal(e)
		}
		e = fx.service.AuthorizeInteractionSuspendTx(ctx, tx, continuation.InteractionSuspendOccurrence{TenantID: 10, TaskID: 20, TaskTurnID: 30, AttemptID: 40, InteractionID: p.InteractionID, AdapterSessionGeneration: 1})
		_ = tx.Rollback(ctx)
		if e != continuation.ErrInteractionSuspendAnswerWon {
			t.Fatal(e)
		}
	})
	t.Run("legacy questions align to blocking without losing the safe body", func(t *testing.T) {
		fx.reset(t)
		published := fx.publish(t, "legacy-blocking")
		fx.exec(t, `UPDATE workforce_agent_interactions SET body_json=body_json-'isBlocking' WHERE id=$1`, published.InteractionID)
		fx.exec(t, `ALTER TABLE workforce_agent_interactions DROP COLUMN is_blocking`)
		fx.exec(t, infra.Schema)
		var columnValue bool
		var bodyValue bool
		if e := pool.QueryRow(ctx, `SELECT is_blocking,body_json->>'isBlocking'='true' FROM workforce_agent_interactions WHERE id=$1`, published.InteractionID).Scan(&columnValue, &bodyValue); e != nil || !columnValue || !bodyValue {
			t.Fatal(columnValue, bodyValue, e)
		}
	})
}
