package notification

import (
	"context"
	"encoding/json"
	"errors"
	"strconv"
	"time"

	"github.com/octarq-org/octarq/server/internal/eventbus"
	"github.com/octarq-org/octarq/server/internal/models"
	"github.com/octarq-org/octarq/server/plugin"
	"gorm.io/gorm"
)

type InAppChannel struct {
	db *gorm.DB
}

var _ plugin.NotificationChannel = (*InAppChannel)(nil)

func NewInAppChannel(db *gorm.DB) *InAppChannel {
	return &InAppChannel{db: db}
}

func (c *InAppChannel) Name() string {
	return "in_app"
}

func (c *InAppChannel) DisplayName() string {
	return "In-App Notification"
}

func (c *InAppChannel) ConfigSchema() json.RawMessage {
	return json.RawMessage(`{
		"$schema": "http://json-schema.org/draft-07/schema#",
		"type": "object",
		"title": "In-App Notification Settings",
		"properties": {},
		"additionalProperties": false
	}`)
}

func (c *InAppChannel) Send(ctx context.Context, recipient plugin.NotificationRecipient, payload plugin.NotificationPayload) error {
	if c.db == nil {
		return errors.New("notification: in_app channel requires database handle")
	}

	dataStr := ""
	if payload.Data != nil {
		if b, err := json.Marshal(payload.Data); err == nil {
			dataStr = string(b)
		}
	}

	priority := payload.Priority
	if priority == "" {
		priority = "normal"
	}

	notif := models.Notification{
		UserID:    recipient.UserID,
		OrgID:     recipient.OrgID,
		EventType: payload.EventType,
		Title:     payload.Title,
		Body:      payload.Body,
		Data:      dataStr,
		Priority:  priority,
		CreatedAt: time.Now(),
	}

	if err := c.db.WithContext(ctx).Create(&notif).Error; err != nil {
		return err
	}

	var orgID uint
	if n, err := strconv.ParseUint(recipient.OrgID, 10, 64); err == nil {
		orgID = uint(n)
	}

	payloadBytes, _ := json.Marshal(notif)
	eventbus.PublishEnvelope(plugin.Envelope{
		ID:         strconv.FormatUint(uint64(notif.ID), 10),
		OrgID:      orgID,
		Key:        "notification",
		EntityKey:  recipient.UserID,
		Payload:    payloadBytes,
		OccurredAt: notif.CreatedAt,
	})

	return nil
}
