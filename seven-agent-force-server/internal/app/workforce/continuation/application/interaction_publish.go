package application

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"github.com/CaixyPromise/seven-agent/force/internal/app/workforce/continuation/domain"
	base "github.com/CaixyPromise/seven-agent/force/internal/app/workforce/resultprocessor/checkpoint"
	workercheckpoint "github.com/CaixyPromise/seven-agent/force/internal/worker/checkpoint"
	"github.com/jackc/pgx/v5"
	"io"
	"time"
)

type InteractionSuspendOccurrence struct{ TenantID, TaskID, TaskTurnID, AttemptID, InteractionID, AdapterSessionGeneration int64 }
type InteractionSuspendOccurrenceGate interface {
	AuthorizeInteractionSuspendTx(context.Context, pgx.Tx, InteractionSuspendOccurrence) error
}
type OperationSuspendOccurrence struct{ TenantID, TaskID, TaskTurnID, AttemptID, OperationID, AdapterSessionGeneration int64 }
type OperationSuspendOccurrenceGate interface {
	AuthorizeOperationSuspendTx(context.Context, pgx.Tx, OperationSuspendOccurrence) error
}
type ReserveInteractionRequest struct {
	InteractionCaptureRequest
	AdapterSessionGeneration int64  `json:"adapterSessionGeneration,string"`
	SessionDigest            string `json:"sessionDigest"`
	CurrentRound             int64  `json:"currentRound,string"`
	SuspendControlKey        string `json:"suspendControlKey"`
}
type InteractionReservation struct {
	ID                       int64       `json:"id,string"`
	Identity                 string      `json:"identity"`
	SnapshotDigest           string      `json:"snapshotDigest"`
	SessionDigest            string      `json:"sessionDigest"`
	AdapterSessionGeneration int64       `json:"adapterSessionGeneration,string"`
	AutomaticRevisionCount   int         `json:"automaticRevisionCount"`
	Envelope                 KeyEnvelope `json:"envelope"`
	CurrentRound             int64       `json:"currentRound,string"`
	SuspendControlKeySHA256  string      `json:"suspendControlKeySha256"`
}
type InteractionReservationResult struct {
	Disposition string                       `json:"disposition"`
	Reservation InteractionReservation       `json:"reservation"`
	Key         CheckpointKeyMaterialization `json:"key"`
}
type InteractionEncryptedBlob struct {
	ObjectKey string `json:"objectKey"`
	SHA256    string `json:"sha256"`
	SizeBytes int64  `json:"sizeBytes"`
}
type PublishInteractionRequest struct {
	InteractionCaptureRequest
	Bundle   InteractionEncryptedBlob `json:"bundle"`
	Overlay  InteractionEncryptedBlob `json:"overlay"`
	Session  InteractionEncryptedBlob `json:"session"`
	Manifest InteractionEncryptedBlob `json:"manifest"`
}
type InteractionSuspendRepository interface {
	BeginInteractionTx(context.Context) (pgx.Tx, error)
	LockInteractionCaptureSourceTx(context.Context, pgx.Tx, ReserveInteractionRequest) (string, int, error)
	ReserveInteractionTx(context.Context, pgx.Tx, ReserveInteractionRequest, InteractionReservation) (InteractionReservation, error)
	AuthorizeInteractionCaptureTx(context.Context, pgx.Tx, InteractionCaptureRequest) error
	AuthorizeInteractionPublishTx(context.Context, pgx.Tx, InteractionCaptureRequest) error
	LoadInteractionReservationTx(context.Context, pgx.Tx, InteractionCaptureRequest) (InteractionReservation, error)
	PublishInteractionTx(context.Context, pgx.Tx, PublishInteractionRequest, workercheckpoint.Manifest) error
	LoadInteractionBudgetBaselineTx(context.Context, pgx.Tx, int64) (int64, *workercheckpoint.ExecutionBudget, error)
}
type InteractionObjectReader interface {
	OpenInteractionObject(context.Context, string) (io.ReadCloser, error)
}
type InteractionObjectReaderFunc func(context.Context, string) (io.ReadCloser, error)

func (f InteractionObjectReaderFunc) OpenInteractionObject(c context.Context, k string) (io.ReadCloser, error) {
	return f(c, k)
}

type InteractionSuspendService struct {
	repository  InteractionSuspendRepository
	keys        KeyWrapper
	objects     InteractionObjectReader
	occurrences InteractionSuspendOccurrenceGate
	operations  OperationSuspendOccurrenceGate
}

func NewInteractionSuspend(repository InteractionSuspendRepository, keys KeyWrapper, objects InteractionObjectReader, gate InteractionSuspendOccurrenceGate) *InteractionSuspendService {
	return &InteractionSuspendService{repository: repository, keys: keys, objects: objects, occurrences: gate}
}

func (s *InteractionSuspendService) WithOperationSuspendGate(gate OperationSuspendOccurrenceGate) *InteractionSuspendService {
	s.operations = gate
	return s
}

func (s *InteractionSuspendService) authorizeOccurrence(ctx context.Context, tx pgx.Tx, r InteractionCaptureRequest, sessionGeneration int64) error {
	if (r.InteractionID > 0) == (r.OperationID > 0) {
		return domain.ErrInteractionSuspend
	}
	if r.InteractionID > 0 {
		return s.occurrences.AuthorizeInteractionSuspendTx(ctx, tx, InteractionSuspendOccurrence{r.TenantID, r.TaskID, r.TaskTurnID, r.AttemptID, r.InteractionID, sessionGeneration})
	}
	if s.operations == nil {
		return domain.ErrInteractionSuspend
	}
	return s.operations.AuthorizeOperationSuspendTx(ctx, tx, OperationSuspendOccurrence{r.TenantID, r.TaskID, r.TaskTurnID, r.AttemptID, r.OperationID, sessionGeneration})
}

func (s *InteractionSuspendService) ReserveInteractionSuspend(ctx context.Context, r ReserveInteractionRequest) (InteractionReservationResult, error) {
	if s == nil || s.repository == nil || s.keys == nil || s.occurrences == nil || len(r.SessionDigest) > 128 || r.AdapterSessionGeneration <= 0 || r.CurrentRound <= 0 || r.SuspendControlKey == "" || len(r.SuspendControlKey) > 256 {
		return InteractionReservationResult{}, domain.ErrInteractionSuspend
	}
	tx, err := s.repository.BeginInteractionTx(ctx)
	if err != nil {
		return InteractionReservationResult{}, err
	}
	defer tx.Rollback(ctx)
	snapshot, budget, err := s.repository.LockInteractionCaptureSourceTx(ctx, tx, r)
	if err != nil {
		return InteractionReservationResult{}, err
	}
	if err = s.authorizeOccurrence(ctx, tx, r.InteractionCaptureRequest, r.AdapterSessionGeneration); err != nil {
		if errors.Is(err, domain.ErrInteractionSuspendAnswerWon) {
			return InteractionReservationResult{Disposition: "ANSWER_WON"}, nil
		}
		return InteractionReservationResult{}, err
	}
	controlSum := sha256.Sum256([]byte(r.SuspendControlKey))
	identity := domain.InteractionCheckpointIdentity(r.TaskID, r.TaskTurnID, r.AttemptID, r.Generation, r.InteractionID, r.AdapterSessionGeneration, r.CurrentRound, snapshot, hex.EncodeToString(controlSum[:]))
	if r.OperationID > 0 {
		identity = domain.OperationCheckpointIdentity(r.TaskID, r.TaskTurnID, r.AttemptID, r.Generation, r.OperationID, r.AdapterSessionGeneration, r.CurrentRound, snapshot, hex.EncodeToString(controlSum[:]))
	}
	key := make([]byte, 32)
	defer clear(key)
	if _, err = rand.Read(key); err != nil {
		return InteractionReservationResult{}, err
	}
	envelope, err := s.keys.Wrap(ctx, key)
	if err != nil {
		return InteractionReservationResult{}, err
	}
	record, err := s.repository.ReserveInteractionTx(ctx, tx, r, InteractionReservation{Identity: identity, SnapshotDigest: snapshot, SessionDigest: r.SessionDigest, AdapterSessionGeneration: r.AdapterSessionGeneration, AutomaticRevisionCount: budget, Envelope: envelope, CurrentRound: r.CurrentRound, SuspendControlKeySHA256: hex.EncodeToString(controlSum[:])})
	if err != nil {
		return InteractionReservationResult{}, err
	}
	// Replay uses the original envelope, never a fresh encryption identity.
	plain, err := s.keys.Unwrap(ctx, record.Envelope)
	if err != nil || len(plain) != 32 {
		return InteractionReservationResult{}, domain.ErrInteractionSuspend
	}
	defer clear(plain)
	if err = tx.Commit(ctx); err != nil {
		return InteractionReservationResult{}, err
	}
	return InteractionReservationResult{Disposition: "RESERVED", Reservation: record, Key: CheckpointKeyMaterialization{Algorithm: "AES-256-GCM-CHUNKED", PlainKeyBase64: base64.StdEncoding.EncodeToString(plain), Envelope: record.Envelope, AADIdentity: record.Identity}}, nil
}

func (s *InteractionSuspendService) PublishInteractionSuspend(ctx context.Context, r PublishInteractionRequest) error {
	if s == nil || s.repository == nil || s.keys == nil || s.objects == nil || s.occurrences == nil {
		return domain.ErrInteractionSuspend
	}
	tx, err := s.repository.BeginInteractionTx(ctx)
	if err != nil {
		return err
	}
	if err = s.repository.AuthorizeInteractionPublishTx(ctx, tx, r.InteractionCaptureRequest); err != nil {
		_ = tx.Rollback(ctx)
		return err
	}
	record, err := s.repository.LoadInteractionReservationTx(ctx, tx, r.InteractionCaptureRequest)
	_ = tx.Rollback(ctx)
	if err != nil {
		return err
	}
	key, err := s.keys.Unwrap(ctx, record.Envelope)
	if err != nil || len(key) != 32 {
		return domain.ErrInteractionSuspend
	}
	defer clear(key)
	p := base.Input{TaskID: r.TaskID, TaskTurnID: r.TaskTurnID, AttemptID: r.AttemptID}
	bk, ok, sk, mk := base.ObjectKeysV2(p, record.Identity)
	if r.Bundle.ObjectKey != bk || r.Overlay.ObjectKey != ok || r.Session.ObjectKey != sk || r.Manifest.ObjectKey != mk {
		return domain.ErrInteractionSuspend
	}
	body, _, err := s.verifyInteractionBlob(ctx, r.Manifest, key, record.Identity, "manifest", 1<<20, true)
	if err != nil {
		return err
	}
	m, err := workercheckpoint.DecodeManifest(body)
	if err != nil {
		return err
	}
	canonical, err := json.Marshal(m)
	if err != nil || !bytes.Equal(body, canonical) {
		return domain.ErrInteractionSuspend
	}
	if m.CheckpointIdentity != record.Identity || m.TaskID != r.TaskID || m.TaskTurnID != r.TaskTurnID || m.AttemptID != r.AttemptID || m.Generation != r.Generation || m.InteractionID != r.InteractionID || m.OperationID != r.OperationID || m.SnapshotDigest != record.SnapshotDigest || (record.SessionDigest != "" && m.SessionDigest != record.SessionDigest) || m.AdapterSessionGeneration != record.AdapterSessionGeneration || m.AutomaticRevisionCount != record.AutomaticRevisionCount || m.CurrentRound != record.CurrentRound || m.SuspendControlKeySHA256 != record.SuspendControlKeySHA256 || m.ExecutionBudget.OwnerEpoch != r.OwnerEpoch {
		return domain.ErrInteractionSuspend
	}
	for _, b := range []struct {
		blob  InteractionEncryptedBlob
		kind  string
		plain workercheckpoint.Blob
		limit int64
	}{{r.Bundle, "repository-bundle", m.RepositoryBundle, base.MaxArchiveBytes}, {r.Overlay, "workspace-overlay", m.WorkspaceOverlay, base.MaxArchiveBytes}, {r.Session, "session", m.Session, base.MaxSessionArchiveBytes}} {
		_, actual, e := s.verifyInteractionBlob(ctx, b.blob, key, record.Identity, b.kind, b.limit, false)
		if e != nil {
			return e
		}
		if actual != b.plain {
			return domain.ErrInteractionSuspend
		}
	}
	// Object-store work is outside transactions. Reauthorize under Task lock
	// after readback; cancellation or owner replacement wins before publication.
	tx, err = s.repository.BeginInteractionTx(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	if err = s.repository.AuthorizeInteractionPublishTx(ctx, tx, r.InteractionCaptureRequest); err != nil {
		return err
	}
	limit, previous, err := s.repository.LoadInteractionBudgetBaselineTx(ctx, tx, r.TaskID)
	if err != nil {
		return err
	}
	budget := m.ExecutionBudget
	if budget.ActiveLimitMillis != limit || (limit > 0 && (budget.ActiveElapsedMillis >= limit || budget.AbsoluteDeadlineUnixMillis <= time.Now().UnixMilli())) {
		return domain.ErrInteractionSuspend
	}
	if previous != nil && (budget.ActiveElapsedMillis < previous.ActiveElapsedMillis || budget.InteractionWaitingMillis < previous.InteractionWaitingMillis || budget.AbsoluteDeadlineUnixMillis != previous.AbsoluteDeadlineUnixMillis || budget.ActiveLimitMillis != previous.ActiveLimitMillis) {
		return domain.ErrInteractionSuspend
	}
	if err = s.authorizeOccurrence(ctx, tx, r.InteractionCaptureRequest, record.AdapterSessionGeneration); err != nil {
		return err
	}
	if err = s.repository.PublishInteractionTx(ctx, tx, r, m); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

type suspendBoundedWriter struct {
	destination io.Writer
	remaining   int64
	written     int64
}

func (w *suspendBoundedWriter) Write(p []byte) (int, error) {
	if int64(len(p)) > w.remaining {
		return 0, domain.ErrInteractionSuspend
	}
	n, e := w.destination.Write(p)
	w.remaining -= int64(n)
	w.written += int64(n)
	return n, e
}
func (s *InteractionSuspendService) verifyInteractionBlob(ctx context.Context, b InteractionEncryptedBlob, key []byte, identity, kind string, limit int64, retain bool) ([]byte, workercheckpoint.Blob, error) {
	if b.SizeBytes <= 0 || b.SizeBytes > limit+limit/16+4096 || len(b.SHA256) != 64 {
		return nil, workercheckpoint.Blob{}, domain.ErrInteractionSuspend
	}
	r, err := s.objects.OpenInteractionObject(ctx, b.ObjectKey)
	if err != nil {
		return nil, workercheckpoint.Blob{}, domain.ErrInteractionSuspend
	}
	defer r.Close()
	cipherHash, plainHash := sha256.New(), sha256.New()
	var body bytes.Buffer
	var dest io.Writer = plainHash
	if retain {
		dest = io.MultiWriter(plainHash, &body)
	}
	out := &suspendBoundedWriter{destination: dest, remaining: limit}
	limited := &io.LimitedReader{R: io.TeeReader(r, cipherHash), N: b.SizeBytes + 1}
	if err = base.DecryptStream(limited, out, key, []byte("seven.continuation\x00"+identity+"\x00"+kind)); err != nil {
		return nil, workercheckpoint.Blob{}, domain.ErrInteractionSuspend
	}
	if limited.N != 1 || hex.EncodeToString(cipherHash.Sum(nil)) != b.SHA256 {
		return nil, workercheckpoint.Blob{}, domain.ErrInteractionSuspend
	}
	if err = r.Close(); err != nil {
		return nil, workercheckpoint.Blob{}, domain.ErrInteractionSuspend
	}
	return body.Bytes(), workercheckpoint.Blob{SHA256: hex.EncodeToString(plainHash.Sum(nil)), SizeBytes: out.written}, nil
}
