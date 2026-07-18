import os

fixes = {
    "dns": [("dns.Domain", "Domain"), ("dns.ProviderAccount", "ProviderAccount")],
    "links": [("links.Link", "Link"), ("links.LinkEvent", "LinkEvent"), ("links.RoutingRule", "RoutingRule")],
    "mail": [("mail.Mailbox", "Mailbox"), ("mail.Email", "Email"), ("mail.Attachment", "Attachment"), ("mail.SMTPSender", "SMTPSender")],
}

for plugin, pairs in fixes.items():
    folder = f"plugins/{plugin}"
    if not os.path.exists(folder): continue
    for file in os.listdir(folder):
        if not file.endswith(".go"): continue
        filepath = os.path.join(folder, file)
        with open(filepath, "r") as f:
            content = f.read()
        
        orig = content
        for old, new in pairs:
            # We want to replace `old` with `new`, but ONLY if they match exactly (e.g. `dns.Domain`)
            content = content.replace(old, new)
            
        if content != orig:
            with open(filepath, "w") as f:
                f.write(content)
