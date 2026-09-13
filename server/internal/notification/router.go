package notification

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"sync"

	"github.com/octarq-org/octarq/internal/models"
	"github.com/octarq-org/octarq/plugin"
	"gorm.io/gorm"
)

// Router routes notifications to registered channels based on event type and user preferences.
type Router struct {
	db         *gorm.DB
	channels   map[string]plugin.NotificationChannel
	chMu       sync.RWMutex
	dispatcher *Dispatcher
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

// NewRouter constructs a notification Router with built-in in_app and email channels.
func NewRouter(db *gorm.DB, opts ...RouterOption) *Router {
	r := &Router{
		db:         db,
		channels:   make(map[string]plugin.NotificationChannel),
		dispatcher: NewDispatcher(),
	}

	for _, opt := range opts {
		opt(r)
	}

	// Register Core built-in channels
	_ = r.RegisterChannel(NewInAppChannel(db))
	_ = r.RegisterChannel(NewEmailChannel(db, nil))

	return r
}

// RegisterChannel registers or replaces a notification channel SPI driver.
func (r *Router) RegisterChannel(ch plugin.NotificationChannel) error {
	if ch == nil {
		return errors.New("notification: nil channel")
	}
	name := strings.TrimSpace(ch.Name())
	if name == "" {
		return errors.New("notification: empty channel name")
	}

	r.chMu.Lock()
	defer r.chMu.Unlock()
	r.channels[name] = ch
	return nil
}

// GetChannel looks up a registered channel by identifier name.
func (r *Router) GetChannel(name string) (plugin.NotificationChannel, bool) {
	r.chMu.RLock()
	defer r.chMu.RUnlock()
	ch, ok := r.channels[strings.TrimSpace(name)]
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
