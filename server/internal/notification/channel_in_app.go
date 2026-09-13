package notification

import (
	"context"
	"encoding/json"
	"errors"
	"strconv"
	"time"

	"github.com/octarq-org/octarq/internal/eventbus"
	"github.com/octarq-org/octarq/internal/models"
	"github.com/octarq-org/octarq/plugin"
	"gorm.io/gorm"
)

// InAppChannel delivers in-app notifications by persisting them to the database
// and streaming them over the SSE event spine.
type InAppChannel struct {
	db *gorm.DB
}

var _ plugin.NotificationChannel = (*InAppChannel)(nil)

// NewInAppChannel creates an InAppChannel backed by GORM and the eventbus spine.
func NewInAppChannel(db *gorm.DB) *InAppChannel {
	return &InAppChannel{db: db}
}

// Name returns the channel identifier "in_app".
func (c *InAppChannel) Name() string {
	return "in_app"
}

// DisplayName returns the human-readable channel name.
func (c *InAppChannel) DisplayName() string {
	return "In-App Notification"
}

// ConfigSchema returns the JSON Schema for user configuration (in-app requires no extra config).
func (c *InAppChannel) ConfigSchema() json.RawMessage {
	return json.RawMessage(`{
		"$schema": "http://json-schema.org/draft-07/schema#",
		"type": "object",
		"title": "In-App Notification Settings",
		"properties": {},
		"additionalProperties": false
	}`)
}

// Send records the notification in the database and emits an SSE event on the eventbus spine.
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

	// Publish to in-process event spine for SSE streaming (/api/realtime/stream)
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
