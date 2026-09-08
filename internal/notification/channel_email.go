package notification

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/octarq-org/octarq/internal/models"
	"github.com/octarq-org/octarq/plugin"
	"gorm.io/gorm"
)

// EmailSenderFunc defines the contract for sending transactional emails.
type EmailSenderFunc func(ctx context.Context, orgID uint, to, subject, htmlBody, textBody string) error

// EmailChannel delivers notifications via email, reusing the core mail sending engine.
type EmailChannel struct {
	db     *gorm.DB
	sender EmailSenderFunc
}

var _ plugin.NotificationChannel = (*EmailChannel)(nil)

// NewEmailChannel constructs an EmailChannel. sender can be nil initially and configured via SetSender.
func NewEmailChannel(db *gorm.DB, sender EmailSenderFunc) *EmailChannel {
	return &EmailChannel{
		db:     db,
		sender: sender,
	}
}

// SetSender updates the transactional email sending function.
func (c *EmailChannel) SetSender(sender EmailSenderFunc) {
	c.sender = sender
}

// Name returns the channel identifier "email".
func (c *EmailChannel) Name() string {
	return "email"
}

// DisplayName returns the human-readable channel name.
func (c *EmailChannel) DisplayName() string {
	return "Email"
}

// ConfigSchema returns the JSON Schema for user configuration.
func (c *EmailChannel) ConfigSchema() json.RawMessage {
	return json.RawMessage(`{
		"$schema": "http://json-schema.org/draft-07/schema#",
		"type": "object",
		"title": "Email Notification Settings",
		"properties": {
			"email": {
				"type": "string",
				"format": "email",
				"description": "Override notification destination email address"
			}
		}
	}`)
}

// Send resolves the recipient's email address and relays the notification via the configured mail sender.
func (c *EmailChannel) Send(ctx context.Context, recipient plugin.NotificationRecipient, payload plugin.NotificationPayload) error {
	toEmail := ""

	// 1. Check user-configured override
	if recipient.Config != nil {
		if v, ok := recipient.Config["email"].(string); ok && strings.TrimSpace(v) != "" {
			toEmail = strings.TrimSpace(v)
		}
	}

	// 2. Lookup user's account email from the database
	if toEmail == "" && c.db != nil && recipient.UserID != "" {
		if uid, err := strconv.ParseUint(recipient.UserID, 10, 64); err == nil && uid > 0 {
			var user models.User
			if err := c.db.WithContext(ctx).First(&user, uid).Error; err == nil {
				toEmail = strings.TrimSpace(user.Email)
			}
		}
	}

	if toEmail == "" {
		return fmt.Errorf("notification: recipient email cannot be resolved for user %q", recipient.UserID)
	}

	if c.sender == nil {
		return errors.New("notification: no email sender configured")
	}

	var orgID uint
	if n, err := strconv.ParseUint(recipient.OrgID, 10, 64); err == nil {
		orgID = uint(n)
	}

	subject := payload.Title
	if subject == "" {
		subject = fmt.Sprintf("[%s] Notification", payload.EventType)
	}

	textBody := payload.Body
	htmlBody := fmt.Sprintf("<h2>%s</h2><p>%s</p>", subject, strings.ReplaceAll(textBody, "\n", "<br/>"))

	return c.sender(ctx, orgID, toEmail, subject, htmlBody, textBody)
}
