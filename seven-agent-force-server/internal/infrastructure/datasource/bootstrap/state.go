package bootstrap

type SchemaState string

const (
	SchemaStateEmpty           SchemaState = "EMPTY_SCHEMA"
	SchemaStateManaged         SchemaState = "MANAGED_SCHEMA"
	SchemaStateLegacyUnmanaged SchemaState = "LEGACY_UNMANAGED_SCHEMA"
)

type Inspection struct {
	Driver             string      `json:"driver"`
	State              SchemaState `json:"state"`
	VersionTable       string      `json:"versionTable"`
	VersionTableExists bool        `json:"versionTableExists"`
	BusinessTableCount int         `json:"businessTableCount"`
	CurrentVersion     int64       `json:"currentVersion"`
	RecommendedAction  string      `json:"recommendedAction"`
}

type Result struct {
	Inspection      Inspection `json:"inspection"`
	BaselineApplied bool       `json:"baselineApplied"`
	SyncApplied     bool       `json:"syncApplied"`
	UpdateApplied   bool       `json:"updateApplied"`
	FinalVersion    int64      `json:"finalVersion"`
}
