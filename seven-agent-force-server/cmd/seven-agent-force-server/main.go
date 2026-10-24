package main

import (
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/CaixyPromise/seven-agent/force/internal/bootstrap"
	"github.com/CaixyPromise/seven-agent/force/internal/shared/buildinfo"
)

const homeEnvironment = "SEVEN_FRAMEWORK_HOME"

type runtimePaths struct {
	home           string
	configDir      string
	migrationsRoot string
}

type executableFunc func() (string, error)
type lookupEnvFunc func(string) (string, bool)

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr, os.Executable, os.LookupEnv))
}

func run(args []string, stdout io.Writer, stderr io.Writer, executable executableFunc, lookupEnv lookupEnvFunc) int {
	command := "serve"
	commandArgs := args
	if len(args) > 0 && !strings.HasPrefix(args[0], "-") {
		command = args[0]
		commandArgs = args[1:]
	}

	switch command {
	case "serve":
		return runServe(commandArgs, stderr, executable, lookupEnv)
	case "migrate":
		return runMigrate(commandArgs, stdout, stderr, executable, lookupEnv)
	case "version":
		fmt.Fprintln(stdout, buildinfo.String())
		return 0
	case "help", "-h", "--help":
		printUsage(stdout)
		return 0
	default:
		fmt.Fprintf(stderr, "unknown command %q\n\n", command)
		printUsage(stderr)
		return 2
	}
}

func runServe(args []string, stderr io.Writer, executable executableFunc, lookupEnv lookupEnvFunc) int {
	flags := flag.NewFlagSet("serve", flag.ContinueOnError)
	flags.SetOutput(stderr)
	home := flags.String("home", "", "release package root")
	configDir := flags.String("config-dir", "", "configuration directory")
	migrationsDir := flags.String("migrations-dir", "", "root directory containing mysql and postgres migrations")
	if err := flags.Parse(args); err != nil {
		return 2
	}
	if flags.NArg() != 0 {
		fmt.Fprintln(stderr, "serve does not accept positional arguments")
		return 2
	}

	paths, err := resolveRuntimePaths(*home, *configDir, *migrationsDir, executable, lookupEnv)
	if err != nil {
		fmt.Fprintf(stderr, "resolve runtime resources: %v\n", err)
		return 1
	}
	app, err := bootstrap.NewWithOptions(bootstrap.Options{
		ConfigDir:      paths.configDir,
		MigrationsRoot: paths.migrationsRoot,
	})
	if err != nil {
		fmt.Fprintf(stderr, "bootstrap application: %v\n", err)
		return 1
	}
	if err := app.Run(); err != nil {
		fmt.Fprintf(stderr, "run application: %v\n", err)
		return 1
	}
	return 0
}

func resolveRuntimePaths(
	homeFlag string,
	configFlag string,
	migrationsFlag string,
	executable executableFunc,
	lookupEnv lookupEnvFunc,
) (runtimePaths, error) {
	home := strings.TrimSpace(homeFlag)
	if home == "" && lookupEnv != nil {
		home, _ = lookupEnv(homeEnvironment)
		home = strings.TrimSpace(home)
	}
	if home == "" {
		executablePath, err := executable()
		if err != nil {
			return runtimePaths{}, fmt.Errorf("locate executable: %w", err)
		}
		executablePath, err = filepath.Abs(executablePath)
		if err != nil {
			return runtimePaths{}, fmt.Errorf("resolve executable path: %w", err)
		}
		executableDir := filepath.Dir(executablePath)
		if filepath.Base(executableDir) == "bin" {
			home = filepath.Dir(executableDir)
		} else {
			home = executableDir
		}
	}

	resolvedHome, err := absoluteDirectory(home, "home")
	if err != nil {
		return runtimePaths{}, err
	}
	configDir := strings.TrimSpace(configFlag)
	if configDir == "" {
		configDir = filepath.Join(resolvedHome, "configs")
	}
	resolvedConfigDir, err := absoluteDirectory(configDir, "configuration")
	if err != nil {
		return runtimePaths{}, err
	}
	if _, err := os.Stat(filepath.Join(resolvedConfigDir, "application.yaml")); err != nil {
		return runtimePaths{}, fmt.Errorf("configuration file application.yaml is unavailable in %s: %w", resolvedConfigDir, err)
	}

	migrationsRoot := strings.TrimSpace(migrationsFlag)
	if migrationsRoot == "" {
		migrationsRoot = filepath.Join(resolvedHome, "migrations")
	}
	resolvedMigrationsRoot, err := absoluteDirectory(migrationsRoot, "migrations")
	if err != nil {
		return runtimePaths{}, err
	}
	for _, dialect := range []string{"mysql", "postgres"} {
		if _, err := os.Stat(filepath.Join(resolvedMigrationsRoot, dialect)); err != nil {
			return runtimePaths{}, fmt.Errorf("%s migrations are unavailable in %s: %w", dialect, resolvedMigrationsRoot, err)
		}
	}
	return runtimePaths{
		home:           resolvedHome,
		configDir:      resolvedConfigDir,
		migrationsRoot: resolvedMigrationsRoot,
	}, nil
}

func absoluteDirectory(path string, purpose string) (string, error) {
	path = strings.TrimSpace(path)
	if path == "" {
		return "", fmt.Errorf("%s directory is required", purpose)
	}
	absolutePath, err := filepath.Abs(path)
	if err != nil {
		return "", fmt.Errorf("resolve %s directory: %w", purpose, err)
	}
	info, err := os.Stat(absolutePath)
	if err != nil {
		return "", fmt.Errorf("%s directory %s is unavailable: %w", purpose, absolutePath, err)
	}
	if !info.IsDir() {
		return "", fmt.Errorf("%s path %s is not a directory", purpose, absolutePath)
	}
	return absolutePath, nil
}

func printUsage(output io.Writer) {
	fmt.Fprintln(output, "seven-agent-force-server [serve|migrate|version] [flags]")
	fmt.Fprintln(output, "")
	fmt.Fprintln(output, "Commands:")
	fmt.Fprintln(output, "  serve      start the API service (default)")
	fmt.Fprintln(output, "  migrate    manage MySQL or PostgreSQL migrations")
	fmt.Fprintln(output, "  version    print version, commit, and build date")
}
