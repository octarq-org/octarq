package plugin

import (
	"context"
	"reflect"
	"testing"
)

// These wrappers are the only way a plugin reaches request identity and scope
// data. Their defaults matter: an unset value must fall back to the least
// privileged answer, never to an elevated one.

func TestContextValues_UserAndScopeDefaults(t *testing.T) {
	ctx := context.Background()

	if got := UserIDFromContext(ctx); got != 0 {
		t.Fatalf("UserIDFromContext(unset) = %d, want 0", got)
	}
	if got := UserIDFromContext(WithUserID(ctx, 77)); got != 77 {
		t.Fatalf("UserIDFromContext = %d, want 77", got)
	}

	if got := ScopeKeyFromContext(ctx); got != DefaultWorkScope {
		t.Fatalf("ScopeKeyFromContext(unset) = %q, want %q", got, DefaultWorkScope)
	}
	if got := ScopeKeyFromContext(WithScopeKey(ctx, "personal:private")); got != "personal:private" {
		t.Fatalf("ScopeKeyFromContext = %q", got)
	}
	if got := ScopeKeyFromContext(WithScopeKey(ctx, "  ")); got != DefaultWorkScope {
		t.Fatalf("ScopeKeyFromContext(blank) = %q, want the work default", got)
	}

	if got := AllowedScopesFromContext(ctx); !reflect.DeepEqual(got, []string{ScopeWildcardWork}) {
		t.Fatalf("AllowedScopesFromContext(unset) = %v, want [work:*]", got)
	}
	if got := AllowedScopesFromContext(WithAllowedScopes(ctx, "personal:*")); !reflect.DeepEqual(got, []string{"personal:*"}) {
		t.Fatalf("AllowedScopesFromContext = %v", got)
	}
	if got := AllowedScopesFromContext(WithAllowedScopes(ctx, "  ", "")); !reflect.DeepEqual(got, []string{ScopeWildcardWork}) {
		t.Fatalf("AllowedScopesFromContext(blank entries) = %v, want [work:*]", got)
	}
}

func TestContextValues_PersonalScopeNeedsExplicitGrant(t *testing.T) {
	ctx := context.Background()

	if AllowsPersonalScope(ctx) {
		t.Fatal("a default context must not reach personal records")
	}
	if ScopeAllowed(ctx, "personal:private") {
		t.Fatal("a default context must not allow personal:private")
	}
	if !ScopeAllowed(ctx, "work:default") {
		t.Fatal("a default context must allow work scopes")
	}

	personal := WithScopeKey(ctx, "personal:private")
	if !AllowsPersonalScope(personal) {
		t.Fatal("selecting a personal scope must grant personal access")
	}

	granted := WithAllowedScopes(ctx, ScopeWildcardPersonal)
	if !AllowsPersonalScope(granted) {
		t.Fatal("an explicit personal:* grant must enable personal access")
	}
}

func TestContextValues_TokenAgentScope(t *testing.T) {
	ctx := context.Background()

	if IsAgentFromContext(ctx) {
		t.Fatal("an unset context must not read as an agent")
	}
	if got := AllowedToolsFromContext(ctx); got != nil {
		t.Fatalf("AllowedToolsFromContext(unset) = %v, want nil", got)
	}
	if got := RoleFromContext(ctx); got != "" {
		t.Fatalf("RoleFromContext(unset) = %q, want the least privileged role", got)
	}

	jsonAgent := WithTokenAgent(ctx, true, `["octarq_network__list_links", " octarq_infra__notify "]`)
	if !IsAgentFromContext(jsonAgent) {
		t.Fatal("WithTokenAgent did not record the agent flag")
	}
	want := []string{"octarq_network__list_links", "octarq_infra__notify"}
	if got := AllowedToolsFromContext(jsonAgent); !reflect.DeepEqual(got, want) {
		t.Fatalf("AllowedToolsFromContext(json) = %v, want %v", got, want)
	}

	sliceAgent := WithTokenAgent(ctx, true, []string{"a", "b"})
	if got := AllowedToolsFromContext(sliceAgent); !reflect.DeepEqual(got, []string{"a", "b"}) {
		t.Fatalf("AllowedToolsFromContext(slice) = %v", got)
	}

	// A malformed JSON array falls back to comma splitting rather than dropping
	// every entry, which would turn a broken allowlist into an empty one.
	broken := WithTokenAgent(ctx, true, `["a",`)
	if got := AllowedToolsFromContext(broken); !reflect.DeepEqual(got, []string{`["a"`}) {
		t.Fatalf("AllowedToolsFromContext(malformed) = %v, want the comma-split remainder", got)
	}

	empty := WithTokenAgent(ctx, true, "")
	if got := AllowedToolsFromContext(empty); got != nil {
		t.Fatalf("AllowedToolsFromContext(empty) = %v, want nil", got)
	}

	withRole := WithTokenRole(ctx, "member")
	if got := RoleFromContext(withRole); got != "member" {
		t.Fatalf("RoleFromContext = %q, want member", got)
	}
}

func TestContextValues_NilContextIsSafe(t *testing.T) {
	//nolint:staticcheck // a nil context must not panic the fail-closed readers.
	var ctx context.Context

	if IsAgentFromContext(ctx) {
		t.Fatal("nil context must not read as an agent")
	}
	if got := AllowedToolsFromContext(ctx); got != nil {
		t.Fatalf("AllowedToolsFromContext(nil) = %v, want nil", got)
	}
	if got := RoleFromContext(ctx); got != "" {
		t.Fatalf("RoleFromContext(nil) = %q, want empty", got)
	}
	if got := ScopeKeyFromContext(ctx); got != DefaultWorkScope {
		t.Fatalf("ScopeKeyFromContext(nil) = %q", got)
	}
	if got := AllowedScopesFromContext(ctx); !reflect.DeepEqual(got, []string{ScopeWildcardWork}) {
		t.Fatalf("AllowedScopesFromContext(nil) = %v", got)
	}
}
