package main

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"sort"
	"strconv"
	"sync"
	"sync/atomic"
	"time"

	attemptruntime "github.com/CaixyPromise/seven-agent/force/internal/app/workforce/attempt/runtime"
	executionaccessdomain "github.com/CaixyPromise/seven-agent/force/internal/app/workforce/executionaccess/domain"
	executionaccess "github.com/CaixyPromise/seven-agent/force/internal/app/workforce/executionaccess/facade"
	interactiondomain "github.com/CaixyPromise/seven-agent/force/internal/app/workforce/interaction/domain"
	interaction "github.com/CaixyPromise/seven-agent/force/internal/app/workforce/interaction/facade"
	taskdomain "github.com/CaixyPromise/seven-agent/force/internal/app/workforce/task/domain"
	"github.com/CaixyPromise/seven-agent/force/internal/worker/agentprotocol"
	"github.com/CaixyPromise/seven-agent/force/internal/worker/control"
	"github.com/CaixyPromise/seven-agent/force/internal/worker/interactionspool"
)

var errInteractionTransport = errors.New("WORKER_INTERACTION_TRANSPORT_FAILED")
var errInteractionBinding = errors.New("WORKER_INTERACTION_BINDING_INVALID")
var errInteractionRetry = errors.New("WORKER_INTERACTION_RETRY")

// Concrete production implementation is the authenticated Machine HTTP client.
// Tests substitute this boundary; no container transport can implement trust.
type interactionControlClient interface {
	PublishInteraction(context.Context, taskdomain.Attempt, int64, json.RawMessage) (control.InteractionPublication, error)
	PullInteractions(context.Context, taskdomain.Attempt, int64) ([]interaction.Delivery, error)
	AcknowledgeInteraction(context.Context, taskdomain.Attempt, int64, interaction.AckRequest) (interaction.Receipt, error)
}
type executionAccessControlClient interface {
	PullExecutionOperationResults(context.Context, taskdomain.Attempt, int64, int) ([]executionaccess.ToolResult, error)
	AckExecutionOperationResult(context.Context, taskdomain.Attempt, int64, executionaccess.ResultAckRequest) (executionaccess.Receipt, error)
	AckColdExecutionOperationResult(context.Context, taskdomain.Attempt, int64, int64, executionaccess.ResultAckRequest) (executionaccess.Receipt, error)
}
type interactionRuntimeChannel interface {
	State(context.Context) (attemptruntime.InteractionExecutionState, error)
	Notifications(context.Context, int64) (attemptruntime.InteractionBatch, error)
	ConfirmSpooled(context.Context, int64) error
	Send(context.Context, string, int64, string, json.RawMessage) error
}

type interactionTransportFailure struct {
	TaskID, AttemptID, Generation, OwnerEpoch int64
	Err                                       error
}
type interactionExecutionKey struct{ task, attempt, generation, owner int64 }
type interactionDropPermit struct {
	receipt interactionspool.Receipt
	proof   [32]byte
}

// One hub belongs to one Worker process. Its privateRoot MUST NOT be an Attempt
// mount. The parent must consume Errors and cancel/fail the corresponding runtime;
// logging alone is not failure handling. No registration is admitted after a fatal
// error. Each of at most 64 active registrations emits at most one buffered error.
type interactionTransportHub struct {
	replayMu  sync.Mutex
	ctx       context.Context
	cancel    context.CancelFunc
	client    interactionControlClient
	spool     *interactionspool.Spool
	mu        sync.Mutex
	active    map[interactionExecutionKey]*interactionPump
	permits   map[interactionspool.Receipt]interactionDropPermit
	errors    chan interactionTransportFailure
	firstErr  error
	closed    bool
	wg        sync.WaitGroup
	closeOnce sync.Once
	closeErr  error
	interval  time.Duration
}

func newInteractionTransportHub(ctx context.Context, privateRoot string, client interactionControlClient) (*interactionTransportHub, error) {
	if client == nil {
		return nil, errInteractionTransport
	}
	lifetime, cancel := context.WithCancel(ctx)
	h := &interactionTransportHub{ctx: lifetime, cancel: cancel, client: client, active: make(map[interactionExecutionKey]*interactionPump), permits: make(map[interactionspool.Receipt]interactionDropPermit), errors: make(chan interactionTransportFailure, 64), interval: 250 * time.Millisecond}
	spool, err := interactionspool.Open(privateRoot, interactionspool.Limits{MaxEntries: 4096, MaxBytes: 64 << 20, MaxFrameBytes: agentprotocol.MaxFrameBytes}, h.verifyServerAck)
	if err != nil {
		cancel()
		return nil, err
	}
	h.spool = spool
	context.AfterFunc(lifetime, func() { _ = h.Close() })
	return h, nil
}

func (h *interactionTransportHub) Errors() <-chan interactionTransportFailure { return h.errors }
func (h *interactionTransportHub) Err() error                                 { h.mu.Lock(); defer h.mu.Unlock(); return h.firstErr }

// Register is called only from an authority-approved started/reattached runtime.
// It replays only the exact owner epoch AND native session/round identity. Old
// owners remain on disk; any migration requires a separate explicit authority path.
func (h *interactionTransportHub) Register(ctx context.Context, taskID int64, attempt taskdomain.Attempt, channel interactionRuntimeChannel) (func(), error) {
	return h.register(ctx, taskID, attempt, channel, nil, 0, 0)
}

func (h *interactionTransportHub) RegisterColdResume(ctx context.Context, taskID int64, attempt taskdomain.Attempt, channel interactionRuntimeChannel, operationID, sourceSessionGeneration int64) (func(), error) {
	if operationID <= 0 || sourceSessionGeneration <= 0 {
		return nil, errInteractionBinding
	}
	return h.register(ctx, taskID, attempt, channel, nil, operationID, sourceSessionGeneration)
}

func (h *interactionTransportHub) register(ctx context.Context, taskID int64, attempt taskdomain.Attempt, channel interactionRuntimeChannel, replay *interactionReplayGrant, coldOperationID, sourceSessionGeneration int64) (func(), error) {
	if taskID <= 0 || attempt.ID <= 0 || attempt.Generation <= 0 || attempt.OwnerEpoch <= 0 || channel == nil || attempt.TaskID != taskID {
		return nil, errInteractionBinding
	}
	key := interactionExecutionKey{taskID, attempt.ID, attempt.Generation, attempt.OwnerEpoch}
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.closed || h.ctx.Err() != nil || h.firstErr != nil {
		return nil, errInteractionTransport
	}
	if len(h.active) >= 64 {
		return nil, errInteractionTransport
	}
	// Reject another owner registration for the same Attempt until cleanup joins.
	for active := range h.active {
		if active.task == taskID && active.attempt == attempt.ID {
			return nil, errInteractionBinding
		}
	}
	lifetime, cancel := context.WithCancel(ctx)
	p := &interactionPump{hub: h, key: key, attempt: attempt, channel: channel, ctx: lifetime, cancel: cancel, done: make(chan struct{}), sent: make(map[interactionDeliveryKey]string), toolSent: make(map[toolResultDeliveryKey]string), toolResolved: make(map[int64]bool)}
	p.coldOperationID, p.sourceSessionGeneration = coldOperationID, sourceSessionGeneration
	p.replay = replay
	p.stop = context.AfterFunc(h.ctx, cancel)
	h.active[key] = p
	h.wg.Add(1)
	go func() {
		defer h.wg.Done()
		defer close(p.done)
		defer p.stop()
		defer cancel()
		err := p.run()
		// Normal runtime completion drains retained notifications before the
		// parent's later ExecutionSession/container cleanup. Cancellation never
		// starts fresh work; already-persisted frames remain for strict replay.
		if err == nil && p.flush.Load() && ctx.Err() == nil && h.ctx.Err() == nil {
			p.drainMu.Lock()
			drainParent := p.drainParent
			p.drainMu.Unlock()
			if drainParent == nil {
				drainParent = ctx
			}
			drainCtx, drainCancel := context.WithTimeout(drainParent, 2*time.Second)
			stopDrain := context.AfterFunc(h.ctx, drainCancel)
			stopParent := context.AfterFunc(ctx, drainCancel)
			p.draining = true
			for page := 0; page < 64; page++ {
				err = p.step(drainCtx)
				if err != nil || !p.hasMore {
					break
				}
				if page == 63 {
					err = errInteractionTransport
				}
			}
			if err == nil {
				err = h.requireEmpty(drainCtx, key)
			}
			p.didDrain = err == nil
			stopDrain()
			stopParent()
			drainCancel()
			if err != nil {
				err = errInteractionTransport
			}
		}
		p.resultErr = err
		h.mu.Lock()
		// Retain the joined pump until the parent's cleanup so a suspension
		// drain followed by the final success gate can read the same result.
		if !p.flush.Load() {
			delete(h.active, key)
		}
		if err != nil && ctx.Err() == nil && h.ctx.Err() == nil {
			if h.firstErr == nil {
				h.firstErr = err
			}
			h.errors <- interactionTransportFailure{TaskID: taskID, AttemptID: attempt.ID, Generation: attempt.Generation, OwnerEpoch: attempt.OwnerEpoch, Err: err}
		}
		h.mu.Unlock()
	}()
	return func() {
		p.flush.Store(true)
		cancel()
		<-p.done
		h.mu.Lock()
		if h.active[key] == p {
			delete(h.active, key)
		}
		h.mu.Unlock()
	}, nil
}

func (h *interactionTransportHub) Close() error {
	h.closeOnce.Do(func() {
		h.mu.Lock()
		h.closed = true
		h.mu.Unlock()
		h.cancel()
		h.wg.Wait()
		h.closeErr = h.spool.Close()
		close(h.errors)
	})
	return h.closeErr
}

// Drain is the parent's pre-Finalizer success gate. Call it while the session
// and container are still alive, before deferred cleanup. It joins this pump,
// surfaces final publication/ACK errors, and rejects any unconfirmed records for
// the Attempt (including fenced identities that cannot be implicitly replayed).
func (h *interactionTransportHub) Drain(ctx context.Context, attempt taskdomain.Attempt) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if attempt.TaskID <= 0 || attempt.ID <= 0 || attempt.Generation <= 0 || attempt.OwnerEpoch <= 0 {
		return errInteractionBinding
	}
	key := interactionExecutionKey{attempt.TaskID, attempt.ID, attempt.Generation, attempt.OwnerEpoch}
	h.mu.Lock()
	p := h.active[key]
	firstErr := h.firstErr
	h.mu.Unlock()
	if p != nil {
		p.drainMu.Lock()
		if p.drainParent == nil {
			p.drainParent = ctx
		}
		p.drainMu.Unlock()
		p.flush.Store(true)
		p.cancel()
		select {
		case <-p.done:
			if p.resultErr != nil {
				return p.resultErr
			}
			if !p.didDrain {
				return errInteractionTransport
			}
		case <-ctx.Done():
			return ctx.Err()
		}
	} else {
		if firstErr != nil {
			return firstErr
		}
		return errInteractionTransport
	}
	return h.requireEmpty(ctx, key)
}

func (h *interactionTransportHub) requireEmpty(ctx context.Context, key interactionExecutionKey) error {
	entries, err := h.spool.Pending(ctx)
	if err != nil {
		return err
	}
	for _, entry := range entries {
		identity := entry.Key.Identity
		if identity.TaskID == key.task && identity.AttemptID == key.attempt {
			return errInteractionTransport
		}
	}
	return nil
}

func (h *interactionTransportHub) verifyServerAck(ctx context.Context, r interactionspool.Receipt, proof []byte) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	permit, ok := h.permits[r]
	if !ok || permit.receipt != r || subtle.ConstantTimeCompare(proof, permit.proof[:]) != 1 {
		return interactionspool.ErrUnauthorized
	}
	return nil
}

// The only minting point follows a successful authenticated HTTP result and
// exact raw-frame digest readback. The transient capability never enters disk,
// a container frame, logs or public configuration.
func (h *interactionTransportHub) dropPublished(ctx context.Context, r interactionspool.Receipt, result control.InteractionPublication) error {
	if result.FrameSHA256 != r.SHA256 {
		return errInteractionBinding
	}
	permit := interactionDropPermit{receipt: r}
	if _, err := rand.Read(permit.proof[:]); err != nil {
		return errInteractionTransport
	}
	h.mu.Lock()
	h.permits[r] = permit
	h.mu.Unlock()
	defer func() { h.mu.Lock(); delete(h.permits, r); h.mu.Unlock() }()
	return h.spool.AcknowledgeServer(ctx, r, permit.proof[:])
}

type interactionDeliveryKey struct {
	interaction, response int64
	round                 string
	session               int64
}
type toolResultDeliveryKey struct {
	operation int64
	round     string
	local     string
	native    string
}
type interactionPump struct {
	replay              *interactionReplayGrant
	hub                 *interactionTransportHub
	key                 interactionExecutionKey
	attempt             taskdomain.Attempt
	channel             interactionRuntimeChannel
	ctx                 context.Context
	cancel              context.CancelFunc
	stop                func() bool
	done                chan struct{}
	after, localThrough int64
	gapThrough          int64
	seen                bool
	sent                map[interactionDeliveryKey]string
	available           []interaction.Delivery
	completed           map[interactionDeliveryKey]bool
	toolSent            map[toolResultDeliveryKey]string
	// toolResolved contains native calls whose result was accepted by the
	// Adapter but for which no later public execution activity has yet proved
	// that the Agent continued. It is intentionally process-local: the Server's
	// NATIVE_CALL_RESOLVED state is the durable readback after reconnect.
	toolResolved map[int64]bool
	// A cold-resumed Operation is delivered through the successor Adapter
	// session, but its receipt remains bound to the source session generation.
	// Keep that authority separate from hot-path toolResolved entries.
	coldOperationID, sourceSessionGeneration int64
	flush                                    atomic.Bool
	draining, hasMore                        bool
	drainMu                                  sync.Mutex
	drainParent                              context.Context
	resultErr                                error // read only after done closes
	didDrain                                 bool  // read only after done closes
}

func (p *interactionPump) run() error {
	retries := 0
	for {
		if p.ctx.Err() != nil {
			return nil
		}
		ctx, cancel := context.WithTimeout(p.ctx, 10*time.Second)
		err := p.step(ctx)
		cancel()
		if err != nil {
			if p.ctx.Err() != nil {
				return nil
			}
			if !errors.Is(err, errInteractionRetry) {
				return err
			}
			retries++
			if retries >= 8 {
				return errInteractionTransport
			}
		} else {
			retries = 0
		}
		delay := p.hub.interval
		if retries > 0 {
			delay *= time.Duration(retries)
		}
		timer := time.NewTimer(delay)
		select {
		case <-p.ctx.Done():
			timer.Stop()
			return nil
		case <-timer.C:
		}
	}
}

func interactionRemoteError(err error) error {
	if err == nil {
		return nil
	}
	var api *control.APIError
	if errors.As(err, &api) && api.StatusCode < 500 && api.StatusCode != 429 {
		// Preserve only the Server's stable reason code for operator diagnosis.
		// The arbitrary response message is intentionally discarded.
		if code := safeInteractionRemoteReason(api.ReasonCode); code != "" {
			return fmt.Errorf("%w: %s", errInteractionBinding, code)
		}
		return errInteractionBinding
	}
	return errInteractionRetry
}

func safeInteractionRemoteReason(value string) string {
	if len(value) < 1 || len(value) > 128 {
		return ""
	}
	for _, r := range value {
		if (r < 'A' || r > 'Z') && (r < '0' || r > '9') && r != '_' {
			return ""
		}
	}
	return value
}
func interactionChannelError(err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF) || errors.Is(err, context.DeadlineExceeded) {
		return errInteractionRetry
	}
	return errInteractionTransport
}

// interactionStageError keeps operator diagnostics useful without allowing a
// channel, Adapter, or remote response body to cross the safe error boundary.
// Callers may still classify the root sentinel with errors.Is.
func interactionStageError(stage string, err error) error {
	if err == nil {
		return nil
	}
	return fmt.Errorf("%w: %s", interactionChannelError(err), stage)
}

func (p *interactionPump) step(ctx context.Context) error {
	state, err := p.channel.State(ctx)
	if err != nil {
		return interactionChannelError(err)
	}
	if state.RoundKey == "" && state.SessionGeneration == 0 {
		return nil
	}
	identity := interactionspool.Identity{TaskID: p.key.task, AttemptID: p.key.attempt, Generation: p.key.generation, OwnerEpoch: p.key.owner, CurrentRound: state.RoundKey, SessionGeneration: state.SessionGeneration}
	if state.RoundKey == "" || state.SessionGeneration <= 0 {
		return errInteractionBinding
	}
	if p.replay != nil && (state.RoundKey != p.replay.binding.RoundKey || state.SessionGeneration != p.replay.binding.AdapterSessionGeneration) {
		// Never carry an unfinished old occurrence into a different native round.
		if err := p.replay.requireComplete(ctx, p.hub); err != nil {
			return err
		}
		p.replay = nil
	}
	if p.localThrough > p.after {
		if err = p.channel.ConfirmSpooled(ctx, p.localThrough); err != nil {
			return interactionChannelError(err)
		}
		p.after = p.localThrough
	}
	batch, err := p.channel.Notifications(ctx, p.after)
	if err != nil {
		return interactionChannelError(err)
	}
	if len(batch.Notifications) > 128 || batch.Through < p.after {
		return errInteractionBinding
	}
	p.hasMore = batch.HasMore
	if batch.ActivityDroppedThrough < 0 || (batch.ActivityGap && batch.ActivityDroppedThrough <= p.after) {
		return errInteractionBinding
	}
	last := p.after
	batchBytes := 0
	for index, n := range batch.Notifications {
		knownActivityGap := batch.ActivityGap && n.Sequence-1 <= batch.ActivityDroppedThrough
		if n.Sequence <= last || (n.Sequence != last+1 && (p.seen || index > 0) && !knownActivityGap) || n.SessionGeneration != state.SessionGeneration || n.RoundKey == "" {
			return errInteractionBinding
		}
		// Marshal once to the exact representation Machine HTTP will embed; these
		// immutable bytes, not a later parsed/reconstructed frame, are persisted.
		frame, err := json.Marshal(n.Frame)
		if err != nil {
			return errInteractionBinding
		}
		batchBytes += len(frame)
		if len(frame) > agentprotocol.MaxFrameBytes || batchBytes > 64<<20 {
			return errInteractionBinding
		}
		decoded, err := agentprotocol.Decode(append(append([]byte(nil), frame...), '\n'), "up")
		if err != nil {
			return errInteractionBinding
		}
		var binding struct {
			RoundKey string `json:"roundKey"`
		}
		if json.Unmarshal(decoded.Payload, &binding) != nil {
			return errInteractionBinding
		}
		switch decoded.Method {
		case "interaction.request", "interaction.received", "interaction.closed", "message.received", "execution.suspended", "tool.catalog", "tool.request", "tool.received", "tool.closed":
			if binding.RoundKey != n.RoundKey {
				return errInteractionBinding
			}
		case "message", "plan", "activity":
			if binding.RoundKey != "" && binding.RoundKey != n.RoundKey {
				return errInteractionBinding
			}
		default:
			return errInteractionBinding
		}
		id := identity
		id.CurrentRound = n.RoundKey
		occurrence := strconv.FormatInt(n.Sequence, 10)
		retained, replayErr := p.replay.matchesNotification(occurrence, frame)
		if replayErr != nil {
			return replayErr
		}
		if !retained {
			if _, err = p.hub.spool.Put(ctx, interactionspool.Key{Identity: id, Occurrence: occurrence}, frame); err != nil {
				return err
			}
		}
		last = n.Sequence
	}
	if len(batch.Notifications) > 0 && batch.Through != last {
		return errInteractionBinding
	}
	if len(batch.Notifications) == 0 && batch.Through > p.after && p.seen {
		return errInteractionBinding
	}
	if batch.ActivityGap && batch.ActivityDroppedThrough > p.gapThrough {
		key := "gap:" + strconv.FormatInt(batch.ActivityDroppedThrough, 10)
		payload, _ := json.Marshal(map[string]any{"roundKey": state.RoundKey, "text": "部分早期活动已合并"})
		frame, _ := json.Marshal(agentprotocol.Frame{Protocol: agentprotocol.Version, Method: "activity", Key: key, Payload: payload})
		if _, err = p.hub.spool.Put(ctx, interactionspool.Key{Identity: identity, Occurrence: key}, frame); err != nil {
			return err
		}
		p.gapThrough = batch.ActivityDroppedThrough
	}
	p.localThrough = batch.Through
	if len(batch.Notifications) > 0 {
		if err = p.channel.ConfirmSpooled(ctx, p.localThrough); err != nil {
			return interactionChannelError(err)
		}
	}
	p.after = batch.Through
	p.seen = true
	var deliveries []interaction.Delivery
	var toolResults []executionaccess.ToolResult
	if !p.draining && state.State == "RUNNING" {
		deliveries, err = p.hub.client.PullInteractions(ctx, p.attempt, state.SessionGeneration)
		if err != nil {
			var api *control.APIError
			// No permission to pull new work is not permission to send anything.
			// Keep draining previously started observations/receipts during phase
			// transitions; owner/session/cancel fencing remains a fatal error.
			if !errors.As(err, &api) || api.ReasonCode != interactiondomain.ErrNotAllowed.Error() {
				return interactionRemoteError(err)
			}
			deliveries = nil
		}
		if len(deliveries) > 50 {
			return errInteractionBinding
		}
		if err == nil {
			p.available = deliveries
		}
		if access, ok := p.hub.client.(executionAccessControlClient); ok {
			toolResults, err = access.PullExecutionOperationResults(ctx, p.attempt, state.SessionGeneration, 20)
			if err != nil {
				var api *control.APIError
				if !errors.As(err, &api) || (api.ReasonCode != executionaccessdomain.ErrActionNotAllowed.Error() && api.ReasonCode != executionaccessdomain.ErrNotFound.Error()) {
					return interactionRemoteError(err)
				}
				toolResults = nil
			}
		}
	}
	// Replay has no implicit owner or round migration; stale records stay private.
	entries, err := p.hub.spool.Replay(ctx, identity)
	if err != nil {
		return err
	}
	if p.replay != nil {
		old, replayErr := p.replay.entries(ctx, p.hub)
		if replayErr != nil {
			return replayErr
		}
		entries = append(entries, old...)
	}
	if len(entries) > 64 {
		p.hasMore = true
	}
	sort.Slice(entries, func(i, j int) bool {
		left, _ := strconv.ParseInt(entries[i].Receipt.Key.Occurrence, 10, 64)
		right, _ := strconv.ParseInt(entries[j].Receipt.Key.Occurrence, 10, 64)
		return left < right
	})
	for index, entry := range entries {
		if index == 64 {
			break
		} // bound one Server drain before checking cancellation/state
		if err = p.publish(ctx, entry); err != nil {
			return err
		}
	}
	for _, delivery := range deliveries {
		if p.draining {
			break
		}
		if err = p.deliver(ctx, state, delivery); err != nil {
			if errors.Is(err, attemptruntime.ErrInteractionPausing) {
				// Prepare can win after the state poll. Nothing was written or
				// acknowledged; retain this delivery for the next RUNNING poll.
				return nil
			}
			return err
		}
	}
	for _, result := range toolResults {
		if p.draining {
			break
		}
		if err = p.deliverToolResult(ctx, state, result); err != nil {
			return err
		}
	}
	return nil
}

func (p *interactionPump) publish(ctx context.Context, entry interactionspool.Entry) error {
	frame, err := agentprotocol.Decode(append(append([]byte(nil), entry.Frame...), '\n'), "up")
	if err != nil {
		return errInteractionBinding
	}
	result, err := p.hub.client.PublishInteraction(ctx, p.attempt, entry.Receipt.Key.Identity.SessionGeneration, json.RawMessage(entry.Frame))
	if err != nil {
		return interactionRemoteError(err)
	}
	if result.FrameSHA256 != entry.Receipt.SHA256 {
		return errInteractionBinding
	}
	switch frame.Method {
	case "interaction.request", "interaction.closed":
		// Saved describes a user's answer/delivery, not question publication.
		// A newly opened question correctly has Saved=false.
		if result.Receipt.InteractionID <= 0 {
			return errInteractionBinding
		}
	case "interaction.received", "message.received":
		var binding struct {
			RoundKey       string `json:"roundKey"`
			ResponseID     string `json:"responseId"`
			Status         string `json:"status"`
			InteractionKey string `json:"interactionKey"`
			MessageKey     string `json:"messageKey"`
		}
		if json.Unmarshal(frame.Payload, &binding) != nil || binding.RoundKey != entry.Receipt.Key.Identity.CurrentRound {
			return errInteractionBinding
		}
		if binding.Status == "RECEIVED" {
			if result.Receipt.InteractionID <= 0 {
				return errInteractionBinding
			}
			response := int64(0)
			if frame.Method == "interaction.received" {
				response, err = strconv.ParseInt(binding.ResponseID, 10, 64)
				if err != nil || response <= 0 || result.Receipt.ResponseID != response {
					return errInteractionBinding
				}
			}
			// A repeatable DELIVERED Server readback permits recovery after ACK
			// succeeded but its response was lost. New ACKs require an exact
			// native binding to an authenticated pending Server delivery.
			matched := result.Receipt.Delivered
			for _, delivery := range p.available {
				round := delivery.RoundKey
				if (delivery.Kind == interactiondomain.MessageKind || delivery.ColdResume) && round == "" {
					round = entry.Receipt.Key.Identity.CurrentRound
				}
				localKey := binding.InteractionKey
				kind := interactiondomain.QuestionKind
				if frame.Method == "message.received" {
					localKey = binding.MessageKey
					kind = interactiondomain.MessageKind
				}
				if delivery.InteractionID == result.Receipt.InteractionID && delivery.ResponseID == response && delivery.Kind == kind && delivery.LocalRequestKey == localKey && round == binding.RoundKey && delivery.AdapterSessionGeneration == entry.Receipt.Key.Identity.SessionGeneration {
					matched = true
				}
			}
			if !matched {
				return errInteractionBinding
			}
			ack, err := p.hub.client.AcknowledgeInteraction(ctx, p.attempt, entry.Receipt.Key.Identity.SessionGeneration, interaction.AckRequest{InteractionID: result.Receipt.InteractionID, ResponseID: response})
			if err != nil {
				return interactionRemoteError(err)
			}
			if ack.InteractionID != result.Receipt.InteractionID || ack.ResponseID != response || !ack.Delivered {
				return errInteractionBinding
			}
			if p.completed == nil {
				p.completed = make(map[interactionDeliveryKey]bool)
			}
			if len(p.completed) >= 4096 {
				return errInteractionTransport
			}
			p.completed[interactionDeliveryKey{result.Receipt.InteractionID, response, binding.RoundKey, entry.Receipt.Key.Identity.SessionGeneration}] = true
		}
	case "execution.suspended", "message", "plan", "activity", "tool.catalog":
		// A stored suspension observation is not continuation/checkpoint authority.
		if !result.Receipt.Saved {
			return errInteractionBinding
		}
	case "tool.request":
		if result.DeliveryIntent.IntentID > 0 {
			immediate := result.ImmediateToolResult
			if result.DeliveryIntent.Version <= 0 || immediate.IntentID != result.DeliveryIntent.IntentID || immediate.Status != "SCHEDULED" || immediate.RoundKey != entry.Receipt.Key.Identity.CurrentRound || immediate.LocalToolCallKey == "" || immediate.NativeToolCallID == "" {
				return errInteractionBinding
			}
			body, marshalErr := json.Marshal(agentprotocol.ToolResultPayload{RoundKey: immediate.RoundKey, LocalToolCallKey: immediate.LocalToolCallKey, NativeToolCallID: immediate.NativeToolCallID, OperationID: immediate.IntentID, Status: immediate.Status, Summary: immediate.Summary, ResultDigest: immediate.ResultDigest})
			if marshalErr != nil || agentprotocol.ValidateToolPayload("tool.result", body) != nil {
				return errInteractionBinding
			}
			key := toolResultDeliveryKey{operation: immediate.IntentID, round: immediate.RoundKey, local: immediate.LocalToolCallKey, native: immediate.NativeToolCallID}
			if previous, sent := p.toolSent[key]; sent && previous != string(body) {
				return errInteractionBinding
			} else if !sent {
				if sendErr := p.channel.Send(ctx, immediate.RoundKey, entry.Receipt.Key.Identity.SessionGeneration, "tool.result", body); sendErr != nil {
					return interactionStageError("DELIVERY_INTENT_RESULT_SEND", sendErr)
				}
				p.toolSent[key] = string(body)
			}
		} else if result.Operation.OperationID <= 0 || result.Operation.Version <= 0 {
			return errInteractionBinding
		}
	case "tool.received":
		var binding agentprotocol.ToolReceiptPayload
		if json.Unmarshal(frame.Payload, &binding) != nil || binding.Status != "RECEIVED" {
			return errInteractionBinding
		}
		if result.DeliveryIntent.IntentID > 0 {
			if result.DeliveryIntent.IntentID != binding.OperationID || result.DeliveryIntent.Status == "" {
				return errInteractionBinding
			}
			break
		}
		if result.Operation.OperationID != binding.OperationID || result.Operation.Status == "" {
			return errInteractionBinding
		}
		access, ok := p.hub.client.(executionAccessControlClient)
		if !ok {
			return errInteractionTransport
		}
		receipt, ackErr := access.AckExecutionOperationResult(ctx, p.attempt, entry.Receipt.Key.Identity.SessionGeneration, executionaccess.ResultAckRequest{OperationID: binding.OperationID, NativeResolved: true})
		if ackErr != nil {
			return interactionRemoteError(ackErr)
		}
		if receipt.OperationID != binding.OperationID || receipt.Status == "" {
			return errInteractionBinding
		}
		if p.toolResolved == nil {
			p.toolResolved = make(map[int64]bool)
		}
		p.toolResolved[binding.OperationID] = true
	case "tool.closed":
		if result.DeliveryIntent.IntentID > 0 {
			if result.DeliveryIntent.Status == "" {
				return errInteractionBinding
			}
		} else if result.Operation.OperationID <= 0 || result.Operation.Status == "" {
			return errInteractionBinding
		}
	default:
		return errInteractionBinding
	}
	if (frame.Method == "message" || frame.Method == "plan" || frame.Method == "activity") && len(p.toolResolved) > 0 {
		if err = p.markAgentContinued(ctx, entry.Receipt.Key.Identity.SessionGeneration); err != nil {
			return err
		}
	}
	return p.hub.dropPublished(ctx, entry.Receipt, result)
}

// markAgentContinued advances only after a later public execution event has
// itself been durably persisted. Native tool resolution and subsequent Agent
// activity are separate facts; neither the Adapter receipt nor the Web client
// may infer the latter.
func (p *interactionPump) markAgentContinued(ctx context.Context, sessionGeneration int64) error {
	access, ok := p.hub.client.(executionAccessControlClient)
	if !ok {
		return errInteractionTransport
	}
	if p.coldOperationID > 0 {
		receipt, err := access.AckColdExecutionOperationResult(ctx, p.attempt, p.sourceSessionGeneration, p.coldOperationID, executionaccess.ResultAckRequest{OperationID: p.coldOperationID, NativeResolved: true, AgentContinued: true})
		if err != nil {
			return interactionRemoteError(err)
		}
		if receipt.OperationID != p.coldOperationID || receipt.Status == "" {
			return errInteractionBinding
		}
		p.coldOperationID = 0
	}
	ids := make([]int64, 0, len(p.toolResolved))
	for id := range p.toolResolved {
		ids = append(ids, id)
	}
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
	for _, id := range ids {
		receipt, err := access.AckExecutionOperationResult(ctx, p.attempt, sessionGeneration, executionaccess.ResultAckRequest{OperationID: id, NativeResolved: true, AgentContinued: true})
		if err != nil {
			return interactionRemoteError(err)
		}
		if receipt.OperationID != id || receipt.Status == "" {
			return errInteractionBinding
		}
		delete(p.toolResolved, id)
	}
	return nil
}

func (p *interactionPump) deliverToolResult(ctx context.Context, state attemptruntime.InteractionExecutionState, result executionaccess.ToolResult) error {
	if state.State != "RUNNING" || result.OperationID <= 0 || result.AdapterSessionGeneration != state.SessionGeneration || result.RoundKey != state.RoundKey {
		return errInteractionBinding
	}
	body, err := marshalOperationToolResult(result)
	if err != nil {
		return errInteractionBinding
	}
	key := toolResultDeliveryKey{operation: result.OperationID, round: result.RoundKey, local: result.LocalToolCallKey, native: result.NativeToolCallID}
	previous, sent := p.toolSent[key]
	if sent && previous != string(body) {
		return errInteractionBinding
	}
	if !sent {
		if err = p.channel.Send(ctx, result.RoundKey, state.SessionGeneration, "tool.result", body); err != nil {
			return interactionStageError("TOOL_RESULT_SEND", err)
		}
		p.toolSent[key] = string(body)
	}
	access, ok := p.hub.client.(executionAccessControlClient)
	if !ok {
		return errInteractionTransport
	}
	receipt, err := access.AckExecutionOperationResult(ctx, p.attempt, state.SessionGeneration, executionaccess.ResultAckRequest{OperationID: result.OperationID})
	if err != nil {
		return fmt.Errorf("%w: TOOL_RESULT_SAVED_ACK", interactionRemoteError(err))
	}
	if receipt.OperationID != result.OperationID || receipt.Status == "" {
		return errInteractionBinding
	}
	return nil
}

func (p *interactionPump) deliver(ctx context.Context, state attemptruntime.InteractionExecutionState, d interaction.Delivery) error {
	if state.State != "RUNNING" {
		return nil
	}
	if d.InteractionID <= 0 || d.AdapterSessionGeneration != state.SessionGeneration {
		return errInteractionBinding
	}
	round := d.RoundKey
	if (d.Kind == interactiondomain.MessageKind || d.ColdResume) && round == "" {
		round = state.RoundKey
	}
	if round != state.RoundKey {
		return errInteractionBinding
	}
	var payload any
	method := ""
	switch d.Kind {
	case interactiondomain.QuestionKind:
		if d.ResponseID <= 0 {
			return errInteractionBinding
		}
		answers := make([]agentprotocol.Answer, 0, len(d.Answers))
		for _, answer := range d.Answers {
			selected := append([]string{}, answer.SelectedOptionKeys...)
			answers = append(answers, agentprotocol.Answer{QuestionKey: answer.QuestionKey, SelectedOptionKeys: selected, Text: answer.Text})
		}
		method = "interaction.respond"
		payload = agentprotocol.InteractionAnswer{RoundKey: round, InteractionKey: d.LocalRequestKey, ResponseID: strconv.FormatInt(d.ResponseID, 10), Answers: answers}
	case interactiondomain.MessageKind:
		if d.ResponseID != 0 {
			return errInteractionBinding
		}
		if state.Waiting {
			return nil
		}
		method = "user.message"
		payload = agentprotocol.UserMessage{RoundKey: round, MessageKey: d.LocalRequestKey, Text: d.Text}
	default:
		return errInteractionBinding
	}
	body, err := json.Marshal(payload)
	if err != nil || agentprotocol.ValidateInteractionPayload(method, body) != nil {
		return errInteractionBinding
	}
	key := interactionDeliveryKey{d.InteractionID, d.ResponseID, round, state.SessionGeneration}
	if p.completed[key] {
		return nil
	}
	if previous, ok := p.sent[key]; ok {
		if previous != string(body) {
			return errInteractionBinding
		}
		return nil
	}
	if len(p.sent) >= 4096 {
		return errInteractionTransport
	}
	if err = p.channel.Send(ctx, round, state.SessionGeneration, method, body); err != nil {
		if errors.Is(err, attemptruntime.ErrInteractionPausing) {
			return attemptruntime.ErrInteractionPausing
		}
		return interactionChannelError(err)
	}
	p.sent[key] = string(body)
	// WRITTEN is deliberately not AcknowledgeInteraction. Native receipt handling
	// above is the only path to Server delivery ACK.
	return nil
}
