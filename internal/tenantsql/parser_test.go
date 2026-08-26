package tenantsql

import (
	"strings"
	"testing"

	"github.com/xwb1989/sqlparser"
)

func TestParser_ValidQueries(t *testing.T) {
	p := NewParser()

	tests := []struct {
		name string
		sql  string
	}{
		{
			name: "single table simple SELECT",
			sql:  "SELECT id, name FROM tenant_users",
		},
		{
			name: "JOIN two tenant_* views",
			sql:  "SELECT u.id, o.total FROM tenant_users u JOIN tenant_orders o ON u.id = o.user_id",
		},
		{
			name: "SELECT with WHERE, GROUP BY, HAVING, ORDER BY, LIMIT, OFFSET",
			sql:  "SELECT category, count(id) AS cnt, sum(amount) AS total FROM tenant_sales WHERE status = 'completed' GROUP BY category HAVING count(id) > 5 ORDER BY total DESC LIMIT 10 OFFSET 20",
		},
		{
			name: "nested subquery on tenant views",
			sql:  "SELECT id, name FROM tenant_users WHERE id IN (SELECT user_id FROM tenant_orders WHERE total > 100)",
		},
		{
			name: "subquery in SELECT expression",
			sql:  "SELECT u.id, (SELECT count(o.id) FROM tenant_orders o WHERE o.user_id = u.id) AS order_cnt FROM tenant_users u",
		},
		{
			name: "all default allowed functions",
			sql:  "SELECT count(1), sum(amt), min(amt), max(amt), avg(amt), coalesce(notes, ''), lower(name), upper(name), length(name), date_trunc('month', created_at), strftime('%Y-%m', created_at) FROM tenant_records",
		},
		{
			name: "case-insensitive function names and table prefix",
			sql:  "SELECT COUNT(id), SUM(val), LOWER(name), UPPER(code) FROM TENANT_ACCOUNTS",
		},
		{
			name: "SELECT with comments",
			sql:  "SELECT id /* comment */ FROM tenant_users -- end of line comment\n WHERE status = 1",
		},
		{
			name: "SELECT constant without tables",
			sql:  "SELECT 1 + 1",
		},
		{
			name: "SELECT with trailing semicolon",
			sql:  "SELECT id, name FROM tenant_users;",
		},
		{
			name: "SELECT with trailing semicolon and whitespace",
			sql:  "SELECT id, name FROM tenant_users;   \n\t",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := p.Validate(tt.sql)
			if err != nil {
				t.Fatalf("expected valid SQL for %q, got error: %v", tt.sql, err)
			}
		})
	}
}

func TestParser_RejectedQueries(t *testing.T) {
	p := NewParser()

	tests := []struct {
		name        string
		sql         string
		errContains string
	}{
		{
			name:        "INSERT statement rejected",
			sql:         "INSERT INTO tenant_users (id, name) VALUES (1, 'alice')",
			errContains: "disallowed statement type",
		},
		{
			name:        "UPDATE statement rejected",
			sql:         "UPDATE tenant_users SET name = 'bob' WHERE id = 1",
			errContains: "disallowed statement type",
		},
		{
			name:        "DELETE statement rejected",
			sql:         "DELETE FROM tenant_users WHERE id = 1",
			errContains: "disallowed statement type",
		},
		{
			name:        "DROP TABLE rejected",
			sql:         "DROP TABLE tenant_users",
			errContains: "disallowed statement type",
		},
		{
			name:        "ALTER TABLE rejected",
			sql:         "ALTER TABLE tenant_users ADD COLUMN bio text",
			errContains: "disallowed statement type",
		},
		{
			name:        "base table direct query emails without tenant_ prefix",
			sql:         "SELECT * FROM emails",
			errContains: "disallowed table \"emails\"",
		},
		{
			name:        "base table direct query links without tenant_ prefix",
			sql:         "SELECT * FROM links",
			errContains: "disallowed table \"links\"",
		},
		{
			name:        "base table in JOIN",
			sql:         "SELECT u.id FROM tenant_users u JOIN emails e ON u.id = e.user_id",
			errContains: "disallowed table \"emails\"",
		},
		{
			name:        "base table in subquery WHERE",
			sql:         "SELECT * FROM tenant_users WHERE id IN (SELECT user_id FROM emails)",
			errContains: "disallowed table \"emails\"",
		},
		{
			name:        "base table in subquery SELECT expression",
			sql:         "SELECT u.id, (SELECT count(*) FROM secret_table) FROM tenant_users u",
			errContains: "disallowed table \"secret_table\"",
		},
		{
			name:        "unknown / dangerous function pg_sleep",
			sql:         "SELECT pg_sleep(1) FROM tenant_users",
			errContains: "disallowed function \"pg_sleep\"",
		},
		{
			name:        "unknown function load_extension",
			sql:         "SELECT load_extension('x') FROM tenant_users",
			errContains: "disallowed function \"load_extension\"",
		},
		{
			name:        "unknown function benchmark",
			sql:         "SELECT benchmark(1000000, 1) FROM tenant_users",
			errContains: "disallowed function \"benchmark\"",
		},
		{
			name:        "multi-statement injection: SELECT then DROP",
			sql:         "SELECT 1; DROP TABLE tenant_users",
			errContains: "sql parsing failed",
		},
		{
			name:        "multi-statement: multiple SELECTs",
			sql:         "SELECT * FROM tenant_users; SELECT * FROM tenant_orders",
			errContains: "sql parsing failed",
		},
		{
			name:        "comment obfuscation with unauthorized table",
			sql:         "SELECT * FROM emails /**/ WHERE 1=1",
			errContains: "disallowed table \"emails\"",
		},
		{
			name:        "empty string",
			sql:         "",
			errContains: "sql statement cannot be empty",
		},
		{
			name:        "whitespace only string",
			sql:         "   \t\n  \r\n",
			errContains: "sql statement cannot be empty",
		},
		{
			name:        "overlength SQL string exceeding 10KB",
			sql:         "SELECT * FROM tenant_users WHERE name = '" + strings.Repeat("A", 10241) + "'",
			errContains: "exceeds maximum allowed length",
		},
		{
			name:        "cross-database table qualification",
			sql:         "SELECT * FROM db.tenant_users",
			errContains: "cross-database queries are not permitted",
		},
		{
			name:        "cross-database backticked table qualification",
			sql:         "SELECT * FROM `db`.`tenant_users`",
			errContains: "cross-database queries are not permitted",
		},
		{
			name:        "cross-database column qualification",
			sql:         "SELECT db.tenant_users.id FROM tenant_users",
			errContains: "cross-database queries are not permitted",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := p.Validate(tt.sql)
			if err == nil {
				t.Fatalf("expected rejection for SQL %q, but got nil error", tt.sql)
				return
			}
			if !strings.Contains(err.Error(), tt.errContains) {
				t.Fatalf("expected error containing %q, got: %q", tt.errContains, err.Error())
			}
		})
	}
}

func TestParser_PanicIsolation(t *testing.T) {
	p := NewParser()

	// Complex CTE forms that are known risk areas for xwb1989/sqlparser
	panicCandidates := []struct {
		name string
		sql  string
	}{
		{
			name: "nested WITH + UNION + window function",
			sql:  "WITH cte1 AS (SELECT id, ROW_NUMBER() OVER (PARTITION BY dept ORDER BY salary DESC) as rank FROM tenant_employees), cte2 AS (SELECT * FROM cte1 WHERE rank = 1) SELECT * FROM cte1 UNION ALL SELECT * FROM cte2",
		},
		{
			name: "recursive CTE with multiple self-joins",
			sql:  "WITH RECURSIVE org_chart AS (SELECT id, manager_id, 1 as level FROM tenant_users WHERE manager_id IS NULL UNION ALL SELECT u.id, u.manager_id, o.level + 1 FROM tenant_users u JOIN org_chart o ON u.manager_id = o.id) SELECT * FROM org_chart",
		},
		{
			name: "deeply nested CTE with unions and subqueries",
			sql:  "WITH q1 AS (SELECT a, b, DENSE_RANK() OVER (ORDER BY a) FROM tenant_t1 UNION SELECT c, d, 0 FROM tenant_t2), q2 AS (WITH inner_q AS (SELECT * FROM q1) SELECT * FROM inner_q) SELECT * FROM q1 JOIN q2 ON q1.a = q2.c",
		},
	}

	for _, tc := range panicCandidates {
		t.Run(tc.name, func(t *testing.T) {
			defer func() {
				if r := recover(); r != nil {
					t.Fatalf("unexpected unhandled panic in Validate: %v", r)
				}
			}()

			err := p.Validate(tc.sql)
			// CTEs unsupported by xwb1989 must be safely rejected with an error without crashing
			if err == nil {
				t.Fatalf("expected error for unsupported complex CTE %q, got nil", tc.sql)
				return
			}
		})
	}
}

func TestParser_ConfigurableOptions(t *testing.T) {
	t.Run("custom view prefix", func(t *testing.T) {
		p := NewParser(WithViewPrefix("v_tenant_"))

		// Allowed with new prefix
		if err := p.Validate("SELECT * FROM v_tenant_users"); err != nil {
			t.Fatalf("expected valid with v_tenant_ prefix, got error: %v", err)
		}

		// Disallowed with old prefix
		if err := p.Validate("SELECT * FROM tenant_users"); err == nil {
			t.Fatalf("expected error for tenant_users when prefix is v_tenant_")
			return
		}
	})

	t.Run("custom allowed functions whitelist", func(t *testing.T) {
		p := NewParser(WithAllowedFunctions([]string{"count", "sum"}))

		// Allowed functions
		if err := p.Validate("SELECT count(id), sum(val) FROM tenant_users"); err != nil {
			t.Fatalf("expected valid for count/sum, got error: %v", err)
		}

		// Disallowed function
		if err := p.Validate("SELECT avg(val) FROM tenant_users"); err == nil {
			t.Fatalf("expected error for avg() when only count/sum are allowed")
			return
		}
	})

	t.Run("empty allowed functions whitelist rejects all functions", func(t *testing.T) {
		p := NewParser(WithAllowedFunctions([]string{}))

		// Queries without functions should succeed
		if err := p.Validate("SELECT id, name FROM tenant_users"); err != nil {
			t.Fatalf("expected valid for column selection, got: %v", err)
		}

		// Queries with any function should fail
		if err := p.Validate("SELECT count(id) FROM tenant_users"); err == nil {
			t.Fatalf("expected error for count() when function whitelist is empty")
			return
		}
	})
}

func TestParser_NoErrorRawSQLLeak(t *testing.T) {
	p := NewParser()

	injectionSecret := "SECRET_TOKEN_DO_NOT_LEAK_IN_RESPONSE_12345"
	sql := "SELECT * FROM emails WHERE password = '" + injectionSecret + "'"

	err := p.Validate(sql)
	if err == nil {
		t.Fatalf("expected error for emails query")
		return
	}

	errMsg := err.Error()
	if strings.Contains(errMsg, injectionSecret) {
		t.Fatalf("error message leaked sensitive SQL content: %s", errMsg)
	}
	if strings.Contains(errMsg, "WHERE password =") {
		t.Fatalf("error message leaked raw SQL clauses: %s", errMsg)
	}
	if !strings.Contains(errMsg, "emails") {
		t.Fatalf("error message should pinpoint offending table token: %s", errMsg)
	}
}

// ⑥ 默认函数白名单下，特殊表达式（group_concat/substr/convert/match/限定函数）全部拒绝。
func TestParser_SpecialFunctionsRejectedByDefault(t *testing.T) {
	p := NewParser()
	cases := []string{
		`SELECT group_concat(name) FROM tenant_links`,
		`SELECT substr(name, 1, 3) FROM tenant_links`,
		`SELECT substring(name FROM 1 FOR 3) FROM tenant_links`,
		`SELECT CONVERT(name, CHAR) FROM tenant_links`,
		`SELECT CONVERT(name USING utf8) FROM tenant_links`,
		`SELECT MATCH(title) AGAINST('x') FROM tenant_links`,
		`SELECT db.count(id) FROM tenant_links`,
	}
	for _, sql := range cases {
		if err := p.Validate(sql); err == nil {
			t.Errorf("expected rejection for %q", sql)
		}
	}
}

// ⑦ 显式扩白后同一批查询放行。
func TestParser_AllowedSpecialFunctionsViaOptions(t *testing.T) {
	p := NewParser(WithAllowedFunctions([]string{
		"count", "group_concat", "substr", "substring", "convert", "cast",
	}))
	cases := []string{
		`SELECT group_concat(name) FROM tenant_links`,
		`SELECT substr(name, 1, 3) FROM tenant_links`,
		`SELECT substring(name FROM 1 FOR 3) FROM tenant_links`,
		`SELECT CONVERT(name, CHAR) FROM tenant_links`,
		`SELECT CONVERT(name USING utf8) FROM tenant_links`,
	}
	for _, sql := range cases {
		if err := p.Validate(sql); err != nil {
			t.Errorf("expected %q to pass with extended whitelist, got %v", sql, err)
		}
	}
}

// ⑧ 非 SELECT 语句类型的拒绝文案覆盖各语句标签。
func TestParser_StatementTypeLabels(t *testing.T) {
	p := NewParser()
	cases := map[string]string{
		"SET @x = 1":                   "SET",
		"SHOW TABLES":                  "SHOW",
		"USE otherdb":                  "USE",
		"CREATE DATABASE foo":          "DBDDL",
		"CREATE TABLE t_copy (id int)": "DDL",
		"(SELECT 1) UNION (SELECT 2)":  "UNION",
	}
	for sql, label := range cases {
		err := p.Validate(sql)
		if err == nil {
			t.Errorf("expected rejection for %q", sql)
			continue
		}
		if !strings.Contains(err.Error(), label) {
			t.Errorf("expected label %q in error for %q, got: %v", label, sql, err)
		}
	}
}

// ⑨ 三段式列限定名（db.tbl.col）按跨库引用拒绝。
func TestParser_QualifiedColumnRejected(t *testing.T) {
	p := NewParser()
	err := p.Validate("SELECT mydb.tenant_users.id FROM tenant_users")
	if err == nil || !strings.Contains(err.Error(), "cross-database") {
		t.Fatalf("expected cross-database rejection, got %v", err)
	}
}

// ⑩ 尾随内容拒绝。
func TestParser_TrailingContentRejected(t *testing.T) {
	p := NewParser()
	if err := p.Validate("SELECT id FROM tenant_users WHERE 1=1 extra_here"); err == nil {
		t.Error("expected rejection for trailing content after statement")
	}
}

// ⑪ 注入必崩解析器，证明 recover 路径把 panic 转为普通解析失败。
func TestParseSafely_RecoversPanic(t *testing.T) {
	old := parseFunc
	parseFunc = func(string) (sqlparser.Statement, error) {
		panic("boom: injected parser failure")
	}
	defer func() { parseFunc = old }()

	p := NewParser()
	err := p.Validate("SELECT id FROM tenant_users")
	if err == nil || !strings.Contains(err.Error(), "sql parsing failed") {
		t.Fatalf("expected panic converted to parse failure, got %v", err)
	}
}
