package builtin

import (
	"github.com/octarq-org/octarq/server/plugin"
	"github.com/octarq-org/octarq/server/plugins/help"
	"github.com/octarq-org/octarq/server/plugins/llmgateway"
	"github.com/octarq-org/octarq/server/plugins/notify"
	"github.com/octarq-org/octarq/server/plugins/ontology"
	"github.com/octarq-org/octarq/server/plugins/tenantquery"
	"github.com/octarq-org/octarq/server/plugins/vault"
)

func Default() []plugin.Plugin {
	return []plugin.Plugin{vault.New(), help.New(), tenantquery.New(), notify.New(), ontology.New(), llmgateway.New()}
}
