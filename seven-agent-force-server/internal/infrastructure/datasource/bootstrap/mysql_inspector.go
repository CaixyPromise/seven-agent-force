package bootstrap

import (
	"context"
	"database/sql"
	"fmt"
)

type mysqlInspectorImpl struct{}

func newMySQLInspector() *mysqlInspectorImpl {
	return &mysqlInspectorImpl{}
}

func (i *mysqlInspectorImpl) Inspect(ctx context.Context, db *sql.DB, versionTable string) (Inspection, error) {
	if db == nil {
		return Inspection{}, fmt.Errorf("mysql datasource is not configured")
	}

	versionExists, err := mysqlTableExists(ctx, db, versionTable)
	if err != nil {
		return Inspection{}, fmt.Errorf("check version table exists: %w", err)
	}
	businessTableCount, err := mysqlBusinessTableCount(ctx, db, versionTable)
	if err != nil {
		return Inspection{}, fmt.Errorf("count business tables: %w", err)
	}

	inspection := Inspection{
		Driver:             "mysql",
		VersionTable:       versionTable,
		VersionTableExists: versionExists,
		BusinessTableCount: businessTableCount,
	}

	switch {
	case versionExists:
		inspection.State = SchemaStateManaged
		inspection.RecommendedAction = "run update"
		inspection.CurrentVersion, err = mysqlCurrentVersion(ctx, db, versionTable)
		if err != nil {
			return Inspection{}, fmt.Errorf("load current version: %w", err)
		}
	case businessTableCount == 0:
		inspection.State = SchemaStateEmpty
		inspection.RecommendedAction = "run baseline then update"
	default:
		inspection.State = SchemaStateLegacyUnmanaged
		inspection.RecommendedAction = "enable allowLegacySync and run bootstrap"
	}

	return inspection, nil
}

func mysqlTableExists(ctx context.Context, db *sql.DB, table string) (bool, error) {
	var exists bool
	query := `
SELECT EXISTS (
  SELECT 1
  FROM information_schema.tables
  WHERE (DATABASE() IS NULL OR table_schema = DATABASE())
    AND table_name = ?
)`
	if err := db.QueryRowContext(ctx, query, table).Scan(&exists); err != nil {
		return false, err
	}
	return exists, nil
}

func mysqlBusinessTableCount(ctx context.Context, db *sql.DB, versionTable string) (int, error) {
	var count int
	query := `
SELECT COUNT(*)
FROM information_schema.tables
WHERE (DATABASE() IS NULL OR table_schema = DATABASE())
  AND table_type = 'BASE TABLE'
  AND table_name <> ?`
	if err := db.QueryRowContext(ctx, query, versionTable).Scan(&count); err != nil {
		return 0, err
	}
	return count, nil
}

func mysqlCurrentVersion(ctx context.Context, db *sql.DB, versionTable string) (int64, error) {
	query := fmt.Sprintf("SELECT COALESCE(MAX(version_id), 0) FROM %s", versionTable)
	var version int64
	if err := db.QueryRowContext(ctx, query).Scan(&version); err != nil {
		return 0, err
	}
	return version, nil
}
