package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestResolveRuntimePathsFromReleaseBinary(t *testing.T) {
	home := prepareHome(t)
	paths, err := resolveRuntimePaths("", "", "", func() (string, error) {
		return filepath.Join(home, "bin", "seven-agent-force-server"), nil
	}, func(string) (string, bool) { return "", false })
	if err != nil {
		t.Fatalf("resolve release paths: %v", err)
	}
	if paths.home != home {
		t.Fatalf("home = %q, want %q", paths.home, home)
	}
	if paths.configDir != filepath.Join(home, "configs") {
		t.Fatalf("config dir = %q", paths.configDir)
	}
	if paths.migrationsRoot != filepath.Join(home, "migrations") {
		t.Fatalf("migrations root = %q", paths.migrationsRoot)
	}
}

func TestResolveRuntimePathsPrecedence(t *testing.T) {
	explicitHome := prepareHome(t)
	environmentHome := prepareHome(t)
	explicitConfig := filepath.Join(t.TempDir(), "config")
	explicitMigrations := filepath.Join(t.TempDir(), "migrations")
	mustMkdir(t, explicitConfig)
	mustWrite(t, filepath.Join(explicitConfig, "application.yaml"), "profile: release\n")
	mustMkdir(t, filepath.Join(explicitMigrations, "mysql"))
	mustMkdir(t, filepath.Join(explicitMigrations, "postgres"))

	paths, err := resolveRuntimePaths(explicitHome, explicitConfig, explicitMigrations, func() (string, error) {
		return filepath.Join(environmentHome, "bin", "seven-agent-force-server"), nil
	}, func(key string) (string, bool) {
		if key == homeEnvironment {
			return environmentHome, true
		}
		return "", false
	})
	if err != nil {
		t.Fatalf("resolve explicit paths: %v", err)
	}
	if paths.home != explicitHome || paths.configDir != explicitConfig || paths.migrationsRoot != explicitMigrations {
		t.Fatalf("explicit paths not honored: %+v", paths)
	}
}

func TestResolveRuntimePathsRejectsIncompletePackage(t *testing.T) {
	home := t.TempDir()
	mustMkdir(t, filepath.Join(home, "configs"))
	mustWrite(t, filepath.Join(home, "configs", "application.yaml"), "profile: release\n")
	mustMkdir(t, filepath.Join(home, "migrations", "mysql"))

	_, err := resolveRuntimePaths(home, "", "", func() (string, error) { return "", nil }, nil)
	if err == nil || !strings.Contains(err.Error(), "postgres migrations") {
		t.Fatalf("expected missing postgres migrations error, got %v", err)
	}
}

func TestVersionDoesNotExposeRuntimePaths(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	code := run([]string{"version"}, &stdout, &stderr, func() (string, error) {
		return "/private/operator/path/bin/seven-agent-force-server", nil
	}, func(string) (string, bool) {
		return "/private/operator/path", true
	})
	if code != 0 {
		t.Fatalf("version exit code = %d, stderr=%s", code, stderr.String())
	}
	if strings.Contains(stdout.String(), "/private/") {
		t.Fatalf("version leaked runtime path: %s", stdout.String())
	}
	for _, expected := range []string{"seven-agent-force-server", "version=", "commit=", "buildDate="} {
		if !strings.Contains(stdout.String(), expected) {
			t.Fatalf("version output missing %q: %s", expected, stdout.String())
		}
	}
}

func prepareHome(t *testing.T) string {
	t.Helper()
	home := t.TempDir()
	mustMkdir(t, filepath.Join(home, "bin"))
	mustMkdir(t, filepath.Join(home, "configs"))
	mustWrite(t, filepath.Join(home, "configs", "application.yaml"), "profile: release\n")
	mustMkdir(t, filepath.Join(home, "migrations", "mysql"))
	mustMkdir(t, filepath.Join(home, "migrations", "postgres"))
	return home
}

func mustMkdir(t *testing.T, path string) {
	t.Helper()
	if err := os.MkdirAll(path, 0o755); err != nil {
		t.Fatalf("create directory %s: %v", path, err)
	}
}

func mustWrite(t *testing.T, path string, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}
