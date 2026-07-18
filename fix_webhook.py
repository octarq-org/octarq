with open("plugins/mail/webhook.go", "r") as f:
    content = f.read()

# Webhook functions should be methods on Plugin or a webhookEngine struct
# Let's see what methods it has.
content = content.replace("func (h *Handler)", "func (p *Plugin)")
content = content.replace("h.db.", "p.db.")
content = content.replace("h.OnEmail", "p.OnEmail")
content = content.replace("h.bus.", "p.bus.") # wait, does plugin have eventbus?
content = content.replace("mail.Email", "Email")
content = content.replace("mail.Mailbox", "Mailbox")
content = content.replace("mail.Attachment", "Attachment")
content = content.replace("mail.SMTPSender", "SMTPSender")
content = content.replace("mail.MessageIDHeader", "MessageIDHeader")

with open("plugins/mail/webhook.go", "w") as f:
    f.write(content)
