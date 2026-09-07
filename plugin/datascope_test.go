package plugin

import (
	"context"
	"errors"
	"fmt"
	"testing"
)

type mockDataScopeFilter struct {
	scopeApplied bool
	lastUserID   string
	lastOrgID    string
}

func (m *mockDataScopeFilter) ApplyScope(ctx context.Context, query interface{}, userID string, orgID string) (interface{}, error) {
	m.scopeApplied = true
	m.lastUserID = userID
	m.lastOrgID = orgID
	if orgID == "error-org" {
		return nil, errors.New("mock scope error")
	}
	if s, ok := query.(string); ok {
		return fmt.Sprintf("%s WHERE org_id = '%s'", s, orgID), nil
	}
	return query, nil
}

type customProDataScopeFilter struct {
	deptID string
}

func (c *customProDataScopeFilter) ApplyScope(ctx context.Context, query interface{}, userID string, orgID string) (interface{}, error) {
	if s, ok := query.(string); ok {
		return fmt.Sprintf("%s WHERE org_id = '%s' AND dept_id = '%s'", s, orgID, c.deptID), nil
	}
	return query, nil
}

func TestGetDataScopeFilter_NilContext(t *testing.T) {
	filter, ok := GetDataScopeFilter(nil)
	if ok || filter != nil {
		t.Fatalf("expected (nil, false) for nil context, got (%v, %v)", filter, ok)
	}
}

func TestGetDataScopeFilter_Unregistered(t *testing.T) {
	reg := NewRegistry()
	ctx := &Context{
		Lookup: reg.Lookup,
	}
	filter, ok := GetDataScopeFilter(ctx)
	if ok || filter != nil {
		t.Fatalf("expected (nil, false) when unregistered, got (%v, %v)", filter, ok)
	}
}

func TestGetDataScopeFilter_TypeMismatch(t *testing.T) {
	reg := NewRegistry()
	reg.Provide(ServiceDataScope, "not-a-filter")
	ctx := &Context{
		Lookup: reg.Lookup,
	}
	filter, ok := GetDataScopeFilter(ctx)
	if ok || filter != nil {
		t.Fatalf("expected (nil, false) for mismatched type, got (%v, %v)", filter, ok)
	}
}

func TestGetDataScopeFilter_RegisteredAndOverride(t *testing.T) {
	reg := NewRegistry()
	mockFilter := &mockDataScopeFilter{}
	reg.Provide(ServiceDataScope, DataScopeFilter(mockFilter))

	ctx := &Context{
		Provide: reg.Provide,
		Lookup:  reg.Lookup,
	}

	filter, ok := GetDataScopeFilter(ctx)
	if !ok || filter == nil {
		t.Fatalf("expected filter to be resolved, got (%v, %v)", filter, ok)
	}

	out, err := filter.ApplyScope(context.Background(), "SELECT * FROM links", "user-1", "org-42")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := "SELECT * FROM links WHERE org_id = 'org-42'"
	if out != want {
		t.Fatalf("got %v, want %v", out, want)
	}
	if !mockFilter.scopeApplied || mockFilter.lastUserID != "user-1" || mockFilter.lastOrgID != "org-42" {
		t.Fatalf("filter was not invoked with expected parameters: %+v", mockFilter)
	}

	// Test error propagation
	_, err = filter.ApplyScope(context.Background(), "SELECT 1", "user-1", "error-org")
	if err == nil {
		t.Fatal("expected error from filter.ApplyScope, got nil")
	}

	// Test Pro custom filter override pattern (simulating a separate registry or Pro override)
	proReg := NewRegistry()
	proFilter := &customProDataScopeFilter{deptID: "dept-99"}
	proReg.Provide(ServiceDataScope, DataScopeFilter(proFilter))
	proCtx := &Context{
		Provide: proReg.Provide,
		Lookup:  proReg.Lookup,
	}

	resolvedPro, ok := GetDataScopeFilter(proCtx)
	if !ok || resolvedPro == nil {
		t.Fatalf("expected Pro filter to be resolved, got (%v, %v)", resolvedPro, ok)
	}
	proOut, err := resolvedPro.ApplyScope(context.Background(), "SELECT * FROM links", "user-1", "org-42")
	if err != nil {
		t.Fatalf("unexpected Pro error: %v", err)
	}
	wantPro := "SELECT * FROM links WHERE org_id = 'org-42' AND dept_id = 'dept-99'"
	if proOut != wantPro {
		t.Fatalf("got %v, want %v", proOut, wantPro)
	}
}
