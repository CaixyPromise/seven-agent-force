package application

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"sort"
	"strings"
	"time"

	"github.com/CaixyPromise/seven-agent/force/internal/app/workforce/workerregistry/domain"
)

var (
	ErrUnauthorized             = errors.New("worker credential is invalid")
	ErrNotFound                 = errors.New("worker not found")
	ErrStaleInstance            = errors.New("stale worker instance")
	ErrConflict                 = errors.New("worker state conflict")
	ErrNotDrained               = errors.New("worker must be drained before disable")
	ErrProviderRuntimeActive    = errors.New("worker has an active Provider-backed Runtime")
	ErrInventorySchedulingDrift = errors.New("worker image scheduling does not match its manifest")
	ErrInvalidCapacity          = errors.New("worker capacity is invalid")
	ErrCapacityExceedsWorker    = errors.New("configured capacity exceeds the Worker reported limit")
	ErrMetricRangeInvalid       = errors.New("worker metric range is invalid")
)

type Repository interface {
	Create(context.Context, domain.Worker, int64, string, int64) error
	RotateToken(context.Context, int64, int64, string, string) error
	Authenticate(context.Context, int64) (string, string, error)
	Register(context.Context, RegisterCommand) error
	Heartbeat(context.Context, HeartbeatCommand) error
	RecordMetricSample(context.Context, MetricSample) error
	QueryMetricSeries(context.Context, *int64, time.Time, time.Time, int) ([]domain.MetricPoint, error)
	DeleteMetricSamplesBefore(context.Context, time.Time, int) (int64, error)
	PublishInventory(context.Context, InventoryCommand) error
	Deregister(context.Context, int64, string, string) error
	SetDesiredState(context.Context, int64, string, int64) error
	UpdateCapacity(context.Context, int64, int, int64) error
	List(context.Context, int) ([]domain.Worker, error)
	Get(context.Context, int64) (domain.Worker, []domain.Image, error)
	ListAssignments(context.Context, int64, int) ([]domain.Assignment, error)
	RequestInventoryRefresh(context.Context, int64) (int64, error)
	Decommission(context.Context, int64, int64) error
	RecoverableRuntimes(context.Context, int64, string) ([]domain.RecoverableRuntime, error)
}

type IDGenerator interface{ NextID() int64 }

type Topology interface {
	EnsureWorkerQueue(context.Context, domain.QueueContract) error
	DeleteWorkerQueue(context.Context, domain.QueueContract) (int, error)
}

type Service struct {
	repo          Repository
	topology      Topology
	idGen         IDGenerator
	metricFailure func(error)
}

func (s *Service) SetMetricFailureHandler(handler func(error)) { s.metricFailure = handler }

func New(repo Repository, topology Topology, idGen ...IDGenerator) *Service {
	service := &Service{repo: repo, topology: topology}
	if len(idGen) > 0 {
		service.idGen = idGen[0]
	}
	return service
}

type Credential struct {
	Token     string `json:"token"`
	TokenHint string `json:"tokenHint"`
}

func (s *Service) Provision(ctx context.Context, name, description string, actor int64, max int) (domain.Worker, Credential, error) {
	if s.idGen == nil {
		return domain.Worker{}, Credential{}, errors.New("workforce ID generator is not configured")
	}
	name = strings.TrimSpace(name)
	if name == "" || len(name) > 120 {
		return domain.Worker{}, Credential{}, errors.New("worker name is required and must be at most 120 characters")
	}
	if len(description) > 1000 {
		return domain.Worker{}, Credential{}, errors.New("worker description exceeds 1000 characters")
	}
	if max < 1 || max > 64 {
		return domain.Worker{}, Credential{}, errors.New("maxActiveAttempts must be between 1 and 64")
	}
	token, hash, hint, err := newToken()
	if err != nil {
		return domain.Worker{}, Credential{}, err
	}
	w := domain.Worker{ID: s.idGen.NextID(), Name: name, Description: strings.TrimSpace(description), DesiredState: domain.DesiredEnabled, State: domain.StateProvisioned, MaxActiveAttempts: max, TokenHint: hint, Version: 1}
	if actor <= 0 {
		return domain.Worker{}, Credential{}, errors.New("authenticated actor is required")
	}
	if err := s.repo.Create(ctx, w, s.idGen.NextID(), hash, actor); err != nil {
		return domain.Worker{}, Credential{}, err
	}
	return w, Credential{Token: token, TokenHint: hint}, nil
}

func (s *Service) RotateToken(ctx context.Context, workerID int64) (Credential, error) {
	if s.idGen == nil {
		return Credential{}, errors.New("workforce ID generator is not configured")
	}
	token, hash, hint, err := newToken()
	if err != nil {
		return Credential{}, err
	}
	if err := s.repo.RotateToken(ctx, workerID, s.idGen.NextID(), hash, hint); err != nil {
		return Credential{}, err
	}
	return Credential{Token: token, TokenHint: hint}, nil
}

func (s *Service) Authenticate(ctx context.Context, workerID int64, token string) error {
	if len(token) < 32 || len(token) > 256 {
		return ErrUnauthorized
	}
	digest := sha256.Sum256([]byte(token))
	actual := hex.EncodeToString(digest[:])
	stored, desired, err := s.repo.Authenticate(ctx, workerID)
	if err != nil {
		return ErrUnauthorized
	}
	if len(stored) != len(actual) || subtle.ConstantTimeCompare([]byte(stored), []byte(actual)) != 1 {
		return ErrUnauthorized
	}
	if desired == domain.DesiredDisabled {
		return ErrUnauthorized
	}
	return nil
}

type RegisterCommand struct {
	WorkerID           int64
	InstanceID         string
	HostBootID         string
	StartedAt          string
	Version            string
	Revision           string
	BuildDate          string
	ObservedRemoteIP   string
	MaxActiveAttempts  int
	MaxActiveReviews   int
	MaxTotalContainers int
	Capabilities       json.RawMessage
}

type RegisterResult struct {
	RegistrationID           int64                 `json:"registrationId,string"`
	WorkerID                 int64                 `json:"workerId,string"`
	InstanceID               string                `json:"instanceId"`
	DesiredState             string                `json:"desiredState"`
	HeartbeatIntervalSeconds int                   `json:"heartbeatIntervalSeconds"`
	StaleAfterSeconds        int                   `json:"staleAfterSeconds"`
	AssignmentLeaseSeconds   int                   `json:"assignmentLeaseSeconds"`
	ReconcileRequired        bool                  `json:"reconcileRequired"`
	WorkerState              string                `json:"workerState"`
	Reconciliation           domain.Reconciliation `json:"reconciliation"`
	InventoryVersion         int                   `json:"inventoryVersion"`
	Queue                    domain.QueueContract  `json:"queue"`
	ServerTime               string                `json:"serverTime"`
}

func (s *Service) Register(ctx context.Context, c RegisterCommand) (RegisterResult, error) {
	if c.WorkerID <= 0 || strings.TrimSpace(c.InstanceID) == "" || strings.TrimSpace(c.HostBootID) == "" {
		return RegisterResult{}, errors.New("workerId, instanceId and hostBootId are required")
	}
	if c.MaxActiveAttempts < 1 || c.MaxActiveAttempts > 64 {
		return RegisterResult{}, errors.New("maxActiveAttempts is invalid")
	}
	// Pre-Slice-4 Workers do not publish review capacity. Preserve their
	// registration protocol while withholding review assignment through the
	// independentReviewConsumer capability predicate in ReviewRun scheduling.
	if c.MaxActiveReviews == 0 {
		c.MaxActiveReviews = 1
	}
	if c.MaxTotalContainers == 0 {
		c.MaxTotalContainers = c.MaxActiveAttempts + c.MaxActiveReviews
	}
	if c.MaxActiveReviews < 1 || c.MaxActiveReviews > 64 || c.MaxTotalContainers < c.MaxActiveAttempts+c.MaxActiveReviews || c.MaxTotalContainers > 128 {
		return RegisterResult{}, errors.New("review capacity is invalid")
	}
	if len(c.Capabilities) == 0 || len(c.Capabilities) > 64<<10 || !json.Valid(c.Capabilities) {
		return RegisterResult{}, errors.New("capabilities are invalid")
	}
	worker, _, err := s.repo.Get(ctx, c.WorkerID)
	if err != nil {
		return RegisterResult{}, err
	}
	effectiveMax := min(c.MaxActiveAttempts, worker.MaxActiveAttempts)
	queue := domain.QueueFor(c.WorkerID, effectiveMax)
	if s.topology != nil {
		if err := s.topology.EnsureWorkerQueue(ctx, queue); err != nil {
			return RegisterResult{}, fmt.Errorf("ensure worker queue: %w", err)
		}
	}
	if err := s.repo.Register(ctx, c); err != nil {
		return RegisterResult{}, err
	}
	worker, _, err = s.repo.Get(ctx, c.WorkerID)
	if err != nil {
		return RegisterResult{}, err
	}
	runtimes, err := s.repo.RecoverableRuntimes(ctx, c.WorkerID, c.InstanceID)
	if err != nil {
		return RegisterResult{}, err
	}
	reconciliation := domain.Reconciliation{Required: len(runtimes) > 0, Runtimes: runtimes}
	workerState := worker.State
	if reconciliation.Required {
		workerState = domain.StateReconciling
		reconciliation.Deadline = runtimes[0].ReattachDeadline
	}
	if s.idGen == nil {
		return RegisterResult{}, errors.New("workforce ID generator is not configured")
	}
	return RegisterResult{RegistrationID: s.idGen.NextID(), WorkerID: c.WorkerID, InstanceID: c.InstanceID, DesiredState: worker.DesiredState, HeartbeatIntervalSeconds: 10, StaleAfterSeconds: 35, AssignmentLeaseSeconds: 30, ReconcileRequired: reconciliation.Required, WorkerState: workerState, Reconciliation: reconciliation, InventoryVersion: int(worker.InventoryVersion), Queue: queue, ServerTime: time.Now().UTC().Format(time.RFC3339Nano)}, nil
}

type HeartbeatCommand struct {
	WorkerID                          int64
	InstanceID                        string
	HostBootID                        string
	ConsumerState                     string
	Hostname                          string
	ReportedIP                        string
	PrimaryInterface                  string
	PrimaryMAC                        string
	Sequence                          int64
	DockerReady                       bool
	ActiveAttempts, MaxActiveAttempts int
	Interfaces, ResourceSnapshot      json.RawMessage
}

type HeartbeatResult struct {
	DesiredState               string `json:"desiredState"`
	DerivedState               string `json:"derivedState"`
	ConsumerDirective          string `json:"consumerDirective"`
	RefreshInventoryGeneration int    `json:"refreshInventoryGeneration"`
}

type MetricSample struct {
	WorkerID                                           int64
	LogicalCPUCores, ActiveAttempts, MaxActiveAttempts int
	CPUUsagePercent, Load1                             *float64
	MemoryUsedBytes, MemoryTotalBytes                  int64
	MemoryUsagePercent                                 *float64
	WorkspaceDiskUsedBytes, WorkspaceDiskTotalBytes    int64
	WorkspaceDiskUsagePercent                          *float64
	DockerReady                                        bool
}

type metricResourceSnapshot struct {
	LogicalCPUCores           int     `json:"logicalCpuCores"`
	CPUUsagePercent           float64 `json:"cpuUsagePercent"`
	CPUUsageAvailable         bool    `json:"cpuUsageAvailable"`
	Load1                     float64 `json:"load1"`
	MemoryUsedBytes           int64   `json:"memoryUsedBytes"`
	MemoryTotalBytes          int64   `json:"memoryTotalBytes"`
	MemoryUsagePercent        float64 `json:"memoryUsagePercent"`
	WorkspaceDiskUsedBytes    int64   `json:"workspaceDiskUsedBytes"`
	WorkspaceDiskTotalBytes   int64   `json:"workspaceDiskTotalBytes"`
	WorkspaceDiskUsagePercent float64 `json:"workspaceDiskUsagePercent"`
}

func (s *Service) Heartbeat(ctx context.Context, c HeartbeatCommand) (HeartbeatResult, error) {
	if c.Sequence < 1 || c.ActiveAttempts < 0 || c.MaxActiveAttempts < 1 || c.ActiveAttempts > c.MaxActiveAttempts {
		return HeartbeatResult{}, errors.New("heartbeat counters are invalid")
	}
	if len(c.Interfaces) > 64<<10 || len(c.ResourceSnapshot) > 64<<10 || !json.Valid(c.Interfaces) || !json.Valid(c.ResourceSnapshot) {
		return HeartbeatResult{}, errors.New("heartbeat payload is invalid")
	}
	if err := s.repo.Heartbeat(ctx, c); err != nil {
		return HeartbeatResult{}, err
	}
	if sample, err := metricSampleFromHeartbeat(c); err != nil {
		s.reportMetricFailure(err)
	} else if err = s.repo.RecordMetricSample(ctx, sample); err != nil {
		s.reportMetricFailure(err)
	}
	w, _, err := s.repo.Get(ctx, c.WorkerID)
	if err != nil {
		return HeartbeatResult{}, err
	}
	directive := "CONSUME"
	if w.State == domain.StateReconciling {
		directive = "STOP"
	} else if w.DesiredState == domain.DesiredDraining {
		directive = "DRAIN"
	} else if w.DesiredState == domain.DesiredDisabled || w.State == domain.StateDecommissioned {
		directive = "STOP"
	}
	return HeartbeatResult{DesiredState: w.DesiredState, DerivedState: w.State, ConsumerDirective: directive, RefreshInventoryGeneration: int(w.InventoryRefreshGeneration)}, nil
}

func (s *Service) reportMetricFailure(err error) {
	if s.metricFailure != nil {
		s.metricFailure(err)
	}
}

func metricSampleFromHeartbeat(c HeartbeatCommand) (MetricSample, error) {
	var value metricResourceSnapshot
	if err := json.Unmarshal(c.ResourceSnapshot, &value); err != nil {
		return MetricSample{}, err
	}
	if value.LogicalCPUCores < 1 || value.LogicalCPUCores > math.MaxInt16 || value.MemoryUsedBytes < 0 || value.MemoryTotalBytes < value.MemoryUsedBytes || value.WorkspaceDiskUsedBytes < 0 || value.WorkspaceDiskTotalBytes < value.WorkspaceDiskUsedBytes {
		return MetricSample{}, errors.New("worker metric snapshot is invalid")
	}
	percent := func(value float64) (*float64, error) {
		if math.IsNaN(value) || math.IsInf(value, 0) || value < 0 || value > 100 {
			return nil, errors.New("worker metric percentage is invalid")
		}
		copy := value
		return &copy, nil
	}
	memory, err := percent(value.MemoryUsagePercent)
	if err != nil {
		return MetricSample{}, err
	}
	disk, err := percent(value.WorkspaceDiskUsagePercent)
	if err != nil {
		return MetricSample{}, err
	}
	var cpu *float64
	if value.CPUUsageAvailable {
		cpu, err = percent(value.CPUUsagePercent)
		if err != nil {
			return MetricSample{}, err
		}
	}
	var load1 *float64
	if !math.IsNaN(value.Load1) && !math.IsInf(value.Load1, 0) && value.Load1 >= 0 {
		copy := value.Load1
		load1 = &copy
	}
	return MetricSample{WorkerID: c.WorkerID, LogicalCPUCores: value.LogicalCPUCores, CPUUsagePercent: cpu, Load1: load1, MemoryUsedBytes: value.MemoryUsedBytes, MemoryTotalBytes: value.MemoryTotalBytes, MemoryUsagePercent: memory, WorkspaceDiskUsedBytes: value.WorkspaceDiskUsedBytes, WorkspaceDiskTotalBytes: value.WorkspaceDiskTotalBytes, WorkspaceDiskUsagePercent: disk, ActiveAttempts: c.ActiveAttempts, MaxActiveAttempts: c.MaxActiveAttempts, DockerReady: c.DockerReady}, nil
}

type metricRange struct {
	duration time.Duration
	step     int
}

var metricRanges = map[string]metricRange{"1h": {time.Hour, 60}, "6h": {6 * time.Hour, 120}, "24h": {24 * time.Hour, 300}, "7d": {7 * 24 * time.Hour, 1800}, "30d": {30 * 24 * time.Hour, 7200}}

func (s *Service) Metrics(ctx context.Context, workerID *int64, rangeName string) (domain.MetricSeries, error) {
	config, ok := metricRanges[strings.TrimSpace(rangeName)]
	if !ok {
		return domain.MetricSeries{}, ErrMetricRangeInvalid
	}
	step := time.Duration(config.step) * time.Second
	to := time.Now().UTC().Truncate(step).Add(step)
	from := to.Add(-config.duration)
	points, err := s.repo.QueryMetricSeries(ctx, workerID, from, to, config.step)
	if err != nil {
		return domain.MetricSeries{}, err
	}
	return domain.MetricSeries{From: from, To: to, StepSeconds: config.step, Points: points}, nil
}

func (s *Service) DeleteExpiredMetrics(ctx context.Context, now time.Time) (int64, error) {
	var total int64
	for {
		count, err := s.repo.DeleteMetricSamplesBefore(ctx, now.UTC().Add(-30*24*time.Hour), 10000)
		if err != nil {
			return total, err
		}
		total += count
		if count < 10000 {
			return total, nil
		}
	}
}

func (s *Service) Reconciliation(ctx context.Context, workerID int64, instanceID string, hostBootID string) (domain.Reconciliation, error) {
	worker, _, err := s.repo.Get(ctx, workerID)
	if err != nil {
		return domain.Reconciliation{}, err
	}
	if worker.CurrentInstanceID != instanceID || worker.HostBootID != hostBootID {
		return domain.Reconciliation{}, ErrStaleInstance
	}
	runtimes, err := s.repo.RecoverableRuntimes(ctx, workerID, instanceID)
	if err != nil {
		return domain.Reconciliation{}, err
	}
	result := domain.Reconciliation{Required: len(runtimes) > 0, Runtimes: runtimes}
	if len(runtimes) > 0 {
		result.Deadline = runtimes[0].ReattachDeadline
	}
	return result, nil
}

type InventoryCommand struct {
	WorkerID         int64
	InstanceID       string
	HostBootID       string
	InventoryDigest  string
	InventoryVersion int64
	Images           []domain.Image
}

func (s *Service) PublishInventory(ctx context.Context, c InventoryCommand) error {
	if c.InventoryVersion < 1 || len(c.InventoryDigest) != 64 {
		return errors.New("inventory identity is invalid")
	}
	if len(c.Images) > 512 {
		return errors.New("inventory exceeds 512 images")
	}
	for index := range c.Images {
		image := &c.Images[index]
		if image.ContractVersion != "seven.worker-image/v1" || (image.Role != "attempt" && image.Role != "provider-proxy") || len(image.ImageDigest) < 64 {
			return errors.New("inventory contains an unsupported image")
		}
		// Rejected discovery records deliberately carry no executable manifest.
		// Preserve them as inventory/drift evidence, but assign an unroutable
		// scheduling identity instead of requiring a manifest that admission
		// explicitly refused to trust.
		if image.Role != "attempt" || image.AdmissionStatus != "ADMITTED" {
			setUnroutableScheduling(image)
			continue
		}
		scheduling, err := ParseManifestScheduling(image.Manifest)
		if err != nil {
			return fmt.Errorf("inventory image scheduling is invalid: %w", err)
		}
		image.SchedulingScope = scheduling.Scope
		image.SchedulingPriority = scheduling.Priority
		image.EnvironmentCapabilities = scheduling.EnvironmentCapabilitiesJSON
		image.ExecutionKinds = scheduling.ExecutionKindsJSON
		image.SchedulingDigest = scheduling.Digest
	}
	return s.repo.PublishInventory(ctx, c)
}

func setUnroutableScheduling(image *domain.Image) {
	image.SchedulingScope = "TEST_ONLY"
	image.SchedulingPriority = 0
	image.EnvironmentCapabilities = json.RawMessage(`[]`)
	executionKinds := `[]`
	if image.Role == "attempt" {
		// The worker-images schema requires an attempt image to retain at least
		// one execution kind even when rejected. AdmissionStatus remains the
		// routing fence; this synthetic kind only makes the evidence row valid.
		executionKinds = `["AGENT_ATTEMPT"]`
	}
	image.ExecutionKinds = json.RawMessage(executionKinds)
	sum := sha256.Sum256([]byte(`{"scope":"TEST_ONLY","priority":0,"environmentCapabilities":[],"executionKinds":` + executionKinds + `}`))
	image.SchedulingDigest = hex.EncodeToString(sum[:])
}

type manifestScheduling struct {
	Scope                   string   `json:"scope"`
	Priority                int      `json:"priority"`
	EnvironmentCapabilities []string `json:"environmentCapabilities"`
	ProviderProtocols       []string `json:"providerProtocols"`
	HookEvents              []string `json:"hookEvents"`
	ExecutionKinds          []string `json:"-"`
}

type ResolvedManifestScheduling struct {
	Scope                       string
	Priority                    int
	EnvironmentCapabilitiesJSON json.RawMessage
	ExecutionKindsJSON          json.RawMessage
	Digest                      string
}

// ParseManifestScheduling derives the scheduling identity from the admitted
// image manifest. Inventory callers cannot override these values beside the
// manifest that admission already measured.
func ParseManifestScheduling(manifest json.RawMessage) (ResolvedManifestScheduling, error) {
	if len(manifest) == 0 || len(manifest) > 1<<20 || !json.Valid(manifest) {
		return ResolvedManifestScheduling{}, errors.New("manifest is not valid JSON")
	}
	var envelope struct {
		Scheduling     json.RawMessage `json:"scheduling"`
		ExecutionKinds json.RawMessage `json:"executionKinds"`
	}
	if err := json.Unmarshal(manifest, &envelope); err != nil || len(envelope.Scheduling) == 0 || string(envelope.Scheduling) == "null" || len(envelope.ExecutionKinds) == 0 || string(envelope.ExecutionKinds) == "null" {
		return ResolvedManifestScheduling{}, errors.New("manifest.scheduling is required")
	}
	var scheduling manifestScheduling
	decoder := json.NewDecoder(strings.NewReader(string(envelope.Scheduling)))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&scheduling); err != nil {
		return ResolvedManifestScheduling{}, errors.New("manifest.scheduling has an invalid shape")
	}
	var extra any
	if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
		return ResolvedManifestScheduling{}, errors.New("manifest.scheduling has trailing data")
	}
	var schedulingFields map[string]json.RawMessage
	if err := json.Unmarshal(envelope.Scheduling, &schedulingFields); err != nil {
		return ResolvedManifestScheduling{}, errors.New("manifest.scheduling is not an object")
	}
	for _, field := range []string{"scope", "priority", "environmentCapabilities", "providerProtocols", "hookEvents"} {
		value, exists := schedulingFields[field]
		if !exists || string(value) == "null" {
			return ResolvedManifestScheduling{}, fmt.Errorf("%s is required", field)
		}
	}
	if err := json.Unmarshal(envelope.ExecutionKinds, &scheduling.ExecutionKinds); err != nil {
		return ResolvedManifestScheduling{}, errors.New("manifest.executionKinds has an invalid shape")
	}
	if scheduling.Scope != "PRODUCTION" && scheduling.Scope != "TEST_ONLY" {
		return ResolvedManifestScheduling{}, errors.New("scope must be PRODUCTION or TEST_ONLY")
	}
	if scheduling.Priority < -100000 || scheduling.Priority > 100000 {
		return ResolvedManifestScheduling{}, errors.New("priority is outside the supported range")
	}
	var err error
	scheduling.EnvironmentCapabilities, err = normalizeSchedulingValues(scheduling.EnvironmentCapabilities, 128, false)
	if err != nil {
		return ResolvedManifestScheduling{}, fmt.Errorf("environmentCapabilities: %w", err)
	}
	scheduling.ProviderProtocols, err = normalizeSchedulingValues(scheduling.ProviderProtocols, 2, true)
	if err != nil {
		return ResolvedManifestScheduling{}, fmt.Errorf("providerProtocols: %w", err)
	}
	for _, protocol := range scheduling.ProviderProtocols {
		if protocol != "OPENAI" && protocol != "ANTHROPIC" {
			return ResolvedManifestScheduling{}, errors.New("providerProtocols contains an unsupported value")
		}
	}
	scheduling.HookEvents, err = normalizeSchedulingValues(scheduling.HookEvents, 128, false)
	if err != nil {
		return ResolvedManifestScheduling{}, fmt.Errorf("hookEvents: %w", err)
	}
	scheduling.ExecutionKinds, err = normalizeSchedulingValues(scheduling.ExecutionKinds, 2, true)
	if err != nil {
		return ResolvedManifestScheduling{}, fmt.Errorf("executionKinds: %w", err)
	}
	for _, kind := range scheduling.ExecutionKinds {
		if kind != "AGENT_ATTEMPT" && kind != "REVIEW_RUN" {
			return ResolvedManifestScheduling{}, errors.New("executionKinds contains an unsupported value")
		}
	}
	environmentJSON, _ := json.Marshal(scheduling.EnvironmentCapabilities)
	executionKindsJSON, _ := json.Marshal(scheduling.ExecutionKinds)
	canonical, _ := json.Marshal(scheduling)
	sum := sha256.Sum256(canonical)
	return ResolvedManifestScheduling{
		Scope:                       scheduling.Scope,
		Priority:                    scheduling.Priority,
		EnvironmentCapabilitiesJSON: environmentJSON,
		ExecutionKindsJSON:          executionKindsJSON,
		Digest:                      hex.EncodeToString(sum[:]),
	}, nil
}

func normalizeSchedulingValues(values []string, maximum int, required bool) ([]string, error) {
	if len(values) > maximum || (required && len(values) == 0) {
		return nil, errors.New("list size is invalid")
	}
	result := make([]string, 0, len(values))
	seen := make(map[string]struct{}, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" || len(value) > 128 || !isSchedulingToken(value) {
			return nil, errors.New("value is invalid")
		}
		if _, exists := seen[value]; exists {
			return nil, errors.New("duplicate value")
		}
		seen[value] = struct{}{}
		result = append(result, value)
	}
	sort.Strings(result)
	return result, nil
}

func isSchedulingToken(value string) bool {
	for index, character := range value {
		if character >= 'a' && character <= 'z' || character >= 'A' && character <= 'Z' || character >= '0' && character <= '9' || (index > 0 && (character == '.' || character == '_' || character == '-')) {
			continue
		}
		return false
	}
	return value != ""
}

func (s *Service) List(ctx context.Context, limit int) ([]domain.Worker, error) {
	return s.repo.List(ctx, limit)
}
func (s *Service) Get(ctx context.Context, id int64) (domain.Worker, []domain.Image, error) {
	return s.repo.Get(ctx, id)
}
func (s *Service) ListAssignments(ctx context.Context, id int64, limit int) ([]domain.Assignment, error) {
	if _, _, err := s.repo.Get(ctx, id); err != nil {
		return nil, err
	}
	return s.repo.ListAssignments(ctx, id, limit)
}
func (s *Service) SetDesiredState(ctx context.Context, id int64, state string, version int64) error {
	state = strings.ToUpper(strings.TrimSpace(state))
	if state != domain.DesiredEnabled && state != domain.DesiredDraining && state != domain.DesiredDisabled {
		return errors.New("desiredState must be ENABLED, DRAINING, or DISABLED")
	}
	return s.repo.SetDesiredState(ctx, id, state, version)
}

func (s *Service) UpdateCapacity(ctx context.Context, id int64, maximum int, version int64) error {
	if id <= 0 || version <= 0 || maximum < 1 || maximum > 64 {
		return ErrInvalidCapacity
	}
	return s.repo.UpdateCapacity(ctx, id, maximum, version)
}

func (s *Service) RequestInventoryRefresh(ctx context.Context, id int64) (int64, error) {
	return s.repo.RequestInventoryRefresh(ctx, id)
}

func (s *Service) Decommission(ctx context.Context, id int64, version int64) (int, error) {
	w, _, err := s.repo.Get(ctx, id)
	if err != nil {
		return 0, err
	}
	if w.DecommissionedAt != "" {
		if s.topology == nil {
			return 0, nil
		}
		return s.topology.DeleteWorkerQueue(ctx, domain.QueueFor(w.ID, w.MaxActiveAttempts))
	}
	if w.State != domain.StateDisabled {
		return 0, ErrConflict
	}
	contract := domain.QueueFor(w.ID, w.MaxActiveAttempts)
	if err = s.repo.Decommission(ctx, w.ID, version); err != nil {
		return 0, err
	}
	if s.topology == nil {
		return 0, nil
	}
	return s.topology.DeleteWorkerQueue(ctx, contract)
}
func (s *Service) Deregister(ctx context.Context, workerID int64, instanceID string, reason string) error {
	return s.repo.Deregister(ctx, workerID, instanceID, reason)
}

func newToken() (string, string, string, error) {
	body := make([]byte, 32)
	if _, err := rand.Read(body); err != nil {
		return "", "", "", err
	}
	token := "swk_" + base64.RawURLEncoding.EncodeToString(body)
	sum := sha256.Sum256([]byte(token))
	hint := "…" + token[len(token)-4:]
	return token, hex.EncodeToString(sum[:]), hint, nil
}
