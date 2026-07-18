import re
import os

with open("internal/models/models_test.go", "r") as f:
    content = f.read()

# Extract TestEffectiveHosts
effective_hosts_match = re.search(r'func TestEffectiveHosts\(t \*testing\.T\) \{.*?\n\}', content, re.DOTALL)
effective_hosts_code = effective_hosts_match.group(0) if effective_hosts_match else ""

# Extract TestRoutingRulesRoundTrip
routing_rules_match = re.search(r'func TestRoutingRulesRoundTrip\(t \*testing\.T\) \{.*?\n\}', content, re.DOTALL)
routing_rules_code = routing_rules_match.group(0) if routing_rules_match else ""

# Remove them from models_test.go
content = content.replace(effective_hosts_code, "")
content = content.replace(routing_rules_code, "")

with open("internal/models/models_test.go", "w") as f:
    f.write(content)

# Write to plugins/dns/models_test.go
if effective_hosts_code:
    effective_hosts_code = effective_hosts_code.replace("HostList", "models.HostList")
    dns_test_content = f"""package dns

import (
	"testing"
	"github.com/octarq-org/octarq/internal/models"
)

{effective_hosts_code}
"""
    with open("plugins/dns/models_test.go", "w") as f:
        f.write(dns_test_content)

# Write to plugins/links/models_test.go
if routing_rules_code:
    links_test_content = f"""package links

import (
	"testing"
)

{routing_rules_code}
"""
    with open("plugins/links/models_test.go", "w") as f:
        f.write(links_test_content)

