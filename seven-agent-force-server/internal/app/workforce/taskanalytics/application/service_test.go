package application

import (
	"context"
	"errors"
	"testing"
	"time"
)

type repositoryStub struct{}

func (repositoryStub) ProjectList(context.Context, int64, Window) (ProjectListAnalytics, error) {
	return ProjectListAnalytics{}, nil
}
func (repositoryStub) Project(context.Context, int64, int64, Window) (ProjectAnalytics, error) {
	return ProjectAnalytics{}, nil
}
func (repositoryStub) Tasks(context.Context, int64, Window) (TaskAnalytics, error) {
	return TaskAnalytics{}, nil
}
func (repositoryStub) Workbench(context.Context, int64, Window, bool) (Workbench, error) {
	return Workbench{}, nil
}

func TestParseWindowUsesFixedUTCDays(t *testing.T) {
	now := time.Date(2026, 9, 18, 23, 30, 0, 0, time.FixedZone("test", 8*3600))
	window, err := ParseWindow("14d", map[string]int{"14d": 14}, now)
	if err != nil {
		t.Fatal(err)
	}
	if got, want := window.To, time.Date(2026, 9, 19, 0, 0, 0, 0, time.UTC); !got.Equal(want) {
		t.Fatalf("to=%s want %s", got, want)
	}
	if window.To.Sub(window.From) != 14*24*time.Hour {
		t.Fatalf("unexpected window: %#v", window)
	}
}

func TestParseWindowRejectsArbitraryIntervals(t *testing.T) {
	_, err := ParseWindow("365d", map[string]int{"7d": 7}, time.Now())
	if !errors.Is(err, ErrRangeInvalid) {
		t.Fatalf("err=%v", err)
	}
}

func TestServiceAppliesPageSpecificRanges(t *testing.T) {
	service := New(repositoryStub{})
	service.now = func() time.Time { return time.Date(2026, 9, 18, 12, 0, 0, 0, time.UTC) }
	if _, err := service.ProjectList(context.Background(), 1, "14d"); !errors.Is(err, ErrRangeInvalid) {
		t.Fatalf("Project list accepted Task-only range: %v", err)
	}
	if _, err := service.Tasks(context.Background(), 1, "90d"); !errors.Is(err, ErrRangeInvalid) {
		t.Fatalf("Task analytics accepted Project-only range: %v", err)
	}
	if _, err := service.Workbench(context.Background(), 1, "90d", true); !errors.Is(err, ErrRangeInvalid) {
		t.Fatalf("Workbench accepted unsupported range: %v", err)
	}
	for _, value := range []string{"7d", "14d", "30d"} {
		if _, err := service.Workbench(context.Background(), 1, value, true); err != nil {
			t.Fatalf("Workbench rejected %s: %v", value, err)
		}
	}
}
