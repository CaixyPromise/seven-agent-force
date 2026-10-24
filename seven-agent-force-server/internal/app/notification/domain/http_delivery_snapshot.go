package domain

import "time"

// HTTPDeliverySnapshot is the immutable G5.2 connection revision accepted
// with one outbound delivery. It freezes the bounded configuration and the
// envelope-encrypted secret so later endpoint, mapping or credential changes
// cannot silently alter a pending delivery.
type HTTPDeliverySnapshot struct {
	// ID is the internal primary key.
	ID int64 `db:"id"`
	// DeliveryID identifies the one delivery that owns this snapshot.
	DeliveryID string `db:"deliveryId"`
	// ScopeID is the installation, Hub, or Node boundary of the connection.
	ScopeID string `db:"scopeId"`
	// ChannelCode identifies the operator-selected static connection.
	ChannelCode string `db:"channelCode"`
	// ChannelType is HTTP_CONNECTOR, FEISHU_WEBHOOK, or WECOM_WEBHOOK.
	ChannelType string `db:"channelType"`
	// ChannelPriority is the connection priority visible to a bounded mapping.
	ChannelPriority int `db:"channelPriority"`
	// ConfigJSON is the immutable structured non-secret request configuration.
	ConfigJSON string `db:"configJson"`
	// SecretCiphertext is the envelope-encrypted connection secret revision.
	SecretCiphertext string `db:"secretCiphertext"`
	// SecretEDEK wraps the data-encryption key for SecretCiphertext.
	SecretEDEK string `db:"secretEdek"`
	// SecretWrapKeyRef identifies the wrapping-key revision.
	SecretWrapKeyRef string `db:"secretWrapKeyRef"`
	// CreateTime is the durable acceptance time.
	CreateTime time.Time `db:"createTime"`
	// UpdateTime is retained for standard repository mapping consistency.
	UpdateTime time.Time `db:"updateTime"`
}
