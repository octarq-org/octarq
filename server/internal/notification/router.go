package notification

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"sync"

	"github.com/octarq-org/octarq/server/internal/models"
	"github.com/octarq-org/octarq/server/plugin"
	"gorm.io/gorm"
)

// Descriptor describes a notification channel type (built-in or plugin-contributed).
type Descriptor struct {
	Type        string `json:"type"`
	Title       string `json:"title"`
	Description string `json:"description"`
	Icon        string `json:"icon"`
	PluginName  string `json:"pluginName,omitempty"`
}

// Router routes notifications to registered channels based on event type and user preferences.
type Router struct {
	db          *gorm.DB
	channels    map[string]plugin.NotificationChannel
	descriptors map[string]Descriptor
	chMu        sync.RWMutex
	dispatcher  *Dispatcher
}

// RouterOption configures a Router instance.
type RouterOption func(*Router)

// WithDispatcher overrides the default asynchronous dispatcher.
func WithDispatcher(d *Dispatcher) RouterOption {
	return func(r *Router) {
		if d != nil {
			r.dispatcher = d
		}
	}
}

// NewRouter constructs a notification Router with built-in channels (in_app, email, telegram, webhook).
func NewRouter(db *gorm.DB, opts ...RouterOption) *Router {
	r := &Router{
		db:          db,
		channels:    make(map[string]plugin.NotificationChannel),
		descriptors: make(map[string]Descriptor),
		dispatcher:  NewDispatcher(),
	}

	for _, opt := range opts {
		opt(r)
	}

	// Register Core built-in channels with their metadata descriptors
	_ = r.RegisterChannelWithDescriptor(NewInAppChannel(db), Descriptor{
		Type:        "in_app",
		Title:       "In-App Notification",
		Description: "Deliver notifications in-app via event spine",
		Icon:        "bell",
	})
	_ = r.RegisterChannelWithDescriptor(NewEmailChannel(db, nil), Descriptor{
		Type:        "email",
		Title:       "Email",
		Description: "Deliver notifications via transactional email",
		Icon:        "mail",
	})
	_ = r.RegisterChannelWithDescriptor(NewTelegramChannel(db), Descriptor{
		Type:        "telegram",
		Title:       "Telegram",
		Description: "Deliver notifications via Telegram bot",
		Icon:        "send",
	})
	_ = r.RegisterChannelWithDescriptor(NewWebhookChannel(db), Descriptor{
		Type:        "webhook",
		Title:       "Webhook",
		Description: "Custom HTTP POST payload to any URL",
		Icon:        "webhook",
	})

	return r
}

// RegisterChannel registers or replaces a notification channel SPI driver.
func (r *Router) RegisterChannel(ch plugin.NotificationChannel) error {
	if ch == nil {
		return errors.New("notification: nil channel")
	}
	name := strings.ToLower(strings.TrimSpace(ch.Name()))
	if name == "" {
		return errors.New("notification: empty channel name")
	}

	title := ch.DisplayName()
	if title == "" {
		title = name
		if len(name) > 0 {
			title = strings.ToUpper(name[:1]) + name[1:]
		}
	}

	desc := Descriptor{
		Type:        name,
		Title:       title,
		Description: fmt.Sprintf("Deliver notifications via %s", title),
		Icon:        "bell",
	}

	return r.RegisterChannelWithDescriptor(ch, desc)
}

// RegisterChannelWithDescriptor registers or replaces a notification channel along with its metadata descriptor.
func (r *Router) RegisterChannelWithDescriptor(ch plugin.NotificationChannel, desc Descriptor) error {
	if ch == nil {
		return errors.New("notification: nil channel")
	}
	name := strings.ToLower(strings.TrimSpace(ch.Name()))
	if name == "" {
		return errors.New("notification: empty channel name")
	}

	desc.Type = name
	if desc.Title == "" {
		desc.Title = ch.DisplayName()
	}
	if desc.Title == "" {
		desc.Title = strings.ToUpper(name[:1]) + name[1:]
	}
	if desc.Icon == "" {
		desc.Icon = "bell"
	}
	if desc.Description == "" {
		desc.Description = fmt.Sprintf("Deliver notifications via %s", desc.Title)
	}

	r.chMu.Lock()
	defer r.chMu.Unlock()
	r.channels[name] = ch
	r.descriptors[name] = desc
	return nil
}

// GetChannel looks up a registered channel by identifier name.
func (r *Router) GetChannel(name string) (plugin.NotificationChannel, bool) {
	r.chMu.RLock()
	defer r.chMu.RUnlock()
	ch, ok := r.channels[strings.ToLower(strings.TrimSpace(name))]
	return ch, ok
}

// ListChannels returns all registered notification channels sorted by name.
func (r *Router) ListChannels() []plugin.NotificationChannel {
	r.chMu.RLock()
	defer r.chMu.RUnlock()
	list := make([]plugin.NotificationChannel, 0, len(r.channels))
	for _, ch := range r.channels {
		list = append(list, ch)
	}
	sort.Slice(list, func(i, j int) bool {
		return list[i].Name() < list[j].Name()
	})
	return list
}

// Descriptors returns all registered notification channel type descriptors sorted by type.
func (r *Router) Descriptors() []Descriptor {
	r.chMu.RLock()
	defer r.chMu.RUnlock()
	list := make([]Descriptor, 0, len(r.descriptors))
	for _, d := range r.descriptors {
		list = append(list, d)
	}
	sort.Slice(list, func(i, j int) bool {
		return list[i].Type < list[j].Type
	})
	return list
}

// GetDescriptor returns the descriptor for a registered channel type.
func (r *Router) GetDescriptor(name string) (Descriptor, bool) {
	r.chMu.RLock()
	defer r.chMu.RUnlock()
	d, ok := r.descriptors[strings.ToLower(strings.TrimSpace(name))]
	return d, ok
}

// SendDirect delivers a notification directly via a specific channel type.
func (r *Router) SendDirect(ctx context.Context, typ, cfgJSON, text string) error {
	typ = strings.ToLower(strings.TrimSpace(typ))
	ch, ok := r.GetChannel(typ)
	if !ok {
		return fmt.Errorf("unknown notification channel type: %s", typ)
	}

	var cfgMap map[string]interface{}
	if strings.TrimSpace(cfgJSON) != "" && strings.TrimSpace(cfgJSON) != "{}" {
		_ = json.Unmarshal([]byte(cfgJSON), &cfgMap)
	}
	if cfgMap == nil {
		cfgMap = make(map[string]interface{})
	}
	cfgMap["_raw_config"] = cfgJSON

	rec := plugin.NotificationRecipient{
		UserID: "1",
		OrgID:  "1",
		Config: cfgMap,
	}
	payload := plugin.NotificationPayload{
		EventType: "direct",
		Title:     text,
		Body:      text,
		Priority:  "normal",
	}

	return ch.Send(ctx, rec, payload)
}

// Dispatcher returns the underlying delivery dispatcher.
func (r *Router) Dispatcher() *Dispatcher {
	return r.dispatcher
}

// Wait blocks until all asynchronous delivery tasks across all channels complete.
func (r *Router) Wait() {
	if r.dispatcher != nil {
		r.dispatcher.Wait()
	}
}

// ResolveChannels finds the matching channel identifiers for a user and eventType.
func (r *Router) ResolveChannels(ctx context.Context, userID, eventType string) []string {
	channels, err := ResolveChannels(ctx, r.db, userID, eventType)
	if err != nil || len(channels) == 0 {
		return DefaultChannels
	}
	return channels
}

// EmitTo delivers payload to a specific recipient, routing to their configured or fallback channels.
func (r *Router) EmitTo(ctx context.Context, recipient plugin.NotificationRecipient, payload plugin.NotificationPayload) error {
	if strings.TrimSpace(payload.EventType) == "" {
		return errors.New("notification: eventType cannot be empty")
	}

	channels := r.ResolveChannels(ctx, recipient.UserID, payload.EventType)

	var dispatched int
	for _, chName := range channels {
		ch, ok := r.GetChannel(chName)
		if !ok {
			continue
		}
		r.dispatcher.Dispatch(ctx, ch, recipient, payload)
		dispatched++
	}

	if dispatched == 0 {
		return fmt.Errorf("notification: no active channel found for channels %v", channels)
	}

	return nil
}

// Emit broadcasts or targets a notification event based on payload.UserID or payload.OrgID,
// routing to each recipient's channels.
func (r *Router) Emit(ctx context.Context, payload plugin.NotificationPayload) error {
	if strings.TrimSpace(payload.EventType) == "" {
		return errors.New("notification: eventType cannot be empty")
	}

	// 1. Direct user target
	targetUser := strings.TrimSpace(payload.UserID)
	if targetUser == "" && payload.Data != nil {
		if u, ok := payload.Data["userId"].(string); ok {
			targetUser = strings.TrimSpace(u)
		}
	}
	if targetUser != "" {
		rec := plugin.NotificationRecipient{
			UserID: targetUser,
			OrgID:  payload.OrgID,
		}
		return r.EmitTo(ctx, rec, payload)
	}

	// 2. Org-scoped target: notify all workspace members
	targetOrg := strings.TrimSpace(payload.OrgID)
	if targetOrg == "" && payload.Data != nil {
		if o, ok := payload.Data["orgId"].(string); ok {
			targetOrg = strings.TrimSpace(o)
		}
	}

	if targetOrg != "" && r.db != nil {
		if orgNum, err := strconv.ParseUint(targetOrg, 10, 64); err == nil && orgNum > 0 {
			var members []models.OrgMember
			if err := r.db.WithContext(ctx).Where("org_id = ?", orgNum).Find(&members).Error; err == nil && len(members) > 0 {
				for _, m := range members {
					rec := plugin.NotificationRecipient{
						UserID: strconv.FormatUint(uint64(m.UserID), 10),
						OrgID:  targetOrg,
					}
					_ = r.EmitTo(ctx, rec, payload)
				}
				return nil
			}
		}
	}

	// 3. Fallback: instance admins or default user 1
	if r.db != nil {
		var admins []models.User
		if err := r.db.WithContext(ctx).Where("is_instance_admin = ?", true).Find(&admins).Error; err == nil && len(admins) > 0 {
			for _, a := range admins {
				rec := plugin.NotificationRecipient{
					UserID: strconv.FormatUint(uint64(a.ID), 10),
					OrgID:  "1",
				}
				_ = r.EmitTo(ctx, rec, payload)
			}
			return nil
		}
	}

	// Single default recipient fallback
	rec := plugin.NotificationRecipient{
		UserID: "1",
		OrgID:  "1",
	}
	return r.EmitTo(ctx, rec, payload)
}
