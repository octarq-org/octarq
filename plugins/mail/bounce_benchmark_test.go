package mail

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/danielgtaylor/huma/v2/adapters/humago"
	"github.com/glebarez/sqlite"
	"github.com/octarq-org/octarq/internal/models"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func BenchmarkEmailBounceWebhook(b *testing.B) {
	db, err := gorm.Open(sqlite.Open("file::memory:?cache=shared"), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		b.Fatalf("failed to create test db: %v", err)
	}

	// Migrate schemas
	db.AutoMigrate(&models.Org{}, &Mailbox{}, &models.AuditLog{}, &MailSuppression{}, &models.NotificationChannel{})

	p := &Plugin{db: db}

	// Create org
	org := &models.Org{
		Slug:         "acme",
		InboundToken: "tok",
	}
	db.Create(org)

	// Create a bunch of mailboxes
	var emails []string
	for i := 0; i < 100; i++ {
		email := fmt.Sprintf("user%d@acme.example", i)
		emails = append(emails, email)
		db.Create(&Mailbox{
			OrgID:   org.ID,
			Address: email,
		})
	}

	// Create payload with many bounce events
	type snsRecipient struct {
		EmailAddress string `json:"emailAddress"`
	}
	type snsBounce struct {
		BounceType        string         `json:"bounceType"`
		BouncedRecipients []snsRecipient `json:"bouncedRecipients"`
	}
	type snsPayload struct {
		NotificationType string    `json:"notificationType"`
		Bounce           snsBounce `json:"bounce"`
	}

	var recipients []snsRecipient
	for _, email := range emails {
		recipients = append(recipients, snsRecipient{EmailAddress: email})
	}

	payload := snsPayload{
		NotificationType: "Bounce",
		Bounce: snsBounce{
			BounceType:        "Permanent",
			BouncedRecipients: recipients,
		},
	}
	payloadBytes, _ := json.Marshal(payload)
	payloadStr := string(payloadBytes)

	ctx := context.Background()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		req := httptest.NewRequest(http.MethodPost, "/api/webhook/acme/email/bounce/tok", bytes.NewBufferString(payloadStr))
		input := &EmailBounceWebhookInput{
			Ctx:     humago.NewContext(nil, req, httptest.NewRecorder()),
			OrgSlug: "acme",
			Token:   "tok",
		}

		_, err := p.emailBounceWebhook(ctx, input)
		if err != nil {
			b.Fatalf("webhook failed: %v", err)
		}
	}
}
