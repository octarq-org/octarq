package mail

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/octarq-org/octarq/internal/models"
)

func TestParseListUnsubscribeHeader(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		hdr  string
		want string
	}{
		{
			name: "single http url in brackets",
			hdr:  "<https://example.com/unsubscribe?id=123>",
			want: "https://example.com/unsubscribe?id=123",
		},
		{
			name: "http and mailto in brackets prefers http",
			hdr:  "<https://example.com/unsub>, <mailto:unsub@example.com?subject=unsub>",
			want: "https://example.com/unsub",
		},
		{
			name: "mailto only",
			hdr:  "<mailto:unsubscribe@example.com?subject=unsub_user123>",
			want: "mailto:unsubscribe@example.com?subject=unsub_user123",
		},
		{
			name: "raw url without brackets",
			hdr:  "https://newsletter.example.com/optout?email=test@example.com",
			want: "https://newsletter.example.com/optout?email=test@example.com",
		},
		{
			name: "empty header",
			hdr:  "",
			want: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ParseListUnsubscribeHeader(tt.hdr)
			if got != tt.want {
				t.Fatalf("ParseListUnsubscribeHeader(%q) = %q, want %q", tt.hdr, got, tt.want)
			}
		})
	}
}

func TestExtractUnsubscribeURLFromRawMIME(t *testing.T) {
	t.Parallel()

	rawMIME := "From: newsletter@promo.com\r\n" +
		"To: user@example.com\r\n" +
		"Subject: Weekly Deals\r\n" +
		"List-Unsubscribe: <https://promo.com/unsub/token456>, <mailto:unsub@promo.com>\r\n" +
		"List-Unsubscribe-Post: List-Unsubscribe=One-Click\r\n" +
		"\r\n" +
		"Check out our weekly specials!"

	got := ExtractUnsubscribeURL([]byte(rawMIME))
	want := "https://promo.com/unsub/token456"
	if got != want {
		t.Fatalf("ExtractUnsubscribeURL() = %q, want %q", got, want)
	}
}

func TestInboundProcessesUnsubscribeURL(t *testing.T) {
	t.Parallel()

	p, mkCtx := setupFullMailTestDB(t)
	ctx := context.Background()

	org := models.Org{Name: "Unsub Org", Slug: "unsub-org", InboundToken: "tok-unsub-123"}
	p.db.Create(&org)

	mb := Mailbox{OrgID: org.ID, Address: "inbox@unsub.example.com", Enabled: true}
	p.db.Create(&mb)

	raw := "From: Sales Bot <bot@vendor.com>\r\n" +
		"To: inbox@unsub.example.com\r\n" +
		"Subject: Product Update\r\n" +
		"Date: " + time.Now().Format(time.RFC1123Z) + "\r\n" +
		"List-Unsubscribe: <https://vendor.com/opt-out/999>\r\n" +
		"\r\n" +
		"Hello world"

	inReq := httptest.NewRequest(http.MethodPost, "/api/webhook/unsub-org/email/inbound/tok-unsub-123", strings.NewReader(raw))
	input := &InboundInput{Ctx: mkCtx(inReq), OrgSlug: org.Slug, Token: org.InboundToken}

	out, err := p.inbound(ctx, input)
	if err != nil {
		t.Fatalf("inbound failed: %v", err)
	}
	emailIDVal, ok := out.Body["id"]
	if !ok {
		t.Fatalf("expected email id in response, got %+v", out.Body)
	}
	emailID := emailIDVal.(uint)

	var email Email
	if err := p.db.First(&email, emailID).Error; err != nil {
		t.Fatalf("failed to query stored email: %v", err)
	}
	if email.UnsubscribeURL != "https://vendor.com/opt-out/999" {
		t.Fatalf("stored email UnsubscribeURL = %q, want https://vendor.com/opt-out/999", email.UnsubscribeURL)
	}
	if email.Folder != "inbox" {
		t.Fatalf("stored email Folder = %q, want inbox", email.Folder)
	}
}
