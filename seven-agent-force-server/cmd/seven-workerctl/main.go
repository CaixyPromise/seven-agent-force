// seven-workerctl is the user-facing CLI for the Seven Worker Control Plane.
package main

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/CaixyPromise/seven-agent/force/internal/shared/buildinfo"
)

const (
	defaultServerURL = "http://127.0.0.1:9277"
	maxResponseBytes = 256 << 20
)

type commonFlags struct {
	serverURL string
	token     string
	timeout   time.Duration
}

type stringList []string

func (s *stringList) String() string { return strings.Join(*s, ",") }
func (s *stringList) Set(value string) error {
	*s = append(*s, value)
	return nil
}

func main() { os.Exit(run(os.Args[1:], os.Stdout, os.Stderr, http.DefaultClient)) }

func run(args []string, stdout io.Writer, stderr io.Writer, client *http.Client) int {
	if len(args) == 0 {
		printUsage(stdout)
		return 0
	}
	switch args[0] {
	case "version":
		fmt.Fprintln(stdout, buildinfo.String())
		return 0
	case "health":
		return runHealth(args[1:], stdout, stderr, client)
	case "task":
		return runTask(args[1:], stdout, stderr, client)
	case "artifact":
		return runArtifact(args[1:], stdout, stderr, client)
	case "help", "-h", "--help":
		printUsage(stdout)
		return 0
	default:
		fmt.Fprintf(stderr, "unknown command %q\n", args[0])
		return 2
	}
}

func addCommon(flags *flag.FlagSet) *commonFlags {
	value := &commonFlags{}
	flags.StringVar(&value.serverURL, "server", defaultServerURL, "Control Plane base URL")
	// Never use the environment credential as the flag default. flag prints
	// non-empty defaults in usage output, which would disclose the bearer token
	// whenever parsing fails or --help is requested.
	flags.StringVar(&value.token, "token", "", "Framework access token (or SEVEN_ACCESS_TOKEN)")
	flags.DurationVar(&value.timeout, "timeout", 30*time.Second, "request timeout")
	return value
}

func (c *commonFlags) accessToken() string {
	if value := strings.TrimSpace(c.token); value != "" {
		return value
	}
	return strings.TrimSpace(os.Getenv("SEVEN_ACCESS_TOKEN"))
}

func runHealth(args []string, stdout io.Writer, stderr io.Writer, client *http.Client) int {
	flags := flag.NewFlagSet("health", flag.ContinueOnError)
	flags.SetOutput(stderr)
	serverURL := flags.String("server", defaultServerURL, "Seven Worker Control Plane base URL")
	timeout := flags.Duration("timeout", 5*time.Second, "request timeout")
	if err := flags.Parse(args); err != nil || flags.NArg() != 0 {
		return 2
	}
	return request(client, *timeout, *serverURL, "", http.MethodGet, "/api/healthz", nil, stdout, stderr)
}

func runTask(args []string, stdout io.Writer, stderr io.Writer, client *http.Client) int {
	if len(args) == 0 {
		fmt.Fprintln(stderr, "usage: seven-workerctl task <create|list|get|events|cancel>")
		return 2
	}
	subcommand, args := args[0], args[1:]
	flags := flag.NewFlagSet("task "+subcommand, flag.ContinueOnError)
	flags.SetOutput(stderr)
	common := addCommon(flags)
	switch subcommand {
	case "create":
		projectID := flags.String("project", "", "Workforce Project ID")
		modelCredentialID := flags.String("model-credential", "", "model credential ID")
		primaryModel := flags.String("model", "", "primary model from the credential catalog")
		runtimeReleaseID := flags.String("runtime-release", "", "execution image release ID")
		executionTimeout := flags.Int("execution-timeout-seconds", 0, "optional Agent execution limit in seconds; 0 means unlimited")
		instruction := flags.String("instruction", "", "task instruction")
		name := flags.String("name", "", "optional task name")
		checkCommand := flags.String("completion-check-command", "", "optional additional completion-check command")
		reviewEnabled := flags.Bool("review-enabled", true, "enable independent review for this task")
		deliveryPreference := flags.String("delivery-preference", "PROJECT_DEFAULT", "PROJECT_DEFAULT, ASK_AFTER_GATES, or SAVE_ONLY")
		idempotency := flags.String("idempotency-key", randomKey(), "idempotency key")
		var tags stringList
		var referenceAssets stringList
		flags.Var(&tags, "tag", "task tag (repeatable)")
		flags.Var(&referenceAssets, "reference-asset", "reference asset ID (repeatable)")
		if err := flags.Parse(args); err != nil || flags.NArg() != 0 {
			return 2
		}
		parsedProjectID, err := strconv.ParseInt(strings.TrimSpace(*projectID), 10, 64)
		if err != nil || parsedProjectID < 1 || strconv.FormatInt(parsedProjectID, 10) != strings.TrimSpace(*projectID) {
			fmt.Fprintln(stderr, "--project must be a positive decimal Workforce Project ID")
			return 2
		}
		parsedCredentialID, err := strconv.ParseInt(strings.TrimSpace(*modelCredentialID), 10, 64)
		if err != nil || parsedCredentialID < 1 || strconv.FormatInt(parsedCredentialID, 10) != strings.TrimSpace(*modelCredentialID) {
			fmt.Fprintln(stderr, "--model-credential must be a positive decimal model credential ID")
			return 2
		}
		model := strings.TrimSpace(*primaryModel)
		if model == "" || len([]byte(model)) > 256 || strings.IndexFunc(model, func(r rune) bool { return r < 32 || r == 127 }) >= 0 {
			fmt.Fprintln(stderr, "--model must be a non-empty model ID of at most 256 bytes")
			return 2
		}
		parsedRuntimeReleaseID, err := strconv.ParseInt(strings.TrimSpace(*runtimeReleaseID), 10, 64)
		if err != nil || parsedRuntimeReleaseID < 1 || strconv.FormatInt(parsedRuntimeReleaseID, 10) != strings.TrimSpace(*runtimeReleaseID) {
			fmt.Fprintln(stderr, "--runtime-release must be a positive decimal execution image release ID")
			return 2
		}
		if *executionTimeout < 0 || *executionTimeout > 2147483647 {
			fmt.Fprintln(stderr, "--execution-timeout-seconds must be 0 or a positive integer no greater than 2147483647")
			return 2
		}
		preference := strings.ToUpper(strings.TrimSpace(*deliveryPreference))
		if preference != "PROJECT_DEFAULT" && preference != "ASK_AFTER_GATES" && preference != "SAVE_ONLY" {
			fmt.Fprintln(stderr, "--delivery-preference must be PROJECT_DEFAULT, ASK_AFTER_GATES, or SAVE_ONLY")
			return 2
		}
		for index, assetID := range referenceAssets {
			parsed, parseErr := strconv.ParseInt(strings.TrimSpace(assetID), 10, 64)
			if parseErr != nil || parsed < 1 || strconv.FormatInt(parsed, 10) != strings.TrimSpace(assetID) {
				fmt.Fprintln(stderr, "--reference-asset values must be positive decimal asset IDs")
				return 2
			}
			referenceAssets[index] = strconv.FormatInt(parsed, 10)
		}
		body := map[string]any{
			"instruction":        strings.TrimSpace(*instruction),
			"reviewEnabled":      *reviewEnabled,
			"deliveryPreference": preference,
			"modelSelection": map[string]string{
				"credentialId": strconv.FormatInt(parsedCredentialID, 10),
				"model":        model,
			},
			"runtimeReleaseId": strconv.FormatInt(parsedRuntimeReleaseID, 10),
		}
		if *executionTimeout > 0 {
			body["executionTimeoutSeconds"] = *executionTimeout
		}
		if value := strings.TrimSpace(*name); value != "" {
			body["name"] = value
		}
		if len(tags) > 0 {
			body["tags"] = []string(tags)
		}
		if len(referenceAssets) > 0 {
			body["referenceAssetIds"] = []string(referenceAssets)
		}
		if command := strings.TrimSpace(*checkCommand); command != "" {
			body["additionalCompletionCheck"] = map[string]any{"mode": "COMMAND", "command": command}
		}
		headers := map[string]string{"Idempotency-Key": *idempotency}
		return apiRequest(client, common, http.MethodPost, "/api/workforce/projects/"+strconv.FormatInt(parsedProjectID, 10)+"/tasks", body, headers, stdout, stderr)
	case "list":
		limit := flags.Int("limit", 50, "maximum tasks")
		if err := flags.Parse(args); err != nil || flags.NArg() != 0 {
			return 2
		}
		return apiRequest(client, common, http.MethodGet, "/api/workforce/tasks?limit="+strconv.Itoa(*limit), nil, nil, stdout, stderr)
	case "get", "events", "cancel":
		if err := flags.Parse(args); err != nil || flags.NArg() != 1 {
			fmt.Fprintf(stderr, "usage: seven-workerctl task %s [flags] <task-id>\n", subcommand)
			return 2
		}
		taskID := url.PathEscape(flags.Arg(0))
		method, path := http.MethodGet, "/api/workforce/tasks/"+taskID
		if subcommand == "events" {
			path += "/events"
		}
		if subcommand == "cancel" {
			method, path = http.MethodPost, path+"/cancel"
		}
		return apiRequest(client, common, method, path, nil, nil, stdout, stderr)
	default:
		fmt.Fprintf(stderr, "unknown task command %q\n", subcommand)
		return 2
	}
}

func runArtifact(args []string, stdout io.Writer, stderr io.Writer, client *http.Client) int {
	if len(args) == 0 {
		fmt.Fprintln(stderr, "usage: seven-workerctl artifact <list|download>")
		return 2
	}
	subcommand, args := args[0], args[1:]
	flags := flag.NewFlagSet("artifact "+subcommand, flag.ContinueOnError)
	flags.SetOutput(stderr)
	common := addCommon(flags)
	switch subcommand {
	case "list":
		if err := flags.Parse(args); err != nil || flags.NArg() != 1 {
			return 2
		}
		return apiRequest(client, common, http.MethodGet, "/api/workforce/tasks/"+url.PathEscape(flags.Arg(0))+"/artifacts", nil, nil, stdout, stderr)
	case "download":
		output := flags.String("output", "", "output file path")
		if err := flags.Parse(args); err != nil || flags.NArg() != 2 || strings.TrimSpace(*output) == "" {
			fmt.Fprintln(stderr, "usage: seven-workerctl artifact download [flags] <task-id> <artifact-id> --output FILE")
			return 2
		}
		var body bytes.Buffer
		path := "/api/workforce/tasks/" + url.PathEscape(flags.Arg(0)) + "/artifacts/" + url.PathEscape(flags.Arg(1)) + "/download"
		if code := apiRequestRaw(client, common, http.MethodGet, path, nil, nil, &body, stderr); code != 0 {
			return code
		}
		absolute, err := filepath.Abs(*output)
		if err != nil {
			fmt.Fprintf(stderr, "resolve output path: %v\n", err)
			return 1
		}
		if err := os.WriteFile(absolute, body.Bytes(), 0o600); err != nil {
			fmt.Fprintf(stderr, "write artifact: %v\n", err)
			return 1
		}
		fmt.Fprintln(stdout, absolute)
		return 0
	default:
		fmt.Fprintf(stderr, "unknown artifact command %q\n", subcommand)
		return 2
	}
}

func apiRequest(client *http.Client, common *commonFlags, method, path string, body any, headers map[string]string, stdout, stderr io.Writer) int {
	return apiRequestWithMode(client, common, method, path, body, headers, stdout, stderr, true)
}

func apiRequestRaw(client *http.Client, common *commonFlags, method, path string, body any, headers map[string]string, stdout, stderr io.Writer) int {
	return apiRequestWithMode(client, common, method, path, body, headers, stdout, stderr, false)
}

func apiRequestWithMode(client *http.Client, common *commonFlags, method, path string, body any, headers map[string]string, stdout, stderr io.Writer, appendNewline bool) int {
	var encoded []byte
	if body != nil {
		encoded, _ = json.Marshal(body)
	}
	if headers == nil {
		headers = map[string]string{}
	}
	if token := common.accessToken(); token != "" {
		headers["Authorization"] = "Bearer " + token
	}
	return requestWithHeaders(client, common.timeout, common.serverURL, method, path, encoded, headers, stdout, stderr, appendNewline)
}

func request(client *http.Client, timeout time.Duration, serverURL, token, method, path string, body []byte, stdout, stderr io.Writer) int {
	headers := map[string]string{}
	if token != "" {
		headers["Authorization"] = "Bearer " + strings.TrimSpace(token)
	}
	return requestWithHeaders(client, timeout, serverURL, method, path, body, headers, stdout, stderr, true)
}

func requestWithHeaders(client *http.Client, timeout time.Duration, serverURL, method, path string, body []byte, headers map[string]string, stdout, stderr io.Writer, appendNewline bool) int {
	base, err := url.Parse(strings.TrimRight(strings.TrimSpace(serverURL), "/"))
	if err != nil || base.Scheme == "" || base.Host == "" {
		fmt.Fprintf(stderr, "invalid server URL %q\n", serverURL)
		return 2
	}
	base.Path = strings.TrimRight(base.Path, "/") + strings.Split(path, "?")[0]
	if index := strings.Index(path, "?"); index >= 0 {
		base.RawQuery = path[index+1:]
	}
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	request, err := http.NewRequestWithContext(ctx, method, base.String(), bytes.NewReader(body))
	if err != nil {
		fmt.Fprintf(stderr, "create request: %v\n", err)
		return 1
	}
	if body != nil {
		request.Header.Set("Content-Type", "application/json")
	}
	for key, value := range headers {
		request.Header.Set(key, value)
	}
	if client == nil {
		client = http.DefaultClient
	}
	response, err := client.Do(request)
	if err != nil {
		fmt.Fprintf(stderr, "request Control Plane: %v\n", err)
		return 1
	}
	defer response.Body.Close()
	responseBody, err := io.ReadAll(io.LimitReader(response.Body, maxResponseBytes))
	if err != nil {
		fmt.Fprintf(stderr, "read response: %v\n", err)
		return 1
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		fmt.Fprintf(stderr, "Control Plane returned HTTP %d: %s\n", response.StatusCode, strings.TrimSpace(string(responseBody)))
		return 1
	}
	_, _ = stdout.Write(responseBody)
	if appendNewline && len(responseBody) > 0 && responseBody[len(responseBody)-1] != '\n' {
		fmt.Fprintln(stdout)
	}
	return 0
}

func randomKey() string {
	var raw [8]byte
	_, _ = rand.Read(raw[:])
	return "ctl-" + hex.EncodeToString(raw[:])
}

func printUsage(output io.Writer) {
	fmt.Fprintln(output, "seven-workerctl <health|task|artifact|version> [flags]")
}
