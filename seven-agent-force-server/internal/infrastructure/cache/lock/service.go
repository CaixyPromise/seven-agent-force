package lock

import (
	cacheinfra "github.com/CaixyPromise/seven-agent/force/internal/infrastructure/cache"
	lockinfra "github.com/CaixyPromise/seven-agent/force/internal/infrastructure/lock"
)

// Deprecated: use internal/infrastructure/lock. This compatibility package is
// kept so older imports continue to compile while callers are migrated.
type DistributedLock = lockinfra.DistributedLock

// Deprecated: use internal/infrastructure/lock.
type SchedulerLockService = lockinfra.SchedulerLockService

// Deprecated: use internal/infrastructure/lock.
type ReplayProtectionService = lockinfra.ReplayProtectionService

// Deprecated: use internal/infrastructure/lock.RedisService.
type Service = lockinfra.RedisService

// Deprecated: use internal/infrastructure/lock.NewRedisService.
func NewService(provider cacheinfra.Provider) *Service {
	return lockinfra.NewRedisService(provider)
}
