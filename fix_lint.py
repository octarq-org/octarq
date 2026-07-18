import os
import re

for filepath in ["app/app.go", "internal/api/api_test.go"]:
    if not os.path.exists(filepath): continue
    with open(filepath, "r") as f:
        content = f.read()

    orig = content
    content = content.replace('mailmodels "github.com/octarq-org/octarq/plugins/mail"\n', "")
    content = content.replace('mailmodels.', 'mail.')
    if "app.go" in filepath:
        content = content.replace('mailplugin.', 'mail.')
        content = content.replace('mailplugin "github.com/octarq-org/octarq/plugins/mail"', 'mail "github.com/octarq-org/octarq/plugins/mail"')

    if content != orig:
        with open(filepath, "w") as f:
            f.write(content)

