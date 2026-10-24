package main

import (
	"bufio"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	continuationfacade "github.com/CaixyPromise/seven-agent/force/internal/app/workforce/continuation/facade"
	dispatchdomain "github.com/CaixyPromise/seven-agent/force/internal/app/workforce/taskdispatch/domain"
	"github.com/CaixyPromise/seven-agent/force/internal/shared/buildinfo"
	"github.com/CaixyPromise/seven-agent/force/internal/worker/agentprotocol"
	"github.com/CaixyPromise/seven-agent/force/internal/worker/supervisorwire"
)

const (
	contractVersion = "seven.worker-image/v1"
	manifestPath    = "/opt/seven-worker/contract/manifest.json"
)

type attemptSpec struct {
	SchemaVersion              int                   `json:"schemaVersion"`
	TaskID                     int64                 `json:"taskId,string"`
	ProjectVersionID           int64                 `json:"projectVersionId,string"`
	AttemptID                  int64                 `json:"attemptId,string"`
	Generation                 int64                 `json:"generation,string"`
	OwnerEpoch                 int64                 `json:"ownerEpoch,string"`
	AuthorityExpiresAt         string                `json:"authorityExpiresAt,omitempty"`
	Instruction                string                `json:"instruction"`
	Workspace                  string                `json:"workspace"`
	Output                     string                `json:"output"`
	State                      string                `json:"state"`
	Runtime                    string                `json:"runtime"`
	Temporary                  string                `json:"temporary"`
	Hooks                      *hookSpec             `json:"hooks,omitempty"`
	ResultProcessor            *resultProcessorSpec  `json:"resultProcessor,omitempty"`
	Provider                   *providerFacadeSpec   `json:"provider,omitempty"`
	Continuation               *continuationSpec     `json:"continuation,omitempty"`
	ReviewCorrection           *reviewCorrectionSpec `json:"reviewCorrection,omitempty"`
	InteractionResponse        json.RawMessage       `json:"interactionResponse,omitempty"`
	InteractionQuestionRequest json.RawMessage       `json:"interactionQuestionRequest,omitempty"`
	OperationToolResult        json.RawMessage       `json:"operationToolResult,omitempty"`
	AllowedAdapterTools        []string              `json:"allowedAdapterTools,omitempty"`
}

type reviewCorrectionSpec struct {
	SourceReviewRunID int64  `json:"sourceReviewRunId,string"`
	RevisionKind      string `json:"revisionKind"`
	Verdict           string `json:"verdict,omitempty"`
	ReportPath        string `json:"reportPath,omitempty"`
	ReportDigest      string `json:"reportDigest,omitempty"`
	ReasonCode        string `json:"reasonCode,omitempty"`
}

type continuationSpec struct {
	SourceTurnID         int64  `json:"sourceTurnId,string"`
	TargetTurnID         int64  `json:"targetTurnId,string"`
	CheckpointID         int64  `json:"checkpointId,string"`
	SessionLineageDigest string `json:"sessionLineageDigest"`
	ResumeNonce          string `json:"resumeNonce"`
	ResumeNonceDigest    string `json:"resumeNonceDigest"`
}

// resultProcessorSpec is generated from the frozen TaskExecutionSnapshot. It
// contains no mutable binding, credential, or authoring source: the Worker
// materializes the exact read-only release root before Docker create.
type resultProcessorSpec struct {
	ReleaseID             int64           `json:"releaseId,string"`
	Digest                string          `json:"digest"`
	Root                  string          `json:"root"`
	Bootstrap             string          `json:"bootstrap"`
	Runtime               string          `json:"runtime"`
	PythonRuntimeIdentity string          `json:"pythonRuntimeIdentity"`
	BootstrapDigest       string          `json:"bootstrapDigest"`
	SDKDigest             string          `json:"sdkDigest"`
	ModuleIndexDigest     string          `json:"moduleIndexDigest"`
	Parameters            json.RawMessage `json:"parameters"`
	MaxEvaluationCycles   int             `json:"maxEvaluationCycles"`
}

type resultProcessorImageIdentity struct {
	PythonRuntimeIdentity string   `json:"pythonRuntimeIdentity"`
	BootstrapDigest       string   `json:"bootstrapSha256"`
	SDKDigest             string   `json:"sdkSha256"`
	ModuleIndexDigest     string   `json:"moduleIndexSha256"`
	ModuleRoots           []string `json:"moduleRoots"`
}

type providerFacadeSpec struct {
	Model        string `json:"model"`
	Protocol     string `json:"protocol"`
	Listen       string `json:"listen"`
	ProxyBaseURL string `json:"proxyBaseUrl"`
}

type harnessResult struct {
	Summary      string
	ChangedFiles []string
}

type supervisorFailure struct {
	code       string
	reasonCode string
	exitCode   int
}

func (e *supervisorFailure) Error() string { return e.code + ": " + e.reasonCode }

type supervisorState struct {
	mu                           sync.RWMutex
	State                        string `json:"state"`
	TaskID                       int64  `json:"taskId,string"`
	AttemptID                    int64  `json:"attemptId,string"`
	Generation                   int64  `json:"generation,string"`
	OwnerEpoch                   int64  `json:"ownerEpoch,string"`
	StartedAt                    string `json:"startedAt"`
	HarnessStartedAt             string `json:"harnessStartedAt,omitempty"`
	HarnessPID                   int    `json:"harnessPid,omitempty"`
	RunPermitID                  int64  `json:"runPermitId,string,omitempty"`
	RunPermitDigest              string `json:"runPermitDigest,omitempty"`
	RunNonceDigest               string `json:"runNonceDigest,omitempty"`
	AuthorityExpiresAt           string `json:"authorityExpiresAt,omitempty"`
	OrphanDeadline               string `json:"orphanDeadline,omitempty"`
	ReattachPermitID             int64  `json:"reattachPermitId,string,omitempty"`
	ReattachPermitDigest         string `json:"reattachPermitDigest,omitempty"`
	LastJournalSequence          int64  `json:"lastJournalSequence,string"`
	EvaluationCycleID            int64  `json:"evaluationCycleId,string,omitempty"`
	EvaluationCycle              int    `json:"evaluationCycle,omitempty"`
	EvaluationReadyAt            string `json:"evaluationReadyAt,omitempty"`
	EvaluationDigest             string `json:"evaluationDigest,omitempty"`
	EvaluationCandidate          string `json:"evaluationCandidate,omitempty"`
	EvaluationCandidateDigest    string `json:"evaluationCandidateDigest,omitempty"`
	EvaluationContinuationDigest string `json:"evaluationContinuationDigest,omitempty"`
	SessionDigest                string `json:"sessionDigest,omitempty"`
	Round                        int    `json:"round,omitempty"`
	authorityExpiry              time.Time
	watchdogGrace                time.Duration
	done                         chan error
	runningConfirmed             chan struct{}
	runningConfirmOnce           sync.Once
	evaluationContinue           chan evaluationContinuation
	journalWrite                 func(string, any) error
	sessionID                    string
	adapterProcess               *agentprotocol.Process
	observations                 []agentObservation
	observationSequence          int64
	observationGap               bool
	interaction                  supervisorInteractions
	sessionArchivePath           string
	sessionArchiveErr            error
	managedFeedback              string
	ctx                          context.Context
	cancel                       context.CancelFunc
}

type evaluationContinuation struct {
	Directive       string
	CycleID         int64
	Decision        string
	ReasonCode      string
	Message         string
	ManagedFeedback string
	ResumeNonce     string
}

type eventWriter struct {
	mu     sync.Mutex
	writer io.Writer
}

func (w *eventWriter) Emit(eventType string, payload any) error {
	w.mu.Lock()
	defer w.mu.Unlock()
	return json.NewEncoder(w.writer).Encode(map[string]any{"schemaVersion": 1, "type": eventType, "time": time.Now().UTC().Format(time.RFC3339Nano), "payload": payload})
}

var supervisorEvents = &eventWriter{writer: os.Stdout}

type attemptControlRequest struct {
	SchemaVersion       int             `json:"schemaVersion"`
	Command             string          `json:"command"`
	TaskID              int64           `json:"taskId,string,omitempty"`
	AttemptID           int64           `json:"attemptId,string,omitempty"`
	Generation          int64           `json:"generation,string,omitempty"`
	EventType           string          `json:"eventType,omitempty"`
	HookEvent           string          `json:"hookEvent,omitempty"`
	InvocationID        string          `json:"invocationId,omitempty"`
	ConfigurationDigest string          `json:"configurationDigest,omitempty"`
	HookName            string          `json:"hookName,omitempty"`
	PackageReleaseID    int64           `json:"packageReleaseId,string,omitempty"`
	PackageVersion      string          `json:"packageVersion,omitempty"`
	PackageDigest       string          `json:"packageDigest,omitempty"`
	ReasonCode          string          `json:"reasonCode,omitempty"`
	Diagnostic          string          `json:"diagnostic,omitempty"`
	DurationMS          int64           `json:"durationMs,omitempty"`
	OwnerEpoch          int64           `json:"ownerEpoch,string,omitempty"`
	RunPermitID         int64           `json:"runPermitId,string,omitempty"`
	RunPermitToken      string          `json:"runPermitToken,omitempty"`
	RunNonce            string          `json:"runNonce,omitempty"`
	ExpiresAt           string          `json:"expiresAt,omitempty"`
	PreviousOwnerEpoch  int64           `json:"previousOwnerEpoch,string,omitempty"`
	ReattachPermitID    int64           `json:"reattachPermitId,string,omitempty"`
	ReattachPermitToken string          `json:"reattachPermitToken,omitempty"`
	LeaseExpiresAt      string          `json:"leaseExpiresAt,omitempty"`
	LeaseReceiptDigest  string          `json:"leaseReceiptDigest,omitempty"`
	CycleID             int64           `json:"cycleId,string,omitempty"`
	CycleNumber         int             `json:"cycleNumber,omitempty"`
	Materialization     json.RawMessage `json:"materialization,omitempty"`
	Directive           string          `json:"directive,omitempty"`
	Decision            string          `json:"decision,omitempty"`
	Message             string          `json:"message,omitempty"`
	ManagedFeedback     string          `json:"managedFeedback,omitempty"`
	ResumeNonce         string          `json:"resumeNonce,omitempty"`
	CurrentRound        string          `json:"currentRound,omitempty"`
	SessionGeneration   int64           `json:"sessionGeneration,string,omitempty"`
	Method              string          `json:"method,omitempty"`
	Payload             json.RawMessage `json:"payload,omitempty"`
	After               int64           `json:"after,string,omitempty"`
	Through             int64           `json:"through,string,omitempty"`
}

type controlServer struct {
	spec            attemptSpec
	state           *supervisorState
	hooks           hookConfigurationSpec
	journal         *os.File
	mu              sync.Mutex
	journalMu       sync.Mutex
	startMu         sync.Mutex
	terminalBy      map[string]string
	run             harnessRunner
	journalSequence int64
}

type harnessStarted func(pid int) error
type harnessRunner func(context.Context, attemptSpec, *supervisorState, harnessStarted) error

type controlListener struct {
	net.Listener
	journal *os.File
}

func (l *controlListener) Close() error {
	err := l.Listener.Close()
	if l.journal != nil {
		if closeErr := l.journal.Close(); err == nil {
			err = closeErr
		}
	}
	return err
}

func main() {
	if filepath.Base(os.Args[0]) == "seven-hook" {
		if err := hookMain(os.Args[1:]); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		return
	}
	if len(os.Args) < 2 {
		usage()
		os.Exit(2)
	}
	switch os.Args[1] {
	case "check":
		if err := runCheck(os.Args[2:]); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
	case "version":
		fmt.Println(buildinfo.String())
	case "image":
		if len(os.Args) == 4 && os.Args[2] == "probe" && os.Args[3] == "--json" {
			if err := imageProbe(); err != nil {
				fmt.Fprintln(os.Stderr, err)
				os.Exit(1)
			}
			return
		}
		usage()
		os.Exit(2)
	case "supervise":
		if err := supervise(os.Args[2:]); err != nil {
			if errors.Is(err, context.Canceled) {
				return
			}
			emit("supervisor.failed", map[string]any{"error": err.Error()})
			var failure *supervisorFailure
			if errors.As(err, &failure) {
				os.Exit(failure.exitCode)
			}
			os.Exit(1)
		}
	case "review":
		if err := reviewMain(os.Args[2:]); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
	case "review-provider-relay":
		if err := fakeReviewProviderRelay(os.Args[2:]); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
	case "result-fixture":
		if err := resultFixture(os.Args[2:]); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
	case "result-deliver":
		if err := resultDeliver(os.Args[2:]); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
	case "delivery-egress-gateway":
		if err := deliveryEgressGateway(os.Args[2:]); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
	case "target-egress-gateway":
		if err := targetEgressGateway(os.Args[2:]); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
	case "delivery-egress-probe-target":
		if err := deliveryEgressProbeTarget(os.Args[2:]); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
	case "hook":
		if err := hookMain(os.Args[2:]); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
	case "control":
		if err := controlMain(os.Args[2:]); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
	case "interaction":
		if err := interactionMain(os.Args[2:]); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
	case "status", "interrupt", "diagnostics":
		if err := control(os.Args[1]); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
	default:
		usage()
		os.Exit(2)
	}
}

// reviewMain uses the admitted image Adapter for every Reviewer implementation.
func reviewMain(args []string) error {
	return runReviewAdapter(args)
}

func imageProbe() error {
	executable, err := os.Executable()
	if err != nil {
		return err
	}
	binaryHash, err := fileSHA256(executable)
	if err != nil {
		return err
	}
	manifestHash, err := fileSHA256(manifestPath)
	if err != nil {
		return fmt.Errorf("hash contract manifest: %w", err)
	}
	version := buildinfo.Version
	if version == "" || version == "dev" {
		version = "0.1.0-dev"
	}
	revision := buildinfo.Commit
	if len(revision) < 7 || strings.Contains(revision, "unknown") {
		revision = "0000000"
	}
	harnessType, adapter, executable, harnessVersion := harnessMetadata()
	harnessHash, err := fileSHA256(executable)
	if err != nil {
		return fmt.Errorf("hash Harness executable: %w", err)
	}
	resultProcessor, err := currentResultProcessorImageIdentity()
	if err != nil {
		return fmt.Errorf("verify Result Processor runtime identity: %w", err)
	}
	descriptor, err := loadAgentDescriptor()
	if err != nil {
		return err
	}
	probeContext, cancelProbe := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancelProbe()
	process, err := agentprotocol.OpenProcess(probeContext, descriptor.Executable, descriptor.Args, adapterEnvironment())
	if err != nil {
		return err
	}
	if err := process.ValidateReady(descriptor); err != nil {
		process.Close()
		return err
	}
	process.Close()
	manifestBody, err := os.ReadFile(manifestPath)
	if err != nil {
		return err
	}
	var declared struct {
		Capabilities []string `json:"capabilities"`
	}
	if json.Unmarshal(manifestBody, &declared) != nil || len(declared.Capabilities) > 64 {
		return agentprotocol.ErrManifest
	}
	capabilities := make([]map[string]string, 0, len(declared.Capabilities))
	for _, name := range declared.Capabilities {
		capabilities = append(capabilities, map[string]string{"name": name, "status": "supported"})
	}
	return json.NewEncoder(os.Stdout).Encode(map[string]any{
		"schemaVersion":            1,
		"kind":                     "SevenAttemptImageProbe",
		"contract":                 contractVersion,
		"manifestSha256":           manifestHash,
		"agentAdapter":             descriptor,
		"adapterHandshakeVerified": true,
		"supervisor":               map[string]any{"executable": "/usr/local/bin/seven-worker-cli", "version": version, "buildRevision": revision, "binarySha256": binaryHash},
		"harness":                  map[string]any{"type": harnessType, "adapter": adapter, "executable": executable, "version": harnessVersion, "binarySha256": harnessHash},
		"protocols": map[string]any{
			"attemptSpec": map[string]int{"min": 1, "max": 1}, "supervisorControl": map[string]int{"min": 2, "max": 2},
			"eventEnvelope": map[string]int{"min": 1, "max": 1}, "completionProposal": map[string]int{"min": 1, "max": 1},
			"hookInput": map[string]int{"min": 1, "max": 1}, "imageProbe": map[string]int{"min": 1, "max": 1},
			"attemptControl": map[string]int{"min": 1, "max": 1}, "reviewInput": map[string]int{"min": 1, "max": 1},
			"reviewReport": map[string]int{"min": 1, "max": 1}, "managedHookEvent": map[string]int{"min": 1, "max": 1},
		},
		"runtime":         map[string]any{"os": runtime.GOOS, "architecture": runtime.GOARCH, "uid": os.Getuid(), "gid": os.Getgid()},
		"resultProcessor": resultProcessor,
		"assetsVerified":  true, "supervisorHooksVerified": false, "codexManagedHooksVerified": false, "managedHooksVerified": false,
		"capabilities": capabilities,
	})
}

// fakeReviewProviderRelay is a test-only, review-scoped local Facade/Gateway.
// It runs in its own network-none sidecar and shares only a Docker volume UDS
// with its paired Reviewer. This works on Docker Desktop as well as Linux;
// Docker Desktop cannot pass a host-created Unix socket through an osxfs bind
// mount. It deliberately has no upstream URL, provider credential, or real
// Provider code; its only authority is the ephemeral file in that Run's
// private volume.
func fakeReviewProviderRelay(args []string) error {
	flags := flag.NewFlagSet("review-provider-relay", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	var socket, credentialID, credentialVersion, protocol, model, contract string
	flags.StringVar(&socket, "socket", "", "Unix socket path")
	flags.StringVar(&credentialID, "credential-id", "", "frozen model credential id")
	flags.StringVar(&credentialVersion, "credential-version", "", "model credential secret version")
	flags.StringVar(&protocol, "protocol", "", "frozen provider protocol")
	flags.StringVar(&model, "model", "", "frozen model")
	flags.StringVar(&contract, "contract", "", "frozen structured-output contract")
	if err := flags.Parse(args); err != nil || socket == "" || !positiveDecimalID(credentialID) || !positiveDecimalID(credentialVersion) || protocol != "OPENAI" || model == "" || contract != codexReviewStructuredContract || flags.NArg() != 0 {
		return errors.New("review provider relay contract is invalid")
	}
	if err := os.MkdirAll(filepath.Dir(socket), 0o700); err != nil {
		return err
	}
	if err := os.Remove(socket); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	listener, err := net.Listen("unix", socket)
	if err != nil {
		return err
	}
	defer listener.Close()
	if err = os.Chmod(socket, 0o600); err != nil {
		return err
	}
	authority, err := os.ReadFile(filepath.Join(filepath.Dir(socket), "review-authority"))
	if err != nil || len(strings.TrimSpace(string(authority))) != 64 {
		return errors.New("review provider authority is invalid")
	}
	authorityValue := strings.TrimSpace(string(authority))
	server := &http.Server{Handler: http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodPost || request.URL.Path != "/v1/responses" {
			writeFakeReviewRelayError(response, http.StatusNotFound, "NOT_FOUND")
			return
		}
		request.Body = http.MaxBytesReader(response, request.Body, 64<<10)
		defer request.Body.Close()
		var payload struct {
			Model string `json:"model"`
			Store *bool  `json:"store"`
		}
		if err := json.NewDecoder(request.Body).Decode(&payload); err != nil || payload.Model != model || (payload.Store != nil && *payload.Store) || request.Header.Get("X-Seven-Review-Authority") != authorityValue || request.Header.Get("X-Seven-Review-Credential-Id") != credentialID || request.Header.Get("X-Seven-Review-Credential-Version") != credentialVersion || request.Header.Get("X-Seven-Review-Protocol") != protocol || request.Header.Get("X-Seven-Review-Contract") != contract {
			writeFakeReviewRelayError(response, http.StatusForbidden, "REVIEW_PROVIDER_CONTRACT_REJECTED")
			return
		}
		// This is the only usage observation in Slice 4. It is a bounded,
		// allowlisted counter rather than a Provider response, diagnostic, or
		// credential-bearing trace. The Worker reads it before tearing down the
		// run-scoped sidecar and the Server validates it again on submission.
		_ = os.WriteFile("/provider/usage.json", []byte(`{"requests":1}`), 0o600)
		response.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(response, `{"id":"review-fake-provider","object":"response","output":[],"usage":{"input_tokens":0,"output_tokens":0}}`)
	})}
	err = server.Serve(listener)
	if errors.Is(err, http.ErrServerClosed) {
		return nil
	}
	return err
}

func validFakeReviewDigest(value string) bool {
	if len(value) != 64 {
		return false
	}
	for _, character := range value {
		if !(character >= 'a' && character <= 'f') && !(character >= '0' && character <= '9') {
			return false
		}
	}
	return true
}

func validFakeReviewProviderID(value string) bool {
	id, err := strconv.ParseInt(value, 10, 64)
	return err == nil && id > 0 && strconv.FormatInt(id, 10) == value
}

func writeFakeReviewRelayError(response http.ResponseWriter, status int, code string) {
	response.Header().Set("Content-Type", "application/json")
	response.WriteHeader(status)
	_, _ = io.WriteString(response, `{"error":{"code":"`+code+`"}}`)
}

const fakeReviewCorrectionMarker = "SEVEN_WORKER_REVIEW_CORRECTION.md"

// currentResultProcessorImageIdentity binds the protected Python runtime to
// the exact Harness image.  The Task snapshot carries these hashes and the
// Supervisor rechecks them before importing user code, so a tag update or a
// mutable runtime directory cannot silently change a published Processor.
func currentResultProcessorImageIdentity() (resultProcessorImageIdentity, error) {
	body, err := os.ReadFile(manifestPath)
	if err != nil {
		return resultProcessorImageIdentity{}, err
	}
	var manifest struct {
		ResultProcessor resultProcessorImageIdentity `json:"resultProcessor"`
	}
	decoder := json.NewDecoder(bytes.NewReader(body))
	if err := decoder.Decode(&manifest); err != nil {
		return resultProcessorImageIdentity{}, err
	}
	for name, value := range map[string]string{
		"pythonRuntimeIdentity": manifest.ResultProcessor.PythonRuntimeIdentity,
		"bootstrapSha256":       manifest.ResultProcessor.BootstrapDigest,
		"sdkSha256":             manifest.ResultProcessor.SDKDigest,
		"moduleIndexSha256":     manifest.ResultProcessor.ModuleIndexDigest,
	} {
		if len(value) != 64 {
			return resultProcessorImageIdentity{}, fmt.Errorf("resultProcessor.%s is invalid", name)
		}
	}
	if len(manifest.ResultProcessor.ModuleRoots) == 0 {
		return resultProcessorImageIdentity{}, errors.New("resultProcessor.moduleRoots is required")
	}
	actual := map[string]string{}
	for name, path := range map[string]string{
		"pythonRuntimeIdentity": "/opt/seven-worker/result-runtime/python-runtime.json",
		"bootstrapSha256":       "/opt/seven-worker/result-runtime/bootstrap.py",
		"sdkSha256":             "/opt/seven-worker/result-runtime/sdk-index.txt",
		"moduleIndexSha256":     "/opt/seven-worker/result-runtime/module-index.json",
	} {
		digest, err := fileSHA256(path)
		if err != nil {
			return resultProcessorImageIdentity{}, err
		}
		actual[name] = digest
	}
	if actual["pythonRuntimeIdentity"] != manifest.ResultProcessor.PythonRuntimeIdentity || actual["bootstrapSha256"] != manifest.ResultProcessor.BootstrapDigest || actual["sdkSha256"] != manifest.ResultProcessor.SDKDigest || actual["moduleIndexSha256"] != manifest.ResultProcessor.ModuleIndexDigest {
		return resultProcessorImageIdentity{}, errors.New("protected Result Processor runtime files do not match manifest")
	}
	return manifest.ResultProcessor, nil
}

func supervise(args []string) error {
	flags := flag.NewFlagSet("supervise", flag.ContinueOnError)
	specPath := flags.String("spec", os.Getenv("SEVEN_ATTEMPT_SPEC"), "AttemptSpec JSON path")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if strings.TrimSpace(*specPath) == "" {
		return errors.New("--spec or SEVEN_ATTEMPT_SPEC is required")
	}
	body, err := os.ReadFile(*specPath)
	if err != nil {
		return fmt.Errorf("read attempt spec: %w", err)
	}
	var spec attemptSpec
	if err := json.Unmarshal(body, &spec); err != nil {
		var typeErr *json.UnmarshalTypeError
		if errors.As(err, &typeErr) && strings.TrimSpace(typeErr.Field) != "" {
			return fmt.Errorf("decode attempt spec field %s: %w", typeErr.Field, err)
		}
		return fmt.Errorf("decode attempt spec: %w", err)
	}
	if err := validateSpec(spec); err != nil {
		return err
	}
	if err := validateResultProcessorImageIdentity(spec.ResultProcessor); err != nil {
		return err
	}
	for _, dir := range []string{spec.Output, spec.State, spec.Runtime, spec.Temporary} {
		if err := os.MkdirAll(dir, 0o700); err != nil {
			return fmt.Errorf("create attempt directory: %w", err)
		}
	}

	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()
	state := &supervisorState{
		State: "READY", TaskID: spec.TaskID, AttemptID: spec.AttemptID,
		Generation: spec.Generation, OwnerEpoch: spec.OwnerEpoch,
		StartedAt: time.Now().UTC().Format(time.RFC3339Nano), done: make(chan error, 1), runningConfirmed: make(chan struct{}), evaluationContinue: make(chan evaluationContinuation, 1), ctx: ctx, cancel: cancel,
		sessionArchivePath: filepath.Join(spec.State, "session.json"),
	}
	if err := state.configureInteractionWait(); err != nil {
		return err
	}
	if err := loadContinuationSession(spec, state); err != nil {
		return err
	}
	if expiresAt, parseErr := time.Parse(time.RFC3339Nano, spec.AuthorityExpiresAt); parseErr == nil {
		state.authorityExpiry = expiresAt
		state.AuthorityExpiresAt = expiresAt.UTC().Format(time.RFC3339Nano)
	}
	state.watchdogGrace = 120 * time.Second
	providerFacade, err := startProviderFacade(ctx, spec.Provider, state.runningConfirmed)
	if err != nil {
		return err
	}
	if providerFacade != nil {
		defer providerFacade.Close()
	}
	// Handshake proves the admitted implementation before advertising READY.
	// initialize is side-effect free: only the later fenced start launches Agent.
	state.adapterProcess, err = openAdmittedAdapter(ctx, spec)
	if err != nil {
		return err
	}
	defer state.adapterProcess.Close()
	socketPath := os.Getenv("SEVEN_CONTROL_SOCKET")
	if socketPath == "" {
		socketPath = filepath.Join(spec.Runtime, "s")
	}
	if clean := filepath.Clean(socketPath); clean != spec.Runtime && !strings.HasPrefix(clean, filepath.Clean(spec.Runtime)+string(os.PathSeparator)) {
		return errors.New("control socket must be inside current Attempt runtime directory")
	}
	listener, err := listenControlWithRunner(socketPath, spec, state, runHarness)
	if err != nil {
		return err
	}
	defer listener.Close()
	go monitorAuthority(ctx, state)
	emit("supervisor.ready", map[string]any{"taskId": strconv.FormatInt(spec.TaskID, 10), "attemptId": strconv.FormatInt(spec.AttemptID, 10), "generation": spec.Generation, "ownerEpoch": spec.OwnerEpoch, "protocol": "seven.supervisor-control/v2"})
	select {
	case <-ctx.Done():
		state.mu.Lock()
		state.State = "CANCELLED"
		state.mu.Unlock()
		emit("supervisor.cancelled", map[string]any{"reason": ctx.Err().Error()})
		return ctx.Err()
	case err := <-state.done:
		return err
	}
}

func harnessMetadata() (harnessType, adapter, executable, version string) {
	body, err := os.ReadFile(manifestPath)
	if err != nil {
		return "", "", "", ""
	}
	var manifest struct {
		Harness struct{ Type, Adapter, Executable, Version string } `json:"harness"`
	}
	if json.Unmarshal(body, &manifest) != nil {
		return "", "", "", ""
	}
	return manifest.Harness.Type, manifest.Harness.Adapter, manifest.Harness.Executable, manifest.Harness.Version
}

func runHarness(ctx context.Context, spec attemptSpec, state *supervisorState, started harnessStarted) (returnErr error) {
	defer func() {
		if state.adapterProcess != nil {
			state.adapterProcess.Close()
		}
	}()
	dispatchWithContext := func(dispatchContext context.Context, event string, payload any) (hookDecision, error) {
		if spec.Hooks == nil {
			return allowDecision(), nil
		}
		body, _ := json.Marshal(map[string]any{"schemaVersion": 1, "event": event, "taskId": strconv.FormatInt(spec.TaskID, 10), "attemptId": strconv.FormatInt(spec.AttemptID, 10), "configurationDigest": spec.Hooks.ConfigurationDigest, "payload": payload})
		return dispatchHooks(dispatchContext, *spec.Hooks, event, body, func(eventType string, payload any) error {
			emit(eventType, payload)
			return nil
		})
	}
	dispatch := func(event string, payload any) (hookDecision, error) {
		return dispatchWithContext(ctx, event, payload)
	}
	defer func() {
		// Cleanup hooks must get one bounded chance even when the Attempt context
		// was cancelled. They remain observational and cannot alter Finalizer state.
		endContext, cancelEnd := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancelEnd()
		result, err := dispatchWithContext(endContext, "attempt.end", map[string]any{"error": errorText(returnErr)})
		if err != nil {
			emit("hook.failed", map[string]any{"event": "attempt.end", "reasonCode": "HOOK_END_FAILED", "error": err.Error()})
		} else {
			emit("hook.end", map[string]any{"decision": result.Decision, "reasonCode": result.ReasonCode})
		}
	}()
	prepare, err := dispatch("attempt.prepare", map[string]any{"instruction": spec.Instruction, "workspace": spec.Workspace})
	if err != nil {
		return fmt.Errorf("dispatch attempt.prepare: %w", err)
	}
	if prepare.Decision == "DENY" {
		return &supervisorFailure{code: "HOOK_PREPARE_DENIED", reasonCode: prepare.ReasonCode, exitCode: 40}
	}
	for round := 1; ; round++ {
		state.mu.Lock()
		state.Round = round
		state.mu.Unlock()
		result, err := runHarnessRound(ctx, spec, state, round, started)
		if err != nil {
			return err
		}
		if sessionArchiveErr(state) != nil {
			return &supervisorFailure{code: "CODEX_SESSION_ARCHIVE_UNAVAILABLE", reasonCode: "CODEX_SESSION_ARCHIVE_UNAVAILABLE", exitCode: 45}
		}
		if _, err := os.Stat(filepath.Join(spec.State, "hook-audit-failure.json")); err == nil {
			return &supervisorFailure{code: "HOOK_AUDIT_UNAVAILABLE", reasonCode: "HOOK_AUDIT_UNAVAILABLE", exitCode: 42}
		} else if !errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("inspect Hook audit state: %w", err)
		}

		var cycleID int64
		state.mu.Lock()
		state.State = "EVALUATION_READY"
		state.EvaluationCycleID = cycleID
		state.EvaluationCycle = round
		state.EvaluationReadyAt = time.Now().UTC().Format(time.RFC3339Nano)
		state.EvaluationDigest = ""
		state.EvaluationCandidate = ""
		state.EvaluationCandidateDigest = ""
		state.EvaluationContinuationDigest = ""
		state.mu.Unlock()
		readyPayload := map[string]any{"cycleId": strconv.FormatInt(cycleID, 10), "cycleNumber": round, "sessionDigest": stateSessionDigest(state), "summary": result.Summary}
		if err := sWriteJournal(state, "evaluation.ready", readyPayload); err != nil {
			return fmt.Errorf("persist evaluation readiness: %w", err)
		}
		emit("evaluation.ready", readyPayload)

		var continuation evaluationContinuation
		select {
		case <-ctx.Done():
			return ctx.Err()
		case continuation = <-state.evaluationContinue:
		}
		if cycleID == 0 {
			state.mu.RLock()
			cycleID = state.EvaluationCycleID
			state.mu.RUnlock()
		}
		if continuation.CycleID != cycleID {
			return &supervisorFailure{code: "EVALUATION_CYCLE_CONFLICT", reasonCode: "EVALUATION_CYCLE_CONFLICT", exitCode: 44}
		}
		switch continuation.Directive {
		case "RESUME_SAME_SESSION":
			if err := recordEvaluationContinuation(state, continuation); err != nil {
				return err
			}
			continue
		case "PROPOSE_COMPLETION":
			if err := recordEvaluationContinuation(state, continuation); err != nil {
				return err
			}
			proposalDecision, err := dispatch("attempt.propose", map[string]any{"summary": result.Summary, "changedFiles": result.ChangedFiles, "evaluationCycleId": strconv.FormatInt(cycleID, 10)})
			if err != nil {
				return fmt.Errorf("dispatch attempt.propose: %w", err)
			}
			if proposalDecision.Decision == "DENY" {
				return &supervisorFailure{code: "HOOK_PROPOSAL_DENIED", reasonCode: proposalDecision.ReasonCode, exitCode: 41}
			}
			proposal := map[string]any{"schemaVersion": 1, "taskId": strconv.FormatInt(spec.TaskID, 10), "attemptId": strconv.FormatInt(spec.AttemptID, 10), "summary": result.Summary, "changedFiles": result.ChangedFiles, "evaluationCycleId": strconv.FormatInt(cycleID, 10), "sessionDigest": stateSessionDigest(state)}
			body, _ := json.MarshalIndent(proposal, "", "  ")
			if err := os.WriteFile(filepath.Join(spec.Output, "completion.json"), body, 0o600); err != nil {
				return fmt.Errorf("write completion proposal: %w", err)
			}
			state.mu.Lock()
			state.State = "COMPLETED"
			state.mu.Unlock()
			emit("completion.proposed", proposal)
			emit("supervisor.completed", map[string]any{"exitCode": 0})
			return nil
		case "WAIT_FOR_USER", "STOP_MANUAL_REVIEW", "STOP_REJECTED":
			if err := recordEvaluationContinuation(state, continuation); err != nil {
				return err
			}
			if err := writeEvaluationTerminal(spec, state, continuation); err != nil {
				return err
			}
			state.mu.Lock()
			if continuation.Directive == "WAIT_FOR_USER" {
				state.State = "WAITING_USER"
			} else if continuation.Directive == "STOP_MANUAL_REVIEW" {
				state.State = "MANUAL_REVIEW"
			} else {
				state.State = "REJECTED"
			}
			state.mu.Unlock()
			emit("evaluation.stopped", map[string]any{"directive": continuation.Directive, "reasonCode": continuation.ReasonCode, "cycleId": strconv.FormatInt(cycleID, 10)})
			return nil
		default:
			return &supervisorFailure{code: "EVALUATION_DIRECTIVE_INVALID", reasonCode: "EVALUATION_DIRECTIVE_INVALID", exitCode: 44}
		}
	}
}

func runHarnessRound(ctx context.Context, spec attemptSpec, state *supervisorState, round int, started harnessStarted) (harnessResult, error) {
	// Image admission must not substitute a Fake execution or call a model.
	if os.Getenv("SEVEN_IMAGE_ADMISSION") == "1" {
		return harnessResult{}, errors.New("ADAPTER_ADMISSION_EXECUTION_FORBIDDEN")
	}
	return runAgentAdapterRound(ctx, spec, state, round, started)
}

func writeEvaluationTerminal(spec attemptSpec, state *supervisorState, continuation evaluationContinuation) error {
	body, err := json.Marshal(map[string]any{"directive": continuation.Directive, "decision": continuation.Decision, "reasonCode": continuation.ReasonCode, "message": continuation.Message, "evaluationCycleId": strconv.FormatInt(continuation.CycleID, 10), "sessionDigest": stateSessionDigest(state)})
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(spec.Output, "evaluation-terminal.json"), body, 0o600)
}

func recordEvaluationContinuation(state *supervisorState, continuation evaluationContinuation) error {
	state.mu.Lock()
	state.State = "RUNNING"
	// The accepted cycle is immutable evidence, but it is no longer the active
	// cycle. Clear the live slot before the next Harness round so a Worker poll
	// can never reuse the previous cycle ID for a newly-ready evaluation.
	state.EvaluationCycleID = 0
	state.managedFeedback = continuation.ManagedFeedback
	state.EvaluationDigest = sha256Text(continuation.Directive + "\x00" + strconv.FormatInt(continuation.CycleID, 10) + "\x00" + continuation.ReasonCode)
	state.mu.Unlock()
	return sWriteJournal(state, "evaluation.acknowledged", map[string]any{"directive": continuation.Directive, "cycleId": strconv.FormatInt(continuation.CycleID, 10), "decision": continuation.Decision, "reasonCode": continuation.ReasonCode})
}

func stateSessionDigest(state *supervisorState) string {
	state.mu.RLock()
	defer state.mu.RUnlock()
	return state.SessionDigest
}

func sessionArchiveErr(state *supervisorState) error {
	state.mu.RLock()
	defer state.mu.RUnlock()
	return state.sessionArchiveErr
}

func sha256Text(value string) string {
	sum := sha256.Sum256([]byte(value))
	return hex.EncodeToString(sum[:])
}

func decimalOptionalID(value int64) string {
	return strconv.FormatInt(value, 10)
}

func validateSpec(spec attemptSpec) error {
	if spec.SchemaVersion != 1 || spec.TaskID == 0 || spec.AttemptID == 0 || spec.Generation < 1 || spec.OwnerEpoch < 1 {
		return errors.New("attempt spec identity or schema is invalid")
	}
	if _, err := agentprotocol.ToolDefinitions(spec.AllowedAdapterTools); err != nil {
		return errors.New("Adapter tool allowlist is invalid")
	}
	taskRoot := filepath.Clean(filepath.Join("/workspace", strconv.FormatInt(spec.TaskID, 10)))
	attemptRoot := filepath.Join(taskRoot, "attempts", strconv.FormatInt(spec.AttemptID, 10))
	for name, value := range map[string]string{"workspace": spec.Workspace, "output": spec.Output, "state": spec.State, "runtime": spec.Runtime, "temporary": spec.Temporary} {
		clean := filepath.Clean(value)
		if clean != attemptRoot && !strings.HasPrefix(clean, attemptRoot+string(os.PathSeparator)) {
			return fmt.Errorf("%s must be inside current Attempt root", name)
		}
	}
	if filepath.Clean(spec.Workspace) != filepath.Join(attemptRoot, "repo") {
		return errors.New("workspace path does not match Worker Image Contract")
	}
	if spec.Provider != nil {
		if strings.TrimSpace(spec.Provider.Model) == "" {
			return errors.New("Provider model is required")
		}
		if spec.Provider.Listen != "127.0.0.1:8787" || strings.TrimRight(spec.Provider.ProxyBaseURL, "/") != "http://seven-provider-proxy:8080" {
			return errors.New("Provider Facade and Proxy endpoints must use fixed Attempt-private addresses")
		}
	}
	if spec.Continuation != nil {
		continuation := spec.Continuation
		if continuation.SourceTurnID == 0 || continuation.TargetTurnID == 0 || continuation.CheckpointID == 0 || len(continuation.SessionLineageDigest) != 64 || len(continuation.ResumeNonce) != 64 || len(continuation.ResumeNonceDigest) != 64 || sha256Text(continuation.ResumeNonce) != continuation.ResumeNonceDigest {
			return errors.New("Continuation resume identity is invalid")
		}
	}
	if spec.ReviewCorrection != nil {
		inputRoot := filepath.Join(taskRoot, "input", "attempts", strconv.FormatInt(spec.AttemptID, 10), "review")
		if err := validateReviewCorrection(*spec.ReviewCorrection, inputRoot, os.ReadFile); err != nil {
			return err
		}
		if spec.ReviewCorrection.ReportDigest != "" {
			info, err := os.Stat(spec.ReviewCorrection.ReportPath)
			if err != nil || info.Mode().Perm()&0o222 != 0 || strings.Contains(spec.Instruction, continuationfacade.ReviewReportPathPlaceholder) || strings.Count(spec.Instruction, spec.ReviewCorrection.ReportPath) != 1 {
				return errors.New("Independent review correction report authority is invalid")
			}
		}
	}
	if spec.ResultProcessor == nil || spec.ResultProcessor.ReleaseID <= 0 || len(strings.TrimSpace(spec.ResultProcessor.Digest)) != 64 || filepath.Clean(spec.ResultProcessor.Root) != filepath.Join("/opt/seven-worker/result-processors", spec.ResultProcessor.Digest) || spec.ResultProcessor.Bootstrap != "/opt/seven-worker/result-runtime/bootstrap.py" || spec.ResultProcessor.Runtime != "/opt/seven-worker/result-runtime" || spec.ResultProcessor.MaxEvaluationCycles < 1 || spec.ResultProcessor.MaxEvaluationCycles > 5 || !json.Valid(spec.ResultProcessor.Parameters) {
		return errors.New("Result Processor execution identity is invalid")
	}
	for name, value := range map[string]string{
		"pythonRuntimeIdentity": spec.ResultProcessor.PythonRuntimeIdentity,
		"bootstrapDigest":       spec.ResultProcessor.BootstrapDigest,
		"sdkDigest":             spec.ResultProcessor.SDKDigest,
		"moduleIndexDigest":     spec.ResultProcessor.ModuleIndexDigest,
	} {
		if len(value) != 64 || !isLowerHex(value) {
			return fmt.Errorf("Result Processor %s is invalid", name)
		}
	}
	return nil
}

func validateReviewCorrection(correction reviewCorrectionSpec, inputRoot string, readFile func(string) ([]byte, error)) error {
	if correction.SourceReviewRunID < 1 || (correction.RevisionKind != "AUTOMATIC" && correction.RevisionKind != "HUMAN_AUTHORIZED") {
		return errors.New("Independent review correction identity is invalid")
	}
	if correction.ReportDigest != "" {
		validVerdict := correction.Verdict == "REVISE" || (correction.RevisionKind == "HUMAN_AUTHORIZED" && correction.Verdict == "REJECT")
		if len(correction.ReportDigest) != 64 || !isLowerHex(correction.ReportDigest) || !validVerdict || filepath.Clean(correction.ReportPath) != filepath.Join(inputRoot, "review-report.json") {
			return errors.New("Independent review correction report identity is invalid")
		}
		body, err := readFile(correction.ReportPath)
		if err != nil || sha256Text(string(body)) != correction.ReportDigest {
			return errors.New("Independent review correction report digest mismatch")
		}
		return nil
	}
	if correction.RevisionKind != "HUMAN_AUTHORIZED" || !safeReviewCorrectionReason(correction.ReasonCode) {
		return errors.New("Independent review correction reason is invalid")
	}
	return nil
}

func safeReviewCorrectionReason(value string) bool {
	switch strings.TrimSpace(value) {
	case "INDEPENDENT_REVIEW_REVISION_LIMIT_EXHAUSTED",
		"INDEPENDENT_REVIEW_NO_MATERIAL_PROGRESS",
		"INDEPENDENT_REVIEW_CHECKPOINT_UNAVAILABLE",
		"INDEPENDENT_REVIEW_REVISION_IDENTITY_CONFLICT",
		"INDEPENDENT_REVIEW_REJECTED",
		"INDEPENDENT_REVIEW_REPORT_INVALID",
		"INDEPENDENT_REVIEW_RETRY_EXHAUSTED":
		return true
	default:
		return false
	}
}

// validateResultProcessorImageIdentity performs the image-local half of the
// execution contract.  validateSpec deliberately stays filesystem-free so
// state-machine tests can validate an untrusted spec without pretending the
// host test process is an admitted Harness image.  The supervisor and fixture
// command always call this before importing any user Processor code.
func validateResultProcessorImageIdentity(spec *resultProcessorSpec) error {
	if spec == nil {
		return errors.New("Result Processor execution identity is missing")
	}
	imageIdentity, err := currentResultProcessorImageIdentity()
	if err != nil {
		return fmt.Errorf("read Result Processor identity from exact Harness image: %w", err)
	}
	if spec.PythonRuntimeIdentity != imageIdentity.PythonRuntimeIdentity ||
		spec.BootstrapDigest != imageIdentity.BootstrapDigest ||
		spec.SDKDigest != imageIdentity.SDKDigest ||
		spec.ModuleIndexDigest != imageIdentity.ModuleIndexDigest {
		return errors.New("Result Processor runtime identity does not match the exact Harness image")
	}
	return nil
}

func loadContinuationSession(spec attemptSpec, state *supervisorState) error {
	if spec.Continuation == nil {
		return nil
	}
	body, err := os.ReadFile(state.sessionArchivePath)
	if err != nil {
		return &supervisorFailure{code: "CONTINUATION_SESSION_UNAVAILABLE", reasonCode: "CONTINUATION_SESSION_UNAVAILABLE", exitCode: 45}
	}
	var archive struct {
		SchemaVersion int    `json:"schemaVersion"`
		Kind          string `json:"kind"`
		SessionID     string `json:"sessionId"`
		SessionDigest string `json:"sessionDigest"`
	}
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.DisallowUnknownFields()
	if err = decoder.Decode(&archive); err != nil {
		return &supervisorFailure{code: "CONTINUATION_SESSION_INVALID", reasonCode: "CONTINUATION_SESSION_UNAVAILABLE", exitCode: 45}
	}
	var trailing any
	if err = decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		return &supervisorFailure{code: "CONTINUATION_SESSION_INVALID", reasonCode: "CONTINUATION_SESSION_UNAVAILABLE", exitCode: 45}
	}
	if archive.SchemaVersion != 1 || archive.Kind != "agent-session" || strings.TrimSpace(archive.SessionID) == "" || len(archive.SessionID) > 512 || archive.SessionDigest != sha256Text(archive.SessionID) || archive.SessionDigest != spec.Continuation.SessionLineageDigest {
		return &supervisorFailure{code: "CONTINUATION_SESSION_IDENTITY_MISMATCH", reasonCode: "CONTINUATION_SESSION_UNAVAILABLE", exitCode: 45}
	}
	state.mu.Lock()
	state.sessionID = archive.SessionID
	state.SessionDigest = archive.SessionDigest
	state.managedFeedback = spec.Instruction
	state.mu.Unlock()
	return nil
}

func errorText(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}

func listenControl(path string, spec attemptSpec, state *supervisorState) (net.Listener, error) {
	return listenControlWithRunner(path, spec, state, runHarness)
}

func listenControlWithChmod(path string, spec attemptSpec, state *supervisorState, chmod func(string, os.FileMode) error) (net.Listener, error) {
	return listenControlWithRunnerAndChmod(path, spec, state, runHarness, chmod)
}

func listenControlWithRunner(path string, spec attemptSpec, state *supervisorState, run harnessRunner) (net.Listener, error) {
	return listenControlWithRunnerAndChmod(path, spec, state, run, os.Chmod)
}

func listenControlWithRunnerAndChmod(path string, spec attemptSpec, state *supervisorState, run harnessRunner, chmod func(string, os.FileMode) error) (net.Listener, error) {
	state.mu.Lock()
	if state.runningConfirmed == nil {
		state.runningConfirmed = make(chan struct{})
	}
	state.mu.Unlock()
	_ = os.Remove(path)
	server := &controlServer{spec: spec, state: state, terminalBy: map[string]string{}, run: run}
	journalName := "runtime-journal.jsonl"
	if spec.Hooks != nil {
		// Keep the P0.6 evidence path stable while extending the same append-only
		// journal with owner and watchdog facts.
		journalName = "hook-events.jsonl"
	}
	journalRoot := spec.State
	if journalRoot == "" {
		// Low-level conformance tests and diagnostics may construct a minimal
		// Attempt spec without a state path. Keep their journal beside the
		// private control socket instead of leaking it into the source tree.
		journalRoot = spec.Runtime
	}
	journalPath := filepath.Join(journalRoot, journalName)
	var err error
	server.journal, err = os.OpenFile(journalPath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		return nil, fmt.Errorf("open Runtime journal: %w", err)
	}
	state.mu.Lock()
	state.journalWrite = server.writeJournal
	state.mu.Unlock()
	if spec.Hooks != nil {
		body, err := os.ReadFile(spec.Hooks.Spec)
		if err != nil {
			return nil, fmt.Errorf("read Hook configuration for control validation: %w", err)
		}
		if err := json.Unmarshal(body, &server.hooks); err != nil || server.hooks.SchemaVersion != 1 || server.hooks.ConfigurationDigest != spec.Hooks.ConfigurationDigest {
			return nil, errors.New("Hook configuration identity is invalid for control validation")
		}
		loadedHooks := make([]map[string]any, 0, len(server.hooks.Hooks))
		for _, hook := range server.hooks.Hooks {
			loadedHooks = append(loadedHooks, map[string]any{
				"releaseId": strconv.FormatInt(hook.ReleaseID, 10),
				"name":      hook.Name,
				"version":   hook.Version,
			})
		}
		loaded := map[string]any{
			"attemptId":           strconv.FormatInt(spec.AttemptID, 10),
			"projectVersionId":    strconv.FormatInt(spec.ProjectVersionID, 10),
			"configurationDigest": spec.Hooks.ConfigurationDigest,
			"hooks":               loadedHooks,
			"loadedAt":            time.Now().UTC().Format(time.RFC3339Nano),
		}
		if err := server.writeJournal("hook.configuration.loaded", loaded); err != nil {
			return nil, err
		}
		if err := supervisorEvents.Emit("hook.configuration.loaded", loaded); err != nil {
			return nil, err
		}
	}
	listener, err := net.Listen("unix", path)
	if err != nil {
		if server.journal != nil {
			_ = server.journal.Close()
		}
		return nil, fmt.Errorf("listen control socket: %w", err)
	}
	managedListener := &controlListener{Listener: listener, journal: server.journal}
	if err := chmod(path, 0o600); err != nil {
		// Docker Desktop's VirtioFS bind mounts can reject chmod on a Unix
		// socket with EINVAL even though the socket is already confined to the
		// Attempt-private runtime directory. Keep all other failures fatal, but
		// do not return before the accept loop is started for the EINVAL case.
		if !errors.Is(err, syscall.EINVAL) {
			listener.Close()
			if server.journal != nil {
				_ = server.journal.Close()
			}
			return nil, err
		}
	}
	go func() {
		for {
			conn, err := managedListener.Accept()
			if err != nil {
				return
			}
			go server.handle(conn)
		}
	}()
	return managedListener, nil
}

// sWriteJournal lets the long-running Harness loop persist control facts
// without reaching back into the UDS server.  The callback is installed before
// the socket accepts any request, and it retains the single-writer journal
// ordering provided by controlServer.writeJournal.
func sWriteJournal(state *supervisorState, eventType string, payload any) error {
	state.mu.RLock()
	write := state.journalWrite
	state.mu.RUnlock()
	if write == nil {
		return errors.New("Runtime journal is unavailable")
	}
	return write(eventType, payload)
}

func (s *controlServer) handle(conn net.Conn) {
	defer conn.Close()
	_ = conn.SetDeadline(time.Now().Add(2 * time.Second))
	reader := bufio.NewReaderSize(conn, controlStreamRequestLimit+1)
	line, readErr := readControlFrame(reader, controlStreamRequestLimit)
	if readErr == nil && s.handleControlStream(conn, reader, line) {
		return
	}
	if readErr != nil && !errors.Is(readErr, io.EOF) {
		_ = json.NewEncoder(conn).Encode(map[string]any{"ok": false, "error": "invalid request"})
		return
	}
	s.handleControlRequest(conn, line)
}

// Both transports dispatch through the same authority and lifecycle methods.
func (s *controlServer) handleControlRequest(conn net.Conn, line []byte) {
	if len(line) == 0 || len(line) > controlRequestLimit {
		_ = json.NewEncoder(conn).Encode(map[string]any{"ok": false, "error": "request exceeds limit"})
		return
	}
	if agentprotocol.ValidateDocument(line) != nil {
		_ = json.NewEncoder(conn).Encode(map[string]any{"ok": false, "reasonCode": "SUPERVISOR_REQUEST_INVALID"})
		return
	}
	decoder := json.NewDecoder(strings.NewReader(string(line)))
	decoder.DisallowUnknownFields()
	var request attemptControlRequest
	if err := decoder.Decode(&request); err != nil {
		_ = json.NewEncoder(conn).Encode(map[string]any{"ok": false, "error": "invalid request"})
		return
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		_ = json.NewEncoder(conn).Encode(map[string]any{"ok": false, "error": "request must contain exactly one JSON object"})
		return
	}
	if s.handleInteractionControl(conn, request) {
		return
	}
	if request.Command == "hook-event" {
		if err := s.acceptHookEvent(request); err != nil {
			_ = json.NewEncoder(conn).Encode(map[string]any{"ok": false, "error": err.Error()})
			return
		}
		_ = json.NewEncoder(conn).Encode(map[string]any{"ok": true, "command": request.Command, "invocationId": request.InvocationID})
		return
	}
	if request.Command == "start-harness" {
		_ = conn.SetDeadline(time.Now().Add(35 * time.Second))
		response, reason, err := s.startHarness(request)
		if err != nil {
			_ = json.NewEncoder(conn).Encode(map[string]any{"ok": false, "reasonCode": reason, "error": err.Error()})
			return
		}
		_ = json.NewEncoder(conn).Encode(map[string]any{"ok": true, "command": request.Command, "harnessStarted": response})
		return
	}
	if request.Command == "confirm-running" {
		reason, err := s.confirmRunning(request)
		if err != nil {
			_ = json.NewEncoder(conn).Encode(map[string]any{"ok": false, "reasonCode": reason, "error": err.Error()})
			return
		}
		_ = json.NewEncoder(conn).Encode(map[string]any{"ok": true, "command": request.Command})
		return
	}
	if request.Command == "adopt-owner" {
		ack, reason, err := s.adoptOwner(request)
		if err != nil {
			_ = json.NewEncoder(conn).Encode(map[string]any{"ok": false, "reasonCode": reason, "error": err.Error()})
			return
		}
		_ = json.NewEncoder(conn).Encode(map[string]any{"ok": true, "command": request.Command, "reattachAck": ack})
		return
	}
	if request.Command == "renew-authority" {
		reason, err := s.renewAuthority(request)
		if err != nil {
			_ = json.NewEncoder(conn).Encode(map[string]any{"ok": false, "reasonCode": reason, "error": err.Error()})
			return
		}
		_ = json.NewEncoder(conn).Encode(map[string]any{"ok": true, "command": request.Command})
		return
	}
	if request.Command == "run-evaluation" {
		response, reason, err := s.runEvaluation(request)
		if err != nil {
			_ = json.NewEncoder(conn).Encode(map[string]any{"ok": false, "reasonCode": reason, "error": err.Error()})
			return
		}
		_ = json.NewEncoder(conn).Encode(map[string]any{"ok": true, "command": request.Command, "candidate": response})
		return
	}
	if request.Command == "continue-evaluation" {
		reason, err := s.continueEvaluation(request)
		if err != nil {
			_ = json.NewEncoder(conn).Encode(map[string]any{"ok": false, "reasonCode": reason, "error": err.Error()})
			return
		}
		_ = json.NewEncoder(conn).Encode(map[string]any{"ok": true, "command": request.Command})
		return
	}
	if request.Command != "status" && request.Command != "interrupt" && request.Command != "diagnostics" && request.Command != "state" && request.Command != "evaluation-state" {
		_ = json.NewEncoder(conn).Encode(map[string]any{"ok": false, "error": "unknown command"})
		return
	}
	if request.Command == "interrupt" {
		s.state.mu.RLock()
		valid := request.TaskID > 0 && request.AttemptID > 0 && request.Generation > 0 && request.OwnerEpoch > 0 && request.TaskID == s.state.TaskID && request.AttemptID == s.state.AttemptID && request.Generation == s.state.Generation && request.OwnerEpoch == s.state.OwnerEpoch
		cancel := s.state.cancel
		if valid && cancel != nil {
			cancel()
		}
		s.state.mu.RUnlock()
		if !valid {
			_ = json.NewEncoder(conn).Encode(map[string]any{"ok": false, "reasonCode": "SUPERVISOR_IDENTITY_MISMATCH", "error": "interrupt authority does not match current owner"})
			return
		}
	}
	s.state.mu.RLock()
	snapshot := protocolStateSnapshot(s.state)
	s.state.mu.RUnlock()
	_ = json.NewEncoder(conn).Encode(map[string]any{"ok": true, "command": request.Command, "state": snapshot})
}

// protocolStateSnapshot keeps every int64 identity and concurrency token
// lossless at the JSON boundary. The map is used instead of marshaling the
// live state so synchronization-only fields never escape the Supervisor.
func protocolStateSnapshot(state *supervisorState) map[string]any {
	snapshot := map[string]any{
		"state": state.State, "taskId": strconv.FormatInt(state.TaskID, 10), "attemptId": strconv.FormatInt(state.AttemptID, 10),
		"generation": strconv.FormatInt(state.Generation, 10), "ownerEpoch": strconv.FormatInt(state.OwnerEpoch, 10), "startedAt": state.StartedAt,
		"harnessStartedAt": state.HarnessStartedAt, "harnessPid": state.HarnessPID,
		"runPermitId": decimalOptionalID(state.RunPermitID), "runPermitDigest": state.RunPermitDigest,
		"runNonceDigest":     state.RunNonceDigest,
		"authorityExpiresAt": state.AuthorityExpiresAt, "orphanDeadline": state.OrphanDeadline,
		"lastJournalSequence": strconv.FormatInt(state.LastJournalSequence, 10),
		"evaluationCycleId":   decimalOptionalID(state.EvaluationCycleID), "evaluationCycle": state.EvaluationCycle,
		"evaluationReadyAt": state.EvaluationReadyAt, "evaluationDigest": state.EvaluationDigest,
		"evaluationCandidate": state.EvaluationCandidate, "evaluationCandidateDigest": state.EvaluationCandidateDigest,
		"sessionDigest": state.SessionDigest, "round": state.Round,
		"observations":        append([]agentObservation(nil), state.observations...),
		"observationSequence": strconv.FormatInt(state.observationSequence, 10), "observationGap": state.observationGap,
	}
	state.addInteractionSnapshot(snapshot)
	return snapshot
}

type evaluationMaterialization struct {
	CycleID   int64             `json:"cycleId,string"`
	Variables map[string]string `json:"variables"`
	ExpiresAt string            `json:"expiresAt"`
}

type evaluationCandidateSnapshot struct {
	CycleID        int64  `json:"cycleId,string"`
	CycleNumber    int    `json:"cycleNumber"`
	CandidateFile  string `json:"candidateFile"`
	ContractSHA256 string `json:"contractSha256"`
	SessionDigest  string `json:"sessionDigest,omitempty"`
}

// runEvaluation executes the exact read-only Processor release after a
// Harness round has ended.  The Result Contract stays in the private runtime
// directory rather than travelling through the bounded UDS response.  The
// Worker verifies and forwards those bytes to Server for the durable ACK.
func (s *controlServer) runEvaluation(request attemptControlRequest) (evaluationCandidateSnapshot, string, error) {
	if request.SchemaVersion != 2 || request.TaskID != s.spec.TaskID || request.AttemptID != s.spec.AttemptID || request.Generation != s.spec.Generation {
		return evaluationCandidateSnapshot{}, "SUPERVISOR_IDENTITY_MISMATCH", errors.New("evaluation identity does not match Attempt")
	}
	if s.spec.ResultProcessor == nil {
		return evaluationCandidateSnapshot{}, "RESULT_PROCESSOR_IDENTITY_INVALID", errors.New("Result Processor execution identity is unavailable")
	}
	if request.OwnerEpoch != s.spec.OwnerEpoch || request.CycleID <= 0 || request.CycleNumber < 1 || request.CycleNumber > s.spec.ResultProcessor.MaxEvaluationCycles {
		return evaluationCandidateSnapshot{}, "EVALUATION_IDENTITY_INVALID", errors.New("evaluation identity is invalid")
	}
	if len(request.Materialization) == 0 || len(request.Materialization) > 12<<10 {
		return evaluationCandidateSnapshot{}, "EVALUATION_MATERIALIZATION_INVALID", errors.New("evaluation materialization is invalid")
	}
	var materialization evaluationMaterialization
	decoder := json.NewDecoder(bytes.NewReader(request.Materialization))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&materialization); err != nil {
		return evaluationCandidateSnapshot{}, "EVALUATION_MATERIALIZATION_INVALID", errors.New("evaluation materialization is invalid")
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) || materialization.CycleID != request.CycleID || !validEvaluationVariables(materialization.Variables) {
		return evaluationCandidateSnapshot{}, "EVALUATION_MATERIALIZATION_INVALID", errors.New("evaluation materialization is invalid")
	}
	if materialization.ExpiresAt != "" {
		expiresAt, err := time.Parse(time.RFC3339Nano, materialization.ExpiresAt)
		if err != nil || !expiresAt.After(time.Now().UTC()) {
			return evaluationCandidateSnapshot{}, "EVALUATION_MATERIALIZATION_EXPIRED", errors.New("evaluation materialization is expired")
		}
	}

	s.startMu.Lock()
	defer s.startMu.Unlock()
	s.state.mu.Lock()
	if s.state.State == "EVALUATION_PENDING_ACK" && s.state.EvaluationCycleID == request.CycleID && s.state.EvaluationCycle == request.CycleNumber && s.state.EvaluationCandidate != "" {
		response := evaluationCandidateSnapshot{CycleID: request.CycleID, CycleNumber: request.CycleNumber, CandidateFile: filepath.Base(s.state.EvaluationCandidate), ContractSHA256: s.state.EvaluationCandidateDigest, SessionDigest: s.state.SessionDigest}
		s.state.mu.Unlock()
		return response, "", nil
	}
	if s.state.State != "EVALUATION_READY" || (s.state.EvaluationCycleID != 0 && s.state.EvaluationCycleID != request.CycleID) || s.state.EvaluationCycle != request.CycleNumber {
		state := s.state.State
		s.state.mu.Unlock()
		return evaluationCandidateSnapshot{}, "EVALUATION_NOT_READY", fmt.Errorf("Supervisor state %s cannot run evaluation", state)
	}
	s.state.EvaluationCycleID = request.CycleID
	s.state.mu.Unlock()

	candidatePath, contractDigest, err := s.executeEvaluation(request, materialization)
	if err != nil {
		// Processor failures are not Agent infrastructure failures.  Produce a
		// bounded, normal Result Contract that the Server will persist as
		// MANUAL_REVIEW, preserving the no-proposal/no-finalization boundary.
		candidatePath, contractDigest, err = s.writeEvaluationFailureCandidate(request, materialization, err)
		if err != nil {
			return evaluationCandidateSnapshot{}, "RESULT_PROCESSOR_EXECUTION_FAILED", err
		}
	}
	s.state.mu.Lock()
	s.state.State = "EVALUATION_PENDING_ACK"
	s.state.EvaluationCandidate = candidatePath
	s.state.EvaluationCandidateDigest = contractDigest
	s.state.EvaluationDigest = contractDigest
	s.state.mu.Unlock()
	payload := map[string]any{"cycleId": strconv.FormatInt(request.CycleID, 10), "cycleNumber": request.CycleNumber, "candidateFile": filepath.Base(candidatePath), "contractDigest": contractDigest, "sessionDigest": stateSessionDigest(s.state)}
	if err := s.writeJournal("evaluation.candidate", payload); err != nil {
		return evaluationCandidateSnapshot{}, "RUNTIME_JOURNAL_UNAVAILABLE", err
	}
	emit("evaluation.candidate", payload)
	return evaluationCandidateSnapshot{CycleID: request.CycleID, CycleNumber: request.CycleNumber, CandidateFile: filepath.Base(candidatePath), ContractSHA256: contractDigest, SessionDigest: stateSessionDigest(s.state)}, "", nil
}

func (s *controlServer) executeEvaluation(request attemptControlRequest, materialization evaluationMaterialization) (string, string, error) {
	if s.spec.ResultProcessor == nil {
		return "", "", errors.New("Result Processor execution identity is unavailable")
	}
	root := filepath.Clean(s.spec.ResultProcessor.Root)
	bootstrap := filepath.Clean(s.spec.ResultProcessor.Bootstrap)
	if root != filepath.Join("/opt/seven-worker/result-processors", s.spec.ResultProcessor.Digest) || bootstrap != "/opt/seven-worker/result-runtime/bootstrap.py" {
		return "", "", errors.New("Result Processor execution identity changed")
	}
	if err := os.MkdirAll(filepath.Join(s.spec.Runtime, "evaluations"), 0o700); err != nil {
		return "", "", err
	}
	cycleName := strconv.FormatInt(request.CycleID, 10)
	contextPath := filepath.Join(s.spec.Runtime, "evaluations", cycleName+".context.json")
	materializationPath := filepath.Join(s.spec.Runtime, "evaluations", cycleName+".materialization.json")
	candidatePath := filepath.Join(s.spec.Runtime, "evaluations", cycleName+".result.json")
	if !insideRuntimeDirectory(s.spec.Runtime, contextPath) || !insideRuntimeDirectory(s.spec.Runtime, materializationPath) || !insideRuntimeDirectory(s.spec.Runtime, candidatePath) {
		return "", "", errors.New("evaluation private path is invalid")
	}
	parameters := json.RawMessage("{}")
	if len(s.spec.ResultProcessor.Parameters) != 0 {
		parameters = s.spec.ResultProcessor.Parameters
	}
	var parameterObject map[string]any
	if err := json.Unmarshal(parameters, &parameterObject); err != nil {
		return "", "", errors.New("Result Processor parameters are invalid")
	}
	contextBody, err := json.Marshal(map[string]any{
		"taskId": strconv.FormatInt(s.spec.TaskID, 10), "attemptId": strconv.FormatInt(s.spec.AttemptID, 10), "cycleId": strconv.FormatInt(request.CycleID, 10),
		"cycleNumber": request.CycleNumber, "workspaceRepo": s.spec.Workspace, "parameters": parameterObject,
	})
	if err != nil {
		return "", "", err
	}
	materializationBody, err := json.Marshal(materialization)
	if err != nil {
		return "", "", err
	}
	if err := atomicPrivateWrite(contextPath, contextBody); err != nil {
		return "", "", err
	}
	if err := atomicPrivateWrite(materializationPath, materializationBody); err != nil {
		return "", "", err
	}
	defer os.Remove(contextPath)
	defer os.Remove(materializationPath)

	baseContext := s.state.ctx
	if baseContext == nil {
		baseContext = context.Background()
	}
	evaluationCtx, cancel := context.WithTimeout(baseContext, 30*time.Second)
	defer cancel()
	command := exec.CommandContext(evaluationCtx, "python3", bootstrap, "--processor-root", root, "--context", contextPath, "--result", candidatePath, "--materialization", materializationPath)
	command.Dir = s.spec.Runtime
	command.Env = []string{"PATH=/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin", "LANG=C.UTF-8", "LC_ALL=C.UTF-8", "PYTHONNOUSERSITE=1"}
	command.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	output, commandErr := boundedCommandOutput(evaluationCtx, command, 64<<10)
	if commandErr != nil {
		if len(output) != 0 {
			emit("result.processor.stderr", map[string]any{"message": redactedProcessorDiagnostic(output, materialization.Variables, 4096)})
		}
		return "", "", fmt.Errorf("run protected Result Processor bootstrap: %w", commandErr)
	}
	if len(output) != 0 {
		emit("result.processor.diagnostics", map[string]any{"message": redactedProcessorDiagnostic(output, materialization.Variables, 4096)})
	}
	body, err := os.ReadFile(candidatePath)
	if err != nil || len(body) == 0 || len(body) > 1<<20 || !json.Valid(body) {
		return "", "", errors.New("Result Processor candidate contract is unavailable")
	}
	sum := sha256.Sum256(body)
	return candidatePath, hex.EncodeToString(sum[:]), nil
}

func (s *controlServer) writeEvaluationFailureCandidate(request attemptControlRequest, materialization evaluationMaterialization, cause error) (string, string, error) {
	path := filepath.Join(s.spec.Runtime, "evaluations", strconv.FormatInt(request.CycleID, 10)+".result.json")
	body, err := json.Marshal(map[string]any{
		"decision": "MANUAL_REVIEW", "reasonCode": "RESULT_PROCESSOR_EXECUTION_FAILED",
		"message": "结果处理脚本未能完成，任务已转入人工处理。",
	})
	if err != nil {
		return "", "", err
	}
	if err := atomicPrivateWrite(path, body); err != nil {
		return "", "", err
	}
	emit("result.processor.failed", map[string]any{"reasonCode": "RESULT_PROCESSOR_EXECUTION_FAILED", "diagnostic": redactedProcessorDiagnostic([]byte(cause.Error()), materialization.Variables, 4096)})
	sum := sha256.Sum256(body)
	return path, hex.EncodeToString(sum[:]), nil
}

func (s *controlServer) continueEvaluation(request attemptControlRequest) (string, error) {
	if request.SchemaVersion != 2 || request.TaskID != s.spec.TaskID || request.AttemptID != s.spec.AttemptID || request.Generation != s.spec.Generation || request.OwnerEpoch != s.spec.OwnerEpoch {
		return "SUPERVISOR_IDENTITY_MISMATCH", errors.New("evaluation continuation identity does not match Attempt")
	}
	continuation := evaluationContinuation{Directive: request.Directive, CycleID: request.CycleID, Decision: request.Decision, ReasonCode: request.ReasonCode, Message: request.Message, ManagedFeedback: request.ManagedFeedback, ResumeNonce: request.ResumeNonce}
	if continuation.CycleID <= 0 || len(continuation.Message) > 8192 || len(continuation.ManagedFeedback) > 8192 || len(continuation.ReasonCode) > 128 {
		return "EVALUATION_CONTINUATION_INVALID", errors.New("evaluation continuation is invalid")
	}
	if continuation.Directive != "RESUME_SAME_SESSION" && continuation.Directive != "PROPOSE_COMPLETION" && continuation.Directive != "WAIT_FOR_USER" && continuation.Directive != "STOP_REJECTED" && continuation.Directive != "STOP_MANUAL_REVIEW" {
		return "EVALUATION_DIRECTIVE_INVALID", errors.New("evaluation directive is invalid")
	}
	if continuation.Directive == "RESUME_SAME_SESSION" && len(continuation.ResumeNonce) < 32 {
		return "SESSION_RESUME_NONCE_INVALID", errors.New("session resume nonce is invalid")
	}
	payload, _ := json.Marshal(continuation)
	digest := sha256.Sum256(payload)
	digestText := hex.EncodeToString(digest[:])
	s.state.mu.Lock()
	if s.state.EvaluationCycleID != continuation.CycleID || s.state.State != "EVALUATION_PENDING_ACK" {
		state := s.state.State
		s.state.mu.Unlock()
		return "EVALUATION_NOT_PENDING_ACK", fmt.Errorf("Supervisor state %s cannot continue evaluation", state)
	}
	if s.state.EvaluationContinuationDigest != "" {
		if s.state.EvaluationContinuationDigest == digestText {
			s.state.mu.Unlock()
			return "", nil
		}
		s.state.mu.Unlock()
		return "EVALUATION_CONTINUATION_CONFLICT", errors.New("evaluation continuation was already accepted")
	}
	s.state.EvaluationContinuationDigest = digestText
	s.state.State = "EVALUATION_ACKNOWLEDGED"
	channel := s.state.evaluationContinue
	s.state.mu.Unlock()
	if channel == nil {
		return "EVALUATION_CHANNEL_UNAVAILABLE", errors.New("evaluation continuation channel is unavailable")
	}
	select {
	case channel <- continuation:
		return "", nil
	default:
		return "EVALUATION_CONTINUATION_CONFLICT", errors.New("evaluation continuation is already pending")
	}
}

func validEvaluationVariables(values map[string]string) bool {
	if values == nil || len(values) > 128 {
		return false
	}
	for key, value := range values {
		if key == "" || len(key) > 128 || len(value) > 64<<10 {
			return false
		}
		for index, character := range key {
			if !(character == '_' || character >= 'A' && character <= 'Z' || character >= 'a' && character <= 'z' || index > 0 && character >= '0' && character <= '9') {
				return false
			}
		}
	}
	return true
}

func insideRuntimeDirectory(root, path string) bool {
	cleanRoot := filepath.Clean(root)
	cleanPath := filepath.Clean(path)
	return cleanPath != cleanRoot && strings.HasPrefix(cleanPath, cleanRoot+string(os.PathSeparator))
}

func atomicPrivateWrite(path string, body []byte) error {
	if len(body) == 0 || len(body) > 1<<20 {
		return errors.New("private control body is invalid")
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	file, err := os.CreateTemp(filepath.Dir(path), ".seven-private-")
	if err != nil {
		return err
	}
	name := file.Name()
	defer os.Remove(name)
	if _, err = file.Write(body); err == nil {
		err = file.Sync()
	}
	if closeErr := file.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		return err
	}
	if err = os.Chmod(name, 0o600); err != nil {
		return err
	}
	return os.Rename(name, path)
}

func boundedCommandOutput(ctx context.Context, command *exec.Cmd, limit int) ([]byte, error) {
	buffer := &limitedBuffer{limit: limit}
	command.Stdout = buffer
	command.Stderr = buffer
	if err := command.Start(); err != nil {
		return buffer.Bytes(), err
	}
	done := make(chan error, 1)
	go func() { done <- command.Wait() }()
	select {
	case err := <-done:
		if buffer.overflow {
			return buffer.Bytes(), errors.New("Result Processor diagnostics exceeded limit")
		}
		return buffer.Bytes(), err
	case <-ctx.Done():
		if command.Process != nil {
			_ = syscall.Kill(-command.Process.Pid, syscall.SIGTERM)
		}
		select {
		case <-done:
		case <-time.After(5 * time.Second):
			if command.Process != nil {
				_ = syscall.Kill(-command.Process.Pid, syscall.SIGKILL)
			}
			<-done
		}
		return buffer.Bytes(), ctx.Err()
	}
}

type limitedBuffer struct {
	limit    int
	value    bytes.Buffer
	overflow bool
}

func (b *limitedBuffer) Write(value []byte) (int, error) {
	if b.limit <= 0 {
		b.limit = 1
	}
	if remaining := b.limit - b.value.Len(); remaining > 0 {
		if len(value) > remaining {
			_, _ = b.value.Write(value[:remaining])
			b.overflow = true
		} else {
			_, _ = b.value.Write(value)
		}
	} else if len(value) != 0 {
		b.overflow = true
	}
	return len(value), nil
}

func (b *limitedBuffer) Bytes() []byte { return append([]byte(nil), b.value.Bytes()...) }

func boundedDiagnostic(value []byte, limit int) string {
	if len(value) > limit {
		value = value[:limit]
	}
	return strings.TrimSpace(string(value))
}

// redactedProcessorDiagnostic is used only for diagnostics emitted after a
// cycle-scoped materialization.  The Result Contract itself is checked again
// by the Worker before it is persisted.  Sorting longer values first prevents
// a shorter secret fragment from leaving a longer materialized value visible.
func redactedProcessorDiagnostic(value []byte, variables map[string]string, limit int) string {
	values := make([]string, 0, len(variables))
	seen := make(map[string]struct{}, len(variables))
	for _, candidate := range variables {
		if candidate == "" || len(candidate) > 64<<10 {
			continue
		}
		if _, exists := seen[candidate]; exists {
			continue
		}
		seen[candidate] = struct{}{}
		values = append(values, candidate)
	}
	sort.Slice(values, func(i, j int) bool { return len(values[i]) > len(values[j]) })
	for _, secret := range values {
		value = bytes.ReplaceAll(value, []byte(secret), []byte("[REDACTED]"))
	}
	return boundedDiagnostic(value, limit)
}

func (s *controlServer) adoptOwner(request attemptControlRequest) (*dispatchdomain.ReattachAck, string, error) {
	s.startMu.Lock()
	defer s.startMu.Unlock()
	if request.SchemaVersion != 2 || request.TaskID != s.spec.TaskID || request.AttemptID != s.spec.AttemptID || request.Generation != s.spec.Generation {
		return nil, "SUPERVISOR_IDENTITY_MISMATCH", errors.New("adopt-owner identity does not match Attempt")
	}
	if request.ReattachPermitID == 0 || len(request.ReattachPermitToken) < 32 || request.PreviousOwnerEpoch < 1 || request.OwnerEpoch != request.PreviousOwnerEpoch+1 {
		return nil, "REATTACH_PERMIT_INVALID", errors.New("reattach permit identity is invalid")
	}
	expiresAt, err := time.Parse(time.RFC3339Nano, request.ExpiresAt)
	if err != nil || !expiresAt.After(time.Now().UTC()) {
		return nil, "REATTACH_PERMIT_EXPIRED", errors.New("reattach permit is expired")
	}
	leaseExpiresAt, err := time.Parse(time.RFC3339Nano, request.LeaseExpiresAt)
	if err != nil || !leaseExpiresAt.After(time.Now().UTC()) || leaseExpiresAt.After(time.Now().UTC().Add(45*time.Second)) || len(request.LeaseReceiptDigest) != 64 {
		return nil, "EXECUTION_LEASE_INVALID", errors.New("new execution authority is invalid")
	}
	permitSum := sha256.Sum256([]byte(request.ReattachPermitToken))
	permitDigest := hex.EncodeToString(permitSum[:])
	s.state.mu.Lock()
	if s.state.ReattachPermitID != 0 {
		if s.state.ReattachPermitID == request.ReattachPermitID && s.state.ReattachPermitDigest == permitDigest && s.state.OwnerEpoch == request.OwnerEpoch {
			ack := reattachAckSnapshot(s.state, request.PreviousOwnerEpoch)
			s.state.mu.Unlock()
			return ack, "", nil
		}
		s.state.mu.Unlock()
		return nil, "REATTACH_PAYLOAD_CONFLICT", errors.New("reattach permit was already used with different authority")
	}
	if s.state.OwnerEpoch != request.PreviousOwnerEpoch {
		current := s.state.OwnerEpoch
		s.state.mu.Unlock()
		return nil, "OWNER_EPOCH_CONFLICT", fmt.Errorf("owner epoch is %d", current)
	}
	s.state.OwnerEpoch = request.OwnerEpoch
	s.state.ReattachPermitID = request.ReattachPermitID
	s.state.ReattachPermitDigest = permitDigest
	s.state.authorityExpiry = leaseExpiresAt
	s.state.AuthorityExpiresAt = leaseExpiresAt.UTC().Format(time.RFC3339Nano)
	s.state.OrphanDeadline = ""
	s.spec.OwnerEpoch = request.OwnerEpoch
	ack := reattachAckSnapshot(s.state, request.PreviousOwnerEpoch)
	s.state.mu.Unlock()
	if err := s.writeJournal("owner.adopted", ack); err != nil {
		return nil, "RUNTIME_JOURNAL_UNAVAILABLE", err
	}
	return ack, "", nil
}

func (s *controlServer) renewAuthority(request attemptControlRequest) (string, error) {
	if request.SchemaVersion != 2 || request.TaskID != s.spec.TaskID || request.AttemptID != s.spec.AttemptID || request.Generation != s.spec.Generation || len(request.LeaseReceiptDigest) != 64 {
		return "SUPERVISOR_IDENTITY_MISMATCH", errors.New("authority identity is invalid")
	}
	expiresAt, err := time.Parse(time.RFC3339Nano, request.LeaseExpiresAt)
	if err != nil || !expiresAt.After(time.Now().UTC()) || expiresAt.After(time.Now().UTC().Add(45*time.Second)) {
		return "EXECUTION_LEASE_INVALID", errors.New("execution lease expiry is invalid")
	}
	s.state.mu.Lock()
	defer s.state.mu.Unlock()
	if request.OwnerEpoch != s.state.OwnerEpoch {
		return "OWNER_EPOCH_CONFLICT", errors.New("execution lease belongs to another owner")
	}
	if !s.state.authorityExpiry.IsZero() && !expiresAt.After(s.state.authorityExpiry) {
		if expiresAt.Equal(s.state.authorityExpiry) {
			return "", nil
		}
		return "EXECUTION_LEASE_REGRESSION", errors.New("execution lease expiry must be monotonic")
	}
	s.state.authorityExpiry = expiresAt
	s.state.AuthorityExpiresAt = expiresAt.UTC().Format(time.RFC3339Nano)
	s.state.OrphanDeadline = ""
	return "", nil
}

func reattachAckSnapshot(state *supervisorState, previousOwner int64) *dispatchdomain.ReattachAck {
	return &dispatchdomain.ReattachAck{PermitID: state.ReattachPermitID, PermitDigest: state.ReattachPermitDigest, OldOwnerEpoch: previousOwner, NewOwnerEpoch: state.OwnerEpoch, SupervisorState: state.State, HarnessPID: state.HarnessPID, RunNonceDigest: state.RunNonceDigest, LastJournalSequence: state.LastJournalSequence}
}

func monitorAuthority(ctx context.Context, state *supervisorState) {
	ticker := time.NewTicker(250 * time.Millisecond)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case now := <-ticker.C:
			state.mu.Lock()
			if state.authorityExpiry.IsZero() || now.Before(state.authorityExpiry) {
				state.mu.Unlock()
				continue
			}
			grace := state.watchdogGrace
			if grace <= 0 {
				grace = 120 * time.Second
			}
			deadline := state.authorityExpiry.Add(grace)
			state.OrphanDeadline = deadline.UTC().Format(time.RFC3339Nano)
			cancel := state.cancel
			expired := !now.Before(deadline)
			state.mu.Unlock()
			if expired && cancel != nil {
				_ = supervisorEvents.Emit("watchdog.expired", map[string]any{"deadline": deadline.UTC().Format(time.RFC3339Nano)})
				cancel()
				return
			}
		}
	}
}

func (s *controlServer) startHarness(request attemptControlRequest) (map[string]any, string, error) {
	s.startMu.Lock()
	defer s.startMu.Unlock()
	if request.SchemaVersion != 2 || request.TaskID != s.spec.TaskID || request.AttemptID != s.spec.AttemptID || request.Generation != s.spec.Generation || request.OwnerEpoch != s.spec.OwnerEpoch {
		return nil, "SUPERVISOR_IDENTITY_MISMATCH", errors.New("start-harness identity does not match Attempt spec")
	}
	if request.RunPermitID == 0 || len(request.RunPermitToken) < 32 || len(request.RunNonce) < 32 {
		return nil, "RUN_PERMIT_INVALID", errors.New("run permit identity is invalid")
	}
	expiresAt, err := time.Parse(time.RFC3339Nano, request.ExpiresAt)
	if err != nil || !expiresAt.After(time.Now().UTC()) {
		return nil, "RUN_PERMIT_EXPIRED", errors.New("run permit is expired")
	}
	permitSum := sha256.Sum256([]byte(request.RunPermitToken))
	nonceSum := sha256.Sum256([]byte(request.RunNonce))
	permitDigest := hex.EncodeToString(permitSum[:])
	nonceDigest := hex.EncodeToString(nonceSum[:])

	s.state.mu.Lock()
	if s.state.RunNonceDigest != "" {
		if s.state.RunNonceDigest != nonceDigest || s.state.RunPermitID != request.RunPermitID || s.state.RunPermitDigest != permitDigest {
			s.state.mu.Unlock()
			return nil, "RUN_NONCE_CONFLICT", errors.New("Harness has already been started with a different run identity")
		}
		response := harnessStartedSnapshot(s.state)
		s.state.mu.Unlock()
		return response, "", nil
	}
	if s.state.State != "READY" {
		state := s.state.State
		s.state.mu.Unlock()
		return nil, "SUPERVISOR_NOT_READY", fmt.Errorf("Supervisor state %s cannot start Harness", state)
	}
	s.state.State = "STARTING"
	s.state.RunPermitID = request.RunPermitID
	s.state.RunPermitDigest = permitDigest
	s.state.RunNonceDigest = nonceDigest
	run := s.run
	done := s.state.done
	ctx := s.state.ctx
	if ctx == nil {
		ctx = context.Background()
	}
	s.state.mu.Unlock()

	type startResult struct {
		response map[string]any
		err      error
	}
	startedResult := make(chan startResult, 1)
	go func() {
		marked := false
		err := run(ctx, s.spec, s.state, func(pid int) error {
			if pid < 1 {
				return errors.New("Harness PID is invalid")
			}
			s.state.mu.Lock()
			if s.state.State != "STARTING" {
				current := s.state.State
				s.state.mu.Unlock()
				return fmt.Errorf("Supervisor state %s cannot accept Harness start", current)
			}
			s.state.HarnessStartedAt = time.Now().UTC().Format(time.RFC3339Nano)
			s.state.HarnessPID = pid
			response := harnessStartedSnapshot(s.state)
			s.state.mu.Unlock()
			marked = true
			emit("harness.started", response)
			_ = s.writeJournal("harness.started", response)
			startedResult <- startResult{response: response}
			select {
			case <-s.state.runningConfirmed:
				return nil
			case <-ctx.Done():
				return ctx.Err()
			}
		})
		if !marked {
			if err == nil {
				err = errors.New("Harness runner exited before reporting start")
			}
			startedResult <- startResult{err: err}
		}
		if done != nil {
			done <- err
		}
	}()
	result := <-startedResult
	if result.err != nil {
		s.state.mu.Lock()
		if s.state.State == "STARTING" {
			s.state.State = "FAILED"
		}
		s.state.mu.Unlock()
		return nil, "HARNESS_START_FAILED", result.err
	}
	return result.response, "", nil
}

func (s *controlServer) confirmRunning(request attemptControlRequest) (string, error) {
	if request.SchemaVersion != 2 || request.TaskID != s.spec.TaskID || request.AttemptID != s.spec.AttemptID || request.Generation != s.spec.Generation {
		return "SUPERVISOR_IDENTITY_MISMATCH", errors.New("confirm-running identity does not match Attempt spec")
	}
	nonceSum := sha256.Sum256([]byte(request.RunNonce))
	nonceDigest := hex.EncodeToString(nonceSum[:])
	s.state.mu.Lock()
	if request.OwnerEpoch != s.state.OwnerEpoch {
		s.state.mu.Unlock()
		return "OWNER_EPOCH_CONFLICT", errors.New("confirm-running belongs to another owner")
	}
	if request.RunPermitID == 0 || request.RunPermitID != s.state.RunPermitID || len(request.RunNonce) < 32 || nonceDigest != s.state.RunNonceDigest || s.state.HarnessStartedAt == "" || s.state.HarnessPID < 1 {
		s.state.mu.Unlock()
		return "RUN_CONFIRMATION_IDENTITY_MISMATCH", errors.New("confirm-running does not match the started Harness")
	}
	if s.state.State != "STARTING" && s.state.State != "RUNNING" {
		current := s.state.State
		s.state.mu.Unlock()
		return "SUPERVISOR_NOT_STARTING", fmt.Errorf("Supervisor state %s cannot confirm RUNNING", current)
	}
	s.state.State = "RUNNING"
	payload := harnessStartedSnapshot(s.state)
	s.state.mu.Unlock()
	if err := s.writeJournal("runtime.running_confirmed", payload); err != nil {
		return "RUNTIME_JOURNAL_UNAVAILABLE", err
	}
	s.state.runningConfirmOnce.Do(func() { close(s.state.runningConfirmed) })
	emit("runtime.running_confirmed", payload)
	return "", nil
}

func harnessStartedSnapshot(state *supervisorState) map[string]any {
	return map[string]any{
		"attemptId": strconv.FormatInt(state.AttemptID, 10), "generation": state.Generation, "ownerEpoch": state.OwnerEpoch,
		"runPermitId": decimalOptionalID(state.RunPermitID), "runPermitDigest": state.RunPermitDigest,
		"runNonceDigest": state.RunNonceDigest, "harnessPid": state.HarnessPID,
		"startedAt": state.HarnessStartedAt,
	}
}

func (s *controlServer) acceptHookEvent(request attemptControlRequest) error {
	if err := validateHookControlEvent(s.spec, s.hooks, request); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if terminal := s.terminalBy[request.InvocationID]; terminal != "" {
		if terminal == request.EventType {
			return nil
		}
		return errors.New("Hook invocation already has a different terminal event")
	}
	if request.EventType != "hook.started" {
		s.terminalBy[request.InvocationID] = request.EventType
	}
	payload := map[string]any{
		"event": request.HookEvent, "invocationId": request.InvocationID,
		"configurationDigest": request.ConfigurationDigest, "hookName": request.HookName, "packageReleaseId": strconv.FormatInt(request.PackageReleaseID, 10),
		"packageVersion": request.PackageVersion, "packageDigest": request.PackageDigest,
		"reasonCode": request.ReasonCode, "durationMs": request.DurationMS,
	}
	if request.Diagnostic != "" {
		payload["diagnostic"] = request.Diagnostic
	}
	if err := s.writeJournal(request.EventType, payload); err != nil {
		return err
	}
	return supervisorEvents.Emit(request.EventType, payload)
}

func (s *controlServer) writeJournal(eventType string, payload any) error {
	s.journalMu.Lock()
	defer s.journalMu.Unlock()
	if s.journal == nil {
		return errors.New("Runtime journal is unavailable")
	}
	payloadBody, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	s.journalSequence++
	digest := sha256.Sum256(payloadBody)
	envelope := supervisorwire.JournalEntry{SchemaVersion: 1, Sequence: s.journalSequence, FactID: fmt.Sprintf("jrn_%d_%d", s.spec.AttemptID, s.journalSequence), Type: eventType, Time: time.Now().UTC().Format(time.RFC3339Nano), PayloadDigest: hex.EncodeToString(digest[:]), Payload: json.RawMessage(payloadBody)}
	if err := json.NewEncoder(s.journal).Encode(envelope); err != nil {
		return fmt.Errorf("persist Runtime journal: %w", err)
	}
	if err := s.journal.Sync(); err != nil {
		return fmt.Errorf("sync Runtime journal: %w", err)
	}
	s.state.mu.Lock()
	s.state.LastJournalSequence = s.journalSequence
	s.state.mu.Unlock()
	return nil
}

func validateHookControlEvent(spec attemptSpec, configuration hookConfigurationSpec, request attemptControlRequest) error {
	if request.SchemaVersion != 1 || request.Command != "hook-event" {
		return errors.New("invalid Attempt control schema or command")
	}
	if spec.Hooks == nil || request.TaskID != spec.TaskID || request.AttemptID != spec.AttemptID || request.Generation != spec.Generation || request.ConfigurationDigest != spec.Hooks.ConfigurationDigest {
		return errors.New("Hook event Attempt or Hook configuration identity mismatch")
	}
	if request.EventType != "hook.started" && request.EventType != "hook.allow" && request.EventType != "hook.deny" && request.EventType != "hook.failed" {
		return errors.New("unknown Hook event type")
	}
	if !knownHookEvent(request.HookEvent) || !strings.HasPrefix(request.InvocationID, "hinv_") || len(request.InvocationID) > 128 || len(request.ReasonCode) > 128 || len(request.Diagnostic) > 4096 || request.DurationMS < 0 || request.DurationMS > 30_000 {
		return errors.New("invalid Hook event fields")
	}
	for _, pkg := range configuration.Hooks {
		if pkg.ReleaseID == request.PackageReleaseID && pkg.Version == request.PackageVersion && pkg.Digest == request.PackageDigest {
			return nil
		}
	}
	return errors.New("Hook Package identity mismatch")
}

func control(command string) error {
	request, _ := json.Marshal(map[string]string{"command": command})
	return controlBytes(append(request, '\n'))
}

// controlJSON is the container-local bridge used by the Worker through
// `docker exec -i`. The Supervisor UDS remains Attempt-private; this avoids
// relying on Docker Desktop bind mounts to propagate Unix sockets to the host.
func controlJSON() error {
	body, err := io.ReadAll(io.LimitReader(os.Stdin, controlRequestLimit+1))
	if err != nil {
		return err
	}
	if len(body) == 0 || len(body) > controlRequestLimit || !json.Valid(body) {
		return errors.New("control request must be one valid JSON object within the control limit")
	}
	return controlBytes(append(body, '\n'))
}

func controlBytes(request []byte) error {
	socket := os.Getenv("SEVEN_CONTROL_SOCKET")
	if socket == "" {
		return errors.New("SEVEN_CONTROL_SOCKET is required")
	}
	conn, err := net.DialTimeout("unix", socket, 2*time.Second)
	if err != nil {
		return err
	}
	defer conn.Close()
	if _, err := conn.Write(request); err != nil {
		return err
	}
	response, err := bufio.NewReader(io.LimitReader(conn, 1<<20)).ReadBytes('\n')
	if err != nil && !errors.Is(err, io.EOF) {
		return err
	}
	_, err = os.Stdout.Write(response)
	return err
}

func fileSHA256(path string) (string, error) {
	file, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer file.Close()
	hash := sha256.New()
	if _, err := io.Copy(hash, file); err != nil {
		return "", err
	}
	return hex.EncodeToString(hash.Sum(nil)), nil
}

func emit(eventType string, payload any) {
	_ = supervisorEvents.Emit(eventType, payload)
}

func usage() {
	fmt.Fprintln(os.Stderr, "seven-worker-cli <image probe --json|supervise --spec FILE|check --input FILE --repository DIR|hook dispatch --event EVENT|control [--stream]|interaction ask --request FILE|status|interrupt|diagnostics|version>")
}
