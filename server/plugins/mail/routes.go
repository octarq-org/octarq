package mail

import (
	"net/http"

	"github.com/danielgtaylor/huma/v2"
	"github.com/octarq-org/octarq/idempotency"
	"github.com/octarq-org/octarq/plugin"
)

func registerAPIRoutes(p *Plugin, api huma.API, ctx *plugin.Context) {
	huma.Register(api, huma.Operation{Method: "GET", Path: "/api/smtp-senders", Summary: "List SMTP Senders", Tags: []string{"SMTP"}}, p.listSMTPSenders)
	huma.Register(api, huma.Operation{Method: "POST", Path: "/api/smtp-senders", Summary: "Create SMTP Sender", Tags: []string{"SMTP"}, DefaultStatus: 201}, p.createSMTPSender)
	huma.Register(api, huma.Operation{Method: "PUT", Path: "/api/smtp-senders/{id}", Summary: "Update SMTP Sender", Tags: []string{"SMTP"}}, p.updateSMTPSender)
	huma.Register(api, huma.Operation{Method: "DELETE", Path: "/api/smtp-senders/{id}", Summary: "Delete SMTP Sender", Tags: []string{"SMTP"}}, p.deleteSMTPSender)
	huma.Register(api, huma.Operation{Method: "POST", Path: "/api/smtp-senders/{id}/test", Summary: "Test SMTP Sender", Tags: []string{"SMTP"}}, p.testSMTPSender)

	huma.Register(api, huma.Operation{Method: "GET", Path: "/api/mailboxes", Summary: "List Mailboxes", Tags: []string{"Mailboxes"}}, p.listMailboxes)
	huma.Register(api, huma.Operation{Method: "POST", Path: "/api/mailboxes", Summary: "Create Mailbox", Tags: []string{"Mailboxes"}, DefaultStatus: 201}, p.createMailbox)
	huma.Register(api, huma.Operation{Method: "PUT", Path: "/api/mailboxes/{id}", Summary: "Update Mailbox", Tags: []string{"Mailboxes"}}, p.updateMailbox)
	huma.Register(api, huma.Operation{Method: "DELETE", Path: "/api/mailboxes/{id}", Summary: "Delete Mailbox", Tags: []string{"Mailboxes"}}, p.deleteMailbox)

	huma.Register(api, huma.Operation{Method: "GET", Path: "/api/emails", Summary: "List Emails", Tags: []string{"Emails"}}, p.listEmails)
	huma.Register(api, huma.Operation{Method: "GET", Path: "/api/mail/emails", Summary: "List Emails (Mail API)", Tags: []string{"Emails"}}, p.listEmails)
	huma.Register(api, huma.Operation{Method: "POST", Path: "/api/emails/read-all", Summary: "Mark All Emails Read", Tags: []string{"Emails"}}, p.readAllEmails)
	huma.Register(api, huma.Operation{Method: "GET", Path: "/api/emails/{id}", Summary: "Get Email", Tags: []string{"Emails"}}, p.getEmail)
	huma.Register(api, huma.Operation{Method: "GET", Path: "/api/emails/{id}/raw", Summary: "Get Raw Email EML", Tags: []string{"Emails"}}, p.rawEmail)
	huma.Register(api, huma.Operation{Method: "GET", Path: "/api/emails/{id}/attachments/{index}", Summary: "Download Email Attachment", Tags: []string{"Emails"}}, p.getAttachment)
	huma.Register(api, huma.Operation{Method: "PUT", Path: "/api/emails/{id}", Summary: "Update Email State", Tags: []string{"Emails"}}, p.updateEmail)
	huma.Register(api, huma.Operation{Method: "PUT", Path: "/api/mail/emails/{id}/folder", Summary: "Update Email Folder", Tags: []string{"Emails"}}, p.updateEmailFolder)
	huma.Register(api, huma.Operation{Method: "PUT", Path: "/api/emails/{id}/folder", Summary: "Update Email Folder (Emails API)", Tags: []string{"Emails"}}, p.updateEmailFolder)
	huma.Register(api, huma.Operation{Method: "DELETE", Path: "/api/emails/{id}", Summary: "Delete Email", Tags: []string{"Emails"}}, p.deleteEmail)

	huma.Register(api, huma.Operation{Method: "GET", Path: "/api/mail/contacts", Summary: "List Contacts", Tags: []string{"Contacts"}}, p.listContacts)
	huma.Register(api, huma.Operation{Method: "POST", Path: "/api/mail/drafts", Summary: "Save Draft", Tags: []string{"Drafts"}, DefaultStatus: 200}, p.saveDraft)
	huma.Register(api, huma.Operation{Method: "DELETE", Path: "/api/mail/drafts/{id}", Summary: "Delete Draft", Tags: []string{"Drafts"}}, p.deleteDraft)

	var sendEmailMws huma.Middlewares
	if ctx.Lookup != nil {
		if idem, ok := plugin.LookupServiceAs[func(http.Handler) http.Handler](ctx.Lookup, idempotency.ServiceName); ok {
			sendEmailMws = append(sendEmailMws, idempotency.HumaMiddleware(idem))
		}
	}
	huma.Register(api, huma.Operation{
		Method: "POST", Path: "/api/emails/send", Summary: "Send Email", Tags: []string{"Emails"},
		Middlewares: sendEmailMws,
	}, p.sendEmail)

	huma.Register(api, huma.Operation{Method: "GET", Path: "/api/mail/suppressions", Summary: "List Mail Suppressions", Tags: []string{"Emails"}}, p.listSuppressions)
	huma.Register(api, huma.Operation{Method: "POST", Path: "/api/mail/suppressions", Summary: "Create Mail Suppression", Tags: []string{"Emails"}, DefaultStatus: 201}, p.createSuppression)
	huma.Register(api, huma.Operation{Method: "DELETE", Path: "/api/mail/suppressions/{id}", Summary: "Delete Mail Suppression", Tags: []string{"Emails"}}, p.deleteSuppression)

	huma.Register(api, huma.Operation{
		Method: "POST", Path: "/api/webhook/{orgSlug}/email/inbound/{token}",
		Summary: "Inbound Email Webhook", Tags: []string{"Mailboxes"},
		Metadata: map[string]any{"public": true},
	}, p.inbound)
	huma.Register(api, huma.Operation{
		Method: "POST", Path: "/api/webhook/{orgSlug}/email/inbound/raw/{token}",
		Summary: "Generic Inbound Email Webhook (Raw EML)", Tags: []string{"Mailboxes"},
		Metadata: map[string]any{"public": true},
	}, p.inboundGeneric)
	huma.Register(api, huma.Operation{
		Method: "POST", Path: "/api/webhook/{orgSlug}/email/bounce/{token}",
		Summary: "Email Bounce Webhook", Tags: []string{"Mailboxes"},
		Metadata: map[string]any{"public": true},
	}, p.emailBounceWebhook)
}
