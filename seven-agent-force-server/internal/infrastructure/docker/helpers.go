package docker

import (
	"encoding/json"
	"fmt"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"
)

func stripSHA(value string) string {
	if strings.HasPrefix(value, "sha256:") {
		return strings.TrimPrefix(value, "sha256:")
	}
	return value
}

func firstNonBlank(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func paginate[T any](items []T, current, size int64) *PageResult[T] {
	if current <= 0 {
		current = 1
	}
	if size <= 0 {
		size = 10
	}
	total := int64(len(items))
	start := (current - 1) * size
	if start >= total {
		return &PageResult[T]{Current: current, Size: size, Total: total, Records: []T{}}
	}
	end := start + size
	if end > total {
		end = total
	}
	return &PageResult[T]{
		Current: current,
		Size:    size,
		Total:   total,
		Records: items[start:end],
	}
}

func normalizeInspect(value any) map[string]any {
	if value == nil {
		return map[string]any{}
	}
	payload, err := json.Marshal(value)
	if err != nil {
		return map[string]any{"raw": fmt.Sprint(value)}
	}
	var result map[string]any
	if err := json.Unmarshal(payload, &result); err != nil {
		return map[string]any{"raw": string(payload)}
	}
	sanitizeInspectValue(result)
	return result
}

func sanitizeInspectValue(value any) {
	switch typed := value.(type) {
	case map[string]any:
		for key, nested := range typed {
			if isSensitiveKey(key) {
				typed[key] = "******"
				continue
			}
			if strings.EqualFold(key, "env") {
				typed[key] = sanitizeEnvList(nested)
				continue
			}
			sanitizeInspectValue(nested)
		}
	case []any:
		for _, item := range typed {
			sanitizeInspectValue(item)
		}
	}
}

func sanitizeEnvList(value any) any {
	items, ok := value.([]any)
	if !ok {
		return value
	}
	result := make([]any, 0, len(items))
	for _, item := range items {
		text, ok := item.(string)
		if !ok {
			result = append(result, item)
			continue
		}
		key, _, found := strings.Cut(text, "=")
		if found && isSensitiveKey(key) {
			result = append(result, strings.TrimSpace(key)+"=******")
			continue
		}
		result = append(result, text)
	}
	return result
}

func isSensitiveKey(key string) bool {
	normalized := strings.ToLower(strings.TrimSpace(key))
	if normalized == "" {
		return false
	}
	for _, marker := range []string{"password", "passwd", "pwd", "token", "secret", "authorization", "credential", "private_key", "apikey", "api_key"} {
		if strings.Contains(normalized, marker) {
			return true
		}
	}
	return false
}

func maskSensitiveValue(key, value string) string {
	if isSensitiveKey(key) {
		return "******"
	}
	return value
}

func stringValue(value any) string {
	switch typed := value.(type) {
	case string:
		return strings.TrimSpace(typed)
	case fmt.Stringer:
		return strings.TrimSpace(typed.String())
	default:
		if value == nil {
			return ""
		}
		return strings.TrimSpace(fmt.Sprint(value))
	}
}

func intValue(value any) int {
	switch typed := value.(type) {
	case int:
		return typed
	case int64:
		return int(typed)
	case float64:
		return int(typed)
	case json.Number:
		i, _ := typed.Int64()
		return int(i)
	case string:
		i, _ := strconv.Atoi(strings.TrimSpace(typed))
		return i
	default:
		return 0
	}
}

func encodeRepository(repository string) string {
	parts := strings.Split(repository, "/")
	for i := range parts {
		parts[i] = url.PathEscape(parts[i])
	}
	return strings.Join(parts, "/")
}

func allowRepository(rt registryRuntime, repository string) bool {
	raw := strings.TrimSpace(rt.NamespaceWhitelistJSON)
	if raw == "" {
		return true
	}
	var rules []string
	if err := json.Unmarshal([]byte(raw), &rules); err != nil || len(rules) == 0 {
		return true
	}
	for _, rule := range rules {
		rule = strings.TrimSpace(rule)
		if rule != "" && strings.HasPrefix(repository, rule) {
			return true
		}
	}
	return false
}

func fillManifestMetadata(view *RemoteManifestView, payload map[string]any) {
	if view == nil || payload == nil {
		return
	}
	if manifests, ok := payload["manifests"].([]any); ok && len(manifests) > 0 {
		view.ChildManifestCount = len(manifests)
		osSet := map[string]struct{}{}
		archSet := map[string]struct{}{}
		for _, item := range manifests {
			entry, _ := item.(map[string]any)
			platform, _ := entry["platform"].(map[string]any)
			os := stringValue(platform["os"])
			arch := stringValue(platform["architecture"])
			variant := stringValue(platform["variant"])
			if os != "" {
				osSet[os] = struct{}{}
			}
			if arch != "" {
				if variant != "" {
					arch += "/" + variant
				}
				archSet[arch] = struct{}{}
			}
		}
		view.OS = strings.Join(sortedKeys(osSet), ", ")
		view.Architecture = strings.Join(sortedKeys(archSet), ", ")
	}
	if layers, ok := payload["layers"].([]any); ok {
		view.LayerCount = len(layers)
	}
	if annotations, ok := payload["annotations"].(map[string]any); ok {
		view.Created = firstNonBlank(stringValue(annotations["org.opencontainers.image.created"]), view.Created)
	}
	view.Created = firstNonBlank(view.Created, stringValue(payload["created"]))
}

func sortedKeys(values map[string]struct{}) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func sortStrings(values []string) {
	sort.Strings(values)
}

func safeStrings(values []string) []string {
	if values == nil {
		return []string{}
	}
	result := make([]string, 0, len(values))
	for _, value := range values {
		if value != "" {
			result = append(result, value)
		}
	}
	return result
}

func safeLabels(values map[string]string) map[string]string {
	if len(values) == 0 {
		return map[string]string{}
	}
	result := make(map[string]string, len(values))
	for key, value := range values {
		result[key] = maskSensitiveValue(key, value)
	}
	return result
}

func contains(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}

func trimValues(values []string) []string {
	result := make([]string, 0, len(values))
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			result = append(result, strings.TrimSpace(value))
		}
	}
	return result
}

func firstString(values []string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func suggestServiceName(imageReference string) string {
	candidate := imageReference
	if slash := strings.LastIndex(candidate, "/"); slash >= 0 {
		candidate = candidate[slash+1:]
	}
	if colon := strings.Index(candidate, ":"); colon >= 0 {
		candidate = candidate[:colon]
	}
	candidate = strings.Trim(candidate, "._-")
	if candidate == "" {
		candidate = "service"
	}
	replacer := strings.NewReplacer(".", "-", ":", "-", "/", "-")
	return strings.ToLower(replacer.Replace(candidate))
}

func buildComposeYaml(serviceName, imageReference string, environment []KeyValueCommand, ports []PortBindingCommand, volumes []VolumeBindingCommand, labels []KeyValueCommand, workingDir, user string, entrypoint, command []string, tty, stdinOpen bool, networkMode string, privileged bool, restartPolicy string, publishAllPorts bool, capAdd, capDrop []string, projectName string) string {
	var yaml strings.Builder
	if strings.TrimSpace(projectName) != "" {
		yaml.WriteString("name: ")
		yaml.WriteString(projectName)
		yaml.WriteByte('\n')
	}
	yaml.WriteString("services:\n  ")
	yaml.WriteString(firstNonBlank(serviceName, "service"))
	yaml.WriteString(":\n    image: ")
	yaml.WriteString(firstNonBlank(imageReference, "image:latest"))
	yaml.WriteByte('\n')
	appendStringList(&yaml, "entrypoint", entrypoint, 4)
	appendStringList(&yaml, "command", command, 4)
	appendKeyValues(&yaml, "environment", environment, 4)
	appendStringList(&yaml, "ports", portSpecs(ports), 4)
	appendStringList(&yaml, "volumes", volumeSpecs(volumes), 4)
	appendKeyValues(&yaml, "labels", labels, 4)
	appendString(&yaml, "working_dir", workingDir, 4)
	appendString(&yaml, "user", user, 4)
	if tty {
		appendBool(&yaml, "tty", true, 4)
	}
	if stdinOpen {
		appendBool(&yaml, "stdin_open", true, 4)
	}
	if privileged {
		appendBool(&yaml, "privileged", true, 4)
	}
	if publishAllPorts {
		appendBool(&yaml, "publish_all_ports", true, 4)
	}
	appendString(&yaml, "network_mode", networkMode, 4)
	appendString(&yaml, "restart", restartPolicy, 4)
	appendStringList(&yaml, "cap_add", capAdd, 4)
	appendStringList(&yaml, "cap_drop", capDrop, 4)
	return yaml.String()
}

func portSpecs(ports []PortBindingCommand) []string {
	specs := make([]string, 0, len(ports))
	for _, item := range ports {
		if item.ContainerPort == 0 {
			continue
		}
		var spec strings.Builder
		if strings.TrimSpace(item.HostIP) != "" {
			spec.WriteString(strings.TrimSpace(item.HostIP))
			spec.WriteByte(':')
		}
		if item.HostPort > 0 {
			spec.WriteString(strconv.Itoa(int(item.HostPort)))
			spec.WriteByte(':')
		}
		spec.WriteString(strconv.Itoa(int(item.ContainerPort)))
		if proto := firstNonBlank(item.Protocol, "tcp"); !strings.EqualFold(proto, "tcp") {
			spec.WriteByte('/')
			spec.WriteString(strings.ToLower(proto))
		}
		specs = append(specs, spec.String())
	}
	return specs
}

func volumeSpecs(volumes []VolumeBindingCommand) []string {
	specs := make([]string, 0, len(volumes))
	for _, item := range volumes {
		if strings.TrimSpace(item.Source) == "" || strings.TrimSpace(item.Target) == "" {
			continue
		}
		spec := strings.TrimSpace(item.Source) + ":" + strings.TrimSpace(item.Target)
		if item.ReadOnly {
			spec += ":ro"
		}
		specs = append(specs, spec)
	}
	return specs
}

func appendStringList(yaml *strings.Builder, key string, values []string, indent int) {
	values = trimValues(values)
	if len(values) == 0 {
		return
	}
	yaml.WriteString(strings.Repeat(" ", indent))
	yaml.WriteString(key)
	yaml.WriteString(":\n")
	for _, value := range values {
		yaml.WriteString(strings.Repeat(" ", indent+2))
		yaml.WriteString("- \"")
		yaml.WriteString(escapeYAML(value))
		yaml.WriteString("\"\n")
	}
}

func appendKeyValues(yaml *strings.Builder, key string, values []KeyValueCommand, indent int) {
	if len(values) == 0 {
		return
	}
	yaml.WriteString(strings.Repeat(" ", indent))
	yaml.WriteString(key)
	yaml.WriteString(":\n")
	for _, item := range values {
		if strings.TrimSpace(item.Key) == "" {
			continue
		}
		yaml.WriteString(strings.Repeat(" ", indent+2))
		yaml.WriteString(strings.TrimSpace(item.Key))
		yaml.WriteString(": \"")
		yaml.WriteString(escapeYAML(maskSensitiveValue(item.Key, item.Value)))
		yaml.WriteString("\"\n")
	}
}

func appendString(yaml *strings.Builder, key, value string, indent int) {
	value = strings.TrimSpace(value)
	if value == "" {
		return
	}
	yaml.WriteString(strings.Repeat(" ", indent))
	yaml.WriteString(key)
	yaml.WriteString(": \"")
	yaml.WriteString(escapeYAML(value))
	yaml.WriteString("\"\n")
}

func appendBool(yaml *strings.Builder, key string, value bool, indent int) {
	yaml.WriteString(strings.Repeat(" ", indent))
	yaml.WriteString(key)
	yaml.WriteString(": ")
	yaml.WriteString(strconv.FormatBool(value))
	yaml.WriteByte('\n')
}

func escapeYAML(value string) string {
	return strings.ReplaceAll(strings.ReplaceAll(value, `\`, `\\`), `"`, `\"`)
}

func fallbackLoopbackRealm(realm string) string {
	parsed, err := url.Parse(strings.TrimSpace(realm))
	if err != nil || !strings.EqualFold(parsed.Hostname(), "host.docker.internal") {
		return ""
	}
	host := "127.0.0.1"
	if parsed.Port() != "" {
		host += ":" + parsed.Port()
	}
	parsed.Host = host
	return parsed.String()
}

func truncate(value string, max int) string {
	if max <= 0 || len(value) <= max {
		return value
	}
	return value[:max] + "..."
}

func formatTime(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.Format(time.RFC3339)
}
