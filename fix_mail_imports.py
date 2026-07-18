import os

replacements = {
    "mail.Mailbox": "mailmodels.Mailbox",
    "mail.Email": "mailmodels.Email",
    "mail.Attachment": "mailmodels.Attachment",
    "mail.SMTPSender": "mailmodels.SMTPSender",
    "mail.MessageIDHeader": "mailmodels.MessageIDHeader",
}

for root, _, files in os.walk("."):
    if "vendor" in root or ".git" in root or "node_modules" in root or "plugins/mail" in root:
        continue
    for file in files:
        if file.endswith(".go"):
            filepath = os.path.join(root, file)
            with open(filepath, "r") as f:
                content = f.read()
                
            orig = content
            for old, new in replacements.items():
                content = content.replace(old, new)
                
            if content != orig:
                # Add import mailmodels "github.com/octarq-org/octarq/plugins/mail"
                import_stmt = '\tmailmodels "github.com/octarq-org/octarq/plugins/mail"\n'
                if import_stmt not in content:
                    content = content.replace("import (\n", "import (\n" + import_stmt)
                with open(filepath, "w") as f:
                    f.write(content)
