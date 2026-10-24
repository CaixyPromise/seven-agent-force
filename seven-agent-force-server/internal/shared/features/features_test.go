package features

import (
	"reflect"
	"testing"

	"github.com/CaixyPromise/seven-agent/force/internal/shared/config"
)

func TestResolveFeatureMatrix(t *testing.T) {
	tests := []struct {
		name   string
		mode   config.PlatformMode
		docker bool
		want   []string
	}{
		{name: "local", want: []string{}},
		{name: "local docker", docker: true, want: []string{"docker.admin"}},
		{name: "hub", mode: config.PlatformModeHub, want: []string{"federation.hub", "platform.control"}},
		{name: "hub docker", mode: config.PlatformModeHub, docker: true, want: []string{"docker.admin", "federation.hub", "platform.control"}},
		{name: "node", mode: config.PlatformModeNode, want: []string{"federation.node"}},
		{name: "node docker", mode: config.PlatformModeNode, docker: true, want: []string{"docker.admin", "federation.node"}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := Resolve(config.Config{
				Platform: config.PlatformConfig{Mode: tc.mode},
				Docker:   config.DockerConfig{Enabled: tc.docker},
			}).EnabledCodes()
			if !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("enabled codes=%v want=%v", got, tc.want)
			}
		})
	}
}

func TestKnown(t *testing.T) {
	for _, code := range []Code{PlatformControl, FederationHub, FederationNode, DockerAdmin} {
		if !Known(code) {
			t.Fatalf("expected known feature code %q", code)
		}
	}
	if Known("future.capability") {
		t.Fatal("unknown feature code must not be accepted")
	}
}

func TestWithoutReturnsIndependentSet(t *testing.T) {
	original := Set{PlatformControl: {}, DockerAdmin: {}}
	effective := original.Without(DockerAdmin)
	if effective.Enabled(DockerAdmin) || !effective.Enabled(PlatformControl) {
		t.Fatalf("effective set=%v", effective)
	}
	if !original.Enabled(DockerAdmin) {
		t.Fatalf("original set was mutated: %v", original)
	}
}
