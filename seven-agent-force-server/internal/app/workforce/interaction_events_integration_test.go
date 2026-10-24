package workforce

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	deliveryfacade "github.com/CaixyPromise/seven-agent/force/internal/app/workforce/delivery/facade"
	interactionapp "github.com/CaixyPromise/seven-agent/force/internal/app/workforce/interaction/application"
	interactiondomain "github.com/CaixyPromise/seven-agent/force/internal/app/workforce/interaction/domain"
	interactionfacade "github.com/CaixyPromise/seven-agent/force/internal/app/workforce/interaction/facade"
	interactioninfra "github.com/CaixyPromise/seven-agent/force/internal/app/workforce/interaction/infrastructure"
	notificationinfra "github.com/CaixyPromise/seven-agent/force/internal/app/workforce/tasknotification/infrastructure"

	"github.com/CaixyPromise/seven-agent/force/internal/app/workforce/tasknotification/domain"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type nativeQuestionIDs struct{ next int64 }

func (g *nativeQuestionIDs) NextID() int64 { g.next++; return g.next }

// Clone migrated table definitions into a private, disposable schema. No public
// task, credential, subscription or identity rows are copied or modified.
func TestPostgreSQLNativeQuestionAttention(t *testing.T) {
	dsn := os.Getenv("INTERACTION_TEST_POSTGRES_DSN")
	if dsn == "" {
		t.Skip("NOT RUN: INTERACTION_TEST_POSTGRES_DSN is required")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	admin, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatal("invalid integration configuration")
	}
	defer admin.Close()
	schema := fmt.Sprintf("qa_question_attention_%d", time.Now().UnixNano())
	quoted := pgx.Identifier{schema}.Sanitize()
	if _, err = admin.Exec(ctx, "CREATE SCHEMA "+quoted); err != nil {
		t.Fatal(err)
	}
	defer func() {
		cleanup, stop := context.WithTimeout(context.Background(), 10*time.Second)
		defer stop()
		if _, err := admin.Exec(cleanup, "DROP SCHEMA "+quoted+" CASCADE"); err != nil {
			t.Error(err)
		}
	}()
	for _, table := range []string{"workforce_tasks", "workforce_task_turns", "workforce_agent_interactions", "workforce_task_events", "workforce_task_conversation_items", "workforce_task_notification_subscriptions", "workforce_task_notification_subscription_channels", "workforce_task_notification_records", "workforce_task_notification_record_channels", "workforce_task_notification_record_recipients", "workforce_outbox_events"} {
		if _, err = admin.Exec(ctx, "CREATE TABLE "+pgx.Identifier{schema, table}.Sanitize()+" (LIKE "+pgx.Identifier{"public", table}.Sanitize()+" INCLUDING ALL)"); err != nil {
			t.Fatal(err)
		}
	}
	cfg, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		t.Fatal("invalid integration configuration")
	}
	cfg.ConnConfig.RuntimeParams["search_path"] = quoted + ",pg_catalog"
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	for _, sql := range []string{
		`INSERT INTO workforce_tasks(id,tenant_id,created_by,idempotency_key,request_hash,repository_url,repository_ref,task_instruction,name,status,current_task_turn_id,result_processor_release_id,project_id,project_version_id,execution_requirements_json,execution_requirements_digest) VALUES(20,10,7,'qa','qa','https://example.invalid/qa','main','qa','原生提问验收','RUNNING',30,81,82,83,'{}','aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa')`,
		`INSERT INTO workforce_task_turns(id,task_id,tenant_id,turn_number,instruction,execution_snapshot_digest,continuation_lineage_id,created_by,processor_release_id,processor_digest,parameters_json) VALUES(30,20,10,1,'qa','qa',30,7,81,'qa','{}')`,
	} {
		if _, err = pool.Exec(ctx, sql); err != nil {
			t.Fatal(err)
		}
	}
	// A cloned pre-fix development DB may still have INTEGER here. Match the
	// corrected migration; schemaalignment separately tests the actual widening.
	if _, err = pool.Exec(ctx, `ALTER TABLE workforce_task_notification_records ALTER COLUMN source_occurrence TYPE BIGINT USING source_occurrence::BIGINT`); err != nil {
		t.Fatal(err)
	}
	store := notificationinfra.NewStore(pool, &nativeQuestionIDs{next: 700})
	for _, enabled := range []bool{false, true} {
		t.Run(fmt.Sprintf("subscription_enabled_%t", enabled), func(t *testing.T) {
			tx, err := pool.Begin(ctx)
			if err != nil {
				t.Fatal(err)
			}
			defer tx.Rollback(ctx)
			command := domain.DefaultSubscriptionCommand()
			command.Enabled = enabled
			if _, _, err = store.CreateSubscription(ctx, tx, 10, 20, 7, command); err != nil {
				t.Fatal(err)
			}
			now := time.Now().UTC()
			i := interactiondomain.Interaction{ID: 8000000000000000012, TenantID: 10, TaskID: 20, TaskTurnID: 30, AttemptID: 40, AdapterSessionGeneration: 1, LocalRequestKey: "native:qa", RoundKey: "round:1", PrincipalID: 80, Kind: interactiondomain.QuestionKind, State: interactiondomain.Open, DeliveryState: interactiondomain.NoDelivery, Version: 1, Digest: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", IsBlocking: false, Questions: []interactiondomain.Question{{Key: "body_language", Title: "正文语言", Kind: "SINGLE_CHOICE", Required: true, AllowFreeText: true, Options: []interactiondomain.Option{{Key: "o1-1", Label: "中文（推荐）"}, {Key: "o1-2", Label: "英文"}}}}, CreatedAt: now, ExpiresAt: now.Add(24 * time.Hour)}
			if err = interactioninfra.New().Put(ctx, tx, i, true); err != nil {
				t.Fatal(err)
			}
			sequence, err := (interactionEvents{notifications: store}).RecordInteractionEvent(ctx, tx, interactionfacade.Event{TenantID: 10, TaskID: 20, TaskTurnID: 30, AttemptID: 40, ItemID: i.ID, Version: 1, Type: interactiondomain.QuestionKind, AttentionRequired: true})
			if err != nil {
				t.Fatal(err)
			}
			if err = interactioninfra.New().PutItem(ctx, tx, interactiondomain.Item{ID: i.ID, TenantID: 10, TaskID: 20, InteractionID: i.ID, Version: 1, Type: interactiondomain.QuestionKind, SourceKind: interactiondomain.QuestionKind, SourceKey: "8000000000000000012", Digest: i.Digest, Body: i.Projection(), CreatedAt: now, Sequence: sequence}); err != nil {
				t.Fatal(err)
			}
			var records, outbox int
			if err = tx.QueryRow(ctx, `SELECT count(*) FROM workforce_task_notification_records`).Scan(&records); err != nil {
				t.Fatal(err)
			}
			if err = tx.QueryRow(ctx, `SELECT count(*) FROM workforce_outbox_events`).Scan(&outbox); err != nil {
				t.Fatal(err)
			}
			wanted := 0
			if enabled {
				wanted = 1
			}
			if records != 1 || outbox != wanted {
				t.Fatalf("records=%d outbox=%d", records, outbox)
			}
			// Reauthorization must project a delivery approval card through the
			// real notification and conversation adapters, even after the Agent exits.
			if _, err = tx.Exec(ctx, `UPDATE workforce_tasks SET status='SUCCEEDED' WHERE id=20`); err != nil {
				t.Fatal(err)
			}
			service := interactionapp.New(pool, &nativeQuestionIDs{next: 900}, interactioninfra.New(), nil, nil).WithEventRecorder(interactionEvents{notifications: store})
			if err = (deliveryConversationProjector{target: service}).ProjectDeliveryTx(ctx, tx, deliveryfacade.Projection{ID: 8000000000000000013, IntentID: 8000000000000000013, RequestID: 8000000000000000014, TenantID: 10, TaskID: 20, TaskTurnID: 30, AttemptID: 40, Version: 7, IntentVersion: 6, RequestVersion: 7, Status: "WAITING_USER_DECISION", DeliveryMode: "NEW_BRANCH_AND_DRAFT_REVIEW", Authorization: "ASK_AFTER_GATES", AttentionRequired: true, AllowedActions: []string{"DELIVER", "SKIP_THIS_RESULT"}, OccurredAt: now}); err != nil {
				t.Fatalf("delivery reauthorization projection: %v", err)
			}
			if err = (deliveryConversationProjector{target: service}).ProjectDeliveryTx(ctx, tx, deliveryfacade.Projection{ID: 8000000000000000013, IntentID: 8000000000000000013, RequestID: 8000000000000000014, TenantID: 10, TaskID: 20, TaskTurnID: 30, AttemptID: 40, Version: 9, IntentVersion: 8, RequestVersion: 9, Status: "WAITING_USER_DECISION", DeliveryMode: "NEW_BRANCH_AND_DRAFT_REVIEW", Authorization: "ASK_AFTER_GATES", AttentionRequired: true, AllowedActions: []string{"DELIVER", "SKIP_THIS_RESULT"}, OccurredAt: now}); err != nil {
				t.Fatalf("repeated delivery approval projection: %v", err)
			}
			if err = tx.QueryRow(ctx, `SELECT count(*) FROM workforce_task_notification_records`).Scan(&records); err != nil || records != 1 {
				t.Fatalf("delivery projection recreated Agent attention: records=%d err=%v", records, err)
			}
			var cardVersion int64
			if err = tx.QueryRow(ctx, `SELECT version FROM workforce_task_conversation_items WHERE id=8000000000000000013`).Scan(&cardVersion); err != nil || cardVersion != 9 {
				t.Fatalf("delivery card did not advance: version=%d err=%v", cardVersion, err)
			}
		})
	}
}
