import os
import re

replacements = {
    "models.Domain": "dns.Domain",
    "models.ProviderAccount": "dns.ProviderAccount",
    "models.Link": "links.Link",
    "models.LinkEvent": "links.LinkEvent",
    "models.RoutingRule": "links.RoutingRule",
    "models.RoutingRules": "links.RoutingRules",
    "models.Mailbox": "mailmodels.Mailbox",
    "models.Email": "mailmodels.Email",
    "models.SMTPSender": "mailmodels.SMTPSender",
    "models.MessageIDHeader": "mailmodels.MessageIDHeader",
}

for root, _, files in os.walk("."):
    if "vendor" in root or ".git" in root or "node_modules" in root:
        continue
    for file in files:
        if file.endswith("_test.go"):
            filepath = os.path.join(root, file)
            with open(filepath, "r") as f:
                content = f.read()
                
            orig = content
            
            # Determine package name
            pkg_match = re.search(r'^package\s+(\w+)', content, re.MULTILINE)
            pkg_name = pkg_match.group(1) if pkg_match else ""
            
            # Replace model references
            for old, new in replacements.items():
                if (pkg_name == "links" and new.startswith("links.")):
                    new = new.replace("links.", "")
                elif (pkg_name == "dns" and new.startswith("dns.")):
                    new = new.replace("dns.", "")
                elif (pkg_name == "mail" and new.startswith("mailmodels.")):
                    new = new.replace("mailmodels.", "")
                
                content = content.replace(old, new)
                
            # AutoMigrate patching (REMOVED Attachment)
            plugin_models = "&links.Link{}, &links.LinkEvent{}, &dns.Domain{}, &dns.ProviderAccount{}, &mailmodels.Mailbox{}, &mailmodels.Email{}, &mailmodels.SMTPSender{}"
            
            if pkg_name == "links":
                plugin_models = plugin_models.replace("links.", "")
            if pkg_name == "dns":
                plugin_models = plugin_models.replace("dns.", "")
            if pkg_name == "mail":
                plugin_models = plugin_models.replace("mailmodels.", "")
                
            content = content.replace("db.AutoMigrate(models.AllModels()...)", f"db.AutoMigrate(append(models.AllModels(), {plugin_models})...)")
            content = content.replace("gdb.AutoMigrate(models.AllModels()...)", f"gdb.AutoMigrate(append(models.AllModels(), {plugin_models})...)")
            content = content.replace("db.AutoMigrate(append(models.AllModels(), &models.Link{}, &models.LinkEvent{})...)", f"db.AutoMigrate(append(models.AllModels(), {plugin_models})...)")
            content = content.replace("db.AutoMigrate(append(models.AllModels(), &links.Link{}, &links.LinkEvent{})...)", f"db.AutoMigrate(append(models.AllModels(), {plugin_models})...)")
            
            # In sqlguard_test.go and audit_test.go, they had specific &models.Link{}
            content = content.replace("&models.Link{}", "&links.Link{}")
            content = content.replace("&models.LinkEvent{}", "&links.LinkEvent{}")
            
            if content != orig:
                if "dns." in content and '"github.com/octarq-org/octarq/plugins/dns"' not in content:
                    content = content.replace("import (\n", 'import (\n\tdns "github.com/octarq-org/octarq/plugins/dns"\n')
                if "links." in content and '"github.com/octarq-org/octarq/plugins/links"' not in content:
                    content = content.replace("import (\n", 'import (\n\tlinks "github.com/octarq-org/octarq/plugins/links"\n')
                if "mailmodels." in content and '"github.com/octarq-org/octarq/plugins/mail"' not in content:
                    content = content.replace("import (\n", 'import (\n\tmailmodels "github.com/octarq-org/octarq/plugins/mail"\n')
                
                with open(filepath, "w") as f:
                    f.write(content)
