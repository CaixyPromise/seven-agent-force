package imageadmission

import (
	"context"
	"errors"
	"os/exec"
	"strings"
	"testing"
)

func TestEnvironmentProbeDoesNotTreatUnavailableDockerAsMissingCapability(t *testing.T) {
	missing := exec.Command("/bin/sh", "-c", "exit 1").Run()
	missingAsh := exec.Command("/bin/sh", "-c", "exit 127").Run()
	dockerFailure := exec.Command("/bin/sh", "-c", "exit 125").Run()
	for _, tc := range []struct {
		name                   string
		commandErr, contextErr error
		unavailable            bool
	}{
		{"missing command", missing, nil, false},
		{"missing command (Alpine ash)", missingAsh, nil, false},
		{"docker unavailable", dockerFailure, nil, true},
		{"probe cancelled", context.Canceled, context.Canceled, true},
		{"probe timed out", missing, context.DeadlineExceeded, true},
		{"docker missing", exec.ErrNotFound, nil, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			err := environmentProbeFailure("shell.posix", tc.commandErr, tc.contextErr)
			if errors.Is(err, ErrInspectionUnavailable) != tc.unavailable {
				t.Fatalf("wrong unavailable classification: %v", err)
			}
			if strings.HasPrefix(tc.name, "missing command") && (err == nil || !strings.Contains(err.Error(), "IMAGE_ENVIRONMENT_CAPABILITY_MISSING")) {
				t.Fatalf("missing command must fail admission: %v", err)
			}
		})
	}
}
