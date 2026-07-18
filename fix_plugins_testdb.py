import os
import re

for filepath in ["plugins/dns/verify_test.go", "plugins/links/links_test.go", "plugins/mail/wrap_links_test.go"]:
    if not os.path.exists(filepath): continue
    with open(filepath, "r") as f:
        content = f.read()
    orig = content

    if "plugins/dns" in filepath:
        # replace AutoMigrate
        content = content.replace("db.AutoMigrate(append(models.AllModels(), &models.Link{}, &models.LinkEvent{})...)",
            "db.AutoMigrate(append(models.AllModels(), append(New().Models(), links.New().Models()...)...)...)")
        content = content.replace("db.AutoMigrate(models.AllModels()...)",
            "db.AutoMigrate(append(models.AllModels(), append(New().Models(), links.New().Models()...)...)...)")
        
        if "links." in content and '"github.com/octarq-org/octarq/plugins/links"' not in content:
            content = content.replace("import (\n", "import (\n\tlinks \"github.com/octarq-org/octarq/plugins/links\"\n")

    elif "plugins/mail" in filepath:
        content = content.replace("db.AutoMigrate(append(models.AllModels(), &models.Link{}, &models.LinkEvent{})...)",
            "db.AutoMigrate(append(models.AllModels(), append(append(New().Models(), links.New().Models()...), dns.New().Models()...)...)...)")
        content = content.replace("db.AutoMigrate(models.AllModels()...)",
            "db.AutoMigrate(append(models.AllModels(), append(append(New().Models(), links.New().Models()...), dns.New().Models()...)...)...)")
        
        if "links." in content and '"github.com/octarq-org/octarq/plugins/links"' not in content:
            content = content.replace("import (\n", "import (\n\tlinks \"github.com/octarq-org/octarq/plugins/links\"\n")
        if "dns." in content and '"github.com/octarq-org/octarq/plugins/dns"' not in content:
            content = content.replace("import (\n", "import (\n\tdns \"github.com/octarq-org/octarq/plugins/dns\"\n")

    elif "plugins/links" in filepath:
        content = content.replace("db.AutoMigrate(models.AllModels()...)",
            "db.AutoMigrate(append(models.AllModels(), append(New().Models(), dns.New().Models()...)...)...)")
        if "dns." in content and '"github.com/octarq-org/octarq/plugins/dns"' not in content:
            content = content.replace("import (\n", "import (\n\tdns \"github.com/octarq-org/octarq/plugins/dns\"\n")
            
    if content != orig:
        with open(filepath, "w") as f:
            f.write(content)
