package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"path/filepath"
	"strings"

	"github.com/CaixyPromise/seven-agent/force/internal/infrastructure/datasource"
	dsbootstrap "github.com/CaixyPromise/seven-agent/force/internal/infrastructure/datasource/bootstrap"
	"github.com/CaixyPromise/seven-agent/force/internal/shared/config"
	"github.com/CaixyPromise/seven-agent/force/internal/shared/logger"
	"github.com/pressly/goose/v3"
)

func runMigrate(
	args []string,
	stdout io.Writer,
	stderr io.Writer,
	executable executableFunc,
	lookupEnv lookupEnvFunc,
) int {
	flags := flag.NewFlagSet("migrate", flag.ContinueOnError)
	flags.SetOutput(stderr)
	home := flags.String("home", "", "release package root")
	configDir := flags.String("config-dir", "", "configuration directory")
	migrationsRoot := flags.String("migrations-dir", "", "root directory containing mysql and postgres migrations")
	legacyMigrationsDir := flags.String("dir", "", "migration dialect directory (deprecated; use --migrations-dir)")
	tableName := flags.String("table", goose.DefaultTablename, "goose version table name")
	baselineVersion := flags.String("baseline-version", "", "override datasource.bootstrap.baselineVersion for inspect/bootstrap")
	allowLegacySync := flags.Bool("allow-legacy-sync", false, "temporarily allow legacy unmanaged schema sync during bootstrap")
	verbose := flags.Bool("verbose", true, "enable verbose goose output")
	if err := flags.Parse(args); err != nil {
		return 2
	}
	if flags.NArg() == 0 {
		fmt.Fprintln(stderr, "usage: seven-agent-force-server migrate [flags] <up|down|status|version|inspect|bootstrap|create|fix> [args]")
		return 2
	}

	command := flags.Arg(0)
	commandArgs := flags.Args()[1:]
	allowedCommands := map[string]struct{}{
		"up": {}, "down": {}, "status": {}, "version": {},
		"inspect": {}, "bootstrap": {}, "create": {}, "fix": {},
	}
	if _, ok := allowedCommands[command]; !ok {
		fmt.Fprintf(stderr, "unsupported migration command %q\n", command)
		return 2
	}

	paths, err := resolveRuntimePaths(*home, *configDir, *migrationsRoot, executable, lookupEnv)
	if err != nil {
		fmt.Fprintf(stderr, "resolve runtime resources: %v\n", err)
		return 1
	}
	cfg, err := config.Load(paths.configDir)
	if err != nil {
		fmt.Fprintf(stderr, "load config: %v\n", err)
		return 1
	}
	dialectDir := filepath.Join(paths.migrationsRoot, cfg.Datasource.Driver)
	if strings.TrimSpace(*legacyMigrationsDir) != "" {
		dialectDir, err = absoluteDirectory(*legacyMigrationsDir, "migration dialect")
		if err != nil {
			fmt.Fprintf(stderr, "resolve migrations directory: %v\n", err)
			return 1
		}
	}
	bootstrapCfg := cfg.Datasource.Bootstrap
	bootstrapCfg.MigrationsDir = dialectDir
	if flagProvided(flags, "table") {
		bootstrapCfg.VersionTable = *tableName
	}
	if flagProvided(flags, "baseline-version") {
		bootstrapCfg.BaselineVersion = *baselineVersion
	}
	if flagProvided(flags, "allow-legacy-sync") {
		bootstrapCfg.AllowLegacySync = *allowLegacySync
	}
	activeTableName := bootstrapCfg.VersionTable
	if activeTableName == "" {
		activeTableName = *tableName
	}

	switch command {
	case "create":
		if len(commandArgs) < 1 {
			fmt.Fprintln(stderr, "usage: seven-agent-force-server migrate create <name> [sql|go]")
			return 2
		}
		migrationType := "sql"
		if len(commandArgs) > 1 {
			migrationType = commandArgs[1]
		}
		if err := goose.Create(nil, dialectDir, commandArgs[0], migrationType); err != nil {
			fmt.Fprintf(stderr, "create migration: %v\n", err)
			return 1
		}
		return 0
	case "fix":
		if err := goose.Fix(dialectDir); err != nil {
			fmt.Fprintf(stderr, "fix migrations: %v\n", err)
			return 1
		}
		return 0
	case "inspect", "bootstrap":
		if !cfg.Datasource.Configured() {
			fmt.Fprintf(stderr, "datasource.%s is not configured; set datasource.driver and matching datasource section\n", cfg.Datasource.Driver)
			return 1
		}
	}
	if command == "bootstrap" && !bootstrapCfg.ManualEnabled() {
		fmt.Fprintln(stderr, "datasource bootstrap manual mode is disabled; set datasource.bootstrap.mode=manual|both and enabled=true")
		return 1
	}
	if !cfg.Datasource.Configured() {
		fmt.Fprintf(stderr, "datasource.%s is not configured; set datasource.driver and matching datasource section\n", cfg.Datasource.Driver)
		return 1
	}

	log, err := logger.New(cfg.Logging, cfg.Profile)
	if err != nil {
		fmt.Fprintf(stderr, "build logger: %v\n", err)
		return 1
	}
	defer logger.Sync(log)
	provider, err := datasource.NewProvider(cfg.Datasource, log)
	if err != nil {
		fmt.Fprintf(stderr, "open datasource provider: %v\n", err)
		return 1
	}
	defer provider.Close()

	bootstrapper := dsbootstrap.NewService(log)
	switch command {
	case "inspect":
		inspection, err := bootstrapper.Inspect(context.Background(), provider, bootstrapCfg)
		if err != nil {
			fmt.Fprintf(stderr, "inspect datasource bootstrap state: %v\n", err)
			return 1
		}
		fmt.Fprintf(stdout, "driver=%s state=%s version_table=%s version_table_exists=%t business_table_count=%d current_version=%d recommended_action=%s\n",
			inspection.Driver,
			inspection.State,
			inspection.VersionTable,
			inspection.VersionTableExists,
			inspection.BusinessTableCount,
			inspection.CurrentVersion,
			inspection.RecommendedAction,
		)
		return 0
	case "bootstrap":
		result, err := bootstrapper.Bootstrap(context.Background(), provider, bootstrapCfg)
		if err != nil {
			fmt.Fprintf(stderr, "bootstrap datasource schema: %v\n", err)
			return 1
		}
		fmt.Fprintf(stdout, "state=%s baseline_applied=%t sync_applied=%t update_applied=%t final_version=%d\n",
			result.Inspection.State,
			result.BaselineApplied,
			result.SyncApplied,
			result.UpdateApplied,
			result.FinalVersion,
		)
		return 0
	}

	goose.SetDialect(provider.Dialect())
	goose.SetTableName(activeTableName)
	goose.SetVerbose(*verbose)
	if err := goose.RunContext(context.Background(), command, provider.DB(), dialectDir, commandArgs...); err != nil {
		fmt.Fprintf(stderr, "goose %s failed: %v\n", command, err)
		return 1
	}
	return 0
}

func flagProvided(flags *flag.FlagSet, name string) bool {
	provided := false
	flags.Visit(func(current *flag.Flag) {
		if current.Name == name {
			provided = true
		}
	})
	return provided
}
