package docker

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	dockersystem "github.com/docker/docker/api/types/system"
)

func TestMergeDaemonConfigPreservesReadonlyKeys(t *testing.T) {
	raw := map[string]any{
		"data-root":        "/var/lib/docker",
		"registry-mirrors": []any{"https://old.example.com"},
		"iptables":         true,
	}
	editable, err := validateDaemonEditable(map[string]any{
		"registry-mirrors": []any{"https://mirror.example.com"},
		"live-restore":     true,
	})
	if err != nil {
		t.Fatalf("validate editable: %v", err)
	}
	merged := mergeDaemonConfig(raw, editable)
	if merged["data-root"] != "/var/lib/docker" {
		t.Fatalf("readonly key not preserved: %+v", merged)
	}
	if merged["live-restore"] != true {
		t.Fatalf("editable key not merged: %+v", merged)
	}
	mirrors, ok := merged["registry-mirrors"].([]string)
	if !ok || len(mirrors) != 1 || mirrors[0] != "https://mirror.example.com" {
		t.Fatalf("registry mirrors not normalized: %#v", merged["registry-mirrors"])
	}
}

func TestDaemonConfigRejectsReadonlyFieldUpdate(t *testing.T) {
	_, err := validateDaemonEditable(map[string]any{"data-root": "/tmp/docker"})
	if err == nil {
		t.Fatalf("expected readonly field rejection")
	}
}

func TestDaemonConfigDetectsRemoteAndRootlessUnsupported(t *testing.T) {
	if !isRemoteDockerHost("tcp://127.0.0.1:2375") {
		t.Fatalf("tcp host should be remote")
	}
	if isRemoteDockerHost("unix:///var/run/docker.sock") {
		t.Fatalf("unix host should be local")
	}
	if !dockerInfoRootless(dockersystem.Info{SecurityOptions: []string{"name=rootless"}}) {
		t.Fatalf("rootless security option not detected")
	}
	if !dockerInfoRootless(dockersystem.Info{DockerRootDir: "/home/demo/.local/share/docker"}) {
		t.Fatalf("rootless docker root dir not detected")
	}
}

func TestDaemonConfigBackupAndAtomicWrite(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "daemon.json")
	if err := os.WriteFile(path, []byte(`{"data-root":"/var/lib/docker"}`), 0600); err != nil {
		t.Fatalf("write original: %v", err)
	}
	if err := backupAndWriteDaemonConfig(path, map[string]any{"data-root": "/var/lib/docker", "live-restore": true}); err != nil {
		t.Fatalf("backup and write: %v", err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read written config: %v", err)
	}
	var raw map[string]any
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatalf("written config invalid json: %v", err)
	}
	if raw["live-restore"] != true || raw["data-root"] != "/var/lib/docker" {
		t.Fatalf("unexpected written config: %+v", raw)
	}
	matches, err := filepath.Glob(filepath.Join(dir, "daemon.json.bak.*"))
	if err != nil {
		t.Fatalf("glob backups: %v", err)
	}
	if len(matches) != 1 {
		t.Fatalf("expected one backup, got %d", len(matches))
	}
}

func TestDaemonConfigReadRejectsInvalidJSON(t *testing.T) {
	path := filepath.Join(t.TempDir(), "daemon.json")
	if err := os.WriteFile(path, []byte(`{"broken"`), 0600); err != nil {
		t.Fatalf("write invalid json: %v", err)
	}
	if _, err := readDaemonConfigFile(path); err == nil {
		t.Fatalf("expected invalid json error")
	}
}
