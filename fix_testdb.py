import os
import re

migrates_str = "db.AutoMigrate(append(models.AllModels(), append(append(dns.New().Models(), links.New().Models()...), mail.New().Models()...)...)...)"
g_migrates_str = "gdb.AutoMigrate(append(models.AllModels(), append(append(dns.New().Models(), links.New().Models()...), mail.New().Models()...)...)...)"

for root, _, files in os.walk("internal/"):
    for file in files:
        if file.endswith("_test.go"):
            filepath = os.path.join(root, file)
            with open(filepath, "r") as f:
                content = f.read()
                
            orig = content
            
            content = content.replace("db.AutoMigrate(models.AllModels()...)", migrates_str)
            content = content.replace("gdb.AutoMigrate(models.AllModels()...)", g_migrates_str)
            
            # replace &models.Link{} and &models.LinkEvent{} with models from plugins
            content = content.replace("&models.Link{}", "links.New().Models()...")
            content = content.replace("&models.Link{}, &models.LinkEvent{}", "links.New().Models()...")
            content = content.replace("gdb.AutoMigrate(links.New().Models()...)", "gdb.AutoMigrate(append(models.AllModels(), links.New().Models()...)...)")
            content = content.replace("gdb.AutoMigrate(links.New().Models()..., &models.AuditLog{})", "gdb.AutoMigrate(append(models.AllModels(), links.New().Models()...)...)")

            if content != orig:
                imports = ""
                if "dns." in content and '"github.com/octarq-org/octarq/plugins/dns"' not in content:
                    imports += '\tdns "github.com/octarq-org/octarq/plugins/dns"\n'
                if "links." in content and '"github.com/octarq-org/octarq/plugins/links"' not in content:
                    imports += '\tlinks "github.com/octarq-org/octarq/plugins/links"\n'
                if "mail." in content and '"github.com/octarq-org/octarq/plugins/mail"' not in content:
                    imports += '\tmail "github.com/octarq-org/octarq/plugins/mail"\n'
                if imports:
                    content = content.replace("import (\n", "import (\n" + imports)
                
                with open(filepath, "w") as f:
                    f.write(content)
