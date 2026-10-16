package api

import (
	"context"
	"fmt"
	"net/mail"
	"strings"

	"github.com/danielgtaylor/huma/v2"
	"github.com/danielgtaylor/huma/v2/adapters/humago"
	"github.com/octarq-org/octarq/server/plugin"
)

type TestInstanceMailInputBody struct {
	To string `json:"to" required:"true" doc:"Recipient email address for the test email"`
}

type TestInstanceMailInput struct {
	Ctx  huma.Context `hidden:"true"`
	Body TestInstanceMailInputBody
}

func (i *TestInstanceMailInput) Resolve(ctx huma.Context) []error {
	i.Ctx = ctx
	return nil
}

type TestInstanceMailOutputBody struct {
	OK bool `json:"ok"`
}

type TestInstanceMailOutput struct {
	Body TestInstanceMailOutputBody
}

// testInstanceMail sends a test email to a recipient using the instance system mail sender.
// POST /api/instance/mail/test
//
// Requires instance administrator privileges. Returns 401 for unauthenticated
// requests, 403 for non-instance-admins, 400 for invalid recipient email or
// delivery errors, and 503 if no system mail sender is configured.
func (h *Handler) testInstanceMail(ctx context.Context, input *TestInstanceMailInput) (*TestInstanceMailOutput, error) {
	if input.Ctx == nil {
		return nil, huma.Error500InternalServerError("Request context is unavailable; retry the request")
	}
	r, _ := humago.Unwrap(input.Ctx)
	r, ok := h.auth.AuthenticateRequest(r)
	if !ok {
		return nil, huma.Error401Unauthorized("Authentication required; sign in to continue")
	}
	if !h.isInstanceAdmin(r) {
		return nil, huma.Error403Forbidden("Instance administrator role required; contact an instance administrator")
	}

	to := strings.ToLower(strings.TrimSpace(input.Body.To))
	if addr, err := mail.ParseAddress(to); err != nil || addr.Address != to || !strings.Contains(to, "@") {
		return nil, huma.Error400BadRequest("Valid recipient email address required; enter an address in the form user@example.com")
	}

	fn, ok := plugin.LookupServiceAs[plugin.SystemMailSender](h.LookupService, plugin.ServiceMailSendSystem)
	if !ok {
		return nil, huma.Error503ServiceUnavailable("System mail sender is not configured; set up an SMTP sender in instance settings")
	}

	subject := "Octarq System Mail Test"
	htmlBody := "<p>This is a test email sent from your octarq instance.</p>"
	textBody := "This is a test email sent from your octarq instance."

	if err := fn(to, subject, htmlBody, textBody); err != nil {
		return nil, huma.Error400BadRequest(fmt.Sprintf("Could not send test email: %v", err))
	}

	h.audit(r, "instance.mail_test", "system_mail", 0, map[string]any{"to": to})

	out := &TestInstanceMailOutput{}
	out.Body.OK = true
	return out, nil
}
