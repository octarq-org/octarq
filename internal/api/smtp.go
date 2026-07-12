package api

import (
	"context"
	"strings"

	"github.com/danielgtaylor/huma/v2"
	"github.com/danielgtaylor/huma/v2/adapters/humago"
	"github.com/octarq-org/octarq/internal/models"
)

type ListSMTPSendersInput struct {
	Ctx huma.Context `hidden:"true"`
}

func (i *ListSMTPSendersInput) Resolve(ctx huma.Context) []error {
	i.Ctx = ctx
	return nil
}

type ListSMTPSendersOutput struct {
	Body []models.SMTPSender
}

func (h *Handler) listSMTPSenders(ctx context.Context, input *ListSMTPSendersInput) (*ListSMTPSendersOutput, error) {
	if input.Ctx == nil {
		return nil, huma.Error500InternalServerError("Missing huma context")
	}
	r, _ := humago.Unwrap(input.Ctx)
	r, ok := h.auth.AuthenticateRequest(r)
	if !ok {
		return nil, huma.Error401Unauthorized("unauthorized")
	}
	var senders []models.SMTPSender
	h.orgDB(r).Order("name ASC").Find(&senders)
	for i := range senders {
		senders[i].PassSet = senders[i].Pass != ""
	}
	return &ListSMTPSendersOutput{Body: senders}, nil
}

type CreateSMTPSenderInput struct {
	Ctx  huma.Context `hidden:"true"`
	Body struct {
		Name      string `json:"name"`
		Host      string `json:"host"`
		Port      int    `json:"port"`
		User      string `json:"user"`
		Pass      string `json:"pass"`
		FromEmail string `json:"fromEmail"`
	}
}

func (i *CreateSMTPSenderInput) Resolve(ctx huma.Context) []error {
	i.Ctx = ctx
	return nil
}

type CreateSMTPSenderOutput struct {
	Body models.SMTPSender
}

func (h *Handler) createSMTPSender(ctx context.Context, input *CreateSMTPSenderInput) (*CreateSMTPSenderOutput, error) {
	if input.Ctx == nil {
		return nil, huma.Error500InternalServerError("Missing huma context")
	}
	r, _ := humago.Unwrap(input.Ctx)
	r, ok := h.auth.AuthenticateRequest(r)
	if !ok {
		return nil, huma.Error401Unauthorized("unauthorized")
	}
	name := strings.TrimSpace(input.Body.Name)
	host := strings.TrimSpace(input.Body.Host)
	user := strings.TrimSpace(input.Body.User)
	pass := input.Body.Pass
	if name == "" || host == "" || input.Body.Port == 0 || user == "" || pass == "" {
		return nil, huma.Error400BadRequest("name, host, port, user and pass are required")
	}

	encPass, err := h.cipher.Encrypt([]byte(pass))
	if err != nil {
		return nil, huma.Error500InternalServerError("encrypt failed")
	}

	sender := models.SMTPSender{
		OrgID:     h.orgID(r),
		Name:      name,
		Host:      host,
		Port:      input.Body.Port,
		User:      user,
		Pass:      encPass,
		FromEmail: strings.TrimSpace(input.Body.FromEmail),
	}

	if err := h.db.Create(&sender).Error; err != nil {
		return nil, huma.Error500InternalServerError("failed to save")
	}
	h.audit(r, "smtp.create", "smtp_sender", sender.ID, map[string]any{"name": sender.Name, "host": sender.Host})
	sender.PassSet = sender.Pass != ""
	return &CreateSMTPSenderOutput{Body: sender}, nil
}

type UpdateSMTPSenderInput struct {
	Ctx  huma.Context `hidden:"true"`
	ID   uint         `path:"id"`
	Body struct {
		Name      *string `json:"name,omitempty"`
		Host      *string `json:"host,omitempty"`
		Port      *int    `json:"port,omitempty"`
		User      *string `json:"user,omitempty"`
		Pass      *string `json:"pass,omitempty"` // optional on update
		FromEmail *string `json:"fromEmail,omitempty"`
	}
}

func (i *UpdateSMTPSenderInput) Resolve(ctx huma.Context) []error {
	i.Ctx = ctx
	return nil
}

type UpdateSMTPSenderOutput struct {
	Body models.SMTPSender
}

func (h *Handler) updateSMTPSender(ctx context.Context, input *UpdateSMTPSenderInput) (*UpdateSMTPSenderOutput, error) {
	if input.Ctx == nil {
		return nil, huma.Error500InternalServerError("Missing huma context")
	}
	r, _ := humago.Unwrap(input.Ctx)
	r, ok := h.auth.AuthenticateRequest(r)
	if !ok {
		return nil, huma.Error401Unauthorized("unauthorized")
	}

	var sender models.SMTPSender
	if h.db.Where("id = ? AND owner_id = ?", input.ID, h.orgID(r)).First(&sender).Error != nil {
		return nil, huma.Error404NotFound("not found")
	}

	if input.Body.Name != nil {
		sender.Name = strings.TrimSpace(*input.Body.Name)
	}
	if input.Body.Host != nil {
		sender.Host = strings.TrimSpace(*input.Body.Host)
	}
	if input.Body.Port != nil {
		sender.Port = *input.Body.Port
	}
	if input.Body.User != nil {
		sender.User = strings.TrimSpace(*input.Body.User)
	}
	if input.Body.FromEmail != nil {
		sender.FromEmail = strings.TrimSpace(*input.Body.FromEmail)
	}

	if input.Body.Pass != nil && *input.Body.Pass != "" {
		enc, err := h.cipher.Encrypt([]byte(*input.Body.Pass))
		if err != nil {
			return nil, huma.Error500InternalServerError("encrypt failed")
		}
		sender.Pass = enc
	}

	h.db.Save(&sender)
	meta := map[string]any{
		"name":      sender.Name,
		"host":      sender.Host,
		"port":      sender.Port,
		"user":      sender.User,
		"fromEmail": sender.FromEmail,
	}
	if input.Body.Pass != nil && *input.Body.Pass != "" {
		meta["pass"] = "[REDACTED]"
	}
	h.audit(r, "smtp.update", "smtp_sender", sender.ID, meta)
	sender.PassSet = sender.Pass != ""
	return &UpdateSMTPSenderOutput{Body: sender}, nil
}

type DeleteSMTPSenderInput struct {
	Ctx huma.Context `hidden:"true"`
	ID  uint         `path:"id"`
}

func (i *DeleteSMTPSenderInput) Resolve(ctx huma.Context) []error {
	i.Ctx = ctx
	return nil
}

type DeleteSMTPSenderOutput struct {
	Body map[string]bool
}

func (h *Handler) deleteSMTPSender(ctx context.Context, input *DeleteSMTPSenderInput) (*DeleteSMTPSenderOutput, error) {
	if input.Ctx == nil {
		return nil, huma.Error500InternalServerError("Missing huma context")
	}
	r, _ := humago.Unwrap(input.Ctx)
	r, ok := h.auth.AuthenticateRequest(r)
	if !ok {
		return nil, huma.Error401Unauthorized("unauthorized")
	}

	if res := h.db.Where("id = ? AND owner_id = ?", input.ID, h.orgID(r)).Delete(&models.SMTPSender{}); res.RowsAffected == 0 {
		return nil, huma.Error404NotFound("not found")
	}
	h.audit(r, "smtp.delete", "smtp_sender", input.ID, nil)
	return &DeleteSMTPSenderOutput{Body: map[string]bool{"ok": true}}, nil
}
