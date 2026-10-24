package infrastructure

import (
	"encoding/hex"
	"encoding/json"
	"errors"
)

// admittedProcessorRuntime is intentionally a narrow projection of Harness
// Admission evidence. A Result Processor Fixture must use the same protected
// Python bootstrap/SDK identity that a Task freezes; accepting only an image
// digest would permit a stale or incomplete Harness release to execute user
// Processor code.
type admittedProcessorRuntime struct {
	PythonRuntimeIdentity string   `json:"pythonRuntimeIdentity"`
	BootstrapDigest       string   `json:"bootstrapSha256"`
	SDKDigest             string   `json:"sdkSha256"`
	ModuleIndexDigest     string   `json:"moduleIndexSha256"`
	ModuleRoots           []string `json:"moduleRoots"`
}

func requireAdmittedProcessorRuntime(evidence json.RawMessage) error {
	var envelope struct {
		Probe struct {
			ResultProcessor admittedProcessorRuntime `json:"resultProcessor"`
		} `json:"probe"`
	}
	if len(evidence) == 0 || json.Unmarshal(evidence, &envelope) != nil {
		return errors.New("fixture Harness Release admission evidence is invalid")
	}
	identity := envelope.Probe.ResultProcessor
	for _, value := range []string{
		identity.PythonRuntimeIdentity,
		identity.BootstrapDigest,
		identity.SDKDigest,
		identity.ModuleIndexDigest,
	} {
		if len(value) != 64 {
			return errors.New("fixture Harness Release lacks Result Processor runtime identity")
		}
		if _, err := hex.DecodeString(value); err != nil {
			return errors.New("fixture Harness Release has an invalid Result Processor runtime digest")
		}
	}
	if len(identity.ModuleRoots) == 0 {
		return errors.New("fixture Harness Release lacks admitted Result Processor module roots")
	}
	return nil
}
