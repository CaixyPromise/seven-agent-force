package governance

import (
	"encoding/csv"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"testing"
)

// dg0LegacyMigrationCutoff is the last already-reviewed migration version.
// Governance rules below apply only to later source migrations, so historical
// mixed-case physical tables remain available through table_registry.csv until
// their separately reviewed DG4 rename batch.
const dg0LegacyMigrationCutoff int64 = 20260730160000

var (
	dg0CreateTablePattern           = regexp.MustCompile(`(?im)\bCREATE\s+(?:(?:UNLOGGED|TEMP(?:ORARY)?)\s+)?TABLE\s+(?:IF\s+NOT\s+EXISTS\s+)?(?:(?:["\x60]?public["\x60]?)\.)?["\x60]?([A-Za-z_][A-Za-z0-9_]*)["\x60]?`)
	dg0AlterRenamePattern           = regexp.MustCompile(`(?im)\bALTER\s+TABLE\s+(?:IF\s+EXISTS\s+)?(?:(?:["\x60]?[A-Za-z_][A-Za-z0-9_]*["\x60]?)\.)?["\x60]?[A-Za-z_][A-Za-z0-9_]*["\x60]?\s+RENAME\s+TO\s+(?:(?:["\x60]?public["\x60]?)\.)?["\x60]?([A-Za-z_][A-Za-z0-9_]*)["\x60]?`)
	dg0RenameTablePattern           = regexp.MustCompile(`(?im)(?:\bRENAME\s+TABLE|,)\s*(?:(?:["\x60]?[A-Za-z_][A-Za-z0-9_]*["\x60]?)\.)?["\x60]?[A-Za-z_][A-Za-z0-9_]*["\x60]?\s+TO\s+(?:(?:["\x60]?public["\x60]?)\.)?["\x60]?([A-Za-z_][A-Za-z0-9_]*)["\x60]?`)
	dg0ForeignKeyDeclarationPattern = regexp.MustCompile(`(?im)\bFOREIGN\s+KEY\s*\(`)
	dg0ForeignKeyReferencePattern   = regexp.MustCompile(`(?im)\bREFERENCES\s+(?:(?:["\x60]?[A-Za-z_][A-Za-z0-9_]*["\x60]?)\.)?["\x60]?[A-Za-z_][A-Za-z0-9_]*["\x60]?\s*\(`)
	dg0ColumnRenamePattern          = regexp.MustCompile(`(?im)\b(?:(?:ALTER\s+TABLE\s+(?:IF\s+EXISTS\s+)?(?:(?:["\x60]?[A-Za-z_][A-Za-z0-9_]*["\x60]?)\.)?["\x60]?[A-Za-z_][A-Za-z0-9_]*["\x60]?\s+)?RENAME\s+COLUMN\b)`)
	dg0BlockCommentPattern          = regexp.MustCompile(`(?s)/\*.*?\*/`)
	dg0LineCommentPattern           = regexp.MustCompile(`(?m)--[^\r\n]*`)
	dg0ScriptSQLStatementPattern    = regexp.MustCompile(`(?is)\b(?:SELECT\s+.+?\bFROM\b|INSERT\s+(?:IGNORE\s+)?INTO\b|UPDATE\s+[A-Za-z_\x60"]|DELETE\s+FROM\b|CREATE\s+(?:(?:UNLOGGED|TEMP(?:ORARY)?)\s+)?TABLE\b|ALTER\s+TABLE\b|DROP\s+TABLE\b)`)
	// Tests are ignored only when they are assertion-only. A test that invokes a
	// database client in the same process command is still operational SQL and
	// must be declared in the manifest. Keeping the client name inside the same
	// call avoids classifying a test that merely mentions a MySQL container as a
	// database-writing script.
	dg0ScriptDatabaseExecutionPattern = regexp.MustCompile(`(?is)(?:(?:subprocess\.(?:run|check_output|Popen)|execFileSync|spawn(?:Sync)?)\s*\(\s*(?:\[[^\]]{0,800}?\b(?:mysql|psql|sqlite3|sqlcmd)\b|["'][^"']{0,800}?\b(?:mysql|psql|sqlite3|sqlcmd)\b)|os\.system\s*\(\s*["'][^"']{0,800}?\b(?:mysql|psql|sqlite3|sqlcmd)\b)`)

	// The first capture is an optional schema; the second is the physical table.
	// It intentionally covers only concrete relation positions, not columns or
	// arbitrary SQL fragments.
	dg0OperationalRelationPattern = regexp.MustCompile(`(?i)\b(?:DELETE\s+FROM|FROM|JOIN|INSERT\s+(?:IGNORE\s+)?INTO|UPDATE|ALTER\s+TABLE|CREATE\s+TABLE|DROP\s+TABLE)\s+(?:(["\x60]?[A-Za-z_][A-Za-z0-9_]*["\x60]?)\.)?["\x60]?([A-Za-z_][A-Za-z0-9_]*)["\x60]?`)
	dg0AuditScriptRelationPattern = regexp.MustCompile(`(?m)(?:(?:^|[\r\n])\s*FROM|(?:^|[\r\n]|["'\x60]\s*)(?:SELECT\b[^\r\n;]*\bFROM|INSERT\s+(?:IGNORE\s+)?INTO|UPDATE|DELETE\s+FROM|ALTER\s+TABLE|CREATE\s+(?:(?:UNLOGGED|TEMP(?:ORARY)?)\s+)?TABLE|DROP\s+TABLE))\s+(?:(["\x60]?[A-Za-z_][A-Za-z0-9_]*["\x60]?)\.)?["\x60]?([A-Za-z_][A-Za-z0-9_]*)["\x60]?`)
)

type dg0OperationalSQLManifestEntry struct {
	SourcePath    string
	SourceKind    string
	Dialect       string
	TableContract string
	Reason        string
}

type dg0OperationalSQLSource struct {
	SourcePath string
	SourceKind string
	Dialect    string
	SQL        []string
}

func TestDG0FutureMigrationsUseLowerSnakeTablesAndRejectForeignKeys(t *testing.T) {
	root := governanceRepositoryRoot(t)
	violations, err := dg0FutureMigrationViolations(root)
	if err != nil {
		t.Fatalf("scan DG0 future migrations: %v", err)
	}
	if len(violations) > 0 {
		t.Fatalf("future migrations violate the DG0 table/foreign-key contract:\n%s", strings.Join(violations, "\n"))
	}
}

func TestDG0LegacyMixedCaseMigrationTablesAreExplicitlyRegistered(t *testing.T) {
	root := governanceRepositoryRoot(t)
	registered, err := dg0RegisteredTables(root)
	if err != nil {
		t.Fatalf("load DG0 table registry: %v", err)
	}
	var violations []string
	for _, migration := range dg0MigrationFiles(t, root) {
		version, err := dg0MigrationVersion(migration)
		if err != nil {
			t.Fatalf("parse migration version %s: %v", migration, err)
		}
		if version > dg0LegacyMigrationCutoff {
			continue
		}
		content, err := os.ReadFile(migration)
		if err != nil {
			t.Fatalf("read migration %s: %v", migration, err)
		}
		for _, table := range dg0PhysicalTableTargets(string(content)) {
			if lowerSnakeTablePattern.MatchString(table) {
				continue
			}
			if _, ok := registered[table]; ok {
				continue
			}
			relative, _ := filepath.Rel(root, migration)
			violations = append(violations, fmt.Sprintf("%s legacy table %q is absent from migrations/governance/table_registry.csv", filepath.ToSlash(relative), table))
		}
	}
	if len(violations) > 0 {
		t.Fatalf("historical mixed-case tables must remain an explicit migration registry allowlist:\n%s", strings.Join(violations, "\n"))
	}
}

func TestDG0OperationalSQLManifestCoversCommandJobListenerAndScriptSources(t *testing.T) {
	root := governanceRepositoryRoot(t)
	manifest, err := dg0OperationalSQLManifest(root)
	if err != nil {
		t.Fatalf("load operational SQL manifest: %v", err)
	}
	registered, err := dg0RegisteredTables(root)
	if err != nil {
		t.Fatalf("load DG0 table registry: %v", err)
	}
	sources, err := dg0OperationalSQLSources(root)
	if err != nil {
		t.Fatalf("scan operational SQL sources: %v", err)
	}

	seen := make(map[string]struct{}, len(sources))
	violations := make([]string, 0)
	for _, source := range sources {
		entry, ok := manifest[source.SourcePath]
		if !ok {
			violations = append(violations, source.SourcePath+" is SQL-bearing but absent from migrations/governance/operational_sql_manifest.csv")
			continue
		}
		seen[source.SourcePath] = struct{}{}
		if entry.SourceKind != source.SourceKind {
			violations = append(violations, fmt.Sprintf("%s manifest kind=%q, want %q", source.SourcePath, entry.SourceKind, source.SourceKind))
		}
		if entry.Dialect != source.Dialect {
			violations = append(violations, fmt.Sprintf("%s manifest dialect=%q, source contract=%q", source.SourcePath, entry.Dialect, source.Dialect))
		}
		if entry.TableContract != "registry" && entry.TableContract != "attempt-private" {
			violations = append(violations, fmt.Sprintf("%s manifest table contract=%q, want registry or attempt-private", source.SourcePath, entry.TableContract))
		}
		references := dg0OperationalTableReferences(source.SQL)
		if source.SourceKind == "audit-script" || source.SourceKind == "script" {
			references = dg0AuditScriptTableReferences(source.SQL)
		}
		if entry.TableContract == "registry" {
			for _, table := range references {
				if _, ok := registered[table]; !ok {
					violations = append(violations, fmt.Sprintf("%s references unregistered physical table %q", source.SourcePath, table))
				}
			}
		} else if entry.Dialect != "sqlite-private" || entry.SourceKind != "command" {
			violations = append(violations, fmt.Sprintf("%s attempt-private contract requires command/sqlite-private, got %s/%s", source.SourcePath, entry.SourceKind, entry.Dialect))
		}
	}
	for sourcePath, entry := range manifest {
		if _, ok := seen[sourcePath]; !ok {
			violations = append(violations, fmt.Sprintf("manifest entry %s is stale or no longer SQL-bearing", sourcePath))
		}
		if (entry.SourceKind == "audit-script" || entry.SourceKind == "script") && entry.Dialect != "mysql-only" {
			violations = append(violations, fmt.Sprintf("legacy SQL script %s must remain mysql-only until its dialect is separately reviewed, got %q", sourcePath, entry.Dialect))
		}
	}
	if len(violations) > 0 {
		sort.Strings(violations)
		t.Fatalf("operational SQL bypasses dialect/table governance:\n%s", strings.Join(violations, "\n"))
	}
}

func governanceRepositoryRoot(t *testing.T) string {
	t.Helper()
	root, err := filepath.Abs(filepath.Join("..", "..", "..", ".."))
	if err != nil {
		t.Fatalf("resolve repository root: %v", err)
	}
	return root
}

func dg0FutureMigrationViolations(root string) ([]string, error) {
	registered, err := dg0FutureMigrationTargets(root)
	if err != nil {
		return nil, err
	}
	violations := make([]string, 0)
	for _, migration := range dg0MigrationFiles(nil, root) {
		version, err := dg0MigrationVersion(migration)
		if err != nil {
			return nil, fmt.Errorf("parse %s: %w", migration, err)
		}
		if version <= dg0LegacyMigrationCutoff {
			continue
		}
		content, err := os.ReadFile(migration)
		if err != nil {
			return nil, fmt.Errorf("read %s: %w", migration, err)
		}
		stripped := dg0StripSQLComments(string(content))
		relative, _ := filepath.Rel(root, migration)
		for _, table := range dg0PhysicalTableTargets(stripped) {
			if !lowerSnakeTablePattern.MatchString(table) {
				violations = append(violations, fmt.Sprintf("%s creates or renames a physical table to %q; new targets must be lower snake_case", filepath.ToSlash(relative), table))
			}
			if _, ok := registered[table]; !ok {
				violations = append(violations, fmt.Sprintf("%s creates or renames physical table %q without a registry entry", filepath.ToSlash(relative), table))
			}
		}
		if dg0NewForeignKeyDeclaration(stripped) {
			violations = append(violations, fmt.Sprintf("%s declares FOREIGN KEY or REFERENCES; relationship integrity must be owned by application logic", filepath.ToSlash(relative)))
		}
		if dg0ColumnRenamePattern.MatchString(stripped) {
			violations = append(violations, fmt.Sprintf("%s renames a column; DG0 preserves existing column names while table migration is planned", filepath.ToSlash(relative)))
		}
	}
	sort.Strings(violations)
	return violations, nil
}

// dg0FutureMigrationTargets combines renamed legacy targets with explicitly
// planned new physical tables. A future table may not appear only in a SQL
// migration: it must be registered in the same reviewed change before the
// migration can pass CI.
func dg0FutureMigrationTargets(root string) (map[string]struct{}, error) {
	registered, err := dg0RegisteredTables(root)
	if err != nil {
		return nil, fmt.Errorf("load table registry: %w", err)
	}
	file, err := os.Open(filepath.Join(root, "migrations", "governance", "future_table_registry.csv"))
	if err != nil {
		return nil, fmt.Errorf("open future table registry: %w", err)
	}
	defer file.Close()
	rows, err := csv.NewReader(file).ReadAll()
	if err != nil {
		return nil, fmt.Errorf("read future table registry: %w", err)
	}
	wantHeader := []string{"target_table", "domain_batch", "owner", "created_in", "contract"}
	if len(rows) == 0 || strings.Join(rows[0], ",") != strings.Join(wantHeader, ",") {
		return nil, fmt.Errorf("invalid future table registry header")
	}
	for _, row := range rows[1:] {
		if len(row) != len(wantHeader) {
			return nil, fmt.Errorf("invalid future table registry row %v", row)
		}
		table := strings.TrimSpace(row[0])
		if !lowerSnakeTablePattern.MatchString(table) {
			return nil, fmt.Errorf("future table %q is not lower snake_case", table)
		}
		if strings.TrimSpace(row[1]) == "" || strings.TrimSpace(row[2]) == "" || strings.TrimSpace(row[3]) == "" || strings.TrimSpace(row[4]) == "" {
			return nil, fmt.Errorf("future table %q lacks batch, owner, migration, or contract", table)
		}
		if _, exists := registered[table]; exists {
			return nil, fmt.Errorf("future table %q duplicates an existing registry target", table)
		}
		registered[table] = struct{}{}
	}
	return registered, nil
}

func dg0MigrationFiles(t *testing.T, root string) []string {
	if t != nil {
		t.Helper()
	}
	paths := make([]string, 0)
	for _, dialect := range []string{"mysql", "postgres"} {
		entries, err := os.ReadDir(filepath.Join(root, "migrations", dialect))
		if err != nil {
			if t != nil {
				t.Fatalf("read %s migrations: %v", dialect, err)
			}
			return nil
		}
		for _, entry := range entries {
			if entry.IsDir() || filepath.Ext(entry.Name()) != ".sql" {
				continue
			}
			paths = append(paths, filepath.Join(root, "migrations", dialect, entry.Name()))
		}
	}
	sort.Strings(paths)
	return paths
}

func dg0MigrationVersion(path string) (int64, error) {
	versionText, _, ok := strings.Cut(filepath.Base(path), "_")
	if !ok {
		return 0, fmt.Errorf("invalid migration filename")
	}
	return strconv.ParseInt(versionText, 10, 64)
}

func dg0PhysicalTableTargets(sqlText string) []string {
	result := make([]string, 0)
	for _, pattern := range []*regexp.Regexp{dg0CreateTablePattern, dg0AlterRenamePattern, dg0RenameTablePattern} {
		for _, match := range pattern.FindAllStringSubmatch(sqlText, -1) {
			if len(match) > 1 {
				result = append(result, match[1])
			}
		}
	}
	return result
}

func dg0StripSQLComments(sqlText string) string {
	withoutBlocks := dg0BlockCommentPattern.ReplaceAllString(sqlText, "")
	return dg0LineCommentPattern.ReplaceAllString(withoutBlocks, "")
}

func dg0NewForeignKeyDeclaration(sqlText string) bool {
	return dg0ForeignKeyDeclarationPattern.MatchString(sqlText) || dg0ForeignKeyReferencePattern.MatchString(sqlText)
}

func dg0RegisteredTables(root string) (map[string]struct{}, error) {
	file, err := os.Open(filepath.Join(root, "migrations", "governance", "table_registry.csv"))
	if err != nil {
		return nil, err
	}
	defer file.Close()
	rows, err := csv.NewReader(file).ReadAll()
	if err != nil {
		return nil, err
	}
	if len(rows) < 2 {
		return nil, fmt.Errorf("table registry is empty")
	}
	wantHeader := []string{"current_table", "target_table", "domain_batch", "rename", "verify", "release", "recovery"}
	if strings.Join(rows[0], ",") != strings.Join(wantHeader, ",") {
		return nil, fmt.Errorf("table registry header=%v", rows[0])
	}
	registered := make(map[string]struct{}, len(rows)*2)
	targetOwners := make(map[string]string, len(rows)-1)
	for _, row := range rows[1:] {
		if len(row) != len(wantHeader) {
			return nil, fmt.Errorf("invalid registry row %v", row)
		}
		if _, exists := registered[row[0]]; exists {
			return nil, fmt.Errorf("duplicate registry table %q", row[0])
		}
		if owner, exists := targetOwners[row[1]]; exists && owner != row[0] {
			return nil, fmt.Errorf("target table %q is claimed by both %q and %q", row[1], owner, row[0])
		}
		registered[row[0]] = struct{}{}
		registered[row[1]] = struct{}{}
		targetOwners[row[1]] = row[0]
	}
	return registered, nil
}

func dg0OperationalSQLManifest(root string) (map[string]dg0OperationalSQLManifestEntry, error) {
	file, err := os.Open(filepath.Join(root, "migrations", "governance", "operational_sql_manifest.csv"))
	if err != nil {
		return nil, err
	}
	defer file.Close()
	rows, err := csv.NewReader(file).ReadAll()
	if err != nil {
		return nil, err
	}
	wantHeader := []string{"source_path", "source_kind", "dialect", "table_contract", "reason"}
	if len(rows) < 1 || strings.Join(rows[0], ",") != strings.Join(wantHeader, ",") {
		return nil, fmt.Errorf("invalid operational SQL manifest header")
	}
	manifest := make(map[string]dg0OperationalSQLManifestEntry, len(rows)-1)
	for _, row := range rows[1:] {
		if len(row) != len(wantHeader) {
			return nil, fmt.Errorf("invalid operational SQL manifest row %v", row)
		}
		entry := dg0OperationalSQLManifestEntry{
			SourcePath:    filepath.ToSlash(row[0]),
			SourceKind:    row[1],
			Dialect:       row[2],
			TableContract: row[3],
			Reason:        row[4],
		}
		if entry.SourcePath == "" || filepath.IsAbs(entry.SourcePath) || strings.Contains(entry.SourcePath, "../") {
			return nil, fmt.Errorf("unsafe manifest source path %q", entry.SourcePath)
		}
		if entry.SourceKind != "command" && entry.SourceKind != "job" && entry.SourceKind != "listener" && entry.SourceKind != "audit-script" && entry.SourceKind != "script" {
			return nil, fmt.Errorf("unsupported manifest source kind %q", entry.SourceKind)
		}
		if entry.Dialect != "mysql-only" && entry.Dialect != "postgres-capable" && entry.Dialect != "sqlite-private" {
			return nil, fmt.Errorf("unsupported manifest dialect %q", entry.Dialect)
		}
		if (entry.TableContract != "registry" && entry.TableContract != "attempt-private") || strings.TrimSpace(entry.Reason) == "" {
			return nil, fmt.Errorf("manifest source %q lacks a supported table contract or reason", entry.SourcePath)
		}
		if _, exists := manifest[entry.SourcePath]; exists {
			return nil, fmt.Errorf("duplicate operational SQL manifest source %q", entry.SourcePath)
		}
		manifest[entry.SourcePath] = entry
	}
	return manifest, nil
}

func dg0OperationalSQLSources(root string) ([]dg0OperationalSQLSource, error) {
	sources := make([]dg0OperationalSQLSource, 0)
	for _, scanRoot := range []string{filepath.Join(root, "cmd"), filepath.Join(root, "internal")} {
		err := filepath.WalkDir(scanRoot, func(path string, entry os.DirEntry, walkErr error) error {
			if walkErr != nil {
				return walkErr
			}
			if entry.IsDir() || filepath.Ext(path) != ".go" || strings.HasSuffix(path, "_test.go") {
				return nil
			}
			relative, err := filepath.Rel(root, path)
			if err != nil {
				return err
			}
			kind, operational := dg0OperationalGoSourceKind(filepath.ToSlash(relative))
			if !operational {
				return nil
			}
			fileSet := token.NewFileSet()
			file, err := parser.ParseFile(fileSet, path, nil, parser.ParseComments)
			if err != nil {
				return err
			}
			sqlText := dg0RawSQLLiterals(file)
			if len(sqlText) == 0 {
				return nil
			}
			if problem := commandSQLDialectDeclarationProblem(file); problem != "" {
				return fmt.Errorf("%s: %s", filepath.ToSlash(relative), problem)
			}
			dialect, err := dg0SQLDialectDeclaration(file)
			if err != nil {
				return fmt.Errorf("%s: %w", filepath.ToSlash(relative), err)
			}
			sources = append(sources, dg0OperationalSQLSource{SourcePath: filepath.ToSlash(relative), SourceKind: kind, Dialect: dialect, SQL: sqlText})
			return nil
		})
		if err != nil {
			return nil, err
		}
	}

	for _, scriptRoot := range []string{filepath.Join(root, "scripts"), filepath.Join(root, "script")} {
		if _, err := os.Stat(scriptRoot); os.IsNotExist(err) {
			continue
		} else if err != nil {
			return nil, err
		}
		err := filepath.WalkDir(scriptRoot, func(path string, entry os.DirEntry, walkErr error) error {
			if walkErr != nil {
				return walkErr
			}
			if entry.IsDir() || !dg0ScriptExtension(path) {
				return nil
			}
			content, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			if dg0PureAssertionScript(entry.Name(), content) || !dg0ScriptSQLStatementPattern.Match(content) {
				return nil
			}
			relative, err := filepath.Rel(root, path)
			if err != nil {
				return err
			}
			relative = filepath.ToSlash(relative)
			sources = append(sources, dg0OperationalSQLSource{
				SourcePath: relative,
				SourceKind: dg0ScriptSourceKind(relative),
				Dialect:    "mysql-only",
				SQL:        []string{string(content)},
			})
			return nil
		})
		if err != nil {
			return nil, err
		}
	}
	sort.Slice(sources, func(left, right int) bool { return sources[left].SourcePath < sources[right].SourcePath })
	return sources, nil
}

func dg0OperationalGoSourceKind(relative string) (string, bool) {
	if strings.HasPrefix(relative, "cmd/") {
		return "command", true
	}
	parts := strings.Split(relative, "/")
	for _, part := range parts {
		switch part {
		case "job", "jobs":
			return "job", true
		case "listener", "listeners":
			return "listener", true
		}
	}
	return "", false
}

func dg0RawSQLLiterals(file *ast.File) []string {
	result := make([]string, 0)
	ast.Inspect(file, func(node ast.Node) bool {
		literal, ok := node.(*ast.BasicLit)
		if !ok || literal.Kind != token.STRING {
			return true
		}
		value, err := strconv.Unquote(literal.Value)
		if err == nil && rawSQLStatementPattern.MatchString(value) {
			result = append(result, value)
		}
		return true
	})
	return result
}

func dg0SQLDialectDeclaration(file *ast.File) (string, error) {
	for _, group := range file.Comments {
		for _, comment := range group.List {
			if comment.End() >= file.Package {
				continue
			}
			text := strings.TrimSpace(strings.TrimPrefix(comment.Text, "//"))
			if strings.HasPrefix(text, sqlGovernanceDirectivePrefix) {
				return strings.TrimSpace(strings.TrimPrefix(text, sqlGovernanceDirectivePrefix)), nil
			}
		}
	}
	return "", fmt.Errorf("missing file-level %s declaration", sqlGovernanceDirectivePrefix)
}

func dg0ScriptExtension(path string) bool {
	switch filepath.Ext(path) {
	case ".py", ".js", ".mjs", ".sh", ".sql":
		return true
	default:
		return false
	}
}

func dg0ScriptSourceKind(relative string) string {
	if strings.HasPrefix(relative, "scripts/audit/") {
		return "audit-script"
	}
	return "script"
}

func dg0PureAssertionScript(name string, content []byte) bool {
	if !strings.HasPrefix(name, "test_") && !strings.HasSuffix(name, "_test.py") && !strings.HasSuffix(name, "_test.js") && !strings.HasSuffix(name, "_test.mjs") {
		return false
	}
	lower := strings.ToLower(string(content))
	if !strings.Contains(lower, "assert") && !strings.Contains(lower, "unittest") && !strings.Contains(lower, "pytest") {
		return false
	}
	return !dg0ScriptDatabaseExecutionPattern.MatchString(string(content))
}

func dg0OperationalTableReferences(statements []string) []string {
	return dg0TableReferencesFromMatches(statements, dg0OperationalRelationPattern)
}

func dg0AuditScriptTableReferences(scripts []string) []string {
	return dg0TableReferencesFromMatches(scripts, dg0AuditScriptRelationPattern)
}

func dg0TableReferencesFromMatches(statements []string, pattern *regexp.Regexp) []string {
	ctes := make(map[string]struct{})
	result := make(map[string]struct{})
	for _, statement := range statements {
		for _, match := range sqlCTEPattern.FindAllStringSubmatch(statement, -1) {
			ctes[strings.ToLower(match[1])] = struct{}{}
		}
		for _, match := range pattern.FindAllStringSubmatch(statement, -1) {
			if len(match) < 3 {
				continue
			}
			schema := strings.Trim(match[1], "\"`")
			if dg0SystemSchema(schema) {
				continue
			}
			table := match[2]
			if _, cte := ctes[strings.ToLower(table)]; cte {
				continue
			}
			result[table] = struct{}{}
		}
	}
	tables := make([]string, 0, len(result))
	for table := range result {
		tables = append(tables, table)
	}
	sort.Strings(tables)
	return tables
}

func dg0SystemSchema(schema string) bool {
	switch strings.ToLower(schema) {
	case "information_schema", "performance_schema", "pg_catalog", "mysql":
		return true
	default:
		return false
	}
}

func TestDG0MigrationTableAndForeignKeyDetectors(t *testing.T) {
	source := `
-- CREATE TABLE ignoredLegacy (id bigint);
CREATE TABLE sys_order (id bigint);
ALTER TABLE sys_order RENAME TO sys_order_archive;
RENAME TABLE sys_order_archive TO sys_order_history, sys_order_history TO sys_order_final;
/* FOREIGN KEY (user_id) REFERENCES sys_user(id) */
`
	targets := dg0PhysicalTableTargets(dg0StripSQLComments(source))
	wantTargets := []string{"sys_order", "sys_order_archive", "sys_order_history", "sys_order_final"}
	if strings.Join(targets, ",") != strings.Join(wantTargets, ",") {
		t.Fatalf("migration targets=%v want=%v", targets, wantTargets)
	}
	if dg0NewForeignKeyDeclaration(dg0StripSQLComments(source)) {
		t.Fatal("comment-only FOREIGN KEY declaration was detected")
	}
	if !dg0NewForeignKeyDeclaration(`ALTER TABLE sys_order ADD CONSTRAINT fk_order_user FOREIGN KEY (user_id) REFERENCES sys_user(id)`) {
		t.Fatal("new FOREIGN KEY declaration was not detected")
	}
	if !dg0NewForeignKeyDeclaration(`CREATE TABLE sys_order (owner_id bigint REFERENCES sys_user(id))`) {
		t.Fatal("inline REFERENCES foreign key declaration was not detected")
	}
	if dg0NewForeignKeyDeclaration(`ALTER TABLE sys_order DROP FOREIGN KEY fk_order_user`) {
		t.Fatal("FOREIGN KEY removal must not be treated as a new declaration")
	}
	if dg0ColumnRenamePattern.MatchString(`ALTER TABLE sys_order RENAME TO sys_order_archive`) {
		t.Fatal("table rename was incorrectly treated as a column rename")
	}
	if !dg0ColumnRenamePattern.MatchString(`ALTER TABLE sys_order RENAME COLUMN owner_id TO purchaser_id`) {
		t.Fatal("ALTER TABLE RENAME COLUMN was not detected")
	}
	if !dg0ColumnRenamePattern.MatchString(`RENAME COLUMN owner_id TO purchaser_id`) {
		t.Fatal("standalone RENAME COLUMN was not detected")
	}
}

func TestDG0OperationalSourceAndTableDetectors(t *testing.T) {
	for relative, wantKind := range map[string]string{
		"cmd/tool/main.go":                             "command",
		"internal/app/file/job/maintenance.go":         "job",
		"internal/app/notification/listener/worker.go": "listener",
		"internal/app/system/config/infrastructure.go": "",
	} {
		kind, ok := dg0OperationalGoSourceKind(relative)
		if (wantKind != "") != ok || kind != wantKind {
			t.Fatalf("source kind %s = (%q,%t), want (%q,%t)", relative, kind, ok, wantKind, wantKind != "")
		}
	}
	tables := dg0OperationalTableReferences([]string{
		`SELECT * FROM sys_user u JOIN sys_user_role ur ON ur.userId = u.id`,
		`INSERT INTO seven.sysSsoSession (id) VALUES (1)`,
		`SELECT * FROM information_schema.tables`,
		`WITH visible_users AS (SELECT id FROM sys_user) SELECT * FROM visible_users`,
	})
	wantTables := []string{"sysSsoSession", "sys_user", "sys_user_role"}
	if strings.Join(tables, ",") != strings.Join(wantTables, ",") {
		t.Fatalf("operational table references=%v want=%v", tables, wantTables)
	}
	auditTables := dg0AuditScriptTableReferences([]string{`from __future__ import annotations
query = "DELETE FROM sys_user WHERE id = 1"
metadata = "SELECT 1 FROM information_schema.tables"`})
	if strings.Join(auditTables, ",") != "sys_user" {
		t.Fatalf("audit script table references=%v want=[sys_user]", auditTables)
	}
	nonLeadingSQL := []byte("from __future__ import annotations\nquery = \"UPDATE sys_user SET status = 0\"\n")
	if !dg0ScriptSQLStatementPattern.Match(nonLeadingSQL) {
		t.Fatal("non-leading script SQL was not detected")
	}
	if !dg0PureAssertionScript("test_fixture.py", []byte("def test_sql():\n  assert 'SELECT id FROM sys_user'\n")) {
		t.Fatal("pure assertion script was not excluded")
	}
	if dg0PureAssertionScript("test_fixture.py", []byte("import subprocess\nsubprocess.run(['mysql', '-e', 'SELECT 1'])\nassert True\n")) {
		t.Fatal("database-executing test script was incorrectly excluded")
	}
	if !dg0PureAssertionScript("test_fixture.py", []byte("import subprocess\nsubprocess.run(['docker', 'compose', 'config'])\nassert 'SELECT id FROM sys_user'\n")) {
		t.Fatal("non-database assertion test was incorrectly treated as operational SQL")
	}
}

func TestDG0FutureTableRegistryRequiresExplicitMetadata(t *testing.T) {
	root := t.TempDir()
	governanceDir := filepath.Join(root, "migrations", "governance")
	if err := os.MkdirAll(governanceDir, 0o750); err != nil {
		t.Fatalf("create temporary governance directory: %v", err)
	}
	if err := os.WriteFile(filepath.Join(governanceDir, "table_registry.csv"), []byte("current_table,target_table,domain_batch,rename,verify,release,recovery\nsys_legacy,sys_legacy,legacy,no_op,baseline_required,no_op,resume_forward\n"), 0o600); err != nil {
		t.Fatalf("write table registry: %v", err)
	}
	future := filepath.Join(governanceDir, "future_table_registry.csv")
	if err := os.WriteFile(future, []byte("target_table,domain_batch,owner,created_in,contract\nsys_new_table,example,internal/app/example,20260801100000,expand_verify_contract\n"), 0o600); err != nil {
		t.Fatalf("write future table registry: %v", err)
	}
	targets, err := dg0FutureMigrationTargets(root)
	if err != nil {
		t.Fatalf("load valid future table registry: %v", err)
	}
	if _, ok := targets["sys_new_table"]; !ok {
		t.Fatal("registered future table is absent from allowed migration targets")
	}
	if err := os.WriteFile(future, []byte("target_table,domain_batch,owner,created_in,contract\nsysNewTable,example,internal/app/example,20260801100000,expand_verify_contract\n"), 0o600); err != nil {
		t.Fatalf("rewrite invalid future table registry: %v", err)
	}
	if _, err := dg0FutureMigrationTargets(root); err == nil {
		t.Fatal("camelCase future table was accepted")
	}
}
