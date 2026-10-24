package docker

import (
	"fmt"
	"path/filepath"
	"strings"

	apperrors "github.com/CaixyPromise/seven-agent/force/internal/common/errors"
	"go.yaml.in/yaml/v3"
)

type composePolicySpec struct {
	Services []composePolicyService `json:"services"`
}

type composePolicyService struct {
	Name        string                 `json:"name"`
	Image       string                 `json:"image,omitempty"`
	Privileged  bool                   `json:"privileged,omitempty"`
	NetworkMode string                 `json:"networkMode,omitempty"`
	PIDMode     string                 `json:"pidMode,omitempty"`
	IPCMode     string                 `json:"ipcMode,omitempty"`
	Networks    []string               `json:"networks,omitempty"`
	Volumes     []VolumeBindingCommand `json:"volumes,omitempty"`
	Environment []KeyValueCommand      `json:"environment,omitempty"`
	Labels      []KeyValueCommand      `json:"labels,omitempty"`
}

func parseComposePolicySpec(raw string) (*composePolicySpec, error) {
	var root map[string]any
	if err := yaml.Unmarshal([]byte(raw), &root); err != nil {
		return nil, apperrors.Params("解析 compose YAML 失败：" + err.Error())
	}
	servicesNode, ok := root["services"].(map[string]any)
	if !ok || len(servicesNode) == 0 {
		return &composePolicySpec{}, nil
	}
	spec := &composePolicySpec{Services: make([]composePolicyService, 0, len(servicesNode))}
	for name, rawService := range servicesNode {
		serviceMap, _ := rawService.(map[string]any)
		service := composePolicyService{Name: strings.TrimSpace(name)}
		service.Image = stringValue(serviceMap["image"])
		service.Privileged = boolFromAny(serviceMap["privileged"])
		service.NetworkMode = stringValue(serviceMap["network_mode"])
		service.PIDMode = stringValue(serviceMap["pid"])
		service.IPCMode = stringValue(serviceMap["ipc"])
		service.Networks = parseComposeNameList(serviceMap["networks"])
		service.Volumes = parseComposeVolumes(serviceMap["volumes"])
		service.Environment = parseComposeKeyValues(serviceMap["environment"])
		service.Labels = parseComposeKeyValues(serviceMap["labels"])
		spec.Services = append(spec.Services, service)
	}
	return spec, nil
}

func parseComposeNameList(raw any) []string {
	switch value := raw.(type) {
	case []any:
		result := make([]string, 0, len(value))
		for _, item := range value {
			if name := strings.TrimSpace(fmt.Sprint(item)); name != "" {
				result = append(result, name)
			}
		}
		return result
	case map[string]any:
		result := make([]string, 0, len(value))
		for name := range value {
			if strings.TrimSpace(name) != "" {
				result = append(result, strings.TrimSpace(name))
			}
		}
		return result
	case string:
		if strings.TrimSpace(value) == "" {
			return nil
		}
		return []string{strings.TrimSpace(value)}
	default:
		return nil
	}
}

func boolFromAny(value any) bool {
	switch typed := value.(type) {
	case bool:
		return typed
	case string:
		switch strings.ToLower(strings.TrimSpace(typed)) {
		case "true", "yes", "on", "1":
			return true
		default:
			return false
		}
	case int:
		return typed != 0
	case int64:
		return typed != 0
	default:
		return false
	}
}

func parseComposeVolumes(raw any) []VolumeBindingCommand {
	items, ok := raw.([]any)
	if !ok {
		return nil
	}
	result := make([]VolumeBindingCommand, 0, len(items))
	for _, item := range items {
		switch value := item.(type) {
		case string:
			result = append(result, parseComposeVolumeString(value))
		case map[string]any:
			binding := VolumeBindingCommand{
				Type:   firstNonBlank(stringValue(value["type"]), "volume"),
				Source: firstNonBlank(stringValue(value["source"]), stringValue(value["src"])),
				Target: firstNonBlank(stringValue(value["target"]), stringValue(value["dst"]), stringValue(value["destination"])),
			}
			if binding.Source != "" || binding.Target != "" {
				result = append(result, binding)
			}
		}
	}
	return result
}

func parseComposeVolumeString(value string) VolumeBindingCommand {
	parts := strings.Split(value, ":")
	binding := VolumeBindingCommand{Type: "volume"}
	if len(parts) >= 1 {
		binding.Source = strings.TrimSpace(parts[0])
	}
	if len(parts) >= 2 {
		binding.Target = strings.TrimSpace(parts[1])
	}
	if isHostPath(binding.Source) {
		binding.Type = "bind"
	}
	return binding
}

func isHostPath(value string) bool {
	value = strings.TrimSpace(value)
	if value == "" {
		return false
	}
	return strings.HasPrefix(value, "/") || strings.HasPrefix(value, "./") || strings.HasPrefix(value, "../") || strings.HasPrefix(value, "~")
}

func parseComposeKeyValues(raw any) []KeyValueCommand {
	switch value := raw.(type) {
	case map[string]any:
		result := make([]KeyValueCommand, 0, len(value))
		for key, item := range value {
			result = append(result, KeyValueCommand{Key: strings.TrimSpace(key), Value: fmt.Sprint(item)})
		}
		return result
	case []any:
		result := make([]KeyValueCommand, 0, len(value))
		for _, item := range value {
			switch typed := item.(type) {
			case string:
				key, val, _ := strings.Cut(typed, "=")
				result = append(result, KeyValueCommand{Key: strings.TrimSpace(key), Value: val})
			case map[string]any:
				for key, val := range typed {
					result = append(result, KeyValueCommand{Key: strings.TrimSpace(key), Value: fmt.Sprint(val)})
				}
			}
		}
		return result
	default:
		return nil
	}
}

func composeServiceNames(spec *composePolicySpec) []string {
	if spec == nil {
		return nil
	}
	result := make([]string, 0, len(spec.Services))
	for _, service := range spec.Services {
		if service.Name != "" {
			result = append(result, service.Name)
		}
	}
	return result
}

func composeNormalizedSpec(spec *composePolicySpec) any {
	if spec == nil {
		return nil
	}
	return spec
}

func cleanHostPath(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	if strings.HasPrefix(value, "~") {
		return value
	}
	return filepath.Clean(value)
}
