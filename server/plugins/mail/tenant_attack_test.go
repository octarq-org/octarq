package mail

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/danielgtaylor/huma/v2"
	"github.com/danielgtaylor/huma/v2/adapters/humago"
	"gorm.io/gorm"
)

func TestMail_CrossTenantAttack_AntiPenetration(t *testing.T) {
	t.Parallel()

	p, db := setupCrossOrgMailDB(t)
	ctx := context.Background()

	// Seed data for Org 1
	mb1 := Mailbox{
		OrgID:   1,
		Address: "inbox@org1.example.com",
		Note:    "Org 1 Mailbox",
		Enabled: true,
	}
	s1 := SMTPSender{
		OrgID:     1,
		Name:      "Org 1 SMTP",
		Host:      "smtp.org1.com",
		Port:      587,
		User:      "u1",
		Pass:      "p1",
		FromEmail: "no-reply@org1.example.com",
	}
	sup1 := MailSuppression{
		OrgID:   1,
		Address: "blocked@org1.example.com",
		Reason:  "manual",
		Source:  "manual",
	}
	c1 := MailContact{
		OrgID:   1,
		Address: "contact@org1.example.com",
		Name:    "Org 1 Contact",
	}
	if err := db.Create(&mb1).Error; err != nil {
		t.Fatalf("seed mb1: %v", err)
	}
	if err := db.Create(&s1).Error; err != nil {
		t.Fatalf("seed s1: %v", err)
	}
	if err := db.Create(&sup1).Error; err != nil {
		t.Fatalf("seed sup1: %v", err)
	}
	if err := db.Create(&c1).Error; err != nil {
		t.Fatalf("seed c1: %v", err)
	}

	email1 := Email{
		MailboxID:  mb1.ID,
		Subject:    "Confidential Org 1 Email",
		FromAddr:   "sender@ext.com",
		ToAddr:     mb1.Address,
		Text:       "Secret Org 1 data",
		ReceivedAt: time.Now(),
	}
	if err := db.Create(&email1).Error; err != nil {
		t.Fatalf("seed email1: %v", err)
	}

	// Seed data for Org 2
	mb2 := Mailbox{
		OrgID:   2,
		Address: "inbox@org2.example.com",
		Note:    "Org 2 Mailbox",
		Enabled: true,
	}
	s2 := SMTPSender{
		OrgID:     2,
		Name:      "Org 2 SMTP",
		Host:      "smtp.org2.com",
		Port:      587,
		User:      "u2",
		Pass:      "p2",
		FromEmail: "no-reply@org2.example.com",
	}
	sup2 := MailSuppression{
		OrgID:   2,
		Address: "blocked@org2.example.com",
		Reason:  "manual",
		Source:  "manual",
	}
	c2 := MailContact{
		OrgID:   2,
		Address: "contact@org2.example.com",
		Name:    "Org 2 Contact",
	}
	if err := db.Create(&mb2).Error; err != nil {
		t.Fatalf("seed mb2: %v", err)
	}
	if err := db.Create(&s2).Error; err != nil {
		t.Fatalf("seed s2: %v", err)
	}
	if err := db.Create(&sup2).Error; err != nil {
		t.Fatalf("seed sup2: %v", err)
	}
	if err := db.Create(&c2).Error; err != nil {
		t.Fatalf("seed c2: %v", err)
	}

	email2 := Email{
		MailboxID:  mb2.ID,
		Subject:    "Confidential Org 2 Email",
		FromAddr:   "boss@org2.example.com",
		ToAddr:     mb2.Address,
		Text:       "Secret Org 2 financial report",
		ReceivedAt: time.Now(),
	}
	if err := db.Create(&email2).Error; err != nil {
		t.Fatalf("seed email2: %v", err)
	}

	makeCtxForOrg := func(orgID uint, method, path string) huma.Context {
		req := httptest.NewRequest(method, path, nil)
		req.Header.Set("X-Org-ID", "1")
		if orgID != 1 {
			req.Header.Set("X-Org-ID", "2")
		}
		req.Header.Set("X-Role", "admin")
		return humago.NewContext(nil, req, httptest.NewRecorder())
	}

	// Attack Scenario 1: Tenant 1 attempts to UPDATE Tenant 2's Mailbox
	t.Run("UpdateOtherTenantMailbox_Returns404", func(t *testing.T) {
		hCtx := makeCtxForOrg(1, http.MethodPut, "/api/mailboxes/update")
		in := &UpdateMailboxInput{
			Ctx:  hCtx,
			ID:   mb2.ID,
			Body: mailboxDTO{Note: "Hacked by Tenant 1"},
		}
		_ = in.Resolve(hCtx)

		_, err := p.updateMailbox(ctx, in)
		if err == nil {
			t.Fatalf("expected 404 updating tenant 2 mailbox as tenant 1, got nil")
		}
		hErr, ok := err.(huma.StatusError)
		if !ok || hErr.GetStatus() != http.StatusNotFound {
			t.Fatalf("expected 404, got: %v", err)
		}

		var check Mailbox
		db.First(&check, mb2.ID)
		if check.Note != "Org 2 Mailbox" {
			t.Fatalf("mailbox 2 note was modified: %s", check.Note)
		}
	})

	// Attack Scenario 2: Tenant 1 attempts to DELETE Tenant 2's Mailbox
	t.Run("DeleteOtherTenantMailbox_Returns404", func(t *testing.T) {
		hCtx := makeCtxForOrg(1, http.MethodDelete, "/api/mailboxes/delete")
		in := &DeleteMailboxInput{
			Ctx: hCtx,
			ID:  mb2.ID,
		}
		_ = in.Resolve(hCtx)

		_, err := p.deleteMailbox(ctx, in)
		if err == nil {
			t.Fatalf("expected 404 deleting tenant 2 mailbox as tenant 1, got nil")
		}
		hErr, ok := err.(huma.StatusError)
		if !ok || hErr.GetStatus() != http.StatusNotFound {
			t.Fatalf("expected 404, got: %v", err)
		}

		var check Mailbox
		if err := db.First(&check, mb2.ID).Error; err != nil {
			t.Fatalf("mailbox 2 was deleted: %v", err)
		}
	})

	// Attack Scenario 3: Tenant 1 attempts to UPDATE Tenant 2's SMTP Sender
	t.Run("UpdateOtherTenantSMTPSender_Returns404", func(t *testing.T) {
		hCtx := makeCtxForOrg(1, http.MethodPut, "/api/smtp/update")
		newName := "Hacked SMTP"
		in := &UpdateSMTPSenderInput{
			Ctx: hCtx,
			ID:  s2.ID,
			Body: struct {
				Name      *string `json:"name,omitempty"`
				Host      *string `json:"host,omitempty"`
				Port      *int    `json:"port,omitempty"`
				User      *string `json:"user,omitempty"`
				Pass      *string `json:"pass,omitempty"`
				FromEmail *string `json:"fromEmail,omitempty"`
			}{Name: &newName},
		}
		_ = in.Resolve(hCtx)

		_, err := p.updateSMTPSender(ctx, in)
		if err == nil {
			t.Fatalf("expected 404 updating tenant 2 smtp sender as tenant 1, got nil")
		}
		hErr, ok := err.(huma.StatusError)
		if !ok || hErr.GetStatus() != http.StatusNotFound {
			t.Fatalf("expected 404, got: %v", err)
		}

		var check SMTPSender
		db.First(&check, s2.ID)
		if check.Name != "Org 2 SMTP" {
			t.Fatalf("smtp sender 2 name was modified: %s", check.Name)
		}
	})

	// Attack Scenario 4: Tenant 1 attempts to DELETE Tenant 2's SMTP Sender
	t.Run("DeleteOtherTenantSMTPSender_Returns404", func(t *testing.T) {
		hCtx := makeCtxForOrg(1, http.MethodDelete, "/api/smtp/delete")
		in := &DeleteSMTPSenderInput{
			Ctx: hCtx,
			ID:  s2.ID,
		}
		_ = in.Resolve(hCtx)

		_, err := p.deleteSMTPSender(ctx, in)
		if err == nil {
			t.Fatalf("expected 404 deleting tenant 2 smtp sender as tenant 1, got nil")
		}
		hErr, ok := err.(huma.StatusError)
		if !ok || hErr.GetStatus() != http.StatusNotFound {
			t.Fatalf("expected 404, got: %v", err)
		}

		var check SMTPSender
		if err := db.First(&check, s2.ID).Error; err != nil {
			t.Fatalf("smtp sender 2 was deleted: %v", err)
		}
	})

	// Attack Scenario 5: Tenant 1 attempts to DELETE Tenant 2's Suppression
	t.Run("DeleteOtherTenantSuppression_Returns404", func(t *testing.T) {
		hCtx := makeCtxForOrg(1, http.MethodDelete, "/api/suppressions/delete")
		in := &DeleteSuppressionInput{
			Ctx: hCtx,
			ID:  sup2.ID,
		}
		_ = in.Resolve(hCtx)

		_, err := p.deleteSuppression(ctx, in)
		if err == nil {
			t.Fatalf("expected 404 deleting tenant 2 suppression as tenant 1, got nil")
		}
		hErr, ok := err.(huma.StatusError)
		if !ok || hErr.GetStatus() != http.StatusNotFound {
			t.Fatalf("expected 404, got: %v", err)
		}

		var check MailSuppression
		if err := db.First(&check, sup2.ID).Error; err != nil {
			t.Fatalf("suppression 2 was deleted: %v", err)
		}
	})

	// Attack Scenario 6: Tenant 1 attempts to READ or DELETE Tenant 2's Email
	t.Run("AccessOtherTenantEmail_Returns404", func(t *testing.T) {
		hCtx := makeCtxForOrg(1, http.MethodGet, "/api/emails/get")
		getIn := &GetEmailInput{
			Ctx: hCtx,
			ID:  email2.ID,
		}
		_ = getIn.Resolve(hCtx)

		_, err := p.getEmail(ctx, getIn)
		if err == nil {
			t.Fatalf("expected 404 getting tenant 2 email as tenant 1, got nil")
		}
		hErr, ok := err.(huma.StatusError)
		if !ok || hErr.GetStatus() != http.StatusNotFound {
			t.Fatalf("expected 404, got: %v", err)
		}

		delIn := &DeleteEmailInput{
			Ctx: hCtx,
			ID:  email2.ID,
		}
		_ = delIn.Resolve(hCtx)

		_, err = p.deleteEmail(ctx, delIn)
		if err == nil {
			t.Fatalf("expected 404 deleting tenant 2 email as tenant 1, got nil")
		}

		var check Email
		if err := db.First(&check, email2.ID).Error; err != nil {
			t.Fatalf("email 2 was deleted: %v", err)
		}
	})

	// Attack Scenario 7: Direct TenantDB injection / privilege escalation
	t.Run("DirectTenantDB_FailClosedEnforcement", func(t *testing.T) {
		tdb1 := p.tenantDB(1)
		if tdb1 == nil {
			t.Fatalf("tenantDB(1) is nil")
		}

		// Injecting Mailbox with OrgID 2
		if err := tdb1.Create(&Mailbox{OrgID: 2, Address: "evil@org2.com"}).Error; err == nil {
			t.Fatalf("expected error creating Mailbox OrgID 2 in TenantDB(1), got nil")
		}

		// Injecting SMTPSender with OrgID 2
		if err := tdb1.Create(&SMTPSender{OrgID: 2, Name: "Evil SMTP"}).Error; err == nil {
			t.Fatalf("expected error creating SMTPSender OrgID 2 in TenantDB(1), got nil")
		}

		// Injecting MailSuppression with OrgID 2
		if err := tdb1.Create(&MailSuppression{OrgID: 2, Address: "evil@org2.com"}).Error; err == nil {
			t.Fatalf("expected error creating MailSuppression OrgID 2 in TenantDB(1), got nil")
		}

		// Injecting MailContact with OrgID 2
		if err := tdb1.Create(&MailContact{OrgID: 2, Address: "evil@org2.com"}).Error; err == nil {
			t.Fatalf("expected error creating MailContact OrgID 2 in TenantDB(1), got nil")
		}

		// Overwrite Mailbox 2 via Save
		if err := tdb1.Save(&Mailbox{ID: mb2.ID, OrgID: 2, Note: "hacked"}).Error; err == nil {
			t.Fatalf("expected error saving Mailbox OrgID 2 in TenantDB(1), got nil")
		}

		// Querying Mailbox 2 via TenantDB 1
		var forbidden Mailbox
		if err := tdb1.Where("id = ?", mb2.ID).First(&forbidden).Error; err == nil {
			t.Fatalf("tenant 1 read tenant 2 mailbox: %+v", forbidden)
		} else if err != gorm.ErrRecordNotFound {
			t.Fatalf("expected gorm.ErrRecordNotFound, got: %v", err)
		}

		// Querying SMTPSender 2 via TenantDB 1
		var forbiddenSender SMTPSender
		if err := tdb1.Where("id = ?", s2.ID).First(&forbiddenSender).Error; err == nil {
			t.Fatalf("tenant 1 read tenant 2 sender: %+v", forbiddenSender)
		} else if err != gorm.ErrRecordNotFound {
			t.Fatalf("expected gorm.ErrRecordNotFound, got: %v", err)
		}
	})

	// Attack Scenario 8: High-concurrency cross-tenant queries (-race test)
	t.Run("ConcurrentCrossTenantAccess_NoStateLeak", func(t *testing.T) {
		var wg sync.WaitGroup
		errCh := make(chan error, 40)

		for i := 0; i < 20; i++ {
			wg.Add(2)

			go func() {
				defer wg.Done()
				tdb := p.tenantDB(1)
				var mbs []Mailbox
				if err := tdb.Find(&mbs).Error; err != nil {
					errCh <- err
					return
				}
				for _, m := range mbs {
					if m.OrgID != 1 {
						t.Errorf("tenant 1 read mailbox belonging to org %d", m.OrgID)
					}
				}
			}()

			go func() {
				defer wg.Done()
				tdb := p.tenantDB(2)
				var mbs []Mailbox
				if err := tdb.Find(&mbs).Error; err != nil {
					errCh <- err
					return
				}
				for _, m := range mbs {
					if m.OrgID != 2 {
						t.Errorf("tenant 2 read mailbox belonging to org %d", m.OrgID)
					}
				}
			}()
		}

		wg.Wait()
		close(errCh)
		for err := range errCh {
			t.Errorf("concurrent query error: %v", err)
		}
	})
}
