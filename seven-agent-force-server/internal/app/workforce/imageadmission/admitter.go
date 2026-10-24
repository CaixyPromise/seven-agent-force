package imageadmission

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"os/exec"
	"sort"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	principalinfra "github.com/CaixyPromise/seven-agent/force/internal/app/workforce/principal/infrastructure"
	"github.com/CaixyPromise/seven-agent/force/internal/app/workforce/runtimeconfig"
	"github.com/CaixyPromise/seven-agent/force/internal/worker/agentprotocol"
)

type Report struct {
	ImageRef                string           `json:"imageRef"`
	ImageDigest             string           `json:"imageDigest"`
	Contract                string           `json:"contract"`
	SupervisorVersion       string           `json:"supervisorVersion"`
	HarnessType             string           `json:"harnessType"`
	HarnessVersion          string           `json:"harnessVersion"`
	Probe                   map[string]any   `json:"probe"`
	Checks                  []string         `json:"checks"`
	SchedulingScope         string           `json:"schedulingScope"`
	SchedulingPriority      int              `json:"schedulingPriority"`
	ProviderProtocols       []string         `json:"providerProtocols"`
	EnvironmentCapabilities []string         `json:"environmentCapabilities"`
	HookEvents              []string         `json:"hookEvents"`
	ExecutionKinds          []string         `json:"executionKinds"`
	SchedulingDigest        string           `json:"schedulingDigest"`
	Metadata                ManifestMetadata `json:"metadata"`
	Manifest                json.RawMessage  `json:"manifest"`
}

type ManifestMetadata struct {
	Name        string `json:"name"`
	Version     string `json:"version"`
	Description string `json:"description"`
}

type Admitter struct {
	db       *pgxpool.Pool
	workerID int64
	idGen    IDGenerator
}
type IDGenerator interface{ NextID() int64 }

// An unavailable probe provides no verdict about an immutable image. Inventory
// refresh must retry it without replacing the last authoritative admission.
var ErrInspectionUnavailable = errors.New("IMAGE_INSPECTION_UNAVAILABLE")

type contractManifest struct {
	SchemaVersion  int              `json:"schemaVersion"`
	Kind           string           `json:"kind"`
	Contract       string           `json:"contract"`
	Role           string           `json:"role"`
	ExecutionKinds []string         `json:"executionKinds"`
	Metadata       ManifestMetadata `json:"metadata"`
	Scheduling     struct {
		Scope                   string   `json:"scope"`
		Priority                int      `json:"priority"`
		EnvironmentCapabilities []string `json:"environmentCapabilities"`
		ProviderProtocols       []string `json:"providerProtocols"`
		HookEvents              []string `json:"hookEvents"`
	} `json:"scheduling"`
	Supervisor struct {
		Executable    string `json:"executable"`
		Version       string `json:"version"`
		BuildRevision string `json:"buildRevision"`
	} `json:"supervisor"`
	Harness struct {
		Type         string `json:"type"`
		Adapter      string `json:"adapter"`
		Executable   string `json:"executable"`
		Version      string `json:"version"`
		BinarySHA256 string `json:"binarySha256"`
	} `json:"harness"`
	Reviewer struct {
		HarnessType                 string `json:"harnessType"`
		Adapter                     string `json:"adapter"`
		ReportSchema                string `json:"reportSchema"`
		ProviderIdentitySource      string `json:"providerIdentitySource"`
		VerdictSource               string `json:"verdictSource"`
		TaskVerdictSelection        string `json:"taskVerdictSelection"`
		EnvironmentVerdictSelection string `json:"environmentVerdictSelection"`
	} `json:"reviewer"`
	ReviewFilesystem struct {
		ResultRepository struct {
			Path    string `json:"path"`
			Access  string `json:"access"`
			Storage string `json:"storage"`
		} `json:"resultRepository"`
		Scratch struct {
			Path        string `json:"path"`
			Access      string `json:"access"`
			Storage     string `json:"storage"`
			CopyOnWrite bool   `json:"copyOnWrite"`
		} `json:"scratch"`
		Input struct {
			Path      string `json:"path"`
			Access    string `json:"access"`
			Storage   string `json:"storage"`
			NoExec    bool   `json:"noexec"`
			RunScoped bool   `json:"runScoped"`
		} `json:"input"`
		Report struct {
			Path      string `json:"path"`
			Access    string `json:"access"`
			Storage   string `json:"storage"`
			RunScoped bool   `json:"runScoped"`
		} `json:"report"`
		ProviderRelay struct {
			Path      string `json:"path"`
			Access    string `json:"access"`
			Storage   string `json:"storage"`
			RunScoped bool   `json:"runScoped"`
		} `json:"providerRelay"`
	} `json:"reviewFilesystem"`
	ResultProcessor struct {
		PythonRuntimeIdentity string   `json:"pythonRuntimeIdentity"`
		BootstrapDigest       string   `json:"bootstrapSha256"`
		SDKDigest             string   `json:"sdkSha256"`
		ModuleIndexDigest     string   `json:"moduleIndexSha256"`
		ModuleRoots           []string `json:"moduleRoots"`
	} `json:"resultProcessor"`
	Capabilities []string `json:"capabilities"`
}

func New(db *pgxpool.Pool, workerID int64, idGen IDGenerator) *Admitter {
	return &Admitter{db: db, workerID: workerID, idGen: idGen}
}

// Inspect performs Worker-local admission without persisting business state.
// The resulting evidence is reported to the Server through the Worker Machine
// API, which remains the only owner of scheduling state.
func Inspect(ctx context.Context, cfg config.HarnessConfig) (Report, error) {
	return (&Admitter{}).inspectAndProbe(ctx, cfg)
}

func (a *Admitter) Admit(ctx context.Context, cfg config.HarnessConfig) (Report, error) {
	if a.idGen == nil {
		return Report{}, errors.New("workforce ID generator is not configured")
	}
	candidateID := a.idGen.NextID()
	_, err := a.db.Exec(ctx, `INSERT INTO workforce_image_candidates (id,requested_ref,requested_harness_type,status) VALUES ($1,$2,'unknown','STATIC_CHECKING')`, candidateID, cfg.Image)
	if err != nil {
		return Report{}, err
	}
	report, admitErr := a.inspectAndProbe(ctx, cfg)
	verdict := "ADMITTED"
	errorCode := ""
	if admitErr != nil {
		verdict, errorCode = "REJECTED", stableCode(admitErr)
	}
	probeJSON, _ := json.Marshal(report.Probe)
	checksJSON, _ := json.Marshal(report.Checks)
	runID := a.idGen.NextID()
	_, dbErr := a.db.Exec(ctx, `
INSERT INTO workforce_image_admission_runs (id,candidate_id,worker_id,image_digest,contract_version,supervisor_version,harness_type,harness_version,observed_probe_json,checks_json,verdict,error_code,finished_at)
VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,NULLIF($12,''),CURRENT_TIMESTAMP)`,
		runID, candidateID, a.workerID, report.ImageDigest, report.Contract, report.SupervisorVersion, report.HarnessType, report.HarnessVersion, nullJSON(probeJSON), checksJSON, verdict, errorCode)
	if dbErr != nil {
		return Report{}, dbErr
	}
	_, _ = a.db.Exec(ctx, `UPDATE workforce_image_candidates SET resolved_digest=NULLIF($2,''),requested_harness_type=COALESCE(NULLIF($5,''),requested_harness_type),status=$3,rejection_code=NULLIF($4,''),updated_at=CURRENT_TIMESTAMP WHERE id=$1`, candidateID, report.ImageDigest, verdict, errorCode, report.HarnessType)
	if admitErr != nil {
		return Report{}, admitErr
	}
	evidence, _ := json.Marshal(map[string]any{"admissionRunId": runID, "checks": report.Checks, "probe": report.Probe, "localOnly": cfg.AllowLocalImage})
	declared, err := probeCapabilities(report.Probe)
	if err != nil {
		return report, err
	}
	capabilities, _ := json.Marshal(declared)
	environmentCapabilities, _ := json.Marshal(report.EnvironmentCapabilities)
	providerProtocols, _ := json.Marshal(report.ProviderProtocols)
	hookEvents, _ := json.Marshal(report.HookEvents)
	executionKinds, _ := json.Marshal(report.ExecutionKinds)
	releaseID := a.idGen.NextID()
	err = a.db.QueryRow(ctx, `
INSERT INTO workforce_harness_releases (id,alias,image_ref,image_digest,contract_version,supervisor_version,harness_type,harness_version,capabilities_json,scheduling_scope,scheduling_priority,provider_protocols_json,environment_capabilities_json,hook_events_json,execution_kinds_json,scheduling_digest,manifest_json,admission_evidence_json,status)
VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18,'ACTIVE')
ON CONFLICT (image_digest) DO UPDATE SET image_ref=EXCLUDED.image_ref,contract_version=EXCLUDED.contract_version,supervisor_version=EXCLUDED.supervisor_version,harness_type=EXCLUDED.harness_type,harness_version=EXCLUDED.harness_version,capabilities_json=EXCLUDED.capabilities_json,scheduling_scope=EXCLUDED.scheduling_scope,scheduling_priority=EXCLUDED.scheduling_priority,provider_protocols_json=EXCLUDED.provider_protocols_json,environment_capabilities_json=EXCLUDED.environment_capabilities_json,hook_events_json=EXCLUDED.hook_events_json,execution_kinds_json=EXCLUDED.execution_kinds_json,scheduling_digest=EXCLUDED.scheduling_digest,manifest_json=EXCLUDED.manifest_json,admission_evidence_json=EXCLUDED.admission_evidence_json,status='ACTIVE',updated_at=CURRENT_TIMESTAMP
RETURNING id`,
		releaseID, cfg.Alias, cfg.Image, report.ImageDigest, report.Contract, report.SupervisorVersion, report.HarnessType, report.HarnessVersion, capabilities, report.SchedulingScope, report.SchedulingPriority, providerProtocols, environmentCapabilities, hookEvents, executionKinds, report.SchedulingDigest, report.Manifest, evidence).Scan(&releaseID)
	if err != nil {
		return Report{}, err
	}
	tx, err := a.db.Begin(ctx)
	if err != nil {
		return Report{}, err
	}
	defer tx.Rollback(ctx)
	principalID, err := principalinfra.EnsureWorker(ctx, tx, a.idGen, a.workerID)
	if err != nil {
		return Report{}, err
	}
	_, err = tx.Exec(ctx, `INSERT INTO workforce_harness_channels (alias,harness_release_id,updated_by_principal_id) VALUES ($1,$2,$3) ON CONFLICT(alias) DO UPDATE SET harness_release_id=EXCLUDED.harness_release_id,updated_by_principal_id=EXCLUDED.updated_by_principal_id,updated_at=CURRENT_TIMESTAMP`, cfg.Alias, releaseID, principalID)
	if err != nil {
		return Report{}, err
	}
	if err = tx.Commit(ctx); err != nil {
		return Report{}, err
	}
	return report, nil
}

func (a *Admitter) inspectAndProbe(ctx context.Context, cfg config.HarnessConfig) (Report, error) {
	inspectCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	output, err := exec.CommandContext(inspectCtx, "docker", "image", "inspect", cfg.Image).Output()
	if err != nil {
		return Report{ImageRef: cfg.Image}, fmt.Errorf("IMAGE_INSPECT_FAILED: %w", err)
	}
	var images []struct {
		ID     string `json:"Id"`
		Config struct {
			User         string            `json:"User"`
			Entrypoint   []string          `json:"Entrypoint"`
			ExposedPorts map[string]any    `json:"ExposedPorts"`
			Volumes      map[string]any    `json:"Volumes"`
			Labels       map[string]string `json:"Labels"`
		} `json:"Config"`
	}
	if err := json.Unmarshal(output, &images); err != nil || len(images) != 1 {
		return Report{ImageRef: cfg.Image}, errors.New("IMAGE_INSPECT_INVALID: invalid Docker inspect output")
	}
	image := images[0]
	report := Report{ImageRef: cfg.Image, ImageDigest: image.ID, Checks: []string{"image.inspect"}}
	if image.Config.User == "" || image.Config.User == "0" || strings.HasPrefix(image.Config.User, "root") {
		return report, errors.New("IMAGE_ROOT_USER_FORBIDDEN: image user must be non-root")
	}
	if len(image.Config.ExposedPorts) != 0 {
		return report, errors.New("IMAGE_PORT_DECLARATION_FORBIDDEN: ExposedPorts must be empty")
	}
	if len(image.Config.Volumes) != 0 {
		return report, errors.New("IMAGE_VOLUME_DECLARATION_FORBIDDEN: declared Volumes must be empty")
	}
	if len(image.Config.Entrypoint) != 1 || image.Config.Entrypoint[0] != "/usr/local/bin/seven-worker-cli" {
		return report, errors.New("IMAGE_ENTRYPOINT_INVALID: expected seven-worker-cli entrypoint")
	}
	if image.Config.Labels["io.github.caixypromise.seven-worker.image.contract"] != cfg.Contract {
		return report, errors.New("IMAGE_CONTRACT_INVALID: contract label mismatch")
	}
	report.Checks = append(report.Checks, "non_root", "no_exposed_ports", "no_declared_volumes", "entrypoint", "contract_label")
	manifestOutput, err := exec.CommandContext(inspectCtx, "docker", "run", "--rm", "--network", "none", "--read-only", "--cap-drop", "ALL", "--security-opt", "no-new-privileges", "--entrypoint", "/bin/cat", image.ID, "/opt/seven-worker/contract/manifest.json").Output()
	if err != nil || len(manifestOutput) > 1<<20 {
		return report, errors.New("IMAGE_CONTRACT_MISSING: contract manifest is unreadable or too large")
	}
	var manifest contractManifest
	if err := json.Unmarshal(manifestOutput, &manifest); err != nil {
		return report, fmt.Errorf("IMAGE_CONTRACT_INVALID: %w", err)
	}
	descriptor, err := agentprotocol.ParseDescriptor(manifestOutput)
	if err != nil {
		return report, errors.New("IMAGE_ADAPTER_CONTRACT_INVALID")
	}
	if manifest.SchemaVersion != 1 || manifest.Kind != "SevenAttemptImage" || manifest.Supervisor.Executable != "/usr/local/bin/seven-worker-cli" || !validHarnessName(manifest.Harness.Type) || manifest.Harness.Version == "" || len(manifest.Harness.BinarySHA256) != 64 || len(manifest.ResultProcessor.PythonRuntimeIdentity) != 64 || len(manifest.ResultProcessor.BootstrapDigest) != 64 || len(manifest.ResultProcessor.SDKDigest) != 64 || len(manifest.ResultProcessor.ModuleIndexDigest) != 64 || len(manifest.ResultProcessor.ModuleRoots) == 0 {
		return report, errors.New("IMAGE_CONTRACT_INVALID: required manifest fields do not match a supported v1 Harness profile")
	}
	scheduling, err := normalizeScheduling(manifest.Scheduling.Scope, manifest.Scheduling.Priority, manifest.Scheduling.ProviderProtocols, manifest.Scheduling.EnvironmentCapabilities, manifest.Scheduling.HookEvents, manifest.ExecutionKinds)
	if err != nil {
		return report, err
	}
	report.SchedulingScope = scheduling.Scope
	report.SchedulingPriority = scheduling.Priority
	report.ProviderProtocols = scheduling.ProviderProtocols
	report.EnvironmentCapabilities = scheduling.EnvironmentCapabilities
	report.HookEvents = scheduling.HookEvents
	report.ExecutionKinds = scheduling.ExecutionKinds
	report.SchedulingDigest = scheduling.Digest
	report.Metadata = manifest.Metadata
	report.Manifest = append(json.RawMessage(nil), manifestOutput...)
	if err := verifyEnvironmentCapabilities(ctx, image.ID, scheduling.EnvironmentCapabilities); err != nil {
		return report, err
	}
	if manifest.Reviewer.ReportSchema != "" {
		if err := validateReviewerManifest(manifest, cfg.Contract); err != nil {
			return report, err
		}
	}
	manifestSum := fmt.Sprintf("%x", sha256.Sum256(manifestOutput))
	report.Checks = append(report.Checks, "contract_manifest", "scheduling_contract")
	probeCtx, probeCancel := context.WithTimeout(ctx, 15*time.Second)
	defer probeCancel()
	probeOutput, err := exec.CommandContext(probeCtx, "docker", "run", "--rm", "--network", "none", "--read-only", "--cap-drop", "ALL", "--security-opt", "no-new-privileges", "--pids-limit", "64", image.ID, "image", "probe", "--json").Output()
	if err != nil {
		return report, fmt.Errorf("IMAGE_PROBE_INVALID: %w", err)
	}
	if len(probeOutput) > 1<<20 {
		return report, errors.New("IMAGE_PROBE_INVALID: output exceeds 1 MiB")
	}
	if err := json.Unmarshal(probeOutput, &report.Probe); err != nil {
		return report, fmt.Errorf("IMAGE_PROBE_INVALID: %w", err)
	}
	declared, err := probeCapabilities(report.Probe)
	if err != nil || !sameStrings(declared, manifest.Capabilities) {
		return report, errors.New("IMAGE_CAPABILITIES_INVALID")
	}
	contract, _ := report.Probe["contract"].(string)
	if contract != cfg.Contract {
		return report, errors.New("IMAGE_PROTOCOL_INCOMPATIBLE: probe contract mismatch")
	}
	protocols, _ := report.Probe["protocols"].(map[string]any)
	if !protocolVersion(protocols["attemptControl"], 1) || !protocolVersion(protocols["managedHookEvent"], 1) || !protocolVersion(protocols["supervisorControl"], 2) {
		return report, errors.New("IMAGE_PROTOCOL_INCOMPATIBLE: managed Hook Attempt control protocols are missing")
	}
	probeManifestHash, _ := report.Probe["manifestSha256"].(string)
	if probeManifestHash != manifestSum {
		return report, errors.New("IMAGE_PROTOCOL_INCOMPATIBLE: probe manifest hash mismatch")
	}
	verified, _ := report.Probe["adapterHandshakeVerified"].(bool)
	actualAdapter, _ := json.Marshal(report.Probe["agentAdapter"])
	expectedAdapter, _ := json.Marshal(descriptor)
	if !verified || !bytes.Equal(actualAdapter, expectedAdapter) {
		// Compare typed JSON rather than map key order.
		var actual agentprotocol.Descriptor
		if !verified || json.Unmarshal(actualAdapter, &actual) != nil {
			return report, errors.New("IMAGE_ADAPTER_HANDSHAKE_INVALID")
		}
		normalized, _ := json.Marshal(actual)
		if !bytes.Equal(normalized, expectedAdapter) {
			return report, errors.New("IMAGE_ADAPTER_HANDSHAKE_INVALID")
		}
	}
	probeProcessor, probeProcessorOK := report.Probe["resultProcessor"]
	probeProcessorJSON, _ := json.Marshal(probeProcessor)
	var probeProcessorIdentity struct {
		PythonRuntimeIdentity string   `json:"pythonRuntimeIdentity"`
		BootstrapDigest       string   `json:"bootstrapSha256"`
		SDKDigest             string   `json:"sdkSha256"`
		ModuleIndexDigest     string   `json:"moduleIndexSha256"`
		ModuleRoots           []string `json:"moduleRoots"`
	}
	if !probeProcessorOK || json.Unmarshal(probeProcessorJSON, &probeProcessorIdentity) != nil || probeProcessorIdentity.PythonRuntimeIdentity != manifest.ResultProcessor.PythonRuntimeIdentity || probeProcessorIdentity.BootstrapDigest != manifest.ResultProcessor.BootstrapDigest || probeProcessorIdentity.SDKDigest != manifest.ResultProcessor.SDKDigest || probeProcessorIdentity.ModuleIndexDigest != manifest.ResultProcessor.ModuleIndexDigest || !sameStrings(probeProcessorIdentity.ModuleRoots, manifest.ResultProcessor.ModuleRoots) {
		return report, errors.New("IMAGE_PROTOCOL_INCOMPATIBLE: Result Processor runtime identity mismatch")
	}
	report.Contract = contract
	supervisor, _ := report.Probe["supervisor"].(map[string]any)
	report.SupervisorVersion, _ = supervisor["version"].(string)
	harness, _ := report.Probe["harness"].(map[string]any)
	report.HarnessType, _ = harness["type"].(string)
	report.HarnessVersion, _ = harness["version"].(string)
	probeBinaryHash, _ := harness["binarySha256"].(string)
	if report.SupervisorVersion == "" || report.SupervisorVersion != manifest.Supervisor.Version || report.HarnessType != manifest.Harness.Type || report.HarnessVersion != manifest.Harness.Version || probeBinaryHash != manifest.Harness.BinarySHA256 {
		return report, errors.New("IMAGE_PROBE_INVALID: required version fields are missing")
	}
	report.Checks = append(report.Checks, "isolated_probe", "protocol_compatible", "manifest_probe_consistent", "result_processor_runtime_identity")
	if err := runProcessorBoundaryConformance(ctx, report.ImageDigest); err != nil {
		return report, err
	}
	report.Checks = append(report.Checks, "adapter_handshake", "processor_boundary_conformance", "attempt_private_write")
	return report, nil
}

func validateReviewerManifest(manifest contractManifest, expectedContract string) error {
	if manifest.Contract != expectedContract || manifest.Role != "attempt" || manifest.Reviewer.HarnessType != manifest.Harness.Type || manifest.Reviewer.Adapter != manifest.Harness.Adapter || manifest.Reviewer.ReportSchema != "review-report/v1" || manifest.Reviewer.ProviderIdentitySource != "task.model-credential" || manifest.Reviewer.TaskVerdictSelection != "rejected" || manifest.Reviewer.EnvironmentVerdictSelection != "rejected" || !containsString(manifest.ExecutionKinds, "AGENT_ATTEMPT") || !containsString(manifest.ExecutionKinds, "REVIEW_RUN") {
		return errors.New("IMAGE_REVIEW_CONTRACT_INVALID: reviewer identity or execution kinds are not fixed")
	}
	for _, capability := range []string{"review.provider.gateway-client", "review.fixed-provider-identity"} {
		if !containsString(manifest.Capabilities, capability) {
			return fmt.Errorf("IMAGE_REVIEW_CONTRACT_INVALID: reviewer capability %s is missing", capability)
		}
	}
	if manifest.ReviewFilesystem.ResultRepository.Path != "/result/repository.git" || manifest.ReviewFilesystem.ResultRepository.Access != "read_only" || manifest.ReviewFilesystem.ResultRepository.Storage != "bind" ||
		manifest.ReviewFilesystem.Scratch.Path != "/scratch" || manifest.ReviewFilesystem.Scratch.Access != "read_write" || manifest.ReviewFilesystem.Scratch.Storage != "bind" || !manifest.ReviewFilesystem.Scratch.CopyOnWrite ||
		manifest.ReviewFilesystem.Input.Path != "/review/input.json" || manifest.ReviewFilesystem.Input.Access != "read_only" || manifest.ReviewFilesystem.Input.Storage != "bind" || !manifest.ReviewFilesystem.Input.NoExec || !manifest.ReviewFilesystem.Input.RunScoped ||
		manifest.ReviewFilesystem.Report.Path != "/report" || manifest.ReviewFilesystem.Report.Access != "read_write" || manifest.ReviewFilesystem.Report.Storage != "volume" || !manifest.ReviewFilesystem.Report.RunScoped ||
		manifest.ReviewFilesystem.ProviderRelay.Path != "/provider/review-gateway.sock" || manifest.ReviewFilesystem.ProviderRelay.Access != "read_only" || manifest.ReviewFilesystem.ProviderRelay.Storage != "volume" || !manifest.ReviewFilesystem.ProviderRelay.RunScoped {
		return errors.New("IMAGE_REVIEW_CONTRACT_INVALID: reviewer filesystem contract is incomplete")
	}
	return nil
}

func containsString(values []string, expected string) bool {
	for _, value := range values {
		if value == expected {
			return true
		}
	}
	return false
}

func protocolVersionOne(value any) bool {
	return protocolVersion(value, 1)
}

func sameStrings(left, right []string) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		if left[index] != right[index] {
			return false
		}
	}
	return true
}

func protocolVersion(value any, expected float64) bool {
	rangeValue, ok := value.(map[string]any)
	if !ok {
		return false
	}
	minimum, minOK := rangeValue["min"].(float64)
	maximum, maxOK := rangeValue["max"].(float64)
	return minOK && maxOK && minimum == expected && maximum == expected
}

func validHarnessName(value string) bool {
	if len(value) == 0 || len(value) > 64 {
		return false
	}
	for _, c := range value {
		if !(c >= 'a' && c <= 'z' || c >= '0' && c <= '9' || c == '-' || c == '_') {
			return false
		}
	}
	return true
}

type normalizedScheduling struct {
	Scope                   string   `json:"scope"`
	Priority                int      `json:"priority"`
	ProviderProtocols       []string `json:"providerProtocols"`
	EnvironmentCapabilities []string `json:"environmentCapabilities"`
	HookEvents              []string `json:"hookEvents"`
	ExecutionKinds          []string `json:"executionKinds"`
	Digest                  string   `json:"-"`
}

func normalizeScheduling(scope string, priority int, providerProtocols, environmentCapabilities, hookEvents, executionKinds []string) (normalizedScheduling, error) {
	scope = strings.ToUpper(strings.TrimSpace(scope))
	if scope != "PRODUCTION" && scope != "TEST_ONLY" || priority < -100000 || priority > 100000 {
		return normalizedScheduling{}, errors.New("IMAGE_SCHEDULING_INVALID: invalid scope or priority")
	}
	providerProtocols, err := normalizeProtocolNames(providerProtocols)
	if err != nil || len(providerProtocols) == 0 {
		return normalizedScheduling{}, errors.New("IMAGE_SCHEDULING_INVALID: provider protocols are invalid")
	}
	environmentCapabilities, err = normalizeSchedulingNames(environmentCapabilities, false)
	if err != nil {
		return normalizedScheduling{}, err
	}
	hookEvents, err = normalizeHookEvents(hookEvents)
	if err != nil {
		return normalizedScheduling{}, errors.New("IMAGE_SCHEDULING_INVALID: Hook events are invalid")
	}
	executionKinds, err = normalizeSchedulingNames(executionKinds, true)
	if err != nil || len(executionKinds) == 0 {
		return normalizedScheduling{}, errors.New("IMAGE_SCHEDULING_INVALID: execution kinds are invalid")
	}
	value := normalizedScheduling{Scope: scope, Priority: priority, ProviderProtocols: providerProtocols, EnvironmentCapabilities: environmentCapabilities, HookEvents: hookEvents, ExecutionKinds: executionKinds}
	body, _ := json.Marshal(value)
	value.Digest = fmt.Sprintf("%x", sha256.Sum256(body))
	return value, nil
}

func normalizeHookEvents(values []string) ([]string, error) {
	if len(values) > 128 {
		return nil, errors.New("IMAGE_SCHEDULING_INVALID: too many Hook events")
	}
	seen := make(map[string]bool, len(values))
	result := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" || len(value) > 128 || seen[value] {
			return nil, errors.New("IMAGE_SCHEDULING_INVALID: Hook events are invalid")
		}
		for index, character := range value {
			if character >= 'a' && character <= 'z' || character >= 'A' && character <= 'Z' || character >= '0' && character <= '9' || (index > 0 && (character == '.' || character == '_' || character == '-')) {
				continue
			}
			return nil, errors.New("IMAGE_SCHEDULING_INVALID: Hook events are invalid")
		}
		seen[value] = true
		result = append(result, value)
	}
	sort.Strings(result)
	return result, nil
}

func normalizeProtocolNames(values []string) ([]string, error) {
	if len(values) == 0 || len(values) > 2 {
		return nil, errors.New("IMAGE_SCHEDULING_INVALID: provider protocols are invalid")
	}
	seen := map[string]bool{}
	result := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.ToUpper(strings.TrimSpace(value))
		if value != "OPENAI" && value != "ANTHROPIC" || seen[value] {
			return nil, errors.New("IMAGE_SCHEDULING_INVALID: provider protocols are invalid")
		}
		seen[value] = true
		result = append(result, value)
	}
	sort.Strings(result)
	return result, nil
}

func verifyEnvironmentCapabilities(ctx context.Context, imageID string, capabilities []string) error {
	commands := map[string]string{
		"language.node": "command -v node", "language.go": "command -v go", "language.java": "command -v java",
		"language.python": "command -v python3", "package-manager.pnpm": "command -v pnpm", "package-manager.npm": "command -v npm",
		"build-tool.maven": "command -v mvn", "build-tool.gradle": "command -v gradle", "shell.posix": "command -v sh",
		"browser.chromium": "command -v chromium || command -v chromium-browser",
	}
	for _, capability := range capabilities {
		command, known := commands[capability]
		if !known {
			continue
		}
		// Include Docker container creation and cold-start overhead, just as the
		// full isolated image probe does. A missed deadline still has no verdict.
		probeCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
		err := exec.CommandContext(probeCtx, "docker", "run", "--rm", "--network", "none", "--read-only", "--cap-drop", "ALL", "--security-opt", "no-new-privileges", "--entrypoint", "/bin/sh", imageID, "-c", command).Run()
		probeErr := probeCtx.Err()
		cancel()
		if err != nil {
			return environmentProbeFailure(capability, err, probeErr)
		}
	}
	return nil
}

func environmentProbeFailure(capability string, commandErr, contextErr error) error {
	var exit *exec.ExitError
	if contextErr == nil && errors.As(commandErr, &exit) && exit.ExitCode() == 1 {
		return fmt.Errorf("IMAGE_ENVIRONMENT_CAPABILITY_MISSING: %s", capability)
	}
	return fmt.Errorf("%w: environment probe for %s did not complete", ErrInspectionUnavailable, capability)
}

func normalizeSchedulingNames(values []string, executionKinds bool) ([]string, error) {
	if len(values) > 128 {
		return nil, errors.New("IMAGE_SCHEDULING_INVALID: too many capability entries")
	}
	seen := make(map[string]bool, len(values))
	result := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if executionKinds {
			value = strings.ToUpper(value)
			if value != "AGENT_ATTEMPT" && value != "REVIEW_RUN" {
				return nil, errors.New("IMAGE_SCHEDULING_INVALID: unsupported execution kind")
			}
		} else {
			value = strings.ToLower(value)
			if value == "" || len(value) > 128 {
				return nil, errors.New("IMAGE_SCHEDULING_INVALID: invalid environment capability")
			}
			for _, c := range value {
				if !(c >= 'a' && c <= 'z' || c >= '0' && c <= '9' || c == '-' || c == '.' || c == '_') {
					return nil, errors.New("IMAGE_SCHEDULING_INVALID: invalid environment capability")
				}
			}
		}
		if seen[value] {
			return nil, errors.New("IMAGE_SCHEDULING_INVALID: duplicate scheduling entry")
		}
		seen[value] = true
		result = append(result, value)
	}
	sort.Strings(result)
	return result, nil
}

func probeCapabilities(probe map[string]any) ([]string, error) {
	body, err := json.Marshal(probe["capabilities"])
	var entries []struct {
		Name   string `json:"name"`
		Status string `json:"status"`
	}
	if err != nil || json.Unmarshal(body, &entries) != nil || len(entries) == 0 || len(entries) > 128 {
		return nil, errors.New("IMAGE_CAPABILITIES_INVALID")
	}
	capabilities := make([]string, 0, len(entries))
	for _, entry := range entries {
		if entry.Status != "supported" {
			return nil, errors.New("IMAGE_CAPABILITIES_INVALID")
		}
		capabilities = append(capabilities, entry.Name)
	}
	seen := map[string]bool{}
	for _, capability := range capabilities {
		if len(capability) == 0 || len(capability) > 128 || seen[capability] {
			return nil, errors.New("IMAGE_CAPABILITIES_INVALID")
		}
		for _, c := range capability {
			if !(c >= 'a' && c <= 'z' || c >= '0' && c <= '9' || c == '-' || c == '.' || c == '_') {
				return nil, errors.New("IMAGE_CAPABILITIES_INVALID")
			}
		}
		seen[capability] = true
	}
	return capabilities, nil
}

func bounded(value []byte) string {
	if len(value) > 4096 {
		value = value[:4096]
	}
	return strings.TrimSpace(string(value))
}

func stableCode(err error) string {
	message := err.Error()
	if index := strings.Index(message, ":"); index > 0 && index < 80 {
		return message[:index]
	}
	return "IMAGE_CONFORMANCE_FAILED"
}

func nullJSON(value []byte) any {
	if len(value) == 0 || string(value) == "null" {
		return nil
	}
	return value
}
