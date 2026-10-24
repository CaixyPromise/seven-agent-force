package domain

// ValidCompletionCheckDigest accepts the absent legacy identity or a canonical
// SHA-256 digest. Never trim or normalize an immutable snapshot identity.
func ValidCompletionCheckDigest(value string) bool {
	if value == "" {
		return true
	}
	if len(value) != 64 {
		return false
	}
	for _, c := range value {
		if !(c >= '0' && c <= '9') && !(c >= 'a' && c <= 'f') {
			return false
		}
	}
	return true
}
