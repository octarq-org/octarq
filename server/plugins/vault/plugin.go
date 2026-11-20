package vault

import (
	"embed"
	"io/fs"
	"net/http"

	"github.com/octarq-org/octarq/server/plugin"
	"gorm.io/gorm"
)

//go:embed docs
var docs embed.FS

var (
	_ plugin.Plugin                = (*Plugin)(nil)
	_ plugin.Describer             = (*Plugin)(nil)
	_ plugin.HelpDocsFS            = (*Plugin)(nil)
	_ plugin.EncryptedDataReporter = (*Plugin)(nil)
	_ plugin.Validator             = (*Plugin)(nil)
	_ plugin.MCPProvider           = (*Plugin)(nil)
)

type Plugin struct {
	db    *gorm.DB
	host  plugin.Host
	audit func(r *http.Request, action, targetType string, targetID uint, meta map[string]any)
}

func New() *Plugin { return &Plugin{} }

func (p *Plugin) Name() string { return "vault" }

func (p *Plugin) Describe() plugin.Info {
	return plugin.Info{
		Title:            "Credential & Secrets Vault",
		Description:      "Unified vault for third-party integrations, environment secrets, SSH keys, database credentials, certificates, and TOTP codes.",
		Icon:             "key-round",
		Category:         plugin.CategoryUtilities,
		Core:             true,
		EnabledByDefault: true,
		Tags:             []string{"vault", "integrations", "secrets", "credentials"},
	}
}

func (p *Plugin) Models() []any {
	return []any{
		&VaultCollection{},
		&VaultItem{},
		&VaultItemVersion{},
		&VaultServiceToken{},
	}
}

func (p *Plugin) HelpDocsFS() fs.FS { return docs }

func (p *Plugin) Mount(mux plugin.Mux, ctx *plugin.Context) {
	if ctx == nil {
		return
	}
	p.db = ctx.DB
	p.audit = ctx.Audit
	p.host = plugin.EnsureHost(ctx)

	if ctx.Provide != nil {
		ctx.Provide(plugin.ServiceVault, plugin.VaultService(p))
		ctx.Provide(plugin.ServiceCredentialResolver, plugin.CredentialResolver(p))
		ctx.Provide(plugin.PurgeServiceName(p.Name()), plugin.PurgeFunc(p.purge))
		ctx.Provide(plugin.ExportServiceName(p.Name()), plugin.ExportFunc(p.exportData))
	}

	p.registerRoutes(mux, ctx)
}
