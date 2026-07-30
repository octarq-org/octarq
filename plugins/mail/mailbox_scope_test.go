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
			// Domain.Name is globally unique but MailHosts entries are not, so two
			// tenants can legitimately list the same mail host on their own domains.
			models.Host{Host: "shared.example", Enabled: true},
		},
	})

	// Org A (orgID=1) owns mymail.example with mail host "mymail.example"
	db.Create(&dns.Domain{
		OrgID:   1,
		Name:    "mymail.example",
		ForMail: true,
		MailHosts: models.HostList{
			models.Host{Host: "mymail.example", Enabled: true},
			models.Host{Host: "shared.example", Enabled: true},
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

	// 2. Both orgs create the same local-part on a hostname each owns on its own
	// domain -> both succeed, proving the unique index is (owner_id, address) and
	// no longer global. Each uses a domain it actually owns: creating a mailbox on
	// an unclaimed domain is refused, same as the catch-all path refuses delivery.
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
			t.Fatalf("Org A failed to create mailbox on an unclaimed domain: %v", err)
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

// Catch-all auto-creates a mailbox from an inbound message with no operator in
// the loop, so it applies the strict rule: the recipient host must be one of
// THIS workspace's own mail hosts. The looser "not another tenant's" rule that
// governs manual creation would let anything routed at this org's webhook
// materialise a mailbox on a domain it never registered.
func TestCatchAllOnlyCreatesOnOwnedMailHosts(t *testing.T) {
	db, p := setupMailboxTestDB(t)
	p.getWorkspaceSetting = func(orgID uint, key string) string {
		if key == "mail.catch_all" {
			return "true"
		}
		return ""
	}

	const orgA uint = 1
	db.Create(&dns.Domain{
		OrgID:   orgA,
		Name:    "owned.example",
		ForMail: true,
		MailHosts: models.HostList{
			models.Host{Host: "owned.example", Enabled: true},
		},
	})

	// Own mail host -> catch-all creates it.
	if _, ok := p.resolveMailbox(orgA, "anyone@owned.example"); !ok {
		t.Fatal("catch-all should create a mailbox on the workspace's own mail host")
	}

	// A domain this workspace has not registered -> refused, even though no other
	// tenant owns it either. This is the assertion that separates the strict rule
	// from the loose one.
	if _, ok := p.resolveMailbox(orgA, "anyone@unclaimed.example"); ok {
		t.Fatal("catch-all must not create a mailbox on a domain the workspace does not own")
	}
}
