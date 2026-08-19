package links

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
)

// TestListLinksClampsOverLimitToMax is the endpoint-level guard for the
// pagination bug: ?limit=1000 against a max of 500 must return 500 rows, not
// the default 50. Returning 50 there looks to a paginating client exactly like
// the collection ending, so the remaining rows are never fetched.
func TestListLinksClampsOverLimitToMax(t *testing.T) {
	p, mkCtx := setupFullLinksTestDB(t)

	const rows = 520
	batch := make([]Link, 0, rows)
	for i := 0; i < rows; i++ {
		batch = append(batch, Link{OrgID: 1, Slug: fmt.Sprintf("s%04d", i), Target: "https://example.com"})
	}
	if err := p.db.CreateInBatches(&batch, 100).Error; err != nil {
		t.Fatalf("seed links: %v", err)
	}

	list := func(limit int) int {
		t.Helper()
		r := httptest.NewRequest(http.MethodGet, "/api/links", nil)
		out, err := p.listLinks(context.Background(), &ListLinksInput{Ctx: mkCtx(r), Limit: limit})
		if err != nil {
			t.Fatalf("listLinks(limit=%d): %v", limit, err)
		}
		return len(out.Body)
	}

	if got := list(0); got != 50 {
		t.Fatalf("no limit returned %d rows, want the default 50", got)
	}
	if got := list(1000); got != 500 {
		t.Fatalf("limit=1000 returned %d rows, want the max 500 (falling back to the default is silent data loss to a paginating client)", got)
	}
	if got := list(501); got != 500 {
		t.Fatalf("limit=501 returned %d rows, want the max 500", got)
	}
	if got := list(200); got != 200 {
		t.Fatalf("limit=200 returned %d rows, want 200", got)
	}
}
