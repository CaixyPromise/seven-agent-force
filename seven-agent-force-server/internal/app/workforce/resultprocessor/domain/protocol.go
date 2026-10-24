// Package domain defines the immutable Result Processor asset and the result
// contract which crosses the Supervisor -> Worker -> Server boundary.
package domain

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"path"
	"regexp"
	"sort"
	"strings"
	"unicode/utf8"

	asset "github.com/CaixyPromise/seven-agent/force/internal/app/workforce/scriptasset/domain"
)

const (
	ProcessorSchema = "seven.result-processor"
	ManifestFile    = "seven-result-processor.json"
	EntrypointFile  = "processor.py"

	DefaultMaxEvaluationCycles = 3
	MaximumEvaluationCycles    = 5
	MaxContractBytes           = 1 << 20
)

var (
	processEntrypoint = regexp.MustCompile(`(?m)^\s*(?:async\s+)?def\s+process\s*\(\s*context\s*\)\s*:`)
	deliverEntrypoint = regexp.MustCompile(`(?m)^\s*(?:async\s+)?def\s+deliver\s*\(\s*context\s*\)\s*:`)
)

// SourceFile is the shared canonical asset file shape.
type SourceFile = asset.SourceFile

// Manifest describes a Python-only Result Processor release. Template
// provenance intentionally belongs to authoring records, not to this runtime
// manifest or an execution snapshot.
type Manifest struct {
	SchemaVersion       string             `json:"schemaVersion"`
	Name                string             `json:"name"`
	Version             string             `json:"version,omitempty"`
	Category            string             `json:"category,omitempty"`
	Runtime             PythonRuntime      `json:"runtime"`
	ParametersSchema    json.RawMessage    `json:"parametersSchema,omitempty"`
	Environment         []EnvironmentField `json:"environment,omitempty"`
	MaxEvaluationCycles int                `json:"maxEvaluationCycles,omitempty"`
	ContinuationPolicy  string             `json:"continuationPolicy,omitempty"`
	DeliveryPolicy      string             `json:"deliveryPolicy,omitempty"`
	Capabilities        []string           `json:"capabilities,omitempty"`
}

type PythonRuntime struct {
	Kind        string `json:"kind"`
	Requirement string `json:"requirement,omitempty"`
}

type EnvironmentField struct {
	Name                string          `json:"name"`
	Type                string          `json:"type"`
	Phase               string          `json:"phase"`
	Required            bool            `json:"required,omitempty"`
	TaskOverrideAllowed bool            `json:"taskOverrideAllowed,omitempty"`
	DefaultValue        json.RawMessage `json:"defaultValue,omitempty"`
	SecretRef           string          `json:"secretRef,omitempty"`
}

// Validation is persisted for editor feedback and safely contains only asset
// metadata and reason codes.
type Validation struct {
	Valid        bool     `json:"valid"`
	ReasonCode   string   `json:"reasonCode"`
	Errors       []string `json:"errors"`
	ManifestHash string   `json:"manifestSha256,omitempty"`
	SourceHash   string   `json:"sourceSha256,omitempty"`
}

// ResultContract is returned as a Python object from process(context).  The
// protected bootstrap serializes it; stdout is therefore never the business
// protocol channel.
type ResultContract struct {
	Decision     string                `json:"decision"`
	ReasonCode   string                `json:"reasonCode"`
	Message      string                `json:"message"`
	Checks       []Check               `json:"checks,omitempty"`
	Findings     []Finding             `json:"findings,omitempty"`
	Questions    []Question            `json:"questions,omitempty"`
	Outputs      []Output              `json:"outputs,omitempty"`
	Actions      []Action              `json:"actions,omitempty"`
	Continuation ContinuationSelection `json:"continuation,omitempty"`
}

type Check struct {
	Name       string `json:"name"`
	Status     string `json:"status"`
	ReasonCode string `json:"reasonCode,omitempty"`
	Message    string `json:"message,omitempty"`
}

type Finding struct {
	Code     string `json:"code"`
	Message  string `json:"message"`
	Path     string `json:"path,omitempty"`
	Severity string `json:"severity,omitempty"`
}

type Question struct {
	ID       string `json:"id"`
	Question string `json:"question"`
	Required bool   `json:"required,omitempty"`
}

type Output struct {
	Name          string        `json:"name"`
	Kind          string        `json:"kind"`
	Path          string        `json:"path,omitempty"`
	Selection     PathSelection `json:"selection,omitempty"`
	ArchiveFormat string        `json:"archiveFormat,omitempty"`
	Storage       string        `json:"storage"`
}

type Action struct {
	Type       string          `json:"type"`
	When       string          `json:"when,omitempty"`
	TargetID   int64           `json:"targetId,string"`
	BaseBranch string          `json:"baseBranch,omitempty"`
	Branch     string          `json:"branch,omitempty"`
	Operation  string          `json:"operation,omitempty"`
	Payload    json.RawMessage `json:"payload,omitempty"`
}

type PathSelection struct {
	Includes []string `json:"includes"`
	Excludes []string `json:"excludes,omitempty"`
}

type ContinuationSelection struct {
	Includes []string `json:"includes,omitempty"`
}

func DefaultManifest(name string) Manifest {
	return Manifest{
		SchemaVersion:       ProcessorSchema,
		Name:                strings.TrimSpace(name),
		Runtime:             PythonRuntime{Kind: "python", Requirement: ">=3.11 <3.14"},
		MaxEvaluationCycles: DefaultMaxEvaluationCycles,
		ContinuationPolicy:  "CHECKPOINT_REQUIRED",
		DeliveryPolicy:      "OPTIONAL",
	}
}

// Validate applies server-side synchronous validation. Exact interpreter and
// module availability remains the responsibility of the Worker image fixture.
func Validate(manifest Manifest, files []SourceFile) Validation {
	result := Validation{ReasonCode: "RESULT_PROCESSOR_INVALID", Errors: []string{}}
	if manifest.SchemaVersion != ProcessorSchema {
		result.Errors = append(result.Errors, "schemaVersion must be seven.result-processor")
	}
	if strings.TrimSpace(manifest.Name) == "" || len(manifest.Name) > 128 {
		result.Errors = append(result.Errors, "name is required and must be at most 128 characters")
	}
	if strings.ToLower(strings.TrimSpace(manifest.Runtime.Kind)) != "python" {
		result.Errors = append(result.Errors, "runtime.kind must be python")
	}
	if manifest.Runtime.Requirement != "" && manifest.Runtime.Requirement != ">=3.11 <3.14" {
		result.Errors = append(result.Errors, "python runtime requirement must be >=3.11 <3.14")
	}
	if manifest.MaxEvaluationCycles == 0 {
		manifest.MaxEvaluationCycles = DefaultMaxEvaluationCycles
	}
	if manifest.MaxEvaluationCycles < 1 || manifest.MaxEvaluationCycles > MaximumEvaluationCycles {
		result.Errors = append(result.Errors, "maxEvaluationCycles must be between 1 and 5")
	}
	if manifest.ContinuationPolicy == "" {
		manifest.ContinuationPolicy = "CHECKPOINT_REQUIRED"
	}
	if manifest.ContinuationPolicy != "CHECKPOINT_REQUIRED" {
		result.Errors = append(result.Errors, "continuationPolicy must be CHECKPOINT_REQUIRED")
	}
	if manifest.DeliveryPolicy == "" {
		manifest.DeliveryPolicy = "OPTIONAL"
	}
	if manifest.DeliveryPolicy != "OPTIONAL" && manifest.DeliveryPolicy != "DECLARED" {
		result.Errors = append(result.Errors, "deliveryPolicy must be OPTIONAL or DECLARED")
	}
	if err := ValidateParametersSchema(manifest.ParametersSchema); err != nil {
		result.Errors = append(result.Errors, err.Error())
	}

	fileValidation := asset.ValidateFiles(files, ManifestFile)
	result.Errors = append(result.Errors, fileValidation.Errors...)
	result.SourceHash = fileValidation.SourceHash
	filesByPath := make(map[string]SourceFile, len(files))
	for _, file := range files {
		clean, err := asset.CleanPath(file.Path)
		if err != nil {
			continue
		}
		filesByPath[clean] = file
		if clean == "seven_workforce_runtime.py" || strings.HasPrefix(clean, "seven_workforce_runtime/") ||
			clean == "seven_agent.py" || clean == "seven_agent.force.py" || strings.HasPrefix(clean, "seven_agent/") ||
			strings.HasPrefix(clean, "_runtime/") || strings.HasPrefix(clean, "result-runtime/") {
			result.Errors = append(result.Errors, "platform-reserved source path: "+clean)
		}
		if path.Ext(clean) != ".py" {
			result.Errors = append(result.Errors, "Result Processor supports pure Python source only: "+clean)
		}
		if forbiddenPythonSource(file.Content) {
			result.Errors = append(result.Errors, "runtime package installation is forbidden: "+clean)
		}
	}
	entry, present := filesByPath[EntrypointFile]
	if !present {
		result.Errors = append(result.Errors, "processor.py is required")
	} else {
		if !processEntrypoint.MatchString(entry.Content) {
			result.Errors = append(result.Errors, "processor.py must define process(context)")
		}
		declaredDelivery := contains(manifest.Capabilities, "PROCESSOR_DELIVER") || manifest.DeliveryPolicy == "DECLARED"
		if declaredDelivery != deliverEntrypoint.MatchString(entry.Content) {
			result.Errors = append(result.Errors, "PROCESSOR_DELIVER declaration must match optional deliver(context) entrypoint")
		}
	}
	seenEnvironment := map[string]struct{}{}
	for _, field := range manifest.Environment {
		name := strings.TrimSpace(field.Name)
		if !validEnvironmentName(name) {
			result.Errors = append(result.Errors, "environment variable name is invalid: "+name)
			continue
		}
		if _, exists := seenEnvironment[name]; exists {
			result.Errors = append(result.Errors, "duplicate environment variable: "+name)
		}
		seenEnvironment[name] = struct{}{}
		if field.Type != "CONFIG" && field.Type != "SECRET" {
			result.Errors = append(result.Errors, "environment type must be CONFIG or SECRET: "+name)
		}
		if field.Phase != "EVALUATE" && field.Phase != "DELIVER" {
			result.Errors = append(result.Errors, "environment phase must be EVALUATE or DELIVER: "+name)
		}
		if field.Type == "SECRET" && strings.TrimSpace(field.SecretRef) == "" {
			result.Errors = append(result.Errors, "SECRET environment requires secretRef: "+name)
		}
		if len(field.DefaultValue) != 0 && !json.Valid(field.DefaultValue) {
			result.Errors = append(result.Errors, "environment defaultValue must be JSON: "+name)
		}
	}
	canonicalManifest, _ := canonicalManifest(manifest)
	result.ManifestHash = asset.SHA256(canonicalManifest)
	result.Valid = len(result.Errors) == 0
	if result.Valid {
		result.ReasonCode = "OK"
	}
	return result
}

func CanonicalArchive(manifest Manifest, files []SourceFile) ([]byte, string, error) {
	validation := Validate(manifest, files)
	if !validation.Valid {
		return nil, "", errors.New(strings.Join(validation.Errors, "; "))
	}
	body, err := canonicalManifest(manifest)
	if err != nil {
		return nil, "", err
	}
	return asset.CanonicalArchive(ManifestFile, append(body, '\n'), files)
}

func ImportArchive(body []byte) (Manifest, []SourceFile, error) {
	manifestBody, files, err := asset.ImportArchive(ManifestFile, body)
	if err != nil {
		return Manifest{}, nil, err
	}
	decoder := json.NewDecoder(bytes.NewReader(manifestBody))
	decoder.DisallowUnknownFields()
	var manifest Manifest
	if err := decoder.Decode(&manifest); err != nil {
		return Manifest{}, nil, fmt.Errorf("decode Result Processor manifest: %w", err)
	}
	if err := requireEOF(decoder); err != nil {
		return Manifest{}, nil, err
	}
	validation := Validate(manifest, files)
	if !validation.Valid {
		return Manifest{}, nil, errors.New(strings.Join(validation.Errors, "; "))
	}
	return manifest, files, nil
}

// ValidateContract rejects unbounded or ambiguous Processor results before the
// Server ever writes an EvaluationCycle. It intentionally does not decide task
// success; the Result Coordinator remains independent.
func ValidateContract(body []byte) (ResultContract, error) {
	if len(body) == 0 || len(body) > MaxContractBytes || !utf8.Valid(body) {
		return ResultContract{}, errors.New("Result Contract is empty, oversized, or not UTF-8")
	}
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.DisallowUnknownFields()
	var contract ResultContract
	if err := decoder.Decode(&contract); err != nil {
		return ResultContract{}, fmt.Errorf("decode Result Contract: %w", err)
	}
	if err := requireEOF(decoder); err != nil {
		return ResultContract{}, err
	}
	if !isDecision(contract.Decision) || strings.TrimSpace(contract.ReasonCode) == "" || len(contract.ReasonCode) > 128 || len(contract.Message) > 8<<10 {
		return ResultContract{}, errors.New("Result Contract decision, reasonCode, or message is invalid")
	}
	if len(contract.Checks) > 256 || len(contract.Findings) > 256 || len(contract.Questions) > 64 || len(contract.Outputs) > 64 || len(contract.Actions) > 64 {
		return ResultContract{}, errors.New("Result Contract exceeds bounded collection limits")
	}
	outputNames := make(map[string]struct{}, len(contract.Outputs))
	outputPaths := make(map[string]struct{}, len(contract.Outputs))
	for _, output := range contract.Outputs {
		name := strings.TrimSpace(output.Name)
		if name == "" || len(name) > 256 || (output.Storage != "IMMEDIATE" && output.Storage != "ON_DEMAND") || path.Base(name) != name {
			return ResultContract{}, errors.New("Result Contract output must have a name and IMMEDIATE or ON_DEMAND storage")
		}
		switch output.Kind {
		case "PATH":
			clean, err := asset.CleanPath(output.Path)
			if err != nil || clean != output.Path || len(output.Selection.Includes) != 0 || len(output.Selection.Excludes) != 0 || output.ArchiveFormat != "" {
				return ResultContract{}, errors.New("PATH output declaration is invalid")
			}
			if _, exists := outputPaths[clean]; exists {
				return ResultContract{}, errors.New("Result Contract output path is duplicated")
			}
			outputPaths[clean] = struct{}{}
		case "ARCHIVE":
			if output.Path != "" || (output.ArchiveFormat != "ZIP" && output.ArchiveFormat != "TAR_GZ") || len(output.Selection.Includes) == 0 {
				return ResultContract{}, errors.New("ARCHIVE output requires a selection and ZIP or TAR_GZ format")
			}
			if err := validatePathSelection(output.Selection); err != nil {
				return ResultContract{}, err
			}
		default:
			return ResultContract{}, errors.New("Result Contract output kind must be PATH or ARCHIVE")
		}
		if _, exists := outputNames[name]; exists {
			return ResultContract{}, errors.New("Result Contract output name is duplicated")
		}
		outputNames[name] = struct{}{}
	}
	if err := validatePathSelection(PathSelection{Includes: contract.Continuation.Includes}); err != nil {
		return ResultContract{}, fmt.Errorf("continuation selection: %w", err)
	}
	for _, question := range contract.Questions {
		if strings.TrimSpace(question.ID) == "" || strings.TrimSpace(question.Question) == "" {
			return ResultContract{}, errors.New("Result Contract question is invalid")
		}
	}
	deliveryActionCount := 0
	for _, action := range contract.Actions {
		if err := validateAction(action); err != nil {
			return ResultContract{}, err
		}
		if action.Type == "GIT_PUSH" || action.Type == "CUSTOM_DELIVERY" {
			deliveryActionCount++
		}
	}
	if deliveryActionCount > 1 {
		return ResultContract{}, errors.New("Result Contract may contain at most one delivery action")
	}
	return contract, nil
}

// validateAction freezes the small declarative action surface that a Result
// Processor can propose. GIT_PUSH is executed only by the platform Git
// adapter; CUSTOM_DELIVERY is the only action allowed to invoke deliver().
func validateAction(action Action) error {
	if action.Type != "GIT_PUSH" && action.Type != "CUSTOM_DELIVERY" {
		return errors.New("Result Contract action type is not supported")
	}
	when := action.When
	if when == "" {
		when = "AFTER_APPROVAL"
	}
	if when != "ON_PASS" && when != "AFTER_APPROVAL" && when != "MANUAL_ONLY" {
		return errors.New("Result Contract action timing is invalid")
	}
	if action.TargetID <= 0 {
		return errors.New("Result Contract action targetId is required")
	}
	if action.Type == "GIT_PUSH" {
		if !validGitRefName(action.BaseBranch) || !validGitRefName(action.Branch) || action.Operation != "" || len(action.Payload) != 0 {
			return errors.New("GIT_PUSH action fields are invalid")
		}
		return nil
	}
	if strings.TrimSpace(action.Operation) == "" || len(action.Operation) > 128 || action.BaseBranch != "" || action.Branch != "" {
		return errors.New("CUSTOM_DELIVERY action fields are invalid")
	}
	if len(action.Payload) > 32<<10 || (len(action.Payload) != 0 && !validJSONObject(action.Payload)) {
		return errors.New("CUSTOM_DELIVERY payload must be a bounded JSON object")
	}
	return nil
}

func validatePathSelection(selection PathSelection) error {
	if len(selection.Includes) > 256 || len(selection.Excludes) > 256 {
		return errors.New("path selection exceeds pattern limit")
	}
	for _, pattern := range append(append([]string(nil), selection.Includes...), selection.Excludes...) {
		pattern = strings.TrimSpace(pattern)
		if pattern == "" || len(pattern) > 512 || strings.HasPrefix(pattern, "/") || strings.Contains(pattern, "\\") {
			return errors.New("path selection contains an invalid pattern")
		}
		for _, part := range strings.Split(pattern, "/") {
			if part == ".." || part == ".git" || part == ".seven" || part == "codex" || part == "session" {
				return errors.New("path selection contains a protected path")
			}
		}
	}
	return nil
}

func validGitRefName(value string) bool {
	value = strings.TrimSpace(value)
	if value == "" || len(value) > 255 || strings.HasPrefix(value, "-") || strings.HasPrefix(value, "/") || strings.HasSuffix(value, "/") || strings.HasSuffix(value, ".") || strings.Contains(value, "..") || strings.Contains(value, "@{") || strings.ContainsAny(value, " ~^:?*[\\") {
		return false
	}
	for _, part := range strings.Split(value, "/") {
		if part == "" || part == "." || part == ".." || strings.HasSuffix(part, ".lock") {
			return false
		}
	}
	return true
}

func ContractDigest(contract ResultContract) (string, error) {
	body, err := json.Marshal(contract)
	if err != nil {
		return "", err
	}
	if _, err := ValidateContract(body); err != nil {
		return "", err
	}
	return asset.SHA256(body), nil
}

func IsDecision(value string) bool { return isDecision(value) }

func isDecision(value string) bool {
	switch value {
	case "PASS", "REVISE", "WAIT_USER", "REJECT", "MANUAL_REVIEW":
		return true
	default:
		return false
	}
}

func canonicalManifest(manifest Manifest) ([]byte, error) {
	if manifest.MaxEvaluationCycles == 0 {
		manifest.MaxEvaluationCycles = DefaultMaxEvaluationCycles
	}
	if manifest.ContinuationPolicy == "" {
		manifest.ContinuationPolicy = "CHECKPOINT_REQUIRED"
	}
	if manifest.DeliveryPolicy == "" {
		manifest.DeliveryPolicy = "OPTIONAL"
	}
	manifest.Runtime.Kind = strings.ToLower(strings.TrimSpace(manifest.Runtime.Kind))
	manifest.Name = strings.TrimSpace(manifest.Name)
	manifest.Capabilities = append([]string(nil), manifest.Capabilities...)
	sort.Strings(manifest.Capabilities)
	manifest.Environment = append([]EnvironmentField(nil), manifest.Environment...)
	sort.Slice(manifest.Environment, func(i, j int) bool { return manifest.Environment[i].Name < manifest.Environment[j].Name })
	return json.MarshalIndent(manifest, "", "  ")
}

func requireEOF(decoder *json.Decoder) error {
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		return errors.New("JSON must contain exactly one object")
	}
	return nil
}

func validJSONObject(body []byte) bool {
	var value map[string]any
	return json.Unmarshal(body, &value) == nil
}

func contains(values []string, expected string) bool {
	for _, value := range values {
		if value == expected {
			return true
		}
	}
	return false
}

func validEnvironmentName(value string) bool {
	if value == "" || len(value) > 128 {
		return false
	}
	for index, character := range value {
		if !(character == '_' || character >= 'A' && character <= 'Z' || index > 0 && character >= '0' && character <= '9') {
			return false
		}
	}
	return true
}

func forbiddenPythonSource(body string) bool {
	lower := strings.ToLower(body)
	for _, needle := range []string{"pip install", "pip3 install", "python -m pip", "subprocess.run(['pip'", "subprocess.run(\"pip\"", "os.system('pip", "os.system(\"pip"} {
		if strings.Contains(lower, needle) {
			return true
		}
	}
	return false
}
