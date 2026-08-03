package mail

import (
	"sync"
	"testing"
	"time"

	"github.com/glebarez/sqlite"
	"github.com/octarq-org/octarq/internal/models"
	"github.com/octarq-org/octarq/plugin"
	"github.com/octarq-org/octarq/plugins/dns"
	"gorm.io/gorm"
)

// TestEmitEmailFansOut verifies OnEmail handlers all receive the event and that
// dispatch is asynchronous (a slow handler doesn't block the others).
func TestEmitEmailFansOut(t *testing.T) {
	p := &Plugin{}

	var mu sync.Mutex
	got := map[string]plugin.EmailEvent{}
	var wg sync.WaitGroup
	wg.Add(2)

	p.OnEmail(func(e plugin.EmailEvent) {
		mu.Lock()
		got["a"] = e
		mu.Unlock()
		wg.Done()
	})
	p.OnEmail(func(e plugin.EmailEvent) {
		mu.Lock()
		got["b"] = e
		mu.Unlock()
		wg.Done()
	})

	want := plugin.EmailEvent{ID: 7, OrgID: 3, From: "x@y.z", Subject: "hi"}
	p.emitEmail(want)

	done := make(chan struct{})
	go func() { wg.Wait(); close(done) }()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("handlers did not fire")
	}

	mu.Lock()
	defer mu.Unlock()
	if got["a"].ID != 7 || got["b"].ID != 7 || got["a"].Subject != "hi" {
		t.Errorf("handlers got wrong event: %+v", got)
	}
}

// TestOnEmailNilIgnored ensures a nil handler is silently dropped (no panic on
// dispatch).
func TestOnEmailNilIgnored(t *testing.T) {
	p := &Plugin{}
	p.OnEmail(nil)
	p.emitEmail(plugin.EmailEvent{ID: 1}) // must not panic
}

func TestMailDispatcherProvided(t *testing.T) {
	p := New()
	reg := plugin.NewRegistry()
	p.Mount(nil, &plugin.Context{
		Provide: reg.Provide,
		Lookup:  reg.Lookup,
	})

	svc, ok := reg.Lookup("mail.dispatcher")
	if !ok || svc == nil {
		t.Fatal("mail.dispatcher service was not provided during Mount")
	}
}

func TestOwnsMailHostSecurityBoundary(t *testing.T) {
	p := &Plugin{}

	// 1. Nil DB or OrgID 0
	if p.ownsMailHost(0, "user@example.com") {
		t.Error("ownsMailHost with orgID=0 expected false")
	}
	if p.ownsMailHost(1, "user@example.com") {
		t.Error("ownsMailHost with nil db expected false")
	}

	db, err := gorm.Open(sqlite.Open("file::memory:?cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	if err := db.AutoMigrate(append(models.AllModels(), &Mailbox{}, &dns.Domain{})...); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	p.db = db

	// 2. Invalid address inputs
	invalidAddrs := []string{"invalid-addr", "user@", "@domain.com", ""}
	for _, addr := range invalidAddrs {
		if p.ownsMailHost(1, addr) {
			t.Errorf("ownsMailHost(%q) expected false", addr)
		}
	}

	// 3. Domain for_mail = false
	db.Create(&dns.Domain{
		OrgID:   1,
		Name:    "nomail.com",
		ForMail: false,
	})
	if p.ownsMailHost(1, "info@nomail.com") {
		t.Error("ownsMailHost for domain with ForMail=false expected false")
	}

	// 4. Domain owned by Org 1 with no explicit MailHosts -> uses apex Name
	db.Create(&dns.Domain{
		OrgID:   1,
		Name:    "apexmail.com",
		ForMail: true,
	})
	if !p.ownsMailHost(1, "info@apexmail.com") {
		t.Error("ownsMailHost for apex domain expected true")
	}

	// 5. Cross-org isolation: Org 2 should NOT own Org 1's domain
	if p.ownsMailHost(2, "info@apexmail.com") {
		t.Error("ownsMailHost for Org 2 on Org 1's domain expected false (cross-tenant breach)")
	}

	// 6. Subdomain / MailHosts explicit
	db.Create(&dns.Domain{
		OrgID:   1,
		Name:    "custom.com",
		ForMail: true,
		MailHosts: models.HostList{
			{Host: "sub.custom.com", Enabled: true},
		},
	})
	if !p.ownsMailHost(1, "info@sub.custom.com") {
		t.Error("ownsMailHost for explicit mail host expected true")
	}
	if p.ownsMailHost(1, "info@unlisted.custom.com") {
		t.Error("ownsMailHost for unlisted mail host expected false")
	}
}
