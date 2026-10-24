package infrastructure

import (
	"encoding/hex"
	"strings"

	projectfacade "github.com/CaixyPromise/seven-agent/force/internal/app/workforce/project/facade"
)

// compatibleTaskRuntime is the shared pure admission rule. Worker availability,
// capacity and disk pressure remain scheduling decisions outside this rule.
func compatibleTaskRuntime(release catalogRelease, requirements projectfacade.ExecutionRequirements, hooks []string, protocol string) bool {
	return release.status == "ACTIVE" && release.scope == "PRODUCTION" &&
		requirements.ExecutionKind == "AGENT_ATTEMPT" && containsString(release.executionKinds, requirements.ExecutionKind) &&
		release.contract == "seven.worker-image/v1" && strings.HasPrefix(release.imageDigest, "sha256:") && validHexDigest(strings.TrimPrefix(release.imageDigest, "sha256:")) &&
		validRuntimeManifest(release.manifest) && containsAll(release.manifest.Adapter.Capabilities, requirements.AdapterCapabilities) &&
		containsAll(release.environment, requirements.EnvironmentCapabilities) &&
		(protocol == "OPENAI" || protocol == "ANTHROPIC") && containsString(release.protocols, protocol) && containsAll(release.hookEvents, hooks)
}

func validHexDigest(value string) bool {
	if len(value) != 64 || value != strings.ToLower(value) {
		return false
	}
	_, err := hex.DecodeString(value)
	return err == nil
}
