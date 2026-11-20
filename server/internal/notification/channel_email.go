package notification

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/octarq-org/octarq/server/internal/models"
	"github.com/octarq-org/octarq/server/plugin"
	"gorm.io/gorm"
)

type EmailSenderFunc func(ctx context.Context, orgID uint, to, subject, htmlBody, textBody string) error

type EmailChannel struct {
	db     *gorm.DB
	sender EmailSenderFunc
}

var _ plugin.NotificationChannel = (*EmailChannel)(nil)

func NewEmailChannel(db *gorm.DB, sender EmailSenderFunc) *EmailChannel {
	return &EmailChannel{
		db:     db,
		sender: sender,
	}
}

func (c *EmailChannel) SetSender(sender EmailSenderFunc) {
	c.sender = sender
}

func (c *EmailChannel) Name() string {
	return "email"
}

func (c *EmailChannel) DisplayName() string {
	return "Email"
}

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

func (c *EmailChannel) Send(ctx context.Context, recipient plugin.NotificationRecipient, payload plugin.NotificationPayload) error {
	toEmail := ""

	if recipient.Config != nil {
		if v, ok := recipient.Config["email"].(string); ok && strings.TrimSpace(v) != "" {
			toEmail = strings.TrimSpace(v)
		} else if v, ok := recipient.Config["to"].(string); ok && strings.TrimSpace(v) != "" {
			toEmail = strings.TrimSpace(v)
		}
	}

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
	if n, err := strconv.ParseUint(strings.TrimSpace(recipient.OrgID), 10, 64); err == nil && n > 0 {
		orgID = uint(n)
	}
	if orgID == 0 && recipient.Config != nil {
		if v, ok := recipient.Config["orgId"].(float64); ok && v > 0 {
			orgID = uint(v)
		} else if v, ok := recipient.Config["org_id"].(float64); ok && v > 0 {
			orgID = uint(v)
		}
	}
	if orgID == 0 && strings.TrimSpace(payload.OrgID) != "" {
		if n, err := strconv.ParseUint(strings.TrimSpace(payload.OrgID), 10, 64); err == nil && n > 0 {
			orgID = uint(n)
		}
	}
	if orgID == 0 && c.db != nil && strings.TrimSpace(recipient.UserID) != "" {
		if uid, err := strconv.ParseUint(strings.TrimSpace(recipient.UserID), 10, 64); err == nil && uid > 0 {
			var member models.OrgMember
			if err := c.db.WithContext(ctx).Where("user_id = ?", uid).Order("created_at asc").First(&member).Error; err == nil && member.OrgID > 0 {
				orgID = member.OrgID
			}
		}
	}
	if orgID == 0 {
		return errors.New("notification: missing or invalid tenant org context")
	}

	subject := payload.Title
	if subject == "" {
		subject = "Notification from Octarq"
	}

	textBody := payload.Body
	htmlBody := fmt.Sprintf("<h2>%s</h2><p>%s</p>", subject, strings.ReplaceAll(textBody, "\n", "<br/>"))

	return c.sender(ctx, orgID, toEmail, subject, htmlBody, textBody)
}
