package application

import (
	"context"
	"errors"
	"testing"
)

type capacityRepository struct {
	Repository
	workerID int64
	maximum  int
	version  int64
	err      error
}

func (r *capacityRepository) UpdateCapacity(_ context.Context, workerID int64, maximum int, version int64) error {
	r.workerID = workerID
	r.maximum = maximum
	r.version = version
	return r.err
}

func TestUpdateCapacityValidatesAndDelegates(t *testing.T) {
	repository := &capacityRepository{}
	service := New(repository, nil)

	if err := service.UpdateCapacity(context.Background(), 101, 4, 7); err != nil {
		t.Fatal(err)
	}
	if repository.workerID != 101 || repository.maximum != 4 || repository.version != 7 {
		t.Fatalf("unexpected update: %+v", repository)
	}
}

func TestUpdateCapacityRejectsInvalidValues(t *testing.T) {
	repository := &capacityRepository{}
	service := New(repository, nil)

	for _, test := range []struct {
		workerID int64
		maximum  int
		version  int64
	}{
		{workerID: 0, maximum: 1, version: 1},
		{workerID: 1, maximum: 0, version: 1},
		{workerID: 1, maximum: 65, version: 1},
		{workerID: 1, maximum: 1, version: 0},
	} {
		if err := service.UpdateCapacity(context.Background(), test.workerID, test.maximum, test.version); !errors.Is(err, ErrInvalidCapacity) {
			t.Fatalf("expected invalid capacity, got %v", err)
		}
	}
	if repository.workerID != 0 {
		t.Fatal("invalid capacity reached the repository")
	}
}

func TestUpdateCapacityPreservesRepositoryConflict(t *testing.T) {
	repository := &capacityRepository{err: ErrCapacityExceedsWorker}
	service := New(repository, nil)

	if err := service.UpdateCapacity(context.Background(), 101, 4, 7); !errors.Is(err, ErrCapacityExceedsWorker) {
		t.Fatalf("expected repository error, got %v", err)
	}
}
