package tenantquery_test

import (
	"context"
	"strings"
	"testing"

	"github.com/octarq-org/octarq/server/internal/tenantsql"
	"github.com/octarq-org/octarq/server/plugin"
	"github.com/octarq-org/octarq/server/plugins/tenantquery"
	"gorm.io/gorm"
)

type scopedCommitmentRow struct {
	ID       uint   `gorm:"primaryKey"`
	OrgID    uint   `gorm:"column:org_id;index"`
	ScopeKey string `gorm:"column:scope_key;size:64;index"`
	Title    string `gorm:"column:title"`
	Secret   string `gorm:"column:secret"`
}

func (scopedCommitmentRow) TableName() string {
	return "test_scoped_commitments"
}

func TestTenantQuery_ScopeIsolation_PersonalInvisibleToWork(t *testing.T) {
	db, engine := setupTestApp(t)

	if err := db.AutoMigrate(&scopedCommitmentRow{}); err != nil {
		t.Fatalf("migrate scopedCommitmentRow: %v", err)
	}

	view := plugin.TenantView{
		Name: "tenant_scoped_commitments",
		Columns: []plugin.TenantColumn{
			{Name: "id", Type: "integer", Description: "Primary key"},
			{Name: "org_id", Type: "integer", Description: "Organization ID"},
			{Name: "scope_key", Type: "text", Description: "Sovereignty scope key"},
			{Name: "title", Type: "text", Description: "Commitment title"},
			{Name: "secret", Type: "text", Description: "Sensitive secret"},
		},
		Sensitive: []string{"secret"},
		Definition: func(orgID uint) string {
			return db.ToSQL(func(tx *gorm.DB) *gorm.DB {
				return tx.Select("id, org_id, scope_key, title, secret").
					Table("test_scoped_commitments").
					Where("org_id = ?", orgID).
					Find(nil)
			})
		},
	}
	if err := tenantsql.DefaultRegistry().Register(view); err != nil && !strings.Contains(err.Error(), "already registered") {
		t.Fatalf("register tenant_scoped_commitments: %v", err)
	}

	// Seed work and personal rows for Org 1, plus a row for Org 2
	seed := []scopedCommitmentRow{
		{OrgID: 1, ScopeKey: "work:default", Title: "Ship Q4 Release", Secret: "work-sec-1"},
		{OrgID: 1, ScopeKey: "work:ops", Title: "Rotate DNS Keys", Secret: "work-sec-2"},
		{OrgID: 1, ScopeKey: "personal:private", Title: "Annual Health Checkup", Secret: "pers-sec-1"},
		{OrgID: 1, ScopeKey: "personal:family", Title: "Buy Anniversary Gift", Secret: "pers-sec-2"},
		{OrgID: 2, ScopeKey: "work:default", Title: "Other Org Task", Secret: "other-sec"},
	}
	for i := range seed {
		if err := db.Create(&seed[i]).Error; err != nil {
			t.Fatalf("seed row %d: %v", i, err)
		}
	}

	var queryEP plugin.Endpoint
	for _, ep := range engine.Endpoints() {
		if ep.EndpointName() == "query_tenant_sql" {
			queryEP = ep
			break
		}
	}
	if queryEP == nil {
		t.Fatal("query_tenant_sql endpoint not found")
	}

	// 1. Default un-elevated context (defaults to work:* only)
	defaultCtx := plugin.WithOrgID(context.Background(), 1)
	if got := plugin.ScopeKeyFromContext(defaultCtx); got != "work:default" {
		t.Fatalf("ScopeKeyFromContext(defaultCtx) = %q, want work:default", got)
	}
	if scopes := plugin.AllowedScopesFromContext(defaultCtx); len(scopes) != 1 || scopes[0] != "work:*" {
		t.Fatalf("AllowedScopesFromContext(defaultCtx) = %v, want [work:*]", scopes)
	}

	res, err := queryEP.Execute(defaultCtx, tenantquery.QueryTenantSQLInput{
		SQL: "SELECT id, scope_key, title, secret FROM tenant_scoped_commitments ORDER BY id ASC",
	})
	if err != nil {
		t.Fatalf("work-scoped query failed: %v", err)
	}
	out := res.(*tenantquery.QueryTenantSQLOutput)
	if out.RowCount != 2 {
		t.Fatalf("expected 2 work rows for default work:* context, got %d: %+v", out.RowCount, out.Rows)
	}
	for _, r := range out.Rows {
		sk := r["scope_key"].(string)
		if !strings.HasPrefix(sk, "work:") {
			t.Fatalf("personal row leaked into work:* query: %+v", r)
		}
		if r["secret"] != plugin.RedactedValue {
			t.Fatalf("expected redacted secret, got %v", r["secret"])
		}
	}

	// 2. Explicit attempt by team member to query personal:private via WHERE or aggregates (COUNT) must return 0 rows
	bypassRes, err := queryEP.Execute(defaultCtx, tenantquery.QueryTenantSQLInput{
		SQL: "SELECT COUNT(*) AS cnt FROM tenant_scoped_commitments WHERE scope_key LIKE 'personal:%'",
	})
	if err != nil {
		t.Fatalf("bypass count query failed: %v", err)
	}
	bypassOut := bypassRes.(*tenantquery.QueryTenantSQLOutput)
	if len(bypassOut.Rows) != 1 {
		t.Fatalf("expected 1 count row, got %d", len(bypassOut.Rows))
	}
	cntVal := bypassOut.Rows[0]["cnt"]
	if cntVal != int64(0) && cntVal != 0 && cntVal != "0" {
		t.Fatalf("expected 0 personal rows visible in COUNT(*), got %v", cntVal)
	}

	// 3. Explicit work:default ScopeKey still blocks personal:* rows
	workCtx := plugin.WithScopeKey(plugin.WithOrgID(context.Background(), 1), "work:default")
	workRes, err := queryEP.Execute(workCtx, tenantquery.QueryTenantSQLInput{
		SQL: "SELECT scope_key, title FROM tenant_scoped_commitments",
	})
	if err != nil {
		t.Fatalf("explicit work:default query failed: %v", err)
	}
	workOut := workRes.(*tenantquery.QueryTenantSQLOutput)
	if workOut.RowCount != 2 {
		t.Fatalf("expected 2 rows under work:default, got %d", workOut.RowCount)
	}

	// 4. Owner-authenticated session with personal:private (or AllowedScopes including personal:*) unlocks personal:* rows
	ownerCtx := plugin.WithScopeKey(plugin.WithOrgID(context.Background(), 1), "personal:private")
	if !plugin.AllowsPersonalScope(ownerCtx) {
		t.Fatal("expected AllowsPersonalScope(ownerCtx) = true")
	}
	ownerRes, err := queryEP.Execute(ownerCtx, tenantquery.QueryTenantSQLInput{
		SQL: "SELECT scope_key, title FROM tenant_scoped_commitments ORDER BY id ASC",
	})
	if err != nil {
		t.Fatalf("owner personal query failed: %v", err)
	}
	ownerOut := ownerRes.(*tenantquery.QueryTenantSQLOutput)
	if ownerOut.RowCount != 4 {
		t.Fatalf("expected all 4 org-1 rows (work + personal) for owner session, got %d", ownerOut.RowCount)
	}
}
