// Package cachepolicy contains the reviewed DG5 cache classification contract.
// It deliberately does not know repositories, Redis, RabbitMQ, or cached
// values; callers must satisfy this contract before infrastructure may cache.
package cachepolicy

import (
	"crypto/sha256"
	"encoding/hex"
	"sort"
	"strconv"
	"strings"
	"time"
)

const (
	// StorageScopeSystemGlobal is the explicit scope of the system-owned
	// configuration and dictionary source of truth. It is intentionally neither
	// local nor empty, because DG5 must never use legacy NULL/local claiming.
	StorageScopeSystemGlobal = "system:global"

	SchemaVersionV1 = 1
	// SchemaVersionV2 is intentionally a separate targeted-invalidation
	// protocol. It must never be interpreted as a V1 class-wide generation.
	SchemaVersionV2 = 2
	// SchemaVersionV3 is an independent, content-free global application-cache
	// refresh protocol. It is deliberately not a V1 class generation or a V2
	// targeted-session generation.
	SchemaVersionV3 = 3
)

type DataClass string

const (
	DataClassConfigPublicScalar DataClass = "config.public.scalar.v1"
	DataClassDictPublicItems    DataClass = "dict.public.items.v1"
	// DataClassAuthorizationContext and DataClassAuthorizationMenus are private
	// per-user authorization result snapshots. They contain no session/token
	// authority and are invalidated class-wide because role/menu mutations must
	// not enumerate every affected user to be correct.
	DataClassAuthorizationContext DataClass = "authorization.context.v1"
	DataClassAuthorizationMenus   DataClass = "authorization.menu.v1"
	// DataClassActiveSessionValidity is a minimal active-session projection.
	// It is target-addressed, never a cached domain.Session or token record.
	DataClassActiveSessionValidity DataClass = "sso.active-session-validity.v2"
)

// CatalogEntry is the complete review record required before a DG5 read can
// use L1/L2. Durations are frozen against the DG5 measured acceptance fixture;
// MaxStale=0 means a generation check is required before every L1 candidate
// can be used.
type CatalogEntry struct {
	Owner                string
	DataClass            DataClass
	StorageScope         string
	KeyDimensions        []string
	SchemaVersion        int
	Exposure             string
	Sensitivity          string
	L1TTL                time.Duration
	L2TTL                time.Duration
	MaxStale             time.Duration
	AuthoritySource      string
	InvalidationTriggers []string
}

var catalog = map[DataClass]CatalogEntry{
	DataClassConfigPublicScalar: {
		Owner:           "system-config",
		DataClass:       DataClassConfigPublicScalar,
		StorageScope:    StorageScopeSystemGlobal,
		KeyDimensions:   []string{"storage-scope", "exposure", "business-identity", "request-scope", "target", "schema-version", "generation"},
		SchemaVersion:   SchemaVersionV1,
		Exposure:        "PUBLIC",
		Sensitivity:     "NORMAL",
		L1TTL:           30 * time.Second,
		L2TTL:           5 * time.Minute,
		MaxStale:        0,
		AuthoritySource: "sys_config + sys_config_group",
		InvalidationTriggers: []string{
			"group-create", "group-update", "group-enable", "group-delete", "group-move", "config-create", "config-update", "config-enable", "config-delete", "config-apply-pending", "config-rollback", "scope-policy-change",
		},
	},
	DataClassDictPublicItems: {
		Owner:           "system-dict",
		DataClass:       DataClassDictPublicItems,
		StorageScope:    StorageScopeSystemGlobal,
		KeyDimensions:   []string{"storage-scope", "exposure", "business-identity", "request-scope", "target", "schema-version", "generation"},
		SchemaVersion:   SchemaVersionV1,
		Exposure:        "PUBLIC",
		Sensitivity:     "NORMAL",
		L1TTL:           30 * time.Second,
		L2TTL:           5 * time.Minute,
		MaxStale:        0,
		AuthoritySource: "sys_dict_type + sys_dict_item",
		InvalidationTriggers: []string{
			"type-create", "type-update", "type-status", "type-delete", "type-move", "item-create", "item-update", "item-status", "item-delete", "item-move", "item-sort",
		},
	},
	DataClassAuthorizationContext: {
		Owner:           "authorization",
		DataClass:       DataClassAuthorizationContext,
		StorageScope:    StorageScopeSystemGlobal,
		KeyDimensions:   []string{"storage-scope", "private-exposure", "user-identity", "feature-fingerprint", "schema-version", "generation"},
		SchemaVersion:   SchemaVersionV1,
		Exposure:        "PRIVATE",
		Sensitivity:     "RESTRICTED",
		L1TTL:           30 * time.Second,
		L2TTL:           5 * time.Minute,
		MaxStale:        0,
		AuthoritySource: "authorization user aggregate + role/permission/org/dept/post relations",
		InvalidationTriggers: []string{
			"role", "role-menu", "role-permission", "role-data-scope", "menu", "permission", "temporary-grant", "user-status", "user-lock", "user-role", "user-org", "user-dept", "user-post", "org-hierarchy", "dept-hierarchy", "post-role",
		},
	},
	DataClassAuthorizationMenus: {
		Owner:           "authorization",
		DataClass:       DataClassAuthorizationMenus,
		StorageScope:    StorageScopeSystemGlobal,
		KeyDimensions:   []string{"storage-scope", "private-exposure", "user-identity", "feature-fingerprint", "schema-version", "generation"},
		SchemaVersion:   SchemaVersionV1,
		Exposure:        "PRIVATE",
		Sensitivity:     "RESTRICTED",
		L1TTL:           30 * time.Second,
		L2TTL:           5 * time.Minute,
		MaxStale:        0,
		AuthoritySource: "authorization menu projection + role/menu/permission relations",
		InvalidationTriggers: []string{
			"role", "role-menu", "menu", "permission", "temporary-grant", "user-status", "user-lock", "user-role", "user-org", "user-dept", "user-post", "org-hierarchy", "dept-hierarchy", "post-role",
		},
	},
	DataClassActiveSessionValidity: {
		Owner:         "sso",
		DataClass:     DataClassActiveSessionValidity,
		StorageScope:  StorageScopeSystemGlobal,
		KeyDimensions: []string{"storage-scope", "private-exposure", "target-kind", "target-digest", "schema-version", "generation"},
		SchemaVersion: SchemaVersionV2,
		Exposure:      "PRIVATE",
		Sensitivity:   "RESTRICTED",
		// The isolated DG6.2 fixture measures only a short validity lookup.
		// Values are capped by ExpiresAt and are not a production-capacity claim.
		L1TTL:           10 * time.Second,
		L2TTL:           30 * time.Second,
		MaxStale:        0,
		AuthoritySource: "sso_session active validity projection",
		InvalidationTriggers: []string{
			"session-logout", "refresh-reuse", "user-revoke", "client-disable", "platform-revoke", "login-method-revoke", "external-provider-revoke", "external-identity-revoke",
		},
	},
}

var configAllowlist = map[string]DataClass{
	"SEVEN_FRONTEND_METADATA.title":             DataClassConfigPublicScalar,
	"SEVEN_FRONTEND_METADATA.shortTitle":        DataClassConfigPublicScalar,
	"SEVEN_FRONTEND_METADATA.themePrimaryColor": DataClassConfigPublicScalar,
	"SEVEN_FRONTEND_METADATA.loginLogo":         DataClassConfigPublicScalar,
	"SEVEN_FRONTEND_METADATA.favicon":           DataClassConfigPublicScalar,
}

var dictAllowlist = map[string]DataClass{
	"gender": DataClassDictPublicItems,
}

// ReadRequest supplies only in-process key material. KeyMaterial() always
// returns an opaque digest; callers must never log the individual dimensions.
type ReadRequest struct {
	Entry            CatalogEntry
	RequestScope     string
	BusinessIdentity string
	Target           string
	SchemaVersion    int
	MaxStale         time.Duration
}

// TargetedReadRequest is the DG6.2-only request surface. TargetDigest is
// already irreversible when it enters this struct, so neither the cache
// layer nor an adapter need retain a raw session ID.
type TargetedReadRequest struct {
	Entry         CatalogEntry
	TargetKind    string
	TargetDigest  string
	SchemaVersion int
	MaxStale      time.Duration
}

// CacheableValue lets a cache-aside loader return a correct business result
// while declining to persist it when current row classification differs from
// the reviewed catalog. It prevents a validation failure from becoming a
// negative cache entry or a silent broader cache admission.
type CacheableValue struct {
	Value     any
	Cacheable bool
}

// TargetedCacheableValue is used only by the active-session projection. A
// non-positive or elapsed ExpiresAt is returned to the caller only when the
// authority deliberately supplied it; it is never admitted to L1/L2.
type TargetedCacheableValue struct {
	Value     any
	Cacheable bool
	ExpiresAt time.Time
}

// ActiveSessionValiditySnapshot is the sole DG6.2 cached value contract. It
// deliberately lives beside the cache protocol rather than SSO domain types,
// so infrastructure can serialize an opaque projection without importing a
// domain model or deciding session eligibility. The application supplies
// Active/Cacheable after its authoritative session rules run.
type ActiveSessionValiditySnapshot struct {
	UserID     int64
	ClientID   string
	ACR        string
	AMR        []string
	CreateTime time.Time
	ExpiresAt  time.Time
	Active     bool
}

// Catalog returns defensive copies in deterministic data-class order so docs,
// diagnostics, and tests can report the reviewed classification without
// mutable global state.
func Catalog() []CatalogEntry {
	classes := make([]string, 0, len(catalog))
	for class := range catalog {
		classes = append(classes, string(class))
	}
	sort.Strings(classes)
	result := make([]CatalogEntry, 0, len(classes))
	for _, class := range classes {
		entry := cloneEntry(catalog[DataClass(class)])
		result = append(result, entry)
	}
	return result
}

// Entry resolves one exact catalog data class without returning mutable slices.
func Entry(class DataClass) (CatalogEntry, bool) {
	entry, ok := catalog[class]
	if !ok {
		return CatalogEntry{}, false
	}
	return cloneEntry(entry), true
}

// ConfigReadRequest returns a request only for one explicitly reviewed public
// configuration key. Unknown, draft, management, internal, and sensitive
// keys must take their normal authoritative path.
func ConfigReadRequest(fullyQualifiedKey, requestScope, businessIdentity string) (ReadRequest, bool) {
	class, ok := configAllowlist[strings.TrimSpace(fullyQualifiedKey)]
	if !ok {
		return ReadRequest{}, false
	}
	return buildReadRequest(class, requestScope, businessIdentity, fullyQualifiedKey)
}

// DictReadRequest returns a request only for one explicitly reviewed public
// dictionary identity. Canonicalisation is limited to code case/space; it does
// not turn an unreviewed alias into an eligible cache namespace.
func DictReadRequest(dictCode, requestScope, businessIdentity string) (ReadRequest, bool) {
	normalized := strings.ToLower(strings.TrimSpace(dictCode))
	class, ok := dictAllowlist[normalized]
	if !ok {
		return ReadRequest{}, false
	}
	return buildReadRequest(class, requestScope, businessIdentity, normalized)
}

// AuthorizationContextReadRequest creates a private per-user snapshot key.
// featureFingerprint must be an opaque, stable deployment/configuration digest;
// it prevents a startup feature-flag change from reusing an older snapshot.
func AuthorizationContextReadRequest(userID int64, featureFingerprint string) (ReadRequest, bool) {
	return authorizationReadRequest(DataClassAuthorizationContext, userID, featureFingerprint)
}

// AuthorizationMenuReadRequest creates a separate private per-user menu
// projection key. It does not grant permission to cache session authority.
func AuthorizationMenuReadRequest(userID int64, featureFingerprint string) (ReadRequest, bool) {
	return authorizationReadRequest(DataClassAuthorizationMenus, userID, featureFingerprint)
}

// ActiveSessionValidityReadRequest derives an opaque session target. The raw
// identifier is intentionally scoped to this call and never stored in the
// request, key material, or transport envelope.
func ActiveSessionValidityReadRequest(sessionID string) (TargetedReadRequest, bool) {
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return TargetedReadRequest{}, false
	}
	entry, ok := Entry(DataClassActiveSessionValidity)
	if !ok {
		return TargetedReadRequest{}, false
	}
	return TargetedReadRequest{
		Entry: entry, TargetKind: "active-session", TargetDigest: ActiveSessionTargetDigest(sessionID),
		SchemaVersion: entry.SchemaVersion, MaxStale: entry.MaxStale,
	}, true
}

// ActiveSessionValidityReadRequestForDigest is used only after a strict v2
// envelope has already validated its irreversible target. It never accepts a
// raw session identifier.
func ActiveSessionValidityReadRequestForDigest(targetDigest string) (TargetedReadRequest, bool) {
	targetDigest = strings.TrimSpace(targetDigest)
	if !isDigest(targetDigest) {
		return TargetedReadRequest{}, false
	}
	entry, ok := Entry(DataClassActiveSessionValidity)
	if !ok {
		return TargetedReadRequest{}, false
	}
	return TargetedReadRequest{Entry: entry, TargetKind: "active-session", TargetDigest: targetDigest, SchemaVersion: entry.SchemaVersion, MaxStale: entry.MaxStale}, true
}

func authorizationReadRequest(class DataClass, userID int64, featureFingerprint string) (ReadRequest, bool) {
	if userID <= 0 || !isDigest(featureFingerprint) {
		return ReadRequest{}, false
	}
	identity := "user:" + strconv.FormatInt(userID, 10)
	return buildReadRequest(class, "authorization:private", identity, identity+":features:"+strings.ToLower(strings.TrimSpace(featureFingerprint)))
}

func buildReadRequest(class DataClass, requestScope, businessIdentity, target string) (ReadRequest, bool) {
	entry, ok := Entry(class)
	requestScope = strings.TrimSpace(requestScope)
	businessIdentity = strings.TrimSpace(businessIdentity)
	target = strings.TrimSpace(target)
	if !ok || requestScope == "" || businessIdentity == "" || target == "" || entry.StorageScope != StorageScopeSystemGlobal || entry.SchemaVersion <= 0 || entry.L1TTL <= 0 || entry.L2TTL <= 0 || entry.MaxStale < 0 {
		return ReadRequest{}, false
	}
	return ReadRequest{
		Entry:            entry,
		RequestScope:     requestScope,
		BusinessIdentity: businessIdentity,
		Target:           target,
		SchemaVersion:    entry.SchemaVersion,
		MaxStale:         entry.MaxStale,
	}, true
}

// ValidateLoaded verifies the current database row before the loader writes a
// cache value. This is intentionally stricter than a request allowlist:
// changing exposure, sensitivity, schema version, or enabled state causes an
// authoritative read without caching it.
func ValidateLoaded(request ReadRequest, exposure, sensitivity string, schemaVersion int, enabled bool) bool {
	if !enabled || !request.Valid() {
		return false
	}
	return strings.EqualFold(strings.TrimSpace(exposure), request.Entry.Exposure) &&
		strings.EqualFold(strings.TrimSpace(sensitivity), request.Entry.Sensitivity) &&
		schemaVersion == request.Entry.SchemaVersion
}

// Valid confirms that a request still exactly matches an immutable catalog
// entry. Infrastructure calls this before touching L1/L2 so a caller cannot
// construct a wider policy by hand.
func (r ReadRequest) Valid() bool {
	entry, ok := Entry(r.Entry.DataClass)
	if !ok || strings.TrimSpace(r.RequestScope) == "" || strings.TrimSpace(r.BusinessIdentity) == "" || strings.TrimSpace(r.Target) == "" {
		return false
	}
	if !cataloguedTargetMatches(r.Entry.DataClass, r.Target) {
		return false
	}
	return r.Entry.Owner == entry.Owner &&
		r.Entry.StorageScope == StorageScopeSystemGlobal &&
		r.Entry.StorageScope == entry.StorageScope &&
		r.Entry.SchemaVersion == entry.SchemaVersion &&
		r.Entry.Exposure == entry.Exposure &&
		r.Entry.Sensitivity == entry.Sensitivity &&
		r.Entry.L1TTL == entry.L1TTL &&
		r.Entry.L2TTL == entry.L2TTL &&
		r.Entry.MaxStale == entry.MaxStale &&
		r.SchemaVersion == entry.SchemaVersion &&
		r.MaxStale == entry.MaxStale
}

// cataloguedTargetMatches closes the construction bypass: a caller cannot
// copy an otherwise valid CatalogEntry and substitute an arbitrary config or
// dictionary target. Infrastructure accepts only the same immutable target
// allowlists used by the public constructors above.
func cataloguedTargetMatches(class DataClass, target string) bool {
	target = strings.TrimSpace(target)
	switch class {
	case DataClassConfigPublicScalar:
		mapped, ok := configAllowlist[target]
		return ok && mapped == class
	case DataClassDictPublicItems:
		mapped, ok := dictAllowlist[strings.ToLower(target)]
		return ok && mapped == class && target == strings.ToLower(target)
	case DataClassAuthorizationContext, DataClassAuthorizationMenus:
		parts := strings.Split(target, ":features:")
		if len(parts) != 2 || !strings.HasPrefix(parts[0], "user:") || !isDigest(parts[1]) {
			return false
		}
		userID := strings.TrimPrefix(parts[0], "user:")
		return userID != "" && strings.Trim(userID, "0123456789") == ""
	default:
		return false
	}
}

func isDigest(value string) bool {
	value = strings.TrimSpace(value)
	if len(value) != sha256.Size*2 {
		return false
	}
	for _, item := range value {
		if !(item >= '0' && item <= '9') && !(item >= 'a' && item <= 'f') && !(item >= 'A' && item <= 'F') {
			return false
		}
	}
	return true
}

// IsDigest exposes only structural validation for adapter allowlists. It
// cannot reconstruct or reveal a cache target.
func IsDigest(value string) bool { return isDigest(value) }

// KeyMaterial is safe for constructing an opaque cache-key suffix. It is not a
// durable event payload and never exposes the raw target, account, or scope.
func (r ReadRequest) KeyMaterial() string {
	return digest(strings.Join([]string{
		string(r.Entry.DataClass),
		r.Entry.StorageScope,
		strings.ToUpper(strings.TrimSpace(r.Entry.Exposure)),
		strings.TrimSpace(r.BusinessIdentity),
		strings.TrimSpace(r.RequestScope),
		strings.TrimSpace(r.Target),
		"schema=" + itoa(r.SchemaVersion),
	}, "\x00"))
}

// TargetDigest is the only target representation permitted in a DG5 durable
// invalidation event or cache diagnostic.
func (r ReadRequest) TargetDigest() string {
	return digest(string(r.Entry.DataClass) + "\x00" + strings.TrimSpace(r.Target))
}

// ActiveSessionTargetDigest is the only session representation that may cross
// a DG6.2 cache boundary. It is deliberately domain-separated from V1 events.
func ActiveSessionTargetDigest(sessionID string) string {
	return digest("sso-active-session-v2\x00" + strings.TrimSpace(sessionID))
}

func (r TargetedReadRequest) Valid() bool {
	entry, ok := Entry(r.Entry.DataClass)
	return ok && r.Entry.DataClass == DataClassActiveSessionValidity &&
		r.Entry.Owner == entry.Owner && r.Entry.StorageScope == entry.StorageScope &&
		r.Entry.SchemaVersion == entry.SchemaVersion && r.Entry.Exposure == entry.Exposure &&
		r.Entry.Sensitivity == entry.Sensitivity && r.Entry.L1TTL == entry.L1TTL &&
		r.Entry.L2TTL == entry.L2TTL && r.Entry.MaxStale == entry.MaxStale &&
		strings.TrimSpace(r.TargetKind) == "active-session" && isDigest(r.TargetDigest) &&
		r.SchemaVersion == SchemaVersionV2 && r.SchemaVersion == entry.SchemaVersion && r.MaxStale == 0
}

// KeyMaterial is opaque and contains only the irreversible target digest.
func (r TargetedReadRequest) KeyMaterial() string {
	return digest(strings.Join([]string{string(r.Entry.DataClass), r.Entry.StorageScope, r.Entry.Exposure, r.TargetKind, r.TargetDigest, "schema=" + itoa(r.SchemaVersion)}, "\x00"))
}

// ClassTargetDigest names a whole reviewed data class without carrying a raw
// target. It is used for mutations that can affect several catalogued keys.
func ClassTargetDigest(class DataClass) string {
	return digest("class\x00" + string(class))
}

// EventDigest turns an idempotency event identifier into a Redis-safe key
// component without exposing it in cache key diagnostics.
func EventDigest(eventID string) string {
	return digest("event\x00" + strings.TrimSpace(eventID))
}

func cloneEntry(entry CatalogEntry) CatalogEntry {
	entry.KeyDimensions = append([]string(nil), entry.KeyDimensions...)
	entry.InvalidationTriggers = append([]string(nil), entry.InvalidationTriggers...)
	return entry
}

func digest(value string) string {
	sum := sha256.Sum256([]byte(value))
	return hex.EncodeToString(sum[:])
}

func itoa(value int) string {
	if value == 0 {
		return "0"
	}
	negative := value < 0
	if negative {
		value = -value
	}
	var buf [20]byte
	pos := len(buf)
	for value > 0 {
		pos--
		buf[pos] = byte('0' + value%10)
		value /= 10
	}
	if negative {
		pos--
		buf[pos] = '-'
	}
	return string(buf[pos:])
}
