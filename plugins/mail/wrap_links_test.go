package mail

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/glebarez/sqlite"
	"github.com/octarq-org/octarq/internal/mail"
	"github.com/octarq-org/octarq/internal/models"
	"gorm.io/gorm"
)

func TestWrapLinksInEmail(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(models.AllModels()...); err != nil {
		t.Fatal(err)
	}

	p := New()
	p.db = db
	p.orgID = func(r *http.Request) uint { return 1 }
	p.getWorkspaceSetting = func(orgID uint, key string) string { return "" }

	// Set up custom link domain
	db.Create(&models.Domain{
		OrgID:   1,
		Name:    "short.mycorp.com",
		ForLink: true,
		LinkHosts: models.HostList{
			{Host: "short.mycorp.com", Enabled: true},
		},
	})

	msg := mail.Message{
		Text: "Hello, visit our site at https://google.com for info. Also visit http://example.com/some/path.",
		HTML: `<p>Hello, visit <a href="https://google.com">Google</a> and <a href="http://example.com/some/path">Example</a>.</p>`,
	}

	req := httptest.NewRequest(http.MethodPost, "/api/emails/send", nil)
	req.Host = "dashboard.mycorp.com"

	p.wrapLinksInEmail(req, &msg)

	// Assertions on text
	if !strings.Contains(msg.Text, "http://short.mycorp.com/") {
		t.Errorf("Text links not wrapped correctly: %q", msg.Text)
	}
	if strings.Contains(msg.Text, "https://google.com") {
		t.Errorf("Original Google URL still present in Text: %q", msg.Text)
	}

	// Assertions on HTML
	if !strings.Contains(msg.HTML, "http://short.mycorp.com/") {
		t.Errorf("HTML links not wrapped correctly: %q", msg.HTML)
	}
	if strings.Contains(msg.HTML, "https://google.com") {
		t.Errorf("Original Google URL still present in HTML: %q", msg.HTML)
	}

	// Check that links are stored in DB
	var links []models.Link
	db.Find(&links)
	if len(links) != 2 {
		t.Errorf("expected 2 links generated in DB, got %d", len(links))
	}

	// Verify targets are correct
	targets := map[string]bool{
		"https://google.com":           true,
		"http://example.com/some/path": true,
	}
	for _, l := range links {
		if !targets[l.Target] {
			t.Errorf("unexpected target URL stored: %q", l.Target)
		}
		if l.Host != "" {
			t.Errorf("expected host-agnostic link (Host = ''), got %q", l.Host)
		}
	}
}

func TestWrapLinksAvoidDoubleWrapAndInternal(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	db.AutoMigrate(models.AllModels()...)

	p := New()
	p.db = db
	p.orgID = func(r *http.Request) uint { return 1 }
	p.getWorkspaceSetting = func(orgID uint, key string) string { return "" }

	db.Create(&models.Domain{
		OrgID:   1,
		Name:    "avoid.mycorp.com",
		ForLink: true,
		LinkHosts: models.HostList{
			{Host: "avoid.mycorp.com", Enabled: true},
		},
	})

	msg := mail.Message{
		Text: "Internal: http://localhost:8680/info. Already wrapped: http://avoid.mycorp.com/abc123. Normal: https://google.com",
	}

	req := httptest.NewRequest(http.MethodPost, "/api/emails/send", nil)
	req.Host = "avoid.mycorp.com"

	p.wrapLinksInEmail(req, &msg)

	if !strings.Contains(msg.Text, "http://localhost:8680/info") {
		t.Error("localhost link should not be wrapped")
	}
	if !strings.Contains(msg.Text, "http://avoid.mycorp.com/abc123") {
		t.Error("already wrapped short link should not be double wrapped")
	}
	if !strings.Contains(msg.Text, "http://avoid.mycorp.com/") || strings.Contains(msg.Text, "https://google.com") {
		t.Error("normal link should have been wrapped")
	}
}
