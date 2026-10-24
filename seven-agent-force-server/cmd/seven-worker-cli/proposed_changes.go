package main

import (
	"context"
	"os/exec"
	"strings"
)

// proposedChangedFiles supplies an untrusted proposal for any AgentAdapter.
// Trusted result selection and verification remain the Finalizer's responsibility.
func proposedChangedFiles(ctx context.Context, repository string) []string {
	output, err := exec.CommandContext(ctx, "git", "-C", repository, "status", "--porcelain=v1", "-z", "--untracked-files=all").Output()
	if err != nil || len(output) > 4<<20 {
		return []string{}
	}
	result := make([]string, 0)
	for _, entry := range strings.Split(string(output), "\x00") {
		if len(entry) < 4 {
			continue
		}
		path := strings.TrimSpace(entry[3:])
		if path != "" {
			result = append(result, path)
		}
		if len(result) >= 10_000 {
			break
		}
	}
	return result
}
