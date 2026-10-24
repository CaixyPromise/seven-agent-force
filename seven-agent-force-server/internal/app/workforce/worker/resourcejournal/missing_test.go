package resourcejournal

import (
	"errors"
	"testing"
)

func TestDocker28MissingDiagnostics(t *testing.T) {
	for _, body := range []string{"[]\nError response from daemon: get scoped-volume: no such volume", "[]\nError response from daemon: network scoped-network not found", "Error: No such container: scoped-container"} {
		if !missing(body, errors.New("exit status 1")) {
			t.Errorf("actual Docker absence not recognized: %s", body)
		}
	}
	for _, body := range []string{"Cannot connect to the Docker daemon", "context deadline exceeded", "permission denied", "Error response from daemon: network scoped-network has active endpoints", "no such volume mentioned by container output"} {
		if missing(body, errors.New("exit status 1")) {
			t.Errorf("non-absence accepted: %s", body)
		}
	}
}
