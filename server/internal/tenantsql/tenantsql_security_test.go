package tenantsql_test

import (
	"testing"

	"github.com/octarq-org/octarq/server/internal/tenantsql"
	"github.com/stretchr/testify/assert"
)

func TestParserSecurity_AttackVectors(t *testing.T) {
	parser := tenantsql.NewParser(
		tenantsql.WithAllowedViews([]string{"tenant_users", "tenant_links"}),
	)

	tests := []struct {
		name    string
		sql     string
		blocked bool // expected to be blocked (fail validation)
	}{
		{
			name:    "UNION attack",
			sql:     "SELECT * FROM tenant_users UNION SELECT * FROM secret_table",
			blocked: true,
		},
		{
			name:    "PRAGMA attack",
			sql:     "PRAGMA table_info(users)",
			blocked: true,
		},
		{
			name:    "ATTACH DATABASE attack",
			sql:     "ATTACH DATABASE '/tmp/evil.db' AS evil",
			blocked: true,
		},
		{
			name:    "DETACH DATABASE attack",
			sql:     "DETACH DATABASE evil",
			blocked: true,
		},
		{
			name:    "Subquery bypassing prefix",
			sql:     "SELECT * FROM (SELECT * FROM secret_table) sub",
			blocked: true,
		},
		{
			name:    "Subquery in WHERE clause",
			sql:     "SELECT * FROM tenant_users WHERE id IN (SELECT id FROM secret_table)",
			blocked: true,
		},
		{
			name:    "Subquery in SELECT clause",
			sql:     "SELECT (SELECT secret FROM secret_table) FROM tenant_users",
			blocked: true,
		},
		{
			name:    "CTE bypassing prefix",
			sql:     "WITH cte AS (SELECT * FROM secret_table) SELECT * FROM cte",
			blocked: true,
		},
		{
			name:    "Subquery with valid prefix",
			sql:     "SELECT * FROM (SELECT * FROM tenant_users) sub",
			blocked: false,
		},
		{
			name:    "INSERT INTO",
			sql:     "INSERT INTO tenant_users (id) VALUES (1)",
			blocked: true,
		},
		{
			name:    "UPDATE",
			sql:     "UPDATE tenant_users SET name = 'evil'",
			blocked: true,
		},
		{
			name:    "DELETE",
			sql:     "DELETE FROM tenant_users",
			blocked: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := parser.Validate(tt.sql)
			if tt.blocked {
				assert.Error(t, err, "Expected query to be blocked: %s", tt.sql)
			} else {
				assert.NoError(t, err, "Expected query to be allowed: %s", tt.sql)
			}
		})
	}
}
