import os

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
            # Replace AutoMigrate(models.AllModels()...) with AutoMigrate(append(models.AllModels(), <plugin models>...)...)
            content = content.replace("db.AutoMigrate(models.AllModels()...)", f"db.AutoMigrate(append(models.AllModels(), {all_plugin_models_str})...)")
            # Fix existing append(models.AllModels(), &links.Link{}...)
            content = content.replace("append(models.AllModels(), &links.Link{}, &links.LinkEvent{})", f"append(models.AllModels(), {all_plugin_models_str})")
            
            if content != orig:
                import_dns = '\tdns "github.com/octarq-org/octarq/plugins/dns"\n'
                import_links = '\tlinks "github.com/octarq-org/octarq/plugins/links"\n'
                import_mail = '\tmailmodels "github.com/octarq-org/octarq/plugins/mail"\n'
                if import_dns not in content:
                    content = content.replace("import (\n", "import (\n" + import_dns)
                if import_links not in content:
                    content = content.replace("import (\n", "import (\n" + import_links)
                if import_mail not in content:
                    content = content.replace("import (\n", "import (\n" + import_mail)
                with open(filepath, "w") as f:
                    f.write(content)
