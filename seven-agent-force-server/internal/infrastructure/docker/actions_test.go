package docker

import "testing"

func TestContainerAvailableActions(t *testing.T) {
	tests := []struct {
		state string
		want  []string
	}{
		{state: "running", want: []string{"stop", "restart", "logs", "stats", "inspect"}},
		{state: "exited", want: []string{"start", "logs", "inspect", "delete"}},
		{state: "created", want: []string{"start", "logs", "inspect", "delete"}},
		{state: "dead", want: []string{"start", "logs", "inspect", "delete"}},
		{state: "paused", want: []string{"stop", "restart", "logs", "stats", "inspect"}},
	}
	for _, tt := range tests {
		t.Run(tt.state, func(t *testing.T) {
			got := containerAvailableActions(tt.state)
			if !sameStringSlice(got, tt.want) {
				t.Fatalf("available actions mismatch: got=%v want=%v", got, tt.want)
			}
		})
	}
}

func TestComposeAvailableActions(t *testing.T) {
	tests := []struct {
		name           string
		source         ComposeProjectSource
		status         ComposeProjectStatus
		containerCount int
		hasSpec        bool
		active         bool
		want           []string
	}{
		{name: "managed running", source: ComposeProjectSourceManaged, status: ComposeProjectStatusRunning, containerCount: 2, hasSpec: true, want: []string{"preview", "validate", "edit", "logs", "ps", "down", "restart"}},
		{name: "managed stopped", source: ComposeProjectSourceManaged, status: ComposeProjectStatusStopped, containerCount: 0, hasSpec: true, want: []string{"preview", "validate", "edit", "up"}},
		{name: "managed active", source: ComposeProjectSourceManaged, status: ComposeProjectStatusRunning, containerCount: 1, hasSpec: true, active: true, want: []string{"preview", "validate", "edit", "logs", "ps"}},
		{name: "discovered running", source: ComposeProjectSourceDiscovered, status: ComposeProjectStatusRunning, containerCount: 1, hasSpec: false, want: []string{"logs", "ps"}},
		{name: "discovered empty", source: ComposeProjectSourceDiscovered, status: ComposeProjectStatusUnknown, containerCount: 0, hasSpec: false, want: []string{}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := composeAvailableActions(tt.source, tt.status, tt.containerCount, tt.hasSpec, tt.active)
			if !sameStringSlice(got, tt.want) {
				t.Fatalf("available actions mismatch: got=%v want=%v", got, tt.want)
			}
		})
	}
}

func sameStringSlice(left, right []string) bool {
	if len(left) != len(right) {
		return false
	}
	for i := range left {
		if left[i] != right[i] {
			return false
		}
	}
	return true
}
