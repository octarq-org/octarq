package mail

import (
	"context"
	"fmt"
	"html"
	"net"
	"strings"

	"github.com/danielgtaylor/huma/v2"
	"github.com/danielgtaylor/huma/v2/adapters/humago"
	"github.com/octarq-org/octarq/server/plugin/safehttp"
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
		return huma.Error422UnprocessableEntity("SMTP port must be 25, 465, 587, or 2525.")
	}

	host = strings.TrimSpace(host)
	if host == "" {
		return huma.Error422UnprocessableEntity("host is required. Enter a valid mail server hostname.")
	}

	lowerHost := strings.ToLower(host)
	if lowerHost == "localhost" || strings.HasSuffix(lowerHost, ".localhost") || strings.HasSuffix(lowerHost, ".local") {
		return huma.Error422UnprocessableEntity("SMTP host cannot be a private or restricted host address.")
	}

	if ip := net.ParseIP(host); ip != nil {
		if isRestrictedSMTPHostIP(ip) {
			return huma.Error422UnprocessableEntity("SMTP host cannot be a private or restricted IP address.")
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
		return nil, huma.Error500InternalServerError("An unexpected error occurred while processing the request context.")
	}
	r, _ := humago.Unwrap(input.Ctx)
	if p.orgID(r) == 0 {
		return nil, huma.Error401Unauthorized("Authentication required. Sign in to continue.")
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
		return nil, huma.Error500InternalServerError("An unexpected error occurred while processing the request context.")
	}
	r, _ := humago.Unwrap(input.Ctx)
	if p.orgID(r) == 0 {
		return nil, huma.Error401Unauthorized("Authentication required. Sign in to continue.")
	}
	if !p.hasRole(r, "admin") {
		return nil, huma.Error403Forbidden("Administrator role required to create an SMTP sender.")
	}
	name := strings.TrimSpace(input.Body.Name)
	host := strings.TrimSpace(input.Body.Host)
	user := strings.TrimSpace(input.Body.User)
	pass := input.Body.Pass
	if name == "" || host == "" || input.Body.Port == 0 || user == "" || pass == "" {
		return nil, huma.Error400BadRequest("Name, host, port, user, and password are required.")
	}
	if err := validateSMTPTarget(host, input.Body.Port); err != nil {
		return nil, err
	}

	if p.encrypt == nil {
		return nil, huma.Error500InternalServerError("Encryption service is unavailable. Verify encryption configuration.")
	}
	encPass, err := p.encrypt([]byte(pass))
	if err != nil {
		return nil, huma.Error500InternalServerError("Could not encrypt SMTP sender credentials. Try again later.")
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

	tdb := p.tenantDB(p.orgID(r))
	if tdb == nil {
		return nil, huma.Error401Unauthorized("Authentication required. Sign in to continue.")
	}
	if err := tdb.Create(&sender).Error; err != nil {
		return nil, huma.Error500InternalServerError("Could not save SMTP sender. Try again later.")
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
		return nil, huma.Error500InternalServerError("An unexpected error occurred while processing the request context.")
	}
	r, _ := humago.Unwrap(input.Ctx)
	if p.orgID(r) == 0 {
		return nil, huma.Error401Unauthorized("Authentication required. Sign in to continue.")
	}
	if !p.hasRole(r, "admin") {
		return nil, huma.Error403Forbidden("Administrator role required to update an SMTP sender.")
	}

	tdb := p.tenantDB(p.orgID(r))
	if tdb == nil {
		return nil, huma.Error401Unauthorized("Authentication required. Sign in to continue.")
	}
	var sender SMTPSender
	if tdb.Where("id = ?", input.ID).First(&sender).Error != nil {
		return nil, huma.Error404NotFound("SMTP sender not found.")
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
			return nil, huma.Error500InternalServerError("Encryption service is unavailable. Verify encryption configuration.")
		}
		enc, err := p.encrypt([]byte(*input.Body.Pass))
		if err != nil {
			return nil, huma.Error500InternalServerError("Could not encrypt SMTP sender credentials. Try again later.")
		}
		sender.Pass = enc
	}

	tdb.Save(&sender)
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
		return nil, huma.Error500InternalServerError("An unexpected error occurred while processing the request context.")
	}
	r, _ := humago.Unwrap(input.Ctx)
	if p.orgID(r) == 0 {
		return nil, huma.Error401Unauthorized("Authentication required. Sign in to continue.")
	}
	if !p.hasRole(r, "admin") {
		return nil, huma.Error403Forbidden("Administrator role required to delete an SMTP sender.")
	}

	tdb := p.tenantDB(p.orgID(r))
	if tdb == nil {
		return nil, huma.Error401Unauthorized("Authentication required. Sign in to continue.")
	}
	if res := tdb.Where("id = ?", input.ID).Delete(&SMTPSender{}); res.RowsAffected == 0 {
		return nil, huma.Error404NotFound("SMTP sender not found.")
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
		return nil, huma.Error500InternalServerError("An unexpected error occurred while processing the request context.")
	}
	r, _ := humago.Unwrap(input.Ctx)
	if p.orgID(r) == 0 {
		return nil, huma.Error401Unauthorized("Authentication required. Sign in to continue.")
	}
	if !p.hasRole(r, "admin") {
		return nil, huma.Error403Forbidden("Administrator role required to test an SMTP sender.")
	}

	orgKey := fmt.Sprintf("org:%d", p.orgID(r))
	if p.testLimiter != nil {
		if !p.testLimiter.allow(orgKey) {
			return nil, huma.Error429TooManyRequests("too many test attempts. Wait a moment before trying again.")
		}
		p.testLimiter.recordFailure(orgKey)
	}

	tdb := p.tenantDB(p.orgID(r))
	if tdb == nil {
		return nil, huma.Error401Unauthorized("Authentication required. Sign in to continue.")
	}
	var s SMTPSender
	if err := tdb.Where("id = ?", input.ID).First(&s).Error; err != nil {
		return nil, huma.Error404NotFound("SMTP sender not found.")
	}
	if err := p.deliverVia(&s, s.FromEmail, "SMTP test from octarq",
		"<p>Your SMTP sender <strong>"+html.EscapeString(s.Name)+"</strong> is working.</p>",
		"Your SMTP sender '"+s.Name+"' is working."); err != nil {
		return nil, huma.Error400BadRequest("Could not deliver SMTP test message: " + err.Error())
	}
	if p.audit != nil {
		p.audit(r, "smtp.test", "smtp_sender", s.ID, map[string]any{"to": s.FromEmail})
	}
	return &TestSMTPSenderOutput{Body: map[string]bool{"ok": true}}, nil
}
