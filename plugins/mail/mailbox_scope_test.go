package mail

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/danielgtaylor/huma/v2/adapters/humago"
	"github.com/glebarez/sqlite"
	"github.com/octarq-org/octarq/internal/models"
	"github.com/octarq-org/octarq/plugin"
	"github.com/octarq-org/octarq/plugins/dns"
	"gorm.io/gorm"
)

func setupMailboxTestDB(t *testing.T) (*gorm.DB, *Plugin) {
	t.Helper()
	db, err := gorm.Open(sqlite.Open("file::memory:?cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	if err := db.AutoMigrate(append(models.AllModels(), &Mailbox{}, &dns.Domain{})...); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	db.Where("1 = 1").Delete(&Mailbox{})
	db.Where("1 = 1").Delete(&dns.Domain{})

	p := New()
	p.Mount(nil, &plugin.Context{
		DB: db,
		OrgID: func(r *http.Request) uint {
			if val := r.Header.Get("X-Org-ID"); val != "" {
				var id uint
				fmt.Sscanf(val, "%d", &id)
				return id
			}
			return 1
		},
	})
	return db, p
}

func TestMailboxScopeAndDomainValidation(t *testing.T) {
	db, p := setupMailboxTestDB(t)

	// Org B (orgID=2) owns victim.example with mail host "victim.example"
	db.Create(&dns.Domain{
		OrgID:   2,
		Name:    "victim.example",
		ForMail: true,
		MailHosts: models.HostList{
			models.Host{Host: "victim.example", Enabled: true},
		},
	})

	// Org A (orgID=1) owns mymail.example with mail host "mymail.example"
	db.Create(&dns.Domain{
		OrgID:   1,
		Name:    "mymail.example",
		ForMail: true,
		MailHosts: models.HostList{
			models.Host{Host: "mymail.example", Enabled: true},
		},
	})

	ctx := context.Background()

	// 1. Org A creates address under its own mail host -> 201 (success)
	{
		req := httptest.NewRequest(http.MethodPost, "/api/mailboxes", nil)
		req.Header.Set("X-Org-ID", "1")
		input := &CreateMailboxInput{
			Ctx: humago.NewContext(nil, req, httptest.NewRecorder()),
			Body: mailboxDTO{
				Address: "user@mymail.example",
			},
		}
		out, err := p.createMailbox(ctx, input)
		if err != nil {
			t.Fatalf("expected success creating mailbox for Org A, got: %v", err)
		}
		if out.Body.Address != "user@mymail.example" {
			t.Errorf("got %q, want user@mymail.example", out.Body.Address)
		}
	}

	// 2. Org A and Org B both create same@unmanaged.example -> both succeed (no global unique constraint error)
	{
		req1 := httptest.NewRequest(http.MethodPost, "/api/mailboxes", nil)
		req1.Header.Set("X-Org-ID", "1")
		input1 := &CreateMailboxInput{
			Ctx: humago.NewContext(nil, req1, httptest.NewRecorder()),
			Body: mailboxDTO{
				Address: "same@unmanaged.example",
			},
		}
		if _, err := p.createMailbox(ctx, input1); err != nil {
			t.Fatalf("Org A failed to create mailbox on unmanaged domain: %v", err)
		}

		req2 := httptest.NewRequest(http.MethodPost, "/api/mailboxes", nil)
		req2.Header.Set("X-Org-ID", "2")
		input2 := &CreateMailboxInput{
			Ctx: humago.NewContext(nil, req2, httptest.NewRecorder()),
			Body: mailboxDTO{
				Address: "same@unmanaged.example",
			},
		}
		if _, err := p.createMailbox(ctx, input2); err != nil {
			t.Fatalf("Org B failed to create same address on unmanaged domain: %v", err)
		}
	}

	// 3. Org A tries to create billing@victim.example (domain owned by Org B) -> 403
	{
		req := httptest.NewRequest(http.MethodPost, "/api/mailboxes", nil)
		req.Header.Set("X-Org-ID", "1")
		input := &CreateMailboxInput{
			Ctx: humago.NewContext(nil, req, httptest.NewRecorder()),
			Body: mailboxDTO{
				Address: "billing@victim.example",
			},
		}
		_, err := p.createMailbox(ctx, input)
		if err == nil {
			t.Fatal("expected 403 error when Org A creates mailbox on Org B's domain, got nil")
		}
	}
}
