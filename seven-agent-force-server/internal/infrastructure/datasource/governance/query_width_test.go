package governance

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"
)

var (
	sqlReadPattern         = regexp.MustCompile(`(?i)\bSELECT\b`)
	rawSQLStatementPattern = regexp.MustCompile(`(?is)^\s*(?:SELECT\s+.+|INSERT\s+INTO\b|UPDATE\s+["` + "`" + `]?[A-Za-z_][A-Za-z0-9_]*|DELETE\s+FROM\b|WITH\s+[A-Za-z_][A-Za-z0-9_]*\s+AS\b|CREATE\s+(?:OR\s+REPLACE\s+)?(?:TABLE|INDEX|VIEW|SCHEMA)\b|ALTER\s+(?:TABLE|INDEX)\b|DROP\s+(?:TABLE|INDEX|VIEW|SCHEMA)\b)`)
	sqlTablePattern        = regexp.MustCompile(`(?i)\b(?:FROM|JOIN)\s+["` + "`" + `]?([A-Za-z_][A-Za-z0-9_]*)(?:["` + "`" + `]|\b)`)
	sqlCTEPattern          = regexp.MustCompile(`(?i)(?:\bWITH|,)\s*([A-Za-z_][A-Za-z0-9_]*)\s+AS\s*\(`)
	sqlViewPattern         = regexp.MustCompile(`(?is)\bCREATE\s+(?:OR\s+REPLACE\s+)?VIEW\s+["` + "`" + `]?[A-Za-z_][A-Za-z0-9_]*["` + "`" + `]?\s+AS\s+(.*?);`)
)

const sqlGovernanceDirectivePrefix = "sql-governance:"

// reviewedDynamicReadBuilders is intentionally function-scoped. Entries are
// allowed only when the dynamic fragment is an allowlisted predicate/order
// renderer and cannot add FROM/JOIN sources. New dynamic SQL builders fail the
// governance test until reviewed here.
var reviewedDynamicReadBuilders = map[string]string{
	"internal/app/workforce/schemaalignment/alignment.go:buildIdentityMap":             "single sanitized catalog-selected table and column with bound values",
	"internal/app/workforce/schemaalignment/alignment.go:alignIdentityColumn":          "single sanitized catalog-selected table and column with bound values",
	"internal/app/workforce/schemaalignment/alignment.go:alignTenants":                 "single sanitized catalog-selected tenant table with bound values",
	"internal/app/workforce/schemaalignment/alignment.go:alignPrincipals":              "single sanitized allowlisted actor table and column with bound values",
	"internal/app/workforce/schemaalignment/alignment.go:Verify":                       "single catalog-selected table and column sanitized with pgx.Identifier; no dynamic FROM/JOIN composition",
	"internal/app/workforce/schemaalignment/project_first.go:ensureProjectHookHardCut": "single table selected from a fixed retired Hook product allowlist and sanitized with pgx.Identifier",
	"internal/app/authorization/infrastructure/repository.go:PageRoles":                "single-table base plus allowlisted predicates",
	"internal/app/authorization/infrastructure/repository.go:ListMenus":                "single-table base plus fixed enabled predicate",
	"internal/app/authorization/infrastructure/repository.go:ListPermissions":          "single-table base plus allowlisted predicates",
	"internal/app/authorization/infrastructure/repository.go:PagePermissions":          "single-table base plus allowlisted predicates",
	"internal/app/authorization/infrastructure/repository.go:TemporaryPermissionStats": "single-table base plus one fixed dialect-specific 24-hour deadline expression",

	"internal/app/external_login/infrastructure/repository.go:ListProviders":                     "single-table base plus bound filters",
	"internal/app/external_login/infrastructure/repository.go:ListIdentities":                    "single-table base plus bound filters",
	"internal/app/external_login/infrastructure/repository.go:ListTokens":                        "single-table base plus bound filters",
	"internal/app/external_login/infrastructure/enterprise_repository.go:findEnterpriseIdentity": "single-table base plus one of three fixed bound predicates",

	"internal/app/file/infrastructure/repository.go:QueryFiles":                          "two-table reviewed base plus bound filters",
	"internal/app/file/infrastructure/repository.go:queryReferences":                     "single-table base plus fixed scope predicate",
	"internal/app/file/infrastructure/repository.go:QueryProcessTasks":                   "single-table base plus bound filters",
	"internal/app/file/infrastructure/repository.go:getFile":                             "single-table base plus optional fixed FOR UPDATE clause",
	"internal/app/file/infrastructure/repository.go:getChunkUpload":                      "single-table base plus optional fixed FOR UPDATE clause",
	"internal/app/hub_control/infrastructure/repository.go:Page":                         "single-table base plus bound filters",
	"internal/app/hub_control/infrastructure/repository.go:find":                         "single-table base plus optional fixed FOR UPDATE clause",
	"internal/app/workforce/executionaccess/infrastructure/store.go:GetOperation":        "single-table base plus optional fixed FOR UPDATE clause",
	"internal/app/workforce/executionaccess/infrastructure/store.go:ActiveGrants":        "single-table base plus optional fixed FOR UPDATE clause",
	"internal/app/workforce/executionaccess/infrastructure/store.go:GetGrant":            "single-table base plus optional fixed FOR UPDATE clause",
	"internal/app/workforce/executionaccess/infrastructure/store.go:AttemptForOperation": "single-table base plus optional fixed FOR UPDATE clause",
	"internal/app/workforce/executionaccess/infrastructure/store.go:GetAttempt":          "single-table base plus optional fixed FOR UPDATE clause",
	"internal/app/workforce/task/infrastructure/store.go:ListPage":                       "tenant-scoped task page with fixed view predicates, bound search/date/keyset values, and a hard-capped limit",
	"internal/app/workforce/taskanalytics/infrastructure/store.go:Tasks":                 "tenant-scoped status predicates selected from a fixed internal list",
	"internal/app/workforce/taskanalytics/infrastructure/store.go:taskTrend":             "optional fixed project predicate with bound tenant, project, and UTC window values",
	"internal/app/workforce/taskanalytics/infrastructure/store.go:durationTrend":         "optional fixed project predicate with bound tenant, project, and UTC window values",

	"internal/app/notification/infrastructure/delivery_diagnostic_repository.go:deliveryDiagnosticScopePredicate": "fixed EXISTS source plus bound scope values",
	"internal/app/notification/infrastructure/repository.go:FindHTTPDeliverySnapshotByDeliveryID":                 "single-table reviewed select base plus bound identity",
	"internal/app/notification/infrastructure/repository.go:logicalNotificationSelectBase":                        "single-table base with dialect JSON expression",
	"internal/app/notification/infrastructure/repository.go:materializationTaskSelectBase":                        "single-table base with dialect JSON expression",
	"internal/app/notification/infrastructure/repository.go:channelSelectBase":                                    "single-table base with dialect JSON expressions",
	"internal/app/notification/infrastructure/repository.go:externalTargetSelectBase":                             "single-table base with dialect JSON expression",
	"internal/app/notification/infrastructure/repository.go:templateSelectBase":                                   "single-table base with dialect JSON expressions",
	"internal/app/notification/infrastructure/repository.go:sceneBindingSelectBase":                               "single-table base with dialect JSON expression",
	"internal/app/notification/infrastructure/repository.go:deliverySelectBase":                                   "single-table base with dialect JSON expressions",
	"internal/app/notification/infrastructure/repository.go:selectPage":                                           "reviewed base supplied by same-package constructors; only predicates/order/paging appended",
	"internal/app/notification/infrastructure/repository.go:selectInboxPage":                                      "single-table inbox base plus fixed bounded paging",
	"internal/app/notification/infrastructure/template_revision_repository.go:templateRevisionSelectBase":         "single-table base with dialect JSON expressions",

	"internal/app/platform/infrastructure/repository.go:ListPlatforms":                  "single-table base plus bound filters",
	"internal/app/platform/infrastructure/repository.go:listExternalProviderCodes":      "single-table provider lookup; dynamic fragment is one fixed availability predicate, with parameterized IN capped at 100 and bounded LIMIT",
	"internal/app/platform/infrastructure/repository.go:findManagedDefaultPlatform":     "single-table base plus optional fixed FOR UPDATE clause",
	"internal/app/platform/infrastructure/repository.go:listManagedLoginMethods":        "single-table bounded managed list plus optional fixed FOR UPDATE clause",
	"internal/app/platform/infrastructure/repository.go:listManagedSourceRules":         "single-table bounded managed list plus optional fixed FOR UPDATE clause",
	"internal/app/sso/infrastructure/repository.go:ListClients":                         "single-table base plus bound filters",
	"internal/app/sso/infrastructure/repository.go:findClientDetail":                    "one client table with two correlated aggregate subqueries plus optional fixed FOR UPDATE clause",
	"internal/app/sso/infrastructure/repository.go:listActiveSessionsPage":              "single-table base plus one predicate selected only by fixed scope wrappers; all values, cutoff, keyset, and hard-capped limit are bound",
	"internal/app/system/admin/infrastructure/repository.go:QueryOperationLogs":         "single-table base plus bound filters",
	"internal/app/system/config/infrastructure/repository.go:QueryGroups":               "single-table base plus bound filters",
	"internal/app/system/config/infrastructure/repository.go:QueryConfigs":              "single-table base plus bound filters",
	"internal/app/system/config/infrastructure/repository.go:ListAuditLogs":             "single-table base plus bound filters",
	"internal/app/system/config/infrastructure/repository.go:CountGroupByCode":          "single-table count plus optional bound id exclusion",
	"internal/app/system/config/infrastructure/repository.go:FindConfigByGroupAndKey":   "two-table config/group read plus fixed enabled and LIMIT clauses",
	"internal/app/system/config/infrastructure/repository.go:ListConfigsByGroupAndKeys": "two-table config/group read with bounded parameterized key predicates",
	"internal/app/system/config/infrastructure/repository.go:FindConfigsByRawKey":       "two-table config/group read plus fixed enabled and order clauses",
	"internal/app/system/config/infrastructure/repository.go:CountConfigByGroupAndKey":  "single-table count plus optional bound id exclusion",
	"internal/app/system/config/infrastructure/repository.go:ListHistoryByConfigID":     "single-table history read plus optional bounded LIMIT",
	"internal/app/system/dict/infrastructure/repository.go:QueryTypes":                  "single-table base plus bound filters",
	"internal/app/system/dict/infrastructure/repository.go:QueryItems":                  "single-table base plus bound filters",
	"internal/app/system/dict/infrastructure/repository.go:CountTypeByCode":             "single-table count plus optional bound id exclusion",
	"internal/app/system/dict/infrastructure/repository.go:CountItemByValue":            "single-table count plus optional bound id exclusion",

	"internal/app/system/user/infrastructure/user_repository.go:QueryAdminUsers":                    "single-table final read after bounded dimension set resolution",
	"internal/app/system/user/infrastructure/user_repository.go:FindAdminUserByID":                  "single-table bound identity read; dynamic fragment is fixed dialect-only gender projection",
	"internal/app/system/user/infrastructure/user_repository.go:activeUserIDsByRoleQuery":           "single relationship-table lookup plus optional fixed FOR UPDATE clause",
	"internal/app/system/user/infrastructure/user_repository.go:ListOrgs":                           "single-table base plus bound filters",
	"internal/app/system/user/infrastructure/user_repository.go:ListDepts":                          "single-table base plus bound filters",
	"internal/app/system/user/infrastructure/user_repository.go:QueryPosts":                         "single-table base plus bound filters",
	"internal/app/system/user/infrastructure/user_repository.go:existsInCondition":                  "one allowlisted relationship table selected only by internal callers",
	"internal/app/system/user/infrastructure/user_selector_repository.go:ListUserOptions":           "single user table plus one reviewed scope EXISTS",
	"internal/app/system/user/infrastructure/user_selector_repository.go:FindVisibleUserOptionByID": "single user table plus one reviewed scope EXISTS",

	"internal/infrastructure/datasource/bootstrap/mysql_inspector.go:mysqlCurrentVersion":    "fixed metadata table with allowlisted migration table identifier",
	"internal/infrastructure/docker/compose_project_repository.go:ProjectNameExists":         "single-table existence read plus optional bound id exclusion",
	"internal/infrastructure/docker/repository.go:CodeExists":                                "single-table existence read plus optional bound id exclusion",
	"internal/infrastructure/docker/operation_repository.go:ListOperations":                  "single-table base plus bound filters",
	"internal/infrastructure/docker/operation_repository.go:LatestOperation":                 "single-table base plus fixed status predicates",
	"internal/infrastructure/docker/operation_repository.go:quarantineOperationEventOrphans": "single-table CASE update; no dynamic relation source",
	"internal/infrastructure/messaging/outbox/store.go:findReady":                            "single-table base selected from fixed outbox registry",
	"internal/infrastructure/messaging/outbox/store.go:listReady":                            "single-table base selected from fixed outbox registry",
	"internal/infrastructure/messaging/outbox/store.go:listReadyPayloadBounded":              "single-table fixed outbox projection; only bound type predicates and a fixed SQL-side payload-size CASE, with no dynamic relation source",
	"internal/app/system/cache_governance/infrastructure/outbox.go:refreshOperationQuery":    "single sys_outbox_event relation; fixed allowlisted MySQL/PostgreSQL column quoting, bound owner/scope/type/aggregate predicates, and fixed update-time/id ordering",
}

// reviewedWideReadFunctions contains the deliberately wide, tenant-scoped
// analytics reads. They collapse authoritative facts into one browser-facing
// aggregate response and are bounded by a fixed range, project identity, or
// current non-terminal Task set. Keep exceptions function-scoped so unrelated
// joins still fail this governance gate.
var reviewedWideReadFunctions = map[string]string{
	"internal/app/workforce/taskanalytics/infrastructure/store.go:Workbench":      "one tenant, fixed 30-day maximum, bounded four-task and three-attention result, and fixed aggregate query count",
	"internal/app/workforce/taskanalytics/infrastructure/store.go:ProjectList":    "fixed 90-day maximum, active Project scope, and ten-row health result",
	"internal/app/workforce/taskanalytics/infrastructure/store.go:Project":        "one tenant/project identity and fixed 90-day maximum",
	"internal/app/workforce/taskanalytics/infrastructure/store.go:Tasks":          "one tenant and fixed 30-day maximum",
	"internal/app/workforce/taskanalytics/infrastructure/store.go:waitingReasons": "one tenant/project and current non-terminal Tasks only",
	"internal/app/workforce/taskanalytics/infrastructure/store.go:projectStages":  "one tenant/project and fixed 90-day maximum",
	"internal/app/workforce/taskanalytics/infrastructure/store.go:taskActivity":   "one tenant and fixed 30-day maximum",
	"internal/app/workforce/taskanalytics/infrastructure/store.go:waitingAges":    "one tenant and current non-terminal Tasks only",
}

func TestDG2LogicalReadLiteralsUseAtMostThreePhysicalTableInstances(t *testing.T) {
	root, err := filepath.Abs(filepath.Join("..", "..", "..", ".."))
	if err != nil {
		t.Fatalf("resolve repository root: %v", err)
	}
	var violations []string
	for _, scanRoot := range productionGoScanRoots(root) {
		err := filepath.WalkDir(scanRoot, func(path string, entry os.DirEntry, walkErr error) error {
			if walkErr != nil {
				return walkErr
			}
			if entry.IsDir() {
				return nil
			}
			if filepath.Ext(path) != ".go" || strings.HasSuffix(path, "_test.go") {
				return nil
			}
			fileSet := token.NewFileSet()
			file, err := parser.ParseFile(fileSet, path, nil, 0)
			if err != nil {
				return err
			}
			constants := collectStringConstants(file)
			relative, _ := filepath.Rel(root, path)
			for _, declaration := range file.Decls {
				function, ok := declaration.(*ast.FuncDecl)
				if !ok || function.Body == nil {
					continue
				}
				functionKey := filepath.ToSlash(relative) + ":" + function.Name.Name
				ast.Inspect(function.Body, func(node ast.Node) bool {
					expression, ok := node.(ast.Expr)
					if !ok {
						return true
					}
					value, constant := evalStringExpression(expression, constants, map[string]bool{})
					if constant && sqlReadPattern.MatchString(value) {
						instances := physicalTableInstances(value)
						if len(instances) > 3 {
							if _, reviewed := reviewedWideReadFunctions[functionKey]; reviewed {
								return true
							}
							position := fileSet.Position(expression.Pos())
							violations = append(violations, fmt.Sprintf("%s:%d tables=%v", relative, position.Line, instances))
						}
						return true
					}
					if isDynamicReadComposition(expression, constants) {
						if _, reviewed := reviewedDynamicReadBuilders[functionKey]; !reviewed {
							position := fileSet.Position(expression.Pos())
							violations = append(violations, fmt.Sprintf("%s:%d dynamic SQL builder %s is not reviewed", relative, position.Line, functionKey))
						}
					}
					return true
				})
				if functionUsesMutableReadBuilder(function.Body, constants) {
					if _, reviewed := reviewedDynamicReadBuilders[functionKey]; !reviewed {
						position := fileSet.Position(function.Pos())
						violations = append(violations, fmt.Sprintf("%s:%d mutable SQL builder %s is not reviewed", relative, position.Line, functionKey))
					}
				}
			}
			return nil
		})
		if err != nil {
			t.Fatalf("scan SQL literals: %v", err)
		}
	}
	for _, scanRoot := range []string{
		filepath.Join(root, "migrations", "mysql"),
		filepath.Join(root, "migrations", "postgres"),
		filepath.Join(root, "migrations", "postgres-baseline"),
	} {
		err := filepath.WalkDir(scanRoot, func(path string, entry os.DirEntry, walkErr error) error {
			if walkErr != nil {
				return walkErr
			}
			if entry.IsDir() || filepath.Ext(path) != ".sql" {
				return nil
			}
			content, readErr := os.ReadFile(path)
			if readErr != nil {
				return readErr
			}
			relative, _ := filepath.Rel(root, path)
			for _, instances := range viewReadWidthViolations(string(content)) {
				violations = append(violations, fmt.Sprintf("%s view tables=%v", relative, instances))
			}
			return nil
		})
		if err != nil {
			t.Fatalf("scan migration views: %v", err)
		}
	}
	if len(violations) > 0 {
		t.Fatalf("logical read SQL exceeds three physical table instances:\n%s", strings.Join(violations, "\n"))
	}
}

func TestDG1PostgresSQLRejectsGenericStringRewrites(t *testing.T) {
	root, err := filepath.Abs(filepath.Join("..", "..", "..", ".."))
	if err != nil {
		t.Fatalf("resolve repository root: %v", err)
	}
	var violations []string
	for _, scanRoot := range productionGoScanRoots(root) {
		err := filepath.WalkDir(scanRoot, func(path string, entry os.DirEntry, walkErr error) error {
			if walkErr != nil {
				return walkErr
			}
			if entry.IsDir() || filepath.Ext(path) != ".go" || strings.HasSuffix(path, "_test.go") {
				return nil
			}
			fileSet := token.NewFileSet()
			file, err := parser.ParseFile(fileSet, path, nil, 0)
			if err != nil {
				return err
			}
			constants := collectStringConstants(file)
			relative, _ := filepath.Rel(root, path)
			for _, declaration := range file.Decls {
				function, ok := declaration.(*ast.FuncDecl)
				if !ok || function.Body == nil || !functionUsesGenericSQLStringRewrite(function.Body, constants) {
					continue
				}
				position := fileSet.Position(function.Pos())
				violations = append(violations, fmt.Sprintf("%s:%d generic SQL string rewrite in %s", relative, position.Line, function.Name.Name))
			}
			return nil
		})
		if err != nil {
			t.Fatalf("scan generic SQL rewrites: %v", err)
		}
	}
	if len(violations) > 0 {
		t.Fatalf("generic SQL string rewriting is forbidden; use static dialect SQL or an explicit allowlisted renderer:\n%s", strings.Join(violations, "\n"))
	}
}

func TestDG1CommandRawSQLDeclaresDialectContract(t *testing.T) {
	root, err := filepath.Abs(filepath.Join("..", "..", "..", ".."))
	if err != nil {
		t.Fatalf("resolve repository root: %v", err)
	}
	violations, err := commandRawSQLDialectViolations(filepath.Join(root, "cmd"), root)
	if err != nil {
		t.Fatalf("scan command raw SQL declarations: %v", err)
	}
	if len(violations) > 0 {
		t.Fatalf("command raw SQL must declare exactly one dialect contract (%s postgres-capable, mysql-only, or sqlite-private):\n%s", sqlGovernanceDirectivePrefix, strings.Join(violations, "\n"))
	}
}

func productionGoScanRoots(root string) []string {
	return []string{
		filepath.Join(root, "cmd"),
		filepath.Join(root, "internal", "app"),
		filepath.Join(root, "internal", "infrastructure"),
	}
}

func commandRawSQLDialectViolations(commandRoot, root string) ([]string, error) {
	violations := make([]string, 0)
	err := filepath.WalkDir(commandRoot, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() || filepath.Ext(path) != ".go" || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		fileSet := token.NewFileSet()
		file, err := parser.ParseFile(fileSet, path, nil, parser.ParseComments)
		if err != nil {
			return err
		}
		if !containsRawSQLStatement(file) {
			return nil
		}
		if problem := commandSQLDialectDeclarationProblem(file); problem != "" {
			relative, _ := filepath.Rel(root, path)
			violations = append(violations, filepath.ToSlash(relative)+": "+problem)
		}
		return nil
	})
	return violations, err
}

func containsRawSQLStatement(file *ast.File) bool {
	found := false
	ast.Inspect(file, func(node ast.Node) bool {
		if found {
			return false
		}
		literal, ok := node.(*ast.BasicLit)
		if !ok || literal.Kind != token.STRING {
			return true
		}
		value, err := strconv.Unquote(literal.Value)
		if err == nil && rawSQLStatementPattern.MatchString(value) {
			found = true
			return false
		}
		return true
	})
	return found
}

func commandSQLDialectDeclarationProblem(file *ast.File) string {
	declarations := make([]string, 0, 1)
	for _, group := range file.Comments {
		for _, comment := range group.List {
			if comment.End() >= file.Package {
				continue
			}
			text := strings.TrimSpace(strings.TrimPrefix(comment.Text, "//"))
			if !strings.HasPrefix(text, sqlGovernanceDirectivePrefix) {
				continue
			}
			declarations = append(declarations, strings.TrimSpace(strings.TrimPrefix(text, sqlGovernanceDirectivePrefix)))
		}
	}
	if len(declarations) == 0 {
		return "missing file-level " + sqlGovernanceDirectivePrefix + " declaration"
	}
	if len(declarations) != 1 {
		return "must declare exactly one file-level " + sqlGovernanceDirectivePrefix + " contract"
	}
	switch declarations[0] {
	case "postgres-capable", "mysql-only", "sqlite-private":
		return ""
	default:
		return fmt.Sprintf("unsupported %s contract %q", sqlGovernanceDirectivePrefix, declarations[0])
	}
}

func collectStringConstants(file *ast.File) map[string]ast.Expr {
	result := make(map[string]ast.Expr)
	for _, declaration := range file.Decls {
		gen, ok := declaration.(*ast.GenDecl)
		if !ok || gen.Tok != token.CONST {
			continue
		}
		for _, spec := range gen.Specs {
			values, ok := spec.(*ast.ValueSpec)
			if !ok {
				continue
			}
			for index, name := range values.Names {
				if index < len(values.Values) {
					result[name.Name] = values.Values[index]
				}
			}
		}
	}
	return result
}

func evalStringExpression(expression ast.Expr, constants map[string]ast.Expr, resolving map[string]bool) (string, bool) {
	switch value := expression.(type) {
	case *ast.BasicLit:
		if value.Kind != token.STRING {
			return "", false
		}
		text, err := strconv.Unquote(value.Value)
		return text, err == nil
	case *ast.ParenExpr:
		return evalStringExpression(value.X, constants, resolving)
	case *ast.BinaryExpr:
		if value.Op != token.ADD {
			return "", false
		}
		left, leftOK := evalStringExpression(value.X, constants, resolving)
		right, rightOK := evalStringExpression(value.Y, constants, resolving)
		if !leftOK || !rightOK {
			return "", false
		}
		return left + right, true
	case *ast.Ident:
		if resolving[value.Name] {
			return "", false
		}
		constant, ok := constants[value.Name]
		if !ok {
			return "", false
		}
		resolving[value.Name] = true
		result, valid := evalStringExpression(constant, constants, resolving)
		delete(resolving, value.Name)
		return result, valid
	default:
		return "", false
	}
}

func isDynamicReadComposition(expression ast.Expr, constants map[string]ast.Expr) bool {
	switch value := expression.(type) {
	case *ast.BinaryExpr:
		if value.Op != token.ADD {
			return false
		}
		if _, constant := evalStringExpression(value, constants, map[string]bool{}); constant {
			return false
		}
		return expressionContainsReadLiteral(value.X, constants) || expressionContainsReadLiteral(value.Y, constants)
	case *ast.CallExpr:
		selector, ok := value.Fun.(*ast.SelectorExpr)
		if !ok || selector.Sel.Name != "Sprintf" {
			return false
		}
		for _, argument := range value.Args {
			if expressionContainsReadLiteral(argument, constants) {
				return true
			}
		}
	}
	return false
}

func functionUsesMutableReadBuilder(body *ast.BlockStmt, constants map[string]ast.Expr) bool {
	if body == nil {
		return false
	}
	expressions := make(map[string]ast.Expr, len(constants))
	for name, expression := range constants {
		expressions[name] = expression
	}
	readVariables := make(map[string]bool)
	readBuilders := make(map[string]bool)
	mutableRead := false
	ast.Inspect(body, func(node ast.Node) bool {
		switch value := node.(type) {
		case *ast.AssignStmt:
			for index, left := range value.Lhs {
				identifier, ok := left.(*ast.Ident)
				if !ok {
					continue
				}
				if value.Tok == token.ADD_ASSIGN {
					if readVariables[identifier.Name] {
						mutableRead = true
					}
					delete(expressions, identifier.Name)
					continue
				}
				if index >= len(value.Rhs) {
					continue
				}
				expressions[identifier.Name] = value.Rhs[index]
				if expressionContainsReadLiteral(value.Rhs[index], expressions) {
					readVariables[identifier.Name] = true
				}
			}
		case *ast.CallExpr:
			selector, ok := value.Fun.(*ast.SelectorExpr)
			if !ok || selector.Sel.Name != "WriteString" {
				break
			}
			receiver, ok := selector.X.(*ast.Ident)
			if !ok {
				break
			}
			if readBuilders[receiver.Name] {
				mutableRead = true
			}
			for _, argument := range value.Args {
				if expressionContainsReadLiteral(argument, expressions) {
					readBuilders[receiver.Name] = true
				}
			}
		}
		return true
	})
	return mutableRead
}

func functionUsesGenericSQLStringRewrite(body *ast.BlockStmt, constants map[string]ast.Expr) bool {
	if body == nil {
		return false
	}
	readVariables := make(map[string]bool)
	ast.Inspect(body, func(node ast.Node) bool {
		switch value := node.(type) {
		case *ast.AssignStmt:
			for index, left := range value.Lhs {
				identifier, ok := left.(*ast.Ident)
				if !ok || index >= len(value.Rhs) {
					continue
				}
				if expressionContainsReadLiteral(value.Rhs[index], constants) {
					readVariables[identifier.Name] = true
				}
			}
		case *ast.DeclStmt:
			declaration, ok := value.Decl.(*ast.GenDecl)
			if !ok || declaration.Tok != token.VAR {
				return true
			}
			for _, spec := range declaration.Specs {
				valueSpec, ok := spec.(*ast.ValueSpec)
				if !ok {
					continue
				}
				for index, name := range valueSpec.Names {
					if index < len(valueSpec.Values) && expressionContainsReadLiteral(valueSpec.Values[index], constants) {
						readVariables[name.Name] = true
					}
				}
			}
		}
		return true
	})
	violation := false
	ast.Inspect(body, func(node ast.Node) bool {
		call, ok := node.(*ast.CallExpr)
		if !ok || len(call.Args) == 0 {
			return true
		}
		selector, ok := call.Fun.(*ast.SelectorExpr)
		if !ok || (selector.Sel.Name != "Replace" && selector.Sel.Name != "ReplaceAll") {
			return true
		}
		packageName, ok := selector.X.(*ast.Ident)
		if !ok || packageName.Name != "strings" {
			return true
		}
		if expressionContainsReadLiteral(call.Args[0], constants) {
			violation = true
			return false
		}
		if identifier, ok := call.Args[0].(*ast.Ident); ok && readVariables[identifier.Name] {
			violation = true
			return false
		}
		return true
	})
	return violation
}

func expressionContainsReadLiteral(expression ast.Expr, constants map[string]ast.Expr) bool {
	if value, ok := evalStringExpression(expression, constants, map[string]bool{}); ok {
		return sqlReadPattern.MatchString(value)
	}
	found := false
	ast.Inspect(expression, func(node ast.Node) bool {
		if found {
			return false
		}
		expr, ok := node.(ast.Expr)
		if !ok || expr == expression {
			return true
		}
		if value, ok := evalStringExpression(expr, constants, map[string]bool{}); ok && sqlReadPattern.MatchString(value) {
			found = true
			return false
		}
		return true
	})
	return found
}

func physicalTableInstances(sqlText string) []string {
	ctes := make(map[string]struct{})
	for _, match := range sqlCTEPattern.FindAllStringSubmatch(sqlText, -1) {
		ctes[strings.ToLower(match[1])] = struct{}{}
	}
	result := make([]string, 0)
	for _, match := range sqlTablePattern.FindAllStringSubmatch(sqlText, -1) {
		table := strings.ToLower(match[1])
		if _, isCTE := ctes[table]; isCTE {
			continue
		}
		result = append(result, table)
	}
	return result
}

func viewReadWidthViolations(sqlText string) [][]string {
	var result [][]string
	for _, match := range sqlViewPattern.FindAllStringSubmatch(sqlText, -1) {
		instances := physicalTableInstances(match[1])
		if len(instances) > 3 {
			result = append(result, instances)
		}
	}
	return result
}

func TestPhysicalTableInstancesCountsHiddenQueryWidth(t *testing.T) {
	query := `
WITH roles AS (
  SELECT r.id FROM sys_role r JOIN sys_user_role ur ON ur.roleId = r.id
)
SELECT p.id
FROM roles
JOIN sys_role_permission rp ON rp.roleId = roles.id
JOIN sys_permission p ON p.id = rp.permissionId
UNION ALL
SELECT m.id FROM sys_menu m JOIN sys_role_menu rm ON rm.menuId = m.id`
	instances := physicalTableInstances(query)
	if len(instances) != 6 {
		t.Fatalf("physical instances=%v want=6", instances)
	}
}

func TestEvalStringExpressionFoldsConstIdentifierParenAndBinaryConcat(t *testing.T) {
	constants := map[string]ast.Expr{
		"base": &ast.BasicLit{Kind: token.STRING, Value: strconv.Quote("SELECT * FROM sys_user u ")},
	}
	expression := &ast.ParenExpr{X: &ast.BinaryExpr{
		X:  &ast.Ident{Name: "base"},
		Op: token.ADD,
		Y:  &ast.BasicLit{Kind: token.STRING, Value: strconv.Quote("JOIN sys_user_role ur ON ur.userId=u.id JOIN sys_role r ON r.id=ur.roleId JOIN sys_role_menu rm ON rm.roleId=r.id")},
	}}
	value, ok := evalStringExpression(expression, constants, map[string]bool{})
	if !ok {
		t.Fatal("expected constant expression to fold")
	}
	if instances := physicalTableInstances(value); len(instances) != 4 {
		t.Fatalf("physical instances=%v want=4", instances)
	}
}

func TestMutableReadBuilderDetectionRejectsAssignmentAndStringsBuilderBypasses(t *testing.T) {
	for _, source := range []string{
		`package sample
func query() string {
	base := "SELECT u.id FROM sys_user u"
	q := base
	q += " JOIN sys_user_role ur ON ur.userId=u.id"
	return q
}`,
		`package sample
import "strings"
func query() string {
	var q strings.Builder
	q.WriteString("SELECT u.id FROM sys_user u")
	q.WriteString(" JOIN sys_user_role ur ON ur.userId=u.id")
	return q.String()
}`,
	} {
		fileSet := token.NewFileSet()
		file, err := parser.ParseFile(fileSet, "sample.go", source, 0)
		if err != nil {
			t.Fatalf("parse source: %v", err)
		}
		function := file.Decls[len(file.Decls)-1].(*ast.FuncDecl)
		if !functionUsesMutableReadBuilder(function.Body, collectStringConstants(file)) {
			t.Fatalf("mutable read builder bypass was not detected:\n%s", source)
		}
	}
}

func TestGenericSQLStringRewriteDetectionRejectsSQLVariable(t *testing.T) {
	fileSet := token.NewFileSet()
	file, err := parser.ParseFile(fileSet, "sample.go", `package sample
import "strings"
func query() string {
	query := "SELECT id FROM sys_role WHERE isDeleted = 0"
	return strings.ReplaceAll(query, "isDeleted", "\\\"isDeleted\\\"")
}`, 0)
	if err != nil {
		t.Fatalf("parse source: %v", err)
	}
	function := file.Decls[len(file.Decls)-1].(*ast.FuncDecl)
	if !functionUsesGenericSQLStringRewrite(function.Body, collectStringConstants(file)) {
		t.Fatal("generic SQL string rewrite was not detected")
	}
}

func TestCommandSQLDialectDeclarationRejectsMissingAndInvalidContracts(t *testing.T) {
	cases := []struct {
		name    string
		source  string
		problem bool
	}{
		{
			name: "missing",
			source: `package sample
func query() string { return "SELECT id FROM sys_user" }`,
			problem: true,
		},
		{
			name: "invalid",
			source: `// sql-governance: all-dialects
package sample
func query() string { return "SELECT id FROM sys_user" }`,
			problem: true,
		},
		{
			name: "postgres capable",
			source: `// sql-governance: postgres-capable
package sample
func query() string { return "SELECT id FROM sys_user" }`,
		},
		{
			name: "mysql only",
			source: `// sql-governance: mysql-only
package sample
			func query() string { return "DELETE FROM sys_user" }`,
		},
		{
			name: "attempt private sqlite",
			source: `// sql-governance: sqlite-private
package sample
func query() string { return "SELECT thread_id FROM threads" }`,
		},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			fileSet := token.NewFileSet()
			file, err := parser.ParseFile(fileSet, "sample.go", testCase.source, parser.ParseComments)
			if err != nil {
				t.Fatalf("parse source: %v", err)
			}
			gotProblem := commandSQLDialectDeclarationProblem(file) != ""
			if gotProblem != testCase.problem {
				t.Fatalf("declaration problem=%t want=%t", gotProblem, testCase.problem)
			}
		})
	}
}

func TestViewReadWidthDetectionCountsPhysicalSources(t *testing.T) {
	sqlText := `
CREATE VIEW wide_access AS
SELECT u.id
FROM sys_user u
JOIN sys_user_role ur ON ur.userId=u.id
JOIN sys_role r ON r.id=ur.roleId
JOIN sys_role_permission rp ON rp.roleId=r.id;
`
	violations := viewReadWidthViolations(sqlText)
	if len(violations) != 1 || len(violations[0]) != 4 {
		t.Fatalf("view violations=%v want one four-table view", violations)
	}
}
