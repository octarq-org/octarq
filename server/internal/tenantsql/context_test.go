package tenantsql

import (
	"context"
	"testing"
)

func TestContextOrgAndUser(t *testing.T) {
	var nilCtx context.Context
	if OrgIDFromContext(nilCtx) != 0 {
		t.Fatal("expected 0 for nil context")
	}
	if UserIDFromContext(nilCtx) != 0 {
		t.Fatal("expected 0 for nil context")
	}

	ctx := context.Background()
	if OrgIDFromContext(ctx) != 0 {
		t.Fatal("expected 0 for empty context")
	}
	if UserIDFromContext(ctx) != 0 {
		t.Fatal("expected 0 for empty context")
	}

	ctx = WithOrgID(ctx, 42)
	ctx = WithUserID(ctx, 108)

	if got := OrgIDFromContext(ctx); got != 42 {
		t.Fatalf("expected OrgID 42, got %d", got)
	}
	if got := UserIDFromContext(ctx); got != 108 {
		t.Fatalf("expected UserID 108, got %d", got)
	}
}

func TestContextScopeKeyAndAllowedScopes(t *testing.T) {
	var nilCtx context.Context
	// Nil context handling
	if got := ScopeKeyFromContext(nilCtx); got != DefaultWorkScope {
		t.Fatalf("expected DefaultWorkScope for nil ctx, got %q", got)
	}
	if got := AllowedScopesFromContext(nilCtx); len(got) != 1 || got[0] != ScopeWildcardWork {
		t.Fatalf("expected [work:*] for nil ctx, got %v", got)
	}
	if AllowsPersonalScope(nilCtx) {
		t.Fatal("nil ctx should not allow personal scope")
	}

	// Empty string default
	ctx := WithScopeKey(nilCtx, "")
	if got := ScopeKeyFromContext(ctx); got != DefaultWorkScope {
		t.Fatalf("expected DefaultWorkScope, got %q", got)
	}
	if AllowsPersonalScope(ctx) {
		t.Fatal("default work scope should not allow personal scope")
	}

	// Personal scope binding automatically elevates AllowedScopes if not explicit
	ctxPersonal := WithScopeKey(context.Background(), PersonalPrivateScope)
	if got := ScopeKeyFromContext(ctxPersonal); got != PersonalPrivateScope {
		t.Fatalf("expected %q, got %q", PersonalPrivateScope, got)
	}
	if !AllowsPersonalScope(ctxPersonal) {
		t.Fatal("personal scope key should automatically allow personal scope")
	}
	scopes := AllowedScopesFromContext(ctxPersonal)
	if len(scopes) != 2 || scopes[0] != ScopeWildcardWork || scopes[1] != ScopeWildcardPerson {
		t.Fatalf("expected [work:* personal:*], got %v", scopes)
	}

	// Explicit allowed scopes
	ctxExplicit := WithAllowedScopes(context.Background(), "work:custom", " ")
	expScopes := AllowedScopesFromContext(ctxExplicit)
	if len(expScopes) != 1 || expScopes[0] != "work:custom" {
		t.Fatalf("expected [work:custom], got %v", expScopes)
	}

	// Empty scopes fallback
	ctxEmptyScopes := WithAllowedScopes(nilCtx)
	if got := AllowedScopesFromContext(ctxEmptyScopes); len(got) != 1 || got[0] != ScopeWildcardWork {
		t.Fatalf("expected [work:*] fallback, got %v", got)
	}

	// Wildcard matching and ScopeAllowed
	if !MatchScopePattern("*", "any:thing") {
		t.Fatal("expected * to match everything")
	}
	if !MatchScopePattern("work:*", "work:project-a") {
		t.Fatal("expected work:* to match work:project-a")
	}
	if MatchScopePattern("work:*", "personal:private") {
		t.Fatal("work:* should not match personal:private")
	}
	if !MatchScopePattern("exact:match", "exact:match") {
		t.Fatal("exact:match should match")
	}
	if MatchScopePattern("exact:match", "other:scope") {
		t.Fatal("exact:match should not match other:scope")
	}
	if MatchScopePattern("", "") {
		t.Fatal("empty pattern should not match")
	}

	// ScopeAllowed with context
	ctxAllowed := WithAllowedScopes(context.Background(), "work:*", "personal:vault")
	if !ScopeAllowed(ctxAllowed, "work:marketing") {
		t.Fatal("work:marketing should be allowed")
	}
	if !ScopeAllowed(ctxAllowed, "personal:vault") {
		t.Fatal("personal:vault should be allowed")
	}
	if ScopeAllowed(ctxAllowed, "personal:health") {
		t.Fatal("personal:health should NOT be allowed")
	}

	// AllowsPersonalScope variations
	ctxWildcardAll := WithAllowedScopes(context.Background(), "*")
	if !AllowsPersonalScope(ctxWildcardAll) {
		t.Fatal("* should allow personal scope")
	}
	ctxWildcardExact := WithAllowedScopes(context.Background(), "personal:secrets")
	if !AllowsPersonalScope(ctxWildcardExact) {
		t.Fatal("personal:secrets should allow personal scope")
	}
}
