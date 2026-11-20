package tenantsql

import (
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/xwb1989/sqlparser"
)

const DefaultViewPrefix = "tenant_"

const MaxSQLLength = 10 * 1024

var DefaultAllowedFunctions = []string{
	"count",
	"sum",
	"min",
	"max",
	"avg",
	"coalesce",
	"lower",
	"upper",
	"length",
	"date_trunc",
	"strftime",
}

type Parser interface {
	Validate(sql string) error
}

type Option func(*parserOptions)

type parserOptions struct {
	viewPrefix       string
	allowedFunctions map[string]bool
	allowedViews     map[string]bool
}

func WithViewPrefix(prefix string) Option {
	return func(opts *parserOptions) {
		opts.viewPrefix = prefix
	}
}

func WithAllowedFunctions(names []string) Option {
	return func(opts *parserOptions) {
		opts.allowedFunctions = make(map[string]bool, len(names))
		for _, name := range names {
			trimmed := strings.ToLower(strings.TrimSpace(name))
			if trimmed != "" {
				opts.allowedFunctions[trimmed] = true
			}
		}
	}
}

func WithAllowedViews(names []string) Option {
	return func(opts *parserOptions) {
		opts.allowedViews = make(map[string]bool, len(names))
		for _, name := range names {
			trimmed := strings.ToLower(strings.TrimSpace(name))
			if trimmed != "" {
				opts.allowedViews[trimmed] = true
			}
		}
	}
}

type parserImpl struct {
	viewPrefix       string
	allowedFunctions map[string]bool
	allowedViews     map[string]bool
}

func rejectUnknownTableExpr(expr sqlparser.SimpleTableExpr) error {
	switch expr.(type) {
	case nil:
		return nil
	case *sqlparser.Subquery:
		return nil
	case sqlparser.TableName:
		return nil
	default:
		return fmt.Errorf("disallowed table expression %T: only plain views and derived subqueries are permitted", expr)
	}
}

func NewParser(opts ...Option) Parser {
	defaultFuncs := make(map[string]bool, len(DefaultAllowedFunctions))
	for _, fn := range DefaultAllowedFunctions {
		defaultFuncs[strings.ToLower(strings.TrimSpace(fn))] = true
	}

	pOpts := &parserOptions{
		viewPrefix:       DefaultViewPrefix,
		allowedFunctions: defaultFuncs,
	}

	for _, opt := range opts {
		if opt != nil {
			opt(pOpts)
		}
	}

	return &parserImpl{
		viewPrefix:       pOpts.viewPrefix,
		allowedFunctions: pOpts.allowedFunctions,
		allowedViews:     pOpts.allowedViews,
	}
}

func (p *parserImpl) Validate(sql string) (err error) {
	trimmed := strings.TrimSpace(sql)
	if trimmed == "" {
		return errors.New("sql statement cannot be empty")
	}
	if len(sql) > MaxSQLLength {
		return fmt.Errorf("sql statement exceeds maximum allowed length of %d bytes (got %d bytes)", MaxSQLLength, len(sql))
	}

	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("sql parsing failed: %v", r)
		}
	}()

	stmt, parseErr := parseSafely(sql)
	if parseErr != nil {
		return fmt.Errorf("sql parsing failed: %v", parseErr)
	}

	selectStmt, ok := stmt.(*sqlparser.Select)
	if !ok {
		return fmt.Errorf("disallowed statement type %s: only SELECT statements are permitted", statementTypeName(stmt))
	}

	walkErr := sqlparser.Walk(func(node sqlparser.SQLNode) (bool, error) {
		if node == nil {
			return true, nil
		}

		switch n := node.(type) {
		case *sqlparser.AliasedTableExpr:
			if n == nil {
				return true, nil
			}
			if err := rejectUnknownTableExpr(n.Expr); err != nil {
				return false, err
			}
			if tn, ok := n.Expr.(sqlparser.TableName); ok {
				if !tn.Qualifier.IsEmpty() {
					return false, fmt.Errorf("disallowed qualified table %q: cross-database queries are not permitted", tn.Qualifier.String()+"."+tn.Name.String())
				}
				name := tn.Name.String()
				if name == "" || strings.EqualFold(name, "dual") {
					return true, nil
				}
				if !strings.HasPrefix(strings.ToLower(name), strings.ToLower(p.viewPrefix)) {
					return false, fmt.Errorf("disallowed table %q: table name must start with %q", name, p.viewPrefix)
				}
				if len(p.allowedViews) > 0 && !p.allowedViews[strings.ToLower(name)] {
					return false, fmt.Errorf("unknown view %q: view is not in the allowed views whitelist", name)
				}
			}
		case *sqlparser.ColName:
			if n == nil {
				return true, nil
			}
			if !n.Qualifier.Qualifier.IsEmpty() {
				return false, fmt.Errorf("disallowed qualified column qualifier %q: cross-database queries are not permitted", n.Qualifier.Qualifier.String())
			}
		case sqlparser.TableName:
			if !n.Qualifier.IsEmpty() {
				return false, fmt.Errorf("disallowed qualified identifier %q: cross-database queries are not permitted", n.Qualifier.String())
			}
		case *sqlparser.FuncExpr:
			if n == nil {
				return true, nil
			}
			if !n.Qualifier.IsEmpty() {
				return false, fmt.Errorf("disallowed qualified function %q: qualified functions are not permitted", n.Qualifier.String()+"."+n.Name.String())
			}
			fnName := strings.ToLower(n.Name.String())
			if !p.allowedFunctions[fnName] {
				return false, fmt.Errorf("disallowed function %q: function is not in the allowed functions whitelist", n.Name.String())
			}
		case *sqlparser.GroupConcatExpr:
			if n == nil {
				return true, nil
			}
			if !p.allowedFunctions["group_concat"] {
				return false, fmt.Errorf("disallowed function \"group_concat\": function is not in the allowed functions whitelist")
			}
		case *sqlparser.SubstrExpr:
			if n == nil {
				return true, nil
			}
			fnName := strings.ToLower(n.Name.Name.String())
			if fnName == "" {
				fnName = "substr"
			}
			if !p.allowedFunctions[fnName] && !p.allowedFunctions["substr"] && !p.allowedFunctions["substring"] {
				return false, fmt.Errorf("disallowed function %q: function is not in the allowed functions whitelist", fnName)
			}
		case *sqlparser.ConvertExpr:
			if n == nil {
				return true, nil
			}
			if !p.allowedFunctions["convert"] && !p.allowedFunctions["cast"] {
				return false, fmt.Errorf("disallowed function \"convert\": function is not in the allowed functions whitelist")
			}
		case *sqlparser.ConvertUsingExpr:
			if n == nil {
				return true, nil
			}
			if !p.allowedFunctions["convert"] && !p.allowedFunctions["cast"] {
				return false, fmt.Errorf("disallowed function \"convert\": function is not in the allowed functions whitelist")
			}
		case *sqlparser.ValuesFuncExpr:
			return false, fmt.Errorf("disallowed function \"values\": function is not permitted")
		case *sqlparser.MatchExpr:
			if n == nil {
				return true, nil
			}
			if !p.allowedFunctions["match"] {
				return false, fmt.Errorf("disallowed function \"match\": function is not in the allowed functions whitelist")
			}
		}

		return true, nil
	}, selectStmt)

	if walkErr != nil {
		return walkErr
	}

	if err := checkSingleStatement(sql); err != nil {
		return err
	}

	return nil
}

var parseFunc = sqlparser.Parse

func parseSafely(sql string) (stmt sqlparser.Statement, err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("parser panic: %v", r)
		}
	}()
	return parseFunc(sql)
}

func checkSingleStatement(sql string) error {
	tok := sqlparser.NewStringTokenizer(sql)
	_, err := sqlparser.ParseNext(tok)
	if err != nil {
		return fmt.Errorf("sql parsing failed: %v", err)
	}
	_, nextErr := sqlparser.ParseNext(tok)
	if nextErr != io.EOF {
		return errors.New("multiple SQL statements or trailing content are not permitted")
	}

	pieces, splitErr := sqlparser.SplitStatementToPieces(sql)
	if splitErr == nil {
		nonEmptyPieces := 0
		for _, piece := range pieces {
			if strings.TrimSpace(piece) != "" {
				nonEmptyPieces++
			}
		}
		if nonEmptyPieces > 1 {
			return errors.New("multiple SQL statements are not permitted")
		}
	}
	return nil
}

func statementTypeName(stmt sqlparser.Statement) string {
	if stmt == nil {
		return "unknown"
	}
	switch stmt.(type) {
	case *sqlparser.Select:
		return "SELECT"
	case *sqlparser.Insert:
		return "INSERT"
	case *sqlparser.Update:
		return "UPDATE"
	case *sqlparser.Delete:
		return "DELETE"
	case *sqlparser.DDL:
		return "DDL"
	case *sqlparser.DBDDL:
		return "DBDDL"
	case *sqlparser.Set:
		return "SET"
	case *sqlparser.Show:
		return "SHOW"
	case *sqlparser.Use:
		return "USE"
	case *sqlparser.OtherRead:
		return "OTHER_READ"
	case *sqlparser.OtherAdmin:
		return "OTHER_ADMIN"
	case *sqlparser.Union:
		return "UNION"
	default:
		return fmt.Sprintf("%T", stmt)
	}
}

func ExtractReferencedViews(sql string) (views []string, err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("sql parsing failed: %v", r)
		}
	}()

	trimmed := strings.TrimSpace(sql)
	if trimmed == "" {
		return nil, errors.New("sql statement cannot be empty")
	}
	if len(sql) > MaxSQLLength {
		return nil, fmt.Errorf("sql statement exceeds maximum allowed length of %d bytes", MaxSQLLength)
	}

	stmt, parseErr := parseSafely(sql)
	if parseErr != nil {
		return nil, fmt.Errorf("sql parsing failed: %v", parseErr)
	}

	selectStmt, ok := stmt.(*sqlparser.Select)
	if !ok {
		return nil, fmt.Errorf("disallowed statement type %s: only SELECT statements are permitted", statementTypeName(stmt))
	}

	seen := make(map[string]bool)
	walkErr := sqlparser.Walk(func(node sqlparser.SQLNode) (bool, error) {
		if node == nil {
			return true, nil
		}
		if ate, ok := node.(*sqlparser.AliasedTableExpr); ok && ate != nil {
			if tn, ok := ate.Expr.(sqlparser.TableName); ok {
				name := strings.ToLower(tn.Name.String())
				if name != "" && name != "dual" && !seen[name] {
					seen[name] = true
					views = append(views, name)
				}
			}
		}
		return true, nil
	}, selectStmt)

	if walkErr != nil {
		return nil, walkErr
	}
	return views, nil
}
