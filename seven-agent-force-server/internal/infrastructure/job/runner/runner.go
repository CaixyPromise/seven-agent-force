package runner

import (
	"context"
	"fmt"
	"runtime/debug"
	"time"

	lockinfra "github.com/CaixyPromise/seven-agent/force/internal/infrastructure/lock"
)

type Job interface {
	Name() string
	Run(ctx context.Context) error
}

type Metrics interface {
	RecordSchedulerRun(name string, duration time.Duration, err error)
}

func Execute(ctx context.Context, job Job, lockEnabled bool, ttl time.Duration, locker lockinfra.DistributedLock, metrics Metrics) (err error) {
	startedAt := time.Now()
	defer func() {
		if recovered := recover(); recovered != nil {
			err = fmt.Errorf("job %s panicked: %v\n%s", job.Name(), recovered, string(debug.Stack()))
		}
		if metrics != nil {
			metrics.RecordSchedulerRun(job.Name(), time.Since(startedAt), err)
		}
	}()

	if lockEnabled && locker != nil {
		token, ok, lockErr := locker.TryLock(ctx, fmt.Sprintf("scheduler:%s", job.Name()), ttl)
		if lockErr != nil {
			err = lockErr
			return err
		}
		if !ok {
			return nil
		}
		defer func() {
			_, _ = locker.Unlock(context.Background(), fmt.Sprintf("scheduler:%s", job.Name()), token)
		}()
	}

	err = job.Run(ctx)
	return err
}
