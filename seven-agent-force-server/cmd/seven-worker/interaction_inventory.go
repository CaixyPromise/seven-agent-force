package main

import (
	"encoding/json"
	"github.com/CaixyPromise/seven-agent/force/internal/app/workforce/imageadmission"
	"github.com/CaixyPromise/seven-agent/force/internal/worker/agentprotocol"
)

// Called only after imageadmission.Inspect has bound the probe descriptor to
// the immutable image manifest and verified its native handshake. Publish that
// same descriptor at the Server registry reader's manifest.adapter boundary.
func admittedInteractionInventoryManifest(report imageadmission.Report) ([]byte, error) {
	verified, _ := report.Probe["adapterHandshakeVerified"].(bool)
	if !verified {
		return nil, agentprotocol.ErrManifest
	}
	body, err := json.Marshal(map[string]any{
		"harnessType": report.HarnessType, "harnessVersion": report.HarnessVersion,
		"supervisorVersion": report.SupervisorVersion, "metadata": report.Metadata, "probe": report.Probe,
		"adapter": report.Probe["agentAdapter"], "resultProcessor": report.Probe["resultProcessor"],
		"scheduling": map[string]any{
			"scope":                   report.SchedulingScope,
			"priority":                report.SchedulingPriority,
			"providerProtocols":       report.ProviderProtocols,
			"environmentCapabilities": report.EnvironmentCapabilities,
			"hookEvents":              report.HookEvents,
		},
		"executionKinds":   report.ExecutionKinds,
		"schedulingDigest": report.SchedulingDigest,
	})
	if err != nil {
		return nil, agentprotocol.ErrManifest
	}
	if _, err = agentprotocol.ParseDescriptor(body); err != nil {
		return nil, agentprotocol.ErrManifest
	}
	return body, nil
}
