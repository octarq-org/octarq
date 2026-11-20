package plugin

import (
	"context"
	"encoding/json"
	"errors"
)

type NotificationChannel interface {
	Name() string
	DisplayName() string
	Send(ctx context.Context, recipient NotificationRecipient, payload NotificationPayload) error
	ConfigSchema() json.RawMessage
}

type NotificationPayload struct {
	EventType string                 `json:"eventType"`
	Title     string                 `json:"title"`
	Body      string                 `json:"body"`
	Data      map[string]interface{} `json:"data,omitempty"`
	Priority  string                 `json:"priority,omitempty"`
	UserID    string                 `json:"userId,omitempty"`
	OrgID     string                 `json:"orgId,omitempty"`
}

type NotificationRecipient struct {
	UserID string                 `json:"userId"`
	OrgID  string                 `json:"orgId"`
	Config map[string]interface{} `json:"config,omitempty"`
}

func RegisterNotificationChannel(ctx *Context, ch NotificationChannel) error {
	if ch == nil {
		return errors.New("notification: channel cannot be nil")
	}
	if ch.Name() == "" {
		return errors.New("notification: channel name cannot be empty")
	}
	if ctx != nil && ctx.RegisterNotificationChannel != nil {
		ctx.RegisterNotificationChannel(ch)
	}
	return nil
}
