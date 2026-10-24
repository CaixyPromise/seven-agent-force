package governance

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"sort"
	"strings"
	"testing"

	"github.com/CaixyPromise/seven-agent/force/internal/app/workforce/shared/identitycontract"
)

var (
	workforceCreateTable = regexp.MustCompile(`(?i)^\s*CREATE\s+TABLE(?:\s+IF\s+NOT\s+EXISTS)?\s+([a-z][a-z0-9_]*)`)
	workforceAlterTable  = regexp.MustCompile(`(?i)^\s*ALTER\s+TABLE\s+([a-z][a-z0-9_]*)`)
	workforceTextColumn  = regexp.MustCompile(`(?i)^\s*(?:ADD\s+COLUMN\s+)?([a-z][a-z0-9_]*)\s+(?:TEXT\b|VARCHAR\s*\(|CHARACTER\s+VARYING\b)`)
)

func TestWorkforceInternalDatabaseIdentitiesUseBigint(t *testing.T) {
	root := governanceRepositoryRoot(t)
	workforceTextIdentityAllowlist := identitycontract.TextIdentityExceptions()
	paths, err := filepath.Glob(filepath.Join(root, "migrations", "postgres", "*workforce*.sql"))
	if err != nil {
		t.Fatalf("glob Workforce migrations: %v", err)
	}
	sort.Strings(paths)
	seenAllowlist := make(map[string]bool, len(workforceTextIdentityAllowlist))
	var violations []string
	for _, path := range paths {
		body, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read %s: %v", path, err)
		}
		table := ""
		for lineNumber, line := range strings.Split(string(body), "\n") {
			if match := workforceCreateTable.FindStringSubmatch(line); len(match) == 2 {
				table = match[1]
			} else if match := workforceAlterTable.FindStringSubmatch(line); len(match) == 2 {
				table = match[1]
			}
			match := workforceTextColumn.FindStringSubmatch(line)
			if len(match) != 2 || table == "" {
				continue
			}
			column := match[1]
			key := table + "." + column
			if _, allowed := workforceTextIdentityAllowlist[key]; allowed {
				seenAllowlist[key] = true
				continue
			}
			if column != "id" && column != "tenant_id" && !strings.HasSuffix(column, "_id") {
				continue
			}
			relative, _ := filepath.Rel(root, path)
			violations = append(violations, fmt.Sprintf("%s:%d keeps internal identity %s as TEXT", filepath.ToSlash(relative), lineNumber+1, key))
		}
	}
	for key := range workforceTextIdentityAllowlist {
		if !seenAllowlist[key] {
			violations = append(violations, "stale TEXT identity allowlist entry: "+key)
		}
	}
	if len(violations) != 0 {
		sort.Strings(violations)
		t.Fatalf("Workforce identity schema violates BIGINT policy:\n%s", strings.Join(violations, "\n"))
	}
}

func TestWorkforceMigrationsDoNotUsePseudoSystemTenant(t *testing.T) {
	root := governanceRepositoryRoot(t)
	paths, err := filepath.Glob(filepath.Join(root, "migrations", "postgres", "*workforce*.sql"))
	if err != nil {
		t.Fatalf("glob Workforce migrations: %v", err)
	}
	for _, path := range paths {
		body, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read %s: %v", path, err)
		}
		if regexp.MustCompile(`(?i)['"]_system['"]`).Match(body) {
			t.Errorf("%s contains pseudo tenant _system", path)
		}
	}
}

func TestWorkforceJSONAndWebPreserveInt64IdentityPrecision(t *testing.T) {
	root := governanceRepositoryRoot(t)
	var malformedTags []string
	var numericSlices []string
	var unquotedIdentities []string
	jsonInt64Field := regexp.MustCompile("(?m)\\bint64\\s+`json:\"([^\"]+)\"`")
	for _, serverRoot := range []string{filepath.Join(root, "internal", "app", "workforce"), filepath.Join(root, "cmd")} {
		err := filepath.WalkDir(serverRoot, func(path string, entry os.DirEntry, walkErr error) error {
			if walkErr != nil || entry.IsDir() || filepath.Ext(path) != ".go" {
				return walkErr
			}
			body, readErr := os.ReadFile(path)
			if readErr != nil {
				return readErr
			}
			relative, _ := filepath.Rel(root, path)
			if regexp.MustCompile(`\b(?:\[\])?int64\s+"json:\\"`).Match(body) {
				malformedTags = append(malformedTags, filepath.ToSlash(relative))
			}
			for _, match := range regexp.MustCompile(`\[\]int64\s+`+"`json:\"([^\"]+)\"").FindAllSubmatch(body, -1) {
				// A field explicitly excluded from JSON is not a numeric transport
				// surface; custom codecs expose their separately governed wire DTO.
				if string(match[1]) != "-" {
					numericSlices = append(numericSlices, filepath.ToSlash(relative))
				}
			}
			for _, match := range jsonInt64Field.FindAllStringSubmatch(string(body), -1) {
				parts := strings.Split(match[1], ",")
				name := parts[0]
				identity := name == "id" || strings.HasSuffix(name, "Id") || strings.HasSuffix(name, "ID")
				concurrencyIdentity := name == "version" || strings.HasSuffix(name, "Version") ||
					name == "generation" || strings.HasSuffix(name, "Generation") ||
					name == "ownerEpoch" || strings.HasSuffix(name, "OwnerEpoch") ||
					name == "sequence" || strings.HasSuffix(name, "Sequence") ||
					name == "expectedRevision" || name == "revisionNumber"
				if (identity || concurrencyIdentity) && !slices.Contains(parts[1:], "string") {
					unquotedIdentities = append(unquotedIdentities, filepath.ToSlash(relative)+":"+name)
				}
			}
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
	}
	if len(malformedTags) != 0 {
		t.Fatalf("int64 fields contain ineffective JSON tags: %s", strings.Join(malformedTags, ", "))
	}
	if len(numericSlices) != 0 {
		t.Fatalf("int64 identity slices bypass the lossless string transport type: %s", strings.Join(numericSlices, ", "))
	}
	if len(unquotedIdentities) != 0 {
		sort.Strings(unquotedIdentities)
		t.Fatalf("int64 entity identities cross JSON as numbers: %s", strings.Join(unquotedIdentities, ", "))
	}

	unsafeConversion := regexp.MustCompile(`\b(?:Number|parseInt|parseFloat)\s*\([^\n)]*(?:id|Id|ID)\b`)
	unsafeInt64Conversion := regexp.MustCompile(`\b(?:Number|parseInt|parseFloat)\s*\([^\n)]*(?:id|Id|ID|version|Version|generation|Generation)\b`)
	unsafeType := regexp.MustCompile(`\b(?:id|tenantId|userId|orgId|deptId|parentId|leaderUserId|createdBy|approvedBy|taskId|taskTurnId|attemptId|workerId|profileId|releaseId|revisionId|processorId|connectionId|checkpointId|outputId|artifactId|operationId|lastOperationId)\??:\s*number\b`)
	unsafeWorkforceConcurrencyType := regexp.MustCompile(`\b(?:metadataVersion|draftVersion|lifecycleGeneration|inventoryVersion|inventoryRefreshGeneration|profileVersion|cancelGeneration|generation|ownerEpoch|pendingOwnerEpoch)\??:\s*number\b`)
	webRoot := filepath.Join(root, "..", "seven-agent-force-web", "src")
	err := filepath.WalkDir(webRoot, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil || entry.IsDir() || (filepath.Ext(path) != ".ts" && filepath.Ext(path) != ".tsx") {
			return walkErr
		}
		relative, _ := filepath.Rel(webRoot, path)
		slashed := filepath.ToSlash(relative)
		body, readErr := os.ReadFile(path)
		if readErr != nil {
			return readErr
		}
		if unsafeConversion.Match(body) {
			t.Errorf("%s converts an entity identity through JavaScript Number", filepath.ToSlash(relative))
		}
		if unsafeInt64Conversion.Match(body) {
			t.Errorf("%s converts an int64 transport value through JavaScript Number", filepath.ToSlash(relative))
		}
		// Two purely client-local counters use `id` as a React/log row key. They
		// are not API or database identities and remain ordinary numbers.
		localUIIdentity := slashed == "app/system/config/components/ControlledJsonEditor.tsx" ||
			slashed == "app/system/docker/components/DockerLogConsole.tsx" ||
			slashed == "components/notification/notificationInboxContext.ts"
		if unsafeType.Match(body) && !localUIIdentity {
			t.Errorf("%s types an entity identity as JavaScript number", filepath.ToSlash(relative))
		}
		workforceSurface := strings.HasPrefix(slashed, "app/workforce/") ||
			strings.Contains(strings.ToLower(filepath.Base(path)), "workforce") ||
			slices.Contains([]string{"continuationController.ts", "deliveryController.ts", "resultProcessorController.ts", "workspaceInputController.ts"}, filepath.Base(path))
		if workforceSurface && unsafeWorkforceConcurrencyType.Match(body) {
			t.Errorf("%s types a Workforce int64 concurrency value as JavaScript number", filepath.ToSlash(relative))
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	typings, err := os.ReadFile(filepath.Join(webRoot, "api", "typings.d.ts"))
	if err != nil {
		t.Fatal(err)
	}
	if !regexp.MustCompile(`type\s+Int64\s*=\s*string`).Match(typings) {
		t.Fatal("API.Int64 must remain a decimal string alias")
	}
	controller, err := os.ReadFile(filepath.Join(root, "..", "seven-agent-force-web", "src", "api", "workforceController.ts"))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(controller, []byte("toApiIdParam")) || !bytes.Contains(controller, []byte("API.Int64")) {
		t.Fatal("Workforce Web must use the shared lossless Int64 transport helpers")
	}
	requestClient, err := os.ReadFile(filepath.Join(root, "..", "seven-agent-force-web", "src", "api", "request.ts"))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(requestClient, []byte("parseJsonPreserveLargeIntegers")) || !bytes.Contains(requestClient, []byte("transformResponse")) {
		t.Fatal("shared HTTP client must preserve bare large integer literals before JSON.parse")
	}
	losslessJSON, err := os.ReadFile(filepath.Join(root, "..", "seven-agent-force-web", "src", "lib", "lossless-json.mjs"))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(losslessJSON, []byte("preserveLargeIntegerLiterals")) || !bytes.Contains(losslessJSON, []byte("JSON.parse")) {
		t.Fatal("shared lossless JSON transport must protect large integer literals before parsing")
	}
}
