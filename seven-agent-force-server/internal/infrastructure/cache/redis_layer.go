package cache

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/CaixyPromise/seven-agent/force/internal/infrastructure/cache/l1"
	redisinfra "github.com/CaixyPromise/seven-agent/force/internal/infrastructure/cache/redis"
	redisclient "github.com/redis/go-redis/v9"
)

type RedisLayer struct {
	provider Provider
	codec    Codec
	l1       *l1.Store
}

func NewRedisLayer(provider Provider, codec Codec, l1Store *l1.Store) *RedisLayer {
	return &RedisLayer{
		provider: provider,
		codec:    codec,
		l1:       l1Store,
	}
}

func (l *RedisLayer) Name() string {
	return "redis"
}

func (l *RedisLayer) Enabled() bool {
	return l != nil && l.provider != nil && l.provider.Configured() && l.provider.Client() != nil
}

func (l *RedisLayer) Get(ctx context.Context, cacheKey string, dest any) (bool, error) {
	payload, hit, err := l.GetBytes(ctx, cacheKey)
	if err != nil || !hit {
		return hit, err
	}
	if err := l.codec.Unmarshal(payload, dest); err != nil {
		return false, err
	}
	return true, nil
}

func (l *RedisLayer) Set(ctx context.Context, cacheKey string, value any, ttl time.Duration) error {
	payload, err := l.codec.Marshal(value)
	if err != nil {
		return err
	}
	return l.SetBytes(ctx, cacheKey, payload, ttl)
}

func (l *RedisLayer) GetString(ctx context.Context, cacheKey string) (string, bool, error) {
	client, err := l.client()
	if err != nil {
		return "", false, err
	}
	value, err := client.Get(ctx, cacheKey).Result()
	if errors.Is(err, redisclient.Nil) {
		return "", false, nil
	}
	if err != nil {
		return "", false, err
	}
	return value, true, nil
}

func (l *RedisLayer) GetBytes(ctx context.Context, cacheKey string) ([]byte, bool, error) {
	client, err := l.client()
	if err != nil {
		return nil, false, err
	}
	payload, err := client.Get(ctx, cacheKey).Bytes()
	if errors.Is(err, redisclient.Nil) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, err
	}
	return payload, true, nil
}

func (l *RedisLayer) SetString(ctx context.Context, cacheKey string, value string, ttl time.Duration) error {
	client, err := l.client()
	if err != nil {
		return err
	}
	if err := client.Set(ctx, cacheKey, value, ttl).Err(); err != nil {
		return err
	}
	l.invalidate(cacheKey)
	return nil
}

func (l *RedisLayer) SetBytes(ctx context.Context, cacheKey string, payload []byte, ttl time.Duration) error {
	client, err := l.client()
	if err != nil {
		return err
	}
	if err := client.Set(ctx, cacheKey, payload, ttl).Err(); err != nil {
		return err
	}
	l.invalidate(cacheKey)
	return nil
}

func (l *RedisLayer) Delete(ctx context.Context, cacheKey string) error {
	return l.DeleteMany(ctx, cacheKey)
}

func (l *RedisLayer) Exists(ctx context.Context, cacheKey string) (bool, error) {
	client, err := l.client()
	if err != nil {
		return false, err
	}
	count, err := client.Exists(ctx, cacheKey).Result()
	return count > 0, err
}

func (l *RedisLayer) Expire(ctx context.Context, cacheKey string, ttl time.Duration) error {
	client, err := l.client()
	if err != nil {
		return err
	}
	if ttl <= 0 {
		if err := client.Persist(ctx, cacheKey).Err(); err != nil {
			return err
		}
	} else if err := client.Expire(ctx, cacheKey, ttl).Err(); err != nil {
		return err
	}
	l.invalidate(cacheKey)
	return nil
}

func (l *RedisLayer) GetDel(ctx context.Context, cacheKey string, dest any) (bool, error) {
	value, hit, err := l.GetDelString(ctx, cacheKey)
	if err != nil || !hit {
		return hit, err
	}
	if err := l.codec.Unmarshal([]byte(value), dest); err != nil {
		return false, err
	}
	return true, nil
}

func (l *RedisLayer) GetDelString(ctx context.Context, cacheKey string) (string, bool, error) {
	client, err := l.client()
	if err != nil {
		return "", false, err
	}
	result, err := redisinfra.GetDelScript.Run(ctx, client, []string{cacheKey}).Result()
	if errors.Is(err, redisclient.Nil) || result == nil || result == false {
		return "", false, nil
	}
	if err != nil {
		return "", false, err
	}
	l.invalidate(cacheKey)
	return redisValueString(result), true, nil
}

func (l *RedisLayer) CompareAndDelete(ctx context.Context, cacheKey string, expected any) (bool, error) {
	payload, err := l.codec.Marshal(expected)
	if err != nil {
		return false, err
	}
	return l.CompareAndDeleteString(ctx, cacheKey, bytesToString(payload))
}

func (l *RedisLayer) CompareAndDeleteString(ctx context.Context, cacheKey string, expected string) (bool, error) {
	client, err := l.client()
	if err != nil {
		return false, err
	}
	deleted, err := redisinfra.CompareDeleteScript.Run(ctx, client, []string{cacheKey}, expected).Int64()
	if err != nil {
		return false, err
	}
	if deleted > 0 {
		l.invalidate(cacheKey)
	}
	return deleted > 0, nil
}

func (l *RedisLayer) CompareAndSetString(ctx context.Context, cacheKey string, expected string, replacement string, ttl time.Duration) (bool, error) {
	client, err := l.client()
	if err != nil {
		return false, err
	}
	updated, err := compareSetStringScript.Run(ctx, client, []string{cacheKey}, expected, replacement, ttl.Milliseconds()).Int64()
	if err != nil {
		return false, err
	}
	if updated > 0 {
		l.invalidate(cacheKey)
	}
	return updated > 0, nil
}

func (l *RedisLayer) CompareAndSetStringAndExpire(ctx context.Context, cacheKey string, expected string, replacement string, expiryKey string, ttl time.Duration) (bool, error) {
	client, err := l.client()
	if err != nil {
		return false, err
	}
	updated, err := compareSetStringAndExpireScript.Run(ctx, client, []string{cacheKey, expiryKey}, expected, replacement, ttl.Milliseconds()).Int64()
	if err != nil {
		return false, err
	}
	if updated > 0 {
		l.invalidate(cacheKey)
		l.invalidate(expiryKey)
	}
	return updated > 0, nil
}

func (l *RedisLayer) SetMaxTimestamp(ctx context.Context, cacheKey string, value time.Time, ttl time.Duration) (bool, error) {
	client, err := l.client()
	if err != nil {
		return false, err
	}
	encoded := value.UTC().Format("2006-01-02T15:04:05.000000000Z")
	updated, err := setMaxTimestampScript.Run(ctx, client, []string{cacheKey}, encoded, ttl.Milliseconds()).Int64()
	if err != nil {
		return false, err
	}
	if updated > 0 {
		l.invalidate(cacheKey)
	}
	return updated > 0, nil
}

var compareSetStringScript = redisclient.NewScript(`
if redis.call("GET", KEYS[1]) ~= ARGV[1] then
  return 0
end
redis.call("SET", KEYS[1], ARGV[2], "PX", ARGV[3])
return 1
`)

var compareSetStringAndExpireScript = redisclient.NewScript(`
if redis.call("GET", KEYS[1]) ~= ARGV[1] then
  return 0
end
redis.call("SET", KEYS[1], ARGV[2], "PX", ARGV[3])
if redis.call("EXISTS", KEYS[2]) == 1 then
  redis.call("PEXPIRE", KEYS[2], ARGV[3])
end
return 1
`)

var setMaxTimestampScript = redisclient.NewScript(`
local function normalize_timestamp(value)
  if not value then
    return nil
  end
  local prefix, fraction = string.match(value, "^(%d%d%d%d%-%d%d%-%d%dT%d%d:%d%d:%d%d)%.(%d+)Z$")
  if not prefix then
    prefix = string.match(value, "^(%d%d%d%d%-%d%d%-%d%dT%d%d:%d%d:%d%d)Z$")
    fraction = ""
  end
  if not prefix then
    return value
  end
  fraction = string.sub(fraction .. "000000000", 1, 9)
  return prefix .. "." .. fraction .. "Z"
end
local current = redis.call("GET", KEYS[1])
if current and normalize_timestamp(current) >= ARGV[1] then
  return 0
end
redis.call("SET", KEYS[1], ARGV[1], "PX", ARGV[2])
return 1
`)

func (l *RedisLayer) HGetAll(ctx context.Context, cacheKey string) (map[string]string, error) {
	client, err := l.client()
	if err != nil {
		return nil, err
	}
	values, err := client.HGetAll(ctx, cacheKey).Result()
	if err != nil {
		return nil, err
	}
	return values, nil
}

func (l *RedisLayer) HSet(ctx context.Context, cacheKey string, values map[string]string) error {
	client, err := l.client()
	if err != nil {
		return err
	}
	if len(values) == 0 {
		return nil
	}
	payload := make(map[string]any, len(values))
	for field, value := range values {
		payload[field] = value
	}
	if err := client.HSet(ctx, cacheKey, payload).Err(); err != nil {
		return err
	}
	l.invalidate(cacheKey)
	return nil
}

func (l *RedisLayer) HGetAllDel(ctx context.Context, cacheKey string) (map[string]string, error) {
	client, err := l.client()
	if err != nil {
		return nil, err
	}
	result, err := redisinfra.HGetAllDelScript.Run(ctx, client, []string{cacheKey}).Result()
	if err != nil {
		return nil, err
	}
	values := map[string]string{}
	items, ok := result.([]any)
	if !ok {
		return values, nil
	}
	for index := 0; index+1 < len(items); index += 2 {
		values[fmt.Sprint(items[index])] = fmt.Sprint(items[index+1])
	}
	l.invalidate(cacheKey)
	return values, nil
}

func (l *RedisLayer) HDel(ctx context.Context, cacheKey string, fields ...string) error {
	client, err := l.client()
	if err != nil {
		return err
	}
	if len(fields) == 0 {
		return nil
	}
	args := make([]string, 0, len(fields))
	for _, field := range fields {
		if field != "" {
			args = append(args, field)
		}
	}
	if len(args) == 0 {
		return nil
	}
	if err := client.HDel(ctx, cacheKey, args...).Err(); err != nil {
		return err
	}
	l.invalidate(cacheKey)
	return nil
}

func (l *RedisLayer) SetNX(ctx context.Context, cacheKey string, value any, ttl time.Duration) (bool, error) {
	payload, err := l.codec.Marshal(value)
	if err != nil {
		return false, err
	}
	return l.SetNXBytes(ctx, cacheKey, payload, ttl)
}

func (l *RedisLayer) SetNXString(ctx context.Context, cacheKey string, value string, ttl time.Duration) (bool, error) {
	client, err := l.client()
	if err != nil {
		return false, err
	}
	ok, err := client.SetNX(ctx, cacheKey, value, ttl).Result()
	if ok {
		l.invalidate(cacheKey)
	}
	return ok, err
}

func (l *RedisLayer) SetNXBytes(ctx context.Context, cacheKey string, value []byte, ttl time.Duration) (bool, error) {
	client, err := l.client()
	if err != nil {
		return false, err
	}
	ok, err := client.SetNX(ctx, cacheKey, value, ttl).Result()
	if ok {
		l.invalidate(cacheKey)
	}
	return ok, err
}

func (l *RedisLayer) Incr(ctx context.Context, cacheKey string, ttl time.Duration) (int64, error) {
	client, err := l.client()
	if err != nil {
		return 0, err
	}
	value, err := redisinfra.IncrWithTTLScript.Run(ctx, client, []string{cacheKey}, ttl.Milliseconds()).Int64()
	if err != nil {
		return 0, err
	}
	l.invalidate(cacheKey)
	return value, nil
}

func (l *RedisLayer) DeleteMany(ctx context.Context, cacheKeys ...string) error {
	client, err := l.client()
	if err != nil {
		return err
	}
	keys := compactKeys(cacheKeys)
	if len(keys) == 0 {
		return nil
	}
	if err := client.Del(ctx, keys...).Err(); err != nil {
		return err
	}
	l.invalidate(keys...)
	return nil
}

func (l *RedisLayer) TTL(ctx context.Context, cacheKey string) (time.Duration, error) {
	client, err := l.client()
	if err != nil {
		return 0, err
	}
	ttl, err := client.TTL(ctx, cacheKey).Result()
	if err != nil {
		return 0, err
	}
	if ttl < 0 {
		return 0, nil
	}
	return ttl, nil
}

func (l *RedisLayer) SetRaw(ctx context.Context, cacheKey string, payload []byte, ttl time.Duration) error {
	return l.SetBytes(ctx, cacheKey, payload, ttl)
}

func (l *RedisLayer) GetRaw(ctx context.Context, cacheKey string) ([]byte, bool, error) {
	return l.GetBytes(ctx, cacheKey)
}

func (l *RedisLayer) client() (redisclient.UniversalClient, error) {
	if !l.Enabled() {
		return nil, ErrRedisUnavailable
	}
	return l.provider.Client(), nil
}

func (l *RedisLayer) invalidate(cacheKeys ...string) {
	if l.l1 == nil {
		return
	}
	l.l1.Delete(cacheKeys...)
}

func compactKeys(cacheKeys []string) []string {
	result := make([]string, 0, len(cacheKeys))
	for _, cacheKey := range cacheKeys {
		if cacheKey != "" {
			result = append(result, cacheKey)
		}
	}
	return result
}

func redisValueString(value any) string {
	switch typed := value.(type) {
	case string:
		return typed
	case []byte:
		return bytesToString(typed)
	default:
		return fmt.Sprint(value)
	}
}

func bytesToString(value []byte) string {
	return string(value)
}
