package datascope

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"unicode"

	"github.com/octarq-org/octarq/plugin"
	"gorm.io/gorm"
)

var _ plugin.DataScopeFilter = (*OrgFilter)(nil)

var (
	// ErrInvalidOrgID is returned when orgID is empty or blank.
	ErrInvalidOrgID = errors.New("datascope: missing or empty orgID")

	// ErrNilQuery is returned when the query parameter is nil.
	ErrNilQuery = errors.New("datascope: query cannot be nil")
)

// OrgFilter provides the default single-layer data scope filter based on org_id.
// It scopes GORM (*gorm.DB) and raw SQL queries by enforcing WHERE org_id = ?.
// It intentionally does not implement department tree, position hierarchy, or other organization models.
type OrgFilter struct{}

// NewOrgFilter creates a new default OrgFilter instance.
func NewOrgFilter() *OrgFilter {
	return &OrgFilter{}
}

// ApplyScope appends org_id filtering to GORM or SQL queries.
// For *gorm.DB queries, it appends .Where("org_id = ?", orgID).
// For raw SQL string queries, it injects WHERE org_id = ? or AND org_id = ? into the appropriate clause position.
func (f *OrgFilter) ApplyScope(ctx context.Context, query interface{}, userID string, orgID string) (interface{}, error) {
	if ctx != nil {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
	}

	trimmedOrgID := strings.TrimSpace(orgID)
	if trimmedOrgID == "" {
		return nil, ErrInvalidOrgID
	}

	if query == nil {
		return nil, ErrNilQuery
	}

	switch q := query.(type) {
	case *gorm.DB:
		if q == nil {
			return nil, ErrNilQuery
		}
		return q.Where("org_id = ?", trimmedOrgID), nil
	case gorm.DB:
		return q.Where("org_id = ?", trimmedOrgID), nil
	case string:
		return applySQLScope(q, trimmedOrgID)
	case *string:
		if q == nil {
			return nil, ErrNilQuery
		}
		res, err := applySQLScope(*q, trimmedOrgID)
		if err != nil {
			return nil, err
		}
		return &res, nil
	case []byte:
		res, err := applySQLScope(string(q), trimmedOrgID)
		if err != nil {
			return nil, err
		}
		return []byte(res), nil
	default:
		return nil, fmt.Errorf("datascope: unsupported query type: %T", query)
	}
}

func applySQLScope(rawSQL string, orgID string) (string, error) {
	trimmed := strings.TrimSpace(rawSQL)
	if trimmed == "" {
		return "", errors.New("datascope: empty sql query")
	}

	hasTrailingSemicolon := strings.HasSuffix(trimmed, ";")
	trimmed = strings.TrimSuffix(trimmed, ";")
	trimmed = strings.TrimSpace(trimmed)

	hasWhere, trailingPos := scanSQLClauses(trimmed)

	var sb strings.Builder
	if trailingPos >= 0 {
		before := strings.TrimRightFunc(trimmed[:trailingPos], unicode.IsSpace)
		after := strings.TrimLeftFunc(trimmed[trailingPos:], unicode.IsSpace)
		sb.WriteString(before)
		if hasWhere {
			sb.WriteString(" AND org_id = ? ")
		} else {
			sb.WriteString(" WHERE org_id = ? ")
		}
		sb.WriteString(after)
	} else {
		sb.WriteString(trimmed)
		if hasWhere {
			sb.WriteString(" AND org_id = ?")
		} else {
			sb.WriteString(" WHERE org_id = ?")
		}
	}

	if hasTrailingSemicolon {
		sb.WriteString(";")
	}

	return sb.String(), nil
}

func scanSQLClauses(sql string) (hasWhere bool, trailingPos int) {
	trailingPos = -1
	n := len(sql)
	parenDepth := 0
	inQuote := byte(0)

	singleTrailingKeywords := []string{
		"HAVING", "WINDOW", "LIMIT", "OFFSET", "UNION", "INTERSECT", "EXCEPT",
	}

	i := 0
	for i < n {
		b := sql[i]

		// Handle string literals and quoted identifiers
		if inQuote != 0 {
			if b == inQuote {
				if i+1 < n && sql[i+1] == inQuote {
					i += 2
					continue
				}
				inQuote = 0
			}
			i++
			continue
		}

		if b == '\'' || b == '"' || b == '`' {
			inQuote = b
			i++
			continue
		}

		// Handle line comments: --
		if b == '-' && i+1 < n && sql[i+1] == '-' {
			i += 2
			for i < n && sql[i] != '\n' {
				i++
			}
			continue
		}
		// Handle block comments: /* ... */
		if b == '/' && i+1 < n && sql[i+1] == '*' {
			i += 2
			for i+1 < n && !(sql[i] == '*' && sql[i+1] == '/') {
				i++
			}
			i += 2
			continue
		}

		// Handle parentheses
		if b == '(' {
			parenDepth++
			i++
			continue
		} else if b == ')' {
			if parenDepth > 0 {
				parenDepth--
			}
			i++
			continue
		}

		// Only inspect keywords at top level
		if parenDepth == 0 {
			isWordStart := (i == 0 || !isIdentChar(sql[i-1]))
			if isWordStart {
				// Check for WHERE
				if matchWord(sql, i, "WHERE") {
					hasWhere = true
					i += len("WHERE")
					continue
				}

				// Check for trailing clauses if not yet found
				if trailingPos == -1 {
					// Check compound phrases: GROUP BY, ORDER BY, FOR UPDATE, FOR SHARE
					if matchWord(sql, i, "GROUP") {
						if nextWord := nextWordAfter(sql, i+5); strings.EqualFold(nextWord, "BY") {
							trailingPos = i
							i += 5
							continue
						}
					}
					if matchWord(sql, i, "ORDER") {
						if nextWord := nextWordAfter(sql, i+5); strings.EqualFold(nextWord, "BY") {
							trailingPos = i
							i += 5
							continue
						}
					}
					if matchWord(sql, i, "FOR") {
						if nextWord := nextWordAfter(sql, i+3); strings.EqualFold(nextWord, "UPDATE") || strings.EqualFold(nextWord, "SHARE") {
							trailingPos = i
							i += 3
							continue
						}
					}

					for _, kw := range singleTrailingKeywords {
						if matchWord(sql, i, kw) {
							trailingPos = i
							break
						}
					}
				}
			}
		}

		i++
	}

	return hasWhere, trailingPos
}

func matchWord(sql string, pos int, kw string) bool {
	kwLen := len(kw)
	if pos+kwLen > len(sql) {
		return false
	}
	if !strings.EqualFold(sql[pos:pos+kwLen], kw) {
		return false
	}
	if pos+kwLen < len(sql) {
		if isIdentChar(sql[pos+kwLen]) {
			return false
		}
	}
	return true
}

func nextWordAfter(sql string, pos int) string {
	n := len(sql)
	for pos < n && unicode.IsSpace(rune(sql[pos])) {
		pos++
	}
	if pos >= n {
		return ""
	}
	start := pos
	for pos < n && isIdentChar(sql[pos]) {
		pos++
	}
	return sql[start:pos]
}

func isIdentChar(b byte) bool {
	return b == '_' ||
		(b >= 'a' && b <= 'z') ||
		(b >= 'A' && b <= 'Z') ||
		(b >= '0' && b <= '9')
}
