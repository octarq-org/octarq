package notify

import (
	"context"
	"fmt"
	"strings"

	"github.com/octarq-org/octarq/server/internal/models"
	"github.com/octarq-org/octarq/server/internal/notification"
	"github.com/octarq-org/octarq/server/plugin"
)

// NotifyInput is the input for the notify tool. Message is required; Channel
// optionally narrows delivery to one channel type (telegram, slack, webhook,
// email, …) instead of every enabled channel.
type NotifyInput struct {
	Message string `json:"message"`
	Title   string `json:"title,omitempty"`
	Channel string `json:"channel,omitempty"`
}

// NotifyOutput reports how many channels accepted the message and which.
type NotifyOutput struct {
	Delivered int      `json:"delivered"`
	Channels  []string `json:"channels"`
}

func (p *Plugin) registerRoutes(ctx *plugin.Context) {
	_ = plugin.RegisterEndpoint(ctx, plugin.EndpointSpec[NotifyInput, NotifyOutput]{
		Name:        "notify",
		Method:      "POST",
		Path:        "/api/notify",
		Summary:     "Send Notification",
		Description: "Deliver a message to the workspace's enabled notification channels (telegram, slack, webhook, email, in-app) so an agent can reach a human.",
		RiskLevel:   plugin.RiskLevelWrite,
		RequireAuth: true,
		ExposeMCP:   true,
		Handler:     p.notify,
	})
}

func (p *Plugin) notify(ctx context.Context, in NotifyInput) (*NotifyOutput, error) {
	msg := strings.TrimSpace(in.Message)
	if msg == "" {
		return nil, fmt.Errorf("notify: message is required")
	}
	orgID := plugin.OrgIDFromContext(ctx)
	if orgID == 0 {
		return nil, fmt.Errorf("notify: no workspace in this request")
	}
	if p.db == nil {
		return nil, fmt.Errorf("notify: database unavailable")
	}

	q := p.db.WithContext(ctx).Where("owner_id = ? AND enabled = ?", orgID, true)
	if typ := strings.ToLower(strings.TrimSpace(in.Channel)); typ != "" {
		q = q.Where("type = ?", typ)
	}
	var channels []models.NotificationChannel
	if err := q.Find(&channels).Error; err != nil {
		return nil, fmt.Errorf("notify: load channels: %w", err)
	}
	if len(channels) == 0 {
		if in.Channel != "" {
			return nil, fmt.Errorf("notify: no enabled %q notification channel for this workspace", in.Channel)
		}
		return nil, fmt.Errorf("notify: no enabled notification channels for this workspace — configure one in Settings → Alerts")
	}

	text := msg
	if t := strings.TrimSpace(in.Title); t != "" {
		text = t + " — " + msg
	}

	out := &NotifyOutput{Channels: make([]string, 0, len(channels))}
	var failures []string
	for _, ch := range channels {
		plaintext, err := notification.ConfigPlaintext(ch.Config)
		if err != nil {
			failures = append(failures, fmt.Sprintf("%s: %v", ch.Type, err))
			continue
		}
		if err := notification.SendDirectWithTenant(ctx, orgID, ch.Type, plaintext, text); err != nil {
			failures = append(failures, fmt.Sprintf("%s: %v", ch.Type, err))
			continue
		}
		out.Delivered++
		out.Channels = append(out.Channels, ch.Type)
	}
	if len(failures) > 0 {
		return nil, fmt.Errorf("notify: delivery failed on %d of %d channels: %s", len(failures), len(channels), strings.Join(failures, "; "))
	}
	return out, nil
}
