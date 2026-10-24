package infrastructure

import "testing"

func TestSafeRemoteHostRejectsLocalAndAmbiguousIdentities(t *testing.T) {
	t.Parallel()
	for _, host := range []string{"", "localhost", "repo.local", "127.0.0.1", "10.0.0.1", "169.254.1.1", "::1", "git.example.com@127.0.0.1", "git.example.com/path"} {
		if safeRemoteHost(host) {
			t.Fatalf("safeRemoteHost(%q) = true", host)
		}
	}
	for _, host := range []string{"git.example.com", "8.8.8.8", "2001:4860:4860::8888"} {
		if !safeRemoteHost(host) {
			t.Fatalf("safeRemoteHost(%q) = false", host)
		}
	}
}
