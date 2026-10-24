// Package supervisorwire holds strict wire shapes shared by the image-owned
// Supervisor producer and the Worker consumer. It has no compatibility decoder.
package supervisorwire

import "encoding/json"

type JournalEntry struct {
	SchemaVersion int             `json:"schemaVersion"`
	Sequence      int64           `json:"sequence,string"`
	FactID        string          `json:"factId"`
	Type          string          `json:"type"`
	Time          string          `json:"time"`
	PayloadDigest string          `json:"payloadDigest"`
	Payload       json.RawMessage `json:"payload"`
}
