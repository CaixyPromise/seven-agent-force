package domain

import "testing"

func TestInteractionIdentityBindsKnownPrepareFieldsNotUnknownSession(t *testing.T) {
	// No session descriptor parameter exists: reserve and pack use the same known
	// occurrence, snapshot and preparation identity before/after native stop.
	base := InteractionCheckpointIdentity(1, 2, 3, 4, 5, 6, 7, "snapshot", "control-sha")
	for _, changed := range []string{
		InteractionCheckpointIdentity(1, 2, 3, 4, 8, 6, 7, "snapshot", "control-sha"),
		InteractionCheckpointIdentity(1, 2, 3, 4, 5, 8, 7, "snapshot", "control-sha"),
		InteractionCheckpointIdentity(1, 2, 3, 4, 5, 6, 8, "snapshot", "control-sha"),
		InteractionCheckpointIdentity(1, 2, 3, 4, 5, 6, 7, "other", "control-sha"),
		InteractionCheckpointIdentity(1, 2, 3, 4, 5, 6, 7, "snapshot", "other"),
	} {
		if changed == base {
			t.Fatal("preparation drift retained identity")
		}
	}
}
