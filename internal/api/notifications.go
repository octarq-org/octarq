package api

import (
	"context"
	"encoding/json"
	"strconv"
	"time"

	"github.com/danielgtaylor/huma/v2"
	"github.com/danielgtaylor/huma/v2/adapters/humago"
	"github.com/octarq-org/octarq/internal/models"
	"github.com/octarq-org/octarq/internal/notification"
	"github.com/octarq-org/octarq/plugin"
)

// --- In-App Notifications API ---

type ListNotificationsInput struct {
	Ctx        huma.Context `hidden:"true"`
	UnreadOnly bool         `query:"unread_only" doc:"Filter only unread notifications"`
	Limit      int          `query:"limit" doc:"Maximum records to return"`
	Offset     int          `query:"offset" doc:"Offset from start"`
}

func (i *ListNotificationsInput) Resolve(ctx huma.Context) []error {
	i.Ctx = ctx
	return nil
}

type ListNotificationsOutput struct {
	Body []models.Notification
}

func (h *Handler) listNotifications(ctx context.Context, input *ListNotificationsInput) (*ListNotificationsOutput, error) {
	if input.Ctx == nil {
		return nil, huma.Error500InternalServerError("Missing huma context")
	}
	r, _ := humago.Unwrap(input.Ctx)
	r, ok := h.auth.AuthenticateRequest(r)
	if !ok {
		return nil, huma.Error401Unauthorized("unauthorized")
	}

	uid := h.auth.UserID(r)
	if uid == 0 {
		return nil, huma.Error401Unauthorized("unauthorized")
	}

	uidStr := strconv.FormatUint(uint64(uid), 10)
	q := h.db.WithContext(ctx).Where("user_id = ?", uidStr).Order("created_at DESC")
	if input.UnreadOnly {
		q = q.Where("read_at IS NULL")
	}

	limit := plugin.PageLimit(input.Limit, 50, 500)
	offset := plugin.PageOffset(input.Offset)

	var items []models.Notification
	if err := q.Limit(limit).Offset(offset).Find(&items).Error; err != nil {
		return nil, huma.Error500InternalServerError("failed to list notifications")
	}

	return &ListNotificationsOutput{Body: items}, nil
}

type MarkNotificationReadInput struct {
	Ctx huma.Context `hidden:"true"`
	ID  uint         `path:"id" doc:"Notification ID to mark as read"`
}

func (i *MarkNotificationReadInput) Resolve(ctx huma.Context) []error {
	i.Ctx = ctx
	return nil
}

type SimpleOKOutputBody struct {
	OK bool `json:"ok"`
}

type MarkNotificationReadOutput struct {
	Body SimpleOKOutputBody
}

func (h *Handler) markNotificationRead(ctx context.Context, input *MarkNotificationReadInput) (*MarkNotificationReadOutput, error) {
	if input.Ctx == nil {
		return nil, huma.Error500InternalServerError("Missing huma context")
	}
	r, _ := humago.Unwrap(input.Ctx)
	r, ok := h.auth.AuthenticateRequest(r)
	if !ok {
		return nil, huma.Error401Unauthorized("unauthorized")
	}

	uid := h.auth.UserID(r)
	if uid == 0 {
		return nil, huma.Error401Unauthorized("unauthorized")
	}

	uidStr := strconv.FormatUint(uint64(uid), 10)
	var notif models.Notification
	if err := h.db.WithContext(ctx).Where("id = ? AND user_id = ?", input.ID, uidStr).First(&notif).Error; err != nil {
		return nil, huma.Error404NotFound("notification not found")
	}

	now := time.Now()
	notif.ReadAt = &now
	if err := h.db.WithContext(ctx).Save(&notif).Error; err != nil {
		return nil, huma.Error500InternalServerError("failed to mark notification as read")
	}

	return &MarkNotificationReadOutput{Body: SimpleOKOutputBody{OK: true}}, nil
}

type DeleteNotificationInput struct {
	Ctx huma.Context `hidden:"true"`
	ID  uint         `path:"id" doc:"Notification ID to delete"`
}

func (i *DeleteNotificationInput) Resolve(ctx huma.Context) []error {
	i.Ctx = ctx
	return nil
}

type DeleteNotificationOutput struct {
	Body SimpleOKOutputBody
}

func (h *Handler) deleteNotification(ctx context.Context, input *DeleteNotificationInput) (*DeleteNotificationOutput, error) {
	if input.Ctx == nil {
		return nil, huma.Error500InternalServerError("Missing huma context")
	}
	r, _ := humago.Unwrap(input.Ctx)
	r, ok := h.auth.AuthenticateRequest(r)
	if !ok {
		return nil, huma.Error401Unauthorized("unauthorized")
	}

	uid := h.auth.UserID(r)
	if uid == 0 {
		return nil, huma.Error401Unauthorized("unauthorized")
	}

	uidStr := strconv.FormatUint(uint64(uid), 10)
	var notif models.Notification
	if err := h.db.WithContext(ctx).Where("id = ? AND user_id = ?", input.ID, uidStr).First(&notif).Error; err != nil {
		return nil, huma.Error404NotFound("notification not found")
	}

	if err := h.db.WithContext(ctx).Delete(&notif).Error; err != nil {
		return nil, huma.Error500InternalServerError("failed to delete notification")
	}

	return &DeleteNotificationOutput{Body: SimpleOKOutputBody{OK: true}}, nil
}

// --- Notification Preferences API ---

type GetNotificationPreferencesInput struct {
	Ctx huma.Context `hidden:"true"`
}

func (i *GetNotificationPreferencesInput) Resolve(ctx huma.Context) []error {
	i.Ctx = ctx
	return nil
}

type GetNotificationPreferencesOutput struct {
	Body []models.NotificationPreference
}

func (h *Handler) getNotificationPreferences(ctx context.Context, input *GetNotificationPreferencesInput) (*GetNotificationPreferencesOutput, error) {
	if input.Ctx == nil {
		return nil, huma.Error500InternalServerError("Missing huma context")
	}
	r, _ := humago.Unwrap(input.Ctx)
	r, ok := h.auth.AuthenticateRequest(r)
	if !ok {
		return nil, huma.Error401Unauthorized("unauthorized")
	}

	uid := h.auth.UserID(r)
	if uid == 0 {
		return nil, huma.Error401Unauthorized("unauthorized")
	}

	uidStr := strconv.FormatUint(uint64(uid), 10)
	prefs, err := notification.GetPreferences(ctx, h.db, uidStr)
	if err != nil {
		return nil, huma.Error500InternalServerError("failed to load preferences")
	}

	return &GetNotificationPreferencesOutput{Body: prefs}, nil
}

type UpdateNotificationPreferencesInputBody struct {
	Preferences  []notification.PreferenceItem `json:"preferences,omitempty"`
	EventPattern string                        `json:"eventPattern,omitempty"`
	Channels     []string                      `json:"channels,omitempty"`
}

type UpdateNotificationPreferencesInput struct {
	Ctx  huma.Context `hidden:"true"`
	Body UpdateNotificationPreferencesInputBody
}

func (i *UpdateNotificationPreferencesInput) Resolve(ctx huma.Context) []error {
	i.Ctx = ctx
	return nil
}

type UpdateNotificationPreferencesOutput struct {
	Body []models.NotificationPreference
}

func (h *Handler) updateNotificationPreferences(ctx context.Context, input *UpdateNotificationPreferencesInput) (*UpdateNotificationPreferencesOutput, error) {
	if input.Ctx == nil {
		return nil, huma.Error500InternalServerError("Missing huma context")
	}
	r, _ := humago.Unwrap(input.Ctx)
	r, ok := h.auth.AuthenticateRequest(r)
	if !ok {
		return nil, huma.Error401Unauthorized("unauthorized")
	}

	uid := h.auth.UserID(r)
	if uid == 0 {
		return nil, huma.Error401Unauthorized("unauthorized")
	}

	uidStr := strconv.FormatUint(uint64(uid), 10)

	items := input.Body.Preferences
	if len(items) == 0 && input.Body.EventPattern != "" {
		items = []notification.PreferenceItem{
			{
				EventPattern: input.Body.EventPattern,
				Channels:     input.Body.Channels,
			},
		}
	}

	if err := notification.SavePreferences(ctx, h.db, uidStr, items); err != nil {
		return nil, huma.Error500InternalServerError("failed to save preferences")
	}

	prefs, err := notification.GetPreferences(ctx, h.db, uidStr)
	if err != nil {
		return nil, huma.Error500InternalServerError("failed to reload preferences")
	}

	return &UpdateNotificationPreferencesOutput{Body: prefs}, nil
}

// --- Registered Notification Channels API ---

type RegisteredChannelInfo struct {
	Name         string          `json:"name"`
	DisplayName  string          `json:"displayName"`
	ConfigSchema json.RawMessage `json:"configSchema"`
}

type ListRegisteredChannelsInput struct {
	Ctx huma.Context `hidden:"true"`
}

func (i *ListRegisteredChannelsInput) Resolve(ctx huma.Context) []error {
	i.Ctx = ctx
	return nil
}

type ListRegisteredChannelsOutput struct {
	Body []RegisteredChannelInfo
}

func (h *Handler) listRegisteredChannels(ctx context.Context, input *ListRegisteredChannelsInput) (*ListRegisteredChannelsOutput, error) {
	if input.Ctx == nil {
		return nil, huma.Error500InternalServerError("Missing huma context")
	}
	r, _ := humago.Unwrap(input.Ctx)
	_, ok := h.auth.AuthenticateRequest(r)
	if !ok {
		return nil, huma.Error401Unauthorized("unauthorized")
	}

	channels := notification.DefaultRouter().ListChannels()
	res := make([]RegisteredChannelInfo, 0, len(channels))
	for _, ch := range channels {
		res = append(res, RegisteredChannelInfo{
			Name:         ch.Name(),
			DisplayName:  ch.DisplayName(),
			ConfigSchema: ch.ConfigSchema(),
		})
	}

	return &ListRegisteredChannelsOutput{Body: res}, nil
}
