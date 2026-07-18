import os

for filepath in ["plugins/dns/verify_test.go", "plugins/mail/wrap_links_test.go"]:
    if not os.path.exists(filepath): continue
    with open(filepath, "r") as f:
        content = f.read()
    
    if "plugins/dns" in filepath:
        content = content.replace("db.AutoMigrate(append(models.AllModels(), &links.Link{}, &links.LinkEvent{})...)", 
                                  "db.AutoMigrate(append(models.AllModels(), &links.Link{}, &links.LinkEvent{}, &Domain{})...)")
    elif "plugins/mail" in filepath:
        content = content.replace("db.AutoMigrate(append(models.AllModels(), &links.Link{}, &links.LinkEvent{})...)", 
                                  "db.AutoMigrate(append(models.AllModels(), &links.Link{}, &links.LinkEvent{}, &dns.Domain{})...)")
        if "dns." in content and '"github.com/octarq-org/octarq/plugins/dns"' not in content:
            content = content.replace("import (\n", "import (\n\tdns \"github.com/octarq-org/octarq/plugins/dns\"\n")

    with open(filepath, "w") as f:
        f.write(content)
