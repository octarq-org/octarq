package plugin

import (
	"context"
	"encoding/json"
	"errors"
)

// NotificationChannel is the core SPI abstraction for notification delivery channels
// such as in_app, email, telegram, slack, and webhooks.
type NotificationChannel interface {
	// Name 渠道标识（如 "in_app", "email", "telegram", "slack"）
	Name() string
	// DisplayName 用户可见名称
	DisplayName() string
	// Send 发送一条通知
	Send(ctx context.Context, recipient NotificationRecipient, payload NotificationPayload) error
	// ConfigSchema 返回该渠道所需的用户配置 Schema（JSON Schema）
	ConfigSchema() json.RawMessage
}

// NotificationPayload carries the content, context, and priority of an event notification.
type NotificationPayload struct {
	EventType string                 `json:"eventType"` // 如 "security.login_failed", "cron.job_failed"
	Title     string                 `json:"title"`
	Body      string                 `json:"body"`
	Data      map[string]interface{} `json:"data,omitempty"`
	Priority  string                 `json:"priority,omitempty"` // "low" | "normal" | "high" | "critical"
	UserID    string                 `json:"userId,omitempty"`   // optional target user
	OrgID     string                 `json:"orgId,omitempty"`    // optional target organization
}

// NotificationRecipient carries the target user/org and channel-specific configuration.
type NotificationRecipient struct {
	UserID string                 `json:"userId"`
	OrgID  string                 `json:"orgId"`
	Config map[string]interface{} `json:"config,omitempty"` // 用户为该渠道配置的参数（如 TG chat_id）
}

// RegisterNotificationChannel validates and registers a NotificationChannel onto a Context.
// Returns an error if channel or its name is empty. If ctx is nil or does not wire
// RegisterNotificationChannel, it is a no-op for backward compatibility.
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
