import os

def fix_file(filepath):
    with open(filepath, 'r') as f:
        content = f.read()

    orig = content

    if "plugins/mail" in filepath:
        content = content.replace("models.LinkEvent", "links.LinkEvent")
        content = content.replace("models.Link", "links.Link")
        content = content.replace("models.Domain", "dns.Domain")
        content = content.replace("db.AutoMigrate(append(models.AllModels(), &models.Link{}, &models.LinkEvent{})...)", 
                                  "db.AutoMigrate(append(models.AllModels(), &links.Link{}, &links.LinkEvent{}, &dns.Domain{})...)")
        if "links." in content and '"github.com/octarq-org/octarq/plugins/links"' not in content:
            content = content.replace("import (\n", "import (\n\tlinks \"github.com/octarq-org/octarq/plugins/links\"\n\tdns \"github.com/octarq-org/octarq/plugins/dns\"\n")

    elif "plugins/dns" in filepath:
        content = content.replace("models.LinkEvent", "links.LinkEvent")
        content = content.replace("models.Link", "links.Link")
        content = content.replace("models.Domain", "Domain")
        content = content.replace("db.AutoMigrate(append(models.AllModels(), &models.Link{}, &models.LinkEvent{})...)", 
                                  "db.AutoMigrate(append(models.AllModels(), &links.Link{}, &links.LinkEvent{}, &Domain{})...)")
        if "links." in content and '"github.com/octarq-org/octarq/plugins/links"' not in content:
            content = content.replace("import (\n", "import (\n\tlinks \"github.com/octarq-org/octarq/plugins/links\"\n")
            
    elif "plugins/links" in filepath:
        content = content.replace("models.LinkEvent", "LinkEvent")
        content = content.replace("models.Link", "Link")
        content = content.replace("models.Domain", "dns.Domain")
        content = content.replace("models.RoutingRules", "RoutingRules")
        content = content.replace("db.AutoMigrate(models.AllModels()...)", 
                                  "db.AutoMigrate(append(models.AllModels(), &Link{}, &LinkEvent{}, &dns.Domain{})...)")
        if "dns." in content and '"github.com/octarq-org/octarq/plugins/dns"' not in content:
            content = content.replace("import (\n", "import (\n\tdns \"github.com/octarq-org/octarq/plugins/dns\"\n")

    else:
        # For all internal/* tests
        content = content.replace("models.Domain", "dns.Domain")
        content = content.replace("models.ProviderAccount", "dns.ProviderAccount")
        content = content.replace("models.LinkEvent", "links.LinkEvent")
        content = content.replace("models.Link", "links.Link")
        content = content.replace("models.RoutingRule", "links.RoutingRule")
        content = content.replace("models.RoutingRules", "links.RoutingRules")
        content = content.replace("models.Mailbox", "mailmodels.Mailbox")
        content = content.replace("models.Email", "mailmodels.Email")
        content = content.replace("models.SMTPSender", "mailmodels.SMTPSender")

        migrates = "&links.Link{}, &links.LinkEvent{}, &dns.Domain{}, &dns.ProviderAccount{}, &mailmodels.Mailbox{}, &mailmodels.Email{}, &mailmodels.SMTPSender{}"
        content = content.replace("db.AutoMigrate(models.AllModels()...)", f"db.AutoMigrate(append(models.AllModels(), {migrates})...)")
        content = content.replace("gdb.AutoMigrate(models.AllModels()...)", f"gdb.AutoMigrate(append(models.AllModels(), {migrates})...)")

        if content != orig:
            imports = ""
            if "dns." in content and '"github.com/octarq-org/octarq/plugins/dns"' not in content:
                imports += '\tdns "github.com/octarq-org/octarq/plugins/dns"\n'
            if "links." in content and '"github.com/octarq-org/octarq/plugins/links"' not in content:
                imports += '\tlinks "github.com/octarq-org/octarq/plugins/links"\n'
            if "mailmodels." in content and '"github.com/octarq-org/octarq/plugins/mail"' not in content:
                imports += '\tmailmodels "github.com/octarq-org/octarq/plugins/mail"\n'
            if imports:
                content = content.replace("import (\n", "import (\n" + imports)

    if content != orig:
        with open(filepath, 'w') as f:
            f.write(content)

for root, _, files in os.walk("."):
    if "vendor" in root or ".git" in root or "node_modules" in root:
        continue
    for file in files:
        if file.endswith("_test.go"):
            fix_file(os.path.join(root, file))

