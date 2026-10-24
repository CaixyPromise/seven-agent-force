package domain

import (
	"strings"
	"time"
)

const (
	ProtocolOpenAI    = "OPENAI"
	ProtocolAnthropic = "ANTHROPIC"
	StatusActive      = "ACTIVE"
	StatusDisabled    = "DISABLED"
	StatusDeleted     = "DELETED"
)

// Credential is one task-selectable model credential. Models is a catalog for
// task creation only; it is deliberately not an invocation allow-list.
type Credential struct {
	ID                      int64
	TenantID                int64
	Name                    string
	Protocol                string
	BaseURL                 string
	Models                  []string
	SecretCiphertext        string
	SecretEDEK              string
	SecretWrapKeyRef        string
	SecretHint              string
	SecretVersion           int64
	Status                  string
	Version                 int64
	LastDiscoveryStatus     string
	LastDiscoveryReasonCode string
	LastDiscoveredAt        *time.Time
	CreatedAt               time.Time
	UpdatedAt               time.Time
	InvocationCount         int64
	InputTokens             int64
	CachedInputTokens       int64
	OutputTokens            int64
	TotalTokens             int64
}

type View struct {
	ID                      int64    `json:"id,string"`
	Name                    string   `json:"name"`
	Protocol                string   `json:"protocol"`
	BaseURL                 string   `json:"baseUrl"`
	Models                  []string `json:"models"`
	CredentialConfigured    bool     `json:"credentialConfigured"`
	SecretHint              string   `json:"secretHint,omitempty"`
	Status                  string   `json:"status"`
	Version                 int64    `json:"version,string"`
	LastDiscoveryStatus     string   `json:"lastDiscoveryStatus,omitempty"`
	LastDiscoveryReasonCode string   `json:"lastDiscoveryReasonCode,omitempty"`
	LastDiscoveredAt        string   `json:"lastDiscoveredAt,omitempty"`
	CreatedAt               string   `json:"createdAt,omitempty"`
	UpdatedAt               string   `json:"updatedAt,omitempty"`
	InvocationCount         int64    `json:"invocationCount"`
	InputTokens             int64    `json:"inputTokens"`
	CachedInputTokens       int64    `json:"cachedInputTokens"`
	OutputTokens            int64    `json:"outputTokens"`
	TotalTokens             int64    `json:"totalTokens"`
}

func (c Credential) View() View {
	view := View{
		ID: c.ID, Name: c.Name, Protocol: c.Protocol, BaseURL: c.BaseURL,
		Models:               append([]string(nil), c.Models...),
		CredentialConfigured: c.SecretCiphertext != "" && c.SecretEDEK != "",
		SecretHint:           c.SecretHint, Status: c.Status, Version: c.Version,
		LastDiscoveryStatus:     c.LastDiscoveryStatus,
		LastDiscoveryReasonCode: c.LastDiscoveryReasonCode,
		InvocationCount:         c.InvocationCount,
		InputTokens:             c.InputTokens,
		CachedInputTokens:       c.CachedInputTokens,
		OutputTokens:            c.OutputTokens,
		TotalTokens:             c.TotalTokens,
	}
	if c.LastDiscoveredAt != nil {
		view.LastDiscoveredAt = c.LastDiscoveredAt.UTC().Format(time.RFC3339Nano)
	}
	if !c.CreatedAt.IsZero() {
		view.CreatedAt = c.CreatedAt.UTC().Format(time.RFC3339Nano)
	}
	if !c.UpdatedAt.IsZero() {
		view.UpdatedAt = c.UpdatedAt.UTC().Format(time.RFC3339Nano)
	}
	return view
}

func (c Credential) HasModel(model string) bool {
	model = strings.TrimSpace(model)
	for _, item := range c.Models {
		if item == model {
			return true
		}
	}
	return false
}

type ModelOption struct {
	CredentialID   int64  `json:"credentialId,string"`
	CredentialName string `json:"credentialName"`
	Protocol       string `json:"protocol"`
	Model          string `json:"model"`
	Label          string `json:"label"`
}

type DeleteResult struct {
	Deleted         bool `json:"deleted"`
	RetainedHistory bool `json:"retainedHistory"`
}

// RuntimeSelection is frozen into an Assignment. It identifies the credential
// and primary model but never constrains models requested by the Coding Agent.
type RuntimeSelection struct {
	CredentialID      int64  `json:"modelCredentialId,string"`
	CredentialVersion int64  `json:"credentialVersion,string"`
	Protocol          string `json:"modelProtocol"`
	PrimaryModel      string `json:"primaryModel"`
}

type InvocationCredential struct {
	CredentialID      int64
	CredentialVersion int64
	Protocol          string
	BaseURL           string
	PrimaryModel      string
	APIKey            string
}

type RuntimeCredential struct {
	CredentialID      int64
	CredentialVersion int64
	Name              string
	Protocol          string
	BaseURL           string
	PrimaryModel      string
	APIKey            string
}
