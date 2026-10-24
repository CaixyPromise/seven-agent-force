package docker

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/CaixyPromise/seven-agent/force/internal/shared/config"
)

func TestComposeBuilderMetadataReflectsConfig(t *testing.T) {
	svc := &service{cfg: config.DockerConfig{Compose: config.DockerComposeConfig{
		WorkspaceRoots:             []string{"/tmp/compose-a", "/tmp/compose-b"},
		DefaultFileName:            "docker-compose.yaml",
		MaxComposeBytes:            123,
		MaxDockerfileBytes:         456,
		MaxExtraFilesBytes:         789,
		AllowedProjectFileSuffixes: []string{".env", ".yaml"},
	}}}
	meta, err := svc.GetComposeBuilderMetadata(nil)
	if err != nil {
		t.Fatalf("get metadata: %v", err)
	}
	if meta.DefaultFileName != "docker-compose.yaml" || meta.MaxComposeBytes != 123 || meta.MaxDockerfileBytes != 456 {
		t.Fatalf("unexpected metadata: %+v", meta)
	}
	if len(meta.WorkspaceRoots) != 2 || meta.WorkspaceRoots[0] == "" {
		t.Fatalf("unexpected workspace roots: %+v", meta.WorkspaceRoots)
	}
	if len(meta.SupportedServiceFields) == 0 || meta.DefaultService.Restart == "" {
		t.Fatalf("expected supported service fields and defaults: %+v", meta)
	}
}

func TestComposeWorkspaceCheckAllowsConfiguredRoot(t *testing.T) {
	root := t.TempDir()
	manager := newComposeWorkspaceManager(config.DockerComposeConfig{WorkspaceRoots: []string{root}})

	view, err := manager.Check(ComposeWorkspaceCheckRequest{
		WorkingDir:      filepath.Join(root, "demo"),
		CreateIfMissing: true,
	})
	if err != nil {
		t.Fatalf("check workspace: %v", err)
	}
	if !view.Valid || !view.AllowedRoot || !view.CanCreate {
		t.Fatalf("unexpected workspace view: %+v", view)
	}
}

func TestComposeWorkspaceCheckRejectsPathTraversal(t *testing.T) {
	root := t.TempDir()
	manager := newComposeWorkspaceManager(config.DockerComposeConfig{WorkspaceRoots: []string{root}})

	view, err := manager.Check(ComposeWorkspaceCheckRequest{WorkingDir: filepath.Join(root, "..", "escape")})
	if err != nil {
		t.Fatalf("check workspace should return view error, got: %v", err)
	}
	if view.Valid {
		t.Fatalf("path traversal should be invalid: %+v", view)
	}
}

func TestComposeWriteProjectFilesWritesComposeAndDockerfile(t *testing.T) {
	root := t.TempDir()
	manager := newComposeWorkspaceManager(config.DockerComposeConfig{
		WorkspaceRoots:     []string{root},
		DefaultFileName:    "docker-compose.yaml",
		MaxComposeBytes:    1024 * 1024,
		MaxDockerfileBytes: 1024,
		MaxExtraFilesBytes: 1024,
	})

	result, err := manager.WriteProjectFiles("demo", "", `services:
  app:
    build:
      context: ./app
      dockerfile: Dockerfile
    image: demo/app:latest
`, []ComposeBuildFileCommand{{
		ServiceName:       "app",
		Context:           "./app",
		DockerfilePath:    "Dockerfile",
		DockerfileContent: "FROM scratch\n",
	}}, false)
	if err != nil {
		t.Fatalf("write project files: %v", err)
	}
	if _, err := os.Stat(result.ComposeFilePath); err != nil {
		t.Fatalf("compose file not written: %v", err)
	}
	if filepath.Base(result.ComposeFilePath) != "docker-compose.yaml" {
		t.Fatalf("unexpected compose file name: %s", result.ComposeFilePath)
	}
	if _, err := os.Stat(filepath.Join(result.WorkingDir, "app", "Dockerfile")); err != nil {
		t.Fatalf("dockerfile not written: %v", err)
	}
	if result.FileManifestJSON == "" || result.ConfigFilesJSON == "" {
		t.Fatalf("missing manifest/config files json: %+v", result)
	}
}

func TestComposeDockerfilePreviewRejectsEscapingContext(t *testing.T) {
	root := t.TempDir()
	manager := newComposeWorkspaceManager(config.DockerComposeConfig{WorkspaceRoots: []string{root}})

	view, err := manager.PreviewDockerfile(DockerfileBuildPreviewRequest{
		ProjectName:    "demo",
		WorkingDir:     filepath.Join(root, "demo"),
		ServiceName:    "app",
		Context:        "../escape",
		DockerfilePath: "Dockerfile",
	})
	if err != nil {
		t.Fatalf("preview dockerfile: %v", err)
	}
	if view.Valid {
		t.Fatalf("escaping context should be invalid: %+v", view)
	}
}

func TestParseComposeYamlForBuilder(t *testing.T) {
	view, err := parseComposeYamlForBuilder(`name: demo
services:
  web:
    image: nginx:1.25
    ports:
      - "8080:80/tcp"
    environment:
      NODE_ENV: production
    volumes:
      - ./data:/var/lib/data:ro
    networks:
      - frontend
    depends_on:
      - db
    restart: unless-stopped
    working_dir: /app
    user: app
    healthcheck:
      test: ["CMD", "curl", "-f", "http://localhost:80/health"]
      interval: 30s
      timeout: 5s
      retries: 3
      start_period: 10s
    deploy:
      resources:
        limits:
          cpus: "1.0"
          memory: 512M
        reservations:
          memory: 256M
    privileged: true
    network_mode: bridge
    pid: host
    ipc: host
    cap_add:
      - NET_ADMIN
    x-extra: true
networks:
  frontend:
    driver: bridge
volumes:
  data: {}
`)
	if err != nil {
		t.Fatalf("parse compose yaml: %v", err)
	}
	if len(view.Services) != 1 || view.Services[0].ServiceName != "web" || view.Services[0].Image != "nginx:1.25" {
		t.Fatalf("unexpected services: %+v", view.Services)
	}
	if len(view.Services[0].Ports) != 1 || view.Services[0].Ports[0].PublicPort != 8080 || view.Services[0].Ports[0].PrivatePort != 80 {
		t.Fatalf("unexpected ports: %+v", view.Services[0].Ports)
	}
	if view.VisualDraft == nil || len(view.VisualDraft.Services) != 1 {
		t.Fatalf("visual draft not populated: %+v", view.VisualDraft)
	}
	draft := view.VisualDraft.Services[0]
	if draft.Healthcheck == nil || draft.Resources == nil || draft.Advanced == nil {
		t.Fatalf("expected healthcheck/resources/advanced in visual draft: %+v", draft)
	}
	if len(draft.Environment) != 1 || draft.Environment[0].Key != "NODE_ENV" || draft.Environment[0].Value != "production" {
		t.Fatalf("unexpected env parse: %+v", draft.Environment)
	}
	if len(draft.Networks) != 1 || draft.Networks[0] != "frontend" {
		t.Fatalf("unexpected networks parse: %+v", draft.Networks)
	}
	if len(draft.DependsOn) != 1 || draft.DependsOn[0] != "db" {
		t.Fatalf("unexpected depends_on parse: %+v", draft.DependsOn)
	}
	if draft.Resources.CPUs != "1.0" || draft.Resources.Memory != "512M" || draft.Resources.MemoryReservation != "256M" {
		t.Fatalf("unexpected resources parse: %+v", draft.Resources)
	}
	if !draft.Advanced.Privileged || draft.Advanced.NetworkMode != "bridge" || draft.Advanced.PID != "host" || draft.Advanced.IPC != "host" {
		t.Fatalf("unexpected advanced parse: %+v", draft.Advanced)
	}
	if len(view.UnsupportedFields) != 1 || view.UnsupportedFields[0].Path != "services.web.x-extra" {
		t.Fatalf("unexpected unsupported fields: %+v", view.UnsupportedFields)
	}
	if len(view.VisualDraft.Services[0].UnsupportedFields) != 1 || view.VisualDraft.Services[0].UnsupportedFields[0].Path != "services.web.x-extra" {
		t.Fatalf("unexpected visual unsupported fields: %+v", view.VisualDraft.Services[0].UnsupportedFields)
	}
	if len(view.VisualDraft.Networks) != 1 || view.VisualDraft.Networks[0].Name != "frontend" {
		t.Fatalf("unexpected visual networks: %+v", view.VisualDraft.Networks)
	}
	if len(view.VisualDraft.Volumes) != 1 || view.VisualDraft.Volumes[0].Name != "data" {
		t.Fatalf("unexpected visual volumes: %+v", view.VisualDraft.Volumes)
	}
}
