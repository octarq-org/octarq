package vault

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/octarq-org/octarq/server/plugin"
)

var _ plugin.MCPProvider = (*Plugin)(nil)

func (p *Plugin) MCPTier() string { return "" }

func mcpJSONResult(v any) (*mcp.CallToolResult, any, error) {
	b, err := json.Marshal(v)
	if err != nil {
		return nil, nil, err
	}
	return &mcp.CallToolResult{
		Content: []mcp.Content{&mcp.TextContent{Text: string(b)}},
	}, v, nil
}

type mcpListItemsArgs struct {
	Collection   string `json:"collection,omitempty"`
	Environment  string `json:"environment,omitempty"`
	ItemType     string `json:"itemType,omitempty"`
	ProviderType string `json:"providerType,omitempty"`
	Query        string `json:"query,omitempty"`
}

type mcpItemIDArgs struct {
	ItemID uint `json:"itemId"`
}

type mcpSetSecretArgs struct {
	CollectionSlug string         `json:"collectionSlug,omitempty"`
	Environment    string         `json:"environment,omitempty"`
	FolderPath     string         `json:"folderPath,omitempty"`
	ItemType       string         `json:"itemType,omitempty"`
	ProviderType   string         `json:"providerType,omitempty"`
	Provider       string         `json:"provider,omitempty"`
	KeyName        string         `json:"keyName,omitempty"`
	Name           string         `json:"name"`
	Payload        map[string]any `json:"payload"`
	IsDefault      bool           `json:"isDefault,omitempty"`
}

func (p *Plugin) RegisterMCP(srv *mcp.Server) {
	plugin.AddMCPTool(srv, "vault", &mcp.Tool{
		Name:        "octarq_vault__list_items",
		Description: "List vault credential metadata and vault:// references for the current workspace. Never returns plaintext secrets.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in mcpListItemsArgs) (*mcp.CallToolResult, any, error) {
		orgID := plugin.OrgIDFromContext(ctx)
		if orgID == 0 {
			return nil, nil, fmt.Errorf("vault: workspace context required")
		}
		items, err := p.ListItems(ctx, orgID, plugin.VaultFilter{
			Scope:          plugin.VaultScopeOrg,
			CollectionSlug: in.Collection,
			Environment:    in.Environment,
			ItemType:       in.ItemType,
			ProviderType:   in.ProviderType,
			Query:          in.Query,
		})
		if err != nil {
			return nil, nil, err
		}
		return mcpJSONResult(map[string]any{"items": items, "count": len(items)})
	})

	plugin.AddMCPTool(srv, "vault", &mcp.Tool{
		Name:        "octarq_vault__get_item_meta",
		Description: "Get non-secret metadata, field keys, version, and vault:// reference for one vault item.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in mcpItemIDArgs) (*mcp.CallToolResult, any, error) {
		orgID := plugin.OrgIDFromContext(ctx)
		if orgID == 0 || in.ItemID == 0 {
			return nil, nil, fmt.Errorf("vault: workspace context and itemId are required")
		}
		meta, err := p.GetMeta(ctx, orgID, in.ItemID)
		if err != nil {
			return nil, nil, err
		}
		if meta.Scope != plugin.VaultScopeOrg {
			return nil, nil, plugin.ErrSecretNotFound
		}
		return mcpJSONResult(meta)
	})

	plugin.AddMCPTool(srv, "vault", &mcp.Tool{
		Name:        "octarq_vault__set_secret",
		Description: "Create or rotate a secret item in the workspace vault (encrypted under a per-item DEK). Requires admin role.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in mcpSetSecretArgs) (*mcp.CallToolResult, any, error) {
		orgID := plugin.OrgIDFromContext(ctx)
		if orgID == 0 {
			return nil, nil, fmt.Errorf("vault: workspace context required")
		}
		role := strings.ToLower(plugin.RoleFromContext(ctx))
		if role != "owner" && role != "admin" {
			return nil, nil, fmt.Errorf("vault: admin role required to write secrets")
		}
		meta, err := p.PutSecret(ctx, plugin.PutSecretRequest{
			OrgID:          orgID,
			Scope:          plugin.VaultScopeOrg,
			CollectionSlug: in.CollectionSlug,
			Environment:    in.Environment,
			FolderPath:     in.FolderPath,
			ItemType:       in.ItemType,
			ProviderType:   in.ProviderType,
			Provider:       in.Provider,
			KeyName:        in.KeyName,
			Name:           in.Name,
			Payload:        in.Payload,
			IsDefault:      in.IsDefault,
			ChangeNote:     "Updated via MCP",
		})
		if err != nil {
			return nil, nil, err
		}
		return mcpJSONResult(meta)
	})

	plugin.AddMCPTool(srv, "vault", &mcp.Tool{
		Name:        "octarq_vault__test_integration",
		Description: "Verify connectivity or credential integrity of a vault item without exposing secret values.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in mcpItemIDArgs) (*mcp.CallToolResult, any, error) {
		orgID := plugin.OrgIDFromContext(ctx)
		if orgID == 0 || in.ItemID == 0 {
			return nil, nil, fmt.Errorf("vault: workspace context and itemId are required")
		}
		var row VaultItem
		if err := p.db.WithContext(ctx).Where("org_id = ? AND scope = ? AND id = ?", orgID, plugin.VaultScopeOrg, in.ItemID).First(&row).Error; err != nil {
			return nil, nil, plugin.ErrSecretNotFound
		}
		sec, err := p.decryptItem(ctx, row)
		if err != nil {
			return nil, nil, err
		}
		ok, msg := p.testVaultItem(ctx, row, sec.Payload)
		return mcpJSONResult(map[string]any{"ok": ok, "message": msg})
	})

	plugin.AddMCPTool(srv, "vault", &mcp.Tool{
		Name:        "octarq_vault__get_totp",
		Description: "Generate the current 6-digit RFC-6238 TOTP code for a vault 2FA item without exposing the seed.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in mcpItemIDArgs) (*mcp.CallToolResult, any, error) {
		orgID := plugin.OrgIDFromContext(ctx)
		if orgID == 0 || in.ItemID == 0 {
			return nil, nil, fmt.Errorf("vault: workspace context and itemId are required")
		}
		meta, err := p.GetMeta(ctx, orgID, in.ItemID)
		if err != nil {
			return nil, nil, err
		}
		if meta.Scope != plugin.VaultScopeOrg {
			return nil, nil, plugin.ErrSecretNotFound
		}
		code, rem, err := p.ComputeTOTP(ctx, orgID, in.ItemID)
		if err != nil {
			return nil, nil, err
		}
		return mcpJSONResult(map[string]any{"code": code, "remainingSeconds": rem})
	})
}
