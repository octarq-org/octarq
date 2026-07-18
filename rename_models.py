import os

replacements = {
    "models.Domain": "dns.Domain",
    "models.ProviderAccount": "dns.ProviderAccount",
    "models.Link": "links.Link",
    "models.LinkEvent": "links.LinkEvent",
    "models.RoutingRule": "links.RoutingRule",
    "models.RoutingRules": "links.RoutingRules",
    "models.Mailbox": "mail.Mailbox",
    "models.Email": "mail.Email",
    "models.Attachment": "mail.Attachment",
    "models.SMTPSender": "mail.SMTPSender",
    "models.MessageIDHeader": "mail.MessageIDHeader",
}

for root, _, files in os.walk("."):
    if "vendor" in root or ".git" in root or "node_modules" in root:
        continue
    for file in files:
        if file.endswith(".go") and "rename_models.py" not in file:
            filepath = os.path.join(root, file)
            with open(filepath, "r") as f:
                content = f.read()
                
            orig = content
            for old, new in replacements.items():
                content = content.replace(old, new)
                
            if content != orig:
                with open(filepath, "w") as f:
                    f.write(content)
