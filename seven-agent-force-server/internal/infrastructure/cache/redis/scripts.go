package redis

import redisclient "github.com/redis/go-redis/v9"

var (
	GetDelScript = redisclient.NewScript(`
local value = redis.call("GET", KEYS[1])
if not value then
  return false
end
redis.call("DEL", KEYS[1])
return value
`)

	HGetAllDelScript = redisclient.NewScript(`
local values = redis.call("HGETALL", KEYS[1])
if #values == 0 then
  return {}
end
redis.call("DEL", KEYS[1])
return values
`)

	CompareDeleteScript = redisclient.NewScript(`
if redis.call("GET", KEYS[1]) == ARGV[1] then
  return redis.call("DEL", KEYS[1])
end
return 0
`)

	CompareExpireScript = redisclient.NewScript(`
if redis.call("GET", KEYS[1]) == ARGV[1] then
  return redis.call("PEXPIRE", KEYS[1], ARGV[2])
end
return 0
`)

	IncrWithTTLScript = redisclient.NewScript(`
local value = redis.call("INCR", KEYS[1])
if value == 1 and tonumber(ARGV[1]) > 0 then
  redis.call("PEXPIRE", KEYS[1], ARGV[1])
end
return value
`)
)
