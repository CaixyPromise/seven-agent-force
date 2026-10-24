package domain

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"strings"
)

// ConfigAssetBindingSnapshot is persisted only inside the private audit
// payload of sys_config_change_log. It is intentionally not part of any
// facade/VO and ConfigChangeLog keeps its serialized form unexported, so a
// management/history response cannot disclose a fileId, scopeId, reference,
// storage path, or access policy.
//
// The data is a server-derived record of the CONFIG_ASSET slot, not an upload
// capability. Rollback validates it again against the current authenticated
// scope and active reference before it can restore anything.
type ConfigAssetBindingSnapshot struct {
	SchemaVersion int    `json:"schemaVersion"`
	ConfigID      int64  `json:"configId"`
	State         string `json:"state"`
	FileID        int64  `json:"fileId,omitempty"`
	ScopeID       string `json:"scopeId"`
	AssetType     string `json:"assetType"`
	Exposure      string `json:"exposure"`
}

const configAssetSnapshotSchemaVersion = 1

const (
	configAssetSnapshotEmpty = "EMPTY"
	configAssetSnapshotBound = "BOUND"
)

func NewConfigAssetBindingSnapshot(configID int64, state string, fileID int64, scopeID, assetType, exposure string) ConfigAssetBindingSnapshot {
	return ConfigAssetBindingSnapshot{
		SchemaVersion: configAssetSnapshotSchemaVersion,
		ConfigID:      configID,
		State:         strings.ToUpper(strings.TrimSpace(state)),
		FileID:        fileID,
		ScopeID:       strings.TrimSpace(scopeID),
		AssetType:     strings.ToUpper(strings.TrimSpace(assetType)),
		Exposure:      strings.ToUpper(strings.TrimSpace(exposure)),
	}
}

func (s ConfigAssetBindingSnapshot) validate() error {
	if s.SchemaVersion != configAssetSnapshotSchemaVersion || s.ConfigID <= 0 || strings.TrimSpace(s.ScopeID) == "" {
		return fmt.Errorf("invalid configuration asset snapshot identity")
	}
	if s.AssetType != "IMAGE" && s.AssetType != "FILE" {
		return fmt.Errorf("invalid configuration asset snapshot type")
	}
	if s.Exposure != "INTERNAL" && s.Exposure != "AUTHENTICATED" && s.Exposure != "PUBLIC" {
		return fmt.Errorf("invalid configuration asset snapshot exposure")
	}
	switch s.State {
	case configAssetSnapshotEmpty:
		if s.FileID != 0 {
			return fmt.Errorf("empty configuration asset snapshot contains a file")
		}
	case configAssetSnapshotBound:
		if s.FileID <= 0 {
			return fmt.Errorf("bound configuration asset snapshot is missing a file")
		}
	default:
		return fmt.Errorf("invalid configuration asset snapshot state")
	}
	return nil
}

func marshalConfigAssetBindingSnapshot(snapshot *ConfigAssetBindingSnapshot) (string, error) {
	if snapshot == nil {
		return "", nil
	}
	value := NewConfigAssetBindingSnapshot(snapshot.ConfigID, snapshot.State, snapshot.FileID, snapshot.ScopeID, snapshot.AssetType, snapshot.Exposure)
	if err := value.validate(); err != nil {
		return "", err
	}
	payload, err := json.Marshal(value)
	if err != nil {
		return "", fmt.Errorf("marshal configuration asset snapshot: %w", err)
	}
	return string(payload), nil
}

func parseConfigAssetBindingSnapshot(payload string) (*ConfigAssetBindingSnapshot, error) {
	payload = strings.TrimSpace(payload)
	if payload == "" {
		return nil, nil
	}
	decoder := json.NewDecoder(bytes.NewBufferString(payload))
	decoder.DisallowUnknownFields()
	var snapshot ConfigAssetBindingSnapshot
	if err := decoder.Decode(&snapshot); err != nil {
		return nil, fmt.Errorf("decode configuration asset snapshot: %w", err)
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		return nil, fmt.Errorf("configuration asset snapshot has trailing payload")
	}
	if err := snapshot.validate(); err != nil {
		return nil, err
	}
	return &snapshot, nil
}

// SetPrivateAssetSnapshots stores validated server-derived snapshot payloads
// in unexported ConfigChangeLog fields. The function is intentionally not a
// facade concern and cannot make the payload serializable through JSON.
func (c *ConfigChangeLog) SetPrivateAssetSnapshots(oldSnapshot, newSnapshot *ConfigAssetBindingSnapshot) error {
	if c == nil {
		return fmt.Errorf("configuration change log is nil")
	}
	oldPayload, err := marshalConfigAssetBindingSnapshot(oldSnapshot)
	if err != nil {
		return err
	}
	newPayload, err := marshalConfigAssetBindingSnapshot(newSnapshot)
	if err != nil {
		return err
	}
	if (oldPayload == "") != (newPayload == "") {
		return fmt.Errorf("configuration asset audit snapshots must be paired")
	}
	c.oldAssetSnapshotPayload = oldPayload
	c.newAssetSnapshotPayload = newPayload
	return nil
}

// HydratePrivateAssetSnapshotPayloads is used only by the persistence adapter.
// It deliberately does not parse on list/history reads: invalid legacy data
// remains non-disclosable and RollbackConfigChange later rejects it safely.
func (c *ConfigChangeLog) HydratePrivateAssetSnapshotPayloads(oldPayload, newPayload string) {
	if c == nil {
		return
	}
	c.oldAssetSnapshotPayload = strings.TrimSpace(oldPayload)
	c.newAssetSnapshotPayload = strings.TrimSpace(newPayload)
}

// PrivateAssetSnapshotPayloads is for the persistence adapter only. The
// payload is never copied into any facade or management/history response.
func (c ConfigChangeLog) PrivateAssetSnapshotPayloads() (string, string) {
	return c.oldAssetSnapshotPayload, c.newAssetSnapshotPayload
}

// PrivateAssetSnapshots returns the private pair for the rollback use case.
// Missing, partial, malformed, or mismatched legacy payloads are errors so a
// rollback can never guess a file from current state or timestamps.
func (c ConfigChangeLog) PrivateAssetSnapshots() (*ConfigAssetBindingSnapshot, *ConfigAssetBindingSnapshot, error) {
	oldPayload, newPayload := c.PrivateAssetSnapshotPayloads()
	if oldPayload == "" && newPayload == "" {
		return nil, nil, nil
	}
	if oldPayload == "" || newPayload == "" {
		return nil, nil, fmt.Errorf("configuration asset audit snapshots are incomplete")
	}
	oldSnapshot, err := parseConfigAssetBindingSnapshot(oldPayload)
	if err != nil {
		return nil, nil, err
	}
	newSnapshot, err := parseConfigAssetBindingSnapshot(newPayload)
	if err != nil {
		return nil, nil, err
	}
	return oldSnapshot, newSnapshot, nil
}
