// Package schemaalignment owns the one-time, unpublished Workforce text-ID to
// Framework Snowflake alignment. It intentionally does not run from normal
// server startup.
package schemaalignment

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
)

type IDGenerator interface{ NextID() int64 }

type ProtectedSubject struct {
	Ciphertext string
	EDEK       string
	WrapKeyRef string
}

type SubjectProtector interface {
	EncryptSubject(context.Context, string) (ProtectedSubject, error)
	DigestSystemSubject(context.Context, string, string, string) (digest, keyRef string, err error)
}

type DB interface {
	Begin(context.Context) (pgx.Tx, error)
	Query(context.Context, string, ...any) (pgx.Rows, error)
	QueryRow(context.Context, string, ...any) pgx.Row
}

type Column struct {
	Table    string
	Name     string
	Nullable bool
}

type Report struct {
	State              string
	MappedIdentities   int
	ConvertedColumns   int
	TenantRows         int64
	PrincipalRows      int64
	LegacyEvidenceRows int64
	RemainingTextIDs   int
}

type CatalogStats struct {
	WorkforceTables        int64
	BigintIdentityColumns  int
	TextIdentityExceptions int
	PrincipalColumns       int64
	PrincipalRows          int64
	LegacyEvidenceRows     int64
	ForeignKeys            int64
}

const (
	openProjectionConstraintDefinition = "CHECK (((open_integration_projection_status)::text = ANY ((ARRAY['PENDING'::character varying, 'PROJECTED'::character varying, 'SKIPPED'::character varying])::text[])))"
	openProjectionIndexDefinition      = "CREATE INDEX idx_workforce_task_events_open_projection_pending ON public.workforce_task_events USING btree (id) WHERE (((open_integration_projection_status)::text = 'PENDING'::text) AND ((event_type)::text = ANY ((ARRAY['task.succeeded'::character varying, 'task.failed'::character varying, 'task.cancelled'::character varying, 'task.timed_out'::character varying])::text[])))"
)

var systemScopeTables = map[string]struct{}{
	"workforce_hook_packages": {}, "workforce_hook_package_revisions": {}, "workforce_hook_package_releases": {},
	"workforce_result_processors":          {},
	"workforce_result_processor_revisions": {}, "workforce_result_processor_releases": {},
}

var principalColumns = map[string]map[string]string{
	"workforce_completion_check_decisions":            {"actor_user_id": "actor_principal_id"},
	"workforce_workers":                               {"created_by": "created_by_principal_id"},
	"workforce_harness_channels":                      {"updated_by": "updated_by_principal_id"},
	"workforce_hook_packages":                         {"created_by": "created_by_principal_id"},
	"workforce_hook_package_revisions":                {"created_by": "created_by_principal_id"},
	"workforce_hook_package_releases":                 {"published_by": "published_by_principal_id"},
	"workforce_hook_test_runs":                        {"created_by": "created_by_principal_id"},
	"workforce_result_processors":                     {"created_by": "created_by_principal_id"},
	"workforce_result_processor_revisions":            {"created_by": "created_by_principal_id"},
	"workforce_result_processor_releases":             {"published_by": "published_by_principal_id"},
	"workforce_result_processor_bindings":             {"changed_by": "changed_by_principal_id"},
	"workforce_result_processor_environment_bindings": {"created_by": "created_by_principal_id"},
	"workforce_result_processor_fixture_runs":         {"created_by": "created_by_principal_id"},
	"workforce_continuation_checkpoints":              {"approved_by": "approved_by_principal_id"},
	"workforce_result_materialization_runs":           {"created_by": "created_by_principal_id"},
	"workforce_workspace_sources":                     {"created_by": "created_by_principal_id"},
	"workforce_source_ingestion_runs":                 {"created_by": "created_by_principal_id"},
	"workforce_reference_assets":                      {"created_by": "created_by_principal_id"},
	"workforce_git_delivery_targets":                  {"created_by": "created_by_principal_id"},
	"workforce_delivery_requests":                     {"created_by": "created_by_principal_id", "approved_by": "approved_by_principal_id", "reauthorized_by": "reauthorized_by_principal_id"},
	"workforce_delivery_operations":                   {"reauthorized_by": "reauthorized_by_principal_id"},
	"workforce_tasks":                                 {"cancel_requested_by": "cancel_requested_by_principal_id"},
	"workforce_task_turns":                            {"revision_trigger": "revision_trigger_principal_id"},
	"workforce_human_review_decisions":                {"actor": "actor_principal_id"},
	"workforce_execution_operation_decisions":         {"actor": "actor_principal_id"},
	"workforce_task_network_grants":                   {"actor": "actor_principal_id"},
	"workforce_projects":                              {"created_by": "created_by_principal_id", "updated_by": "updated_by_principal_id", "delivery_authorized_by": "delivery_authorized_by_principal_id"},
}

var optionalPrincipalColumns = map[string]struct{}{
	"workforce_workers.created_by_principal_id":                   {},
	"workforce_harness_channels.updated_by_principal_id":          {},
	"workforce_continuation_checkpoints.approved_by_principal_id": {},
	"workforce_delivery_requests.approved_by_principal_id":        {},
	"workforce_delivery_requests.reauthorized_by_principal_id":    {},
	"workforce_delivery_operations.reauthorized_by_principal_id":  {},
	"workforce_tasks.cancel_requested_by_principal_id":            {},
	"workforce_task_turns.revision_trigger_principal_id":          {},
	"workforce_projects.delivery_authorized_by_principal_id":      {},
}

func Inspect(ctx context.Context, db DB) (string, []Column, error) {
	columns, err := textIdentityColumns(ctx, db)
	if err != nil {
		return "", nil, err
	}
	var taskType string
	if err = db.QueryRow(ctx, `SELECT data_type FROM information_schema.columns WHERE table_schema='public' AND table_name='workforce_tasks' AND column_name='id'`).Scan(&taskType); err != nil {
		return "", nil, fmt.Errorf("inspect workforce_tasks.id: %w", err)
	}
	legacyActors, err := legacyPrincipalColumnCount(ctx, db)
	if err != nil {
		return "", nil, err
	}
	scopeDrift, err := scopeContractDriftCount(ctx, db)
	if err != nil {
		return "", nil, err
	}
	principalConstraintDrift, err := principalConstraintDriftCount(ctx, db)
	if err != nil {
		return "", nil, err
	}
	integrationCompatibilityDrift, err := openIntegrationCompatibilityDriftCount(ctx, db)
	if err != nil {
		return "", nil, err
	}
	textExceptionDrift, err := textIdentityExceptionDriftCount(ctx, db)
	if err != nil {
		return "", nil, err
	}
	if taskType == "bigint" && len(columns) == 0 && legacyActors == 0 && scopeDrift == 0 && principalConstraintDrift == 0 && integrationCompatibilityDrift == 0 && textExceptionDrift == 0 {
		return "ALIGNED", columns, nil
	}
	if taskType == "bigint" && len(columns) == 0 && integrationCompatibilityDrift > 0 {
		return "COMPATIBILITY_LEGACY", columns, nil
	}
	if taskType == "bigint" && len(columns) == 0 && textExceptionDrift > 0 {
		return "TEXT_EXCEPTION_LEGACY", columns, nil
	}
	if taskType == "bigint" && len(columns) == 0 && (legacyActors > 0 || scopeDrift > 0 || principalConstraintDrift > 0) {
		return "PRINCIPAL_LEGACY", columns, nil
	}
	if taskType == "text" {
		return "LEGACY", columns, nil
	}
	return "MIXED", columns, nil
}

func principalConstraintDriftCount(ctx context.Context, db DB) (int, error) {
	var count int
	err := db.QueryRow(ctx, `SELECT CASE WHEN to_regclass('public.workforce_principals') IS NULL THEN 2 ELSE 2-(SELECT count(*) FROM pg_constraint WHERE conrelid=to_regclass('public.workforce_principals') AND conname IN ('workforce_principals_type_ck','workforce_principals_shape_ck')) END`).Scan(&count)
	return count, err
}

func openIntegrationCompatibilityDriftCount(ctx context.Context, db DB) (int, error) {
	var tableExists bool
	if err := db.QueryRow(ctx, `SELECT to_regclass('public.workforce_integration_applications') IS NOT NULL`).Scan(&tableExists); err != nil || !tableExists {
		return 0, err
	}
	checks := []struct {
		table, column string
		wantPresent   bool
		allowedTypes  map[string]struct{}
	}{
		{"workforce_integration_applications", "created_by_principal_id", false, nil},
		{"workforce_integration_applications", "created_by", true, map[string]struct{}{"bigint": {}}},
		{"workforce_integration_applications", "credential_id", false, nil},
		{"workforce_integration_applications", "credential_locator", false, nil},
		{"workforce_integration_applications", "credential_digest", false, nil},
		{"workforce_integration_applications", "credential_hint", false, nil},
		{"workforce_integration_applications", "signing_secret_ciphertext", false, nil},
		{"workforce_integration_applications", "signing_secret_edek", false, nil},
		{"workforce_integration_applications", "signing_secret_wrap_key_ref", false, nil},
		{"workforce_integration_applications", "signing_secret_hint", false, nil},
		{"workforce_integration_api_credentials", "id", true, map[string]struct{}{"bigint": {}}},
		{"workforce_integration_api_credentials", "tenant_id", true, map[string]struct{}{"bigint": {}}},
		{"workforce_integration_api_credentials", "integration_application_id", true, map[string]struct{}{"bigint": {}}},
		{"workforce_integration_api_credentials", "credential_locator", true, map[string]struct{}{"text": {}, "character varying": {}}},
		{"workforce_integration_api_credentials", "created_by", true, map[string]struct{}{"bigint": {}}},
		{"workforce_integration_api_credentials", "last_used_at", true, map[string]struct{}{"timestamp with time zone": {}}},
		{"workforce_integration_signing_credentials", "id", true, map[string]struct{}{"bigint": {}}},
		{"workforce_integration_signing_credentials", "tenant_id", true, map[string]struct{}{"bigint": {}}},
		{"workforce_integration_signing_credentials", "integration_application_id", true, map[string]struct{}{"bigint": {}}},
		{"workforce_integration_signing_credentials", "created_by", true, map[string]struct{}{"bigint": {}}},
		{"workforce_integration_signing_credentials", "last_used_at", true, map[string]struct{}{"timestamp with time zone": {}}},
		{"workforce_tasks", "external_correlation_id", true, map[string]struct{}{"text": {}, "character varying": {}}},
		{"workforce_task_events", "open_integration_projection_status", true, map[string]struct{}{"text": {}, "character varying": {}}},
	}
	count := 0
	for _, check := range checks {
		var dataType string
		err := db.QueryRow(ctx, `SELECT data_type FROM information_schema.columns WHERE table_schema='public' AND table_name=$1 AND column_name=$2`, check.table, check.column).Scan(&dataType)
		if errors.Is(err, pgx.ErrNoRows) {
			if check.wantPresent {
				count++
			}
			continue
		}
		if err != nil {
			return 0, err
		}
		if !check.wantPresent {
			count++
			continue
		}
		if _, allowed := check.allowedTypes[dataType]; !allowed {
			count++
		}
	}
	var attemptsExist bool
	if err := db.QueryRow(ctx, `SELECT to_regclass('public.workforce_webhook_delivery_attempts') IS NOT NULL`).Scan(&attemptsExist); err != nil {
		return 0, err
	}
	if attemptsExist {
		for _, column := range []string{"signing_credential_id", "grace_signing_credential_id"} {
			var dataType string
			err := db.QueryRow(ctx, `SELECT data_type FROM information_schema.columns WHERE table_schema='public' AND table_name='workforce_webhook_delivery_attempts' AND column_name=$1`, column).Scan(&dataType)
			if errors.Is(err, pgx.ErrNoRows) || (err == nil && dataType != "bigint") {
				count++
			} else if err != nil {
				return 0, err
			}
		}
	}
	return count, nil
}

func textIdentityExceptionDriftCount(ctx context.Context, db DB) (int, error) {
	count := 0
	for key := range governedTextIdentityExceptions() {
		parts := strings.SplitN(key, ".", 2)
		if len(parts) != 2 {
			return 0, fmt.Errorf("invalid TEXT identity exception %q", key)
		}
		var dataType string
		err := db.QueryRow(ctx, `SELECT data_type FROM information_schema.columns WHERE table_schema='public' AND table_name=$1 AND column_name=$2`, parts[0], parts[1]).Scan(&dataType)
		if errors.Is(err, pgx.ErrNoRows) {
			continue
		}
		if err != nil {
			return 0, err
		}
		if dataType != "text" && dataType != "character varying" {
			count++
		}
	}
	return count, nil
}

func Align(ctx context.Context, db DB, idGen IDGenerator) (Report, error) {
	return AlignWithSubjectProtection(ctx, db, idGen, nil)
}

func AlignWithSubjectProtection(ctx context.Context, db DB, idGen IDGenerator, protector SubjectProtector) (Report, error) {
	if idGen == nil {
		return Report{}, errors.New("Framework Snowflake generator is required")
	}
	state, columns, err := Inspect(ctx, db)
	if err != nil {
		return Report{}, err
	}
	report := Report{State: state, RemainingTextIDs: len(columns)}
	var legacyCredentialTable, legacyInvocationTable, modelCredentialTable, modelInvocationTable bool
	if err = db.QueryRow(ctx, `SELECT
		to_regclass('public.workforce_provider_profiles') IS NOT NULL,
		to_regclass('public.workforce_provider_invocations') IS NOT NULL,
		to_regclass('public.workforce_model_credentials') IS NOT NULL,
		to_regclass('public.workforce_model_invocations') IS NOT NULL`).Scan(
		&legacyCredentialTable, &legacyInvocationTable, &modelCredentialTable, &modelInvocationTable); err != nil {
		return report, err
	}
	legacyModelCredentialCut := legacyCredentialTable && legacyInvocationTable && !modelCredentialTable && !modelInvocationTable
	if (legacyCredentialTable || legacyInvocationTable) && !legacyModelCredentialCut {
		return report, errors.New("mixed Model Credential schema; refusing partial hard cut")
	}
	if state == "ALIGNED" || (state == "MIXED" && legacyModelCredentialCut) {
		// The unpublished schema may gain additive Workforce capabilities after
		// the BIGINT cutover. Keep the alignment command as the single,
		// transactional local-database convergence path even when identity types
		// are already aligned.
		tx, beginErr := db.Begin(ctx)
		if beginErr != nil {
			return report, beginErr
		}
		defer func() { _ = tx.Rollback(ctx) }()
		if _, err = tx.Exec(ctx, `SELECT pg_advisory_xact_lock(774966710092023091)`); err != nil {
			return report, fmt.Errorf("acquire alignment lock: %w", err)
		}
		if err = preflight(ctx, tx); err != nil {
			return report, err
		}
		if err = ensureProviderInvocationSchema(ctx, tx, idGen); err != nil {
			return report, err
		}
		if err = ensureModelCredentialAccess(ctx, tx, idGen); err != nil {
			return report, err
		}
		if err = ensureProjectHookHardCut(ctx, tx); err != nil {
			return report, err
		}
		if err = ensureWorkerMetricSchema(ctx, tx); err != nil {
			return report, err
		}
		if err = ensureTaskAnalyticsSchema(ctx, tx); err != nil {
			return report, err
		}
		if err = repairOpenIntegrationCompatibility(ctx, tx, idGen); err != nil {
			return report, err
		}
		if err = backfillOpenIntegrationProjectionState(ctx, tx); err != nil {
			return report, err
		}
		if err = ensureTaskNotificationSchema(ctx, tx, idGen); err != nil {
			return report, err
		}
		if err = ensureNotificationActionableSchema(ctx, tx); err != nil {
			return report, err
		}
		if err = ensureEnterpriseIdentitySchema(ctx, tx); err != nil {
			return report, err
		}
		if err = protectLegacySystemSubjects(ctx, tx, protector); err != nil {
			return report, err
		}
		if err = ensureTaskNotificationPermission(ctx, tx, idGen); err != nil {
			return report, err
		}
		if err = ensureNotificationGroupRoutePermissions(ctx, tx, idGen); err != nil {
			return report, err
		}
		if err = ensureEnterpriseIdentityPermissions(ctx, tx, idGen); err != nil {
			return report, err
		}
		workerFleetPresent, err := workerFleetControlPlanePresent(ctx, tx)
		if err != nil {
			return report, err
		}
		if !workerFleetPresent {
			return report, errors.New("Worker Fleet migration must be applied before independent review alignment")
		}
		if err = ensureReviewRunSchema(ctx, tx); err != nil {
			return report, err
		}
		if err = ensureGitDeliverySchema(ctx, tx); err != nil {
			return report, err
		}
		if err = ensureUnpublishedSchemaIndexes(ctx, tx); err != nil {
			return report, err
		}
		if err = alignWorkforceColumnTypes(ctx, tx); err != nil {
			return report, err
		}
		if err = verifyTx(ctx, tx, true); err != nil {
			return report, err
		}
		if err = tx.Commit(ctx); err != nil {
			return report, fmt.Errorf("commit additive Workforce alignment: %w", err)
		}
		report.State = "ALIGNED"
		report.RemainingTextIDs = 0
		return report, Verify(ctx, db)
	}
	if state != "LEGACY" && state != "PRINCIPAL_LEGACY" && state != "COMPATIBILITY_LEGACY" && state != "TEXT_EXCEPTION_LEGACY" {
		return report, errors.New("mixed Workforce identity schema; refusing to guess partial alignment")
	}
	tx, err := db.Begin(ctx)
	if err != nil {
		return report, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if _, err = tx.Exec(ctx, `SELECT pg_advisory_xact_lock(774966710092023091)`); err != nil {
		return report, fmt.Errorf("acquire alignment lock: %w", err)
	}
	if err = preflight(ctx, tx); err != nil {
		return report, err
	}
	if err = ensureProviderInvocationSchema(ctx, tx, idGen); err != nil {
		return report, err
	}
	if err = ensureModelCredentialAccess(ctx, tx, idGen); err != nil {
		return report, err
	}
	if err = ensureProjectHookHardCut(ctx, tx); err != nil {
		return report, err
	}
	if err = ensureWorkerMetricSchema(ctx, tx); err != nil {
		return report, err
	}
	if err = ensureTaskAnalyticsSchema(ctx, tx); err != nil {
		return report, err
	}
	if err = ensureSupportSchema(ctx, tx); err != nil {
		return report, err
	}
	if err = ensureNotificationActionableSchema(ctx, tx); err != nil {
		return report, err
	}
	if err = ensureEnterpriseIdentitySchema(ctx, tx); err != nil {
		return report, err
	}
	if err = protectLegacySystemSubjects(ctx, tx, protector); err != nil {
		return report, err
	}
	if err = ensureTaskNotificationPermission(ctx, tx, idGen); err != nil {
		return report, err
	}
	if err = ensureNotificationGroupRoutePermissions(ctx, tx, idGen); err != nil {
		return report, err
	}
	if err = ensureEnterpriseIdentityPermissions(ctx, tx, idGen); err != nil {
		return report, err
	}
	// The identity alignment path predates Worker Fleet. It must not create a
	// partial Reviewer control plane in a database that will later apply the
	// authoritative Fleet migration: doing so would make that migration fail on
	// existing tables. Once Fleet is installed, this same transaction converges
	// the Slice 2-4 Review Policy and ReviewRun schema as usual.
	workerFleetPresent, err := workerFleetControlPlanePresent(ctx, tx)
	if err != nil {
		return report, err
	}
	if workerFleetPresent {
		if err = ensureReviewRunSchema(ctx, tx); err != nil {
			return report, err
		}
		if err = ensureGitDeliverySchema(ctx, tx); err != nil {
			return report, err
		}
	}
	if workerFleetPresent {
		if err = ensureUnpublishedSchemaIndexes(ctx, tx); err != nil {
			return report, err
		}
	}
	if err = repairOpenIntegrationCompatibility(ctx, tx, idGen); err != nil {
		return report, err
	}
	if err = repairTextIdentityExceptions(ctx, tx); err != nil {
		return report, err
	}
	regular := make([]Column, 0, len(columns))
	for _, column := range columns {
		if column.Name != "tenant_id" {
			regular = append(regular, column)
		}
	}
	identityMap, err := loadIdentityMap(ctx, tx)
	if err != nil {
		return report, err
	}
	newIdentityMap, err := buildIdentityMap(ctx, tx, idGen, regular)
	if err != nil {
		return report, err
	}
	for legacy, id := range newIdentityMap {
		identityMap[legacy] = id
	}
	report.MappedIdentities = len(newIdentityMap)
	if report.LegacyEvidenceRows, err = persistLegacyEvidence(ctx, tx, newIdentityMap); err != nil {
		return report, err
	}
	if report.TenantRows, err = alignTenants(ctx, tx, columns); err != nil {
		return report, err
	}
	if err = enforceScopeContracts(ctx, tx); err != nil {
		return report, err
	}
	for _, column := range regular {
		if err = alignIdentityColumn(ctx, tx, column); err != nil {
			return report, err
		}
		report.ConvertedColumns++
	}
	if report.PrincipalRows, err = alignPrincipals(ctx, tx, idGen, identityMap); err != nil {
		return report, err
	}
	// Task notification rows use BIGINT Task and tenant identities.  A legacy
	// database still has those identities as TEXT until the conversion above,
	// so creating/backfilling the additive notification relations any earlier
	// would compare incompatible types and roll the whole alignment back.
	if err = ensureTaskNotificationSchema(ctx, tx, idGen); err != nil {
		return report, err
	}
	if err = backfillOpenIntegrationProjectionState(ctx, tx); err != nil {
		return report, err
	}
	if err = alignWorkforceColumnTypes(ctx, tx); err != nil {
		return report, err
	}
	if err = verifyTx(ctx, tx, workerFleetPresent); err != nil {
		return report, err
	}
	if err = tx.Commit(ctx); err != nil {
		return report, fmt.Errorf("commit identity alignment: %w", err)
	}
	report.State = "ALIGNED"
	report.RemainingTextIDs = 0
	return report, nil
}

func Verify(ctx context.Context, db DB) error {
	if err := verifyWorkerMetricSchema(ctx, db); err != nil {
		return err
	}
	if err := verifyTaskAnalyticsSchema(ctx, db); err != nil {
		return err
	}
	if err := verifyProjectSchema(ctx, db); err != nil {
		return err
	}
	if err := verifyGitDeliverySchema(ctx, db); err != nil {
		return err
	}
	if err := verifyExecutionAccessSchema(ctx, db); err != nil {
		return err
	}
	if err := verifyInteractionSchema(ctx, db); err != nil {
		return err
	}
	if err := verifyFactProducerSchema(ctx, db); err != nil {
		return err
	}
	if err := verifyCompletionCheckSchema(ctx, db); err != nil {
		return err
	}
	state, columns, err := Inspect(ctx, db)
	if err != nil {
		return err
	}
	if state != "ALIGNED" || len(columns) != 0 {
		return fmt.Errorf("Workforce identity schema is %s with %d non-allowlisted TEXT identities", state, len(columns))
	}
	if err = verifyTaskNotificationSchemaShape(ctx, db); err != nil {
		return err
	}
	if err = verifyNotificationActionableSchemaShape(ctx, db); err != nil {
		return err
	}
	if err = verifyEnterpriseIdentitySchemaShape(ctx, db); err != nil {
		return err
	}
	if err = verifyReviewRunSchemaShape(ctx, db); err != nil {
		return err
	}
	if err = verifyReviewRunSchemaBehavior(ctx, db); err != nil {
		return err
	}
	if err = verifyProviderInvocationSchemaShape(ctx, db); err != nil {
		return err
	}
	var taskNotificationPermission bool
	if err = db.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM sys_permission
		WHERE code='workforce:task-notification:operate' AND "isDeleted"=false)`).Scan(&taskNotificationPermission); err != nil {
		return err
	}
	if !taskNotificationPermission {
		return errors.New("Task notification permission is not aligned")
	}
	var taskNotificationRoleBindings bool
	if err = db.QueryRow(ctx, `SELECT NOT EXISTS (
		SELECT 1 FROM sys_role r
		WHERE r.code IN ('SUPER_ADMIN','SYSTEM_ADMIN') AND r."isDeleted"=false
		AND NOT EXISTS (
			SELECT 1 FROM sys_role_permission rp
			JOIN sys_permission p ON p.id=rp."permissionId" AND p."isDeleted"=false
			WHERE rp."roleId"=r.id AND p.code='workforce:task-notification:operate'
		)
	)`).Scan(&taskNotificationRoleBindings); err != nil {
		return err
	}
	if !taskNotificationRoleBindings {
		return errors.New("Task notification role permissions are not aligned")
	}
	var groupRoutePermissionCount int
	if err = db.QueryRow(ctx, `SELECT COUNT(*) FROM sys_permission WHERE code LIKE 'system:notification-group-route:%' AND "isDeleted"=false`).Scan(&groupRoutePermissionCount); err != nil {
		return err
	}
	if groupRoutePermissionCount != 9 {
		return errors.New("Notification group route permissions are not aligned")
	}
	var enterprisePermissionCount int
	if err = db.QueryRow(ctx, `SELECT COUNT(*) FROM sys_permission WHERE code IN (
		'system:enterprise-connection:list','system:enterprise-connection:edit','system:enterprise-connection:status',
		'system:enterprise-identity:list','system:enterprise-identity:login-access') AND "isDeleted"=false`).Scan(&enterprisePermissionCount); err != nil {
		return err
	}
	if enterprisePermissionCount != 5 {
		return errors.New("Enterprise identity permissions are not aligned")
	}
	var enterpriseMenuID int64
	if err = db.QueryRow(ctx, `SELECT id FROM sys_menu WHERE path='/system/enterprise-connections' AND "isDeleted"=false`).Scan(&enterpriseMenuID); err != nil {
		return err
	}
	var enterpriseMenuPermissionsAligned bool
	if err = db.QueryRow(ctx, `SELECT NOT EXISTS (
		SELECT 1 FROM sys_permission p
		WHERE p.code IN (
			'system:enterprise-connection:list','system:enterprise-connection:edit','system:enterprise-connection:status',
			'system:enterprise-identity:list','system:enterprise-identity:login-access') AND p."isDeleted"=false
		AND NOT EXISTS (SELECT 1 FROM sys_menu_permission mp WHERE mp."menuId"=$1 AND mp."permissionId"=p.id)
	)`, enterpriseMenuID).Scan(&enterpriseMenuPermissionsAligned); err != nil {
		return err
	}
	var enterpriseRoleMenusAligned bool
	if err = db.QueryRow(ctx, `SELECT NOT EXISTS (
		SELECT 1 FROM sys_role r
		WHERE r.code IN ('SUPER_ADMIN','SYSTEM_ADMIN') AND r."isDeleted"=false
		AND NOT EXISTS (SELECT 1 FROM sys_role_menu rm WHERE rm."roleId"=r.id AND rm."menuId"=$1)
	)`, enterpriseMenuID).Scan(&enterpriseRoleMenusAligned); err != nil {
		return err
	}
	if !enterpriseMenuPermissionsAligned || !enterpriseRoleMenusAligned {
		return errors.New("Enterprise identity menu access is not aligned")
	}
	var requiredUnpublishedIndexes bool
	if err = db.QueryRow(ctx, `SELECT to_regclass('public.workforce_result_processor_releases_version_uidx') IS NOT NULL`).Scan(&requiredUnpublishedIndexes); err != nil {
		return err
	}
	if !requiredUnpublishedIndexes {
		return errors.New("unpublished Workforce unique indexes are not aligned")
	}
	var sourceIngestionNullableColumns int
	if err = db.QueryRow(ctx, `SELECT COUNT(*) FROM information_schema.columns
		WHERE table_schema=current_schema() AND table_name='workforce_source_ingestion_runs'
		AND column_name IN ('workspace_source_revision_id','worker_id') AND is_nullable='YES'`).Scan(&sourceIngestionNullableColumns); err != nil {
		return err
	}
	if sourceIngestionNullableColumns != 2 {
		return errors.New("unpublished source-ingestion nullability is not aligned")
	}
	numericColumns, err := numericIdentityColumns(ctx, db)
	if err != nil {
		return err
	}
	for _, column := range numericColumns {
		var invalid int64
		query := fmt.Sprintf(`SELECT count(*) FROM %s WHERE %s IS NOT NULL AND %s<=0`, ident(column.Table), ident(column.Name), ident(column.Name))
		if err = db.QueryRow(ctx, query).Scan(&invalid); err != nil {
			return err
		}
		if invalid != 0 {
			return fmt.Errorf("%s.%s contains %d non-positive internal identities", column.Table, column.Name, invalid)
		}
	}
	var invalidPrincipals int64
	if err = db.QueryRow(ctx, `SELECT count(*) FROM workforce_principals WHERE id<=0 OR NOT (
      (principal_type='USER' AND user_id>0 AND worker_id IS NULL AND service_code IS NULL) OR
      (principal_type='WORKER' AND user_id IS NULL AND worker_id>0 AND service_code IS NULL) OR
      (principal_type='SERVICE' AND user_id IS NULL AND worker_id IS NULL AND NULLIF(btrim(service_code),'') IS NOT NULL)
    )`).Scan(&invalidPrincipals); err != nil {
		return err
	}
	if invalidPrincipals != 0 {
		return fmt.Errorf("found %d structurally invalid Workforce principals", invalidPrincipals)
	}
	var foreignKeys int64
	if err = db.QueryRow(ctx, `SELECT count(*) FROM pg_constraint WHERE contype='f' AND conrelid::regclass::text LIKE 'workforce_%'`).Scan(&foreignKeys); err != nil {
		return err
	}
	if foreignKeys != 0 {
		return fmt.Errorf("found %d forbidden Workforce foreign keys", foreignKeys)
	}
	var webhookType, webhookNullable string
	var webhookMaxLength *int
	var webhookDefault *string
	if err = db.QueryRow(ctx, `SELECT data_type,character_maximum_length,is_nullable,column_default
FROM information_schema.columns WHERE table_schema='public' AND table_name='workforce_integration_applications' AND column_name='webhook_url'`).Scan(&webhookType, &webhookMaxLength, &webhookNullable, &webhookDefault); err != nil {
		return err
	}
	if webhookType != "character varying" || webhookMaxLength == nil || *webhookMaxLength != 2048 || webhookNullable != "NO" || webhookDefault == nil || !strings.Contains(*webhookDefault, "''") {
		return fmt.Errorf("Integration Application webhook URL schema is not VARCHAR(2048) NOT NULL DEFAULT empty")
	}
	var projectionColumnAligned bool
	if err = db.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM information_schema.columns
WHERE table_schema='public' AND table_name='workforce_task_events' AND column_name='open_integration_projection_status'
  AND data_type='character varying' AND character_maximum_length=32 AND is_nullable='NO' AND column_default LIKE '%PENDING%')`).Scan(&projectionColumnAligned); err != nil {
		return err
	}
	var projectionConstraintDefinition, projectionIndexDefinition string
	if err = db.QueryRow(ctx, `SELECT COALESCE((SELECT pg_get_constraintdef(oid) FROM pg_constraint
WHERE conrelid='workforce_task_events'::regclass AND conname='workforce_task_events_open_projection_status_ck'
  AND contype='c' AND convalidated),'')`).Scan(&projectionConstraintDefinition); err != nil {
		return err
	}
	if err = db.QueryRow(ctx, `SELECT COALESCE((SELECT pg_get_indexdef(i.oid) FROM pg_class i JOIN pg_index x ON x.indexrelid=i.oid
WHERE i.relnamespace='public'::regnamespace AND i.relname='idx_workforce_task_events_open_projection_pending'
  AND x.indisvalid AND x.indisready),'')`).Scan(&projectionIndexDefinition); err != nil {
		return err
	}
	if !projectionColumnAligned ||
		projectionConstraintDefinition != openProjectionConstraintDefinition ||
		projectionIndexDefinition != openProjectionIndexDefinition {
		return errors.New("Open Integration projection state schema is not aligned")
	}
	credentialChecks := []struct {
		label string
		query string
	}{
		{"API Credentials without matching application", `SELECT count(*) FROM workforce_integration_api_credentials c WHERE NOT EXISTS (SELECT 1 FROM workforce_integration_applications a WHERE a.id=c.integration_application_id AND a.tenant_id=c.tenant_id)`},
		{"Signing Credentials without matching application", `SELECT count(*) FROM workforce_integration_signing_credentials c WHERE NOT EXISTS (SELECT 1 FROM workforce_integration_applications a WHERE a.id=c.integration_application_id AND a.tenant_id=c.tenant_id)`},
		{"applications without exactly one current API Credential", `SELECT count(*) FROM workforce_integration_applications a WHERE 1<>(SELECT count(*) FROM workforce_integration_api_credentials c WHERE c.integration_application_id=a.id AND c.tenant_id=a.tenant_id AND c.status='CURRENT')`},
		{"webhook applications without exactly one current Signing Credential", `SELECT count(*) FROM workforce_integration_applications a WHERE a.webhook_url<>'' AND 1<>(SELECT count(*) FROM workforce_integration_signing_credentials c WHERE c.integration_application_id=a.id AND c.tenant_id=a.tenant_id AND c.status='CURRENT')`},
		{"API Credentials with invalid lifecycle timestamps", `SELECT count(*) FROM workforce_integration_api_credentials WHERE (status='GRACE' AND valid_until IS NULL) OR (status='REVOKED' AND revoked_at IS NULL)`},
		{"Signing Credentials with invalid lifecycle timestamps", `SELECT count(*) FROM workforce_integration_signing_credentials WHERE (status='GRACE' AND valid_until IS NULL) OR (status='REVOKED' AND revoked_at IS NULL)`},
	}
	for _, check := range credentialChecks {
		var invalid int64
		if err = db.QueryRow(ctx, check.query).Scan(&invalid); err != nil {
			return err
		}
		if invalid != 0 {
			return fmt.Errorf("found %d %s", invalid, check.label)
		}
	}
	var deliveryAttemptsExist bool
	if err = db.QueryRow(ctx, `SELECT to_regclass('public.workforce_webhook_delivery_attempts') IS NOT NULL`).Scan(&deliveryAttemptsExist); err != nil {
		return err
	}
	if deliveryAttemptsExist {
		attemptChecks := []struct {
			label string
			query string
		}{
			{"Webhook Delivery Attempts without matching Signing Credential", `SELECT count(*) FROM workforce_webhook_delivery_attempts a WHERE NOT EXISTS (SELECT 1 FROM workforce_integration_signing_credentials c WHERE c.id=a.signing_credential_id AND c.integration_application_id=a.integration_application_id AND c.tenant_id=a.tenant_id)`},
			{"Webhook Delivery Attempts without matching Grace Signing Credential", `SELECT count(*) FROM workforce_webhook_delivery_attempts a WHERE a.grace_signing_credential_id IS NOT NULL AND NOT EXISTS (SELECT 1 FROM workforce_integration_signing_credentials c WHERE c.id=a.grace_signing_credential_id AND c.integration_application_id=a.integration_application_id AND c.tenant_id=a.tenant_id)`},
		}
		for _, check := range attemptChecks {
			var invalid int64
			if err = db.QueryRow(ctx, check.query).Scan(&invalid); err != nil {
				return err
			}
			if invalid != 0 {
				return fmt.Errorf("found %d %s", invalid, check.label)
			}
		}
	}
	return nil
}

func verifyTaskNotificationSchemaShape(ctx context.Context, q interface {
	QueryRow(context.Context, string, ...any) pgx.Row
}) error {
	var columnDrift int
	if err := q.QueryRow(ctx, `WITH expected(table_name,column_name,data_type,is_nullable) AS (VALUES
		('workforce_task_notification_subscriptions','id','bigint','NO'),
		('workforce_task_notification_subscriptions','tenant_id','bigint','NO'),
		('workforce_task_notification_subscriptions','task_id','bigint','NO'),
		('workforce_task_notification_subscriptions','subscriber_user_id','bigint','NO'),
		('workforce_task_notification_subscriptions','events_json','jsonb','NO'),
		('workforce_task_notification_subscriptions','enabled','boolean','NO'),
		('workforce_task_notification_subscriptions','version','integer','NO'),
		('workforce_task_notification_subscriptions','created_by','bigint','NO'),
		('workforce_task_notification_subscriptions','updated_by','bigint','NO'),
		('workforce_task_notification_subscription_channels','id','bigint','NO'),
		('workforce_task_notification_subscription_channels','tenant_id','bigint','NO'),
		('workforce_task_notification_subscription_channels','subscription_id','bigint','NO'),
		('workforce_task_notification_subscription_channels','channel_kind','character varying','NO'),
		('workforce_task_notification_subscription_channels','group_route_id','bigint','YES'),
		('workforce_task_notification_records','id','bigint','NO'),
		('workforce_task_notification_records','tenant_id','bigint','NO'),
		('workforce_task_notification_records','task_id','bigint','NO'),
		('workforce_task_notification_records','task_turn_id','bigint','NO'),
		('workforce_task_notification_records','subscription_id','bigint','YES'),
		('workforce_task_notification_records','subscriber_user_id','bigint','NO'),
		('workforce_task_notification_records','subscription_version','integer','YES'),
		('workforce_task_notification_records','source_id','bigint','NO'),
		('workforce_task_notification_records','source_occurrence','bigint','NO'),
		('workforce_task_notification_records','fact_identity_version','integer','NO'),
		('workforce_task_notification_records','fact_type','character varying','NO'),
		('workforce_task_notification_records','source_type','character varying','NO'),
		('workforce_task_notification_records','focus','character varying','NO'),
		('workforce_task_notification_records','publication_contract_version','integer','NO'),
		('workforce_task_notification_records','scene_code','text','NO'),
		('workforce_task_notification_records','template_variables_json','jsonb','NO'),
		('workforce_task_notification_records','idempotency_identity','text','NO'),
		('workforce_task_notification_records','notification_receipt_identity','text','NO'),
		('workforce_task_notification_records','lease_token','text','NO'),
		('workforce_task_notification_records','publish_attempts','integer','NO'),
		('workforce_task_notification_records','occurred_at','timestamp with time zone','NO')
		,('workforce_task_notification_record_channels','id','bigint','NO')
		,('workforce_task_notification_record_channels','tenant_id','bigint','NO')
		,('workforce_task_notification_record_channels','record_id','bigint','NO')
		,('workforce_task_notification_record_channels','channel_kind','character varying','NO')
		,('workforce_task_notification_record_channels','group_route_id','bigint','YES')
		,('workforce_task_notification_record_channels','mention_user_id','bigint','YES')
		,('workforce_task_notification_record_recipients','id','bigint','NO')
		,('workforce_task_notification_record_recipients','tenant_id','bigint','NO')
		,('workforce_task_notification_record_recipients','record_id','bigint','NO')
		,('workforce_task_notification_record_recipients','user_id','bigint','NO')
		,('workforce_task_notification_record_recipients','recipient_role','character varying','NO')
		,('workforce_task_notification_record_recipients','personal_channel_policy','character varying','NO')
		,('workforce_task_notification_record_recipients','in_app_required','boolean','NO')
		,('sys_notification_user_preferences','id','bigint','NO')
		,('sys_notification_user_preferences','organization_id','bigint','NO')
		,('sys_notification_user_preferences','user_id','bigint','NO')
		,('sys_notification_user_preferences','event_family','text','NO')
		,('sys_notification_user_preferences','channel_kind','text','NO')
		,('sys_notification_user_preferences','enabled','boolean','NO')
		,('sys_notification_user_preferences','version','bigint','NO')
	)
	SELECT count(*) FROM expected e
	LEFT JOIN information_schema.columns c ON c.table_schema='public' AND c.table_name=e.table_name AND c.column_name=e.column_name
	WHERE c.column_name IS NULL OR c.data_type<>e.data_type OR c.is_nullable<>e.is_nullable`).Scan(&columnDrift); err != nil {
		return fmt.Errorf("verify Task notification columns: %w", err)
	}
	if columnDrift != 0 {
		return fmt.Errorf("Task notification schema has %d missing or malformed columns", columnDrift)
	}
	var indexShape bool
	if err := q.QueryRow(ctx, `WITH expected_indexes(index_name,index_definition,predicate) AS (VALUES
		('workforce_task_notification_subscriptions_lookup_idx',
		 'CREATE INDEX workforce_task_notification_subscriptions_lookup_idx ON public.workforce_task_notification_subscriptions USING btree (tenant_id, task_id, enabled)',''),
		('workforce_task_notification_records_task_idx',
		 'CREATE INDEX workforce_task_notification_records_task_idx ON public.workforce_task_notification_records USING btree (tenant_id, task_id, occurred_at DESC, id DESC)',''),
		('workforce_task_notification_records_dispatch_idx',
		 'CREATE INDEX workforce_task_notification_records_dispatch_idx ON public.workforce_task_notification_records USING btree (status, next_retry_at, lease_until, occurred_at) WHERE ((status)::text = ANY ((ARRAY[''PENDING''::character varying, ''PROCESSING''::character varying, ''RETRY_WAIT''::character varying])::text[]))',
		 '((status)::text = ANY ((ARRAY[''PENDING''::character varying, ''PROCESSING''::character varying, ''RETRY_WAIT''::character varying])::text[]))')
	)
	SELECT NOT EXISTS (
		SELECT 1 FROM expected_indexes e
		LEFT JOIN pg_class i ON i.relname=e.index_name AND i.relnamespace='public'::regnamespace
		LEFT JOIN pg_index x ON x.indexrelid=i.oid
		WHERE i.oid IS NULL OR NOT x.indisvalid OR NOT x.indisready
		   OR pg_get_indexdef(i.oid)<>e.index_definition
		   OR COALESCE(pg_get_expr(x.indpred,x.indrelid),'')<>e.predicate
	)`).Scan(&indexShape); err != nil {
		return fmt.Errorf("verify Task notification indexes: %w", err)
	}
	if !indexShape {
		return errors.New("Task notification indexes are not aligned")
	}
	var checkShape bool
	if err := q.QueryRow(ctx, `WITH expected_checks(table_name,definition) AS (VALUES
		('workforce_task_notification_subscriptions','CHECK ((jsonb_typeof(events_json) = ''array''::text))'),
		('workforce_task_notification_subscriptions','CHECK ((version > 0))'),
		('workforce_task_notification_records','CHECK ((subscription_version > 0))'),
		('workforce_task_notification_records','CHECK ((turn_number > 0))'),
		('workforce_task_notification_records','CHECK ((publish_attempts >= 0))'),
		('workforce_task_notification_records','CHECK (((status)::text = ANY ((ARRAY[''PENDING''::character varying, ''PROCESSING''::character varying, ''RETRY_WAIT''::character varying, ''PUBLISHED''::character varying, ''SUPPRESSED''::character varying, ''FAILED''::character varying])::text[])))'),
		('workforce_task_notification_records','CHECK (((fact_type)::text = ANY ((ARRAY[''TASK_ACTION_REQUIRED''::character varying, ''TASK_INTERRUPTED''::character varying, ''TASK_COMPLETED''::character varying, ''DELIVERY_COMPLETED''::character varying])::text[])))'),
		('workforce_task_notification_records','CHECK (((source_type)::text = ANY ((ARRAY[''CONTINUATION_CHECKPOINT''::character varying, ''FINALIZATION_RUN''::character varying, ''ARTIFACT_UPLOAD''::character varying, ''RESULT_EVALUATION''::character varying, ''TASK_ATTEMPT''::character varying, ''DELIVERY_REQUEST''::character varying, ''REVIEW_OPERATION''::character varying, ''INDEPENDENT_REVIEW_RUN''::character varying, ''RUNTIME_RECOVERY''::character varying, ''TASK_TIMEOUT''::character varying, ''TASK_CANCEL''::character varying])::text[])))'),
		('workforce_task_notification_records','CHECK ((source_occurrence >= 0))'),
		('workforce_task_notification_records','CHECK ((fact_identity_version = ANY (ARRAY[1, 2])))'),
		('workforce_task_notification_records','CHECK (((focus)::text = ANY ((ARRAY[''evaluation''::character varying, ''result''::character varying, ''continuation''::character varying, ''delivery''::character varying])::text[])))'),
		('workforce_task_notification_records','CHECK ((((status)::text = ''SUPPRESSED''::text) OR ((subscription_id IS NOT NULL) AND (subscription_version IS NOT NULL))))')
	)
	SELECT NOT EXISTS (
			SELECT 1 FROM expected_checks e
			WHERE NOT EXISTS (
				SELECT 1 FROM pg_constraint c
				WHERE c.conrelid=('public.'||e.table_name)::regclass
				  AND c.contype='c' AND c.convalidated
				  AND pg_get_constraintdef(c.oid)=e.definition
			)
		)`).Scan(&checkShape); err != nil {
		return fmt.Errorf("verify Task notification checks: %w", err)
	}
	if !checkShape {
		return errors.New("Task notification checks are not aligned")
	}
	var uniquenessShape bool
	if err := q.QueryRow(ctx, `SELECT EXISTS (
			SELECT 1 FROM pg_constraint
			WHERE conrelid='workforce_task_notification_subscriptions'::regclass AND contype='u'
			AND pg_get_constraintdef(oid)='UNIQUE (tenant_id, task_id, subscriber_user_id)'
		)
		AND EXISTS (
			SELECT 1 FROM pg_class i JOIN pg_index x ON x.indexrelid=i.oid
			WHERE i.relnamespace='public'::regnamespace
			  AND i.relname='workforce_task_notification_records_fact_v2_uidx'
			  AND x.indisunique AND x.indisvalid AND x.indisready
		)`).Scan(&uniquenessShape); err != nil {
		return fmt.Errorf("verify Task notification uniqueness constraints: %w", err)
	}
	if !uniquenessShape {
		return errors.New("Task notification uniqueness constraints are not aligned")
	}
	orphanChecks := []struct {
		name  string
		query string
	}{
		{
			name: "historical Task default subscription",
			query: `SELECT count(*) FROM workforce_tasks t
				WHERE t.created_by > 0
				  AND NOT EXISTS (
					SELECT 1 FROM workforce_task_notification_subscriptions s
					WHERE s.tenant_id=t.tenant_id AND s.task_id=t.id AND s.subscriber_user_id=t.created_by
				  )`,
		},
		{
			name: "subscription channel",
			query: `SELECT count(*) FROM workforce_task_notification_subscriptions s
				WHERE NOT EXISTS (
					SELECT 1 FROM workforce_task_notification_subscription_channels c
					WHERE c.tenant_id=s.tenant_id AND c.subscription_id=s.id
				)`,
		},
		{
			name: "subscription task",
			query: `SELECT count(*) FROM workforce_task_notification_subscriptions s
				LEFT JOIN workforce_tasks t ON t.id=s.task_id AND t.tenant_id=s.tenant_id
				WHERE t.id IS NULL`,
		},
		{
			name: "record task",
			query: `SELECT count(*) FROM workforce_task_notification_records r
				LEFT JOIN workforce_tasks t ON t.id=r.task_id AND t.tenant_id=r.tenant_id
				WHERE t.id IS NULL`,
		},
		{
			name: "record turn",
			query: `SELECT count(*) FROM workforce_task_notification_records r
				LEFT JOIN workforce_task_turns tr ON tr.id=r.task_turn_id AND tr.task_id=r.task_id AND tr.tenant_id=r.tenant_id
				WHERE tr.id IS NULL`,
		},
		{
			name: "record subscription",
			query: `SELECT count(*) FROM workforce_task_notification_records r
				LEFT JOIN workforce_task_notification_subscriptions s
				  ON s.id=r.subscription_id AND s.tenant_id=r.tenant_id AND s.task_id=r.task_id
				 AND s.subscriber_user_id=r.subscriber_user_id
				WHERE r.subscription_id IS NOT NULL AND s.id IS NULL`,
		},
		{
			name: "notification outbox record",
			query: `SELECT count(*) FROM workforce_outbox_events e
				LEFT JOIN workforce_task_notification_records r ON r.id=e.aggregate_id
				WHERE e.aggregate_type='task_notification' AND r.id IS NULL`,
		},
	}
	for _, check := range orphanChecks {
		var count int64
		if err := q.QueryRow(ctx, check.query).Scan(&count); err != nil {
			return fmt.Errorf("verify Task notification %s relationships: %w", check.name, err)
		}
		if count != 0 {
			return fmt.Errorf("Task notification %s orphan count=%d", check.name, count)
		}
	}
	return nil
}

func ReadCatalogStats(ctx context.Context, db DB) (CatalogStats, error) {
	numeric, err := numericIdentityColumns(ctx, db)
	if err != nil {
		return CatalogStats{}, err
	}
	textColumns, err := textIdentityColumnsIncludingAllowlist(ctx, db)
	if err != nil {
		return CatalogStats{}, err
	}
	stats := CatalogStats{BigintIdentityColumns: len(numeric), TextIdentityExceptions: len(textColumns)}
	queries := []struct {
		destination *int64
		query       string
	}{
		{&stats.WorkforceTables, `SELECT count(*) FROM information_schema.tables WHERE table_schema='public' AND table_name ~ '^workforce_'`},
		{&stats.PrincipalColumns, `SELECT count(*) FROM information_schema.columns WHERE table_schema='public' AND table_name ~ '^workforce_' AND column_name ~ '_principal_id$' AND data_type='bigint'`},
		{&stats.PrincipalRows, `SELECT count(*) FROM workforce_principals`},
		{&stats.LegacyEvidenceRows, `SELECT count(*) FROM workforce_legacy_identity_map`},
		{&stats.ForeignKeys, `SELECT count(*) FROM pg_constraint WHERE contype='f' AND conrelid::regclass::text LIKE 'workforce_%'`},
	}
	for _, item := range queries {
		if err = db.QueryRow(ctx, item.query).Scan(item.destination); err != nil {
			return CatalogStats{}, err
		}
	}
	return stats, nil
}

func numericIdentityColumns(ctx context.Context, q interface {
	Query(context.Context, string, ...any) (pgx.Rows, error)
}) ([]Column, error) {
	rows, err := q.Query(ctx, `SELECT table_name,column_name,is_nullable='YES' FROM information_schema.columns WHERE table_schema='public' AND table_name ~ '^workforce_' AND data_type='bigint' AND (column_name='id' OR column_name='tenant_id' OR column_name ~ '_id$') ORDER BY table_name,column_name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var result []Column
	for rows.Next() {
		var item Column
		if err = rows.Scan(&item.Table, &item.Name, &item.Nullable); err != nil {
			return nil, err
		}
		result = append(result, item)
	}
	return result, rows.Err()
}

func legacyPrincipalColumnCount(ctx context.Context, db DB) (int, error) {
	count := 0
	for table, mappings := range principalColumns {
		for legacy := range mappings {
			var exists bool
			if err := db.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM information_schema.columns WHERE table_schema='public' AND table_name=$1 AND column_name=$2)`, table, legacy).Scan(&exists); err != nil {
				return 0, err
			}
			if exists {
				count++
			}
		}
	}
	return count, nil
}

func scopeContractDriftCount(ctx context.Context, db DB) (int, error) {
	count := 0
	for table := range systemScopeTables {
		var tableExists bool
		if err := db.QueryRow(ctx, `SELECT to_regclass(format('%I.%I', 'public', $1::text)) IS NOT NULL`, table).Scan(&tableExists); err != nil {
			return 0, err
		}
		if !tableExists {
			continue
		}
		var exists bool
		if err := db.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM information_schema.columns WHERE table_schema='public' AND table_name=$1 AND column_name='scope_type')`, table).Scan(&exists); err != nil {
			return 0, err
		}
		if !exists {
			count++
		}
	}
	return count, nil
}

func loadIdentityMap(ctx context.Context, tx pgx.Tx) (map[string]int64, error) {
	result := map[string]int64{}
	rows, err := tx.Query(ctx, `SELECT legacy_identity,entity_id FROM workforce_legacy_identity_map WHERE entity_type='GLOBAL'`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var legacy string
		var id int64
		if err = rows.Scan(&legacy, &id); err != nil {
			return nil, err
		}
		result[legacy] = id
	}
	return result, rows.Err()
}

// ResolveLegacyIdentity is intentionally read-only and exists only for local
// postflight rewrites such as the Worker YAML. Mutation APIs never accept old
// prefixed identities.
func ResolveLegacyIdentity(ctx context.Context, db DB, legacy string) (int64, error) {
	var id int64
	err := db.QueryRow(ctx, `SELECT entity_id FROM workforce_legacy_identity_map WHERE entity_type='GLOBAL' AND legacy_identity=$1`, strings.TrimSpace(legacy)).Scan(&id)
	return id, err
}

func textIdentityColumns(ctx context.Context, q interface {
	Query(context.Context, string, ...any) (pgx.Rows, error)
}) ([]Column, error) {
	rows, err := q.Query(ctx, `SELECT table_name,column_name,is_nullable='YES' FROM information_schema.columns WHERE table_schema='public' AND table_name ~ '^workforce_' AND data_type IN ('text','character varying') AND (column_name='id' OR column_name='tenant_id' OR column_name ~ '_id$') ORDER BY table_name,column_name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var result []Column
	for rows.Next() {
		var item Column
		if err = rows.Scan(&item.Table, &item.Name, &item.Nullable); err != nil {
			return nil, err
		}
		if !isGovernedTextIdentityException(item.Table + "." + item.Name) {
			result = append(result, item)
		}
	}
	return result, rows.Err()
}

func textIdentityColumnsIncludingAllowlist(ctx context.Context, q interface {
	Query(context.Context, string, ...any) (pgx.Rows, error)
}) ([]Column, error) {
	rows, err := q.Query(ctx, `SELECT table_name,column_name,is_nullable='YES' FROM information_schema.columns WHERE table_schema='public' AND table_name ~ '^workforce_' AND data_type IN ('text','character varying') AND (column_name='id' OR column_name='tenant_id' OR column_name ~ '_id$') ORDER BY table_name,column_name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var result []Column
	for rows.Next() {
		var item Column
		if err = rows.Scan(&item.Table, &item.Name, &item.Nullable); err != nil {
			return nil, err
		}
		result = append(result, item)
	}
	return result, rows.Err()
}

func preflight(ctx context.Context, tx pgx.Tx) error {
	checks := []struct{ query, label string }{
		{`SELECT count(*) FROM workforce_attempt_runtimes WHERE status IN ('STARTING','RUNNING','RECOVERING','REATTACHING')`, "active Runtime"},
		{`SELECT count(*) FROM workforce_task_attempts WHERE status IN ('CLAIMED','STARTING','RUNNING','REATTACHING')`, "active Attempt"},
		{`SELECT count(*) FROM workforce_delivery_attempts WHERE (phase IN ('PUSHING','CONFIRMING') OR push_started_at IS NOT NULL) AND status NOT IN ('SUCCEEDED','FAILED','CANCELLED','RECONCILED')`, "active Delivery push"},
	}
	for _, check := range checks {
		var count int64
		if err := tx.QueryRow(ctx, check.query).Scan(&count); err != nil {
			return fmt.Errorf("preflight %s: %w", check.label, err)
		}
		if count != 0 {
			return fmt.Errorf("preflight rejected %d %s records", count, check.label)
		}
	}
	return nil
}

func ensureSupportSchema(ctx context.Context, tx pgx.Tx) error {
	_, err := tx.Exec(ctx, `
DROP INDEX IF EXISTS workforce_materialization_identity_idx;
CREATE TABLE IF NOT EXISTS workforce_legacy_identity_map (
  entity_type TEXT NOT NULL,
  legacy_identity TEXT NOT NULL,
  entity_id BIGINT NOT NULL,
  created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
  PRIMARY KEY(entity_type,legacy_identity),
  UNIQUE(entity_type,entity_id)
);
CREATE TABLE IF NOT EXISTS workforce_principals (
  id BIGINT PRIMARY KEY,
  principal_type TEXT NOT NULL,
  user_id BIGINT,
  worker_id BIGINT,
  service_code TEXT,
  created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP
);
DO $$ BEGIN
  IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conrelid='workforce_principals'::regclass AND conname='workforce_principals_type_ck') THEN
    ALTER TABLE workforce_principals ADD CONSTRAINT workforce_principals_type_ck CHECK (principal_type IN ('USER','WORKER','SERVICE'));
  END IF;
  IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conrelid='workforce_principals'::regclass AND conname='workforce_principals_shape_ck') THEN
    ALTER TABLE workforce_principals ADD CONSTRAINT workforce_principals_shape_ck CHECK (
      (principal_type='USER' AND user_id IS NOT NULL AND worker_id IS NULL AND service_code IS NULL) OR
      (principal_type='WORKER' AND user_id IS NULL AND worker_id IS NOT NULL AND service_code IS NULL) OR
      (principal_type='SERVICE' AND user_id IS NULL AND worker_id IS NULL AND service_code IS NOT NULL AND service_code<>'')
    );
  END IF;
END $$;
CREATE UNIQUE INDEX IF NOT EXISTS workforce_principals_user_uidx ON workforce_principals(user_id) WHERE principal_type='USER';
CREATE UNIQUE INDEX IF NOT EXISTS workforce_principals_worker_uidx ON workforce_principals(worker_id) WHERE principal_type='WORKER';
CREATE UNIQUE INDEX IF NOT EXISTS workforce_principals_service_uidx ON workforce_principals(service_code) WHERE principal_type='SERVICE';`)
	return err
}

// ensureTaskNotificationSchema is the additive, idempotent local alignment for
// the unpublished Gate 0-1 capability. Fresh installations receive the same
// shape from the existing Workforce migration; this path preserves all local
// Task and evidence rows and never introduces database foreign keys.
func ensureTaskNotificationSchema(ctx context.Context, tx pgx.Tx, idGen IDGenerator) error {
	_, err := tx.Exec(ctx, `
CREATE TABLE IF NOT EXISTS workforce_task_notification_subscriptions (
  id BIGINT PRIMARY KEY,
  tenant_id BIGINT NOT NULL,
  task_id BIGINT NOT NULL,
  subscriber_user_id BIGINT NOT NULL,
  events_json JSONB NOT NULL CHECK (jsonb_typeof(events_json)='array'),
  enabled BOOLEAN NOT NULL DEFAULT TRUE,
  version INTEGER NOT NULL DEFAULT 1 CHECK (version>0),
  created_by BIGINT NOT NULL,
  updated_by BIGINT NOT NULL,
  created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
  updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
  UNIQUE (tenant_id,task_id,subscriber_user_id)
);
CREATE TABLE IF NOT EXISTS workforce_task_notification_records (
  id BIGINT PRIMARY KEY,
  tenant_id BIGINT NOT NULL,
  task_id BIGINT NOT NULL,
  task_turn_id BIGINT NOT NULL,
  subscription_id BIGINT,
  subscriber_user_id BIGINT NOT NULL,
  subscription_version INTEGER CHECK (subscription_version>0),
  fact_type TEXT NOT NULL,
  source_type TEXT NOT NULL,
  source_id BIGINT NOT NULL,
  source_occurrence BIGINT NOT NULL DEFAULT 0 CHECK (source_occurrence>=0),
  fact_identity_version INTEGER NOT NULL DEFAULT 1 CHECK (fact_identity_version IN (1,2)),
  reason_code VARCHAR(128) NOT NULL DEFAULT '',
  focus TEXT NOT NULL CHECK (focus IN ('evaluation','result','continuation','delivery')),
  task_name_snapshot TEXT NOT NULL,
  turn_number INTEGER NOT NULL CHECK (turn_number>0),
  summary_snapshot TEXT NOT NULL DEFAULT '',
  title_snapshot TEXT NOT NULL,
  content_snapshot TEXT NOT NULL,
  publication_contract_version INTEGER NOT NULL DEFAULT 1 CHECK (publication_contract_version IN (1,2)),
  scene_code TEXT NOT NULL DEFAULT '',
  template_variables_json JSONB NOT NULL DEFAULT '{}'::jsonb,
  deep_link TEXT NOT NULL,
  idempotency_identity TEXT NOT NULL,
  request_digest TEXT NOT NULL,
  status TEXT NOT NULL DEFAULT 'PENDING' CHECK (status IN ('PENDING','PROCESSING','RETRY_WAIT','PUBLISHED','SUPPRESSED','FAILED')),
  notification_receipt_identity TEXT NOT NULL DEFAULT '',
  warning_code TEXT NOT NULL DEFAULT '',
  last_error_code VARCHAR(128) NOT NULL DEFAULT '',
  publish_attempts INTEGER NOT NULL DEFAULT 0 CHECK (publish_attempts>=0),
  next_retry_at TIMESTAMPTZ,
  lease_token TEXT NOT NULL DEFAULT '',
  lease_until TIMESTAMPTZ,
  occurred_at TIMESTAMPTZ NOT NULL,
  published_at TIMESTAMPTZ,
  created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
  updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
  UNIQUE (tenant_id,task_id,task_turn_id,fact_type,source_type,source_id,source_occurrence,subscriber_user_id),
  CHECK (fact_type IN ('TASK_ACTION_REQUIRED','TASK_INTERRUPTED','TASK_COMPLETED')),
  CHECK (source_type IN ('CONTINUATION_CHECKPOINT','FINALIZATION_RUN','ARTIFACT_UPLOAD','RESULT_EVALUATION','TASK_ATTEMPT','DELIVERY_REQUEST','REVIEW_OPERATION','INDEPENDENT_REVIEW_RUN','RUNTIME_RECOVERY','TASK_TIMEOUT','TASK_CANCEL')),
  CHECK (status='SUPPRESSED' OR (subscription_id IS NOT NULL AND subscription_version IS NOT NULL))
);
CREATE TABLE IF NOT EXISTS workforce_task_notification_subscription_channels (
  id BIGINT PRIMARY KEY, tenant_id BIGINT NOT NULL, subscription_id BIGINT NOT NULL,
  channel_kind VARCHAR(32) NOT NULL CHECK (channel_kind IN ('IN_APP','EMAIL_USER','FEISHU_DIRECT','WECOM_DIRECT','GROUP_ROUTE')),
  group_route_id BIGINT, created_by BIGINT NOT NULL, created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
  UNIQUE (subscription_id,channel_kind,group_route_id),
  CHECK ((channel_kind='GROUP_ROUTE' AND group_route_id IS NOT NULL) OR (channel_kind<>'GROUP_ROUTE' AND group_route_id IS NULL))
);
CREATE TABLE IF NOT EXISTS workforce_task_notification_record_channels (
  id BIGINT PRIMARY KEY, tenant_id BIGINT NOT NULL, record_id BIGINT NOT NULL,
  channel_kind VARCHAR(32) NOT NULL CHECK (channel_kind IN ('IN_APP','EMAIL_USER','FEISHU_DIRECT','WECOM_DIRECT','GROUP_ROUTE')),
  group_route_id BIGINT, mention_user_id BIGINT, created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
  UNIQUE (record_id,channel_kind,group_route_id),
  CHECK ((channel_kind='GROUP_ROUTE' AND group_route_id IS NOT NULL) OR (channel_kind<>'GROUP_ROUTE' AND group_route_id IS NULL))
);
CREATE TABLE IF NOT EXISTS workforce_task_notification_record_recipients (
  id BIGINT PRIMARY KEY, tenant_id BIGINT NOT NULL, record_id BIGINT NOT NULL, user_id BIGINT NOT NULL,
  recipient_role TEXT NOT NULL CHECK (recipient_role IN ('SUBSCRIBER','ATTENTION')),
  personal_channel_policy TEXT NOT NULL CHECK (personal_channel_policy IN ('TASK_SUBSCRIPTION','USER_PREFERENCE')),
  in_app_required BOOLEAN NOT NULL DEFAULT FALSE, created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
  UNIQUE (record_id,user_id)
);
CREATE TABLE IF NOT EXISTS sys_notification_user_preferences (
  id BIGINT PRIMARY KEY, organization_id BIGINT NOT NULL, user_id BIGINT NOT NULL,
  event_family TEXT NOT NULL CHECK (event_family IN ('TASK_ACTION_REQUIRED','TASK_INTERRUPTED','TASK_COMPLETED')),
  channel_kind TEXT NOT NULL CHECK (channel_kind IN ('IN_APP','EMAIL_USER','FEISHU_DIRECT','WECOM_DIRECT')),
  enabled BOOLEAN NOT NULL DEFAULT TRUE, version BIGINT NOT NULL DEFAULT 1 CHECK(version>0),
  creator_id BIGINT NOT NULL, updater_id BIGINT NOT NULL, created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
  updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP, UNIQUE(organization_id,user_id,event_family,channel_kind)
);
CREATE UNIQUE INDEX IF NOT EXISTS workforce_task_notification_subscription_channel_direct_uidx ON workforce_task_notification_subscription_channels(subscription_id,channel_kind) WHERE group_route_id IS NULL;
CREATE UNIQUE INDEX IF NOT EXISTS workforce_task_notification_record_channel_direct_uidx ON workforce_task_notification_record_channels(record_id,channel_kind) WHERE group_route_id IS NULL;
ALTER TABLE workforce_task_notification_records
  ADD COLUMN IF NOT EXISTS publication_contract_version INTEGER NOT NULL DEFAULT 1,
  ADD COLUMN IF NOT EXISTS scene_code TEXT NOT NULL DEFAULT '',
  ADD COLUMN IF NOT EXISTS template_variables_json JSONB NOT NULL DEFAULT '{}'::jsonb,
  ADD COLUMN IF NOT EXISTS source_occurrence BIGINT NOT NULL DEFAULT 0,
  ADD COLUMN IF NOT EXISTS fact_identity_version INTEGER NOT NULL DEFAULT 1;
DO $$ DECLARE c RECORD; BEGIN
  FOR c IN SELECT conname FROM pg_constraint WHERE conrelid='workforce_task_notification_records'::regclass AND contype='u' AND pg_get_constraintdef(oid)='UNIQUE (tenant_id, task_id, task_turn_id, fact_type, source_type, source_id, subscriber_user_id)'
  LOOP EXECUTE format('ALTER TABLE workforce_task_notification_records DROP CONSTRAINT %I',c.conname); END LOOP;
END $$;
DO $$ DECLARE c RECORD; has_occurrence BOOLEAN := FALSE; has_identity_version BOOLEAN := FALSE; BEGIN
  FOR c IN SELECT conname,pg_get_constraintdef(oid) AS definition FROM pg_constraint WHERE conrelid='workforce_task_notification_records'::regclass AND contype='c'
  LOOP
    IF c.definition LIKE '%source_type%' OR c.definition LIKE '%fact_type%' THEN
      EXECUTE format('ALTER TABLE workforce_task_notification_records DROP CONSTRAINT %I',c.conname);
    ELSIF c.conname='workforce_task_notification_records_source_occurrence_check' THEN
      has_occurrence := TRUE;
    ELSIF c.conname='workforce_task_notification_records_fact_identity_version_check' THEN
      has_identity_version := TRUE;
    END IF;
  END LOOP;
  ALTER TABLE workforce_task_notification_records ADD CONSTRAINT workforce_task_notification_records_fact_type_check CHECK (fact_type IN ('TASK_ACTION_REQUIRED','TASK_INTERRUPTED','TASK_COMPLETED','DELIVERY_COMPLETED'));
  ALTER TABLE workforce_task_notification_records ADD CONSTRAINT workforce_task_notification_records_source_type_check CHECK (source_type IN ('CONTINUATION_CHECKPOINT','FINALIZATION_RUN','ARTIFACT_UPLOAD','RESULT_EVALUATION','TASK_ATTEMPT','DELIVERY_REQUEST','REVIEW_OPERATION','INDEPENDENT_REVIEW_RUN','RUNTIME_RECOVERY','TASK_TIMEOUT','TASK_CANCEL'));
  IF NOT has_occurrence THEN ALTER TABLE workforce_task_notification_records ADD CONSTRAINT workforce_task_notification_records_source_occurrence_check CHECK (source_occurrence>=0); END IF;
  IF NOT has_identity_version THEN ALTER TABLE workforce_task_notification_records ADD CONSTRAINT workforce_task_notification_records_fact_identity_version_check CHECK (fact_identity_version IN (1,2)); END IF;
END $$;
CREATE UNIQUE INDEX IF NOT EXISTS workforce_task_notification_records_fact_v2_uidx ON workforce_task_notification_records(tenant_id,task_id,task_turn_id,fact_type,source_type,source_id,source_occurrence,subscriber_user_id);
CREATE INDEX IF NOT EXISTS workforce_task_notification_subscriptions_lookup_idx
  ON workforce_task_notification_subscriptions(tenant_id,task_id,enabled);
CREATE INDEX IF NOT EXISTS workforce_task_notification_records_task_idx
  ON workforce_task_notification_records(tenant_id,task_id,occurred_at DESC,id DESC);
CREATE INDEX IF NOT EXISTS workforce_task_notification_records_dispatch_idx
  ON workforce_task_notification_records(status,next_retry_at,lease_until,occurred_at)
	WHERE status IN ('PENDING','PROCESSING','RETRY_WAIT');
CREATE INDEX IF NOT EXISTS workforce_task_notification_subscription_channels_route_idx ON workforce_task_notification_subscription_channels(tenant_id,group_route_id) WHERE group_route_id IS NOT NULL;
CREATE INDEX IF NOT EXISTS workforce_task_notification_record_channels_route_idx ON workforce_task_notification_record_channels(tenant_id,group_route_id) WHERE group_route_id IS NOT NULL;`)
	if err != nil {
		return fmt.Errorf("ensure Task notification schema: %w", err)
	}
	var hasLegacyChannels bool
	if err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM information_schema.columns WHERE table_schema='public' AND table_name='workforce_task_notification_subscriptions' AND column_name='channels_json')`).Scan(&hasLegacyChannels); err != nil {
		return err
	}
	if hasLegacyChannels {
		rows, queryErr := tx.Query(ctx, `SELECT id,tenant_id,created_by,channels_json::text FROM workforce_task_notification_subscriptions ORDER BY id`)
		if queryErr != nil {
			return queryErr
		}
		type legacyChannel struct {
			Type    string `json:"type"`
			Kind    string `json:"kind"`
			RouteID string `json:"routeId"`
		}
		type legacyRow struct {
			id, tenant, actor int64
			raw               string
		}
		var legacy []legacyRow
		for rows.Next() {
			var x legacyRow
			if scanErr := rows.Scan(&x.id, &x.tenant, &x.actor, &x.raw); scanErr != nil {
				rows.Close()
				return scanErr
			}
			legacy = append(legacy, x)
		}
		if rowsErr := rows.Err(); rowsErr != nil {
			rows.Close()
			return rowsErr
		}
		rows.Close()
		for _, row := range legacy {
			var channels []legacyChannel
			if decodeErr := json.Unmarshal([]byte(row.raw), &channels); decodeErr != nil {
				return fmt.Errorf("decode legacy Task notification channels: %w", decodeErr)
			}
			if len(channels) == 0 {
				channels = []legacyChannel{{Kind: "IN_APP"}}
			}
			for _, channel := range channels {
				kind := strings.ToUpper(strings.TrimSpace(channel.Kind))
				if kind == "" {
					kind = strings.ToUpper(strings.TrimSpace(channel.Type))
				}
				routeID := int64(0)
				if strings.TrimSpace(channel.RouteID) != "" {
					routeID, _ = strconv.ParseInt(strings.TrimSpace(channel.RouteID), 10, 64)
				}
				if _, insertErr := tx.Exec(ctx, `INSERT INTO workforce_task_notification_subscription_channels (id,tenant_id,subscription_id,channel_kind,group_route_id,created_by) VALUES ($1,$2,$3,$4,NULLIF($5::BIGINT,0),$6) ON CONFLICT DO NOTHING`, idGen.NextID(), row.tenant, row.id, kind, routeID, row.actor); insertErr != nil {
					return insertErr
				}
			}
		}
		if _, err = tx.Exec(ctx, `ALTER TABLE workforce_task_notification_subscriptions DROP COLUMN channels_json`); err != nil {
			return fmt.Errorf("drop legacy Task notification channels JSON: %w", err)
		}
	}
	// Historical local Tasks predate the Gate 1 create-time default. Backfill a
	// single creator-owned in-app subscription so read/update APIs stay total
	// without mutating Task execution, result, checkpoint or delivery identity.
	taskRows, taskErr := tx.Query(ctx, `SELECT t.id,t.tenant_id,t.created_by FROM workforce_tasks t WHERE t.created_by>0 AND NOT EXISTS (SELECT 1 FROM workforce_task_notification_subscriptions s WHERE s.tenant_id=t.tenant_id AND s.task_id=t.id AND s.subscriber_user_id=t.created_by) ORDER BY t.id`)
	if taskErr != nil {
		return taskErr
	}
	type subscriptionBackfill struct{ taskID, tenantID, userID int64 }
	var missingSubscriptions []subscriptionBackfill
	for taskRows.Next() {
		var item subscriptionBackfill
		if scanErr := taskRows.Scan(&item.taskID, &item.tenantID, &item.userID); scanErr != nil {
			taskRows.Close()
			return scanErr
		}
		missingSubscriptions = append(missingSubscriptions, item)
	}
	if rowsErr := taskRows.Err(); rowsErr != nil {
		taskRows.Close()
		return rowsErr
	}
	taskRows.Close()
	for _, item := range missingSubscriptions {
		subscriptionID := idGen.NextID()
		command, insertErr := tx.Exec(ctx, `INSERT INTO workforce_task_notification_subscriptions (id,tenant_id,task_id,subscriber_user_id,events_json,enabled,version,created_by,updated_by) VALUES ($1,$2,$3,$4,'["TASK_ACTION_REQUIRED","TASK_INTERRUPTED","TASK_COMPLETED"]'::jsonb,TRUE,1,$4,$4) ON CONFLICT DO NOTHING`, subscriptionID, item.tenantID, item.taskID, item.userID)
		if insertErr != nil {
			return insertErr
		}
		if command.RowsAffected() == 1 {
			if _, insertErr = tx.Exec(ctx, `INSERT INTO workforce_task_notification_subscription_channels (id,tenant_id,subscription_id,channel_kind,group_route_id,created_by) VALUES ($1,$2,$3,'IN_APP',NULL,$4)`, idGen.NextID(), item.tenantID, subscriptionID, item.userID); insertErr != nil {
				return insertErr
			}
		}
	}
	// Freeze existing pending/history records from the subscription version
	// they already reference. This does not reinterpret notification payloads.
	recordRows, queryErr := tx.Query(ctx, `SELECT r.id,r.tenant_id,r.fact_type,r.subscriber_user_id,c.channel_kind,COALESCE(c.group_route_id,0) FROM workforce_task_notification_records r JOIN workforce_task_notification_subscription_channels c ON c.subscription_id=r.subscription_id WHERE NOT EXISTS (SELECT 1 FROM workforce_task_notification_record_channels rc WHERE rc.record_id=r.id) ORDER BY r.id,c.id`)
	if queryErr != nil {
		return queryErr
	}
	type recordChannelBackfill struct {
		recordID, tenantID, userID, routeID int64
		fact, kind                          string
	}
	var backfill []recordChannelBackfill
	for recordRows.Next() {
		var item recordChannelBackfill
		if scanErr := recordRows.Scan(&item.recordID, &item.tenantID, &item.fact, &item.userID, &item.kind, &item.routeID); scanErr != nil {
			recordRows.Close()
			return scanErr
		}
		backfill = append(backfill, item)
	}
	if rowsErr := recordRows.Err(); rowsErr != nil {
		recordRows.Close()
		return rowsErr
	}
	recordRows.Close()
	for _, item := range backfill {
		mentionID := int64(0)
		if item.kind == "GROUP_ROUTE" && (item.fact == "TASK_ACTION_REQUIRED" || item.fact == "TASK_INTERRUPTED") {
			mentionID = item.userID
		}
		if _, insertErr := tx.Exec(ctx, `INSERT INTO workforce_task_notification_record_channels (id,tenant_id,record_id,channel_kind,group_route_id,mention_user_id) VALUES ($1,$2,$3,$4,NULLIF($5::BIGINT,0),NULLIF($6::BIGINT,0)) ON CONFLICT DO NOTHING`, idGen.NextID(), item.tenantID, item.recordID, item.kind, item.routeID, mentionID); insertErr != nil {
			return insertErr
		}
	}
	recipientRows, recipientErr := tx.Query(ctx, `SELECT id,tenant_id,subscriber_user_id FROM workforce_task_notification_records r WHERE NOT EXISTS (SELECT 1 FROM workforce_task_notification_record_recipients rr WHERE rr.record_id=r.id) ORDER BY id`)
	if recipientErr != nil {
		return recipientErr
	}
	type recipientBackfill struct{ recordID, tenantID, userID int64 }
	var recipients []recipientBackfill
	for recipientRows.Next() {
		var item recipientBackfill
		if scanErr := recipientRows.Scan(&item.recordID, &item.tenantID, &item.userID); scanErr != nil {
			recipientRows.Close()
			return scanErr
		}
		recipients = append(recipients, item)
	}
	if rowsErr := recipientRows.Err(); rowsErr != nil {
		recipientRows.Close()
		return rowsErr
	}
	recipientRows.Close()
	for _, item := range recipients {
		if _, insertErr := tx.Exec(ctx, `INSERT INTO workforce_task_notification_record_recipients (id,tenant_id,record_id,user_id,recipient_role,personal_channel_policy,in_app_required) VALUES ($1,$2,$3,$4,'SUBSCRIBER','TASK_SUBSCRIPTION',FALSE) ON CONFLICT DO NOTHING`, idGen.NextID(), item.tenantID, item.recordID, item.userID); insertErr != nil {
			return insertErr
		}
	}
	return nil
}

// ensureNotificationActionableSchema converges the existing local Framework
// tables to the unpublished Gate 2 shape. It is additive, preserves all
// historical bodies and delivery outcomes, and intentionally creates no
// foreign keys.
func ensureNotificationActionableSchema(ctx context.Context, tx pgx.Tx) error {
	_, err := tx.Exec(ctx, `
ALTER TABLE sys_notification_template_revision
  ADD COLUMN IF NOT EXISTS "primaryActionLabelTemplate" VARCHAR(128) NOT NULL DEFAULT '';
ALTER TABLE sys_notification
  ADD COLUMN IF NOT EXISTS "actionType" VARCHAR(32) NOT NULL DEFAULT '',
  ADD COLUMN IF NOT EXISTS "actionLabelSnapshot" VARCHAR(128) NOT NULL DEFAULT '',
  ADD COLUMN IF NOT EXISTS "actionStyle" VARCHAR(32) NOT NULL DEFAULT '';
ALTER TABLE sys_notification_recipient
  ADD COLUMN IF NOT EXISTS "actionType" VARCHAR(32) NOT NULL DEFAULT '',
  ADD COLUMN IF NOT EXISTS "actionLabelSnapshot" VARCHAR(128) NOT NULL DEFAULT '',
  ADD COLUMN IF NOT EXISTS "actionStyle" VARCHAR(32) NOT NULL DEFAULT '';
ALTER TABLE sys_notification_delivery
  ADD COLUMN IF NOT EXISTS "presentationType" VARCHAR(32) NOT NULL DEFAULT 'TEXT',
  ADD COLUMN IF NOT EXISTS "actionLabelSnapshot" VARCHAR(128) NOT NULL DEFAULT '',
  ADD COLUMN IF NOT EXISTS "actionUrlSnapshot" VARCHAR(1024) NOT NULL DEFAULT '',
  ADD COLUMN IF NOT EXISTS "renderedPayloadDigest" CHAR(64) NOT NULL DEFAULT '';
ALTER TABLE sys_user
  ADD COLUMN IF NOT EXISTS email_verified_at TIMESTAMPTZ,
  ADD COLUMN IF NOT EXISTS email_version BIGINT NOT NULL DEFAULT 1,
  ADD COLUMN IF NOT EXISTS email_verification_source VARCHAR(64) NOT NULL DEFAULT '';
DO $$ BEGIN
  IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conrelid='sys_user'::regclass AND conname='sys_user_email_version_ck') THEN
    ALTER TABLE sys_user ADD CONSTRAINT sys_user_email_version_ck CHECK (email_version>0);
  END IF;
END $$;
CREATE INDEX IF NOT EXISTS sys_user_verified_email_idx
  ON sys_user (lower("userEmail"),email_verified_at)
  WHERE "isDeleted"=false AND status=0 AND email_verified_at IS NOT NULL;
CREATE TABLE IF NOT EXISTS sys_notification_group_route (
  id BIGINT PRIMARY KEY, organization_id BIGINT NOT NULL, display_name TEXT NOT NULL,
  provider_type TEXT NOT NULL CHECK (provider_type IN ('FEISHU','WECOM')),
  transport_type TEXT NOT NULL CHECK (transport_type IN ('FEISHU_APP_CHAT','FEISHU_WEBHOOK','WECOM_WEBHOOK')),
  delivery_channel_id BIGINT NOT NULL, required_permission_code TEXT NOT NULL DEFAULT 'workforce:task:create',
  target_ciphertext TEXT NOT NULL DEFAULT '', target_edek TEXT NOT NULL DEFAULT '', target_wrap_key_ref TEXT NOT NULL DEFAULT '', target_digest TEXT NOT NULL DEFAULT '',
  status TEXT NOT NULL DEFAULT 'DRAFT' CHECK (status IN ('DRAFT','VERIFIED','ENABLED','DISABLED','RETIRED')),
  route_version BIGINT NOT NULL DEFAULT 1 CHECK (route_version>0), channel_version BIGINT NOT NULL DEFAULT 1 CHECK (channel_version>0), connection_version BIGINT,
  config_digest TEXT NOT NULL, capabilities_json JSONB NOT NULL DEFAULT '[]'::jsonb, last_probe_status TEXT NOT NULL DEFAULT 'NOT_PROBED', last_probe_reason_code TEXT NOT NULL DEFAULT '', last_probe_at TIMESTAMPTZ,
  creator_id BIGINT NOT NULL, updater_id BIGINT NOT NULL, created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP, updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP
);
CREATE UNIQUE INDEX IF NOT EXISTS sys_notification_group_route_name_uidx ON sys_notification_group_route (organization_id,lower(display_name)) WHERE status<>'RETIRED';
CREATE INDEX IF NOT EXISTS sys_notification_group_route_available_idx ON sys_notification_group_route (organization_id,status,provider_type,display_name);
CREATE TABLE IF NOT EXISTS sys_notification_group_route_candidate (
  id BIGINT PRIMARY KEY,token_hash TEXT NOT NULL UNIQUE,organization_id BIGINT NOT NULL,connection_id BIGINT NOT NULL,connection_version BIGINT NOT NULL,connection_config_digest TEXT NOT NULL,
  display_name TEXT NOT NULL,description TEXT NOT NULL DEFAULT '',target_ciphertext TEXT NOT NULL,target_edek TEXT NOT NULL,target_wrap_key_ref TEXT NOT NULL,target_digest TEXT NOT NULL,
  expires_at TIMESTAMPTZ NOT NULL,consumed_at TIMESTAMPTZ,created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP
);
CREATE INDEX IF NOT EXISTS sys_notification_group_route_candidate_expiry_idx ON sys_notification_group_route_candidate(organization_id,expires_at);
CREATE TABLE IF NOT EXISTS sys_notification_group_route_snapshot (
  id BIGINT PRIMARY KEY, notification_id TEXT NOT NULL, delivery_id TEXT NOT NULL UNIQUE, organization_id BIGINT NOT NULL, route_id BIGINT NOT NULL, route_version BIGINT NOT NULL,
  provider_type TEXT NOT NULL, transport_type TEXT NOT NULL, route_config_digest TEXT NOT NULL, delivery_channel_id BIGINT NOT NULL, channel_version BIGINT NOT NULL, enterprise_connection_version BIGINT,
  target_ciphertext TEXT NOT NULL DEFAULT '',target_edek TEXT NOT NULL DEFAULT '',target_wrap_key_ref TEXT NOT NULL DEFAULT '',target_digest TEXT NOT NULL DEFAULT '',presentation TEXT NOT NULL,action_url_snapshot TEXT NOT NULL DEFAULT '',
  mention_requested BOOLEAN NOT NULL DEFAULT FALSE,mention_user_id BIGINT,mention_identity_id BIGINT,mention_identity_version BIGINT,mention_authority_digest TEXT NOT NULL DEFAULT '',warning_code TEXT NOT NULL DEFAULT '',rendered_payload_digest TEXT NOT NULL DEFAULT '',created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP
);
CREATE INDEX IF NOT EXISTS sys_notification_group_route_snapshot_route_idx ON sys_notification_group_route_snapshot(organization_id,route_id,created_at DESC);
`)
	if err != nil {
		return fmt.Errorf("ensure actionable notification schema: %w", err)
	}
	return nil
}

func verifyNotificationActionableSchemaShape(ctx context.Context, q interface {
	QueryRow(context.Context, string, ...any) pgx.Row
}) error {
	var drift int
	err := q.QueryRow(ctx, `WITH expected(table_name,column_name,data_type,is_nullable) AS (VALUES
		('sys_notification_template_revision','primaryActionLabelTemplate','character varying','NO'),
		('sys_notification','actionType','character varying','NO'),
		('sys_notification','actionLabelSnapshot','character varying','NO'),
		('sys_notification','actionStyle','character varying','NO'),
		('sys_notification_recipient','actionType','character varying','NO'),
		('sys_notification_recipient','actionLabelSnapshot','character varying','NO'),
		('sys_notification_recipient','actionStyle','character varying','NO'),
		('sys_notification_delivery','presentationType','character varying','NO'),
		('sys_notification_delivery','actionLabelSnapshot','character varying','NO'),
		('sys_notification_delivery','actionUrlSnapshot','character varying','NO'),
		('sys_notification_delivery','renderedPayloadDigest','character','NO'),
		('sys_user','email_verified_at','timestamp with time zone','YES'),
		('sys_user','email_version','bigint','NO'),
		('sys_user','email_verification_source','character varying','NO')
	)
	SELECT count(*) FROM expected e
	LEFT JOIN information_schema.columns c ON c.table_schema='public' AND c.table_name=e.table_name AND c.column_name=e.column_name
	WHERE c.column_name IS NULL OR c.data_type<>e.data_type OR c.is_nullable<>e.is_nullable`).Scan(&drift)
	if err != nil {
		return err
	}
	if drift != 0 {
		return fmt.Errorf("actionable notification schema has %d missing or incompatible columns", drift)
	}
	return nil
}

// ensureEnterpriseIdentitySchema converges a post-BIGINT local database to the
// unpublished Gate 3 shape. It is deliberately additive: encryption of legacy
// generic OAuth subjects requires cryptographic dependencies and is performed
// by the dedicated secure subject rewrite, never by inventing placeholder
// identities in SQL.
func ensureEnterpriseIdentitySchema(ctx context.Context, tx pgx.Tx) error {
	_, err := tx.Exec(ctx, `
ALTER TABLE sys_external_user_identity
  ALTER COLUMN "externalSubject" DROP NOT NULL,
  ADD COLUMN IF NOT EXISTS scope_type VARCHAR(32) NOT NULL DEFAULT 'SYSTEM',
  ADD COLUMN IF NOT EXISTS organization_id BIGINT NULL,
  ADD COLUMN IF NOT EXISTS enterprise_connection_id BIGINT NULL,
  ADD COLUMN IF NOT EXISTS subject_kind VARCHAR(64) NULL,
  ADD COLUMN IF NOT EXISTS subject_ciphertext TEXT NULL,
  ADD COLUMN IF NOT EXISTS subject_edek TEXT NULL,
  ADD COLUMN IF NOT EXISTS subject_wrap_key_ref VARCHAR(128) NULL,
  ADD COLUMN IF NOT EXISTS subject_digest VARCHAR(128) NULL,
  ADD COLUMN IF NOT EXISTS subject_digest_key_ref VARCHAR(128) NULL,
  ADD COLUMN IF NOT EXISTS display_hint VARCHAR(128) NULL,
  ADD COLUMN IF NOT EXISTS identity_version BIGINT NOT NULL DEFAULT 1,
  ADD COLUMN IF NOT EXISTS verified_at TIMESTAMPTZ NULL,
  ADD COLUMN IF NOT EXISTS revoked_at TIMESTAMPTZ NULL;
DO $$ BEGIN
  IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conrelid='sys_external_user_identity'::regclass AND conname='chk_external_identity_scope') THEN
    ALTER TABLE sys_external_user_identity ADD CONSTRAINT chk_external_identity_scope CHECK (
      (scope_type='SYSTEM' AND organization_id IS NULL AND enterprise_connection_id IS NULL) OR
      (scope_type='ORGANIZATION' AND organization_id IS NOT NULL AND enterprise_connection_id IS NOT NULL)
    );
  END IF;
END $$;
CREATE UNIQUE INDEX IF NOT EXISTS uk_external_identity_enterprise_subject_active
  ON sys_external_user_identity(organization_id,enterprise_connection_id,subject_digest)
  WHERE scope_type='ORGANIZATION' AND status=0 AND "isDeleted"=0;
CREATE UNIQUE INDEX IF NOT EXISTS uk_external_identity_enterprise_user_active
  ON sys_external_user_identity(organization_id,enterprise_connection_id,"userId")
  WHERE scope_type='ORGANIZATION' AND status=0 AND "isDeleted"=0;
CREATE UNIQUE INDEX IF NOT EXISTS uk_external_identity_system_subject_active
  ON sys_external_user_identity("providerCode",COALESCE("externalIssuer",''),subject_digest)
  WHERE scope_type='SYSTEM' AND status=0 AND "isDeleted"=0;

CREATE TABLE IF NOT EXISTS sys_enterprise_identity_connection (
  id BIGINT PRIMARY KEY,
  organization_id BIGINT NOT NULL,
  provider_type VARCHAR(32) NOT NULL,
  connection_role VARCHAR(32) NOT NULL DEFAULT 'PRIMARY',
  display_name VARCHAR(128) NOT NULL,
  app_id VARCHAR(255), corp_id VARCHAR(255), agent_id VARCHAR(255),
  endpoint_profile VARCHAR(32) NOT NULL DEFAULT 'OFFICIAL_CN',
  client_secret_ciphertext TEXT NOT NULL,
  client_secret_edek TEXT NOT NULL,
  client_secret_wrap_key_ref VARCHAR(128) NOT NULL,
  credential_version BIGINT NOT NULL DEFAULT 1,
  binding_enabled BOOLEAN NOT NULL DEFAULT TRUE,
  notification_direct_enabled BOOLEAN NOT NULL DEFAULT TRUE,
  notification_mention_enabled BOOLEAN NOT NULL DEFAULT TRUE,
  login_enabled BOOLEAN NOT NULL DEFAULT FALSE,
  protocol_capabilities_json JSONB NOT NULL DEFAULT '{}'::jsonb,
  config_digest VARCHAR(128) NOT NULL,
  status VARCHAR(32) NOT NULL DEFAULT 'ACTIVE',
  last_probe_status VARCHAR(32) NOT NULL DEFAULT 'NOT_PROBED',
  last_probe_reason_code VARCHAR(128) NOT NULL DEFAULT '',
  last_probe_at TIMESTAMPTZ,
  version BIGINT NOT NULL DEFAULT 1,
  creator_id BIGINT NOT NULL, updater_id BIGINT NOT NULL,
  created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
  updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
  CONSTRAINT chk_enterprise_connection_provider CHECK (provider_type IN ('FEISHU','WECOM')),
  CONSTRAINT chk_enterprise_connection_role CHECK (connection_role='PRIMARY'),
  CONSTRAINT chk_enterprise_connection_status CHECK (status IN ('ACTIVE','DISABLED')),
  CONSTRAINT chk_enterprise_connection_positive_ids CHECK (id>0 AND organization_id>0 AND creator_id>0 AND updater_id>0)
);
CREATE UNIQUE INDEX IF NOT EXISTS uk_enterprise_connection_primary ON sys_enterprise_identity_connection(organization_id,provider_type) WHERE connection_role='PRIMARY';
CREATE INDEX IF NOT EXISTS idx_enterprise_connection_status ON sys_enterprise_identity_connection(organization_id,status,provider_type);

CREATE TABLE IF NOT EXISTS sys_enterprise_identity_grant (
  id BIGINT PRIMARY KEY, organization_id BIGINT NOT NULL, external_identity_id BIGINT NOT NULL,
  grant_type VARCHAR(64) NOT NULL, status VARCHAR(32) NOT NULL DEFAULT 'ACTIVE', version BIGINT NOT NULL DEFAULT 1,
  granted_by BIGINT NOT NULL, granted_at TIMESTAMPTZ NOT NULL, revoked_by BIGINT, revoked_at TIMESTAMPTZ,
  reason_code VARCHAR(128) NOT NULL DEFAULT '', created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
  updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
  CONSTRAINT chk_enterprise_grant_type CHECK (grant_type IN ('IDENTITY_LINK','NOTIFICATION_DIRECT','NOTIFICATION_MENTION','LOGIN')),
  CONSTRAINT chk_enterprise_grant_status CHECK (status IN ('ACTIVE','REVOKED')),
  CONSTRAINT chk_enterprise_grant_positive_ids CHECK (id>0 AND organization_id>0 AND external_identity_id>0 AND granted_by>0)
);
CREATE UNIQUE INDEX IF NOT EXISTS uk_enterprise_identity_grant ON sys_enterprise_identity_grant(organization_id,external_identity_id,grant_type);
CREATE INDEX IF NOT EXISTS idx_enterprise_identity_grant_active ON sys_enterprise_identity_grant(organization_id,grant_type,status,external_identity_id);

CREATE TABLE IF NOT EXISTS sys_enterprise_identity_authorization (
  id BIGINT PRIMARY KEY, authorization_identity VARCHAR(128) NOT NULL, purpose VARCHAR(32) NOT NULL,
  organization_id BIGINT NOT NULL, connection_id BIGINT NOT NULL, connection_version BIGINT NOT NULL,
  connection_config_digest VARCHAR(128) NOT NULL, bind_user_id BIGINT,
  requested_grants_json JSONB NOT NULL DEFAULT '[]'::jsonb, requested_grants_digest VARCHAR(128) NOT NULL,
  state_hash VARCHAR(128) NOT NULL, nonce_hash VARCHAR(128) NOT NULL DEFAULT '',
  code_verifier_ciphertext TEXT, code_verifier_edek TEXT, code_verifier_wrap_key_ref VARCHAR(128),
  redirect_after VARCHAR(1024) NOT NULL DEFAULT '', expires_at TIMESTAMPTZ NOT NULL,
  status VARCHAR(32) NOT NULL DEFAULT 'ACTIVE', result_identity_id BIGINT, reason_code VARCHAR(128) NOT NULL DEFAULT '',
  consumed_at TIMESTAMPTZ, created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
  updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
  CONSTRAINT chk_enterprise_authorization_purpose CHECK (purpose IN ('NOTIFICATION_BIND','LOGIN')),
  CONSTRAINT chk_enterprise_authorization_status CHECK (status IN ('ACTIVE','PROCESSING','SUCCEEDED','FAILED','OUTCOME_UNKNOWN')),
  CONSTRAINT chk_enterprise_authorization_positive_ids CHECK (id>0 AND organization_id>0 AND connection_id>0)
);
CREATE UNIQUE INDEX IF NOT EXISTS uk_enterprise_authorization_identity ON sys_enterprise_identity_authorization(authorization_identity);
CREATE UNIQUE INDEX IF NOT EXISTS uk_enterprise_authorization_state ON sys_enterprise_identity_authorization(state_hash);
CREATE INDEX IF NOT EXISTS idx_enterprise_authorization_active ON sys_enterprise_identity_authorization(organization_id,status,expires_at);
`)
	if err != nil {
		return fmt.Errorf("ensure enterprise identity tables: %w", err)
	}

	_, err = tx.Exec(ctx, `
DO $$
DECLARE
  has_config_digest BOOLEAN := FALSE;
  has_digest BOOLEAN := FALSE;
BEGIN
  SELECT
    COALESCE(BOOL_OR(column_name='enterpriseConnectionConfigDigest'),FALSE),
    COALESCE(BOOL_OR(column_name='enterpriseConnectionDigest'),FALSE)
  INTO has_config_digest,has_digest
  FROM information_schema.columns
  WHERE table_schema='public'
    AND table_name='sys_notification_channel'
    AND column_name IN ('enterpriseConnectionConfigDigest','enterpriseConnectionDigest');

  IF has_config_digest AND NOT has_digest THEN
    ALTER TABLE sys_notification_channel RENAME COLUMN "enterpriseConnectionConfigDigest" TO "enterpriseConnectionDigest";
  ELSIF has_config_digest AND has_digest THEN
    IF EXISTS (
      SELECT 1 FROM sys_notification_channel
      WHERE NULLIF(BTRIM("enterpriseConnectionConfigDigest"),'') IS NOT NULL
        AND NULLIF(BTRIM("enterpriseConnectionDigest"),'') IS NOT NULL
        AND BTRIM("enterpriseConnectionConfigDigest") <> BTRIM("enterpriseConnectionDigest")
    ) THEN
      RAISE EXCEPTION 'notification channel enterprise connection digest columns conflict';
    END IF;
    UPDATE sys_notification_channel
    SET "enterpriseConnectionDigest"="enterpriseConnectionConfigDigest"
    WHERE NULLIF(BTRIM("enterpriseConnectionDigest"),'') IS NULL
      AND NULLIF(BTRIM("enterpriseConnectionConfigDigest"),'') IS NOT NULL;
    ALTER TABLE sys_notification_channel DROP COLUMN "enterpriseConnectionConfigDigest";
  END IF;
END $$;

ALTER TABLE sys_notification_channel
  ADD COLUMN IF NOT EXISTS "scopeType" VARCHAR(32) NOT NULL DEFAULT 'SYSTEM',
  ADD COLUMN IF NOT EXISTS "organizationId" BIGINT,
  ADD COLUMN IF NOT EXISTS "enterpriseConnectionId" BIGINT,
  ADD COLUMN IF NOT EXISTS "enterpriseConnectionVersion" BIGINT,
  ADD COLUMN IF NOT EXISTS "enterpriseConnectionDigest" CHAR(64) NOT NULL DEFAULT '';
DO $$
BEGIN
  IF EXISTS (
    SELECT 1 FROM sys_notification_channel
    WHERE LENGTH(BTRIM("enterpriseConnectionDigest")) > 64
  ) THEN
    RAISE EXCEPTION 'notification channel enterprise connection digest exceeds 64 characters';
  END IF;
END $$;
ALTER TABLE sys_notification_channel
  ALTER COLUMN "enterpriseConnectionDigest" TYPE CHAR(64) USING BTRIM("enterpriseConnectionDigest")::CHAR(64),
  ALTER COLUMN "enterpriseConnectionDigest" SET DEFAULT '',
  ALTER COLUMN "enterpriseConnectionDigest" SET NOT NULL;
ALTER TABLE sys_notification
  ADD COLUMN IF NOT EXISTS "organizationId" BIGINT;
ALTER TABLE sys_notification_external_target
  ADD COLUMN IF NOT EXISTS "organizationId" BIGINT,
  ADD COLUMN IF NOT EXISTS "userId" BIGINT,
  ADD COLUMN IF NOT EXISTS "externalIdentityId" BIGINT,
  ADD COLUMN IF NOT EXISTS "externalIdentityVersion" BIGINT,
  ADD COLUMN IF NOT EXISTS "enterpriseConnectionId" BIGINT,
  ADD COLUMN IF NOT EXISTS "enterpriseConnectionVersion" BIGINT,
  ADD COLUMN IF NOT EXISTS "authorityDigest" VARCHAR(128) NOT NULL DEFAULT '',
  ADD COLUMN IF NOT EXISTS "grantType" VARCHAR(64) NOT NULL DEFAULT '';
ALTER TABLE sys_notification_delivery ADD COLUMN IF NOT EXISTS "organizationId" BIGINT;
`)
	if err != nil {
		return fmt.Errorf("ensure enterprise identity schema: %w", err)
	}
	return nil
}

func protectLegacySystemSubjects(ctx context.Context, tx pgx.Tx, protector SubjectProtector) error {
	rows, err := tx.Query(ctx, `SELECT id,"providerCode",COALESCE("externalIssuer",''),"externalSubject"
		FROM sys_external_user_identity
		WHERE scope_type='SYSTEM' AND "isDeleted"=0 AND NULLIF(BTRIM(COALESCE("externalSubject",'')),'') IS NOT NULL
		ORDER BY id FOR UPDATE`)
	if err != nil {
		return err
	}
	type legacySubject struct {
		id       int64
		provider string
		issuer   string
		subject  string
	}
	var legacy []legacySubject
	for rows.Next() {
		var item legacySubject
		if err = rows.Scan(&item.id, &item.provider, &item.issuer, &item.subject); err != nil {
			rows.Close()
			return err
		}
		legacy = append(legacy, item)
	}
	if err = rows.Err(); err != nil {
		rows.Close()
		return err
	}
	rows.Close()
	if len(legacy) == 0 {
		return nil
	}
	if protector == nil {
		return fmt.Errorf("%d legacy SYSTEM external identity subjects require cryptographic alignment dependencies", len(legacy))
	}
	for _, item := range legacy {
		encrypted, protectErr := protector.EncryptSubject(ctx, item.subject)
		if protectErr != nil {
			return fmt.Errorf("encrypt SYSTEM external identity %d: %w", item.id, protectErr)
		}
		digest, digestKeyRef, digestErr := protector.DigestSystemSubject(ctx, item.provider, item.issuer, item.subject)
		if digestErr != nil {
			return fmt.Errorf("digest SYSTEM external identity %d: %w", item.id, digestErr)
		}
		if strings.TrimSpace(encrypted.Ciphertext) == "" || strings.TrimSpace(encrypted.EDEK) == "" || strings.TrimSpace(encrypted.WrapKeyRef) == "" || strings.TrimSpace(digest) == "" || strings.TrimSpace(digestKeyRef) == "" {
			return fmt.Errorf("SYSTEM external identity %d protection returned an incomplete value", item.id)
		}
		if _, err = tx.Exec(ctx, `UPDATE sys_external_user_identity SET
			"externalIssuer"=$1,"externalSubject"=NULL,subject_kind='SYSTEM_OPAQUE',subject_ciphertext=$2,subject_edek=$3,
			subject_wrap_key_ref=$4,subject_digest=$5,subject_digest_key_ref=$6,identity_version=identity_version+1,"updateTime"=CURRENT_TIMESTAMP
			WHERE id=$7 AND scope_type='SYSTEM' AND "isDeleted"=0`, strings.TrimSpace(item.issuer), encrypted.Ciphertext, encrypted.EDEK, encrypted.WrapKeyRef, digest, digestKeyRef, item.id); err != nil {
			return fmt.Errorf("persist protected SYSTEM external identity %d: %w", item.id, err)
		}
		var roundTrip string
		if err = tx.QueryRow(ctx, `SELECT subject_digest FROM sys_external_user_identity WHERE id=$1 AND "externalSubject" IS NULL`, item.id).Scan(&roundTrip); err != nil || roundTrip != digest {
			return fmt.Errorf("verify protected SYSTEM external identity %d", item.id)
		}
	}
	return nil
}

func verifyEnterpriseIdentitySchemaShape(ctx context.Context, q interface {
	QueryRow(context.Context, string, ...any) pgx.Row
}) error {
	var drift int
	err := q.QueryRow(ctx, `WITH expected(table_name,column_name,data_type,is_nullable) AS (VALUES
		('sys_enterprise_identity_connection','id','bigint','NO'),
		('sys_enterprise_identity_connection','organization_id','bigint','NO'),
		('sys_enterprise_identity_grant','external_identity_id','bigint','NO'),
		('sys_enterprise_identity_authorization','connection_id','bigint','NO'),
		('sys_external_user_identity','organization_id','bigint','YES'),
		('sys_external_user_identity','subject_ciphertext','text','YES'),
		('sys_external_user_identity','subject_digest','character varying','YES'),
		('sys_notification_channel','organizationId','bigint','YES'),
		('sys_notification_channel','enterpriseConnectionId','bigint','YES'),
		('sys_notification','organizationId','bigint','YES'),
		('sys_notification_external_target','externalIdentityId','bigint','YES'),
		('sys_notification_external_target','authorityDigest','character varying','NO'),
		('sys_notification_delivery','organizationId','bigint','YES')
	)
	SELECT count(*) FROM expected e LEFT JOIN information_schema.columns c
	ON c.table_schema='public' AND c.table_name=e.table_name AND c.column_name=e.column_name
	WHERE c.column_name IS NULL OR c.data_type<>e.data_type OR c.is_nullable<>e.is_nullable`).Scan(&drift)
	if err != nil {
		return err
	}
	if drift != 0 {
		return fmt.Errorf("enterprise identity schema has %d missing or incompatible columns", drift)
	}
	var foreignKeys int64
	if err = q.QueryRow(ctx, `SELECT count(*) FROM pg_constraint WHERE contype='f' AND conrelid IN (
		'sys_enterprise_identity_connection'::regclass,'sys_enterprise_identity_grant'::regclass,'sys_enterprise_identity_authorization'::regclass)`).Scan(&foreignKeys); err != nil {
		return err
	}
	if foreignKeys != 0 {
		return fmt.Errorf("enterprise identity schema contains %d forbidden foreign keys", foreignKeys)
	}
	var unprotectedSystemIdentities int64
	if err = q.QueryRow(ctx, `SELECT count(*) FROM sys_external_user_identity
		WHERE scope_type='SYSTEM' AND "isDeleted"=0 AND (
			NULLIF(BTRIM(COALESCE("externalSubject",'')),'') IS NOT NULL OR
			NULLIF(BTRIM(COALESCE(subject_ciphertext,'')),'') IS NULL OR
			NULLIF(BTRIM(COALESCE(subject_edek,'')),'') IS NULL OR
			NULLIF(BTRIM(COALESCE(subject_wrap_key_ref,'')),'') IS NULL OR
			NULLIF(BTRIM(COALESCE(subject_digest,'')),'') IS NULL OR
			NULLIF(BTRIM(COALESCE(subject_digest_key_ref,'')),'') IS NULL
		)`).Scan(&unprotectedSystemIdentities); err != nil {
		return err
	}
	if unprotectedSystemIdentities != 0 {
		return fmt.Errorf("enterprise identity schema contains %d unprotected SYSTEM subjects", unprotectedSystemIdentities)
	}
	var systemSubjectIndex bool
	if err = q.QueryRow(ctx, `SELECT to_regclass('public.uk_external_identity_system_subject_active') IS NOT NULL`).Scan(&systemSubjectIndex); err != nil {
		return err
	}
	if !systemSubjectIndex {
		return errors.New("SYSTEM external identity subject index is not aligned")
	}
	return nil
}

func ensureTaskNotificationPermission(ctx context.Context, tx pgx.Tx, idGen IDGenerator) error {
	if idGen == nil {
		return errors.New("Task notification permission alignment requires Framework IDGen")
	}
	var available bool
	if err := tx.QueryRow(ctx, `SELECT to_regclass('public.sys_permission') IS NOT NULL
		AND to_regclass('public.sys_role') IS NOT NULL
		AND to_regclass('public.sys_role_permission') IS NOT NULL`).Scan(&available); err != nil {
		return err
	}
	if !available {
		return errors.New("Review Policy permission alignment requires Framework RBAC tables")
	}
	const code = "workforce:task-notification:operate"
	var permissionID int64
	err := tx.QueryRow(ctx, `SELECT id FROM sys_permission WHERE code=$1 AND "isDeleted"=false FOR UPDATE`, code).Scan(&permissionID)
	if errors.Is(err, pgx.ErrNoRows) {
		permissionID = idGen.NextID()
		if permissionID <= 0 {
			return errors.New("Framework IDGen returned an invalid Task notification permission ID")
		}
		_, err = tx.Exec(ctx, `INSERT INTO sys_permission
			(id,code,name,"resourceType",method,path,status,description,"createTime","updateTime","isDeleted")
			VALUES ($1,$2,'管理任务通知','API','PUT','/workforce/tasks/{id}/notification-subscription',0,
			'管理本租户任务创建人的通知订阅',CURRENT_TIMESTAMP,CURRENT_TIMESTAMP,false)`, permissionID, code)
	}
	if err != nil {
		return fmt.Errorf("align Task notification permission: %w", err)
	}
	rows, err := tx.Query(ctx, `SELECT id FROM sys_role WHERE code IN ('SUPER_ADMIN','SYSTEM_ADMIN') AND "isDeleted"=false ORDER BY id`)
	if err != nil {
		return err
	}
	var roleIDs []int64
	for rows.Next() {
		var roleID int64
		if err = rows.Scan(&roleID); err != nil {
			rows.Close()
			return err
		}
		roleIDs = append(roleIDs, roleID)
	}
	if err = rows.Err(); err != nil {
		rows.Close()
		return err
	}
	rows.Close()
	for _, roleID := range roleIDs {
		var exists bool
		if err = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM sys_role_permission WHERE "roleId"=$1 AND "permissionId"=$2)`, roleID, permissionID).Scan(&exists); err != nil {
			return err
		}
		if exists {
			continue
		}
		relationID := idGen.NextID()
		if relationID <= 0 {
			return errors.New("Framework IDGen returned an invalid role permission ID")
		}
		if _, err = tx.Exec(ctx, `INSERT INTO sys_role_permission
			(id,"roleId","permissionId","creatorId","createTime","updateTime")
			VALUES ($1,$2,$3,NULL,CURRENT_TIMESTAMP,CURRENT_TIMESTAMP)`, relationID, roleID, permissionID); err != nil {
			return fmt.Errorf("align Task notification role permission: %w", err)
		}
	}
	return nil
}

func ensureNotificationGroupRoutePermissions(ctx context.Context, tx pgx.Tx, idGen IDGenerator) error {
	if idGen == nil {
		return errors.New("Notification group route permission alignment requires Framework IDGen")
	}
	definitions := []struct{ code, name, method, path string }{
		{"system:notification-group-route:list", "通知群列表", "GET", "/notification/group-routes"},
		{"system:notification-group-route:query", "查看通知群", "GET", "/notification/group-routes/:routeId"},
		{"system:notification-group-route:create", "创建通知群", "POST", "/notification/group-routes"},
		{"system:notification-group-route:update", "更新通知群", "PUT", "/notification/group-routes/:routeId"},
		{"system:notification-group-route:probe", "测试通知群", "POST", "/notification/group-routes/:routeId/probe"},
		{"system:notification-group-route:enable", "启用通知群", "POST", "/notification/group-routes/:routeId/enable"},
		{"system:notification-group-route:disable", "停用通知群", "POST", "/notification/group-routes/:routeId/disable"},
		{"system:notification-group-route:retire", "退役通知群", "POST", "/notification/group-routes/:routeId/retire"},
		{"system:notification-group-route:delete", "删除通知群", "DELETE", "/notification/group-routes/:routeId"},
	}
	rows, err := tx.Query(ctx, `SELECT id FROM sys_role WHERE code IN ('SUPER_ADMIN','SYSTEM_ADMIN') AND "isDeleted"=false ORDER BY id`)
	if err != nil {
		return err
	}
	var roleIDs []int64
	for rows.Next() {
		var id int64
		if err = rows.Scan(&id); err != nil {
			rows.Close()
			return err
		}
		roleIDs = append(roleIDs, id)
	}
	rows.Close()
	for _, definition := range definitions {
		var permissionID int64
		err = tx.QueryRow(ctx, `SELECT id FROM sys_permission WHERE code=$1 AND "isDeleted"=false FOR UPDATE`, definition.code).Scan(&permissionID)
		if errors.Is(err, pgx.ErrNoRows) {
			permissionID = idGen.NextID()
			_, err = tx.Exec(ctx, `INSERT INTO sys_permission (id,code,name,"resourceType",method,path,status,description,"createTime","updateTime","isDeleted") VALUES ($1,$2,$3,'API',$4,$5,0,$3,CURRENT_TIMESTAMP,CURRENT_TIMESTAMP,false)`, permissionID, definition.code, definition.name, definition.method, definition.path)
		}
		if err != nil {
			return err
		}
		for _, roleID := range roleIDs {
			var exists bool
			if err = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM sys_role_permission WHERE "roleId"=$1 AND "permissionId"=$2)`, roleID, permissionID).Scan(&exists); err != nil {
				return err
			}
			if !exists {
				if _, err = tx.Exec(ctx, `INSERT INTO sys_role_permission (id,"roleId","permissionId","creatorId","createTime","updateTime") VALUES ($1,$2,$3,NULL,CURRENT_TIMESTAMP,CURRENT_TIMESTAMP)`, idGen.NextID(), roleID, permissionID); err != nil {
					return err
				}
			}
		}
	}
	return nil
}

func ensureEnterpriseIdentityPermissions(ctx context.Context, tx pgx.Tx, idGen IDGenerator) error {
	if idGen == nil {
		return errors.New("Enterprise identity permission alignment requires Framework IDGen")
	}
	definitions := []struct{ code, name, method, path, description string }{
		{"system:enterprise-connection:list", "企业连接列表", "GET", "/system/enterprise-connections", "查询本组织企业连接"},
		{"system:enterprise-connection:edit", "编辑企业连接", "PUT", "/system/enterprise-connections", "保存并检查本组织企业连接"},
		{"system:enterprise-connection:status", "启停企业连接", "PUT", "/system/enterprise-connections/:connectionId/status", "启用或停用企业连接"},
		{"system:enterprise-identity:list", "企业账号成员列表", "GET", "/system/enterprise-identities", "查询本组织已绑定成员"},
		{"system:enterprise-identity:login-access", "企业账号登录授权", "PUT", "/system/enterprise-identities/:identityId/login-access", "授予或撤销企业登录权限"},
	}
	rows, err := tx.Query(ctx, `SELECT id FROM sys_role WHERE code IN ('SUPER_ADMIN','SYSTEM_ADMIN') AND "isDeleted"=false ORDER BY id`)
	if err != nil {
		return err
	}
	var roleIDs []int64
	for rows.Next() {
		var roleID int64
		if err = rows.Scan(&roleID); err != nil {
			rows.Close()
			return err
		}
		roleIDs = append(roleIDs, roleID)
	}
	if err = rows.Err(); err != nil {
		rows.Close()
		return err
	}
	rows.Close()
	for _, definition := range definitions {
		var permissionID int64
		err = tx.QueryRow(ctx, `SELECT id FROM sys_permission WHERE code=$1 AND "isDeleted"=false FOR UPDATE`, definition.code).Scan(&permissionID)
		if errors.Is(err, pgx.ErrNoRows) {
			permissionID = idGen.NextID()
			if permissionID <= 0 {
				return errors.New("Framework IDGen returned an invalid enterprise identity permission ID")
			}
			_, err = tx.Exec(ctx, `INSERT INTO sys_permission
				(id,code,name,"resourceType",method,path,status,description,"createTime","updateTime","isDeleted")
				VALUES ($1,$2,$3,'API',$4,$5,0,$6,CURRENT_TIMESTAMP,CURRENT_TIMESTAMP,false)`,
				permissionID, definition.code, definition.name, definition.method, definition.path, definition.description)
		}
		if err != nil {
			return fmt.Errorf("align enterprise identity permission %s: %w", definition.code, err)
		}
		for _, roleID := range roleIDs {
			var exists bool
			if err = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM sys_role_permission WHERE "roleId"=$1 AND "permissionId"=$2)`, roleID, permissionID).Scan(&exists); err != nil {
				return err
			}
			if !exists {
				relationID := idGen.NextID()
				if relationID <= 0 {
					return errors.New("Framework IDGen returned an invalid enterprise role permission ID")
				}
				if _, err = tx.Exec(ctx, `INSERT INTO sys_role_permission
					(id,"roleId","permissionId","creatorId","createTime","updateTime")
					VALUES ($1,$2,$3,NULL,CURRENT_TIMESTAMP,CURRENT_TIMESTAMP)`, relationID, roleID, permissionID); err != nil {
					return err
				}
			}
		}
	}
	var menuID int64
	err = tx.QueryRow(ctx, `SELECT id FROM sys_menu WHERE path='/system/enterprise-connections' AND "isDeleted"=false FOR UPDATE`).Scan(&menuID)
	if errors.Is(err, pgx.ErrNoRows) {
		var parentID int64
		var parentHierarchy string
		var parentLevel int
		if err = tx.QueryRow(ctx, `SELECT id,hierarchy,level FROM sys_menu WHERE path='/system/access' AND "isDeleted"=false ORDER BY id LIMIT 1 FOR UPDATE`).Scan(&parentID, &parentHierarchy, &parentLevel); err != nil {
			return fmt.Errorf("resolve enterprise identity menu parent: %w", err)
		}
		menuID = idGen.NextID()
		if menuID <= 0 {
			return errors.New("Framework IDGen returned an invalid enterprise identity menu ID")
		}
		hierarchy := strings.TrimRight(strings.TrimSpace(parentHierarchy), "/") + "/" + strconv.FormatInt(menuID, 10)
		_, err = tx.Exec(ctx, `INSERT INTO sys_menu
			(id,name,"parentId","sortOrder",path,component,icon,type,permission,"isFrame","isCache",visible,hierarchy,level,status,"creatorId","createTime","updaterId","updateTime","isDeleted",remark)
			VALUES ($1,'企业连接',$2,51,'/system/enterprise-connections','system/enterprise-connections/page','LinkOutlined','C','system:enterprise-connection:list',true,true,true,$3,$4,0,NULL,CURRENT_TIMESTAMP,NULL,CURRENT_TIMESTAMP,false,'组织企业应用和成员绑定管理')`,
			menuID, parentID, hierarchy, parentLevel+1)
	}
	if err != nil {
		return fmt.Errorf("align enterprise identity menu: %w", err)
	}
	for _, definition := range definitions {
		var permissionID int64
		if err = tx.QueryRow(ctx, `SELECT id FROM sys_permission WHERE code=$1 AND "isDeleted"=false`, definition.code).Scan(&permissionID); err != nil {
			return err
		}
		var exists bool
		if err = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM sys_menu_permission WHERE "menuId"=$1 AND "permissionId"=$2)`, menuID, permissionID).Scan(&exists); err != nil {
			return err
		}
		if !exists {
			relationID := idGen.NextID()
			if relationID <= 0 {
				return errors.New("Framework IDGen returned an invalid enterprise menu permission ID")
			}
			if _, err = tx.Exec(ctx, `INSERT INTO sys_menu_permission (id,"menuId","permissionId","creatorId","createTime") VALUES ($1,$2,$3,NULL,CURRENT_TIMESTAMP)`, relationID, menuID, permissionID); err != nil {
				return err
			}
		}
	}
	for _, roleID := range roleIDs {
		var exists bool
		if err = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM sys_role_menu WHERE "roleId"=$1 AND "menuId"=$2)`, roleID, menuID).Scan(&exists); err != nil {
			return err
		}
		if !exists {
			relationID := idGen.NextID()
			if relationID <= 0 {
				return errors.New("Framework IDGen returned an invalid enterprise role menu ID")
			}
			if _, err = tx.Exec(ctx, `INSERT INTO sys_role_menu (id,"roleId","menuId","createTime","updateTime") VALUES ($1,$2,$3,CURRENT_TIMESTAMP,CURRENT_TIMESTAMP)`, relationID, roleID, menuID); err != nil {
				return err
			}
		}
	}
	return nil
}

func ensureUnpublishedSchemaIndexes(ctx context.Context, tx pgx.Tx) error {
	processorIDExists, err := columnExists(ctx, tx, "workforce_result_processor_releases", "processor_id")
	if err != nil {
		return err
	}
	versionExists, err := columnExists(ctx, tx, "workforce_result_processor_releases", "version")
	if err != nil {
		return err
	}
	if processorIDExists && versionExists {
		if _, err = tx.Exec(ctx, `CREATE UNIQUE INDEX IF NOT EXISTS workforce_result_processor_releases_version_uidx
			ON workforce_result_processor_releases(processor_id,version)`); err != nil {
			return fmt.Errorf("align unpublished Result Processor release index: %w", err)
		}
	}
	for _, column := range []string{"workspace_source_revision_id", "worker_id"} {
		exists, checkErr := columnExists(ctx, tx, "workforce_source_ingestion_runs", column)
		if checkErr != nil {
			return checkErr
		}
		if exists {
			if _, err = tx.Exec(ctx, `ALTER TABLE workforce_source_ingestion_runs ALTER COLUMN `+pgx.Identifier{column}.Sanitize()+` DROP NOT NULL`); err != nil {
				return fmt.Errorf("align unpublished Source Ingestion column %s: %w", column, err)
			}
		}
	}
	return nil
}

// repairOpenIntegrationCompatibility reverses the short-lived unpublished
// schema produced by the first BIGINT alignment pass. That pass treated the
// external credential/correlation locators as internal identities and routed
// Integration Application.created_by through workforce_principals. The public
// contract now keeps external locators as governed TEXT while created_by
// directly references Framework sys_user.id.
func repairOpenIntegrationCompatibility(ctx context.Context, tx pgx.Tx, idGen IDGenerator) error {
	var tableExists bool
	if err := tx.QueryRow(ctx, `SELECT to_regclass('public.workforce_integration_applications') IS NOT NULL`).Scan(&tableExists); err != nil || !tableExists {
		return err
	}
	if err := alignOpenIntegrationWebhookURL(ctx, tx); err != nil {
		return err
	}
	if err := alignOpenIntegrationProjectionState(ctx, tx); err != nil {
		return err
	}

	createdByExists, err := columnExists(ctx, tx, "workforce_integration_applications", "created_by")
	if err != nil {
		return err
	}
	principalCreatorExists, err := columnExists(ctx, tx, "workforce_integration_applications", "created_by_principal_id")
	if err != nil {
		return err
	}
	if principalCreatorExists {
		if !createdByExists {
			if _, err = tx.Exec(ctx, `ALTER TABLE workforce_integration_applications ADD COLUMN created_by BIGINT`); err != nil {
				return fmt.Errorf("add Integration Application created_by: %w", err)
			}
		}
		if _, err = tx.Exec(ctx, `UPDATE workforce_integration_applications a
SET created_by=COALESCE(p.user_id,u.id)
FROM workforce_integration_applications source
LEFT JOIN workforce_principals p ON p.id=source.created_by_principal_id AND p.principal_type='USER'
LEFT JOIN sys_user u ON u.id=source.created_by_principal_id AND u."isDeleted"=false
WHERE a.id=source.id AND a.created_by IS NULL`); err != nil {
			return fmt.Errorf("restore Integration Application creator user: %w", err)
		}
		var missing int64
		if err = tx.QueryRow(ctx, `SELECT count(*) FROM workforce_integration_applications WHERE created_by IS NULL`).Scan(&missing); err != nil {
			return err
		}
		if missing != 0 {
			return fmt.Errorf("restore Integration Application creator left %d unresolved rows", missing)
		}
		if _, err = tx.Exec(ctx, `ALTER TABLE workforce_integration_applications ALTER COLUMN created_by SET NOT NULL; ALTER TABLE workforce_integration_applications DROP COLUMN created_by_principal_id`); err != nil {
			return fmt.Errorf("finalize Integration Application creator: %w", err)
		}
	}

	credentialLocatorExists, err := columnExists(ctx, tx, "workforce_integration_applications", "credential_locator")
	if err != nil {
		return err
	}
	credentialIDExists, err := columnExists(ctx, tx, "workforce_integration_applications", "credential_id")
	if err != nil {
		return err
	}
	if credentialIDExists {
		if !credentialLocatorExists {
			if _, err = tx.Exec(ctx, `ALTER TABLE workforce_integration_applications ADD COLUMN credential_locator TEXT`); err != nil {
				return fmt.Errorf("add Integration Application credential locator: %w", err)
			}
		}
		if _, err = tx.Exec(ctx, `UPDATE workforce_integration_applications a
SET credential_locator=COALESCE(m.legacy_identity,a.credential_id::text)
FROM workforce_integration_applications source
LEFT JOIN workforce_legacy_identity_map m ON m.entity_type='GLOBAL' AND m.entity_id=source.credential_id
WHERE a.id=source.id AND a.credential_locator IS NULL`); err != nil {
			return fmt.Errorf("restore Integration Application credential locator: %w", err)
		}
		var missing int64
		if err = tx.QueryRow(ctx, `SELECT count(*) FROM workforce_integration_applications WHERE NULLIF(btrim(credential_locator),'') IS NULL`).Scan(&missing); err != nil {
			return err
		}
		if missing != 0 {
			return fmt.Errorf("restore Integration Application credential locator left %d unresolved rows", missing)
		}
		if _, err = tx.Exec(ctx, `ALTER TABLE workforce_integration_applications ALTER COLUMN credential_locator SET NOT NULL; ALTER TABLE workforce_integration_applications DROP COLUMN credential_id`); err != nil {
			return fmt.Errorf("finalize Integration Application credential locator: %w", err)
		}
	}
	if err = alignOpenIntegrationCredentialTables(ctx, tx, idGen); err != nil {
		return err
	}

	var correlationType string
	err = tx.QueryRow(ctx, `SELECT data_type FROM information_schema.columns WHERE table_schema='public' AND table_name='workforce_tasks' AND column_name='external_correlation_id'`).Scan(&correlationType)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return err
	}
	if errors.Is(err, pgx.ErrNoRows) {
		if _, err = tx.Exec(ctx, `ALTER TABLE workforce_tasks ADD COLUMN external_correlation_id TEXT`); err != nil {
			return fmt.Errorf("add external correlation identity: %w", err)
		}
		return nil
	}
	if err == nil && correlationType != "text" && correlationType != "character varying" {
		if _, err = tx.Exec(ctx, `ALTER TABLE workforce_tasks ADD COLUMN external_correlation_locator TEXT`); err != nil {
			return fmt.Errorf("add external correlation compatibility column: %w", err)
		}
		if _, err = tx.Exec(ctx, `UPDATE workforce_tasks t
SET external_correlation_locator=COALESCE(m.legacy_identity,t.external_correlation_id::text)
FROM workforce_tasks source
LEFT JOIN workforce_legacy_identity_map m ON m.entity_type='GLOBAL' AND m.entity_id=source.external_correlation_id
WHERE t.id=source.id AND t.external_correlation_id IS NOT NULL`); err != nil {
			return fmt.Errorf("restore external correlation identity: %w", err)
		}
		if _, err = tx.Exec(ctx, `ALTER TABLE workforce_tasks DROP COLUMN external_correlation_id; ALTER TABLE workforce_tasks RENAME COLUMN external_correlation_locator TO external_correlation_id`); err != nil {
			return fmt.Errorf("finalize external correlation identity: %w", err)
		}
	}
	return nil
}

func alignOpenIntegrationWebhookURL(ctx context.Context, tx pgx.Tx) error {
	var exists bool
	if err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM information_schema.columns
WHERE table_schema='public' AND table_name='workforce_integration_applications' AND column_name='webhook_url')`).Scan(&exists); err != nil {
		return err
	}
	if !exists {
		if _, err := tx.Exec(ctx, `ALTER TABLE workforce_integration_applications ADD COLUMN webhook_url VARCHAR(2048) NOT NULL DEFAULT ''`); err != nil {
			return fmt.Errorf("add optional Integration Application webhook URL: %w", err)
		}
		return nil
	}
	var oversized int64
	if err := tx.QueryRow(ctx, `SELECT count(*) FROM workforce_integration_applications WHERE length(webhook_url)>2048`).Scan(&oversized); err != nil {
		return err
	}
	if oversized != 0 {
		return fmt.Errorf("Integration Application webhook URL alignment found %d values longer than 2048 characters", oversized)
	}
	if _, err := tx.Exec(ctx, `UPDATE workforce_integration_applications SET webhook_url='' WHERE webhook_url IS NULL;
ALTER TABLE workforce_integration_applications
  ALTER COLUMN webhook_url TYPE VARCHAR(2048) USING webhook_url::VARCHAR(2048),
  ALTER COLUMN webhook_url SET DEFAULT '',
  ALTER COLUMN webhook_url SET NOT NULL`); err != nil {
		return fmt.Errorf("align optional Integration Application webhook URL: %w", err)
	}
	return nil
}

func alignOpenIntegrationProjectionState(ctx context.Context, tx pgx.Tx) error {
	if _, err := tx.Exec(ctx, `ALTER TABLE workforce_task_events
		ADD COLUMN IF NOT EXISTS open_integration_projection_status VARCHAR(32) NOT NULL DEFAULT 'PENDING';
		ALTER TABLE workforce_task_events
		ALTER COLUMN open_integration_projection_status TYPE VARCHAR(32) USING open_integration_projection_status::VARCHAR(32),
		ALTER COLUMN open_integration_projection_status SET DEFAULT 'PENDING',
		ALTER COLUMN open_integration_projection_status SET NOT NULL`); err != nil {
		return fmt.Errorf("align Open Integration projection status column: %w", err)
	}
	var constraintDefinition string
	if err := tx.QueryRow(ctx, `SELECT COALESCE((SELECT pg_get_constraintdef(oid) FROM pg_constraint
WHERE conrelid='workforce_task_events'::regclass AND conname='workforce_task_events_open_projection_status_ck'),'')`).Scan(&constraintDefinition); err != nil {
		return err
	}
	if constraintDefinition != openProjectionConstraintDefinition {
		if _, err := tx.Exec(ctx, `ALTER TABLE workforce_task_events DROP CONSTRAINT IF EXISTS workforce_task_events_open_projection_status_ck;
UPDATE workforce_task_events SET open_integration_projection_status='SKIPPED'
WHERE open_integration_projection_status='SKIPPED_NO_WEBHOOK'`); err != nil {
			return fmt.Errorf("normalize legacy Open Integration projection status: %w", err)
		}
		var invalid int64
		if err := tx.QueryRow(ctx, `SELECT count(*) FROM workforce_task_events
WHERE open_integration_projection_status NOT IN ('PENDING','PROJECTED','SKIPPED')`).Scan(&invalid); err != nil {
			return err
		}
		if invalid != 0 {
			return fmt.Errorf("Open Integration projection status alignment found %d unsupported values", invalid)
		}
		if _, err := tx.Exec(ctx, `ALTER TABLE workforce_task_events ADD CONSTRAINT workforce_task_events_open_projection_status_ck
CHECK (open_integration_projection_status IN ('PENDING','PROJECTED','SKIPPED'))`); err != nil {
			return fmt.Errorf("align Open Integration projection status constraint: %w", err)
		}
	}
	var indexDefinition string
	if err := tx.QueryRow(ctx, `SELECT COALESCE((SELECT pg_get_indexdef(i.oid) FROM pg_class i
WHERE i.relnamespace='public'::regnamespace AND i.relname='idx_workforce_task_events_open_projection_pending'),'')`).Scan(&indexDefinition); err != nil {
		return err
	}
	if indexDefinition != openProjectionIndexDefinition {
		if _, err := tx.Exec(ctx, `DROP INDEX IF EXISTS idx_workforce_task_events_open_projection_pending;
CREATE INDEX idx_workforce_task_events_open_projection_pending
  ON workforce_task_events(id)
  WHERE open_integration_projection_status='PENDING'
    AND event_type IN ('task.succeeded','task.failed','task.cancelled','task.timed_out')`); err != nil {
			return fmt.Errorf("align Open Integration projection pending index: %w", err)
		}
	}
	return nil
}

func backfillOpenIntegrationProjectionState(ctx context.Context, tx pgx.Tx) error {
	if _, err := tx.Exec(ctx, `UPDATE workforce_task_events e SET open_integration_projection_status='PROJECTED'
WHERE e.open_integration_projection_status='PENDING'
  AND EXISTS (SELECT 1 FROM workforce_public_events p WHERE p.source_task_event_id=e.id)`); err != nil {
		return fmt.Errorf("backfill projected Open Integration Task Events: %w", err)
	}
	if _, err := tx.Exec(ctx, `UPDATE workforce_task_events e SET open_integration_projection_status='SKIPPED'
FROM workforce_tasks t LEFT JOIN workforce_integration_applications a
  ON a.id=t.integration_application_id AND a.tenant_id=t.tenant_id
WHERE e.open_integration_projection_status='PENDING' AND e.task_id=t.id AND COALESCE(a.webhook_url,'')=''
  AND ((e.event_type='task.succeeded' AND t.status='SUCCEEDED')
    OR (e.event_type='task.failed' AND t.status='FAILED')
    OR (e.event_type='task.cancelled' AND t.status='CANCELLED')
    OR (e.event_type='task.timed_out' AND t.status='TIMED_OUT'));`); err != nil {
		return fmt.Errorf("backfill skipped Open Integration Task Events: %w", err)
	}
	return nil
}

func alignOpenIntegrationCredentialTables(ctx context.Context, tx pgx.Tx, idGen IDGenerator) error {
	if idGen == nil {
		return errors.New("Framework Snowflake generator is required for Integration Credential alignment")
	}
	if _, err := tx.Exec(ctx, `
CREATE TABLE IF NOT EXISTS workforce_integration_api_credentials (
  id BIGINT PRIMARY KEY,
  tenant_id BIGINT NOT NULL,
  integration_application_id BIGINT NOT NULL,
  credential_locator TEXT NOT NULL UNIQUE,
  credential_digest TEXT NOT NULL,
  credential_hint TEXT NOT NULL,
  version INTEGER NOT NULL,
  status TEXT NOT NULL,
  valid_until TIMESTAMPTZ,
  revoked_at TIMESTAMPTZ,
  created_by BIGINT NOT NULL,
  created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
  updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
  last_used_at TIMESTAMPTZ,
  UNIQUE (integration_application_id,version),
  CHECK (version>0),
  CHECK (status IN ('CURRENT','GRACE','REVOKED'))
);
CREATE UNIQUE INDEX IF NOT EXISTS idx_workforce_integration_api_credentials_current
  ON workforce_integration_api_credentials(integration_application_id) WHERE status='CURRENT';
CREATE INDEX IF NOT EXISTS idx_workforce_integration_api_credentials_application
  ON workforce_integration_api_credentials(tenant_id,integration_application_id,status,version DESC);
CREATE TABLE IF NOT EXISTS workforce_integration_signing_credentials (
  id BIGINT PRIMARY KEY,
  tenant_id BIGINT NOT NULL,
  integration_application_id BIGINT NOT NULL,
  signing_secret_ciphertext TEXT NOT NULL,
  signing_secret_edek TEXT NOT NULL,
  signing_secret_wrap_key_ref TEXT NOT NULL,
  signing_secret_hint TEXT NOT NULL,
  version INTEGER NOT NULL,
  status TEXT NOT NULL,
  valid_until TIMESTAMPTZ,
  revoked_at TIMESTAMPTZ,
  created_by BIGINT NOT NULL,
  created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
  updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
  last_used_at TIMESTAMPTZ,
  UNIQUE (integration_application_id,version),
  CHECK (version>0),
  CHECK (status IN ('CURRENT','GRACE','REVOKED'))
);
CREATE UNIQUE INDEX IF NOT EXISTS idx_workforce_integration_signing_credentials_current
  ON workforce_integration_signing_credentials(integration_application_id) WHERE status='CURRENT';
CREATE INDEX IF NOT EXISTS idx_workforce_integration_signing_credentials_application
  ON workforce_integration_signing_credentials(tenant_id,integration_application_id,status,version DESC);
ALTER TABLE workforce_integration_api_credentials ADD COLUMN IF NOT EXISTS last_used_at TIMESTAMPTZ;
ALTER TABLE workforce_integration_signing_credentials ADD COLUMN IF NOT EXISTS last_used_at TIMESTAMPTZ;`); err != nil {
		return fmt.Errorf("create versioned Integration Credential schema: %w", err)
	}

	legacyColumns := []string{
		"credential_locator", "credential_digest", "credential_hint", "signing_secret_ciphertext",
		"signing_secret_edek", "signing_secret_wrap_key_ref", "signing_secret_hint",
	}
	legacyPresent := true
	for _, column := range legacyColumns {
		exists, err := columnExists(ctx, tx, "workforce_integration_applications", column)
		if err != nil {
			return err
		}
		legacyPresent = legacyPresent && exists
	}
	if legacyPresent {
		type legacyCredential struct {
			applicationID, tenantID, createdBy                                                  int64
			locator, digest, credentialHint, webhookURL, ciphertext, edek, wrapRef, signingHint string
			createdAt, updatedAt                                                                time.Time
			lastUsedAt                                                                          *time.Time
		}
		rows, err := tx.Query(ctx, `SELECT id,tenant_id,created_by,credential_locator,credential_digest,credential_hint,webhook_url,
signing_secret_ciphertext,signing_secret_edek,signing_secret_wrap_key_ref,signing_secret_hint,created_at,updated_at,last_used_at
FROM workforce_integration_applications ORDER BY id`)
		if err != nil {
			return fmt.Errorf("read legacy Integration Credentials: %w", err)
		}
		legacy := make([]legacyCredential, 0)
		for rows.Next() {
			var item legacyCredential
			if err = rows.Scan(&item.applicationID, &item.tenantID, &item.createdBy, &item.locator, &item.digest, &item.credentialHint, &item.webhookURL,
				&item.ciphertext, &item.edek, &item.wrapRef, &item.signingHint, &item.createdAt, &item.updatedAt, &item.lastUsedAt); err != nil {
				rows.Close()
				return err
			}
			legacy = append(legacy, item)
		}
		if err = rows.Err(); err != nil {
			rows.Close()
			return err
		}
		rows.Close()
		for _, item := range legacy {
			if _, err = tx.Exec(ctx, `INSERT INTO workforce_integration_api_credentials
(id,tenant_id,integration_application_id,credential_locator,credential_digest,credential_hint,version,status,created_by,created_at,updated_at,last_used_at)
VALUES ($1,$2,$3,$4,$5,$6,1,'CURRENT',$7,$8,$9,$10)
ON CONFLICT (integration_application_id,version) DO NOTHING`, idGen.NextID(), item.tenantID, item.applicationID, item.locator, item.digest, item.credentialHint, item.createdBy, item.createdAt, item.updatedAt, item.lastUsedAt); err != nil {
				return fmt.Errorf("backfill API Credential for application %d: %w", item.applicationID, err)
			}
			if item.webhookURL != "" {
				if _, err = tx.Exec(ctx, `INSERT INTO workforce_integration_signing_credentials
(id,tenant_id,integration_application_id,signing_secret_ciphertext,signing_secret_edek,signing_secret_wrap_key_ref,signing_secret_hint,version,status,created_by,created_at,updated_at)
VALUES ($1,$2,$3,$4,$5,$6,$7,1,'CURRENT',$8,$9,$10)
ON CONFLICT (integration_application_id,version) DO NOTHING`, idGen.NextID(), item.tenantID, item.applicationID, item.ciphertext, item.edek, item.wrapRef, item.signingHint, item.createdBy, item.createdAt, item.updatedAt); err != nil {
					return fmt.Errorf("backfill Signing Credential for application %d: %w", item.applicationID, err)
				}
			}
		}
		var mismatchedAPI, mismatchedSigning int64
		if err = tx.QueryRow(ctx, `SELECT count(*) FROM workforce_integration_applications a
WHERE NOT EXISTS (
  SELECT 1 FROM workforce_integration_api_credentials c
  WHERE c.integration_application_id=a.id AND c.tenant_id=a.tenant_id
    AND c.version=1 AND c.status='CURRENT'
	AND c.credential_locator=a.credential_locator
	AND c.credential_digest=a.credential_digest
	AND c.credential_hint=a.credential_hint
    AND c.created_by=a.created_by
	AND c.created_at=a.created_at
	AND c.updated_at=a.updated_at
	AND (c.last_used_at=a.last_used_at OR (c.last_used_at IS NULL AND a.last_used_at IS NULL))
)`).Scan(&mismatchedAPI); err != nil {
			return fmt.Errorf("verify legacy API Credential backfill: %w", err)
		}
		if err = tx.QueryRow(ctx, `SELECT count(*) FROM workforce_integration_applications a
WHERE a.webhook_url<>'' AND NOT EXISTS (
  SELECT 1 FROM workforce_integration_signing_credentials c
  WHERE c.integration_application_id=a.id AND c.tenant_id=a.tenant_id
    AND c.version=1 AND c.status='CURRENT'
	AND c.signing_secret_ciphertext=a.signing_secret_ciphertext
	AND c.signing_secret_edek=a.signing_secret_edek
	AND c.signing_secret_wrap_key_ref=a.signing_secret_wrap_key_ref
	AND c.signing_secret_hint=a.signing_secret_hint
    AND c.created_by=a.created_by
	AND c.created_at=a.created_at
	AND c.updated_at=a.updated_at
)`).Scan(&mismatchedSigning); err != nil {
			return fmt.Errorf("verify legacy Signing Credential backfill: %w", err)
		}
		if mismatchedAPI != 0 || mismatchedSigning != 0 {
			return fmt.Errorf("legacy Integration Credential content mismatch: api=%d signing=%d", mismatchedAPI, mismatchedSigning)
		}
	} else {
		for _, column := range legacyColumns {
			exists, err := columnExists(ctx, tx, "workforce_integration_applications", column)
			if err != nil {
				return err
			}
			if exists {
				return fmt.Errorf("partial legacy Integration Credential schema contains %s", column)
			}
		}
	}

	var missingAPI, missingSigning int64
	if err := tx.QueryRow(ctx, `SELECT count(*) FROM workforce_integration_applications a WHERE NOT EXISTS (
SELECT 1 FROM workforce_integration_api_credentials c WHERE c.integration_application_id=a.id AND c.tenant_id=a.tenant_id AND c.status='CURRENT')`).Scan(&missingAPI); err != nil {
		return err
	}
	if err := tx.QueryRow(ctx, `SELECT count(*) FROM workforce_integration_applications a WHERE a.webhook_url<>'' AND NOT EXISTS (
SELECT 1 FROM workforce_integration_signing_credentials c WHERE c.integration_application_id=a.id AND c.tenant_id=a.tenant_id AND c.status='CURRENT')`).Scan(&missingSigning); err != nil {
		return err
	}
	if missingAPI != 0 || missingSigning != 0 {
		return fmt.Errorf("Integration Credential backfill incomplete: api=%d signing=%d", missingAPI, missingSigning)
	}

	attemptsExist := false
	if err := tx.QueryRow(ctx, `SELECT to_regclass('public.workforce_webhook_delivery_attempts') IS NOT NULL`).Scan(&attemptsExist); err != nil {
		return err
	}
	if attemptsExist {
		if _, err := tx.Exec(ctx, `ALTER TABLE workforce_webhook_delivery_attempts
ADD COLUMN IF NOT EXISTS signing_credential_id BIGINT,
ADD COLUMN IF NOT EXISTS grace_signing_credential_id BIGINT;
UPDATE workforce_webhook_delivery_attempts a SET signing_credential_id=c.id
FROM workforce_integration_signing_credentials c
WHERE a.signing_credential_id IS NULL AND c.integration_application_id=a.integration_application_id AND c.tenant_id=a.tenant_id AND c.status='CURRENT'`); err != nil {
			return fmt.Errorf("align Webhook Delivery Attempt signing provenance: %w", err)
		}
		var missingAttemptCredential int64
		if err := tx.QueryRow(ctx, `SELECT count(*) FROM workforce_webhook_delivery_attempts WHERE signing_credential_id IS NULL`).Scan(&missingAttemptCredential); err != nil {
			return err
		}
		if missingAttemptCredential != 0 {
			return fmt.Errorf("Webhook Delivery Attempt signing provenance backfill left %d rows", missingAttemptCredential)
		}
		if _, err := tx.Exec(ctx, `ALTER TABLE workforce_webhook_delivery_attempts ALTER COLUMN signing_credential_id SET NOT NULL`); err != nil {
			return err
		}
	}

	if legacyPresent {
		if _, err := tx.Exec(ctx, `DROP INDEX IF EXISTS workforce_integration_applications_credential_locator_uq;
ALTER TABLE workforce_integration_applications
DROP COLUMN credential_locator,
DROP COLUMN credential_digest,
DROP COLUMN credential_hint,
DROP COLUMN signing_secret_ciphertext,
DROP COLUMN signing_secret_edek,
DROP COLUMN signing_secret_wrap_key_ref,
DROP COLUMN signing_secret_hint`); err != nil {
			return fmt.Errorf("drop verified legacy Integration Credential columns: %w", err)
		}
	}
	return nil
}

// repairTextIdentityExceptions reverses unpublished alignments that converted
// process-session or external protocol identities to BIGINT. Existing mapped
// values are restored from the durable legacy map after the column is made
// textual; native numeric values are preserved as their decimal text form.
func repairTextIdentityExceptions(ctx context.Context, tx pgx.Tx) error {
	exceptions := governedTextIdentityExceptions()
	keys := make([]string, 0, len(exceptions))
	for key := range exceptions {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		parts := strings.SplitN(key, ".", 2)
		if len(parts) != 2 {
			return fmt.Errorf("invalid TEXT identity exception %q", key)
		}
		table, column := parts[0], parts[1]
		var dataType string
		err := tx.QueryRow(ctx, `SELECT data_type FROM information_schema.columns WHERE table_schema='public' AND table_name=$1 AND column_name=$2`, table, column).Scan(&dataType)
		if errors.Is(err, pgx.ErrNoRows) {
			continue
		}
		if err != nil {
			return err
		}
		if dataType == "text" || dataType == "character varying" {
			continue
		}
		if _, err = tx.Exec(ctx, fmt.Sprintf(`ALTER TABLE %s ALTER COLUMN %s TYPE TEXT USING %s::TEXT`, ident(table), ident(column), ident(column))); err != nil {
			return fmt.Errorf("restore TEXT exception %s: %w", key, err)
		}
		if _, err = tx.Exec(ctx, fmt.Sprintf(`UPDATE %s target SET %s=m.legacy_identity FROM workforce_legacy_identity_map m WHERE m.entity_type='GLOBAL' AND target.%s=m.entity_id::TEXT`, ident(table), ident(column), ident(column))); err != nil {
			return fmt.Errorf("restore legacy values for TEXT exception %s: %w", key, err)
		}
	}
	return nil
}

func buildIdentityMap(ctx context.Context, tx pgx.Tx, idGen IDGenerator, columns []Column) (map[string]int64, error) {
	values := map[string]struct{}{}
	for _, column := range columns {
		query := fmt.Sprintf(`SELECT DISTINCT btrim(%s) FROM %s WHERE NULLIF(btrim(%s),'') IS NOT NULL`, ident(column.Name), ident(column.Table), ident(column.Name))
		rows, err := tx.Query(ctx, query)
		if err != nil {
			return nil, fmt.Errorf("read %s.%s identities: %w", column.Table, column.Name, err)
		}
		for rows.Next() {
			var value string
			if err = rows.Scan(&value); err != nil {
				rows.Close()
				return nil, err
			}
			values[value] = struct{}{}
		}
		rows.Close()
	}
	keys := make([]string, 0, len(values))
	for value := range values {
		keys = append(keys, value)
	}
	sort.Strings(keys)
	result := make(map[string]int64, len(keys))
	for _, value := range keys {
		id := idGen.NextID()
		if id < 1 {
			return nil, errors.New("Snowflake generator returned a non-positive ID")
		}
		result[value] = id
	}
	return result, nil
}

func persistLegacyEvidence(ctx context.Context, tx pgx.Tx, identities map[string]int64) (int64, error) {
	var count int64
	keys := make([]string, 0, len(identities))
	for key := range identities {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, legacy := range keys {
		tag, err := tx.Exec(ctx, `INSERT INTO workforce_legacy_identity_map(entity_type,legacy_identity,entity_id) VALUES ('GLOBAL',$1,$2) ON CONFLICT DO NOTHING`, legacy, identities[legacy])
		if err != nil {
			return count, err
		}
		count += tag.RowsAffected()
	}
	return count, nil
}

func alignIdentityColumn(ctx context.Context, tx pgx.Tx, column Column) error {
	table, name := ident(column.Table), ident(column.Name)
	if _, err := tx.Exec(ctx, fmt.Sprintf(`ALTER TABLE %s ALTER COLUMN %s DROP DEFAULT`, table, name)); err != nil {
		return fmt.Errorf("drop default %s.%s: %w", column.Table, column.Name, err)
	}
	if _, err := tx.Exec(ctx, fmt.Sprintf(`UPDATE %s target SET %s=m.entity_id::TEXT FROM workforce_legacy_identity_map m WHERE m.entity_type='GLOBAL' AND NULLIF(btrim(target.%s),'')=m.legacy_identity`, table, name, name)); err != nil {
		return fmt.Errorf("map %s.%s: %w", column.Table, column.Name, err)
	}
	var empty int64
	if err := tx.QueryRow(ctx, fmt.Sprintf(`SELECT count(*) FROM %s WHERE btrim(%s)=''`, table, name)).Scan(&empty); err != nil {
		return err
	}
	if empty != 0 {
		if _, err := tx.Exec(ctx, fmt.Sprintf(`ALTER TABLE %s ALTER COLUMN %s DROP NOT NULL`, table, name)); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, fmt.Sprintf(`UPDATE %s SET %s=NULL WHERE btrim(%s)=''`, table, name, name)); err != nil {
			return err
		}
	}
	if _, err := tx.Exec(ctx, fmt.Sprintf(`ALTER TABLE %s ALTER COLUMN %s TYPE BIGINT USING %s::BIGINT`, table, name, name)); err != nil {
		return fmt.Errorf("convert %s.%s to BIGINT: %w", column.Table, column.Name, err)
	}
	return nil
}

func alignTenants(ctx context.Context, tx pgx.Tx, columns []Column) (int64, error) {
	var changed int64
	for _, column := range columns {
		if column.Name != "tenant_id" {
			continue
		}
		table := ident(column.Table)
		if _, systemCapable := systemScopeTables[column.Table]; systemCapable {
			if _, err := tx.Exec(ctx, fmt.Sprintf(`ALTER TABLE %s ADD COLUMN IF NOT EXISTS scope_type TEXT`, table)); err != nil {
				return changed, err
			}
			if _, err := tx.Exec(ctx, fmt.Sprintf(`ALTER TABLE %s ALTER COLUMN tenant_id DROP NOT NULL`, table)); err != nil {
				return changed, err
			}
			if _, err := tx.Exec(ctx, fmt.Sprintf(`UPDATE %s SET scope_type=CASE WHEN tenant_id='_system' THEN 'SYSTEM' ELSE 'TENANT' END WHERE scope_type IS NULL OR scope_type=''`, table)); err != nil {
				return changed, err
			}
		}
		rows, err := tx.Query(ctx, fmt.Sprintf(`SELECT DISTINCT tenant_id FROM %s WHERE tenant_id IS NOT NULL`, table))
		if err != nil {
			return changed, err
		}
		var values []string
		for rows.Next() {
			var value string
			if err = rows.Scan(&value); err != nil {
				rows.Close()
				return changed, err
			}
			values = append(values, value)
		}
		rows.Close()
		for _, value := range values {
			if value == "_system" {
				if _, ok := systemScopeTables[column.Table]; !ok {
					return changed, fmt.Errorf("%s uses _system without explicit SYSTEM scope", column.Table)
				}
				tag, execErr := tx.Exec(ctx, fmt.Sprintf(`UPDATE %s SET tenant_id=NULL,scope_type='SYSTEM' WHERE tenant_id='_system'`, table))
				if execErr != nil {
					return changed, execErr
				}
				changed += tag.RowsAffected()
				continue
			}
			digits := strings.TrimPrefix(value, "org:")
			orgID, parseErr := strconv.ParseInt(digits, 10, 64)
			if parseErr != nil || orgID < 1 {
				return changed, fmt.Errorf("invalid tenant identity %q in %s", value, column.Table)
			}
			var exists bool
			if err = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM sys_org WHERE id=$1)`, orgID).Scan(&exists); err != nil || !exists {
				if err == nil {
					err = fmt.Errorf("organization %d does not exist", orgID)
				}
				return changed, err
			}
			tag, execErr := tx.Exec(ctx, fmt.Sprintf(`UPDATE %s SET tenant_id=$1 WHERE tenant_id=$2`, table), strconv.FormatInt(orgID, 10), value)
			if execErr != nil {
				return changed, execErr
			}
			changed += tag.RowsAffected()
		}
		if _, err = tx.Exec(ctx, fmt.Sprintf(`ALTER TABLE %s ALTER COLUMN tenant_id DROP DEFAULT`, table)); err != nil {
			return changed, err
		}
		if _, ok := systemScopeTables[column.Table]; ok {
			if _, err = tx.Exec(ctx, fmt.Sprintf(`ALTER TABLE %s ALTER COLUMN tenant_id DROP NOT NULL`, table)); err != nil {
				return changed, err
			}
		}
		if _, err = tx.Exec(ctx, fmt.Sprintf(`ALTER TABLE %s ALTER COLUMN tenant_id TYPE BIGINT USING tenant_id::BIGINT`, table)); err != nil {
			return changed, fmt.Errorf("convert %s.tenant_id: %w", column.Table, err)
		}
	}
	return changed, nil
}

func alignPrincipals(ctx context.Context, tx pgx.Tx, idGen IDGenerator, identityMap map[string]int64) (int64, error) {
	var changed int64
	for table, mappings := range principalColumns {
		if table == "workforce_completion_check_decisions" {
			count, err := alignCompletionDecisionPrincipals(ctx, tx, idGen)
			if err != nil {
				return changed, err
			}
			changed += count
			continue
		}
		for legacy, target := range mappings {
			legacyExists, err := columnExists(ctx, tx, table, legacy)
			if err != nil || !legacyExists {
				if err != nil {
					return changed, err
				}
				continue
			}
			targetExists, err := columnExists(ctx, tx, table, target)
			if err != nil {
				return changed, err
			}
			if !targetExists {
				if _, err = tx.Exec(ctx, fmt.Sprintf(`ALTER TABLE %s ADD COLUMN %s BIGINT`, ident(table), ident(target))); err != nil {
					return changed, err
				}
			}
			rows, err := tx.Query(ctx, fmt.Sprintf(`SELECT DISTINCT COALESCE(%s,'') FROM %s`, ident(legacy), ident(table)))
			if err != nil {
				return changed, err
			}
			var actors []string
			for rows.Next() {
				var actor string
				if err = rows.Scan(&actor); err != nil {
					rows.Close()
					return changed, err
				}
				actors = append(actors, actor)
			}
			rows.Close()
			for _, actor := range actors {
				principalID, resolveErr := resolvePrincipal(ctx, tx, idGen, identityMap, actor)
				if resolveErr != nil {
					return changed, fmt.Errorf("resolve %s.%s actor %q: %w", table, legacy, actor, resolveErr)
				}
				tag, execErr := tx.Exec(ctx, fmt.Sprintf(`UPDATE %s SET %s=$1 WHERE COALESCE(%s,'')=$2`, ident(table), ident(target), ident(legacy)), principalID, actor)
				if execErr != nil {
					return changed, execErr
				}
				changed += tag.RowsAffected()
			}
			if _, err = tx.Exec(ctx, fmt.Sprintf(`ALTER TABLE %s DROP COLUMN %s`, ident(table), ident(legacy))); err != nil {
				return changed, err
			}
		}
	}
	for table, mappings := range principalColumns {
		for _, target := range mappings {
			if _, optional := optionalPrincipalColumns[table+"."+target]; optional {
				continue
			}
			exists, err := columnExists(ctx, tx, table, target)
			if err != nil {
				return changed, err
			}
			if !exists {
				continue
			}
			var missing int64
			if err = tx.QueryRow(ctx, fmt.Sprintf(`SELECT count(*) FROM %s WHERE %s IS NULL`, ident(table), ident(target))).Scan(&missing); err != nil {
				return changed, err
			}
			if missing != 0 {
				return changed, fmt.Errorf("%s.%s has %d unresolved principals", table, target, missing)
			}
			if _, err = tx.Exec(ctx, fmt.Sprintf(`ALTER TABLE %s ALTER COLUMN %s SET NOT NULL`, ident(table), ident(target))); err != nil {
				return changed, err
			}
		}
	}
	return changed, nil
}

func enforceScopeContracts(ctx context.Context, tx pgx.Tx) error {
	for table := range systemScopeTables {
		tableExists, err := relationExists(ctx, tx, table)
		if err != nil {
			return err
		}
		// Alignment may run against a legacy database that predates one or more
		// unpublished Workforce migrations.  It may repair a relation that is
		// already present, but must never pre-create part of a later migration's
		// schema merely because that relation is eligible for SYSTEM scope.
		if !tableExists {
			continue
		}
		exists, err := columnExists(ctx, tx, table, "scope_type")
		if err != nil {
			return err
		}
		if !exists {
			if _, err = tx.Exec(ctx, fmt.Sprintf(`ALTER TABLE %s ADD COLUMN scope_type TEXT`, ident(table))); err != nil {
				return err
			}
		}
		hasTenant, err := columnExists(ctx, tx, table, "tenant_id")
		if err != nil {
			return err
		}
		name := "ck_" + strings.TrimPrefix(table, "workforce_") + "_scope_tenant"
		var constraintExists bool
		if err = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM pg_constraint WHERE conrelid=$1::regclass AND conname=$2)`, table, name).Scan(&constraintExists); err != nil {
			return err
		}
		update := fmt.Sprintf(`UPDATE %s SET scope_type='SYSTEM' WHERE scope_type IS NULL OR scope_type=''`, ident(table))
		if hasTenant {
			update = fmt.Sprintf(`UPDATE %s SET scope_type=CASE WHEN tenant_id IS NULL THEN 'SYSTEM' ELSE 'TENANT' END WHERE scope_type IS NULL OR scope_type=''`, ident(table))
		}
		if _, err = tx.Exec(ctx, update); err != nil {
			return err
		}
		if _, err = tx.Exec(ctx, fmt.Sprintf(`ALTER TABLE %s ALTER COLUMN scope_type SET NOT NULL`, ident(table))); err != nil {
			return err
		}
		if !constraintExists {
			check := `scope_type='SYSTEM'`
			if hasTenant {
				check = `(scope_type='SYSTEM' AND tenant_id IS NULL) OR (scope_type='TENANT' AND tenant_id IS NOT NULL)`
			}
			if _, err = tx.Exec(ctx, fmt.Sprintf(`ALTER TABLE %s ADD CONSTRAINT %s CHECK (%s)`, ident(table), ident(name), check)); err != nil {
				return err
			}
		}
	}
	return nil
}

func resolvePrincipal(ctx context.Context, tx pgx.Tx, idGen IDGenerator, identityMap map[string]int64, raw string) (int64, error) {
	value := strings.TrimSpace(raw)
	if value == "" || value == "system" || value == "bootstrap" || value == "acceptance" || value == "local-worker-1" || strings.HasPrefix(value, "p09-") || strings.HasPrefix(value, "system:") {
		return ensureService(ctx, tx, idGen, defaultService(value))
	}
	if strings.HasPrefix(value, "worker:") {
		legacy := strings.TrimPrefix(value, "worker:")
		workerID := identityMap[legacy]
		if workerID < 1 {
			return 0, errors.New("unknown Worker actor")
		}
		return ensureWorker(ctx, tx, idGen, workerID)
	}
	if strings.HasPrefix(value, "wrk_") {
		workerID := identityMap[value]
		if workerID < 1 {
			return 0, errors.New("unknown Worker actor")
		}
		return ensureWorker(ctx, tx, idGen, workerID)
	}
	if strings.HasPrefix(value, "user:") {
		value = strings.TrimPrefix(value, "user:")
	}
	if userID, err := strconv.ParseInt(value, 10, 64); err == nil && userID > 0 {
		return ensureUser(ctx, tx, idGen, userID)
	}
	var userID int64
	err := tx.QueryRow(ctx, `SELECT id FROM sys_user WHERE lower("userAccount")=lower($1) AND "isDeleted"=false`, value).Scan(&userID)
	if err == nil {
		return ensureUser(ctx, tx, idGen, userID)
	}
	if errors.Is(err, pgx.ErrNoRows) {
		return 0, errors.New("unknown actor")
	}
	return 0, err
}

func defaultService(value string) string {
	if value == "" {
		return "system"
	}
	return value
}
func ensureService(ctx context.Context, tx pgx.Tx, idGen IDGenerator, code string) (int64, error) {
	var id int64
	err := tx.QueryRow(ctx, `SELECT id FROM workforce_principals WHERE principal_type='SERVICE' AND service_code=$1`, code).Scan(&id)
	if err == nil {
		return id, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return 0, err
	}
	id = idGen.NextID()
	err = tx.QueryRow(ctx, `INSERT INTO workforce_principals(id,principal_type,service_code) VALUES($1,'SERVICE',$2) ON CONFLICT (service_code) WHERE principal_type='SERVICE' DO UPDATE SET service_code=EXCLUDED.service_code RETURNING id`, id, code).Scan(&id)
	return id, err
}
func ensureUser(ctx context.Context, tx pgx.Tx, idGen IDGenerator, userID int64) (int64, error) {
	var ok bool
	if err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM sys_user WHERE id=$1 AND "isDeleted"=false)`, userID).Scan(&ok); err != nil || !ok {
		if err == nil {
			err = errors.New("user does not exist")
		}
		return 0, err
	}
	var id int64
	err := tx.QueryRow(ctx, `SELECT id FROM workforce_principals WHERE principal_type='USER' AND user_id=$1`, userID).Scan(&id)
	if err == nil {
		return id, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return 0, err
	}
	id = idGen.NextID()
	err = tx.QueryRow(ctx, `INSERT INTO workforce_principals(id,principal_type,user_id) VALUES($1,'USER',$2) ON CONFLICT (user_id) WHERE principal_type='USER' DO UPDATE SET user_id=EXCLUDED.user_id RETURNING id`, id, userID).Scan(&id)
	return id, err
}
func ensureWorker(ctx context.Context, tx pgx.Tx, idGen IDGenerator, workerID int64) (int64, error) {
	var id int64
	err := tx.QueryRow(ctx, `SELECT id FROM workforce_principals WHERE principal_type='WORKER' AND worker_id=$1`, workerID).Scan(&id)
	if err == nil {
		return id, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return 0, err
	}
	id = idGen.NextID()
	err = tx.QueryRow(ctx, `INSERT INTO workforce_principals(id,principal_type,worker_id) VALUES($1,'WORKER',$2) ON CONFLICT (worker_id) WHERE principal_type='WORKER' DO UPDATE SET worker_id=EXCLUDED.worker_id RETURNING id`, id, workerID).Scan(&id)
	return id, err
}

func alignModelCredentialStorage(ctx context.Context, tx pgx.Tx) error {
	exists, err := relationExists(ctx, tx, "workforce_model_credentials")
	if err != nil || !exists {
		return err
	}
	changes := []struct {
		column, postgresType, udtName, using string
	}{
		{"name", "VARCHAR(100)", "varchar", "name::VARCHAR(100)"},
		{"protocol", "VARCHAR(16)", "varchar", "protocol::VARCHAR(16)"},
		{"base_url", "VARCHAR(2048)", "varchar", "base_url::VARCHAR(2048)"},
		{"secret_ciphertext", "BYTEA", "bytea", "decode(secret_ciphertext, 'base64')"},
		{"secret_edek", "BYTEA", "bytea", "decode(secret_edek, 'base64')"},
		{"secret_wrap_key_ref", "VARCHAR(256)", "varchar", "secret_wrap_key_ref::VARCHAR(256)"},
		{"secret_hint", "VARCHAR(16)", "varchar", "secret_hint::VARCHAR(16)"},
		{"secret_version", "INTEGER", "int4", "secret_version::INTEGER"},
		{"status", "VARCHAR(16)", "varchar", "status::VARCHAR(16)"},
		{"version", "INTEGER", "int4", "version::INTEGER"},
		{"last_discovery_status", "VARCHAR(32)", "varchar", "last_discovery_status::VARCHAR(32)"},
		{"last_discovery_reason_code", "VARCHAR(128)", "varchar", "last_discovery_reason_code::VARCHAR(128)"},
	}
	for _, change := range changes {
		if err = alterColumnType(ctx, tx, "workforce_model_credentials", change.column, change.postgresType, change.udtName, change.using); err != nil {
			return fmt.Errorf("align model credential column %s: %w", change.column, err)
		}
	}
	return nil
}

func alignModelCredentialReferences(ctx context.Context, tx pgx.Tx) error {
	changes := []struct {
		table, column, postgresType, udtName, using string
	}{
		{"workforce_task_assignments", "credential_version", "INTEGER", "int4", "credential_version::INTEGER"},
		{"workforce_task_assignments", "model_protocol", "VARCHAR(16)", "varchar", "model_protocol::VARCHAR(16)"},
		{"workforce_task_assignments", "primary_model", "VARCHAR(256)", "varchar", "primary_model::VARCHAR(256)"},
		{"workforce_tasks", "primary_model", "VARCHAR(256)", "varchar", "primary_model::VARCHAR(256)"},
		{"workforce_review_runs", "credential_version", "INTEGER", "int4", "credential_version::INTEGER"},
		{"workforce_review_runs", "model_protocol", "VARCHAR(16)", "varchar", "model_protocol::VARCHAR(16)"},
		{"workforce_review_runs", "primary_model", "VARCHAR(256)", "varchar", "primary_model::VARCHAR(256)"},
		{"workforce_review_run_attempts", "credential_version", "INTEGER", "int4", "credential_version::INTEGER"},
		{"workforce_review_run_attempts", "model_protocol", "VARCHAR(16)", "varchar", "model_protocol::VARCHAR(16)"},
		{"workforce_review_run_attempts", "primary_model", "VARCHAR(256)", "varchar", "primary_model::VARCHAR(256)"},
	}
	for _, change := range changes {
		exists, err := relationExists(ctx, tx, change.table)
		if err != nil {
			return err
		}
		if !exists {
			continue
		}
		if err = alterColumnType(ctx, tx, change.table, change.column, change.postgresType, change.udtName, change.using); err != nil {
			return fmt.Errorf("align %s.%s: %w", change.table, change.column, err)
		}
	}
	return nil
}

func alignModelInvocationIdentity(ctx context.Context, tx pgx.Tx, idGen IDGenerator) error {
	exists, err := relationExists(ctx, tx, "workforce_model_invocations")
	if err != nil || !exists {
		return err
	}
	if idGen == nil {
		return errors.New("model invocation alignment requires an ID generator")
	}
	var hasLegacyKey, hasProtocolKey, hasID bool
	if err = tx.QueryRow(ctx, `SELECT
		EXISTS(SELECT 1 FROM information_schema.columns WHERE table_schema='public' AND table_name='workforce_model_invocations' AND column_name='invocation_id'),
		EXISTS(SELECT 1 FROM information_schema.columns WHERE table_schema='public' AND table_name='workforce_model_invocations' AND column_name='protocol_invocation_key'),
		EXISTS(SELECT 1 FROM information_schema.columns WHERE table_schema='public' AND table_name='workforce_model_invocations' AND column_name='id')`).Scan(&hasLegacyKey, &hasProtocolKey, &hasID); err != nil {
		return err
	}
	if hasLegacyKey && !hasProtocolKey {
		if _, err = tx.Exec(ctx, `ALTER TABLE workforce_model_invocations RENAME COLUMN invocation_id TO protocol_invocation_key`); err != nil {
			return fmt.Errorf("rename model invocation protocol key: %w", err)
		}
		hasProtocolKey = true
	}
	if !hasID {
		if _, err = tx.Exec(ctx, `ALTER TABLE workforce_model_invocations ADD COLUMN id BIGINT`); err != nil {
			return fmt.Errorf("add model invocation ID: %w", err)
		}
	}
	rows, err := tx.Query(ctx, `SELECT ctid::text FROM workforce_model_invocations WHERE id IS NULL ORDER BY ctid`)
	if err != nil {
		return err
	}
	var rowLocators []string
	for rows.Next() {
		var locator string
		if err = rows.Scan(&locator); err != nil {
			rows.Close()
			return err
		}
		rowLocators = append(rowLocators, locator)
	}
	if err = rows.Err(); err != nil {
		rows.Close()
		return err
	}
	rows.Close()
	for _, locator := range rowLocators {
		id := idGen.NextID()
		if id <= 0 {
			return errors.New("model invocation alignment generated an invalid ID")
		}
		if _, err = tx.Exec(ctx, `UPDATE workforce_model_invocations SET id=$1 WHERE ctid=$2::tid AND id IS NULL`, id, locator); err != nil {
			return fmt.Errorf("assign model invocation ID: %w", err)
		}
	}
	var primaryConstraint, primaryDefinition string
	err = tx.QueryRow(ctx, `SELECT conname,pg_get_constraintdef(oid) FROM pg_constraint WHERE conrelid='workforce_model_invocations'::regclass AND contype='p'`).Scan(&primaryConstraint, &primaryDefinition)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return err
	}
	if err == nil && !strings.Contains(strings.ToLower(primaryDefinition), "(id)") {
		if _, err = tx.Exec(ctx, `ALTER TABLE workforce_model_invocations DROP CONSTRAINT `+pgx.Identifier{primaryConstraint}.Sanitize()); err != nil {
			return fmt.Errorf("replace model invocation primary key: %w", err)
		}
		primaryConstraint = ""
	}
	if _, err = tx.Exec(ctx, `ALTER TABLE workforce_model_invocations ALTER COLUMN id SET NOT NULL`); err != nil {
		return err
	}
	if primaryConstraint == "" {
		if _, err = tx.Exec(ctx, `ALTER TABLE workforce_model_invocations ADD CONSTRAINT workforce_model_invocations_pkey PRIMARY KEY (id)`); err != nil {
			return fmt.Errorf("create model invocation primary key: %w", err)
		}
	}
	changes := []struct {
		column, postgresType, udtName, using string
	}{
		{"protocol_invocation_key", "VARCHAR(64)", "varchar", "protocol_invocation_key::VARCHAR(64)"},
		{"execution_kind", "VARCHAR(32)", "varchar", "execution_kind::VARCHAR(32)"},
		{"generation", "INTEGER", "int4", "generation::INTEGER"},
		{"owner_epoch", "INTEGER", "int4", "owner_epoch::INTEGER"},
		{"worker_instance_id", "VARCHAR(128)", "varchar", "worker_instance_id::VARCHAR(128)"},
		{"host_boot_id", "VARCHAR(128)", "varchar", "host_boot_id::VARCHAR(128)"},
		{"credential_version", "INTEGER", "int4", "credential_version::INTEGER"},
		{"protocol", "VARCHAR(16)", "varchar", "protocol::VARCHAR(16)"},
		{"primary_model", "VARCHAR(256)", "varchar", "primary_model::VARCHAR(256)"},
		{"requested_model", "VARCHAR(256)", "varchar", "requested_model::VARCHAR(256)"},
		{"response_model", "VARCHAR(256)", "varchar", "response_model::VARCHAR(256)"},
		{"operation", "VARCHAR(32)", "varchar", "operation::VARCHAR(32)"},
		{"request_body_sha256", "BYTEA", "bytea", "decode(request_body_sha256, 'hex')"},
		{"nonce_sha256", "BYTEA", "bytea", "decode(nonce_sha256, 'hex')"},
		{"status", "VARCHAR(32)", "varchar", "status::VARCHAR(32)"},
		{"upstream_status_code", "SMALLINT", "int2", "upstream_status_code::SMALLINT"},
		{"upstream_request_id", "VARCHAR(256)", "varchar", "upstream_request_id::VARCHAR(256)"},
		{"duration_milliseconds", "INTEGER", "int4", "duration_milliseconds::INTEGER"},
		{"input_tokens", "INTEGER", "int4", "input_tokens::INTEGER"},
		{"cached_input_tokens", "INTEGER", "int4", "cached_input_tokens::INTEGER"},
		{"cache_creation_input_tokens", "INTEGER", "int4", "cache_creation_input_tokens::INTEGER"},
		{"output_tokens", "INTEGER", "int4", "output_tokens::INTEGER"},
		{"reasoning_output_tokens", "INTEGER", "int4", "reasoning_output_tokens::INTEGER"},
		{"total_tokens", "INTEGER", "int4", "total_tokens::INTEGER"},
		{"error_code", "VARCHAR(128)", "varchar", "error_code::VARCHAR(128)"},
	}
	for _, change := range changes {
		if err = alterColumnType(ctx, tx, "workforce_model_invocations", change.column, change.postgresType, change.udtName, change.using); err != nil {
			return fmt.Errorf("align model invocation column %s: %w", change.column, err)
		}
	}
	if hasProtocolKey {
		if _, err = tx.Exec(ctx, `DO $$ BEGIN IF NOT EXISTS (
			SELECT 1 FROM pg_constraint
			WHERE conrelid='workforce_model_invocations'::regclass
			  AND contype='u'
			  AND lower(pg_get_constraintdef(oid)) LIKE 'unique (tenant_id, protocol_invocation_key)%'
		) THEN ALTER TABLE workforce_model_invocations
			ADD CONSTRAINT workforce_model_invocations_protocol_key_uk UNIQUE (tenant_id,protocol_invocation_key);
		END IF; END $$`); err != nil {
			return err
		}
	}
	return nil
}

func alterColumnType(ctx context.Context, tx pgx.Tx, table, column, postgresType, desiredUDT, usingExpression string) error {
	var currentUDT string
	err := tx.QueryRow(ctx, `SELECT udt_name FROM information_schema.columns WHERE table_schema='public' AND table_name=$1 AND column_name=$2`, table, column).Scan(&currentUDT)
	if errors.Is(err, pgx.ErrNoRows) || currentUDT == desiredUDT {
		return nil
	}
	if err != nil {
		return err
	}
	statement := `ALTER TABLE ` + pgx.Identifier{table}.Sanitize() + ` ALTER COLUMN ` + pgx.Identifier{column}.Sanitize() + ` TYPE ` + postgresType + ` USING ` + usingExpression
	_, err = tx.Exec(ctx, statement)
	return err
}

// workerFleetControlPlanePresent distinguishes an older identity-only
// Workforce database from a database that has already applied the Worker
// Fleet migration. Alignment is intentionally not a replacement for Goose:
// before Fleet exists it must not pre-create ReviewRun relations that collide
// with the authoritative migration later on.
func workerFleetControlPlanePresent(ctx context.Context, q interface {
	QueryRow(context.Context, string, ...any) pgx.Row
}) (bool, error) {
	var present bool
	err := q.QueryRow(ctx, `SELECT NOT EXISTS (
		SELECT 1
		FROM unnest(ARRAY[
			'workforce_workers',
			'workforce_worker_tokens',
			'workforce_worker_sessions',
			'workforce_worker_images'
		]::text[]) AS required(table_name)
		WHERE to_regclass('public.' || required.table_name) IS NULL
	)`).Scan(&present)
	return present, err
}

// ensureProviderInvocationSchema is the one-time, repeatable hard cut from the
// unpublished Provider Profile schema to Model Credentials. Runtime code only
// reads the new names; this alignment preserves the current development data
// without introducing a dual-read compatibility path.
func ensureProviderInvocationSchema(ctx context.Context, tx pgx.Tx, idGen IDGenerator) error {
	legacy, err := relationExists(ctx, tx, "workforce_provider_profiles")
	if err != nil {
		return err
	}
	if legacy {
		var active bool
		if err := tx.QueryRow(ctx, `SELECT
			EXISTS(SELECT 1 FROM workforce_tasks WHERE status NOT IN ('SUCCEEDED','FAILED','CANCELLED','TIMED_OUT','MANUAL_REVIEW'))
			OR EXISTS(SELECT 1 FROM workforce_task_attempts WHERE status NOT IN ('SUCCEEDED','FAILED','CANCELLED','TIMED_OUT','MANUAL_REVIEW'))
			OR EXISTS(SELECT 1 FROM workforce_review_runs WHERE status NOT IN ('PASSED','REVISION_REQUIRED','REJECTED','MANUAL_REVIEW','CANCELLED'))`).Scan(&active); err != nil {
			return err
		}
		if !active {
			if err := tx.QueryRow(ctx, `SELECT EXISTS(
				SELECT 1
				FROM workforce_provider_invocations
				WHERE status IN ('AUTHORIZED','STREAMING')
			)`).Scan(&active); err != nil {
				return err
			}
		}
		if active {
			return errors.New("MODEL_CREDENTIAL_HARD_CUT_ACTIVE_EXECUTION")
		}
	}
	if idGen == nil {
		return errors.New("model credential alignment requires an ID generator")
	}
	if _, err = tx.Exec(ctx, `DO $$ BEGIN IF to_regclass('public.workforce_provider_profiles') IS NOT NULL AND to_regclass('public.workforce_model_credentials') IS NULL THEN ALTER TABLE workforce_provider_profiles RENAME TO workforce_model_credentials; END IF; IF to_regclass('public.workforce_provider_invocations') IS NOT NULL AND to_regclass('public.workforce_model_invocations') IS NULL THEN ALTER TABLE workforce_provider_invocations RENAME TO workforce_model_invocations; END IF; END $$`); err != nil {
		return fmt.Errorf("align model credential table names: %w", err)
	}
	if err = alignModelCredentialStorage(ctx, tx); err != nil {
		return err
	}
	if err = alignModelInvocationIdentity(ctx, tx, idGen); err != nil {
		return err
	}
	statements := []string{
		`DO $$ BEGIN IF EXISTS(SELECT 1 FROM information_schema.columns WHERE table_schema='public' AND table_name='workforce_model_credentials' AND column_name='allowed_models_json') THEN ALTER TABLE workforce_model_credentials RENAME COLUMN allowed_models_json TO models_json; END IF; END $$`,
		`ALTER TABLE workforce_model_credentials DROP CONSTRAINT IF EXISTS workforce_provider_profiles_protocol_check`,
		`ALTER TABLE workforce_model_credentials DROP CONSTRAINT IF EXISTS workforce_model_credentials_protocol_check`,
		`ALTER TABLE workforce_model_credentials ADD COLUMN IF NOT EXISTS secret_version INTEGER NOT NULL DEFAULT 1`,
		`ALTER TABLE workforce_model_credentials ADD COLUMN IF NOT EXISTS last_discovery_status VARCHAR(32) NOT NULL DEFAULT ''`,
		`ALTER TABLE workforce_model_credentials ADD COLUMN IF NOT EXISTS last_discovery_reason_code VARCHAR(128) NOT NULL DEFAULT ''`,
		`ALTER TABLE workforce_model_credentials ADD COLUMN IF NOT EXISTS last_discovered_at TIMESTAMPTZ`,
		`UPDATE workforce_model_credentials SET protocol=CASE WHEN protocol='OPENAI_RESPONSES' THEN 'OPENAI' ELSE protocol END`,
		`DO $$ BEGIN IF EXISTS(SELECT 1 FROM information_schema.columns WHERE table_schema='public' AND table_name='workforce_model_credentials' AND column_name='default_model') THEN UPDATE workforce_model_credentials SET models_json=COALESCE(models_json,'[]'::jsonb) || jsonb_build_array(default_model) WHERE btrim(default_model)<>'' AND NOT COALESCE(models_json,'[]'::jsonb) @> jsonb_build_array(default_model); END IF; END $$`,
		`UPDATE workforce_model_credentials SET models_json=(SELECT COALESCE(jsonb_agg(value ORDER BY ordinal),'[]'::jsonb) FROM (SELECT value,MIN(ordinal) ordinal FROM jsonb_array_elements_text(COALESCE(models_json,'[]'::jsonb)) WITH ORDINALITY AS values(value,ordinal) WHERE btrim(value)<>'' GROUP BY value) ordered)`,
		`ALTER TABLE workforce_model_credentials DROP COLUMN IF EXISTS alias CASCADE`,
		`ALTER TABLE workforce_model_credentials DROP COLUMN IF EXISTS default_model`,
		`ALTER TABLE workforce_model_credentials DROP COLUMN IF EXISTS capabilities_json`,
		`ALTER TABLE workforce_model_credentials DROP COLUMN IF EXISTS routing_priority`,
		`ALTER TABLE workforce_model_credentials DROP COLUMN IF EXISTS execution_roles_json`,
		`ALTER TABLE workforce_model_credentials DROP COLUMN IF EXISTS max_concurrent_invocations`,
		`ALTER TABLE workforce_model_credentials DROP COLUMN IF EXISTS request_timeout_millis`,
		`ALTER TABLE workforce_model_credentials DROP COLUMN IF EXISTS config_digest`,
		`ALTER TABLE workforce_model_credentials ADD CONSTRAINT workforce_model_credentials_protocol_check CHECK (protocol IN ('OPENAI','ANTHROPIC'))`,
		`ALTER TABLE workforce_model_credentials DROP CONSTRAINT IF EXISTS workforce_provider_profiles_tenant_id_name_key`,
		`ALTER TABLE workforce_model_credentials DROP CONSTRAINT IF EXISTS workforce_model_credentials_tenant_id_name_key`,
		`ALTER TABLE workforce_model_credentials DROP CONSTRAINT IF EXISTS workforce_provider_profiles_status_check`,
		`ALTER TABLE workforce_model_credentials DROP CONSTRAINT IF EXISTS workforce_model_credentials_status_check`,
		`ALTER TABLE workforce_model_credentials DROP CONSTRAINT IF EXISTS workforce_model_credentials_status_ck`,
		`ALTER TABLE workforce_model_credentials DROP CONSTRAINT IF EXISTS workforce_model_credentials_secret_ciphertext_check`,
		`ALTER TABLE workforce_model_credentials DROP CONSTRAINT IF EXISTS workforce_model_credentials_secret_edek_check`,
		`ALTER TABLE workforce_model_credentials DROP CONSTRAINT IF EXISTS workforce_model_credentials_secret_state_ck`,
		`ALTER TABLE workforce_model_credentials ALTER COLUMN secret_ciphertext DROP NOT NULL`,
		`ALTER TABLE workforce_model_credentials ALTER COLUMN secret_edek DROP NOT NULL`,
		`ALTER TABLE workforce_model_credentials ALTER COLUMN secret_wrap_key_ref DROP NOT NULL`,
		`ALTER TABLE workforce_model_credentials ALTER COLUMN secret_hint SET DEFAULT ''`,
		`ALTER TABLE workforce_model_credentials ADD CONSTRAINT workforce_model_credentials_status_ck CHECK (status IN ('ACTIVE','DISABLED','DELETED'))`,
		`ALTER TABLE workforce_model_credentials ADD CONSTRAINT workforce_model_credentials_secret_state_ck CHECK ((status='DELETED' AND secret_ciphertext IS NULL AND secret_edek IS NULL AND secret_wrap_key_ref IS NULL AND secret_hint='') OR (status IN ('ACTIVE','DISABLED') AND secret_ciphertext IS NOT NULL AND secret_edek IS NOT NULL AND secret_wrap_key_ref IS NOT NULL AND octet_length(secret_ciphertext) BETWEEN 1 AND 16384 AND octet_length(secret_edek) BETWEEN 1 AND 4096 AND btrim(secret_wrap_key_ref)<>''))`,
		`DROP INDEX IF EXISTS workforce_model_credentials_tenant_name_uidx`,
		`CREATE UNIQUE INDEX workforce_model_credentials_tenant_name_uidx ON workforce_model_credentials(tenant_id,name) WHERE status<>'DELETED'`,
		`DO $$ BEGIN IF EXISTS(SELECT 1 FROM information_schema.columns WHERE table_schema='public' AND table_name='workforce_model_invocations' AND column_name='provider_profile_id') THEN ALTER TABLE workforce_model_invocations RENAME COLUMN provider_profile_id TO model_credential_id; END IF; IF EXISTS(SELECT 1 FROM information_schema.columns WHERE table_schema='public' AND table_name='workforce_model_invocations' AND column_name='model') THEN ALTER TABLE workforce_model_invocations RENAME COLUMN model TO requested_model; END IF; END $$`,
		`ALTER TABLE workforce_model_invocations ADD COLUMN IF NOT EXISTS credential_version INTEGER`,
		`ALTER TABLE workforce_model_invocations ADD COLUMN IF NOT EXISTS protocol VARCHAR(16)`,
		`ALTER TABLE workforce_model_invocations ADD COLUMN IF NOT EXISTS primary_model VARCHAR(256)`,
		`ALTER TABLE workforce_model_invocations ADD COLUMN IF NOT EXISTS response_model VARCHAR(256)`,
		`ALTER TABLE workforce_model_invocations ADD COLUMN IF NOT EXISTS is_primary_model BOOLEAN`,
		`ALTER TABLE workforce_model_invocations ADD COLUMN IF NOT EXISTS cached_input_tokens INTEGER`,
		`ALTER TABLE workforce_model_invocations ADD COLUMN IF NOT EXISTS cache_creation_input_tokens INTEGER`,
		`ALTER TABLE workforce_model_invocations ADD COLUMN IF NOT EXISTS reasoning_output_tokens INTEGER`,
		`ALTER TABLE workforce_model_invocations ADD COLUMN IF NOT EXISTS usage_json JSONB`,
		`UPDATE workforce_model_invocations i SET credential_version=c.secret_version,protocol=c.protocol,primary_model=i.requested_model,is_primary_model=TRUE FROM workforce_model_credentials c WHERE c.id=i.model_credential_id AND (i.credential_version IS NULL OR i.protocol IS NULL OR i.primary_model IS NULL OR i.is_primary_model IS NULL)`,
		`ALTER TABLE workforce_model_invocations ALTER COLUMN credential_version SET NOT NULL`,
		`ALTER TABLE workforce_model_invocations ALTER COLUMN protocol SET NOT NULL`,
		`ALTER TABLE workforce_model_invocations ALTER COLUMN primary_model SET NOT NULL`,
		`ALTER TABLE workforce_model_invocations ALTER COLUMN is_primary_model SET NOT NULL`,
		`ALTER TABLE workforce_model_invocations DROP COLUMN IF EXISTS provider_config_digest`,
		`ALTER TABLE workforce_model_invocations DROP CONSTRAINT IF EXISTS workforce_provider_invocations_operation_check`,
		`ALTER TABLE workforce_model_invocations DROP CONSTRAINT IF EXISTS workforce_model_invocations_operation_check`,
		`ALTER TABLE workforce_model_invocations ADD CONSTRAINT workforce_model_invocations_operation_check CHECK (operation IN ('responses.create','responses.compact','messages.create','messages.count_tokens'))`,
		`ALTER TABLE workforce_model_invocations DROP CONSTRAINT IF EXISTS workforce_model_invocations_protocol_check`,
		`ALTER TABLE workforce_model_invocations ADD CONSTRAINT workforce_model_invocations_protocol_check CHECK (protocol IN ('OPENAI','ANTHROPIC'))`,
		`ALTER TABLE workforce_model_invocations ADD COLUMN IF NOT EXISTS execution_kind VARCHAR(32) NOT NULL DEFAULT 'AGENT_ATTEMPT'`,
		`ALTER TABLE workforce_model_invocations ADD COLUMN IF NOT EXISTS review_run_id BIGINT`,
		`ALTER TABLE workforce_model_invocations ADD COLUMN IF NOT EXISTS review_run_attempt_id BIGINT`,
		`ALTER TABLE workforce_model_invocations ALTER COLUMN attempt_id DROP NOT NULL`,
		`ALTER TABLE workforce_model_invocations DROP CONSTRAINT IF EXISTS workforce_provider_invocations_execution_identity_ck`,
		`ALTER TABLE workforce_model_invocations DROP CONSTRAINT IF EXISTS workforce_model_invocations_execution_identity_ck`,
		`ALTER TABLE workforce_model_invocations ADD CONSTRAINT workforce_model_invocations_execution_identity_ck CHECK ((execution_kind='AGENT_ATTEMPT' AND attempt_id IS NOT NULL AND review_run_id IS NULL AND review_run_attempt_id IS NULL AND generation>0 AND owner_epoch>0) OR (execution_kind='REVIEW_RUN' AND attempt_id IS NULL AND review_run_id IS NOT NULL AND review_run_attempt_id IS NOT NULL AND generation=0 AND owner_epoch>0))`,
		`CREATE INDEX IF NOT EXISTS workforce_model_invocations_review_run_active_idx ON workforce_model_invocations (tenant_id,review_run_id,review_run_attempt_id,status) WHERE execution_kind='REVIEW_RUN' AND status IN ('AUTHORIZED','STREAMING')`,
		`COMMENT ON TABLE workforce_model_credentials IS '模型密钥配置'`,
		`COMMENT ON COLUMN workforce_model_credentials.secret_ciphertext IS 'API Key 密文'`,
		`COMMENT ON COLUMN workforce_model_credentials.secret_edek IS '加密后的数据密钥'`,
		`COMMENT ON COLUMN workforce_model_credentials.secret_version IS '密钥版本'`,
		`COMMENT ON COLUMN workforce_model_credentials.status IS '使用状态'`,
		`COMMENT ON TABLE workforce_model_invocations IS '模型调用记录'`,
		`COMMENT ON COLUMN workforce_model_invocations.owner_epoch IS '执行权版本'`,
		`COMMENT ON COLUMN workforce_model_invocations.duration_milliseconds IS '调用耗时（毫秒）'`,
		`DO $$ BEGIN IF EXISTS(SELECT 1 FROM information_schema.columns WHERE table_schema='public' AND table_name='workforce_task_assignments' AND column_name='provider_profile_id') THEN ALTER TABLE workforce_task_assignments RENAME COLUMN provider_profile_id TO model_credential_id; END IF; IF EXISTS(SELECT 1 FROM information_schema.columns WHERE table_schema='public' AND table_name='workforce_task_assignments' AND column_name='provider_protocol') THEN ALTER TABLE workforce_task_assignments RENAME COLUMN provider_protocol TO model_protocol; END IF; IF EXISTS(SELECT 1 FROM information_schema.columns WHERE table_schema='public' AND table_name='workforce_task_assignments' AND column_name='provider_model') THEN ALTER TABLE workforce_task_assignments RENAME COLUMN provider_model TO primary_model; END IF; END $$`,
		`ALTER TABLE workforce_task_assignments ADD COLUMN IF NOT EXISTS credential_version INTEGER`,
		`ALTER TABLE workforce_task_assignments ADD COLUMN IF NOT EXISTS model_protocol VARCHAR(16)`,
		`UPDATE workforce_task_assignments a SET credential_version=c.secret_version,model_protocol=c.protocol FROM workforce_model_credentials c WHERE c.id=a.model_credential_id AND (a.credential_version IS NULL OR a.model_protocol IS NULL OR a.model_protocol='OPENAI_RESPONSES')`,
		`ALTER TABLE workforce_task_assignments DROP COLUMN IF EXISTS provider_config_digest`,
		`ALTER TABLE workforce_task_assignments DROP COLUMN IF EXISTS provider_capabilities_json`,
		`DO $$ BEGIN IF to_regclass('public.workforce_review_runs') IS NOT NULL THEN IF EXISTS(SELECT 1 FROM information_schema.columns WHERE table_schema='public' AND table_name='workforce_review_runs' AND column_name='provider_profile_id') THEN ALTER TABLE workforce_review_runs RENAME COLUMN provider_profile_id TO model_credential_id; END IF; IF EXISTS(SELECT 1 FROM information_schema.columns WHERE table_schema='public' AND table_name='workforce_review_runs' AND column_name='provider_protocol') THEN ALTER TABLE workforce_review_runs RENAME COLUMN provider_protocol TO model_protocol; END IF; IF EXISTS(SELECT 1 FROM information_schema.columns WHERE table_schema='public' AND table_name='workforce_review_runs' AND column_name='provider_model') THEN ALTER TABLE workforce_review_runs RENAME COLUMN provider_model TO primary_model; END IF; END IF; END $$`,
		`ALTER TABLE workforce_review_runs ADD COLUMN IF NOT EXISTS credential_version INTEGER`,
		`UPDATE workforce_review_runs r SET credential_version=c.secret_version,model_protocol=c.protocol FROM workforce_model_credentials c WHERE c.id=r.model_credential_id AND (r.credential_version IS NULL OR r.model_protocol='OPENAI_RESPONSES')`,
		`ALTER TABLE workforce_review_runs DROP COLUMN IF EXISTS provider_config_digest`,
		`ALTER TABLE workforce_review_runs DROP COLUMN IF EXISTS provider_capabilities_json`,
		`ALTER TABLE workforce_review_runs DROP CONSTRAINT IF EXISTS workforce_review_runs_assignment_ck`,
		`ALTER TABLE workforce_review_runs ADD CONSTRAINT workforce_review_runs_assignment_ck CHECK (
			((runtime_release_id IS NULL AND reviewer_image_digest='' AND adapter_identity='' AND adapter_version='' AND adapter_protocol='' AND model_credential_id IS NULL AND credential_version IS NULL AND model_protocol='' AND primary_model='' AND provider_proxy_image_digest='' AND selection_digest='' AND worker_id IS NULL AND worker_instance_id='')
			 OR (runtime_release_id>0 AND reviewer_image_digest ~ '^sha256:[0-9a-f]{64}$' AND adapter_identity<>'' AND adapter_version<>'' AND adapter_protocol<>'' AND model_credential_id>0 AND credential_version>0 AND model_protocol IN ('OPENAI','ANTHROPIC') AND primary_model<>'' AND provider_proxy_image_digest ~ '^sha256:[0-9a-f]{64}$' AND selection_digest ~ '^[0-9a-f]{64}$' AND worker_id>0 AND worker_instance_id<>''))
			AND (status NOT IN ('ASSIGNED','RUNNING','VALIDATING_REPORT') OR selection_digest<>'')
		)`,
		`DO $$ BEGIN IF to_regclass('public.workforce_review_run_attempts') IS NOT NULL THEN IF EXISTS(SELECT 1 FROM information_schema.columns WHERE table_schema='public' AND table_name='workforce_review_run_attempts' AND column_name='provider_profile_id') THEN ALTER TABLE workforce_review_run_attempts RENAME COLUMN provider_profile_id TO model_credential_id; END IF; IF EXISTS(SELECT 1 FROM information_schema.columns WHERE table_schema='public' AND table_name='workforce_review_run_attempts' AND column_name='provider_protocol') THEN ALTER TABLE workforce_review_run_attempts RENAME COLUMN provider_protocol TO model_protocol; END IF; IF EXISTS(SELECT 1 FROM information_schema.columns WHERE table_schema='public' AND table_name='workforce_review_run_attempts' AND column_name='provider_model') THEN ALTER TABLE workforce_review_run_attempts RENAME COLUMN provider_model TO primary_model; END IF; END IF; END $$`,
		`ALTER TABLE workforce_review_run_attempts ADD COLUMN IF NOT EXISTS credential_version INTEGER`,
		`UPDATE workforce_review_run_attempts a SET credential_version=c.secret_version,model_protocol=c.protocol FROM workforce_model_credentials c WHERE c.id=a.model_credential_id AND (a.credential_version IS NULL OR a.model_protocol='OPENAI_RESPONSES')`,
		`ALTER TABLE workforce_review_run_attempts ALTER COLUMN credential_version SET NOT NULL`,
		`ALTER TABLE workforce_review_run_attempts DROP COLUMN IF EXISTS provider_config_digest`,
		`ALTER TABLE workforce_review_run_attempts DROP COLUMN IF EXISTS provider_capabilities_json`,
		`ALTER TABLE workforce_tasks ADD COLUMN IF NOT EXISTS model_credential_id BIGINT`,
		`ALTER TABLE workforce_tasks ADD COLUMN IF NOT EXISTS primary_model TEXT`,
		`UPDATE workforce_tasks t SET model_credential_id=a.model_credential_id,primary_model=a.primary_model FROM (SELECT DISTINCT ON (task_id) task_id,model_credential_id,primary_model FROM workforce_task_assignments ORDER BY task_id,created_at) a WHERE t.id=a.task_id AND t.model_credential_id IS NULL`,
		`ALTER TABLE workforce_tasks DROP COLUMN IF EXISTS provider_alias`,
		`ALTER TABLE workforce_tasks DROP COLUMN IF EXISTS provider_model`,
	}
	for _, statement := range statements {
		if _, err := tx.Exec(ctx, statement); err != nil {
			return fmt.Errorf("align model credential hard cut: %w", err)
		}
	}
	// The legacy column names are renamed by the statements above. Run the
	// type alignment again so those newly named columns receive the final type.
	if err = alignModelCredentialStorage(ctx, tx); err != nil {
		return err
	}
	if err = alignModelInvocationIdentity(ctx, tx, idGen); err != nil {
		return err
	}
	if err = alignModelCredentialReferences(ctx, tx); err != nil {
		return err
	}
	return nil
}

type modelCredentialPermissionDefinition struct {
	legacyCode  string
	code        string
	name        string
	method      string
	path        string
	description string
}

func ensureModelCredentialAccess(ctx context.Context, tx pgx.Tx, idGen IDGenerator) error {
	if idGen == nil {
		return errors.New("model credential access alignment requires an ID generator")
	}
	var err error
	for _, relation := range []string{"sys_permission", "sys_role", "sys_role_permission", "sys_menu", "sys_menu_permission", "sys_role_menu"} {
		available, err := relationExists(ctx, tx, relation)
		if err != nil {
			return err
		}
		if !available {
			return nil
		}
	}
	definitions := []modelCredentialPermissionDefinition{
		{legacyCode: "workforce:provider:list", code: "workforce:model-credential:list", name: "查看模型密钥", method: "GET", path: "/workforce/model-credentials", description: "查看当前组织的模型密钥"},
		{legacyCode: "workforce:provider:create", code: "workforce:model-credential:create", name: "新增模型密钥", method: "POST", path: "/workforce/model-credentials", description: "加密保存模型密钥"},
		{legacyCode: "workforce:provider:update", code: "workforce:model-credential:update", name: "编辑模型密钥", method: "PUT", path: "/workforce/model-credentials/{id}", description: "编辑地址、密钥和模型目录"},
		{legacyCode: "workforce:provider:status", code: "workforce:model-credential:status", name: "启停模型密钥", method: "PUT", path: "/workforce/model-credentials/{id}/status", description: "启用或停用模型密钥"},
		{legacyCode: "workforce:provider:secret", code: "workforce:model-credential:usage", name: "查看模型调用", method: "GET", path: "/workforce/model-credentials/{id}/invocations", description: "查看安全的模型调用统计"},
		{code: "workforce:model-credential:delete", name: "删除模型密钥", method: "DELETE", path: "/workforce/model-credentials/{id}", description: "删除模型密钥"},
	}
	permissionIDs := make([]int64, 0, len(definitions))
	for _, definition := range definitions {
		var permissionID int64
		err = tx.QueryRow(ctx, `SELECT id FROM sys_permission WHERE code=$1 AND "isDeleted"=false FOR UPDATE`, definition.code).Scan(&permissionID)
		if errors.Is(err, pgx.ErrNoRows) {
			err = tx.QueryRow(ctx, `SELECT id FROM sys_permission WHERE code=$1 AND "isDeleted"=false FOR UPDATE`, definition.legacyCode).Scan(&permissionID)
			if errors.Is(err, pgx.ErrNoRows) {
				permissionID = idGen.NextID()
				_, err = tx.Exec(ctx, `INSERT INTO sys_permission
					(id,code,name,"resourceType",method,path,status,description,"createTime","updateTime","isDeleted")
					VALUES($1,$2,$3,'API',$4,$5,0,$6,CURRENT_TIMESTAMP,CURRENT_TIMESTAMP,false)`,
					permissionID, definition.code, definition.name, definition.method, definition.path, definition.description)
			} else if err == nil {
				_, err = tx.Exec(ctx, `UPDATE sys_permission SET code=$2,name=$3,"resourceType"='API',method=$4,path=$5,status=0,description=$6,"updateTime"=CURRENT_TIMESTAMP,"isDeleted"=false WHERE id=$1`,
					permissionID, definition.code, definition.name, definition.method, definition.path, definition.description)
			}
		} else if err == nil {
			_, err = tx.Exec(ctx, `UPDATE sys_permission SET name=$2,"resourceType"='API',method=$3,path=$4,status=0,description=$5,"updateTime"=CURRENT_TIMESTAMP WHERE id=$1`,
				permissionID, definition.name, definition.method, definition.path, definition.description)
		}
		if err != nil {
			return fmt.Errorf("align model credential permission %s: %w", definition.code, err)
		}
		permissionIDs = append(permissionIDs, permissionID)
	}
	if _, err = tx.Exec(ctx, `UPDATE sys_permission SET "isDeleted"=true,"updateTime"=CURRENT_TIMESTAMP WHERE code='workforce:provider:probe' AND "isDeleted"=false`); err != nil {
		return err
	}

	var menuID int64
	err = tx.QueryRow(ctx, `SELECT id FROM sys_menu WHERE path='/workforce/model-credentials' AND "isDeleted"=false FOR UPDATE`).Scan(&menuID)
	if errors.Is(err, pgx.ErrNoRows) {
		err = tx.QueryRow(ctx, `SELECT id FROM sys_menu WHERE path='/workforce/providers' AND "isDeleted"=false FOR UPDATE`).Scan(&menuID)
		if errors.Is(err, pgx.ErrNoRows) {
			return errors.New("model credential menu parentage is unavailable")
		}
	}
	if err != nil {
		return err
	}
	if _, err = tx.Exec(ctx, `UPDATE sys_menu SET name='模型中心',path='/workforce/model-credentials',component='workforce/model-credentials/index',permission='workforce:model-credential:list',remark='模型密钥、模型目录与调用用量',"updateTime"=CURRENT_TIMESTAMP,"isDeleted"=false WHERE id=$1`, menuID); err != nil {
		return err
	}
	for _, permissionID := range permissionIDs {
		var linked bool
		if err = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM sys_menu_permission WHERE "menuId"=$1 AND "permissionId"=$2)`, menuID, permissionID).Scan(&linked); err != nil {
			return err
		}
		if !linked {
			if _, err = tx.Exec(ctx, `INSERT INTO sys_menu_permission(id,"menuId","permissionId","creatorId","createTime") VALUES($1,$2,$3,NULL,CURRENT_TIMESTAMP)`, idGen.NextID(), menuID, permissionID); err != nil {
				return err
			}
		}
	}
	rows, err := tx.Query(ctx, `SELECT id FROM sys_role WHERE code IN ('SUPER_ADMIN','SYSTEM_ADMIN') AND "isDeleted"=false`)
	if err != nil {
		return err
	}
	var roleIDs []int64
	for rows.Next() {
		var roleID int64
		if err = rows.Scan(&roleID); err != nil {
			rows.Close()
			return err
		}
		roleIDs = append(roleIDs, roleID)
	}
	if err = rows.Err(); err != nil {
		rows.Close()
		return err
	}
	rows.Close()
	for _, roleID := range roleIDs {
		var linked bool
		if err = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM sys_role_menu WHERE "roleId"=$1 AND "menuId"=$2)`, roleID, menuID).Scan(&linked); err != nil {
			return err
		}
		if !linked {
			if _, err = tx.Exec(ctx, `INSERT INTO sys_role_menu(id,"roleId","menuId","createTime","updateTime") VALUES($1,$2,$3,CURRENT_TIMESTAMP,CURRENT_TIMESTAMP)`, idGen.NextID(), roleID, menuID); err != nil {
				return err
			}
		}
		for _, permissionID := range permissionIDs {
			if err = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM sys_role_permission WHERE "roleId"=$1 AND "permissionId"=$2)`, roleID, permissionID).Scan(&linked); err != nil {
				return err
			}
			if !linked {
				if _, err = tx.Exec(ctx, `INSERT INTO sys_role_permission(id,"roleId","permissionId","creatorId","createTime","updateTime") VALUES($1,$2,$3,NULL,CURRENT_TIMESTAMP,CURRENT_TIMESTAMP)`, idGen.NextID(), roleID, permissionID); err != nil {
					return err
				}
			}
		}
	}
	return nil
}

func verifyProviderInvocationSchemaShape(ctx context.Context, q interface {
	QueryRow(context.Context, string, ...any) pgx.Row
}) error {
	var exists bool
	if err := q.QueryRow(ctx, `SELECT to_regclass('public.workforce_model_invocations') IS NOT NULL`).Scan(&exists); err != nil {
		return err
	}
	if !exists {
		return nil
	}
	for _, expected := range []struct{ column, dataType, nullable string }{
		{"id", "bigint", "NO"},
		{"protocol_invocation_key", "character varying", "NO"},
		{"execution_kind", "character varying", "NO"},
		{"attempt_id", "bigint", "YES"},
		{"review_run_id", "bigint", "YES"},
		{"review_run_attempt_id", "bigint", "YES"},
		{"generation", "integer", "NO"},
		{"owner_epoch", "integer", "NO"},
		{"credential_version", "integer", "NO"},
		{"request_body_sha256", "bytea", "NO"},
		{"nonce_sha256", "bytea", "NO"},
		{"upstream_status_code", "smallint", "YES"},
		{"duration_milliseconds", "integer", "YES"},
		{"input_tokens", "integer", "YES"},
		{"cached_input_tokens", "integer", "YES"},
		{"cache_creation_input_tokens", "integer", "YES"},
		{"output_tokens", "integer", "YES"},
		{"reasoning_output_tokens", "integer", "YES"},
		{"total_tokens", "integer", "YES"},
	} {
		var dataType, nullable string
		if err := q.QueryRow(ctx, `SELECT data_type,is_nullable FROM information_schema.columns WHERE table_schema='public' AND table_name='workforce_model_invocations' AND column_name=$1`, expected.column).Scan(&dataType, &nullable); err != nil {
			return fmt.Errorf("Provider invocation column %s is not aligned: %w", expected.column, err)
		}
		if dataType != expected.dataType || nullable != expected.nullable {
			return fmt.Errorf("Provider invocation column %s has %s/%s, want %s/%s", expected.column, dataType, nullable, expected.dataType, expected.nullable)
		}
	}
	for _, name := range []string{"workforce_model_invocations_execution_identity_ck"} {
		var found bool
		if err := q.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM pg_constraint WHERE conrelid='workforce_model_invocations'::regclass AND conname=$1 AND contype='c' AND convalidated)`, name).Scan(&found); err != nil {
			return err
		}
		if !found {
			return fmt.Errorf("Provider invocation constraint is not aligned: %s", name)
		}
	}
	var identityConstraint string
	if err := q.QueryRow(ctx, `SELECT pg_get_constraintdef(oid) FROM pg_constraint WHERE conrelid='workforce_model_invocations'::regclass AND conname='workforce_model_invocations_execution_identity_ck' AND contype='c' AND convalidated`).Scan(&identityConstraint); err != nil {
		return fmt.Errorf("Provider invocation execution identity constraint is not aligned: %w", err)
	}
	identityConstraint = strings.ToLower(identityConstraint)
	for _, token := range []string{"execution_kind", "attempt_id", "review_run_id", "review_run_attempt_id", "owner_epoch", "agent_attempt", "review_run"} {
		if !strings.Contains(identityConstraint, token) {
			return fmt.Errorf("Provider invocation execution identity constraint is weakened: %s", token)
		}
	}
	var indexExists bool
	if err := q.QueryRow(ctx, `SELECT to_regclass('public.workforce_model_invocations_review_run_active_idx') IS NOT NULL`).Scan(&indexExists); err != nil {
		return err
	}
	if !indexExists {
		return errors.New("Provider invocation ReviewRun active index is not aligned")
	}
	var primaryKey string
	if err := q.QueryRow(ctx, `SELECT pg_get_constraintdef(oid) FROM pg_constraint WHERE conrelid='workforce_model_invocations'::regclass AND contype='p'`).Scan(&primaryKey); err != nil {
		return fmt.Errorf("Provider invocation primary key is not aligned: %w", err)
	}
	if !strings.Contains(strings.ToLower(primaryKey), "(id)") {
		return fmt.Errorf("Provider invocation primary key is not aligned: %s", primaryKey)
	}
	var protocolKeyUnique bool
	if err := q.QueryRow(ctx, `SELECT EXISTS(
		SELECT 1 FROM pg_constraint
		WHERE conrelid='workforce_model_invocations'::regclass
		  AND contype='u'
		  AND lower(pg_get_constraintdef(oid)) LIKE 'unique (tenant_id, protocol_invocation_key)%'
	)`).Scan(&protocolKeyUnique); err != nil {
		return err
	}
	if !protocolKeyUnique {
		return errors.New("Provider invocation protocol key uniqueness is not aligned")
	}
	return nil
}

func verifyModelCredentialSchemaShape(ctx context.Context, q interface {
	QueryRow(context.Context, string, ...any) pgx.Row
}) error {
	var exists bool
	if err := q.QueryRow(ctx, `SELECT to_regclass('public.workforce_model_credentials') IS NOT NULL`).Scan(&exists); err != nil {
		return err
	}
	if !exists {
		return nil
	}
	for _, expected := range []struct{ column, dataType, nullable string }{
		{"id", "bigint", "NO"},
		{"tenant_id", "bigint", "NO"},
		{"name", "character varying", "NO"},
		{"protocol", "character varying", "NO"},
		{"base_url", "character varying", "NO"},
		{"secret_ciphertext", "bytea", "YES"},
		{"secret_edek", "bytea", "YES"},
		{"secret_wrap_key_ref", "character varying", "YES"},
		{"secret_hint", "character varying", "NO"},
		{"secret_version", "integer", "NO"},
		{"status", "character varying", "NO"},
		{"version", "integer", "NO"},
		{"last_discovery_status", "character varying", "NO"},
		{"last_discovery_reason_code", "character varying", "NO"},
	} {
		var dataType, nullable string
		if err := q.QueryRow(ctx, `SELECT data_type,is_nullable FROM information_schema.columns WHERE table_schema='public' AND table_name='workforce_model_credentials' AND column_name=$1`, expected.column).Scan(&dataType, &nullable); err != nil {
			return fmt.Errorf("model credential column %s is not aligned: %w", expected.column, err)
		}
		if dataType != expected.dataType || nullable != expected.nullable {
			return fmt.Errorf("model credential column %s has %s/%s, want %s/%s", expected.column, dataType, nullable, expected.dataType, expected.nullable)
		}
	}
	return nil
}

func verifyTx(ctx context.Context, tx pgx.Tx, requireIndependentReview bool) error {
	if err := verifyWorkerMetricSchema(ctx, tx); err != nil {
		return err
	}
	if err := verifyTaskAnalyticsSchema(ctx, tx); err != nil {
		return err
	}
	// Identity-only legacy databases have not installed Worker Fleet yet.
	// Its authoritative migration owns creation of attempt facts; do not
	// require that later table during the earlier identity conversion.
	if requireIndependentReview {
		if err := verifyGitDeliverySchema(ctx, tx); err != nil {
			return err
		}
		if err := verifyFactProducerSchema(ctx, tx); err != nil {
			return err
		}
	}
	if _, err := tx.Exec(ctx, `
CREATE UNIQUE INDEX IF NOT EXISTS workforce_materialization_identity_output_idx ON workforce_result_materialization_runs (task_turn_id,kind,input_identity_digest,output_id) WHERE output_id IS NOT NULL;
CREATE UNIQUE INDEX IF NOT EXISTS workforce_materialization_identity_platform_idx ON workforce_result_materialization_runs (task_turn_id,kind,input_identity_digest) WHERE output_id IS NULL;`); err != nil {
		return fmt.Errorf("rebuild materialization identity indexes: %w", err)
	}
	if err := verifyTaskNotificationSchemaShape(ctx, tx); err != nil {
		return err
	}
	if err := verifyProviderInvocationSchemaShape(ctx, tx); err != nil {
		return err
	}
	if err := verifyModelCredentialSchemaShape(ctx, tx); err != nil {
		return err
	}
	if requireIndependentReview {
		if err := verifyReviewRunSchemaShape(ctx, tx); err != nil {
			return err
		}
		if err := verifyReviewRunSchemaBehaviorInTx(ctx, tx); err != nil {
			return err
		}
	}
	columns, err := textIdentityColumns(ctx, tx)
	if err != nil {
		return err
	}
	if len(columns) != 0 {
		return fmt.Errorf("alignment left %d non-allowlisted TEXT identity columns", len(columns))
	}
	textExceptionDrift, err := textIdentityExceptionDriftCount(ctx, tx)
	if err != nil {
		return err
	}
	if textExceptionDrift != 0 {
		return fmt.Errorf("alignment left %d governed TEXT identity columns with non-text types", textExceptionDrift)
	}
	var invalid int64
	if err = tx.QueryRow(ctx, `SELECT count(*) FROM workforce_tasks WHERE id<=0 OR tenant_id<=0 OR created_by<=0`).Scan(&invalid); err != nil {
		return err
	}
	if invalid != 0 {
		return fmt.Errorf("alignment produced %d invalid Task identities", invalid)
	}
	return nil
}
func columnExists(ctx context.Context, tx pgx.Tx, table, column string) (bool, error) {
	var exists bool
	err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM information_schema.columns WHERE table_schema='public' AND table_name=$1 AND column_name=$2)`, table, column).Scan(&exists)
	return exists, err
}

func relationExists(ctx context.Context, tx pgx.Tx, table string) (bool, error) {
	var exists bool
	err := tx.QueryRow(ctx, `SELECT to_regclass(format('%I.%I', 'public', $1::text)) IS NOT NULL`, table).Scan(&exists)
	return exists, err
}

func ident(value string) string { return pgx.Identifier{value}.Sanitize() }
