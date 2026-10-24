package runtime

import (
	"context"
	"encoding/json"
	"errors"
	"strconv"
	"sync"
	"time"
)

// InteractionNotification is an untrusted Adapter observation. A local receipt
// only acknowledges Worker spool persistence, never a Server business decision.
type InteractionNotification struct {
	Sequence          int64           `json:"sequence,string"`
	SessionGeneration int64           `json:"sessionGeneration,string"`
	RoundKey          string          `json:"roundKey"`
	Frame             json.RawMessage `json:"frame"`
}

type InteractionBatch struct {
	ActivityGap            bool
	ActivityDroppedThrough int64
	Notifications          []InteractionNotification `json:"notifications"`
	Through                int64                     `json:"through,string"`
	HasMore                bool                      `json:"hasMore"`
}

// ExecutionChannel restricts interaction callers to one authority-bound runtime.
// It cannot start a Harness or choose a different container.
type ExecutionChannel struct {
	session   *ExecutionSession
	container string
}

type InteractionExecutionState struct {
	SuspendControlKey        string
	PendingInteractions      []string
	PendingTools             []string
	State                    string
	ActiveElapsedMillis      int64
	InteractionWaitingMillis int64
	RoundKey                 string
	SessionGeneration        int64
	Waiting                  bool
}

func (c *ExecutionChannel) State(ctx context.Context) (InteractionExecutionState, error) {
	if c == nil || c.session == nil {
		return InteractionExecutionState{}, errors.New("INTERACTION_CHANNEL_INVALID")
	}
	r, err := c.session.Call(ctx, c.container, c.envelope("state"))
	if err != nil {
		return InteractionExecutionState{}, err
	}
	if !r.OK {
		return InteractionExecutionState{}, errors.New("INTERACTION_CHANNEL_REJECTED")
	}
	v := InteractionExecutionState{State: r.State.State, ActiveElapsedMillis: r.State.ActiveElapsedMillis, InteractionWaitingMillis: r.State.InteractionWaitingMillis, RoundKey: r.State.CurrentRound, SessionGeneration: r.State.SessionGeneration, Waiting: r.State.InteractionWaiting && (len(r.State.PendingInteractions) > 0 || len(r.State.PendingTools) > 0) && r.State.InteractionActive}
	v.SuspendControlKey, v.PendingInteractions, v.PendingTools = r.State.SuspendControlKey, append([]string(nil), r.State.PendingInteractions...), append([]string(nil), r.State.PendingTools...)
	if b, _ := ctx.Value(interactionBudgetKey{}).(*InteractionBudget); b != nil {
		b.observe(v)
	}
	return v, nil
}

const InteractionWaitAllowance = 10 * time.Minute
const InteractionCaptureAllowance = 2 * time.Minute

type interactionBudgetKey struct{}

// InteractionBudget is Worker-local accounting, not container-reported authority.
// A fresh bound question observation permits bounded waiting; failed polling
// expires that permission. The same ledger covers baseline and all Agent rounds.
type InteractionBudget struct {
	mu                                      sync.Mutex
	now                                     func() time.Time
	started, last, observed, captureStarted time.Time
	limit, active, waiting                  time.Duration
	wait                                    bool
	state                                   string
	identity                                BudgetIdentity
	absolute                                time.Time
	questionRound                           string
	questionSession                         int64
	questionUntil                           time.Time
	captureElapsed                          time.Duration
}

type BudgetIdentity struct {
	TaskID     int64 `json:"taskId,string"`
	AttemptID  int64 `json:"attemptId,string"`
	Generation int64 `json:"generation,string"`
	OwnerEpoch int64 `json:"ownerEpoch,string"`
}

type ExecutionBudgetSnapshot struct {
	Version int `json:"version"`
	BudgetIdentity
	ActiveLimitMillis          int64 `json:"activeLimitMillis,string"`
	ActiveElapsedMillis        int64 `json:"activeElapsedMillis,string"`
	InteractionWaitingMillis   int64 `json:"interactionWaitingMillis,string"`
	AbsoluteDeadlineUnixMillis int64 `json:"absoluteDeadlineUnixMillis,string"`
}

var ErrBudgetRecovery = errors.New("BUDGET_RECOVERY_EVIDENCE_REQUIRED")
var ErrInteractionPausing = errors.New("SUPERVISOR_PAUSING")

func BudgetSnapshot(ctx context.Context) (ExecutionBudgetSnapshot, error) {
	b, ok := ctx.Value(interactionBudgetKey{}).(*InteractionBudget)
	if !ok {
		return ExecutionBudgetSnapshot{}, ErrBudgetRecovery
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	b.charge(b.now())
	deadlineMillis := int64(0)
	if !b.absolute.IsZero() {
		deadlineMillis = b.absolute.UnixMilli()
	}
	return ExecutionBudgetSnapshot{Version: 1, BudgetIdentity: b.identity, ActiveLimitMillis: b.limit.Milliseconds(), ActiveElapsedMillis: b.active.Milliseconds(), InteractionWaitingMillis: b.waiting.Milliseconds(), AbsoluteDeadlineUnixMillis: deadlineMillis}, nil
}

// BudgetRestoreProvider is a trusted Worker port. Implementations must verify
// private record identity or Server-verified checkpoint metadata and authorize
// source-to-target lineage before returning the target-bound snapshot. Never
// implement this by copying supervisor state or an arbitrary WorkItem payload.
type BudgetRestoreProvider func(context.Context, BudgetIdentity) (ExecutionBudgetSnapshot, error)
type budgetRestoreKey struct{}

func WithBudgetRestoreProvider(ctx context.Context, p BudgetRestoreProvider) context.Context {
	return context.WithValue(ctx, budgetRestoreKey{}, p)
}

func ValidateBudgetSnapshot(s ExecutionBudgetSnapshot, identity BudgetIdentity, limit time.Duration, now time.Time) error {
	if s.Version != 1 || s.BudgetIdentity != identity || identity.TaskID <= 0 || identity.AttemptID <= 0 || identity.Generation <= 0 || s.ActiveLimitMillis != limit.Milliseconds() || s.ActiveElapsedMillis < 0 || s.InteractionWaitingMillis < 0 || s.InteractionWaitingMillis > InteractionWaitAllowance.Milliseconds() {
		return ErrBudgetRecovery
	}
	if limit == 0 {
		if s.AbsoluteDeadlineUnixMillis != 0 {
			return ErrBudgetRecovery
		}
		return nil
	}
	if limit < 0 || s.ActiveElapsedMillis >= s.ActiveLimitMillis || s.AbsoluteDeadlineUnixMillis <= now.UnixMilli() || s.AbsoluteDeadlineUnixMillis > now.Add(limit+InteractionWaitAllowance+InteractionCaptureAllowance).UnixMilli() {
		return ErrBudgetRecovery
	}
	remaining := limit - time.Duration(s.ActiveElapsedMillis)*time.Millisecond + InteractionWaitAllowance - time.Duration(s.InteractionWaitingMillis)*time.Millisecond + InteractionCaptureAllowance
	if s.AbsoluteDeadlineUnixMillis > now.Add(remaining).UnixMilli() {
		return ErrBudgetRecovery
	}
	return nil
}

// ExecutionBudgetContext is used before baseline, and on reattach. A restored
// ledger preserves used active/wait and the original absolute deadline.
func ExecutionBudgetContext(ctx context.Context, limit time.Duration, identity BudgetIdentity, restore bool) (context.Context, context.CancelFunc, error) {
	if limit < 0 {
		return nil, nil, ErrBudgetRecovery
	}
	if b, ok := ctx.Value(interactionBudgetKey{}).(*InteractionBudget); ok {
		b.mu.Lock()
		matched := b.identity == identity && b.limit == limit
		b.mu.Unlock()
		if !matched {
			return nil, nil, ErrBudgetRecovery
		}
		child, cancel := context.WithCancel(ctx)
		return child, cancel, nil
	}
	if !restore {
		child, cancel := WithInteractionBudget(ctx, limit)
		b := child.Value(interactionBudgetKey{}).(*InteractionBudget)
		b.mu.Lock()
		b.identity = identity
		b.mu.Unlock()
		return child, cancel, nil
	}
	p, ok := ctx.Value(budgetRestoreKey{}).(BudgetRestoreProvider)
	if !ok || p == nil {
		return nil, nil, ErrBudgetRecovery
	}
	s, err := p(ctx, identity)
	if err != nil {
		return nil, nil, err
	}
	now := time.Now()
	if err := ValidateBudgetSnapshot(s, identity, limit, now); err != nil {
		return nil, nil, err
	}
	var absolute time.Time
	if s.AbsoluteDeadlineUnixMillis > 0 {
		absolute = time.UnixMilli(s.AbsoluteDeadlineUnixMillis)
	}
	b := &InteractionBudget{now: time.Now, started: now, last: now, limit: limit, identity: identity, absolute: absolute, active: time.Duration(s.ActiveElapsedMillis) * time.Millisecond, waiting: time.Duration(s.InteractionWaitingMillis) * time.Millisecond}
	return startInteractionBudget(ctx, b)
}

// ExecutionBudgetContextFromElapsed starts a replacement Attempt from the
// Server-confirmed Task floor. It is never used for a checkpoint restore,
// which carries a stronger source-to-target budget proof.
func ExecutionBudgetContextFromElapsed(ctx context.Context, limit time.Duration, identity BudgetIdentity, elapsed time.Duration) (context.Context, context.CancelFunc, error) {
	if limit < 0 || elapsed < 0 || (limit > 0 && elapsed >= limit) || identity.TaskID <= 0 || identity.AttemptID <= 0 || identity.Generation <= 0 || identity.OwnerEpoch <= 0 {
		return nil, nil, ErrBudgetRecovery
	}
	now := time.Now()
	b := &InteractionBudget{now: time.Now, started: now, last: now, limit: limit, identity: identity, active: elapsed}
	if limit > 0 {
		b.absolute = now.Add(limit - elapsed + InteractionWaitAllowance + InteractionCaptureAllowance)
	}
	return startInteractionBudget(ctx, b)
}

func (b *InteractionBudget) charge(now time.Time) {
	d := now.Sub(b.last)
	if d <= 0 {
		return
	}
	if b.state == "PAUSING_PREPARE" || b.state == "PAUSING" || b.state == "INTERACTION_SUSPENDED" {
		b.captureElapsed += d
		b.last = now
		return
	}
	pause := time.Duration(0)
	if b.wait && b.observed.Add(time.Second).After(b.last) {
		end := now
		if end.After(b.observed.Add(time.Second)) {
			end = b.observed.Add(time.Second)
		}
		pause = end.Sub(b.last)
		if pause > InteractionWaitAllowance-b.waiting {
			pause = InteractionWaitAllowance - b.waiting
		}
	}
	b.waiting += pause
	b.active += d - pause
	b.last = now
}

func (b *InteractionBudget) observe(s InteractionExecutionState) {
	b.mu.Lock()
	defer b.mu.Unlock()
	now := b.now()
	b.charge(now)
	b.wait = s.Waiting && s.RoundKey != "" && s.SessionGeneration > 0 && s.ActiveElapsedMillis >= 0 && s.InteractionWaitingMillis >= 0 && b.questionRound == s.RoundKey && b.questionSession == s.SessionGeneration && now.Before(b.questionUntil)
	b.observed = now
	b.state = s.State
	if (s.State == "PAUSING_PREPARE" || s.State == "PAUSING" || s.State == "INTERACTION_SUSPENDED") && b.captureStarted.IsZero() {
		b.captureStarted = now
	}
}

// ConfirmBudgetQuestion is called only after the Worker finds the same local
// question in the authenticated Server pending-question response. State alone
// cannot create waiting credit; the permission expires on polling failure.
func ConfirmBudgetQuestion(ctx context.Context, round string, session int64) {
	b, ok := ctx.Value(interactionBudgetKey{}).(*InteractionBudget)
	if !ok {
		return
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	b.questionRound, b.questionSession, b.questionUntil = round, session, b.now().Add(time.Second)
}

func (b *InteractionBudget) expired() bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	now := b.now()
	b.charge(now)
	if !b.absolute.IsZero() && !now.Before(b.absolute) {
		return true
	}
	if !b.captureStarted.IsZero() {
		if b.captureElapsed >= InteractionCaptureAllowance {
			return true
		}
		if b.state == "PAUSING_PREPARE" || b.state == "PAUSING" || b.state == "INTERACTION_SUSPENDED" {
			return false
		}
	}
	return b.limit > 0 && b.active >= b.limit
}

// WithInteractionBudget reuses existing accounting, so nested runtime entry
// cannot restart the active clock. Recovery requires that same trusted ledger.
func WithInteractionBudget(ctx context.Context, limit time.Duration) (context.Context, context.CancelFunc) {
	if _, ok := ctx.Value(interactionBudgetKey{}).(*InteractionBudget); ok {
		return context.WithCancel(ctx)
	}
	now := time.Now()
	b := &InteractionBudget{now: time.Now, started: now, last: now, limit: limit}
	if limit > 0 {
		b.absolute = now.Add(limit + InteractionWaitAllowance + InteractionCaptureAllowance)
	}
	child, cancel, _ := startInteractionBudget(ctx, b)
	return child, cancel
}

func startInteractionBudget(ctx context.Context, b *InteractionBudget) (context.Context, context.CancelFunc, error) {
	ctx = context.WithValue(ctx, interactionBudgetKey{}, b)
	deadlineCtx := ctx
	stopDeadline := func() {}
	if !b.absolute.IsZero() {
		deadlineCtx, stopDeadline = context.WithDeadline(ctx, b.absolute)
	}
	causeCtx, stopCause := context.WithCancelCause(deadlineCtx)
	ctx = activeBudgetContext{causeCtx}
	cancel := func() { stopCause(context.Canceled); stopDeadline() }
	go func() {
		t := time.NewTicker(100 * time.Millisecond)
		defer t.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-t.C:
				if b.expired() {
					stopCause(context.DeadlineExceeded)
					stopDeadline()
					return
				}
			}
		}
	}()
	return ctx, cancel, nil
}

type activeBudgetContext struct{ context.Context }

func (c activeBudgetContext) Err() error {
	if c.Context.Err() != nil && errors.Is(context.Cause(c.Context), context.DeadlineExceeded) {
		return context.DeadlineExceeded
	}
	return c.Context.Err()
}

func watchInteractionBudget(ctx context.Context, channel *ExecutionChannel) context.CancelFunc {
	ctx, cancel := context.WithCancel(ctx)
	go func() {
		t := time.NewTicker(250 * time.Millisecond)
		defer t.Stop()
		for {
			probe, stop := context.WithTimeout(ctx, 500*time.Millisecond)
			_, _ = channel.State(probe)
			stop()
			select {
			case <-ctx.Done():
				return
			case <-t.C:
			}
		}
	}()
	return cancel
}

func (c *ExecutionChannel) envelope(command string) map[string]any {
	s := c.session
	return map[string]any{"schemaVersion": 2, "command": command, "taskId": strconv.FormatInt(s.taskID, 10), "attemptId": strconv.FormatInt(s.attemptID, 10), "generation": strconv.FormatInt(s.generation, 10), "ownerEpoch": strconv.FormatInt(s.ownerEpoch, 10)}
}

func (c *ExecutionChannel) Notifications(ctx context.Context, after int64) (InteractionBatch, error) {
	if c == nil || c.session == nil || after < 0 {
		return InteractionBatch{}, errors.New("INTERACTION_CHANNEL_INVALID")
	}
	req := c.envelope("notifications")
	req["after"] = strconv.FormatInt(after, 10)
	r, err := c.session.Call(ctx, c.container, req)
	if err != nil {
		return InteractionBatch{}, err
	}
	if !r.OK {
		return InteractionBatch{}, errors.New("INTERACTION_CHANNEL_REJECTED")
	}
	return InteractionBatch{Notifications: r.Notifications, Through: r.Through, HasMore: r.HasMore, ActivityGap: r.ActivityGap, ActivityDroppedThrough: r.ActivityDroppedThrough}, nil
}

func (c *ExecutionChannel) ConfirmSpooled(ctx context.Context, through int64) error {
	if c == nil || c.session == nil || through < 0 {
		return errors.New("INTERACTION_CHANNEL_INVALID")
	}
	req := c.envelope("notifications-received")
	req["through"] = strconv.FormatInt(through, 10)
	r, err := c.session.Call(ctx, c.container, req)
	if err != nil {
		return err
	}
	if !r.OK {
		return errors.New("INTERACTION_CHANNEL_REJECTED")
	}
	return nil
}

func (c *ExecutionChannel) Send(ctx context.Context, round string, sessionGeneration int64, method string, payload json.RawMessage) error {
	if c == nil || c.session == nil || round == "" || sessionGeneration <= 0 || !json.Valid(payload) {
		return errors.New("INTERACTION_CHANNEL_INVALID")
	}
	switch method {
	case "interaction.respond", "user.message", "tool.result", "execution.suspend":
	default:
		return errors.New("INTERACTION_CONTROL_UNSUPPORTED")
	}
	req := c.envelope("agent-control")
	req["currentRound"] = round
	req["sessionGeneration"] = strconv.FormatInt(sessionGeneration, 10)
	req["method"] = method
	req["payload"] = payload
	r, err := c.session.Call(ctx, c.container, req)
	if err != nil {
		return err
	}
	if !r.OK {
		if r.ReasonCode == "SUPERVISOR_PAUSING" {
			return ErrInteractionPausing
		}
		return errors.New("INTERACTION_CHANNEL_REJECTED")
	}
	return nil
}
