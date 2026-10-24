package main

import (
	"context"
	"encoding/json"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/CaixyPromise/seven-agent/force/internal/worker/agentprotocol"
)

func reviewAdapterFixture(t *testing.T) (reviewAdapterPaths, codexReviewInput) {
	t.Helper()
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	p := reviewAdapterPaths{input: filepath.Join(root, "input.json"), repository: filepath.Join(root, "repository.git"), workspace: filepath.Join(root, "repo"), schema: filepath.Join(root, "schema.json"), report: filepath.Join(root, "report.json"), requirements: filepath.Join(root, "requirements.json"), reviewContract: filepath.Join(root, "review-contract.json")}
	git := func(stdin string, args ...string) string {
		t.Helper()
		cmd := exec.Command("git", args...)
		cmd.Env = append(adapterEnvironment(), "GIT_CONFIG_NOSYSTEM=1", "GIT_CONFIG_GLOBAL=/dev/null", "GIT_AUTHOR_NAME=Fixture", "GIT_AUTHOR_EMAIL=fixture@example.invalid", "GIT_COMMITTER_NAME=Fixture", "GIT_COMMITTER_EMAIL=fixture@example.invalid")
		cmd.Stdin = strings.NewReader(stdin)
		out, err := cmd.Output()
		if err != nil {
			t.Fatalf("temporary git fixture: %v", err)
		}
		return strings.TrimSpace(string(out))
	}
	git("", "init", "--bare", p.repository)
	tree := git("", "--git-dir", p.repository, "mktree")
	commit := git("fixture\n", "--git-dir", p.repository, "commit-tree", tree)
	git("", "--git-dir", p.repository, "update-ref", "refs/heads/main", commit)
	input := codexReviewInput{SchemaVersion: 1, ReviewRunID: "910000000000000001", TaskID: "910000000000000002", ResultCommit: commit, ResultTree: tree, ReviewContract: json.RawMessage(`{}`), Requirements: json.RawMessage(`{}`)}
	return p, input
}

func TestReviewAdapterCapabilityAndClosedInput(t *testing.T) {
	for _, brand := range []string{"codex", "fake", "unrelated"} {
		for _, gateway := range []bool{false, true} {
			caps := []string{}
			if gateway {
				caps = append(caps, "review.provider.gateway-client")
			}
			body, _ := json.Marshal(map[string]any{"harness": map[string]string{"type": brand}, "capabilities": caps})
			got, err := reviewAdapterNeedsGateway(body)
			if err != nil || got != gateway {
				t.Fatalf("brand changed capability: %v %v", got, err)
			}
		}
	}
	input := codexReviewInput{SchemaVersion: 1, ReviewRunID: "1", TaskID: "2", ResultCommit: strings.Repeat("a", 40), ResultTree: strings.Repeat("b", 40), Requirements: json.RawMessage(`{}`), ReviewContract: json.RawMessage(`{}`)}
	body, _ := json.Marshal(input)
	if _, err := decodeReviewAdapterInput(body, false); err != nil {
		t.Fatal(err)
	}
	if _, err := decodeReviewAdapterInput(body, true); err == nil {
		t.Fatal("gateway accepted missing provider")
	}
	input.Provider = &codexReviewProviderIdentity{CredentialID: "3", CredentialVersion: "1", Protocol: codexReviewProviderProtocol, Model: "frozen-model", StructuredContract: codexReviewStructuredContract}
	body, _ = json.Marshal(input)
	if _, err := decodeReviewAdapterInput(body, true); err != nil {
		t.Fatal(err)
	}
	for _, bad := range [][]byte{append(append([]byte{}, body...), []byte(` {}`)...), []byte(strings.Replace(string(body), `"schemaVersion":1`, `"schemaVersion":1,"verdict":"PASS"`, 1)), []byte(strings.Replace(string(body), `"requirements":{}`, `"requirements":[]`, 1)), []byte(strings.Repeat("x", reviewAdapterFileLimit+1))} {
		if _, err := decodeReviewAdapterInput(bad, false); err == nil {
			t.Fatal("unsafe review input accepted")
		}
	}
	if err := reviewMain([]string{"--model=override"}); err == nil {
		t.Fatal("review accepted args")
	}
}

func TestReviewAdapterPromptDescribesCanonicalCommandBoundary(t *testing.T) {
	for _, required := range []string{"without shell metacharacters", "put the observed outcome in summary instead"} {
		if !strings.Contains(reviewAdapterPrompt, required) {
			t.Fatalf("review prompt does not describe the canonical command boundary %q", required)
		}
	}
}

func TestReviewAdapterCheckoutIdentity(t *testing.T) {
	for _, fault := range []string{"none", "commit", "tree"} {
		t.Run(fault, func(t *testing.T) {
			paths, input := reviewAdapterFixture(t)
			if fault == "commit" {
				input.ResultCommit = strings.Repeat("a", 40)
			}
			if fault == "tree" {
				input.ResultTree = strings.Repeat("b", 40)
			}
			err := prepareReviewAdapterRepository(context.Background(), input, paths.repository, paths.workspace)
			if (err == nil) != (fault == "none") {
				t.Fatalf("identity check: %v", err)
			}
		})
	}
}

func TestReviewAdapterCheckoutPrivateResultRefWithoutHeads(t *testing.T) {
	paths, input := reviewAdapterFixture(t)
	git := func(stdin string, args ...string) string {
		t.Helper()
		cmd := exec.Command("git", args...)
		cmd.Env = append(adapterEnvironment(), "GIT_CONFIG_NOSYSTEM=1", "GIT_CONFIG_GLOBAL=/dev/null", "GIT_AUTHOR_NAME=Fixture", "GIT_AUTHOR_EMAIL=fixture@example.invalid", "GIT_COMMITTER_NAME=Fixture", "GIT_COMMITTER_EMAIL=fixture@example.invalid")
		cmd.Stdin = strings.NewReader(stdin)
		body, err := cmd.Output()
		if err != nil {
			t.Fatalf("temporary private-ref Git fixture: %v", err)
		}
		return strings.TrimSpace(string(body))
	}
	// Finalizer's result bare advertises a private ref, with neither heads nor
	// tags. Include real checkout contents so an empty .git-only clone fails.
	blob := git("good\n", "--git-dir", paths.repository, "hash-object", "-w", "--stdin")
	input.ResultTree = git("100644 blob "+blob+"\tvalue.txt\n", "--git-dir", paths.repository, "mktree")
	input.ResultCommit = git("frozen private result\n", "--git-dir", paths.repository, "commit-tree", input.ResultTree)
	git("", "--git-dir", paths.repository, "update-ref", "refs/seven-workforce/review/result", input.ResultCommit)
	git("", "--git-dir", paths.repository, "update-ref", "-d", "refs/heads/main")
	refsBefore := git("", "--git-dir", paths.repository, "for-each-ref", "--format=%(refname) %(objectname)")
	if refsBefore != "refs/seven-workforce/review/result "+input.ResultCommit {
		t.Fatalf("invalid private-ref fixture: %s", refsBefore)
	}
	configPath := filepath.Join(paths.repository, "config")
	configBefore, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(configPath, 0o444); err != nil {
		t.Fatal(err)
	}
	if err := prepareReviewAdapterRepository(context.Background(), input, paths.repository, paths.workspace); err != nil {
		t.Fatalf("private result commit must be fetched before checkout: %v", err)
	}
	if got := git("", "-C", paths.workspace, "rev-parse", "--verify", "HEAD"); got != input.ResultCommit {
		t.Fatal("wrong ResultCommit checkout")
	}
	if got := git("", "-C", paths.workspace, "rev-parse", "--verify", "HEAD^{tree}"); got != input.ResultTree {
		t.Fatal("wrong ResultTree checkout")
	}
	value, err := os.ReadFile(filepath.Join(paths.workspace, "value.txt"))
	if err != nil || string(value) != "good\n" {
		t.Fatal("frozen checkout file missing")
	}
	if got := git("", "--git-dir", paths.repository, "for-each-ref", "--format=%(refname) %(objectname)"); got != refsBefore {
		t.Fatal("source refs changed")
	}
	configAfter, err := os.ReadFile(configPath)
	if err != nil || string(configAfter) != string(configBefore) {
		t.Fatal("source config changed")
	}
}

func TestReviewAdapterProcessRoundAndReportBoundary(t *testing.T) {
	for _, behavior := range []string{"success", "gateway", "ready-mismatch", "failed", "missing", "symlink", "oversized"} {
		t.Run(behavior, func(t *testing.T) {
			paths, input := reviewAdapterFixture(t)
			gateway := behavior == "gateway"
			if gateway {
				input.Provider = &codexReviewProviderIdentity{CredentialID: "3", CredentialVersion: "1", Protocol: codexReviewProviderProtocol, Model: "frozen-model", StructuredContract: codexReviewStructuredContract}
			}
			body, _ := json.Marshal(input)
			if err := os.WriteFile(paths.input, body, 0o600); err != nil {
				t.Fatal(err)
			}
			executable, err := os.Executable()
			if err != nil {
				t.Fatal(err)
			}
			descriptor := agentprotocol.Descriptor{ID: "test-review", Version: "1", Executable: executable, Args: []string{"-test.run=^TestReviewAdapterProcessHelper$", "--", "--review-adapter-test", behavior}, Capabilities: []string{"execution.start", "execution.interrupt"}}
			started, stopped := false, false
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			capture, err := os.CreateTemp(t.TempDir(), "review-stdout-")
			if err != nil {
				t.Fatal(err)
			}
			previousOutput := os.Stdout
			os.Stdout = capture
			err = executeReviewAdapter(ctx, paths, descriptor, gateway, func(codexReviewInput) (func(), error) { started = true; return func() { stopped = true }, nil })
			os.Stdout = previousOutput
			info, statErr := capture.Stat()
			_ = capture.Close()
			if statErr != nil || info.Size() != 0 {
				t.Fatal("review wrote private output to stdout")
			}
			wantSuccess := behavior == "success" || gateway
			if (err == nil) != wantSuccess {
				t.Fatalf("review result: %v", err)
			}
			if started != gateway || stopped != gateway {
				t.Fatalf("gateway lifecycle: %v %v", started, stopped)
			}
		})
	}
}

func TestReviewAdapterProcessHelper(t *testing.T) {
	marker := -1
	for i, arg := range os.Args {
		if arg == "--review-adapter-test" {
			marker = i
		}
	}
	if marker < 0 {
		return
	}
	behavior := os.Args[marker+1]
	decoder := json.NewDecoder(os.Stdin)
	var request agentprotocol.Frame
	if decoder.Decode(&request) != nil {
		os.Exit(2)
	}
	sequence := 0
	send := func(method, reply string, payload any) {
		sequence++
		body, _ := json.Marshal(payload)
		_ = json.NewEncoder(os.Stdout).Encode(agentprotocol.Frame{Protocol: agentprotocol.Version, Method: method, Key: strings.Repeat("x", sequence), ReplyTo: reply, Payload: body})
	}
	name := "test-review"
	if behavior == "ready-mismatch" {
		name = "other"
	}
	send("ready", request.Key, map[string]any{"name": name, "version": "1", "capabilities": []string{"execution.start", "execution.interrupt"}})
	if decoder.Decode(&request) != nil {
		os.Exit(0)
	}
	var round struct {
		RoundKey         string            `json:"roundKey"`
		Inputs           map[string]string `json:"inputs"`
		SessionReference string            `json:"sessionReference"`
	}
	if json.Unmarshal(request.Payload, &round) != nil || request.Method != "execution.start" || round.SessionReference != "" || round.Inputs["mode"] != "REVIEW" || len(round.Inputs) != 10 {
		os.Exit(3)
	}
	for _, key := range []string{"workspace", "output_schema", "requirements_path", "review_contract_path"} {
		if _, err := os.Stat(round.Inputs[key]); err != nil {
			os.Exit(4)
		}
	}
	if round.Inputs["prompt"] != reviewAdapterPrompt || !validGitObjectID(round.Inputs["resultCommit"]) || !validGitObjectID(round.Inputs["resultTree"]) {
		os.Exit(5)
	}
	send("execution.started", request.Key, map[string]any{"roundKey": round.RoundKey, "nativePid": os.Getpid()})
	// This private observation must never be printed by the review entrypoint.
	send("message", "", map[string]any{"roundKey": round.RoundKey, "text": "PRIVATE_REVIEW_TEXT"})
	output := round.Inputs["output_path"]
	switch behavior {
	case "missing", "failed":
	case "symlink":
		_ = os.Symlink(round.Inputs["requirements_path"], output)
	case "oversized":
		_ = os.WriteFile(output, []byte(strings.Repeat("x", reviewAdapterFileLimit+1)), 0o600)
	default:
		_ = os.WriteFile(output, []byte(`{"schemaVersion":"review-report/v1","verdict":"PASS","summary":"fixture"}`), 0o600)
	}
	outcome := "FINISHED"
	if behavior == "failed" {
		outcome = "FAILED"
	}
	send("execution.finished", request.Key, map[string]any{"roundKey": round.RoundKey, "outcome": outcome, "summary": "PRIVATE_REVIEW_TEXT"})
	_, _ = io.Copy(io.Discard, os.Stdin)
	os.Exit(0)
}

func TestReviewAdapterRejectsExistingAndLinkedReportPaths(t *testing.T) {
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(root, "report.json")
	if err := reviewAdapterOutputAvailable(path); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("existing"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := reviewAdapterOutputAvailable(path); err == nil {
		t.Fatal("stale report accepted")
	}
	link := filepath.Join(root, "linked")
	if err := os.Symlink(root, link); err != nil {
		t.Fatal(err)
	}
	if err := validateReviewAdapterReport(filepath.Join(link, "report.json")); err == nil {
		t.Fatal("symlink parent accepted")
	}
	if err := writeReviewAdapterDocument(path, []byte("overwrite")); err == nil {
		t.Fatal("existing document overwritten")
	}
}

func TestReviewAdapterInputFileBounds(t *testing.T) {
	root := t.TempDir()
	for _, size := range []int{0, reviewAdapterFileLimit + 1} {
		path := filepath.Join(root, "input")
		if err := os.WriteFile(path, []byte(strings.Repeat("x", size)), 0o600); err != nil {
			t.Fatal(err)
		}
		if _, err := readReviewAdapterFile(path); err == nil {
			t.Fatal("invalid input size accepted")
		}
	}
	link := filepath.Join(root, "link")
	if err := os.Symlink(filepath.Join(root, "input"), link); err != nil {
		t.Fatal(err)
	}
	if _, err := readReviewAdapterFile(link); err == nil {
		t.Fatal("linked input accepted")
	}
}
