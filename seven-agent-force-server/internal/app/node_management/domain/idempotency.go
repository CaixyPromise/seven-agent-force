package domain

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"strings"
	"time"

	nodeinfra "github.com/CaixyPromise/seven-agent/force/internal/app/node_management/infrastructure"
	apperrors "github.com/CaixyPromise/seven-agent/force/internal/common/errors"
	"github.com/google/uuid"
)

const (
	defaultProcessingTTL = 2 * time.Minute
	defaultRetentionTTL  = 24 * time.Hour
)

// CommandMetadata identifies one Node command and its request value.
type CommandMetadata struct {
	NodeCode       string
	Method         string
	Path           string
	IdempotencyKey string
	RequestDigest  string
}

// CommandCoordinator owns command acceptance, preparation, and replay semantics.
type CommandCoordinator interface {
	Prepare(ctx context.Context, metadata CommandMetadata, prepare func(context.Context) ([]byte, error)) ([]byte, error)
	Execute(ctx context.Context, metadata CommandMetadata, operation func(context.Context) ([]byte, error)) (result []byte, replayed bool, err error)
}

// Coordinator coordinates value-idempotent commands over an infrastructure store.
type Coordinator struct {
	store         nodeinfra.CommandStore
	processingTTL time.Duration
	retentionTTL  time.Duration
}

// NewCommandCoordinator creates a fail-closed command coordinator.
func NewCommandCoordinator(store nodeinfra.CommandStore) *Coordinator {
	return &Coordinator{store: store, processingTTL: defaultProcessingTTL, retentionTTL: defaultRetentionTTL}
}

// Prepare captures immutable command input independently of the processing claim.
func (c *Coordinator) Prepare(ctx context.Context, metadata CommandMetadata, prepare func(context.Context) ([]byte, error)) ([]byte, error) {
	if c == nil || c.store == nil {
		return nil, apperrors.ServiceUnavailable("命令幂等存储不可用")
	}
	if completed, hit, err := c.store.GetClaim(ctx, commandCacheKey(metadata)); err != nil {
		return nil, apperrors.ServiceUnavailable("命令幂等存储不可用")
	} else if hit {
		if completed.RequestDigest != strings.TrimSpace(metadata.RequestDigest) {
			return nil, apperrors.ObjectState("幂等键已用于不同请求")
		}
		if len(completed.Result) > 0 {
			return nil, nil
		}
		return nil, apperrors.ObjectState("相同命令正在执行").WithDetails(map[string]any{"retryAfterSeconds": 2})
	}
	key := preparedCacheKey(metadata)
	if existing, hit, err := c.store.GetPrepared(ctx, key); err != nil {
		return nil, apperrors.ServiceUnavailable("命令准备状态不可用")
	} else if hit {
		return resolvePrepared(existing, metadata)
	}
	payload, err := prepare(ctx)
	if err != nil {
		return nil, err
	}
	prepared := nodeinfra.PreparedCommand{RequestDigest: strings.TrimSpace(metadata.RequestDigest), Payload: append([]byte(nil), payload...)}
	created, err := c.store.CreatePrepared(ctx, key, prepared, c.retentionTTL)
	if err != nil {
		return nil, apperrors.ServiceUnavailable("命令准备状态不可用")
	}
	if created {
		return append([]byte(nil), prepared.Payload...), nil
	}
	existing, hit, err := c.store.GetPrepared(ctx, key)
	if err != nil || !hit {
		return nil, apperrors.ServiceUnavailable("命令准备状态无法确认")
	}
	return resolvePrepared(existing, metadata)
}

// Execute owns a short processing claim and a retained safe replay result.
func (c *Coordinator) Execute(ctx context.Context, metadata CommandMetadata, operation func(context.Context) ([]byte, error)) ([]byte, bool, error) {
	if c == nil || c.store == nil {
		return nil, false, apperrors.ServiceUnavailable("命令幂等存储不可用")
	}
	key := commandCacheKey(metadata)
	claim := nodeinfra.CommandClaim{RequestDigest: strings.TrimSpace(metadata.RequestDigest), OwnerToken: uuid.NewString()}
	acquired, err := c.store.CreateClaim(ctx, key, claim, c.processingTTL)
	if err != nil {
		return nil, false, apperrors.ServiceUnavailable("命令幂等存储不可用")
	}
	if !acquired {
		return c.resolveExisting(ctx, key, metadata, operation)
	}

	result, operationErr := operation(ctx)
	if operationErr != nil {
		_, _ = c.store.DeleteClaim(ctx, key, claim)
		return nil, false, operationErr
	}
	if len(result) == 0 {
		result = []byte("null")
	}
	completed := claim
	completed.Result = append([]byte(nil), result...)
	updated, err := c.store.CompleteClaim(ctx, key, preparedCacheKey(metadata), claim, completed, c.retentionTTL)
	if err != nil || !updated {
		return nil, false, apperrors.ServiceUnavailable("命令结果暂时无法确认，请使用相同幂等键重试")
	}
	return result, false, nil
}

func (c *Coordinator) resolveExisting(ctx context.Context, key string, metadata CommandMetadata, operation func(context.Context) ([]byte, error)) ([]byte, bool, error) {
	existing, hit, err := c.store.GetClaim(ctx, key)
	if err != nil {
		return nil, false, apperrors.ServiceUnavailable("命令幂等存储不可用")
	}
	if !hit {
		return c.Execute(ctx, metadata, operation)
	}
	if existing.RequestDigest != strings.TrimSpace(metadata.RequestDigest) {
		return nil, false, apperrors.ObjectState("幂等键已用于不同请求")
	}
	if len(existing.Result) > 0 {
		return append([]byte(nil), existing.Result...), true, nil
	}
	return nil, false, apperrors.ObjectState("相同命令正在执行").WithDetails(map[string]any{"retryAfterSeconds": 2})
}

func resolvePrepared(prepared nodeinfra.PreparedCommand, metadata CommandMetadata) ([]byte, error) {
	if prepared.RequestDigest != strings.TrimSpace(metadata.RequestDigest) {
		return nil, apperrors.ObjectState("幂等键已用于不同请求")
	}
	return append([]byte(nil), prepared.Payload...), nil
}

// RetryAfter extracts a bounded retry delay from an in-progress error.
func RetryAfter(err error) int {
	details, ok := apperrors.From(err).Details().(map[string]any)
	if !ok {
		return 0
	}
	value, _ := details["retryAfterSeconds"].(int)
	return value
}

func commandCacheKey(metadata CommandMetadata) string {
	return scopedCacheKey("command", metadata)
}

func preparedCacheKey(metadata CommandMetadata) string {
	return scopedCacheKey("prepared", metadata)
}

func scopedCacheKey(kind string, metadata CommandMetadata) string {
	return "node_management:{" + CommandScopeHash(metadata) + "}:" + kind
}

// CommandScopeHash returns the opaque SHA-256 identity shared by command replay storage and durable status replay.
func CommandScopeHash(metadata CommandMetadata) string {
	scope := strings.Join([]string{strings.TrimSpace(metadata.NodeCode), strings.ToUpper(strings.TrimSpace(metadata.Method)), strings.TrimSpace(metadata.Path), strings.TrimSpace(metadata.IdempotencyKey)}, "|")
	sum := sha256.Sum256([]byte(scope))
	return hex.EncodeToString(sum[:])
}

var _ CommandCoordinator = (*Coordinator)(nil)
