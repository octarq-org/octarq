package mail

import (
	"context"
	"fmt"
	"html"
	"net"
	"strings"

	"github.com/danielgtaylor/huma/v2"
	"github.com/danielgtaylor/huma/v2/adapters/humago"
	"github.com/octarq-org/octarq/plugin/safehttp"
)

func isRestrictedSMTPHostIP(ip net.IP) bool {
	return safehttp.DisallowedIP(ip)
}

// validateSMTPTarget provides an early, write-time error check for host and port when configuring an SMTP sender
// (returning 422 Unprocessable Entity immediately to the admin UI for obvious misconfigurations).
//
// This is NOT the security boundary against SSRF/TOCTOU/DNS rebinding; the actual security boundary is enforced at dial time in internal/mail.
func validateSMTPTarget(host string, port int) error {
	allowedPorts := map[int]bool{25: true, 465: true, 587: true, 2525: true}
	if !allowedPorts[port] {
		return huma.Error422UnprocessableEntity("port must be 25, 465, 587, or 2525")
	}

	host = strings.TrimSpace(host)
	if host == "" {
		return huma.Error422UnprocessableEntity("host is required")
	}

	lowerHost := strings.ToLower(host)
	if lowerHost == "localhost" || strings.HasSuffix(lowerHost, ".localhost") || strings.HasSuffix(lowerHost, ".local") {
		return huma.Error422UnprocessableEntity("host cannot be a private or restricted host address")
	}

	if ip := net.ParseIP(host); ip != nil {
		if isRestrictedSMTPHostIP(ip) {
			return huma.Error422UnprocessableEntity("host cannot be a private or restricted IP address")
		}
	}
	return nil
}

type ListSMTPSendersInput struct {
	Ctx huma.Context `hidden:"true"`
}

func (i *ListSMTPSendersInput) Resolve(ctx huma.Context) []error {
	i.Ctx = ctx
	return nil
}

type ListSMTPSendersOutput struct {
	Body []SMTPSender
}

func (p *Plugin) listSMTPSenders(ctx context.Context, input *ListSMTPSendersInput) (*ListSMTPSendersOutput, error) {
	if input.Ctx == nil {
		return nil, huma.Error500InternalServerError("Missing huma context")
	}
	r, _ := humago.Unwrap(input.Ctx)
	if p.orgID(r) == 0 {
		return nil, huma.Error401Unauthorized("unauthorized")
	}
	var senders []SMTPSender
	p.orgDB(r).Order("name ASC").Find(&senders)
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
	Body SMTPSender
}

func (p *Plugin) createSMTPSender(ctx context.Context, input *CreateSMTPSenderInput) (*CreateSMTPSenderOutput, error) {
	if input.Ctx == nil {
		return nil, huma.Error500InternalServerError("Missing huma context")
	}
	r, _ := humago.Unwrap(input.Ctx)
	if p.orgID(r) == 0 {
		return nil, huma.Error401Unauthorized("unauthorized")
	}
	if !p.hasRole(r, "admin") {
		return nil, huma.Error403Forbidden("forbidden: admin role required to create SMTP sender")
	}
	name := strings.TrimSpace(input.Body.Name)
	host := strings.TrimSpace(input.Body.Host)
	user := strings.TrimSpace(input.Body.User)
	pass := input.Body.Pass
	if name == "" || host == "" || input.Body.Port == 0 || user == "" || pass == "" {
		return nil, huma.Error400BadRequest("name, host, port, user and pass are required")
	}
	if err := validateSMTPTarget(host, input.Body.Port); err != nil {
		return nil, err
	}

	if p.encrypt == nil {
		return nil, huma.Error500InternalServerError("encrypt unavailable")
	}
	encPass, err := p.encrypt([]byte(pass))
	if err != nil {
		return nil, huma.Error500InternalServerError("encrypt failed")
	}

	sender := SMTPSender{
		OrgID:     p.orgID(r),
		Name:      name,
		Host:      host,
		Port:      input.Body.Port,
		User:      user,
		Pass:      encPass,
		FromEmail: strings.TrimSpace(input.Body.FromEmail),
	}

	if err := p.db.Create(&sender).Error; err != nil {
		return nil, huma.Error500InternalServerError("failed to save")
	}
	if p.audit != nil {
		p.audit(r, "smtp.create", "smtp_sender", sender.ID, map[string]any{"name": sender.Name, "host": sender.Host})
	}
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
	Body SMTPSender
}

func (p *Plugin) updateSMTPSender(ctx context.Context, input *UpdateSMTPSenderInput) (*UpdateSMTPSenderOutput, error) {
	if input.Ctx == nil {
		return nil, huma.Error500InternalServerError("Missing huma context")
	}
	r, _ := humago.Unwrap(input.Ctx)
	if p.orgID(r) == 0 {
		return nil, huma.Error401Unauthorized("unauthorized")
	}
	if !p.hasRole(r, "admin") {
		return nil, huma.Error403Forbidden("forbidden: admin role required to update SMTP sender")
	}

	var sender SMTPSender
	if p.db.Where("id = ? AND owner_id = ?", input.ID, p.orgID(r)).First(&sender).Error != nil {
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

	if err := validateSMTPTarget(sender.Host, sender.Port); err != nil {
		return nil, err
	}

	if input.Body.Pass != nil && *input.Body.Pass != "" {
		if p.encrypt == nil {
			return nil, huma.Error500InternalServerError("encrypt unavailable")
		}
		enc, err := p.encrypt([]byte(*input.Body.Pass))
		if err != nil {
			return nil, huma.Error500InternalServerError("encrypt failed")
		}
		sender.Pass = enc
	}

	p.db.Save(&sender)
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
	if p.audit != nil {
		p.audit(r, "smtp.update", "smtp_sender", sender.ID, meta)
	}
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

func (p *Plugin) deleteSMTPSender(ctx context.Context, input *DeleteSMTPSenderInput) (*DeleteSMTPSenderOutput, error) {
	if input.Ctx == nil {
		return nil, huma.Error500InternalServerError("Missing huma context")
	}
	r, _ := humago.Unwrap(input.Ctx)
	if p.orgID(r) == 0 {
		return nil, huma.Error401Unauthorized("unauthorized")
	}
	if !p.hasRole(r, "admin") {
		return nil, huma.Error403Forbidden("forbidden: admin role required to delete SMTP sender")
	}

	if res := p.db.Where("id = ? AND owner_id = ?", input.ID, p.orgID(r)).Delete(&SMTPSender{}); res.RowsAffected == 0 {
		return nil, huma.Error404NotFound("not found")
	}
	if p.audit != nil {
		p.audit(r, "smtp.delete", "smtp_sender", input.ID, nil)
	}
	return &DeleteSMTPSenderOutput{Body: map[string]bool{"ok": true}}, nil
}

type TestSMTPSenderInput struct {
	Ctx huma.Context `hidden:"true"`
	ID  uint         `path:"id"`
}

func (i *TestSMTPSenderInput) Resolve(ctx huma.Context) []error {
	i.Ctx = ctx
	return nil
}

type TestSMTPSenderOutput struct {
	Body map[string]bool
}

// testSMTPSender sends a real message from the sender to its own From address.
// A dial-only probe would miss the failure modes operators actually hit — bad
// password, relay refused, STARTTLS downgrade — so the test costs one real
// delivery, addressed to the configured From so no third party is involved.
func (p *Plugin) testSMTPSender(ctx context.Context, input *TestSMTPSenderInput) (*TestSMTPSenderOutput, error) {
	if input.Ctx == nil {
		return nil, huma.Error500InternalServerError("Missing huma context")
	}
	r, _ := humago.Unwrap(input.Ctx)
	if p.orgID(r) == 0 {
		return nil, huma.Error401Unauthorized("unauthorized")
	}
	if !p.hasRole(r, "admin") {
		return nil, huma.Error403Forbidden("forbidden: admin role required to test SMTP sender")
	}

	orgKey := fmt.Sprintf("org:%d", p.orgID(r))
	if p.testLimiter != nil {
		if !p.testLimiter.allow(orgKey) {
			return nil, huma.Error429TooManyRequests("too many test attempts; please wait before retrying")
		}
		p.testLimiter.recordFailure(orgKey)
	}

	var s SMTPSender
	if err := p.db.Where("id = ? AND owner_id = ?", input.ID, p.orgID(r)).First(&s).Error; err != nil {
		return nil, huma.Error404NotFound("not found")
	}
	if err := p.deliverVia(&s, s.FromEmail, "SMTP test from octarq",
		"<p>Your SMTP sender <strong>"+html.EscapeString(s.Name)+"</strong> is working.</p>",
		"Your SMTP sender '"+s.Name+"' is working."); err != nil {
		return nil, huma.Error400BadRequest(err.Error())
	}
	if p.audit != nil {
		p.audit(r, "smtp.test", "smtp_sender", s.ID, map[string]any{"to": s.FromEmail})
	}
	return &TestSMTPSenderOutput{Body: map[string]bool{"ok": true}}, nil
}
