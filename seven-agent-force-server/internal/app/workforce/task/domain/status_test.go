package domain

import "testing"

func TestTaskStatusCanTransition(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		from TaskStatus
		to   TaskStatus
		want bool
	}{
		{name: "queue created task", from: TaskCreated, to: TaskQueued, want: true},
		{name: "cancel queued task", from: TaskQueued, to: TaskCancelled, want: true},
		{name: "skip execution", from: TaskQueued, to: TaskSucceeded, want: false},
		{name: "review disabled", from: TaskValidating, to: TaskPackaging, want: true},
		{name: "resume after findings", from: TaskReviewing, to: TaskRunning, want: true},
		{name: "upload never reruns attempt", from: TaskUploading, to: TaskRetryWait, want: false},
		{name: "success is terminal", from: TaskSucceeded, to: TaskRunning, want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := tt.from.CanTransition(tt.to); got != tt.want {
				t.Fatalf("%s.CanTransition(%s) = %v, want %v", tt.from, tt.to, got, tt.want)
			}
		})
	}
}

func TestTaskStatusTerminal(t *testing.T) {
	t.Parallel()

	if !TaskSucceeded.Terminal() {
		t.Fatal("TaskSucceeded.Terminal() = false, want true")
	}
	if TaskRunning.Terminal() {
		t.Fatal("TaskRunning.Terminal() = true, want false")
	}
}
