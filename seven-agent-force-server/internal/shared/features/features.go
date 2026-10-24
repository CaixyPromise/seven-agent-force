// Package features resolves optional runtime capabilities from deployment configuration.
package features

import (
	"sort"

	"github.com/CaixyPromise/seven-agent/force/internal/shared/config"
)

// Code identifies an optional runtime capability.
type Code string

const (
	PlatformControl Code = "platform.control"
	FederationHub   Code = "federation.hub"
	FederationNode  Code = "federation.node"
	DockerAdmin     Code = "docker.admin"
)

// Known reports whether code is one of the optional capabilities supported by this release.
func Known(code Code) bool {
	switch code {
	case PlatformControl, FederationHub, FederationNode, DockerAdmin:
		return true
	default:
		return false
	}
}

// Set is the immutable-at-runtime set of enabled optional capabilities.
type Set map[Code]struct{}

// Resolve derives the only valid capability combination from existing configuration.
func Resolve(cfg config.Config) Set {
	resolved := Set{}
	switch cfg.Platform.Mode {
	case config.PlatformModeHub:
		resolved[PlatformControl] = struct{}{}
		resolved[FederationHub] = struct{}{}
	case config.PlatformModeNode:
		resolved[FederationNode] = struct{}{}
	}
	if cfg.Docker.Enabled {
		resolved[DockerAdmin] = struct{}{}
	}
	return resolved
}

// Enabled reports whether a capability is enabled.
func (s Set) Enabled(code Code) bool {
	_, ok := s[code]
	return ok
}

// Without returns a copy of the set without code. Runtime availability checks
// use this to derive the effective capabilities without mutating configuration.
func (s Set) Without(code Code) Set {
	result := make(Set, len(s))
	for current := range s {
		if current != code {
			result[current] = struct{}{}
		}
	}
	return result
}

// EnabledCodes returns stable, sorted capability codes for API responses and logs.
func (s Set) EnabledCodes() []string {
	codes := make([]string, 0, len(s))
	for code := range s {
		codes = append(codes, string(code))
	}
	sort.Strings(codes)
	return codes
}

// OrResolve keeps direct module tests concise while production bootstrap passes one shared set.
func OrResolve(current Set, cfg config.Config) Set {
	if current != nil {
		return current
	}
	return Resolve(cfg)
}
