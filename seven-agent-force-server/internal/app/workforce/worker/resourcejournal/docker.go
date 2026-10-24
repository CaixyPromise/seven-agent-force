package resourcejournal

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"regexp"
	"strings"
)

type Command func(context.Context, string, ...string) (string, error)

// Execute journals explicit Docker creates and intercepts exact removals. It
// never lists/prunes a daemon or adopts resources from an untrusted container.
func (j *Journal) Execute(ctx context.Context, run Command, name string, args ...string) (string, error) {
	if name != "docker" || len(args) == 0 {
		return run(ctx, name, args...)
	}
	if args[0] == "run" {
		// Replace auto-remove one-shot helpers with an explicit, journaled
		// lifetime. Worker loss can then reconcile even an interrupted init.
		var nonce [16]byte
		if _, err := rand.Read(nonce[:]); err != nil {
			return "", err
		}
		create := []string{"create", "--name", "seven-helper-" + hex.EncodeToString(nonce[:]), "--label", "seven.cleanup.ephemeral=true"}
		for _, arg := range args[1:] {
			if arg != "--rm" {
				create = append(create, arg)
			}
		}
		id, err := j.Execute(ctx, run, name, create...)
		if err != nil {
			return "", err
		}
		defer j.Execute(context.Background(), run, name, "rm", "-f", strings.TrimSpace(id))
		return run(ctx, name, "start", "-a", strings.TrimSpace(id))
	}
	kind, op, offset := "container", args[0], 1
	if op == "network" || op == "volume" || op == "container" {
		if len(args) < 2 {
			return run(ctx, name, args...)
		}
		kind, op, offset = op, args[1], 2
	}
	if op == "prune" || args[0] == "system" {
		return "", errors.New("CLEANUP_BROAD_OPERATION_FORBIDDEN")
	}
	if op == "rm" {
		if len(args) <= offset {
			return "", errors.New("CLEANUP_TARGET_REQUIRED")
		}
		target := args[len(args)-1]
		r, ok := j.Find(kind, target)
		if !ok {
			body, err := run(ctx, "docker", kind, "inspect", target)
			if err != nil && missing(body, err) {
				return "", nil
			}
			return "", errors.New("CLEANUP_RESOURCE_NOT_JOURNALED")
		}
		return "", j.Cleanup(r.Key, func(c context.Context, r Resource) error { return Remove(c, run, r) })
	}
	if op != "create" {
		return run(ctx, name, args...)
	}
	target := ""
	labels := map[string]string{}
	for i := offset; i < len(args); i++ {
		if args[i] == "--name" && i+1 < len(args) {
			target = args[i+1]
		}
		if args[i] == "--label" && i+1 < len(args) {
			k, v, ok := strings.Cut(args[i+1], "=")
			if ok {
				labels[k] = v
			}
		}
	}
	if kind != "container" {
		target = args[len(args)-1]
	}
	if target == "" {
		return "", errors.New("CLEANUP_EXPLICIT_NAME_REQUIRED")
	}
	r, err := j.Begin(kind, target, labels)
	if err != nil {
		return "", err
	}
	if err = j.linkMounts(r.Key, args); err != nil {
		return "", err
	}
	wrapped := append([]string{}, args[:offset]...)
	wrapped = append(wrapped, "--label", OwnershipLabel+"="+r.Key)
	wrapped = append(wrapped, args[offset:]...)
	id, err := run(ctx, name, wrapped...)
	if err != nil {
		_ = j.Cleanup(r.Key, func(c context.Context, r Resource) error { return Remove(c, run, r) })
		return "", err
	}
	id = normalizeDockerCreateIdentity(kind, id)
	if id == "" {
		_ = j.Cleanup(r.Key, func(c context.Context, r Resource) error { return Remove(c, run, r) })
		return "", errors.New("CLEANUP_CREATE_IDENTITY_INVALID")
	}
	if err = j.Bind(r.Key, id); err != nil {
		return "", err
	}
	return id, nil
}

var dockerObjectIDPattern = regexp.MustCompile(`^[a-f0-9]{64}$`)

func normalizeDockerCreateIdentity(kind, output string) string {
	if kind == "volume" {
		return strings.TrimSpace(output)
	}
	for _, line := range strings.Split(output, "\n") {
		line = strings.TrimSpace(line)
		if dockerObjectIDPattern.MatchString(line) {
			return line
		}
	}
	return ""
}

// Remove first verifies the exact immutable ID and every ownership label, then
// verifies absence. Only Docker's explicit no-such-object response is absence;
// daemon failures, timeouts and malformed responses remain retryable failures.
func Remove(ctx context.Context, run Command, r Resource) error {
	target := r.ID
	if target == "" {
		target = r.Name
	}
	body, err := run(ctx, "docker", r.Kind, "inspect", target)
	if err != nil {
		if missing(body, err) {
			return nil
		}
		return errors.New("CLEANUP_INSPECT_FAILED")
	}
	var rows []struct {
		ID     string            `json:"Id"`
		Name   string            `json:"Name"`
		Labels map[string]string `json:"Labels"`
		Config struct {
			Labels map[string]string `json:"Labels"`
		} `json:"Config"`
	}
	if json.Unmarshal([]byte(body), &rows) != nil || len(rows) != 1 {
		return errors.New("CLEANUP_INSPECT_INVALID")
	}
	actual := rows[0].ID
	labels := rows[0].Labels
	if r.Kind == "volume" {
		actual = rows[0].Name
	}
	if r.Kind == "container" {
		labels = rows[0].Config.Labels
	}
	if actual == "" || (r.ID != "" && actual != r.ID) {
		return errors.New("CLEANUP_IDENTITY_CONFLICT")
	}
	for k, v := range r.Labels {
		// Optional metadata (for example no Hook bundle) may be explicitly
		// empty. Compare presence as well as value: absent is not empty.
		// The ownership nonce itself must never be empty.
		actualValue, present := labels[k]
		if !present || actualValue != v || (k == OwnershipLabel && v == "") {
			return errors.New("CLEANUP_OWNERSHIP_CONFLICT")
		}
	}
	args := []string{r.Kind, "rm"}
	if r.Kind == "container" {
		args = append(args, "-f", "-v")
	}
	args = append(args, actual)
	if _, err = run(ctx, "docker", args...); err != nil {
		return errors.New("CLEANUP_REMOVE_FAILED")
	}
	body, err = run(ctx, "docker", r.Kind, "inspect", actual)
	if err != nil && missing(body, err) {
		return nil
	}
	return errors.New("CLEANUP_ABSENCE_UNCONFIRMED")
}

func missing(body string, err error) bool {
	// Docker Desktop versions differ in colon placement. Inspect stdout can
	// also contain [] before the daemon diagnostic. Do not append exec's
	// "exit status 1" to a line whose terminal phrase establishes absence.
	for _, source := range []string{body, err.Error()} {
		for _, line := range strings.Split(strings.ToLower(source), "\n") {
			line = strings.TrimSpace(line)
			if strings.Contains(line, "no such container:") || strings.Contains(line, "no such network:") || strings.Contains(line, "no such volume:") || strings.Contains(line, "no such object:") ||
				(strings.HasPrefix(line, "error response from daemon: get ") && strings.HasSuffix(line, ": no such volume")) ||
				(strings.HasPrefix(line, "error response from daemon: network ") && strings.HasSuffix(line, " not found")) {
				return true
			}
		}
	}
	return false
}
