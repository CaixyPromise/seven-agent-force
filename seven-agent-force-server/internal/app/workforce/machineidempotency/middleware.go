package machineidempotency

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/cloudwego/hertz/pkg/app"
	"github.com/jackc/pgx/v5/pgxpool"

	registryapp "github.com/CaixyPromise/seven-agent/force/internal/app/workforce/workerregistry/application"
	apperrors "github.com/CaixyPromise/seven-agent/force/internal/common/errors"
	"github.com/CaixyPromise/seven-agent/force/internal/common/response"
	secretvalue "github.com/CaixyPromise/seven-agent/force/internal/infrastructure/security/crypto/secretvalue"
)

type Middleware struct {
	db       *pgxpool.Pool
	registry *registryapp.Service
	secrets  secretvalue.Service
	now      func() time.Time
}

func New(db *pgxpool.Pool, registry *registryapp.Service, secrets secretvalue.Service) *Middleware {
	return &Middleware{db: db, registry: registry, secrets: secrets, now: time.Now}
}

type replayRecord struct {
	status       string
	digest       string
	responseCode int
	encrypted    []byte
	createdAt    time.Time
}

func (m *Middleware) Wrap(normalizedRoute string, ttl time.Duration, next app.HandlerFunc) app.HandlerFunc {
	return func(ctx context.Context, c *app.RequestContext) {
		if m == nil || m.db == nil || m.registry == nil || m.secrets == nil {
			writeError(c, http.StatusServiceUnavailable, "MACHINE_IDEMPOTENCY_UNAVAILABLE", "machine idempotency is unavailable")
			return
		}
		key := strings.TrimSpace(string(c.Request.Header.Peek("Idempotency-Key")))
		if key == "" || len(key) > 128 {
			writeError(c, http.StatusBadRequest, "IDEMPOTENCY_KEY_INVALID", "Idempotency-Key is required and must not exceed 128 characters")
			return
		}
		body := append([]byte(nil), c.Request.Body()...)
		workerID := machineWorkerID(c, body)
		auth := strings.TrimSpace(string(c.Request.Header.Peek("Authorization")))
		if workerID == 0 || !strings.HasPrefix(auth, "Bearer ") || m.registry.Authenticate(ctx, workerID, strings.TrimSpace(strings.TrimPrefix(auth, "Bearer "))) != nil {
			writeError(c, http.StatusUnauthorized, "WORKER_UNAUTHORIZED", "worker credential is invalid")
			return
		}
		digest, err := requestDigest(workerID, string(c.Request.Method()), string(c.Request.URI().Path()), body)
		if err != nil {
			writeError(c, http.StatusBadRequest, "INVALID_JSON", "request body is invalid")
			return
		}
		record, inserted, err := m.begin(ctx, workerID, string(c.Request.Method()), normalizedRoute, key, digest, ttl)
		if err != nil {
			writeError(c, http.StatusServiceUnavailable, "MACHINE_IDEMPOTENCY_UNAVAILABLE", "machine idempotency could not be persisted")
			return
		}
		if !inserted {
			if record.digest != digest {
				writeError(c, http.StatusConflict, "IDEMPOTENCY_CONFLICT", "Idempotency-Key was already used for a different request")
				return
			}
			if record.status == "COMPLETED" {
				plain, decryptErr := m.decrypt(ctx, record.encrypted)
				if decryptErr != nil {
					writeError(c, http.StatusServiceUnavailable, "IDEMPOTENCY_REPLAY_UNAVAILABLE", "stored response could not be replayed")
					return
				}
				c.Response.Header.Set("Idempotency-Replayed", "true")
				c.Response.SetStatusCode(record.responseCode)
				c.Response.SetBody(plain)
				return
			}
			if m.now().Sub(record.createdAt) < time.Minute {
				c.Response.Header.Set("Retry-After", "1")
				writeError(c, http.StatusConflict, "MACHINE_REQUEST_IN_PROGRESS", "an identical machine request is still in progress")
				return
			}
			tag, takeoverErr := m.db.Exec(ctx, `UPDATE workforce_machine_idempotency_records SET created_at=CURRENT_TIMESTAMP,expires_at=$6 WHERE worker_id=$1 AND http_method=$2 AND normalized_route=$3 AND idempotency_key=$4 AND request_digest=$5 AND status='PROCESSING' AND created_at=$7`, workerID, string(c.Request.Method()), normalizedRoute, key, digest, m.now().Add(ttl), record.createdAt)
			if takeoverErr != nil {
				writeError(c, http.StatusServiceUnavailable, "MACHINE_IDEMPOTENCY_UNAVAILABLE", "stale machine request could not be resumed")
				return
			}
			if tag.RowsAffected() != 1 {
				c.Response.Header.Set("Retry-After", "1")
				writeError(c, http.StatusConflict, "MACHINE_REQUEST_IN_PROGRESS", "an identical machine request is being resumed")
				return
			}
		}
		next(ctx, c)
		responseBody := append([]byte(nil), c.Response.Body()...)
		if !replayableResponseStatus(c.Response.StatusCode()) {
			// A 5xx response represents an uncommitted or uncertain handler failure,
			// not a stable business outcome. Remove the PROCESSING row so the same
			// business idempotency key can execute again immediately. Endpoint-level
			// state-machine idempotency remains responsible for reconstructing any
			// business write that committed before the response failed.
			if err = m.abandon(ctx, workerID, string(c.Request.Method()), normalizedRoute, key, digest); err != nil {
				writeError(c, http.StatusServiceUnavailable, "MACHINE_IDEMPOTENCY_PERSIST_FAILED", "failed machine response could not be released for retry")
			}
			return
		}
		if err = m.complete(ctx, workerID, string(c.Request.Method()), normalizedRoute, key, digest, c.Response.StatusCode(), responseBody, ttl); err != nil {
			// The business handler may already have committed. Returning a stable
			// failure forces the Worker to retry with the same key; endpoint-level
			// state-machine idempotency then reconstructs the response.
			writeError(c, http.StatusServiceUnavailable, "MACHINE_IDEMPOTENCY_PERSIST_FAILED", "machine response could not be made replayable")
		}
	}
}

func replayableResponseStatus(status int) bool {
	return status > 0 && status < http.StatusInternalServerError
}

func (m *Middleware) abandon(ctx context.Context, worker int64, method, route, key, digest string) error {
	tag, err := m.db.Exec(ctx, `DELETE FROM workforce_machine_idempotency_records WHERE worker_id=$1 AND http_method=$2 AND normalized_route=$3 AND idempotency_key=$4 AND request_digest=$5 AND status='PROCESSING'`, worker, method, route, key, digest)
	if err != nil {
		return err
	}
	if tag.RowsAffected() != 1 {
		return errors.New("machine idempotency failure record was not released")
	}
	return nil
}

func (m *Middleware) begin(ctx context.Context, worker int64, method, route, key, digest string, ttl time.Duration) (replayRecord, bool, error) {
	tag, err := m.db.Exec(ctx, `INSERT INTO workforce_machine_idempotency_records (worker_id,http_method,normalized_route,idempotency_key,request_digest,status,expires_at) VALUES ($1,$2,$3,$4,$5,'PROCESSING',$6) ON CONFLICT DO NOTHING`, worker, method, route, key, digest, m.now().Add(ttl))
	if err != nil {
		return replayRecord{}, false, err
	}
	if tag.RowsAffected() == 1 {
		return replayRecord{}, true, nil
	}
	var r replayRecord
	err = m.db.QueryRow(ctx, `SELECT request_digest,status,COALESCE(response_status,0),COALESCE(encrypted_response_blob,''::bytea),created_at FROM workforce_machine_idempotency_records WHERE worker_id=$1 AND http_method=$2 AND normalized_route=$3 AND idempotency_key=$4`, worker, method, route, key).Scan(&r.digest, &r.status, &r.responseCode, &r.encrypted, &r.createdAt)
	return r, false, err
}

func (m *Middleware) complete(ctx context.Context, worker int64, method, route, key, digest string, status int, body []byte, ttl time.Duration) error {
	secret, err := m.secrets.EncryptBytes(ctx, body)
	if err != nil {
		return err
	}
	encrypted, err := json.Marshal(secret)
	if err != nil {
		return err
	}
	tag, err := m.db.Exec(ctx, `UPDATE workforce_machine_idempotency_records SET status='COMPLETED',response_status=$6,safe_response_json=$7,encrypted_response_blob=$8,completed_at=CURRENT_TIMESTAMP,expires_at=$9 WHERE worker_id=$1 AND http_method=$2 AND normalized_route=$3 AND idempotency_key=$4 AND request_digest=$5 AND status='PROCESSING'`, worker, method, route, key, digest, status, json.RawMessage(`{"replayable":true}`), encrypted, m.now().Add(ttl))
	if err != nil {
		return err
	}
	if tag.RowsAffected() != 1 {
		return errors.New("machine idempotency record was not completed")
	}
	return nil
}

func (m *Middleware) decrypt(ctx context.Context, body []byte) ([]byte, error) {
	var secret secretvalue.SecretValue
	if len(body) == 0 || json.Unmarshal(body, &secret) != nil {
		return nil, errors.New("encrypted replay response is invalid")
	}
	return m.secrets.DecryptBytes(ctx, secret)
}

func requestDigest(worker int64, method, path string, body []byte) (string, error) {
	canonical := body
	if len(body) > 0 {
		var value any
		decoder := json.NewDecoder(io.LimitReader(bytes.NewReader(body), 2<<20))
		decoder.UseNumber()
		if err := decoder.Decode(&value); err != nil {
			return "", err
		}
		var trailing any
		if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
			return "", errors.New("trailing JSON")
		}
		canonical, _ = json.Marshal(value)
	}
	sum := sha256.Sum256(bytes.Join([][]byte{[]byte(strconv.FormatInt(worker, 10)), []byte(method), []byte(path), canonical}, []byte{0}))
	return hex.EncodeToString(sum[:]), nil
}

func machineWorkerID(c *app.RequestContext, body []byte) int64 {
	if value := strings.TrimSpace(string(c.Request.Header.Peek("X-Seven-Worker-ID"))); value != "" {
		parsed, err := strconv.ParseInt(value, 10, 64)
		if err == nil && parsed > 0 {
			return parsed
		}
		return 0
	}
	var envelope struct {
		WorkerID int64 `json:"workerId,string"`
	}
	_ = json.Unmarshal(body, &envelope)
	if envelope.WorkerID != 0 {
		return envelope.WorkerID
	}
	parsed, err := strconv.ParseInt(strings.TrimSpace(c.Param("workerId")), 10, 64)
	if err != nil || parsed <= 0 {
		return 0
	}
	return parsed
}

func writeError(c *app.RequestContext, status int, code, message string) {
	var err *apperrors.AppError
	switch status {
	case http.StatusBadRequest:
		err = apperrors.Params(message)
	case http.StatusUnauthorized:
		err = apperrors.Unauthorized(message)
	case http.StatusConflict:
		err = apperrors.ObjectState(message)
	default:
		err = apperrors.System(message)
	}
	response.Error(c, err.WithDetails(map[string]any{"reasonCode": code}))
	// Framework browser APIs intentionally carry many application errors in a
	// 200 envelope. Machine clients need transport-level status to distinguish
	// retry, stale ownership and payload conflicts without parsing localized
	// messages, so the internal Worker contract preserves the requested status.
	c.Response.SetStatusCode(status)
}

func (m *Middleware) Cleanup(ctx context.Context) error {
	_, err := m.db.Exec(ctx, `DELETE FROM workforce_machine_idempotency_records WHERE expires_at<CURRENT_TIMESTAMP`)
	return err
}
