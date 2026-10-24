package application

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
	continuation "github.com/CaixyPromise/seven-agent/force/internal/app/workforce/continuation/facade"
	"github.com/CaixyPromise/seven-agent/force/internal/app/workforce/interaction/domain"
	"github.com/CaixyPromise/seven-agent/force/internal/app/workforce/interaction/facade"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"strconv"
	"time"
)

type Service struct {
	pool      *pgxpool.Pool
	ids       facade.IDGenerator
	repo      Repository
	users     facade.UserAuthority
	machines  facade.MachineAuthority
	finder    continuation.InteractionResumeFinder
	resumer   continuation.InteractionResumer
	events    facade.EventRecorder
	summaries facade.SummaryAuthority
}

func New(pool *pgxpool.Pool, ids facade.IDGenerator, repo Repository, users facade.UserAuthority, machines facade.MachineAuthority) *Service {
	return &Service{pool: pool, ids: ids, repo: repo, users: users, machines: machines}
}

// WithContinuation is startup wiring, before the Service is used concurrently.
// Finder is required even for short answers: absence cannot prove no checkpoint.
func (s *Service) WithContinuation(f continuation.InteractionResumeFinder, r continuation.InteractionResumer) *Service {
	s.finder = f
	s.resumer = r
	return s
}
func (s *Service) WithEventRecorder(e facade.EventRecorder) *Service { s.events = e; return s }

var _ facade.MachineControl = (*Service)(nil)
var _ facade.ExternalProjector = (*Service)(nil)

func (s *Service) transaction(ctx context.Context, fn func(pgx.Tx) error) error {
	if s.pool == nil || s.repo == nil || s.ids == nil {
		return domain.ErrUnavailable
	}
	tx, e := s.pool.BeginTx(ctx, pgx.TxOptions{})
	if e != nil {
		return domain.ErrUnavailable
	}
	defer tx.Rollback(ctx)
	if e = fn(tx); e != nil {
		return safe(e)
	}
	return safe(tx.Commit(ctx))
}

// All unexpected dependency errors are reduced to a stable code, including SQL
// text, constraint diagnostics and continuation/provider errors.
func safe(e error) error {
	if e == nil {
		return nil
	}
	if errors.Is(e, continuation.ErrInteractionSuspendInProgress) {
		return continuation.ErrInteractionSuspendInProgress
	}
	switch e {
	case domain.ErrInvalid, domain.ErrNotFound, domain.ErrStale, domain.ErrConflict, domain.ErrNotAllowed, domain.ErrUnavailable, domain.ErrFenced, domain.ErrBusy:
		return e
	}
	return domain.ErrUnavailable
}
func validTarget(t facade.Target) bool { return t.TenantID > 0 && t.TaskID > 0 }
func active(t facade.Target) bool {
	return validTarget(t) && t.TaskTurnID > 0 && t.AttemptID > 0 && t.AdapterSessionGeneration > 0
}
func capable(t facade.Target, wanted string) bool {
	for _, v := range t.Capabilities {
		if v == wanted {
			return true
		}
	}
	return false
}
func (s *Service) user(ctx context.Context, tx pgx.Tx, u facade.User, action string) (facade.Target, error) {
	if u.TenantID <= 0 || u.TaskID <= 0 || u.UserID <= 0 {
		return facade.Target{}, domain.ErrInvalid
	}
	if s.users == nil {
		return facade.Target{}, domain.ErrUnavailable
	}
	if action != facade.Read && !u.CanContinue {
		return facade.Target{}, domain.ErrNotAllowed
	}
	t, e := s.users.AuthorizeUser(ctx, tx, u, action)
	if e != nil {
		return t, e
	}
	if !validTarget(t) || t.TenantID != u.TenantID || t.TaskID != u.TaskID || (action != facade.Read && t.PrincipalID <= 0) {
		return t, domain.ErrFenced
	}
	if !u.CanContinue {
		t.AllowedActions = []string{}
	}
	actions := []string{}
	for _, v := range t.AllowedActions {
		// Delivery belongs to the control plane, including after the Agent exits.
		// Preserve only the actions already authorized by the user authority;
		// card state and Delivery HTTP permissions still gate the actual decision.
		if v == facade.DecideDelivery || v == facade.RestartDelivery {
			actions = append(actions, v)
			continue
		}
		if (v == facade.Answer && capable(t, "interaction.respond")) || (v == facade.SendMessage && capable(t, "user.message")) || (v == facade.DecideOperation && capable(t, "tool.request") && capable(t, "tool.result")) {
			actions = append(actions, v)
		}
	}
	t.AllowedActions = actions
	if (action == facade.Answer && !capable(t, "interaction.respond")) || (action == facade.SendMessage && !capable(t, "user.message")) {
		return t, domain.ErrNotAllowed
	}
	// Authority locks the Task before this shared event allocation lock.
	_, e = tx.Exec(ctx, `SELECT pg_advisory_xact_lock($1)`, t.TaskID)
	if e == nil && action == facade.Read && t.AttemptID > 0 && t.ClosedAttemptState != "" {
		// Reconcile historical residual questions through the same lifecycle port
		// as cancellation. The parent authority holds the Task lock and supplies
		// the closure state; browser JSON and missing capabilities cannot choose it.
		e = s.CloseAttemptTx(ctx, tx, t.TenantID, t.TaskID, t.AttemptID, t.ClosedAttemptState)
	}
	return t, e
}
func (s *Service) machine(ctx context.Context, tx pgx.Tx, m facade.Machine) (facade.Target, error) {
	if m.TenantID < 0 || m.TaskID < 0 || m.TaskTurnID < 0 || m.AttemptID <= 0 || m.WorkerID <= 0 || m.InstanceID == "" || m.HostBootID == "" || m.Generation <= 0 || m.OwnerEpoch <= 0 || m.AdapterSessionGeneration <= 0 || m.ExecutionLease == "" {
		return facade.Target{}, domain.ErrInvalid
	}
	if s.machines == nil {
		return facade.Target{}, domain.ErrUnavailable
	}
	t, e := s.machines.Validate(ctx, tx, m)
	if e != nil {
		return t, e
	}
	if !active(t) || t.PrincipalID <= 0 || (m.TenantID != 0 && t.TenantID != m.TenantID) || (m.TaskID != 0 && t.TaskID != m.TaskID) || (m.TaskTurnID != 0 && t.TaskTurnID != m.TaskTurnID) || t.AttemptID != m.AttemptID || t.AdapterSessionGeneration != m.AdapterSessionGeneration {
		return t, domain.ErrFenced
	}
	_, e = tx.Exec(ctx, `SELECT pg_advisory_xact_lock($1)`, t.TaskID)
	return t, e
}
func receipt(i domain.Interaction, replay bool) facade.Receipt {
	return facade.Receipt{InteractionID: i.ID, ResponseID: i.ResponseID, Version: i.Version, Saved: i.DeliveryState == domain.Saved || i.DeliveryState == domain.Delivered, Delivered: i.DeliveryState == domain.Delivered, TaskTurnID: i.ResumeTaskTurnID, Replayed: replay}
}
func (s *Service) persist(ctx context.Context, tx pgx.Tx, i domain.Interaction, insert bool) error {
	if e := s.repo.Put(ctx, tx, i, insert); e != nil {
		return e
	}
	raw := i.Projection()
	item := domain.Item{ID: i.ID, TenantID: i.TenantID, TaskID: i.TaskID, Version: i.Version, Type: i.Kind, Body: raw, CreatedAt: i.CreatedAt, SourceKind: i.Kind, SourceKey: strconv.FormatInt(i.ID, 10), Digest: i.Digest}
	if i.Kind == domain.QuestionKind {
		item.InteractionID = i.ID
	}
	return s.project(ctx, tx, item, i.AttemptID, i.TaskTurnID)
}
func (s *Service) project(ctx context.Context, tx pgx.Tx, item domain.Item, attempt, turn int64) error {
	return s.projectWithAttention(ctx, tx, item, attempt, turn, item.Type == domain.QuestionKind && item.Version == 1)
}

func (s *Service) projectWithAttention(ctx context.Context, tx pgx.Tx, item domain.Item, attempt, turn int64, attention bool) error {
	if s.events == nil {
		return domain.ErrUnavailable
	}
	sequence, e := s.events.RecordInteractionEvent(ctx, tx, facade.Event{TenantID: item.TenantID, TaskTurnID: turn, TaskID: item.TaskID, AttemptID: attempt, ItemID: item.ID, Version: item.Version, Type: item.Type, AttentionRequired: attention})
	if e != nil {
		return e
	}
	if sequence <= 0 {
		return domain.ErrUnavailable
	}
	item.Sequence = sequence
	return s.repo.PutItem(ctx, tx, item)
}

func (s *Service) ProjectExternalTx(ctx context.Context, tx pgx.Tx, p facade.ExternalProjection) error {
	digestBytes, digestErr := hex.DecodeString(p.Digest)
	if tx == nil || p.ID <= 0 || p.TenantID <= 0 || p.TaskID <= 0 || p.TaskTurnID <= 0 || p.AttemptID <= 0 || p.Version <= 0 || p.Type == "" || p.SourceKind == "" || p.SourceKey == "" || digestErr != nil || len(digestBytes) != 32 || len(p.Body) == 0 || len(p.Body) > domain.MaxAnswerBytes {
		return domain.ErrInvalid
	}
	var object map[string]any
	if json.Unmarshal(p.Body, &object) != nil || object == nil {
		return domain.ErrInvalid
	}
	item := domain.Item{ID: p.ID, TenantID: p.TenantID, TaskID: p.TaskID, Version: p.Version, Type: p.Type, SourceKind: p.SourceKind, SourceKey: p.SourceKey, Digest: p.Digest, Body: append(json.RawMessage(nil), p.Body...), CreatedAt: p.OccurredAt}
	return s.projectWithAttention(ctx, tx, item, p.AttemptID, p.TaskTurnID, p.AttentionRequired)
}
func (s *Service) Publish(ctx context.Context, m facade.Machine, r facade.PublishRequest) (out facade.Receipt, err error) {
	r.Questions = domain.NormalizeQuestions(r.Questions)
	if (r.RoundKey != "" && !domain.ValidKey(r.RoundKey)) || !domain.ValidKey(r.LocalRequestKey) || domain.ValidateQuestions(r.Questions) != nil {
		return out, domain.ErrInvalid
	}
	err = s.transaction(ctx, func(tx pgx.Tx) error {
		t, e := s.machine(ctx, tx, m)
		if e != nil {
			return e
		}
		if !capable(t, "interaction.request") {
			return domain.ErrNotAllowed
		}
		digest := domain.Digest(r)
		old, e := s.repo.FindOccurrence(ctx, tx, t.AttemptID, t.AdapterSessionGeneration, r.LocalRequestKey)
		if e != nil {
			return e
		}
		if old != nil {
			if old.TenantID != t.TenantID || old.TaskID != t.TaskID || old.Kind != domain.QuestionKind || old.Digest != digest {
				return domain.ErrConflict
			}
			out = receipt(*old, true)
			return nil
		}
		if !t.AcceptsNativeInput {
			return domain.ErrNotAllowed
		}
		if e = s.expire(ctx, tx, t.TenantID, t.TaskID); e != nil {
			return e
		}
		open, e := s.repo.OpenQuestions(ctx, tx, t.TenantID, t.TaskID)
		if e != nil {
			return e
		}
		if len(open) >= 8 {
			return domain.ErrBusy
		}
		i := domain.Interaction{ID: s.ids.NextID(), TenantID: t.TenantID, TaskID: t.TaskID, TaskTurnID: t.TaskTurnID, AttemptID: t.AttemptID, AdapterSessionGeneration: t.AdapterSessionGeneration, LocalRequestKey: r.LocalRequestKey, RoundKey: r.RoundKey, PrincipalID: t.PrincipalID, Kind: domain.QuestionKind, State: domain.Open, DeliveryState: domain.NoDelivery, Version: 1, Digest: digest, Questions: r.Questions, IsBlocking: r.IsBlocking, CreatedAt: time.Now().UTC(), ExpiresAt: time.Now().UTC().Add(24 * time.Hour)}
		if e = s.persist(ctx, tx, i, true); e != nil {
			return e
		}
		out = receipt(i, false)
		return nil
	})
	return
}
func (s *Service) Respond(ctx context.Context, u facade.User, id int64, r facade.ResponseRequest) (out facade.Receipt, err error) {
	r.Answers = domain.NormalizeAnswers(r.Answers)
	var rejected error
	if id <= 0 || r.ExpectedVersion <= 0 || !domain.ValidKey(r.IdempotencyKey) {
		return out, domain.ErrInvalid
	}
	raw, e := json.Marshal(r.Answers)
	if e != nil || len(raw) > domain.MaxAnswerBytes {
		return out, domain.ErrInvalid
	}
	err = s.transaction(ctx, func(tx pgx.Tx) error {
		t, e := s.user(ctx, tx, u, facade.Answer)
		if e != nil {
			return e
		}
		digest := domain.Digest(struct {
			ID      int64
			Request facade.ResponseRequest
		}{id, r})
		old, e := s.repo.FindResponse(ctx, tx, u.TenantID, u.TaskID, t.PrincipalID, r.IdempotencyKey)
		if e != nil {
			return e
		}
		if old != nil {
			if old.Digest != digest || old.InteractionID != id {
				return domain.ErrConflict
			}
			i, e := s.repo.Get(ctx, tx, u.TenantID, u.TaskID, id)
			if e != nil {
				return e
			}
			out = receipt(i, true)
			return nil
		}
		i, e := s.repo.Get(ctx, tx, u.TenantID, u.TaskID, id)
		if e != nil {
			return e
		}
		if !active(t) || i.AttemptID != t.AttemptID || i.TaskTurnID != t.TaskTurnID || i.AdapterSessionGeneration != t.AdapterSessionGeneration {
			return domain.ErrFenced
		}
		if i.Expire(time.Now()) {
			if e = s.persist(ctx, tx, i, false); e != nil {
				return e
			}
			rejected = domain.ErrNotAllowed
			return nil
		}
		if s.finder == nil {
			return domain.ErrUnavailable
		}
		target, e := s.finder.FindInteractionResumeTx(ctx, tx, continuation.InteractionResumeLookup{TenantID: u.TenantID, TaskID: u.TaskID, AttemptID: i.AttemptID, InteractionID: i.ID})
		if e != nil {
			if errors.Is(e, continuation.ErrInteractionSuspendInProgress) {
				return continuation.ErrInteractionSuspendInProgress
			}
			return domain.ErrNotAllowed
		}
		responseID := s.ids.NextID()
		if e = i.SaveAnswer(r.ExpectedVersion, responseID, r.Answers); e != nil {
			return e
		}
		response := domain.Response{ID: responseID, TenantID: u.TenantID, TaskID: u.TaskID, InteractionID: i.ID, PrincipalID: t.PrincipalID, IdempotencyKey: r.IdempotencyKey, Digest: digest, Answers: r.Answers, CreatedAt: time.Now().UTC()}
		if e = s.repo.InsertResponse(ctx, tx, response); e != nil {
			return e
		}
		if target != nil {
			if s.resumer == nil {
				return domain.ErrUnavailable
			}
			instruction, _ := json.Marshal(struct {
				Questions []domain.Question `json:"questions"`
				Answers   []domain.Answer   `json:"answers"`
			}{i.Questions, r.Answers})
			binding, _ := json.Marshal(struct {
				InteractionKey string          `json:"interactionKey"`
				ResponseID     int64           `json:"responseId,string"`
				Answers        []domain.Answer `json:"answers"`
			}{i.LocalRequestKey, responseID, r.Answers})
			resumed, e := s.resumer.ResumeInteractionTx(ctx, tx, continuation.InteractionResumeRequest{TenantID: u.TenantID, TaskID: u.TaskID, SourceTaskTurnID: target.SourceTaskTurnID, SourceAttemptID: i.AttemptID, InteractionID: i.ID, CheckpointID: target.CheckpointID, AnswerID: responseID, CreatorUserID: u.UserID, Instruction: string(instruction), InteractionResponse: binding})
			if e != nil {
				return domain.ErrNotAllowed
			}
			if resumed.TaskTurnID <= 0 {
				return domain.ErrUnavailable
			}
			i.ResumeTaskTurnID = resumed.TaskTurnID
			messages, e := s.repo.SavedSourceMessages(ctx, tx, u.TenantID, u.TaskID, target.SourceTaskTurnID, i.AttemptID)
			if e != nil {
				return e
			}
			for _, message := range messages {
				if e = message.CarryMessage(target.SourceTaskTurnID, i.AttemptID, resumed.TaskTurnID); e != nil {
					return e
				}
				if e = s.persist(ctx, tx, message, false); e != nil {
					return e
				}
			}
		}
		if e = s.persist(ctx, tx, i, false); e != nil {
			return e
		}
		out = receipt(i, false)
		return nil
	})
	if err == nil && rejected != nil {
		err = rejected
	}
	return
}
func (s *Service) PostMessage(ctx context.Context, u facade.User, r facade.MessageRequest) (out facade.Receipt, err error) {
	if e := domain.ValidateMessage(r.Text, r.IdempotencyKey); e != nil {
		return out, e
	}
	err = s.transaction(ctx, func(tx pgx.Tx) error {
		t, e := s.user(ctx, tx, u, facade.SendMessage)
		if e != nil {
			return e
		}
		digest := domain.Digest(r)
		key := "message:" + domain.Digest(struct {
			Actor int64
			Key   string
		}{t.PrincipalID, r.IdempotencyKey})
		old, e := s.repo.FindMessage(ctx, tx, u.TenantID, u.TaskID, t.PrincipalID, key)
		if e != nil {
			return e
		}
		if old != nil {
			if old.Digest != digest {
				return domain.ErrConflict
			}
			out = receipt(*old, true)
			return nil
		}
		i := domain.Interaction{ID: s.ids.NextID(), TenantID: t.TenantID, TaskID: t.TaskID, TaskTurnID: t.TaskTurnID, AttemptID: t.AttemptID, AdapterSessionGeneration: t.AdapterSessionGeneration, LocalRequestKey: key, PrincipalID: t.PrincipalID, Kind: domain.MessageKind, State: domain.Resolved, DeliveryState: domain.Saved, Version: 1, Digest: digest, Text: r.Text, CreatedAt: time.Now().UTC(), ExpiresAt: time.Now().UTC().Add(24 * time.Hour)}
		if !active(t) {
			i.AttemptID = 0
			i.AdapterSessionGeneration = 0
		}
		if e = s.persist(ctx, tx, i, true); e != nil {
			return e
		}
		out = receipt(i, false)
		return nil
	})
	return
}
func pageSize(n int) (int, error) {
	if n == 0 {
		return 20, nil
	}
	if n < 1 || n > 50 {
		return 0, domain.ErrInvalid
	}
	return n, nil
}
func (s *Service) Pull(ctx context.Context, m facade.Machine, limit int) (out []facade.Delivery, err error) {
	limit, err = pageSize(limit)
	if err != nil {
		return
	}
	out = []facade.Delivery{}
	err = s.transaction(ctx, func(tx pgx.Tx) error {
		t, e := s.machine(ctx, tx, m)
		if e != nil {
			return e
		}
		if !t.AcceptsNativeInput {
			return domain.ErrNotAllowed
		}
		items, e := s.repo.Pending(ctx, tx, t.TenantID, t.TaskID, t.TaskTurnID, t.AttemptID, t.AdapterSessionGeneration, limit)
		if e != nil {
			return e
		}
		for _, i := range items {
			if (i.Kind == domain.QuestionKind && !capable(t, "interaction.respond")) || (i.Kind == domain.MessageKind && !capable(t, "user.message")) {
				return domain.ErrNotAllowed
			}
			if i.Kind == domain.MessageKind && i.AttemptID == 0 {
				if e = i.BindMessage(t.TaskTurnID, t.AttemptID, t.AdapterSessionGeneration); e != nil {
					return e
				}
				if e = s.persist(ctx, tx, i, false); e != nil {
					return e
				}
			}
			cold := i.Kind == domain.QuestionKind && i.ResumeTaskTurnID == t.TaskTurnID
			if i.TaskID != t.TaskID || (i.TaskTurnID != t.TaskTurnID && !cold) {
				return domain.ErrFenced
			}
			round, generation := i.RoundKey, i.AdapterSessionGeneration
			if cold {
				round = ""
				generation = t.AdapterSessionGeneration
			}
			out = append(out, facade.Delivery{ColdResume: cold, LocalRequestKey: i.LocalRequestKey, RoundKey: round, AdapterSessionGeneration: generation, InteractionID: i.ID, ResponseID: i.ResponseID, Kind: i.Kind, Answers: i.Answers, Text: i.Text})
		}
		return nil
	})
	return
}
func (s *Service) Ack(ctx context.Context, m facade.Machine, r facade.AckRequest) (out facade.Receipt, err error) {
	if r.InteractionID <= 0 || r.ResponseID < 0 {
		return out, domain.ErrInvalid
	}
	err = s.transaction(ctx, func(tx pgx.Tx) error {
		t, e := s.machine(ctx, tx, m)
		if e != nil {
			return e
		}
		i, e := s.repo.Get(ctx, tx, t.TenantID, t.TaskID, r.InteractionID)
		if e != nil {
			return e
		}
		if (i.Kind == domain.QuestionKind && !capable(t, "interaction.received")) || (i.Kind == domain.MessageKind && !capable(t, "message.received")) {
			return domain.ErrNotAllowed
		}
		cold := i.Kind == domain.QuestionKind && i.ResumeTaskTurnID == t.TaskTurnID
		if !cold && (i.AttemptID != t.AttemptID || i.TaskTurnID != t.TaskTurnID || i.AdapterSessionGeneration != t.AdapterSessionGeneration) {
			return domain.ErrFenced
		}
		var replay bool
		if cold {
			replay, e = i.AcknowledgeCold(r.ResponseID, t.TaskTurnID)
		} else {
			replay, e = i.Acknowledge(r.ResponseID)
		}
		if e != nil {
			return e
		}
		if !replay {
			if e = s.persist(ctx, tx, i, false); e != nil {
				return e
			}
		}
		out = receipt(i, replay)
		return nil
	})
	return
}

// Query separates stable-ID history (Before/Cursor, newest first) from change
// replay (After/Changes, ascending event sequence). Cursor means Before for
// history; for changes feed the returned cursor back as After.
type Query struct {
	Cursor  string
	Before  int64
	After   int64
	Changes bool
	Limit   int
}

func (s *Service) Conversation(ctx context.Context, u facade.User, q Query) (out domain.Conversation, err error) {
	q.Limit, err = pageSize(q.Limit)
	if err != nil {
		return
	}
	if q.After < 0 || q.Before < 0 {
		return out, domain.ErrInvalid
	}
	if q.After > 0 {
		q.Changes = true
	}
	if q.Changes && (q.Before != 0 || q.Cursor != "") {
		return out, domain.ErrInvalid
	}
	if q.Cursor != "" {
		if q.Before != 0 {
			return out, domain.ErrInvalid
		}
		q.Before, err = strconv.ParseInt(q.Cursor, 10, 64)
		if err != nil || q.Before <= 0 || strconv.FormatInt(q.Before, 10) != q.Cursor {
			return out, domain.ErrInvalid
		}
	}
	err = s.transaction(ctx, func(tx pgx.Tx) error {
		t, e := s.user(ctx, tx, u, facade.Read)
		if e != nil {
			return e
		}
		if e = s.expire(ctx, tx, u.TenantID, u.TaskID); e != nil {
			return e
		}
		watermark, e := s.repo.Watermark(ctx, tx, u.TaskID)
		if e != nil {
			return e
		}
		var items []domain.Item
		if q.Changes {
			items, e = s.repo.Items(ctx, tx, u.TenantID, u.TaskID, q.After, q.Limit+1)
		} else {
			items, e = s.repo.History(ctx, tx, u.TenantID, u.TaskID, q.Before, q.Limit+1)
		}
		if e != nil {
			return e
		}
		out = domain.Conversation{Items: items, Watermark: watermark, AllowedActions: t.AllowedActions}
		if out.AllowedActions == nil {
			out.AllowedActions = []string{}
		}
		if len(items) > q.Limit {
			out.Items = items[:q.Limit]
			last := out.Items[q.Limit-1]
			cursor := last.ID
			if q.Changes {
				cursor = last.Sequence
				out.Watermark = cursor
			}
			value := strconv.FormatInt(cursor, 10)
			out.NextCursor = &value
		}
		if q.Changes && out.Watermark < q.After {
			out.Watermark = q.After
		}
		return nil
	})
	return
}
func (s *Service) MarkRead(ctx context.Context, u facade.User, watermark int64) error {
	if watermark < 0 {
		return domain.ErrInvalid
	}
	return s.transaction(ctx, func(tx pgx.Tx) error {
		t, e := s.user(ctx, tx, u, facade.Read)
		if e != nil {
			return e
		}
		if t.PrincipalID <= 0 {
			return domain.ErrUnavailable
		}
		current, e := s.repo.Watermark(ctx, tx, u.TaskID)
		if e != nil {
			return e
		}
		if watermark > current {
			return domain.ErrInvalid
		}
		old, e := s.repo.ReadState(ctx, tx, u.TenantID, u.TaskID, t.PrincipalID)
		if e != nil {
			return e
		}
		if watermark <= old {
			return nil
		}
		return s.repo.PutReadState(ctx, tx, s.ids.NextID(), u.TenantID, u.TaskID, t.PrincipalID, watermark)
	})
}
