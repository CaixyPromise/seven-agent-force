package store

import (
	"fmt"
	"strings"

	"github.com/jmoiron/sqlx"
)

// IsPostgres reports whether db uses a PostgreSQL-compatible sqlx driver.
func IsPostgres(db *sqlx.DB) bool {
	if db == nil {
		return false
	}
	driver := strings.ToLower(strings.TrimSpace(db.DriverName()))
	return strings.Contains(driver, "postgres") || strings.Contains(driver, "pgx")
}

// PostgresRenderer renders reviewed, static MySQL-compatible SQL for
// PostgreSQL. Only identifiers declared when the renderer is constructed are
// quoted. String and dollar-quoted literals, comments, and already quoted
// identifiers are copied byte-for-byte.
//
// The renderer is deliberately not a general SQL parser. Runtime input must
// remain bound values and must never be added to the identifier allowlist.
type PostgresRenderer struct {
	identifiers    map[string]struct{}
	booleanColumns map[string]struct{}
}

// NewPostgresRenderer constructs an explicit identifier allowlist. Boolean
// columns are a subset of identifiers whose static comparisons to 0/1 are
// rendered as FALSE/TRUE for PostgreSQL.
func NewPostgresRenderer(identifiers []string, booleanColumns ...string) (*PostgresRenderer, error) {
	renderer := &PostgresRenderer{
		identifiers:    make(map[string]struct{}, len(identifiers)),
		booleanColumns: make(map[string]struct{}, len(booleanColumns)),
	}
	for _, identifier := range identifiers {
		if err := validateStaticIdentifier(identifier); err != nil {
			return nil, err
		}
		renderer.identifiers[identifier] = struct{}{}
	}
	for _, identifier := range booleanColumns {
		if err := validateStaticIdentifier(identifier); err != nil {
			return nil, err
		}
		if _, ok := renderer.identifiers[identifier]; !ok {
			return nil, fmt.Errorf("PostgreSQL boolean identifier %q is not in the identifier allowlist", identifier)
		}
		renderer.booleanColumns[identifier] = struct{}{}
	}
	return renderer, nil
}

// MustNewPostgresRenderer is intended for package-level, reviewed static
// allowlists. Invalid source-controlled identifiers fail during startup.
func MustNewPostgresRenderer(identifiers []string, booleanColumns ...string) *PostgresRenderer {
	renderer, err := NewPostgresRenderer(identifiers, booleanColumns...)
	if err != nil {
		panic(err)
	}
	return renderer
}

// Render leaves MySQL SQL untouched and applies the explicit PostgreSQL
// renderer only for a PostgreSQL-compatible driver.
func (r *PostgresRenderer) Render(db *sqlx.DB, query string) string {
	if r == nil || !IsPostgres(db) {
		return query
	}
	return r.RenderPostgres(query)
}

// RenderPostgres renders SQL for PostgreSQL without inspecting runtime values.
func (r *PostgresRenderer) RenderPostgres(query string) string {
	if r == nil || query == "" {
		return query
	}

	var result strings.Builder
	result.Grow(len(query) + len(query)/8)
	booleanState := booleanComparisonNone

	for index := 0; index < len(query); {
		switch {
		case query[index] == '\'':
			end := quotedEnd(query, index, '\'', true, postgresEscapeStringPrefix(query, index))
			result.WriteString(query[index:end])
			index = end
			booleanState = booleanComparisonNone
		case query[index] == '"':
			end := quotedEnd(query, index, '"', true, false)
			result.WriteString(query[index:end])
			index = end
			booleanState = booleanComparisonNone
		case query[index] == '`':
			end := quotedEnd(query, index, '`', true, false)
			result.WriteString(query[index:end])
			index = end
			booleanState = booleanComparisonNone
		case strings.HasPrefix(query[index:], "--"):
			end := lineCommentEnd(query, index)
			result.WriteString(query[index:end])
			index = end
		case strings.HasPrefix(query[index:], "/*"):
			end := blockCommentEnd(query, index)
			result.WriteString(query[index:end])
			index = end
		case query[index] == '$':
			if delimiter, ok := dollarQuoteDelimiter(query[index:]); ok {
				end := dollarQuotedEnd(query, index, delimiter)
				result.WriteString(query[index:end])
				index = end
				booleanState = booleanComparisonNone
			} else {
				result.WriteByte(query[index])
				index++
				booleanState = booleanComparisonNone
			}
		case isIdentifierStart(query[index]):
			end := index + 1
			for end < len(query) && isIdentifierPart(query[end]) {
				end++
			}
			identifier := query[index:end]
			if _, ok := r.identifiers[identifier]; ok {
				result.WriteByte('"')
				result.WriteString(identifier)
				result.WriteByte('"')
			} else {
				result.WriteString(identifier)
			}
			if _, ok := r.booleanColumns[identifier]; ok {
				booleanState = booleanComparisonColumn
			} else {
				booleanState = booleanComparisonNone
			}
			index = end
		case isSQLSpace(query[index]):
			result.WriteByte(query[index])
			index++
		case query[index] == '=' && booleanState == booleanComparisonColumn:
			result.WriteByte('=')
			index++
			booleanState = booleanComparisonEquals
		case booleanState == booleanComparisonEquals &&
			(query[index] == '0' || query[index] == '1') &&
			(index+1 == len(query) || !isIdentifierPart(query[index+1])):
			if query[index] == '0' {
				result.WriteString("FALSE")
			} else {
				result.WriteString("TRUE")
			}
			index++
			booleanState = booleanComparisonNone
		default:
			result.WriteByte(query[index])
			index++
			booleanState = booleanComparisonNone
		}
	}
	return result.String()
}

type booleanComparisonState uint8

const (
	booleanComparisonNone booleanComparisonState = iota
	booleanComparisonColumn
	booleanComparisonEquals
)

func validateStaticIdentifier(identifier string) error {
	if identifier == "" || !isIdentifierStart(identifier[0]) {
		return fmt.Errorf("invalid PostgreSQL identifier allowlist entry %q", identifier)
	}
	for index := 1; index < len(identifier); index++ {
		if !isIdentifierPart(identifier[index]) {
			return fmt.Errorf("invalid PostgreSQL identifier allowlist entry %q", identifier)
		}
	}
	return nil
}

func isIdentifierStart(value byte) bool {
	return value == '_' || value >= 'a' && value <= 'z' || value >= 'A' && value <= 'Z'
}

func isIdentifierPart(value byte) bool {
	return isIdentifierStart(value) || value >= '0' && value <= '9' || value == '$'
}

func isSQLSpace(value byte) bool {
	switch value {
	case ' ', '\t', '\n', '\r', '\f':
		return true
	default:
		return false
	}
}

func quotedEnd(query string, start int, quote byte, doubledEscape, backslashEscape bool) int {
	for index := start + 1; index < len(query); index++ {
		if backslashEscape && query[index] == '\\' && index+1 < len(query) {
			index++
			continue
		}
		if query[index] != quote {
			continue
		}
		if doubledEscape && index+1 < len(query) && query[index+1] == quote {
			index++
			continue
		}
		return index + 1
	}
	return len(query)
}

func postgresEscapeStringPrefix(query string, quoteIndex int) bool {
	if quoteIndex <= 0 || query[quoteIndex-1] != 'E' && query[quoteIndex-1] != 'e' {
		return false
	}
	return quoteIndex == 1 || !isIdentifierPart(query[quoteIndex-2])
}

func lineCommentEnd(query string, start int) int {
	if newline := strings.IndexByte(query[start:], '\n'); newline >= 0 {
		return start + newline + 1
	}
	return len(query)
}

func blockCommentEnd(query string, start int) int {
	depth := 1
	for index := start + 2; index < len(query); {
		switch {
		case strings.HasPrefix(query[index:], "/*"):
			depth++
			index += 2
		case strings.HasPrefix(query[index:], "*/"):
			depth--
			index += 2
			if depth == 0 {
				return index
			}
		default:
			index++
		}
	}
	return len(query)
}

func dollarQuoteDelimiter(query string) (string, bool) {
	if len(query) < 2 || query[0] != '$' {
		return "", false
	}
	for index := 1; index < len(query); index++ {
		switch {
		case query[index] == '$':
			return query[:index+1], true
		case index == 1 && !isIdentifierStart(query[index]):
			return "", false
		case index > 1 && !isIdentifierPart(query[index]):
			return "", false
		}
	}
	return "", false
}

func dollarQuotedEnd(query string, start int, delimiter string) int {
	contentStart := start + len(delimiter)
	if relative := strings.Index(query[contentStart:], delimiter); relative >= 0 {
		return contentStart + relative + len(delimiter)
	}
	return len(query)
}
