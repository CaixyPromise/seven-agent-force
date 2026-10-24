package domain

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/url"
	"path"
	"sort"
	"strconv"
	"strings"
	"time"
)

const (
	// OutboxEventNotificationIntent requests bounded in-process audience
	// materialization. It is never sent to a third-party channel queue.
	OutboxEventNotificationIntent = "notification.intent"
	// OutboxEventNotificationInboxChanged records a content-free committed
	// mailbox change for the later realtime fan-out adapter.
	OutboxEventNotificationInboxChanged = "notification.inbox.changed"

	// NotificationStatusAccepted means a logical notification has been stored
	// but its deferred audience is not fully materialized yet.
	NotificationStatusAccepted = "ACCEPTED"
	// NotificationStatusMaterialized means every currently snapshotted audience
	// member has been processed.
	NotificationStatusMaterialized = "MATERIALIZED"
	// NotificationStatusScheduled means materialization waits for scheduleAt.
	NotificationStatusScheduled = "SCHEDULED"

	// TaskStatusPending can be claimed by a bounded worker.
	TaskStatusPending = "PENDING"
	// TaskStatusProcessing is protected by a lease token.
	TaskStatusProcessing = "PROCESSING"
	// TaskStatusDone has fully materialized the audience snapshot.
	TaskStatusDone = "DONE"
	// TaskStatusFailed is retained for diagnostics after an unrecoverable error.
	TaskStatusFailed = "FAILED"

	// AudienceKindUsers identifies explicitly selected user recipients.
	AudienceKindUsers = "USERS"
	// AudienceKindRoles identifies role-derived recipients.
	AudienceKindRoles = "ROLES"

	// InboxActionSeen records the first observable presentation of a message.
	InboxActionSeen = "SEEN"
	// InboxActionRead records a user reading a message.
	InboxActionRead = "READ"
	// InboxActionUnread clears the read marker while preserving first-seen data.
	InboxActionUnread = "UNREAD"
	// InboxActionArchive hides a message from the default inbox.
	InboxActionArchive = "ARCHIVE"
	// InboxActionRestore returns an archived message to the default inbox.
	InboxActionRestore = "RESTORE"
	// InboxActionExpire records the one-way end of a recipient's inbox
	// visibility after its configured expiry instant. The audit row is retained.
	InboxActionExpire = "EXPIRE"
)

// AudienceSnapshot is the normalized immutable set of target definitions that
// was accepted with a logical notification. Role membership is intentionally
// resolved later by a bounded materialization task.
type AudienceSnapshot struct {
	// UserIDs are direct recipient user IDs in ascending, duplicate-free order.
	UserIDs []int64 `json:"userIds,omitempty"`
	// RoleIDs are role recipient IDs in ascending, duplicate-free order.
	RoleIDs []int64 `json:"roleIds,omitempty"`
}

// NormalizeAudience validates and canonicalizes a business audience before it
// participates in an idempotency fingerprint or is persisted.
func NormalizeAudience(userIDs, roleIDs []int64) (AudienceSnapshot, error) {
	result, err := NormalizeOptionalAudience(userIDs, roleIDs)
	if err != nil {
		return AudienceSnapshot{}, err
	}
	if len(result.UserIDs) == 0 && len(result.RoleIDs) == 0 {
		return AudienceSnapshot{}, fmt.Errorf("notification audience must include at least one user or role")
	}
	return result, nil
}

// NormalizeOptionalAudience validates and canonicalizes a platform-user
// audience while permitting no local audience for an external-only delivery.
func NormalizeOptionalAudience(userIDs, roleIDs []int64) (AudienceSnapshot, error) {
	normalizedUsers, err := normalizeAudienceIDs(userIDs, "user")
	if err != nil {
		return AudienceSnapshot{}, err
	}
	normalizedRoles, err := normalizeAudienceIDs(roleIDs, "role")
	if err != nil {
		return AudienceSnapshot{}, err
	}
	result := AudienceSnapshot{UserIDs: normalizedUsers, RoleIDs: normalizedRoles}
	if len(result.UserIDs)+len(result.RoleIDs) > 10000 {
		return AudienceSnapshot{}, fmt.Errorf("notification audience exceeds the 10000-member request limit")
	}
	return result, nil
}

// JSON returns the canonical serialized audience snapshot.
func (a AudienceSnapshot) JSON() (string, error) {
	normalized, err := NormalizeOptionalAudience(a.UserIDs, a.RoleIDs)
	if err != nil {
		return "", err
	}
	raw, err := json.Marshal(normalized)
	if err != nil {
		return "", err
	}
	return string(raw), nil
}

// ParseAudienceSnapshot decodes and validates a stored snapshot.
func ParseAudienceSnapshot(raw string) (AudienceSnapshot, error) {
	var snapshot AudienceSnapshot
	if err := json.Unmarshal([]byte(raw), &snapshot); err != nil {
		return AudienceSnapshot{}, fmt.Errorf("decode notification audience snapshot: %w", err)
	}
	return NormalizeOptionalAudience(snapshot.UserIDs, snapshot.RoleIDs)
}

// HasDeferredAudience reports whether role expansion or a scheduled audience
// requires a durable materialization task.
func (a AudienceSnapshot) HasDeferredAudience() bool {
	return len(a.RoleIDs) > 0
}

// LogicalNotification is the immutable business notification. It does not
// represent provider delivery or a user's mutable inbox state.
type LogicalNotification struct {
	// ID is the internal numeric primary key.
	ID int64 `db:"id"`
	// NotificationID is the stable external notification identifier.
	NotificationID string `db:"notificationId"`
	// ScopeID identifies this installation, Hub, or Node boundary.
	ScopeID string `db:"scopeId"`
	// OrganizationID is the authenticated enterprise identity boundary. It is
	// orthogonal to ScopeID, which remains the deployment routing boundary.
	OrganizationID *int64 `db:"organizationId"`
	// EventKey identifies the business notification kind.
	EventKey string `db:"eventKey"`
	// IdempotencyKey is supplied by the business caller for this event.
	IdempotencyKey string `db:"idempotencyKey"`
	// RequestFingerprint is a canonical SHA-256 fingerprint of the request.
	RequestFingerprint string `db:"requestFingerprint"`
	// AudienceJSON is the canonical direct-user and role snapshot.
	AudienceJSON string `db:"audienceJson"`
	// Category classifies the user-facing message.
	Category string `db:"category"`
	// Priority describes business urgency without selecting an external channel.
	Priority string `db:"priority"`
	// Mandatory marks a message that later preference policy may not suppress.
	Mandatory bool `db:"mandatory"`
	// Title is the immutable plain-text title snapshot.
	Title string `db:"title"`
	// Content is the immutable plain-text body snapshot.
	Content string `db:"content"`
	// DeepLink is an allowlisted internal route or empty.
	DeepLink string `db:"deepLink"`
	// Action fields are frozen from the accepted Template Revision. They are
	// copied to inbox recipients and external deliveries so later template or
	// configuration edits cannot change an accepted notification.
	ActionType          string `db:"actionType"`
	ActionLabelSnapshot string `db:"actionLabelSnapshot"`
	ActionStyle         string `db:"actionStyle"`
	// ScheduleAt delays materialization until the specified instant.
	ScheduleAt *time.Time `db:"scheduleAt"`
	// ExpiresAt stops expired recipients from appearing in the inbox.
	ExpiresAt *time.Time `db:"expiresAt"`
	// TraceID links the business event to structured diagnostics.
	TraceID string `db:"traceId"`
	// Status describes accepted, scheduled, or fully materialized state.
	Status string `db:"status"`
	// CreatorID is the business actor when one exists.
	CreatorID *int64 `db:"creatorId"`
	// CreateTime is the durable notification creation time.
	CreateTime time.Time `db:"createTime"`
	// UpdateTime is the latest logical notification state change time.
	UpdateTime time.Time `db:"updateTime"`
}

// Recipient is the user-owned in-app notification projection. Its lifecycle is
// intentionally independent of external provider delivery state.
type Recipient struct {
	// ID is the internal numeric primary key used by cursor pagination.
	ID int64 `db:"id"`
	// RecipientID is the stable external identifier accepted by inbox routes.
	RecipientID string `db:"recipientId"`
	// NotificationID references the internal logical notification row.
	NotificationID int64 `db:"notificationId"`
	// ScopeID identifies the local installation, Hub, or Node mailbox boundary.
	ScopeID string `db:"scopeId"`
	// UserID is the sole inbox owner.
	UserID int64 `db:"userId"`
	// EventKey is copied from the immutable logical notification for list views.
	EventKey string `db:"eventKey"`
	// Category is copied from the immutable logical notification.
	Category string `db:"category"`
	// Priority is copied from the immutable logical notification.
	Priority string `db:"priority"`
	// Mandatory is copied from the immutable logical notification.
	Mandatory bool `db:"mandatory"`
	// Title is the immutable recipient title snapshot.
	Title string `db:"title"`
	// Content is the immutable recipient content snapshot.
	Content string `db:"content"`
	// DeepLink is the immutable safe internal route snapshot.
	DeepLink            string `db:"deepLink"`
	ActionType          string `db:"actionType"`
	ActionLabelSnapshot string `db:"actionLabelSnapshot"`
	ActionStyle         string `db:"actionStyle"`
	// ExpiresAt controls inbox visibility without deleting audit data.
	ExpiresAt *time.Time `db:"expiresAt"`
	// ExpiredAt records when the bounded expiry worker made the recipient
	// invisible and emitted its durable mailbox-change marker.
	ExpiredAt *time.Time `db:"expiredAt"`
	// FirstSeenAt is set only once and never cleared.
	FirstSeenAt *time.Time `db:"firstSeenAt"`
	// ReadAt is nullable so a user may mark a message unread again.
	ReadAt *time.Time `db:"readAt"`
	// ArchivedAt is nullable so a user may restore an archived message.
	ArchivedAt *time.Time `db:"archivedAt"`
	// MailboxVersion advances on every visible state mutation.
	MailboxVersion int64 `db:"mailboxVersion"`
	// CreateTime is the recipient projection creation time.
	CreateTime time.Time `db:"createTime"`
	// UpdateTime is the latest recipient state change time.
	UpdateTime time.Time `db:"updateTime"`
}

// Mailbox serializes visible changes for exactly one current-user inbox. It is
// independent from an individual recipient's record-level CAS version.
type Mailbox struct {
	// ID is the database-generated internal primary key.
	ID int64 `db:"id"`
	// ScopeID identifies the local installation, Hub, or Node boundary.
	ScopeID string `db:"scopeId"`
	// UserID is the sole mailbox owner.
	UserID int64 `db:"userId"`
	// MailboxKey is an opaque, non-authorizing cache namespace for this mailbox.
	MailboxKey string `db:"mailboxKey"`
	// ChangeSequence is the strictly serialized durable mailbox watermark.
	ChangeSequence int64 `db:"changeSequence"`
	// CreateTime is the durable mailbox creation time.
	CreateTime time.Time `db:"createTime"`
	// UpdateTime is the latest durable sequence advance time.
	UpdateTime time.Time `db:"updateTime"`
}

// MaterializationTask expands a large direct or role audience in bounded,
// resumable batches protected by an owner/token/until fence.
type MaterializationTask struct {
	// ID is the internal numeric primary key.
	ID int64 `db:"id"`
	// TaskID is the stable external diagnostic identifier.
	TaskID string `db:"taskId"`
	// NotificationID references the logical notification to materialize.
	NotificationID int64 `db:"notificationId"`
	// ScopeID keeps the task within the installation, Hub, or Node boundary.
	ScopeID string `db:"scopeId"`
	// AudienceJSON is the immutable normalized audience snapshot.
	AudienceJSON string `db:"audienceJson"`
	// Cursor checkpoints direct and role traversal progress.
	Cursor string `db:"materializationCursor"`
	// Status is pending, processing, done, or failed.
	Status string `db:"status"`
	// MaterializedCount counts successful unique recipient insertion attempts.
	MaterializedCount int64 `db:"materializedCount"`
	// RetryCount counts worker failures or reclaimed attempts.
	RetryCount int `db:"retryCount"`
	// NextRunAt is the earliest time a pending task may be claimed.
	NextRunAt time.Time `db:"nextRunAt"`
	// LeaseOwner identifies the worker currently owning the task.
	LeaseOwner string `db:"leaseOwner"`
	// LeaseToken fences stale workers from updating a reclaimed task.
	LeaseToken string `db:"leaseToken"`
	// LeaseUntil is the time after which another worker may reclaim the task.
	LeaseUntil *time.Time `db:"leaseUntil"`
	// LastError contains the last bounded-worker failure for diagnostics.
	LastError string `db:"lastError"`
	// CreateTime is the durable task creation time.
	CreateTime time.Time `db:"createTime"`
	// UpdateTime is the latest task state change time.
	UpdateTime time.Time `db:"updateTime"`
}

// IntentMessage is the minimal, secret-free shared Outbox payload that wakes
// internal audience materialization after its surrounding transaction commits.
type IntentMessage struct {
	NotificationID int64 `json:"notificationId"`
	// ScopeID makes the durable materialization wakeup self-describing. Older
	// payloads may omit it and are resolved from the logical notification.
	ScopeID string `json:"scopeId"`
}

// InboxCursor identifies a stable page boundary. It is kept in the domain so
// repository queries do not depend on HTTP request types.
type InboxCursor struct {
	// CreateTime is the timestamp component of a descending page boundary.
	CreateTime time.Time
	// ID is the tie-breaker component of a descending page boundary.
	ID int64
}

// InboxQuery describes an owner-scoped recipient query.
type InboxQuery struct {
	// ScopeID is the trusted mailbox scope from module configuration.
	ScopeID string
	// UserID is the authenticated inbox owner.
	UserID int64
	// Archived selects archived rather than default non-archived entries.
	Archived bool
	// Cursor is the optional stable page boundary.
	Cursor *InboxCursor
	// Limit bounds database work and response size.
	Limit int
}

// InboxChangeQuery selects compact recipient snapshots that changed within one
// already-authorized mailbox sequence range. Archive filtering is intentionally
// absent so an open default view can remove an item that was archived elsewhere.
type InboxChangeQuery struct {
	// ScopeID is the trusted mailbox scope from module configuration.
	ScopeID string
	// UserID is the authenticated inbox owner.
	UserID int64
	// AfterSequence is exclusive.
	AfterSequence int64
	// UntilSequence is inclusive and fixed for one paged delta traversal.
	UntilSequence int64
	// Limit bounds database work and response size.
	Limit int
}

// InboxChangedIntent is the secret-free durable handoff from the inbox write
// transaction to later realtime fan-out. It must never contain title, body,
// deep-link, recipient ID, provider target, or other message content.
type InboxChangedIntent struct {
	ScopeID        string `json:"scopeId"`
	UserID         int64  `json:"userId"`
	ChangeSequence int64  `json:"changeSequence"`
	NewUnread      bool   `json:"newUnread"`
}

// ApplyInboxAction changes only recipient-owned inbox fields. It never alters
// delivery state, immutable notification content, or the mailbox sequence. The
// application assigns the next serialized version only after this method proves
// that there is a visible state change.
func (r *Recipient) ApplyInboxAction(action string, now time.Time) (bool, error) {
	if r == nil {
		return false, fmt.Errorf("notification recipient is nil")
	}
	action = strings.ToUpper(strings.TrimSpace(action))
	changed := false
	switch action {
	case InboxActionSeen:
		if r.FirstSeenAt == nil {
			value := now.UTC()
			r.FirstSeenAt = &value
			changed = true
		}
	case InboxActionRead:
		if r.FirstSeenAt == nil {
			value := now.UTC()
			r.FirstSeenAt = &value
			changed = true
		}
		if r.ReadAt == nil {
			value := now.UTC()
			r.ReadAt = &value
			changed = true
		}
	case InboxActionUnread:
		if r.ReadAt != nil {
			r.ReadAt = nil
			changed = true
		}
	case InboxActionArchive:
		if r.ArchivedAt == nil {
			value := now.UTC()
			r.ArchivedAt = &value
			changed = true
		}
	case InboxActionRestore:
		if r.ArchivedAt != nil {
			r.ArchivedAt = nil
			changed = true
		}
	case InboxActionExpire:
		current := now.UTC()
		if r.ExpiresAt != nil && !r.ExpiresAt.After(current) && r.ExpiredAt == nil {
			r.ExpiredAt = &current
			changed = true
		}
	default:
		return false, fmt.Errorf("unsupported inbox action %q", action)
	}
	if changed {
		r.UpdateTime = now.UTC()
	}
	return changed, nil
}

// SetMailboxVersion records the database-serialized sequence allocated for a
// visible recipient mutation in the surrounding transaction.
func (r *Recipient) SetMailboxVersion(nextMailboxVersion int64) error {
	if r == nil {
		return fmt.Errorf("notification recipient is nil")
	}
	if nextMailboxVersion <= 0 {
		return fmt.Errorf("mailbox version must be positive")
	}
	r.MailboxVersion = nextMailboxVersion
	return nil
}

// ValidateLogicalNotification checks the safety and semantic minimum for an
// in-app notification before it is written to a transaction.
func ValidateLogicalNotification(item LogicalNotification) error {
	if strings.TrimSpace(item.ScopeID) == "" {
		return fmt.Errorf("notification scope is required")
	}
	if strings.TrimSpace(item.EventKey) == "" {
		return fmt.Errorf("notification event key is required")
	}
	if strings.TrimSpace(item.IdempotencyKey) == "" {
		return fmt.Errorf("notification idempotency key is required")
	}
	if strings.TrimSpace(item.Title) == "" {
		return fmt.Errorf("notification title is required")
	}
	if strings.TrimSpace(item.Content) == "" {
		return fmt.Errorf("notification content is required")
	}
	if !IsSafeInternalDeepLink(item.DeepLink) {
		return fmt.Errorf("notification deep link must be an allowlisted internal route")
	}
	if item.ScheduleAt != nil && item.ExpiresAt != nil && !item.ExpiresAt.After(*item.ScheduleAt) {
		return fmt.Errorf("notification expiry must be after schedule time")
	}
	for _, field := range []struct {
		name  string
		value string
		limit int
	}{
		{name: "notification scope", value: item.ScopeID, limit: 128},
		{name: "notification event key", value: item.EventKey, limit: 128},
		{name: "notification idempotency key", value: item.IdempotencyKey, limit: 191},
		{name: "notification category", value: item.Category, limit: 64},
		{name: "notification priority", value: item.Priority, limit: 32},
		{name: "notification title", value: item.Title, limit: 512},
		{name: "notification deep link", value: item.DeepLink, limit: 512},
		{name: "notification trace id", value: item.TraceID, limit: 128},
	} {
		if len(field.value) > field.limit {
			return fmt.Errorf("%s exceeds %d bytes", field.name, field.limit)
		}
	}
	if len(item.Content) > 65535 {
		return fmt.Errorf("notification content exceeds 65535 bytes")
	}
	return nil
}

// IsSafeInternalDeepLink permits an empty deep link or a small allowlist of
// application-owned, canonical route prefixes. It rejects schemes, hosts,
// protocol-relative URLs, control characters, escaped paths, and noncanonical
// dot-segment paths so a browser or router cannot normalize a safe-looking
// prefix into a different route.
func IsSafeInternalDeepLink(value string) bool {
	value = strings.TrimSpace(value)
	if value == "" {
		return true
	}
	if strings.ContainsAny(value, "\\\r\n\x00") || strings.HasPrefix(value, "//") {
		return false
	}
	parsed, err := url.Parse(value)
	if err != nil || parsed.IsAbs() || parsed.Host != "" || !strings.HasPrefix(parsed.Path, "/") {
		return false
	}
	// Route authorization is evaluated on a canonical path. Escaped path bytes
	// are intentionally disallowed: different browser/router decoding stages
	// could otherwise turn %2e or %2f into a dot segment or path separator after
	// this prefix check. Query and fragment data remain separate from routing.
	if strings.Contains(parsed.EscapedPath(), "%") || path.Clean(parsed.Path) != parsed.Path {
		return false
	}
	for _, prefix := range []string{"/account", "/notifications", "/system", "/user"} {
		if parsed.Path == prefix || strings.HasPrefix(parsed.Path, prefix+"/") {
			return true
		}
	}
	return parsed.Path == "/" || isSafeWorkforceTaskDeepLinkPath(parsed.Path)
}

func isSafeWorkforceTaskDeepLinkPath(value string) bool {
	const prefix = "/workforce/tasks/"
	if !strings.HasPrefix(value, prefix) {
		return false
	}
	rawID := strings.TrimPrefix(value, prefix)
	if rawID == "" || strings.Contains(rawID, "/") {
		return false
	}
	taskID, err := strconv.ParseInt(rawID, 10, 64)
	return err == nil && taskID > 0 && strconv.FormatInt(taskID, 10) == rawID
}

// CanonicalFingerprint returns the stable request identity used alongside the
// database uniqueness key to distinguish safe retries from key reuse bugs.
func CanonicalFingerprint(item LogicalNotification, audience AudienceSnapshot) string {
	return CanonicalFingerprintWithExternal(item, audience, nil)
}

// CanonicalFingerprintWithExternal includes accepted external-target
// fingerprints without serializing their plaintext provider subjects.
func CanonicalFingerprintWithExternal(item LogicalNotification, audience AudienceSnapshot, external []ExternalRecipientFingerprint) string {
	type fingerprintInput struct {
		ScopeID        string                         `json:"scopeId"`
		OrganizationID int64                          `json:"organizationId,string,omitempty"`
		EventKey       string                         `json:"eventKey"`
		IdempotencyKey string                         `json:"idempotencyKey"`
		Audience       AudienceSnapshot               `json:"audience"`
		Category       string                         `json:"category"`
		Priority       string                         `json:"priority"`
		Mandatory      bool                           `json:"mandatory"`
		Title          string                         `json:"title"`
		Content        string                         `json:"content"`
		DeepLink       string                         `json:"deepLink"`
		ScheduleAt     string                         `json:"scheduleAt"`
		ExpiresAt      string                         `json:"expiresAt"`
		External       []ExternalRecipientFingerprint `json:"externalRecipients,omitempty"`
	}
	canonical := fingerprintInput{
		ScopeID: item.ScopeID, EventKey: item.EventKey, IdempotencyKey: item.IdempotencyKey,
		Audience: audience, Category: item.Category, Priority: item.Priority, Mandatory: item.Mandatory,
		Title: item.Title, Content: item.Content, DeepLink: item.DeepLink,
		External: append([]ExternalRecipientFingerprint(nil), external...),
	}
	if item.OrganizationID != nil {
		canonical.OrganizationID = *item.OrganizationID
	}
	sort.Slice(canonical.External, func(i, j int) bool {
		left, right := canonical.External[i], canonical.External[j]
		if left.ConnectionRef != right.ConnectionRef {
			return left.ConnectionRef < right.ConnectionRef
		}
		if left.IdentityKind != right.IdentityKind {
			return left.IdentityKind < right.IdentityKind
		}
		if left.SubjectDigest != right.SubjectDigest {
			return left.SubjectDigest < right.SubjectDigest
		}
		return left.ProviderParamsJSON < right.ProviderParamsJSON
	})
	if item.ScheduleAt != nil {
		canonical.ScheduleAt = item.ScheduleAt.UTC().Format(time.RFC3339Nano)
	}
	if item.ExpiresAt != nil {
		canonical.ExpiresAt = item.ExpiresAt.UTC().Format(time.RFC3339Nano)
	}
	raw, _ := json.Marshal(canonical)
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:])
}

// CanonicalFingerprintWithStaticRoutes adds fixed administrator-owned
// connections to the semantic idempotency identity.
func CanonicalFingerprintWithStaticRoutes(item LogicalNotification, audience AudienceSnapshot, external []ExternalRecipientFingerprint, routes []StaticRouteFingerprint) string {
	if len(routes) == 0 {
		return CanonicalFingerprintWithExternal(item, audience, external)
	}
	type fingerprintInput struct {
		ScopeID        string                         `json:"scopeId"`
		OrganizationID int64                          `json:"organizationId,string,omitempty"`
		EventKey       string                         `json:"eventKey"`
		IdempotencyKey string                         `json:"idempotencyKey"`
		Audience       AudienceSnapshot               `json:"audience"`
		Category       string                         `json:"category"`
		Priority       string                         `json:"priority"`
		Mandatory      bool                           `json:"mandatory"`
		Title          string                         `json:"title"`
		Content        string                         `json:"content"`
		DeepLink       string                         `json:"deepLink"`
		ScheduleAt     string                         `json:"scheduleAt"`
		ExpiresAt      string                         `json:"expiresAt"`
		External       []ExternalRecipientFingerprint `json:"externalRecipients,omitempty"`
		StaticRoutes   []StaticRouteFingerprint       `json:"staticRoutes"`
	}
	canonical := fingerprintInput{
		ScopeID: item.ScopeID, EventKey: item.EventKey, IdempotencyKey: item.IdempotencyKey,
		Audience: audience, Category: item.Category, Priority: item.Priority, Mandatory: item.Mandatory,
		Title: item.Title, Content: item.Content, DeepLink: item.DeepLink,
		External: append([]ExternalRecipientFingerprint(nil), external...), StaticRoutes: append([]StaticRouteFingerprint(nil), routes...),
	}
	if item.OrganizationID != nil {
		canonical.OrganizationID = *item.OrganizationID
	}
	sort.Slice(canonical.External, func(i, j int) bool {
		left, right := canonical.External[i], canonical.External[j]
		if left.ConnectionRef != right.ConnectionRef {
			return left.ConnectionRef < right.ConnectionRef
		}
		if left.IdentityKind != right.IdentityKind {
			return left.IdentityKind < right.IdentityKind
		}
		if left.SubjectDigest != right.SubjectDigest {
			return left.SubjectDigest < right.SubjectDigest
		}
		return left.ProviderParamsJSON < right.ProviderParamsJSON
	})
	sort.Slice(canonical.StaticRoutes, func(i, j int) bool {
		if canonical.StaticRoutes[i].ConnectionRef != canonical.StaticRoutes[j].ConnectionRef {
			return canonical.StaticRoutes[i].ConnectionRef < canonical.StaticRoutes[j].ConnectionRef
		}
		return canonical.StaticRoutes[i].ProviderCode < canonical.StaticRoutes[j].ProviderCode
	})
	if item.ScheduleAt != nil {
		canonical.ScheduleAt = item.ScheduleAt.UTC().Format(time.RFC3339Nano)
	}
	if item.ExpiresAt != nil {
		canonical.ExpiresAt = item.ExpiresAt.UTC().Format(time.RFC3339Nano)
	}
	raw, _ := json.Marshal(canonical)
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:])
}

func normalizeAudienceIDs(values []int64, kind string) ([]int64, error) {
	set := make(map[int64]struct{}, len(values))
	for _, value := range values {
		if value <= 0 {
			return nil, fmt.Errorf("notification %s id must be positive", kind)
		}
		set[value] = struct{}{}
	}
	result := make([]int64, 0, len(set))
	for value := range set {
		result = append(result, value)
	}
	sort.Slice(result, func(i, j int) bool { return result[i] < result[j] })
	return result, nil
}
