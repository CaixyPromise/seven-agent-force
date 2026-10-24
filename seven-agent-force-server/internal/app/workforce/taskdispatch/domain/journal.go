package domain

// ReplayableJournalFact is shared by Worker filtering and the Server's
// journal-only ingress. These observations never advance execution authority.
func ReplayableJournalFact(event string) bool {
	switch event {
	case "hook.configuration.loaded", "hook.started", "hook.allow", "hook.deny", "hook.failed", "owner.adopted", "watchdog.expired", "runtime.interrupted":
		return true
	default:
		return false
	}
}
