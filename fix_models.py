import os

filepath = "internal/models/models_test.go"
if os.path.exists(filepath):
    with open(filepath, "r") as f:
        content = f.read()

    orig = content
    content = content.replace("Domain", "dns.Domain")
    content = content.replace("RoutingRules", "links.RoutingRules")

    if content != orig:
        imports = ""
        if "dns." in content and '"github.com/octarq-org/octarq/plugins/dns"' not in content:
            imports += '\tdns "github.com/octarq-org/octarq/plugins/dns"\n'
        if "links." in content and '"github.com/octarq-org/octarq/plugins/links"' not in content:
            imports += '\tlinks "github.com/octarq-org/octarq/plugins/links"\n'
        if imports:
            content = content.replace("import (\n", "import (\n" + imports)
        with open(filepath, "w") as f:
            f.write(content)
