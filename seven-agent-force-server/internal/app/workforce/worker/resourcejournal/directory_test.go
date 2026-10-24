package resourcejournal

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestPrivateScratchWaitsForExactContainerAndPreservesResults(t *testing.T) {
	j := openTest(t)
	root := t.TempDir()
	scratch := filepath.Join(root, "scratch")
	retained := filepath.Join(root, "result.json")
	if e := os.Mkdir(scratch, 0700); e != nil {
		t.Fatal(e)
	}
	if e := os.WriteFile(retained, []byte("trusted-result"), 0600); e != nil {
		t.Fatal(e)
	}
	dir, e := j.TrackDirectory(scratch, map[string]string{"seven.attempt.id": "7"})
	if e != nil {
		t.Fatal(e)
	}
	r, e := j.Begin("container", "owned", nil)
	if e != nil {
		t.Fatal(e)
	}
	if e = j.linkMounts(r.Key, []string{"--mount", "type=bind,src=" + dir.Path + ",dst=/workspace"}); e != nil {
		t.Fatal(e)
	}
	if e = j.Cleanup(dir.Key, nil); e == nil {
		t.Fatal("removed directory while container exists")
	}
	if _, e = os.Stat(scratch); e != nil {
		t.Fatal("scratch removed early", e)
	}
	if e = j.Cleanup(r.Key, func(context.Context, Resource) error { return nil }); e != nil {
		t.Fatal(e)
	}
	if e = j.Cleanup(dir.Key, nil); e != nil {
		t.Fatal(e)
	}
	if _, e = os.Stat(scratch); !os.IsNotExist(e) {
		t.Fatal("scratch survived verified cleanup")
	}
	if body, e := os.ReadFile(retained); e != nil || string(body) != "trusted-result" {
		t.Fatal("trusted result changed")
	}
}

func TestDirectoryReplacementIsRetained(t *testing.T) {
	j := openTest(t)
	path := filepath.Join(t.TempDir(), "scratch")
	if e := os.Mkdir(path, 0700); e != nil {
		t.Fatal(e)
	}
	r, e := j.TrackDirectory(path, nil)
	if e != nil {
		t.Fatal(e)
	}
	if e = os.WriteFile(filepath.Join(path, directoryMarker), []byte("different-owner"), 0600); e != nil {
		t.Fatal(e)
	}
	if e = j.Cleanup(r.Key, nil); e == nil {
		t.Fatal("removed replacement directory")
	}
	if _, e = os.Stat(path); e != nil {
		t.Fatal(e)
	}
}

func TestResponseLossLeavesDurableIntentUntilDaemonCanVerify(t *testing.T) {
	j := openTest(t)
	run := func(context.Context, string, ...string) (string, error) {
		return "", errors.New("daemon disconnected after create")
	}
	if _, e := j.Execute(context.Background(), run, "docker", "create", "--name", "uncertain", "image"); e == nil {
		t.Fatal("lost create treated as success")
	}
	r, ok := j.Find("container", "uncertain")
	if !ok || !r.Pending || r.ID != "" {
		t.Fatalf("uncertain create lost: %+v", r)
	}
	reopened, e := Open(j.root)
	if e != nil {
		t.Fatal(e)
	}
	if reopened.Pending() != 1 {
		t.Fatal("restart released uncertain capacity")
	}
}
