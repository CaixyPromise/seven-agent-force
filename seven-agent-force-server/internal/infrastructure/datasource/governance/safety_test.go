package governance

import (
	"testing"
)

func TestValidateIsolatedDatabaseNameRequiresExactDG1Name(t *testing.T) {
	tests := []struct {
		dialect string
		name    string
		wantErr bool
	}{
		{dialect: "mysql", name: "seven_database_governance_mysql"},
		{dialect: "postgres", name: "seven_database_governance_pg"},
		{dialect: "postgresql", name: "seven_database_governance_pg"},
		{dialect: "mysql", name: "lovely_seven", wantErr: true},
		{dialect: "postgres", name: "lovely_seven", wantErr: true},
		{dialect: "mysql", name: "seven_database_governance_mysql_backup", wantErr: true},
		{dialect: "postgres", name: "seven_database_governance_pg_tmp", wantErr: true},
		{dialect: "sqlite", name: "seven_database_governance_mysql", wantErr: true},
	}
	for _, test := range tests {
		t.Run(test.dialect+"/"+test.name, func(t *testing.T) {
			err := ValidateIsolatedDatabaseName(test.dialect, test.name)
			if test.wantErr && err == nil {
				t.Fatal("expected database safety guard to reject name")
			}
			if !test.wantErr && err != nil {
				t.Fatalf("expected database safety guard to accept name: %v", err)
			}
		})
	}
}

func TestMigrationPlansFreezeCleanAndUpgradePaths(t *testing.T) {
	mysql, err := MigrationPlanFor("mysql")
	if err != nil {
		t.Fatalf("MySQL plan: %v", err)
	}
	if mysql.CleanBaselineDir != "" || mysql.MigrationsDir != "migrations/mysql" {
		t.Fatalf("unexpected MySQL migration plan: %+v", mysql)
	}
	postgres, err := MigrationPlanFor("postgres")
	if err != nil {
		t.Fatalf("PostgreSQL plan: %v", err)
	}
	if postgres.CleanBaselineDir != "migrations/postgres-baseline" || postgres.MigrationsDir != "migrations/postgres" {
		t.Fatalf("unexpected PostgreSQL migration plan: %+v", postgres)
	}
	for dialect, plan := range map[string]MigrationPlan{"mysql": mysql, "postgres": postgres} {
		if plan.UpgradeFromVersion != 20260730100000 {
			t.Fatalf("%s upgrade version=%d", dialect, plan.UpgradeFromVersion)
		}
		if plan.ForwardRecovery != ForwardRecoveryResume {
			t.Fatalf("%s recovery=%q", dialect, plan.ForwardRecovery)
		}
	}
}
