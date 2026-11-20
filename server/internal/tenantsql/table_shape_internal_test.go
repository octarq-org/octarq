package tenantsql

import (
	"testing"

	"github.com/xwb1989/sqlparser"
)

func TestRejectUnknownTableExprReachable(t *testing.T) {
	if err := rejectUnknownTableExpr(nil); err != nil {
		t.Fatalf("nil expr must pass through, got: %v", err)
	}
	if err := rejectUnknownTableExpr(&sqlparser.Subquery{}); err != nil {
		t.Fatalf("derived subquery must be allowed, got: %v", err)
	}
	tn := sqlparser.TableName{Name: sqlparser.NewTableIdent("tenant_users")}
	if err := rejectUnknownTableExpr(tn); err != nil {
		t.Fatalf("plain table name must be allowed, got: %v", err)
	}
}
