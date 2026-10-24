// Package governance provides the executable safety boundary for database
// governance acceptance. It is intentionally strict: DG acceptance may only
// run against the two dedicated local databases.
package governance

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
)

const (
	MySQLDatabaseName    = "seven_database_governance_mysql"
	PostgresDatabaseName = "seven_database_governance_pg"

	ForwardRecoveryResume = "resume-forward"
)

// MigrationPlan freezes the executable clean and supported-upgrade paths at
// the DG1 checkpoint.
type MigrationPlan struct {
	Dialect            string
	DatabaseName       string
	CleanBaselineDir   string
	MigrationsDir      string
	UpgradeFromVersion int64
	ForwardRecovery    string
}

// MigrationPlanFor returns the reviewed DG1 plan for a supported dialect.
func MigrationPlanFor(dialect string) (MigrationPlan, error) {
	switch normalizeDialect(dialect) {
	case "mysql":
		return MigrationPlan{
			Dialect:            "mysql",
			DatabaseName:       MySQLDatabaseName,
			MigrationsDir:      "migrations/mysql",
			UpgradeFromVersion: 20260730100000,
			ForwardRecovery:    ForwardRecoveryResume,
		}, nil
	case "postgres":
		return MigrationPlan{
			Dialect:            "postgres",
			DatabaseName:       PostgresDatabaseName,
			CleanBaselineDir:   "migrations/postgres-baseline",
			MigrationsDir:      "migrations/postgres",
			UpgradeFromVersion: 20260730100000,
			ForwardRecovery:    ForwardRecoveryResume,
		}, nil
	default:
		return MigrationPlan{}, fmt.Errorf("unsupported DG1 database dialect %q", dialect)
	}
}

// ValidateIsolatedDatabaseName rejects prefixes, suffixes, case variants, and
// every non-DG database. This check must run before migrations or CRUD.
func ValidateIsolatedDatabaseName(dialect, databaseName string) error {
	plan, err := MigrationPlanFor(dialect)
	if err != nil {
		return err
	}
	if databaseName != plan.DatabaseName {
		return fmt.Errorf(
			"refusing DG1 database operation: connected database %q, require exact %q",
			databaseName,
			plan.DatabaseName,
		)
	}
	return nil
}

// AssertConnectedDatabase asks the server for the selected database and
// validates it against the exact DG1 allowlist. DSN text alone is never trusted.
func AssertConnectedDatabase(ctx context.Context, db *sql.DB, dialect string) error {
	if db == nil {
		return fmt.Errorf("refusing DG1 database operation: nil database")
	}
	var databaseName string
	var query string
	switch normalizeDialect(dialect) {
	case "mysql":
		query = "SELECT DATABASE()"
	case "postgres":
		query = "SELECT current_database()"
	default:
		return fmt.Errorf("unsupported DG1 database dialect %q", dialect)
	}
	if err := db.QueryRowContext(ctx, query).Scan(&databaseName); err != nil {
		return fmt.Errorf("read connected %s database name: %w", dialect, err)
	}
	return ValidateIsolatedDatabaseName(dialect, databaseName)
}

func normalizeDialect(dialect string) string {
	switch strings.ToLower(strings.TrimSpace(dialect)) {
	case "mysql":
		return "mysql"
	case "postgres", "postgresql", "pgx":
		return "postgres"
	default:
		return ""
	}
}
