package governance

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"fmt"
	"strings"
	"testing"
)

// dg4PhysicalTableMapping is fixed, source-controlled acceptance data. It is
// deliberately not constructed from a request, database row, or environment.
type dg4PhysicalTableMapping struct {
	legacy string
	target string
}

type dg4PhysicalSignature struct {
	rowCount         int64
	primaryKeyDigest string
	primaryIndex     string
	allIndexes       string
	identityColumns  string
}

const dg4PhysicalSignatureTable = "dg4_physical_signature"

func dg4B1PhysicalMappings() []dg4PhysicalTableMapping {
	result := make([]dg4PhysicalTableMapping, 0, len(b1TableMappings))
	for _, mapping := range b1TableMappings {
		result = append(result, dg4PhysicalTableMapping{legacy: mapping.legacy, target: mapping.target})
	}
	return result
}

func dg4B2PhysicalMappings() []dg4PhysicalTableMapping {
	result := make([]dg4PhysicalTableMapping, 0, len(b2TableMappings))
	for _, mapping := range b2TableMappings {
		result = append(result, dg4PhysicalTableMapping{legacy: mapping.legacy, target: mapping.target})
	}
	return result
}

func dg4B3PhysicalMappings() []dg4PhysicalTableMapping {
	result := make([]dg4PhysicalTableMapping, 0, len(b3TableMappings))
	for _, mapping := range b3TableMappings {
		result = append(result, dg4PhysicalTableMapping{legacy: mapping.legacy, target: mapping.target})
	}
	return result
}

// dg4CapturePhysicalSignatures stores only de-identified physical metadata in
// the exact isolated acceptance database. The record crosses the historical
// source-process boundary used by the staged B1/B2/B3 runner and is removed
// after B3 proves the final batch.
func dg4CapturePhysicalSignatures(t *testing.T, ctx context.Context, db *sql.DB, dialect, batch string, mappings []dg4PhysicalTableMapping) {
	t.Helper()
	dg4EnsurePhysicalSignatureTable(t, ctx, db)
	if _, err := db.ExecContext(ctx, `DELETE FROM `+dg4PhysicalSignatureTable+` WHERE batch = `+dg4Placeholder(dialect, 1), batch); err != nil {
		t.Fatalf("clear DG4 %s physical signatures: %v", batch, err)
	}
	for _, mapping := range mappings {
		signature := dg4ReadPhysicalSignature(t, ctx, db, dialect, mapping.legacy)
		query := `INSERT INTO ` + dg4PhysicalSignatureTable + ` (
			batch, legacy_table, row_count, primary_key_digest, primary_index_signature, index_signature, identity_signature
		) VALUES (` + dg4Placeholders(dialect, 7) + `)`
		if _, err := db.ExecContext(ctx, query,
			batch,
			mapping.legacy,
			signature.rowCount,
			signature.primaryKeyDigest,
			signature.primaryIndex,
			signature.allIndexes,
			signature.identityColumns,
		); err != nil {
			t.Fatalf("store DG4 %s physical signature for %s: %v", batch, mapping.legacy, err)
		}
	}
}

// dg4AssertPhysicalSignatures verifies that a direct table rename preserved
// the fixture's primary-key identity and the database-owned physical metadata.
// It intentionally runs before the post-rename test creates another fixture
// row, so a legitimate new insert cannot change the before/after comparison.
func dg4AssertPhysicalSignatures(t *testing.T, ctx context.Context, db *sql.DB, dialect, batch string, mappings []dg4PhysicalTableMapping) {
	t.Helper()
	for _, mapping := range mappings {
		query := `SELECT row_count, primary_key_digest, primary_index_signature, index_signature, identity_signature
			FROM ` + dg4PhysicalSignatureTable + ` WHERE batch = ` + dg4Placeholder(dialect, 1) + ` AND legacy_table = ` + dg4Placeholder(dialect, 2)
		var expected dg4PhysicalSignature
		if err := db.QueryRowContext(ctx, query, batch, mapping.legacy).Scan(
			&expected.rowCount,
			&expected.primaryKeyDigest,
			&expected.primaryIndex,
			&expected.allIndexes,
			&expected.identityColumns,
		); err != nil {
			t.Fatalf("load DG4 %s physical signature for %s: %v", batch, mapping.legacy, err)
		}
		actual := dg4ReadPhysicalSignature(t, ctx, db, dialect, mapping.target)
		if actual != expected {
			t.Fatalf("DG4 %s physical signature changed for %s -> %s: actual=%+v expected=%+v", batch, mapping.legacy, mapping.target, actual, expected)
		}
	}
}

func dg4DropPhysicalSignatures(t *testing.T, ctx context.Context, db *sql.DB) {
	t.Helper()
	if _, err := db.ExecContext(ctx, `DROP TABLE `+dg4PhysicalSignatureTable); err != nil {
		t.Fatalf("drop DG4 isolated physical-signature record: %v", err)
	}
}

func dg4EnsurePhysicalSignatureTable(t *testing.T, ctx context.Context, db *sql.DB) {
	t.Helper()
	query := `CREATE TABLE IF NOT EXISTS ` + dg4PhysicalSignatureTable + ` (
		batch VARCHAR(8) NOT NULL,
		legacy_table VARCHAR(128) NOT NULL,
		row_count BIGINT NOT NULL,
		primary_key_digest VARCHAR(128) NOT NULL,
		primary_index_signature TEXT NOT NULL,
		index_signature TEXT NOT NULL,
		identity_signature TEXT NOT NULL,
		PRIMARY KEY (batch, legacy_table)
	)`
	if _, err := db.ExecContext(ctx, query); err != nil {
		t.Fatalf("create DG4 isolated physical-signature record: %v", err)
	}
}

func dg4ReadPhysicalSignature(t *testing.T, ctx context.Context, db *sql.DB, dialect, table string) dg4PhysicalSignature {
	t.Helper()
	rowCount, primaryKeyDigest := dg4PhysicalRowsAndPrimaryKey(t, ctx, db, dialect, table)
	primary, indexes := dg4PhysicalIndexSignatures(t, ctx, db, dialect, table)
	identity := dg4PhysicalIdentitySignature(t, ctx, db, dialect, table)
	return dg4PhysicalSignature{
		rowCount:         rowCount,
		primaryKeyDigest: primaryKeyDigest,
		primaryIndex:     primary,
		allIndexes:       indexes,
		identityColumns:  identity,
	}
}

func dg4PhysicalRowsAndPrimaryKey(t *testing.T, ctx context.Context, db *sql.DB, dialect, table string) (int64, string) {
	t.Helper()
	quoted := dg4QuotePhysicalTable(dialect, table)
	query := `SELECT COUNT(*) FROM ` + quoted
	var rowCount int64
	if err := db.QueryRowContext(ctx, query).Scan(&rowCount); err != nil {
		t.Fatalf("count DG4 physical rows for %s: %v", table, err)
	}
	primaryColumns := dg4PrimaryKeyColumns(t, ctx, db, dialect, table)
	if len(primaryColumns) == 0 {
		return rowCount, dg4Digest("")
	}
	return rowCount, dg4PrimaryKeyDigest(t, ctx, db, dialect, table, primaryColumns)
}

func dg4PrimaryKeyColumns(t *testing.T, ctx context.Context, db *sql.DB, dialect, table string) []string {
	t.Helper()
	query := `
SELECT column_name
FROM information_schema.statistics
WHERE table_schema = DATABASE() AND table_name = ? AND index_name = 'PRIMARY'
ORDER BY seq_in_index`
	args := []any{table}
	if dialect == "postgres" {
		query = `
SELECT attr.attname
FROM pg_index ind
JOIN pg_class tbl ON tbl.oid = ind.indrelid
JOIN unnest(ind.indkey) WITH ORDINALITY AS key_column(attnum, ordinal) ON TRUE
JOIN pg_attribute attr ON attr.attrelid = ind.indrelid AND attr.attnum = key_column.attnum
WHERE tbl.oid = $1::regclass AND ind.indisprimary
ORDER BY key_column.ordinal`
		args = []any{`public.` + dg4QuotePhysicalTable("postgres", table)}
	}
	rows, err := db.QueryContext(ctx, query, args...)
	if err != nil {
		t.Fatalf("read DG4 primary-key columns for %s: %v", table, err)
	}
	defer rows.Close()
	columns := make([]string, 0, 2)
	for rows.Next() {
		var column string
		if err := rows.Scan(&column); err != nil {
			t.Fatalf("scan DG4 primary-key column for %s: %v", table, err)
		}
		columns = append(columns, column)
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("iterate DG4 primary-key columns for %s: %v", table, err)
	}
	return columns
}

func dg4PrimaryKeyDigest(t *testing.T, ctx context.Context, db *sql.DB, dialect, table string, columns []string) string {
	t.Helper()
	quotedColumns := make([]string, 0, len(columns))
	for _, column := range columns {
		quotedColumns = append(quotedColumns, dg4QuotePhysicalTable(dialect, column))
	}
	query := `SELECT ` + strings.Join(quotedColumns, ", ") + ` FROM ` + dg4QuotePhysicalTable(dialect, table) + ` ORDER BY ` + strings.Join(quotedColumns, ", ")
	rows, err := db.QueryContext(ctx, query)
	if err != nil {
		t.Fatalf("read DG4 primary-key values for %s: %v", table, err)
	}
	defer rows.Close()
	var builder strings.Builder
	for rows.Next() {
		values := make([]any, len(columns))
		destinations := make([]any, len(columns))
		for index := range values {
			destinations[index] = &values[index]
		}
		if err := rows.Scan(destinations...); err != nil {
			t.Fatalf("scan DG4 primary-key values for %s: %v", table, err)
		}
		for _, value := range values {
			valueSignature := dg4PrimaryKeyValueSignature(value)
			fmt.Fprintf(&builder, "%d:%s|", len(valueSignature), valueSignature)
		}
		builder.WriteByte('\n')
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("iterate DG4 primary-key values for %s: %v", table, err)
	}
	return dg4Digest(builder.String())
}

func dg4PrimaryKeyValueSignature(value any) string {
	switch typed := value.(type) {
	case nil:
		return "null"
	case []byte:
		return "bytes:" + hex.EncodeToString(typed)
	default:
		return fmt.Sprintf("%T:%v", value, value)
	}
}

func dg4PhysicalIndexSignatures(t *testing.T, ctx context.Context, db *sql.DB, dialect, table string) (string, string) {
	t.Helper()
	if dialect == "postgres" {
		return dg4PostgresIndexSignatures(t, ctx, db, table)
	}
	return dg4MySQLIndexSignatures(t, ctx, db, table)
}

func dg4MySQLIndexSignatures(t *testing.T, ctx context.Context, db *sql.DB, table string) (string, string) {
	t.Helper()
	rows, err := db.QueryContext(ctx, `
SELECT index_name, non_unique, seq_in_index, column_name, COALESCE(collation, ''), COALESCE(sub_part, -1)
FROM information_schema.statistics
WHERE table_schema = DATABASE() AND table_name = ?
ORDER BY index_name, seq_in_index`, table)
	if err != nil {
		t.Fatalf("read MySQL DG4 indexes for %s: %v", table, err)
	}
	defer rows.Close()
	parts := make([]string, 0, 8)
	primary := make([]string, 0, 2)
	for rows.Next() {
		var indexName, columnName, collation string
		var nonUnique, sequence, subPart int64
		if err := rows.Scan(&indexName, &nonUnique, &sequence, &columnName, &collation, &subPart); err != nil {
			t.Fatalf("scan MySQL DG4 index for %s: %v", table, err)
		}
		part := fmt.Sprintf("%s|%d|%d|%s|%s|%d", indexName, nonUnique, sequence, columnName, collation, subPart)
		parts = append(parts, part)
		if indexName == "PRIMARY" {
			primary = append(primary, part)
		}
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("iterate MySQL DG4 indexes for %s: %v", table, err)
	}
	return strings.Join(primary, ";"), strings.Join(parts, ";")
}

func dg4PostgresIndexSignatures(t *testing.T, ctx context.Context, db *sql.DB, table string) (string, string) {
	t.Helper()
	relation := `public.` + dg4QuotePhysicalTable("postgres", table)
	rows, err := db.QueryContext(ctx, `
SELECT idx.relname, ind.indisprimary, ind.indisunique, am.amname,
       COALESCE(pg_get_expr(ind.indpred, ind.indrelid), ''), pg_get_indexdef(ind.indexrelid)
FROM pg_index ind
JOIN pg_class tbl ON tbl.oid = ind.indrelid
JOIN pg_class idx ON idx.oid = ind.indexrelid
JOIN pg_am am ON am.oid = idx.relam
WHERE tbl.oid = $1::regclass
ORDER BY idx.relname`, relation)
	if err != nil {
		t.Fatalf("read PostgreSQL DG4 indexes for %s: %v", table, err)
	}
	defer rows.Close()
	parts := make([]string, 0, 8)
	primary := make([]string, 0, 2)
	for rows.Next() {
		var indexName, method, predicate, definition string
		var isPrimary, isUnique bool
		if err := rows.Scan(&indexName, &isPrimary, &isUnique, &method, &predicate, &definition); err != nil {
			t.Fatalf("scan PostgreSQL DG4 index for %s: %v", table, err)
		}
		part := fmt.Sprintf("%s|%t|%t|%s|%s|%s", indexName, isPrimary, isUnique, method, predicate, dg4NormalizePostgresIndexDefinition(definition, table))
		parts = append(parts, part)
		if isPrimary {
			primary = append(primary, part)
		}
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("iterate PostgreSQL DG4 indexes for %s: %v", table, err)
	}
	return strings.Join(primary, ";"), strings.Join(parts, ";")
}

func dg4PhysicalIdentitySignature(t *testing.T, ctx context.Context, db *sql.DB, dialect, table string) string {
	t.Helper()
	if dialect == "postgres" {
		return dg4PostgresIdentitySignature(t, ctx, db, table)
	}
	return dg4MySQLIdentitySignature(t, ctx, db, table)
}

func dg4MySQLIdentitySignature(t *testing.T, ctx context.Context, db *sql.DB, table string) string {
	t.Helper()
	rows, err := db.QueryContext(ctx, `
SELECT column_name, column_type, extra
FROM information_schema.columns
WHERE table_schema = DATABASE() AND table_name = ? AND extra LIKE '%auto_increment%'
ORDER BY ordinal_position`, table)
	if err != nil {
		t.Fatalf("read MySQL DG4 identity columns for %s: %v", table, err)
	}
	defer rows.Close()
	parts := make([]string, 0, 1)
	for rows.Next() {
		var name, columnType, extra string
		if err := rows.Scan(&name, &columnType, &extra); err != nil {
			t.Fatalf("scan MySQL DG4 identity column for %s: %v", table, err)
		}
		parts = append(parts, name+"|"+columnType+"|"+extra)
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("iterate MySQL DG4 identity columns for %s: %v", table, err)
	}
	var nextValue sql.NullInt64
	if err := db.QueryRowContext(ctx, `
SELECT auto_increment
FROM information_schema.tables
WHERE table_schema = DATABASE() AND table_name = ?`, table).Scan(&nextValue); err != nil {
		t.Fatalf("read MySQL DG4 auto-increment state for %s: %v", table, err)
	}
	if nextValue.Valid {
		parts = append(parts, fmt.Sprintf("table-auto-increment|%d", nextValue.Int64))
	}
	return strings.Join(parts, ";")
}

func dg4PostgresIdentitySignature(t *testing.T, ctx context.Context, db *sql.DB, table string) string {
	t.Helper()
	relation := `public.` + dg4QuotePhysicalTable("postgres", table)
	rows, err := db.QueryContext(ctx, `
SELECT attr.attname, attr.attidentity, COALESCE(pg_get_expr(def.adbin, def.adrelid), '')
FROM pg_attribute attr
LEFT JOIN pg_attrdef def ON def.adrelid = attr.attrelid AND def.adnum = attr.attnum
WHERE attr.attrelid = $1::regclass
  AND attr.attnum > 0
  AND NOT attr.attisdropped
  AND (attr.attidentity <> '' OR COALESCE(pg_get_expr(def.adbin, def.adrelid), '') LIKE 'nextval(%')
ORDER BY attr.attnum`, relation)
	if err != nil {
		t.Fatalf("read PostgreSQL DG4 identity columns for %s: %v", table, err)
	}
	defer rows.Close()
	parts := make([]string, 0, 1)
	for rows.Next() {
		var name, identity, defaultExpression string
		if err := rows.Scan(&name, &identity, &defaultExpression); err != nil {
			t.Fatalf("scan PostgreSQL DG4 identity column for %s: %v", table, err)
		}
		parts = append(parts, name+"|"+identity+"|"+defaultExpression)
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("iterate PostgreSQL DG4 identity columns for %s: %v", table, err)
	}
	return strings.Join(parts, ";")
}

func dg4QuotePhysicalTable(dialect, table string) string {
	if dialect == "postgres" {
		return `"` + strings.ReplaceAll(table, `"`, `""`) + `"`
	}
	return "`" + strings.ReplaceAll(table, "`", "``") + "`"
}

func dg4NormalizePostgresIndexDefinition(definition, table string) string {
	quotedTable := dg4QuotePhysicalTable("postgres", table)
	// PostgreSQL preserves an index's original name when its table is renamed.
	// Normalize only the relation in the ON clause, never every occurrence of
	// the table text: an index name may deliberately contain the legacy table
	// name and is part of the physical metadata we must preserve.
	normalized := strings.ReplaceAll(definition, " ON public."+quotedTable+" ", " ON public.<table> ")
	normalized = strings.ReplaceAll(normalized, " ON public."+table+" ", " ON public.<table> ")
	normalized = strings.ReplaceAll(normalized, " ON "+quotedTable+" ", " ON <table> ")
	normalized = strings.ReplaceAll(normalized, " ON "+table+" ", " ON <table> ")
	return strings.Join(strings.Fields(normalized), " ")
}

func dg4Digest(value string) string {
	digest := sha256.Sum256([]byte(value))
	return hex.EncodeToString(digest[:])
}

func dg4Placeholder(dialect string, ordinal int) string {
	if dialect == "postgres" {
		return fmt.Sprintf("$%d", ordinal)
	}
	return "?"
}

func dg4Placeholders(dialect string, count int) string {
	parts := make([]string, 0, count)
	for ordinal := 1; ordinal <= count; ordinal++ {
		parts = append(parts, dg4Placeholder(dialect, ordinal))
	}
	return strings.Join(parts, ", ")
}
