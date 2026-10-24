// Package resourcejournal owns Worker-private cleanup facts, never business results.
package resourcejournal

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"time"
)

const Timeout = 30 * time.Second
const OwnershipLabel = "seven.cleanup.owner"

type Resource struct {
	Key       string            `json:"key"`
	Kind      string            `json:"kind"`
	Name      string            `json:"name"`
	ID        string            `json:"id,omitempty"`
	Labels    map[string]string `json:"labels"`
	Pending   bool              `json:"pending"`
	Failures  int               `json:"failures"`
	LastError string            `json:"lastError,omitempty"`
	NextRetry time.Time         `json:"nextRetry,omitempty"`
	Path      string            `json:"path,omitempty"`
	DependsOn []string          `json:"dependsOn,omitempty"`
}

type Journal struct {
	mu             sync.Mutex
	root           string
	rows           map[string]Resource
	fault          bool
	containerLimit int
}

func Open(root string) (*Journal, error) {
	if !filepath.IsAbs(root) {
		return nil, errors.New("CLEANUP_ROOT_INVALID")
	}
	if err := os.MkdirAll(root, 0700); err != nil {
		return nil, err
	}
	info, err := os.Lstat(root)
	if err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 || info.Mode().Perm()&0077 != 0 {
		return nil, errors.New("CLEANUP_ROOT_NOT_PRIVATE")
	}
	canonical, err := filepath.EvalSymlinks(root)
	if err != nil {
		return nil, errors.New("CLEANUP_ROOT_NOT_CANONICAL")
	}
	root = canonical
	j := &Journal{root: root, rows: map[string]Resource{}}
	entries, err := os.ReadDir(root)
	if err != nil {
		return nil, err
	}
	for _, entry := range entries {
		if filepath.Ext(entry.Name()) != ".json" {
			continue
		}
		info, err := entry.Info()
		if err != nil || !info.Mode().IsRegular() || info.Mode().Perm()&0077 != 0 {
			return nil, errors.New("CLEANUP_RECORD_INVALID")
		}
		body, err := os.ReadFile(filepath.Join(root, entry.Name()))
		if err != nil {
			return nil, err
		}
		var row Resource
		if json.Unmarshal(body, &row) != nil || !valid(row) || entry.Name() != row.Key+".json" {
			return nil, errors.New("CLEANUP_RECORD_INVALID")
		}
		j.rows[row.Key] = row
	}
	return j, nil
}

func valid(r Resource) bool {
	key, err := hex.DecodeString(r.Key)
	return err == nil && len(key) == 16 && (r.Kind == "container" || r.Kind == "network" || r.Kind == "volume" || (r.Kind == "directory" && filepath.IsAbs(r.Path) && r.Path != string(filepath.Separator))) && r.Name != "" && len(r.Labels) > 0
}

// Begin is durable before Docker create, including an unguessable ownership label
// so a response-loss retry cannot remove a replacement resource with the same name.
func (j *Journal) Begin(kind, name string, labels map[string]string) (Resource, error) {
	var nonce [16]byte
	if _, err := rand.Read(nonce[:]); err != nil {
		return Resource{}, err
	}
	r := Resource{Key: hex.EncodeToString(nonce[:]), Kind: kind, Name: name, Labels: map[string]string{}}
	for k, v := range labels {
		r.Labels[k] = v
	}
	r.Labels[OwnershipLabel] = r.Key
	if !valid(r) {
		return Resource{}, errors.New("CLEANUP_RECORD_INVALID")
	}
	j.mu.Lock()
	defer j.mu.Unlock()
	if j.fault {
		return Resource{}, errors.New("CLEANUP_JOURNAL_UNHEALTHY")
	}
	containers := 0
	for _, old := range j.rows {
		if old.Pending {
			return Resource{}, errors.New("CLEANUP_CAPACITY_RESERVED")
		}
		if old.Kind == "container" {
			containers++
		}
	}
	if kind == "container" && j.containerLimit > 0 && containers >= j.containerLimit {
		return Resource{}, errors.New("CLEANUP_CONTAINER_CAPACITY_EXHAUSTED")
	}
	for _, old := range j.rows {
		if old.Kind == kind && old.Name == name {
			return Resource{}, errors.New("CLEANUP_RESOURCE_ALREADY_RESERVED")
		}
	}
	return r, j.save(r)
}

func (j *Journal) SetContainerLimit(limit int) {
	j.mu.Lock()
	defer j.mu.Unlock()
	j.containerLimit = limit
}

func (j *Journal) save(r Resource) error {
	// Retain the in-memory reservation even if durable storage fails.
	j.rows[r.Key] = r
	body, err := json.Marshal(r)
	if err != nil {
		return err
	}
	f, err := os.CreateTemp(j.root, ".write-")
	if err != nil {
		j.fault = true
		return err
	}
	defer os.Remove(f.Name())
	if _, err = f.Write(body); err == nil {
		err = f.Sync()
	}
	err = errors.Join(err, f.Close())
	if err == nil {
		err = os.Rename(f.Name(), filepath.Join(j.root, r.Key+".json"))
	}
	if err == nil {
		err = syncDirectory(j.root)
	}
	if err != nil {
		j.fault = true
	}
	return err
}

func syncDirectory(root string) error {
	f, err := os.Open(root)
	if err != nil {
		return err
	}
	defer f.Close()
	return f.Sync()
}

func (j *Journal) Find(kind, target string) (Resource, bool) {
	j.mu.Lock()
	defer j.mu.Unlock()
	for _, r := range j.rows {
		if r.Kind == kind && (r.ID == target || r.Name == target) {
			return r, true
		}
	}
	return Resource{}, false
}

func (j *Journal) Bind(key, id string) error {
	j.mu.Lock()
	defer j.mu.Unlock()
	r, ok := j.rows[key]
	if !ok || id == "" {
		return errors.New("CLEANUP_RECORD_MISSING")
	}
	if r.ID != "" && r.ID != id {
		return errors.New("CLEANUP_IDENTITY_CONFLICT")
	}
	r.ID = id
	return j.save(r)
}

// Adopt accepts only a caller-verified Server-owned identity for pre-journal
// runtimes. The remover still verifies those exact labels against Docker.
func (j *Journal) Adopt(kind, id string, labels map[string]string) (Resource, error) {
	if r, ok := j.Find(kind, id); ok {
		return r, nil
	}
	var nonce [16]byte
	if _, err := rand.Read(nonce[:]); err != nil {
		return Resource{}, err
	}
	r := Resource{Key: hex.EncodeToString(nonce[:]), Kind: kind, Name: id, ID: id, Labels: labels}
	if !valid(r) {
		return Resource{}, errors.New("CLEANUP_RECORD_INVALID")
	}
	j.mu.Lock()
	defer j.mu.Unlock()
	return r, j.save(r)
}

func (j *Journal) CleanupMatching(labels map[string]string, remove func(context.Context, Resource) error) error {
	j.mu.Lock()
	var resources []Resource
	for k, r := range j.rows {
		match := len(labels) > 0
		for key, value := range labels {
			if r.Labels[key] != value {
				match = false
			}
		}
		if match {
			row := r
			row.Key = k
			resources = append(resources, row)
		}
	}
	j.mu.Unlock()
	// Docker networks cannot be removed while their containers are attached,
	// and private directories may only disappear after every mounted resource.
	// Stable kind/name ordering makes one cleanup pass converge deterministically.
	priority := func(kind string) int {
		switch kind {
		case "container":
			return 0
		case "volume":
			return 1
		case "network":
			return 2
		default:
			return 3
		}
	}
	sort.Slice(resources, func(i, k int) bool {
		if priority(resources[i].Kind) != priority(resources[k].Kind) {
			return priority(resources[i].Kind) < priority(resources[k].Kind)
		}
		return resources[i].Name < resources[k].Name
	})
	var errs error
	for _, resource := range resources {
		if err := j.Cleanup(resource.Key, remove); err != nil {
			errs = errors.Join(errs, fmt.Errorf("%s %s: %w", resource.Kind, resource.Name, err))
		}
	}
	return errs
}

func (j *Journal) Pending() int {
	j.mu.Lock()
	defer j.mu.Unlock()
	n := 0
	for _, r := range j.rows {
		if r.Pending {
			n++
		}
	}
	if j.fault {
		n++
	}
	return n
}

// Recover is invoked before accepting new capacity. The caller protects groups
// covered by Server recovery authority; all other creation intents become cleanup.
func (j *Journal) Recover(protect func(Resource) bool) error {
	j.mu.Lock()
	defer j.mu.Unlock()
	var errs error
	for _, r := range j.rows {
		if !protect(r) {
			r.Pending = true
			r.NextRetry = time.Time{}
			errs = errors.Join(errs, j.save(r))
		}
	}
	return errs
}

func RetryDelay(failures int) time.Duration {
	d := time.Second
	for i := 1; i < failures && d < 5*time.Minute; i++ {
		d *= 2
	}
	if d > 5*time.Minute {
		d = 5 * time.Minute
	}
	return d
}

// Cleanup uses one independent, bounded context. Errors are safe reason codes:
// Docker stderr and provider bodies must never enter this journal.
func (j *Journal) Cleanup(key string, remove func(context.Context, Resource) error) error {
	j.mu.Lock()
	defer j.mu.Unlock()
	r, ok := j.rows[key]
	if !ok {
		return nil
	}
	r.Pending = true
	if err := j.save(r); err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), Timeout)
	defer cancel()
	var err error
	for _, key := range r.DependsOn {
		if _, exists := j.rows[key]; exists {
			err = errors.New("CLEANUP_DEPENDENCY_PENDING")
			break
		}
	}
	if err == nil {
		if r.Kind == "directory" {
			err = removeDirectory(r)
		} else {
			err = remove(ctx, r)
		}
	}
	if err == nil {
		err = os.Remove(filepath.Join(j.root, key+".json"))
		if err == nil {
			err = syncDirectory(j.root)
		}
		if err == nil {
			delete(j.rows, key)
			return nil
		}
	}
	r.Failures++
	r.LastError = "RESOURCE_CLEANUP_FAILED"
	switch err.Error() {
	case "CLEANUP_DEPENDENCY_PENDING", "CLEANUP_INSPECT_FAILED", "CLEANUP_INSPECT_INVALID", "CLEANUP_IDENTITY_CONFLICT", "CLEANUP_OWNERSHIP_CONFLICT", "CLEANUP_REMOVE_FAILED", "CLEANUP_ABSENCE_UNCONFIRMED", "CLEANUP_DIRECTORY_IDENTITY_CONFLICT", "CLEANUP_DIRECTORY_OWNER_CONFLICT", "CLEANUP_DIRECTORY_REMOVE_FAILED", "CLEANUP_DIRECTORY_ABSENCE_UNCONFIRMED":
		r.LastError = err.Error()
	}
	r.NextRetry = time.Now().UTC().Add(RetryDelay(r.Failures))
	return errors.Join(err, j.save(r))
}

func (j *Journal) Retry(remove func(context.Context, Resource) error) error {
	j.mu.Lock()
	var keys []string
	for k, r := range j.rows {
		if r.Pending && !time.Now().Before(r.NextRetry) {
			keys = append(keys, k)
		}
	}
	j.mu.Unlock()
	var errs error
	for _, k := range keys {
		errs = errors.Join(errs, j.Cleanup(k, remove))
	}
	return errs
}
