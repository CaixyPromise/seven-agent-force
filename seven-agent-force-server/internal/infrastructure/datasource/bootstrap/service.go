package bootstrap

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/CaixyPromise/seven-agent/force/internal/infrastructure/datasource/store"
	"github.com/CaixyPromise/seven-agent/force/internal/shared/config"
	"github.com/pressly/goose/v3"
	"go.uber.org/zap"
)

type mysqlInspector interface {
	Inspect(ctx context.Context, db *sql.DB, versionTable string) (Inspection, error)
}

type Service struct {
	log       *zap.Logger
	runner    Runner
	inspector mysqlInspector
}

func NewService(log *zap.Logger) *Service {
	return &Service{
		log:       log,
		runner:    NewGooseRunner(),
		inspector: newMySQLInspector(),
	}
}

func NewServiceWithRunner(log *zap.Logger, runner Runner) *Service {
	service := NewService(log)
	service.runner = runner
	return service
}

func (s *Service) Inspect(ctx context.Context, provider store.Provider, cfg config.DatasourceBootstrapConfig) (Inspection, error) {
	db, err := ensureProvider(provider)
	if err != nil {
		return Inspection{}, err
	}
	if provider.Driver() != "mysql" {
		return Inspection{}, fmt.Errorf("datasource bootstrap inspect does not support driver: %s", provider.Driver())
	}
	cfg, err = normalizedBootstrapConfig(provider.Driver(), cfg)
	if err != nil {
		return Inspection{}, err
	}
	inspection, err := s.inspector.Inspect(ctx, db, cfg.VersionTable)
	if err != nil {
		s.logError("datasource_bootstrap_failed", err, provider.Driver(), cfg, Inspection{})
		return Inspection{}, err
	}
	s.logInfo("datasource_bootstrap_inspected", provider.Driver(), cfg, inspection,
		zap.String("recommendedAction", inspection.RecommendedAction),
		zap.Int64("currentVersion", inspection.CurrentVersion),
	)
	return inspection, nil
}

func (s *Service) Bootstrap(ctx context.Context, provider store.Provider, cfg config.DatasourceBootstrapConfig) (Result, error) {
	db, err := ensureProvider(provider)
	if err != nil {
		return Result{}, err
	}
	if provider.Driver() != "mysql" {
		return Result{}, fmt.Errorf("datasource bootstrap does not support driver: %s", provider.Driver())
	}

	cfg, err = normalizedBootstrapConfig(provider.Driver(), cfg)
	if err != nil {
		return Result{}, err
	}
	s.logInfo("datasource_bootstrap_started", provider.Driver(), cfg, Inspection{})

	inspection, err := s.inspector.Inspect(ctx, db, cfg.VersionTable)
	if err != nil {
		s.logError("datasource_bootstrap_failed", err, provider.Driver(), cfg, Inspection{})
		return Result{}, err
	}
	s.logInfo("datasource_bootstrap_inspected", provider.Driver(), cfg, inspection,
		zap.String("recommendedAction", inspection.RecommendedAction),
		zap.Int64("currentVersion", inspection.CurrentVersion),
	)

	result := Result{Inspection: inspection}
	switch inspection.State {
	case SchemaStateEmpty:
		baselineVersion, err := parseBaselineVersion(cfg)
		if err != nil {
			s.logError("datasource_bootstrap_failed", err, provider.Driver(), cfg, inspection)
			return Result{}, err
		}
		if _, err := s.runner.UpTo(ctx, db, provider.Dialect(), cfg.MigrationsDir, cfg.VersionTable, baselineVersion); err != nil {
			s.logError("datasource_bootstrap_failed", fmt.Errorf("run baseline up-to %d: %w", baselineVersion, err), provider.Driver(), cfg, inspection)
			return Result{}, fmt.Errorf("run baseline up-to %d: %w", baselineVersion, err)
		}
		result.BaselineApplied = true
		result.UpdateApplied = true
		finalVersion, err := s.runner.Up(ctx, db, provider.Dialect(), cfg.MigrationsDir, cfg.VersionTable)
		if err != nil {
			s.logError("datasource_bootstrap_failed", fmt.Errorf("run update after baseline: %w", err), provider.Driver(), cfg, inspection)
			return Result{}, fmt.Errorf("run update after baseline: %w", err)
		}
		result.FinalVersion = finalVersion
	case SchemaStateManaged:
		result.UpdateApplied = true
		finalVersion, err := s.runner.Up(ctx, db, provider.Dialect(), cfg.MigrationsDir, cfg.VersionTable)
		if err != nil {
			s.logError("datasource_bootstrap_failed", fmt.Errorf("run managed update: %w", err), provider.Driver(), cfg, inspection)
			return Result{}, fmt.Errorf("run managed update: %w", err)
		}
		result.FinalVersion = finalVersion
	case SchemaStateLegacyUnmanaged:
		if !cfg.AllowLegacySync {
			err := fmt.Errorf("detected non-empty unmanaged legacy schema; enable datasource.bootstrap.allowLegacySync=true or run cmd/migrate bootstrap with -allow-legacy-sync")
			s.logError("datasource_bootstrap_failed", err, provider.Driver(), cfg, inspection)
			return Result{}, err
		}
		baselineVersion, err := parseBaselineVersion(cfg)
		if err != nil {
			s.logError("datasource_bootstrap_failed", err, provider.Driver(), cfg, inspection)
			return Result{}, err
		}
		if err := s.syncLegacy(ctx, db, provider.Dialect(), cfg, baselineVersion, inspection); err != nil {
			s.logError("datasource_bootstrap_failed", err, provider.Driver(), cfg, inspection)
			return Result{}, err
		}
		result.SyncApplied = true
		result.UpdateApplied = true
		s.logInfo("datasource_bootstrap_synced", provider.Driver(), cfg, inspection,
			zap.Int64("baselineVersion", baselineVersion),
		)
		finalVersion, err := s.runner.Up(ctx, db, provider.Dialect(), cfg.MigrationsDir, cfg.VersionTable)
		if err != nil {
			s.logError("datasource_bootstrap_failed", fmt.Errorf("run update after legacy sync: %w", err), provider.Driver(), cfg, inspection)
			return Result{}, fmt.Errorf("run update after legacy sync: %w", err)
		}
		result.FinalVersion = finalVersion
	default:
		err := fmt.Errorf("unsupported schema state: %s", inspection.State)
		s.logError("datasource_bootstrap_failed", err, provider.Driver(), cfg, inspection)
		return Result{}, err
	}

	if result.FinalVersion == 0 {
		finalVersion, err := s.runner.Version(ctx, db, provider.Dialect(), cfg.VersionTable)
		if err != nil {
			s.logError("datasource_bootstrap_failed", fmt.Errorf("read final version: %w", err), provider.Driver(), cfg, inspection)
			return Result{}, fmt.Errorf("read final version: %w", err)
		}
		result.FinalVersion = finalVersion
	}

	s.logInfo("datasource_bootstrap_completed", provider.Driver(), cfg, inspection,
		zap.Bool("baselineApplied", result.BaselineApplied),
		zap.Bool("syncApplied", result.SyncApplied),
		zap.Bool("updateApplied", result.UpdateApplied),
		zap.Int64("finalVersion", result.FinalVersion),
	)
	return result, nil
}

func ensureProvider(provider store.Provider) (*sql.DB, error) {
	if provider == nil || !provider.Configured() || provider.DB() == nil {
		return nil, errors.New("datasource provider is not configured")
	}
	return provider.DB(), nil
}

func normalizedBootstrapConfig(driver string, cfg config.DatasourceBootstrapConfig) (config.DatasourceBootstrapConfig, error) {
	if strings.TrimSpace(cfg.MigrationsDir) == "" {
		cfg.MigrationsDir = filepath.Join("migrations", driver)
	}
	if strings.TrimSpace(cfg.VersionTable) == "" {
		cfg.VersionTable = goose.DefaultTablename
	}
	cfg.VersionTable = strings.TrimSpace(cfg.VersionTable)
	if cfg.VersionTable != goose.DefaultTablename {
		return config.DatasourceBootstrapConfig{}, fmt.Errorf(
			"datasource.bootstrap.versionTable must be exactly %q",
			goose.DefaultTablename,
		)
	}
	if strings.TrimSpace(cfg.ChangeOwner) == "" {
		cfg.ChangeOwner = "goose"
	}
	return cfg, nil
}

func parseBaselineVersion(cfg config.DatasourceBootstrapConfig) (int64, error) {
	if strings.TrimSpace(cfg.BaselineVersion) == "" {
		return 0, errors.New("datasource.bootstrap.baselineVersion must be configured")
	}
	version, err := strconv.ParseInt(cfg.BaselineVersion, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("parse baseline version %q: %w", cfg.BaselineVersion, err)
	}
	return version, nil
}

func (s *Service) syncLegacy(
	ctx context.Context,
	db *sql.DB,
	dialect string,
	cfg config.DatasourceBootstrapConfig,
	baselineVersion int64,
	inspection Inspection,
) error {
	if inspection.VersionTableExists {
		return fmt.Errorf("cannot sync legacy schema because version table %s already exists", cfg.VersionTable)
	}
	if dialect != "mysql" {
		return fmt.Errorf("legacy sync does not support dialect: %s", dialect)
	}

	migrations, err := goose.CollectMigrations(cfg.MigrationsDir, 0, goose.MaxVersion)
	if err != nil {
		return fmt.Errorf("collect migrations: %w", err)
	}
	targets := make([]int64, 0, len(migrations))
	foundBaseline := false
	for _, migration := range migrations {
		if !strings.HasSuffix(strings.ToLower(migration.Source), ".sql") {
			return fmt.Errorf("legacy sync only supports SQL migrations: %s", migration.Source)
		}
		if migration.Version <= baselineVersion {
			targets = append(targets, migration.Version)
		}
		if migration.Version == baselineVersion {
			foundBaseline = true
		}
	}
	if !foundBaseline {
		return fmt.Errorf("baseline version %d not found in %s", baselineVersion, cfg.MigrationsDir)
	}

	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin legacy sync transaction: %w", err)
	}
	defer func() {
		_ = tx.Rollback()
	}()

	if _, err := tx.ExecContext(ctx, createVersionTableSQL(cfg.VersionTable)); err != nil {
		return fmt.Errorf("create version table %s: %w", cfg.VersionTable, err)
	}
	if _, err := tx.ExecContext(ctx, insertVersionSQL(cfg.VersionTable), int64(0), true); err != nil {
		return fmt.Errorf("insert initial version: %w", err)
	}
	for _, version := range targets {
		if _, err := tx.ExecContext(ctx, insertVersionSQL(cfg.VersionTable), version, true); err != nil {
			return fmt.Errorf("insert synced version %d: %w", version, err)
		}
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit legacy sync transaction: %w", err)
	}
	return nil
}

func createVersionTableSQL(table string) string {
	return fmt.Sprintf(`CREATE TABLE %s (
		id bigint(20) unsigned NOT NULL AUTO_INCREMENT,
		version_id bigint NOT NULL,
		is_applied boolean NOT NULL,
		tstamp timestamp NULL default now(),
		PRIMARY KEY(id)
	)`, table)
}

func insertVersionSQL(table string) string {
	return fmt.Sprintf("INSERT INTO %s (version_id, is_applied) VALUES (?, ?)", table)
}

func (s *Service) logInfo(event string, driver string, cfg config.DatasourceBootstrapConfig, inspection Inspection, fields ...zap.Field) {
	if s == nil || s.log == nil {
		return
	}
	base := []zap.Field{
		zap.String("event", event),
		zap.String("driver", driver),
		zap.String("migrationsDir", cfg.MigrationsDir),
		zap.String("versionTable", cfg.VersionTable),
	}
	if inspection.State != "" {
		base = append(base,
			zap.String("schemaState", string(inspection.State)),
			zap.Bool("versionTableExists", inspection.VersionTableExists),
			zap.Int("businessTableCount", inspection.BusinessTableCount),
		)
	}
	base = append(base, fields...)
	s.log.Info(event, base...)
}

func (s *Service) logError(event string, err error, driver string, cfg config.DatasourceBootstrapConfig, inspection Inspection) {
	if s == nil || s.log == nil {
		return
	}
	fields := []zap.Field{
		zap.String("event", event),
		zap.String("driver", driver),
		zap.String("migrationsDir", cfg.MigrationsDir),
		zap.String("versionTable", cfg.VersionTable),
		zap.Error(err),
	}
	if inspection.State != "" {
		fields = append(fields,
			zap.String("schemaState", string(inspection.State)),
			zap.Bool("versionTableExists", inspection.VersionTableExists),
			zap.Int("businessTableCount", inspection.BusinessTableCount),
		)
	}
	s.log.Error(event, fields...)
}
