import os
import re

all_plugin_models_str = "&links.Link{}, &links.LinkEvent{}, &dns.Domain{}, &dns.ProviderAccount{}, &mailmodels.Mailbox{}, &mailmodels.Email{}, &mailmodels.Attachment{}, &mailmodels.SMTPSender{}"

for root, _, files in os.walk("."):
    if "vendor" in root or ".git" in root or "node_modules" in root:
        continue
    for file in files:
        if file.endswith("_test.go"):
            filepath = os.path.join(root, file)
            with open(filepath, "r") as f:
                content = f.read()
                
            orig = content
            # We want to replace db.AutoMigrate(models.AllModels()...) 
            # and db.AutoMigrate(append(models.AllModels(), &models.Link{}, &models.LinkEvent{})...)
            # and gdb.AutoMigrate(...) where it has models.AllModels()...
            
            # Simple approach: find db.AutoMigrate(models.AllModels()...) and gdb.AutoMigrate(models.AllModels()...)
            content = content.replace("db.AutoMigrate(models.AllModels()...)", f"db.AutoMigrate(append(models.AllModels(), {all_plugin_models_str})...)")
            content = content.replace("gdb.AutoMigrate(models.AllModels()...)", f"gdb.AutoMigrate(append(models.AllModels(), {all_plugin_models_str})...)")
            content = content.replace("db.AutoMigrate(append(models.AllModels(), &models.Link{}, &models.LinkEvent{})...)", f"db.AutoMigrate(append(models.AllModels(), {all_plugin_models_str})...)")
            
            # For mcp/sqlguard_test.go and mcp/audit_test.go, replace &models.Link{} with &links.Link{}
            content = content.replace("&models.Link{}", "&links.Link{}")
            content = content.replace("&models.LinkEvent{}", "&links.LinkEvent{}")
            content = content.replace("&models.Domain{}", "&dns.Domain{}")

            
            if content != orig:
                import_dns = '\tdns "github.com/octarq-org/octarq/plugins/dns"\n'
                import_links = '\tlinks "github.com/octarq-org/octarq/plugins/links"\n'
                import_mail = '\tmailmodels "github.com/octarq-org/octarq/plugins/mail"\n'
                
                # Check if they are already imported
                if "plugins/dns" not in content:
                    content = content.replace("import (\n", "import (\n" + import_dns)
                if "plugins/links" not in content:
                    content = content.replace("import (\n", "import (\n" + import_links)
                if "plugins/mail" not in content:
                    content = content.replace("import (\n", "import (\n" + import_mail)
                
                with open(filepath, "w") as f:
                    f.write(content)

