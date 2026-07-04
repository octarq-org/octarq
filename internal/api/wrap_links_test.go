package api

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Jungley8/led/config"
	"github.com/Jungley8/led/internal/auth"
	"github.com/Jungley8/led/internal/crypto"
	"github.com/Jungley8/led/internal/geo"
	"github.com/Jungley8/led/internal/mail"
	"github.com/Jungley8/led/internal/models"
	"github.com/Jungley8/led/internal/queue"
	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

func TestWrapLinksInEmail(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file::memory:?cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(models.AllModels()...); err != nil {
		t.Fatal(err)
	}
	db.Where("1 = 1").Delete(&models.Domain{})
	db.Where("1 = 1").Delete(&models.Link{})

	cfg := &config.Config{SecretKey: "secret"}
	cipher := crypto.New(cfg.SecretKey)
	authMgr := auth.New(cfg, cipher).WithDB(db)
	g, _ := geo.Open("")
	h := New(cfg, db, cipher, authMgr, g, queue.New(""))

	// Set up custom link domain
	db.Create(&models.Domain{
		OrgID: 1,
		Name:  "short.mycorp.com",
		LinkHosts: []models.Host{
			{Host: "short.mycorp.com", Enabled: true},
		},
		ForLink: true,
	})

	msg := mail.Message{
		Text: "Hello, visit our site at https://google.com for info. Also visit http://example.com/some/path.",
		HTML: `<p>Hello, visit <a href="https://google.com">Google</a> and <a href="http://example.com/some/path">Example</a>.</p>`,
	}

	req := httptest.NewRequest(http.MethodPost, "/api/emails/send", nil)

	// Create and attach session cookies
	cookieRec := httptest.NewRecorder()
	authMgr.SetSession(cookieRec, 1, 1)
	for _, c := range cookieRec.Result().Cookies() {
		req.AddCookie(c)
	}
	req.Host = "dashboard.mycorp.com"

	h.wrapLinksInEmail(req, &msg)

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
	db, err := gorm.Open(sqlite.Open("file::memory:?cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	db.AutoMigrate(models.AllModels()...)
	db.Where("1 = 1").Delete(&models.Domain{})
	db.Where("1 = 1").Delete(&models.Link{})

	cfg := &config.Config{SecretKey: "secret"}
	cipher := crypto.New(cfg.SecretKey)
	authMgr := auth.New(cfg, cipher).WithDB(db)
	h := New(cfg, db, cipher, authMgr, nil, queue.New(""))

	db.Create(&models.Domain{
		OrgID: 1,
		Name:  "avoid.mycorp.com",
		LinkHosts: []models.Host{
			{Host: "avoid.mycorp.com", Enabled: true},
		},
		ForLink: true,
	})

	msg := mail.Message{
		Text: "Internal: http://localhost:8680/info. Already wrapped: http://avoid.mycorp.com/abc123. Normal: https://google.com",
	}

	req := httptest.NewRequest(http.MethodPost, "/api/emails/send", nil)
	cookieRec := httptest.NewRecorder()
	authMgr.SetSession(cookieRec, 1, 1)
	for _, c := range cookieRec.Result().Cookies() {
		req.AddCookie(c)
	}
	req.Host = "avoid.mycorp.com"

	h.wrapLinksInEmail(req, &msg)

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
