package agentprotocol

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"unicode/utf8"
)

const NetworkCommandToolName = "run_command_with_network_access"

const GitDeliveryToolName = "request_git_delivery"

const NetworkCommandToolDescription = "Run a shell command that requires outbound network access. Call this tool whenever the task container cannot reach a required external host; do not retry the command through the normal shell while this call is pending. Provide the purpose, exact shell command, repository-relative working directory, and every required host, protocol, and port. The platform may ask the user to allow this operation once or for the rest of the task."

const GitDeliveryToolDescription = "Request delivery of the finished and validated task result to the task's configured Git destination. Call this only after implementation and required checks are complete. The platform freezes the result, completes required review, and either uses the task's existing delivery authorization or asks the user to decide. This tool never exposes Git credentials and does not push directly."

const (
	MaxToolPurposeBytes        = 2 << 10
	MaxToolCommandBytes        = 16 << 10
	MaxToolTargets             = 8
	MaxNativeToolCallIDBytes   = 512
	MaxGitDeliverySummaryBytes = 2 << 10
)

var hostnamePattern = regexp.MustCompile(`^[a-z0-9](?:[a-z0-9-]{0,61}[a-z0-9])?(?:\.[a-z0-9](?:[a-z0-9-]{0,61}[a-z0-9])?)+$`)
var nativeToolCallIDPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._:-]{0,511}$`)
var toolNamePattern = regexp.MustCompile(`^[a-z][a-z0-9_]{0,63}$`)

type ToolTarget struct {
	Host     string `json:"host"`
	Protocol string `json:"protocol"`
	Port     int    `json:"port"`
}

type ToolDefinition struct {
	Name        string         `json:"name"`
	Description string         `json:"description"`
	InputSchema map[string]any `json:"inputSchema"`
}

type ToolCatalogPayload struct {
	RoundKey      string   `json:"roundKey"`
	ToolNames     []string `json:"toolNames"`
	CatalogDigest string   `json:"catalogDigest"`
}

func DigestToolCatalogNames(names []string) (string, error) {
	if len(names) > 16 {
		return "", ErrInvalid
	}
	normalized := append([]string(nil), names...)
	sort.Strings(normalized)
	for index, name := range normalized {
		if !toolNamePattern.MatchString(name) || (index > 0 && normalized[index-1] == name) {
			return "", ErrInvalid
		}
	}
	body, err := json.Marshal(normalized)
	if err != nil {
		return "", ErrInvalid
	}
	digest := sha256.Sum256(body)
	return hex.EncodeToString(digest[:]), nil
}

func NetworkCommandToolDefinition() ToolDefinition {
	return ToolDefinition{
		Name: NetworkCommandToolName, Description: NetworkCommandToolDescription,
		InputSchema: map[string]any{
			"type": "object", "additionalProperties": false,
			"required": []string{"purpose", "command", "cwd", "targets"},
			"properties": map[string]any{
				"purpose": map[string]any{"type": "string", "minLength": 1, "maxLength": MaxToolPurposeBytes},
				"command": map[string]any{"type": "string", "minLength": 1, "maxLength": MaxToolCommandBytes},
				"cwd":     map[string]any{"type": "string", "minLength": 1, "maxLength": 4096},
				"targets": map[string]any{"type": "array", "minItems": 1, "maxItems": MaxToolTargets, "items": map[string]any{"type": "object", "additionalProperties": false, "required": []string{"host", "protocol", "port"}, "properties": map[string]any{"host": map[string]any{"type": "string", "minLength": 1, "maxLength": 253}, "protocol": map[string]any{"type": "string", "enum": []string{"http", "https"}}, "port": map[string]any{"type": "integer", "minimum": 1, "maximum": 65535}}}},
			},
		},
	}
}

func GitDeliveryToolDefinition() ToolDefinition {
	return ToolDefinition{
		Name: GitDeliveryToolName, Description: GitDeliveryToolDescription,
		InputSchema: map[string]any{
			"type": "object", "additionalProperties": false,
			"required": []string{"summary"},
			"properties": map[string]any{
				"summary": map[string]any{"type": "string", "minLength": 1, "maxLength": MaxGitDeliverySummaryBytes},
			},
		},
	}
}

// ToolDefinitions resolves only platform-owned names supplied by the caller's
// frozen allowlist. Adapter capabilities alone never open a tool.
func ToolDefinitions(names []string) ([]ToolDefinition, error) {
	definitions := make([]ToolDefinition, 0, len(names))
	seen := make(map[string]struct{}, len(names))
	for _, name := range names {
		if _, exists := seen[name]; exists {
			return nil, ErrInvalid
		}
		seen[name] = struct{}{}
		switch name {
		case NetworkCommandToolName:
			definitions = append(definitions, NetworkCommandToolDefinition())
		case GitDeliveryToolName:
			definitions = append(definitions, GitDeliveryToolDefinition())
		default:
			return nil, ErrUnsupported
		}
	}
	return definitions, nil
}

type NetworkCommandArguments struct {
	Purpose string       `json:"purpose"`
	Command string       `json:"command"`
	CWD     string       `json:"cwd"`
	Targets []ToolTarget `json:"targets"`
}

type GitDeliveryArguments struct {
	Summary string `json:"summary"`
}

type ToolRequestPayload struct {
	ToolName         string          `json:"toolName"`
	LocalToolCallKey string          `json:"localToolCallKey"`
	NativeToolCallID string          `json:"nativeToolCallId"`
	RoundKey         string          `json:"roundKey"`
	Arguments        json.RawMessage `json:"arguments"`
	ArgumentsDigest  string          `json:"argumentsDigest"`
}

type ToolResultPayload struct {
	RoundKey         string `json:"roundKey"`
	LocalToolCallKey string `json:"localToolCallKey"`
	NativeToolCallID string `json:"nativeToolCallId"`
	OperationID      int64  `json:"operationId,string"`
	Status           string `json:"status"`
	ExitCode         *int   `json:"exitCode"`
	Summary          string `json:"summary"`
	Output           string `json:"output"`
	ResultDigest     string `json:"resultDigest"`
	ReasonCode       string `json:"reasonCode"`
}

type ToolReceiptPayload struct {
	RoundKey         string `json:"roundKey"`
	LocalToolCallKey string `json:"localToolCallKey"`
	OperationID      int64  `json:"operationId,string"`
	Status           string `json:"status"`
}

type ToolClosedPayload struct {
	RoundKey         string `json:"roundKey"`
	LocalToolCallKey string `json:"localToolCallKey"`
	Reason           string `json:"reason"`
}

func NormalizeNetworkCommandArguments(in NetworkCommandArguments) (NetworkCommandArguments, error) {
	in.Purpose = strings.TrimSpace(in.Purpose)
	in.Command = strings.TrimSpace(in.Command)
	in.CWD = strings.TrimSpace(in.CWD)
	if in.CWD == "" {
		in.CWD = "."
	}
	if in.Purpose == "" || len([]byte(in.Purpose)) > MaxToolPurposeBytes || in.Command == "" || len([]byte(in.Command)) > MaxToolCommandBytes || len([]byte(in.CWD)) > 4096 || len(in.Targets) == 0 || len(in.Targets) > MaxToolTargets {
		return NetworkCommandArguments{}, ErrInvalid
	}
	if filepath.IsAbs(in.CWD) || filepath.Clean(in.CWD) != in.CWD || in.CWD == ".." || strings.HasPrefix(in.CWD, "../") || strings.ContainsRune(in.CWD, '\x00') {
		return NetworkCommandArguments{}, ErrInvalid
	}
	seen := map[string]struct{}{}
	for i := range in.Targets {
		t := &in.Targets[i]
		t.Host = strings.ToLower(strings.TrimSuffix(strings.TrimSpace(t.Host), "."))
		t.Protocol = strings.ToLower(strings.TrimSpace(t.Protocol))
		if (t.Protocol != "http" && t.Protocol != "https") || t.Port < 1 || t.Port > 65535 || net.ParseIP(t.Host) != nil || !hostnamePattern.MatchString(t.Host) || strings.ContainsAny(t.Host, "*/:@[]") {
			return NetworkCommandArguments{}, ErrInvalid
		}
		key := t.Protocol + "\x00" + t.Host + "\x00" + strconv.Itoa(t.Port)
		if _, ok := seen[key]; ok {
			return NetworkCommandArguments{}, ErrInvalid
		}
		seen[key] = struct{}{}
	}
	sort.Slice(in.Targets, func(i, j int) bool {
		a, b := in.Targets[i], in.Targets[j]
		if a.Host != b.Host {
			return a.Host < b.Host
		}
		if a.Protocol != b.Protocol {
			return a.Protocol < b.Protocol
		}
		return a.Port < b.Port
	})
	return in, nil
}

func DigestToolArguments(raw json.RawMessage) (string, error) {
	return DigestToolArgumentsFor(NetworkCommandToolName, raw)
}

func DigestToolArgumentsFor(toolName string, raw json.RawMessage) (string, error) {
	if !utf8.Valid(raw) {
		return "", ErrInvalid
	}
	var normalized any
	switch toolName {
	case NetworkCommandToolName:
		var args NetworkCommandArguments
		decoder := json.NewDecoder(bytes.NewReader(raw))
		decoder.DisallowUnknownFields()
		if decoder.Decode(&args) != nil || decoder.Decode(&struct{}{}) == nil {
			return "", ErrInvalid
		}
		value, err := NormalizeNetworkCommandArguments(args)
		if err != nil {
			return "", err
		}
		normalized = value
	case GitDeliveryToolName:
		var args GitDeliveryArguments
		decoder := json.NewDecoder(bytes.NewReader(raw))
		decoder.DisallowUnknownFields()
		if decoder.Decode(&args) != nil || decoder.Decode(&struct{}{}) == nil {
			return "", ErrInvalid
		}
		args.Summary = strings.TrimSpace(args.Summary)
		if args.Summary == "" || len([]byte(args.Summary)) > MaxGitDeliverySummaryBytes {
			return "", ErrInvalid
		}
		normalized = args
	default:
		return "", ErrUnsupported
	}
	body, err := json.Marshal(normalized)
	if err != nil {
		return "", ErrInvalid
	}
	digest := sha256.Sum256(body)
	return hex.EncodeToString(digest[:]), nil
}

func ValidateToolPayload(method string, raw json.RawMessage) error {
	switch method {
	case "tool.catalog":
		var p ToolCatalogPayload
		if decodeStrictPayload(raw, &p) != nil || !keyPattern.MatchString(p.RoundKey) {
			return ErrInvalid
		}
		for index := 1; index < len(p.ToolNames); index++ {
			if p.ToolNames[index-1] >= p.ToolNames[index] {
				return ErrInvalid
			}
		}
		digest, err := DigestToolCatalogNames(p.ToolNames)
		if err != nil || digest != p.CatalogDigest {
			return ErrInvalid
		}
	case "tool.request":
		var p ToolRequestPayload
		if decodeStrictPayload(raw, &p) != nil || !keyPattern.MatchString(p.LocalToolCallKey) || !ValidNativeToolCallID(p.NativeToolCallID) || !keyPattern.MatchString(p.RoundKey) {
			return ErrInvalid
		}
		digest, err := DigestToolArgumentsFor(p.ToolName, p.Arguments)
		if err != nil || digest != p.ArgumentsDigest {
			return ErrInvalid
		}
	case "tool.result":
		var p ToolResultPayload
		if decodeStrictPayload(raw, &p) != nil || p.OperationID <= 0 || !keyPattern.MatchString(p.RoundKey) || !keyPattern.MatchString(p.LocalToolCallKey) || !ValidNativeToolCallID(p.NativeToolCallID) {
			return ErrInvalid
		}
		switch p.Status {
		case "SUCCEEDED", "FAILED":
			if p.ExitCode == nil || len([]byte(p.Output)) > MaxMessageBytes || len([]byte(p.Summary)) > 2048 || !isLowerHex64(p.ResultDigest) || p.ReasonCode != "" {
				return ErrInvalid
			}
		case "SCHEDULED":
			if p.ExitCode != nil || p.Output != "" || p.Summary == "" || len([]byte(p.Summary)) > 2048 || !isLowerHex64(p.ResultDigest) || p.ReasonCode != "" {
				return ErrInvalid
			}
		case "DENIED", "OUTCOME_UNKNOWN", "CANCELLED", "EXPIRED":
			if p.ExitCode != nil || p.Output != "" || p.ResultDigest != "" || !keyPattern.MatchString(p.ReasonCode) {
				return ErrInvalid
			}
		default:
			return ErrInvalid
		}
	case "tool.received":
		var p ToolReceiptPayload
		if decodeStrictPayload(raw, &p) != nil || p.OperationID <= 0 || !keyPattern.MatchString(p.RoundKey) || !keyPattern.MatchString(p.LocalToolCallKey) || !keyPattern.MatchString(p.Status) {
			return ErrInvalid
		}
	case "tool.closed":
		var p ToolClosedPayload
		if decodeStrictPayload(raw, &p) != nil || !keyPattern.MatchString(p.RoundKey) || !keyPattern.MatchString(p.LocalToolCallKey) || !keyPattern.MatchString(p.Reason) {
			return ErrInvalid
		}
	}
	return nil
}

// ValidNativeToolCallID validates an opaque App Server or other Adapter-native
// tool-call identity. It is deliberately stricter than arbitrary TEXT because
// the value crosses process, persistence, hot-delivery, and cold-resume seams.
func ValidNativeToolCallID(value string) bool {
	return len(value) <= MaxNativeToolCallIDBytes && nativeToolCallIDPattern.MatchString(value)
}

func decodeStrictPayload(raw []byte, out any) error {
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(out); err != nil {
		return err
	}
	if err := decoder.Decode(&struct{}{}); err == nil {
		return ErrInvalid
	}
	return nil
}

func isLowerHex64(value string) bool {
	decoded, err := hex.DecodeString(value)
	return err == nil && len(decoded) == 32 && strings.ToLower(value) == value
}
