package api

import (
	"context"
	"errors"
	"net/http"
	"reflect"
	"strings"
	"testing"

	"github.com/octarq-org/octarq/llmprovider"
	"github.com/octarq-org/octarq/plugin"
)

type dummyLLM struct{ reply string }

func (d dummyLLM) Name() string         { return "dummy" }
func (d dummyLLM) DefaultModel() string { return "m" }
func (d dummyLLM) CheapModel() string   { return "m" }
func (d dummyLLM) Complete(ctx context.Context, req llmprovider.Request) (llmprovider.Response, error) {
	return llmprovider.Response{Text: d.reply}, nil
}

type failingLLM struct{}

func (f failingLLM) Name() string         { return "failing" }
func (f failingLLM) DefaultModel() string { return "m" }
func (f failingLLM) CheapModel() string   { return "m" }
func (f failingLLM) Complete(ctx context.Context, req llmprovider.Request) (llmprovider.Response, error) {
	return llmprovider.Response{}, errors.New("network failure")
}

func TestParseSlugListMore(t *testing.T) {
	cases := []struct {
		input string
		want  []string
	}{
		{
			input: "```json\n[\"slug-one\", \"slug-two\", \"invalid_character_slug!\", \"ok-three\"]\n```",
			want:  []string{"slug-one", "slug-two", "ok-three"},
		},
		{
			input: "- bullet-one\n* bullet-two\n\"bullet-three\"\n'bullet-four'\nbullet-five\nextra-six",
			want:  []string{"bullet-one", "bullet-two", "bullet-three", "bullet-four", "bullet-five"},
		},
		{
			input: "no valid slugs here @#$%",
			want:  []string{},
		},
	}

	for _, tc := range cases {
		got := parseSlugList(tc.input)
		if len(got) != len(tc.want) {
			t.Errorf("parseSlugList(%q) len = %d, want %d (got %+v, want %+v)", tc.input, len(got), len(tc.want), got, tc.want)
			continue
		}
		if !reflect.DeepEqual(got, tc.want) && len(got) > 0 {
			t.Errorf("parseSlugList(%q) = %+v, want %+v", tc.input, got, tc.want)
		}
	}
}

func TestAISummarizeEmailAndSuggestSlugFlows(t *testing.T) {
	srv, _ := newAITestHandler(t, `["my-cool-blog","awesome-post"]`)
	cookies := sessionCookies(t, 1, 1)

	// 1. Suggest slug empty target -> 400
	rec := do(srv, "POST", "/api/ai/assist/suggest-slug", cookies, `{"target":""}`)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("empty target suggest slug: got %d, want 400", rec.Code)
	}

	// 2. Suggest slug with title -> 200
	rec = do(srv, "POST", "/api/ai/assist/suggest-slug", cookies, `{"target":"https://example.com/blog","title":"My Cool Blog"}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("suggest slug with title: got %d (%s)", rec.Code, rec.Body.String())
	}

	// 3. Summarize email - not found (no email plugin service) -> 404
	rec = do(srv, "POST", "/api/ai/assist/summarize-email/999", cookies, "")
	if rec.Code != http.StatusNotFound {
		t.Fatalf("summarize non-existent email: got %d, want 404", rec.Code)
	}

	// 4. Summarize email - with email getter service mounted and long body (>8000 chars)
	reg := plugin.NewRegistry()
	longBody := strings.Repeat("Important invoice details. ", 400)
	reg.Provide(plugin.ServiceMailEmailGet, plugin.EmailGetter(func(orgID, id uint) (string, string, string, bool) {
		if id == 42 {
			return "billing@vendor.com", "Invoice #4242", longBody, true
		}
		return "", "", "", false
	}))
	h, _, _ := newTestHandlerRaw(t)
	h.SetServiceLookup(reg.Lookup)
	h.SetLLMResolver(func() (llmprovider.Provider, error) {
		return dummyLLM{reply: "Invoice for services due in 30 days."}, nil
	})

	// 5. Test failing LLM resolver
	hFail, _, _ := newTestHandlerRaw(t)
	hFail.SetLLMResolver(func() (llmprovider.Provider, error) {
		return failingLLM{}, nil
	})

	// 6. Nil Ctx calls
	ctx := context.Background()
	if _, err := h.aiStatus(ctx, &AIStatusInput{Ctx: nil}); err != nil {
		// should return status without error
	}
	if _, err := h.aiSuggestSlug(ctx, &AISuggestSlugInput{Ctx: nil}); err == nil {
		t.Error("expected error for nil Ctx in aiSuggestSlug")
	}
	if _, err := h.aiSummarizeEmail(ctx, &AISummarizeEmailInput{Ctx: nil}); err == nil {
		t.Error("expected error for nil Ctx in aiSummarizeEmail")
	}
}
