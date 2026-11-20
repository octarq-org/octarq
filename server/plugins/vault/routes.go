package vault

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/danielgtaylor/huma/v2"
	"github.com/danielgtaylor/huma/v2/adapters/humago"
	"github.com/octarq-org/octarq/server/plugin"
	"gorm.io/gorm"
)

func (p *Plugin) registerRoutes(mux plugin.Mux, ctx *plugin.Context) {
	if mux != nil {
		mux.Handle("GET /api/vault/raw", http.HandlerFunc(p.handleServiceTokenPull))
	}
	if ctx == nil || ctx.Huma == nil {
		return
	}
	api := ctx.Huma

	huma.Register(api, huma.Operation{OperationID: "vaultSchemas", Method: http.MethodGet, Path: "/api/vault/schemas", Summary: "List Vault Schemas", Tags: []string{"Vault"}}, p.listSchemas)

	huma.Register(api, huma.Operation{OperationID: "vaultCollectionsList", Method: http.MethodGet, Path: "/api/vault/collections", Summary: "List Vault Collections", Tags: []string{"Vault"}}, p.listCollections)
	huma.Register(api, huma.Operation{OperationID: "vaultCollectionsCreate", Method: http.MethodPost, Path: "/api/vault/collections", Summary: "Create Vault Collection", Tags: []string{"Vault"}, DefaultStatus: http.StatusCreated}, p.createCollection)
	huma.Register(api, huma.Operation{OperationID: "vaultCollectionsDelete", Method: http.MethodDelete, Path: "/api/vault/collections/{id}", Summary: "Delete Vault Collection", Tags: []string{"Vault"}}, p.deleteCollection)

	huma.Register(api, huma.Operation{OperationID: "vaultItemsList", Method: http.MethodGet, Path: "/api/vault/items", Summary: "List Vault Items", Tags: []string{"Vault"}}, p.listItems)
	huma.Register(api, huma.Operation{OperationID: "vaultItemsCreate", Method: http.MethodPost, Path: "/api/vault/items", Summary: "Create Vault Item", Tags: []string{"Vault"}, DefaultStatus: http.StatusCreated}, p.createItem)
	huma.Register(api, huma.Operation{OperationID: "vaultItemsGet", Method: http.MethodGet, Path: "/api/vault/items/{id}", Summary: "Get Vault Item Metadata", Tags: []string{"Vault"}}, p.getItem)
	huma.Register(api, huma.Operation{OperationID: "vaultItemsUpdate", Method: http.MethodPut, Path: "/api/vault/items/{id}", Summary: "Update Vault Item", Tags: []string{"Vault"}}, p.updateItem)
	huma.Register(api, huma.Operation{OperationID: "vaultItemsDelete", Method: http.MethodDelete, Path: "/api/vault/items/{id}", Summary: "Delete Vault Item", Tags: []string{"Vault"}}, p.deleteItem)
	huma.Register(api, huma.Operation{OperationID: "vaultItemsReveal", Method: http.MethodPost, Path: "/api/vault/items/{id}/reveal", Summary: "Reveal Secret Payload", Tags: []string{"Vault"}}, p.revealItem)
	huma.Register(api, huma.Operation{OperationID: "vaultItemsVersions", Method: http.MethodGet, Path: "/api/vault/items/{id}/versions", Summary: "List Secret Versions", Tags: []string{"Vault"}}, p.listItemVersions)
	huma.Register(api, huma.Operation{OperationID: "vaultItemsRollback", Method: http.MethodPost, Path: "/api/vault/items/{id}/rollback/{version}", Summary: "Rollback Secret Version", Tags: []string{"Vault"}}, p.rollbackItem)
	huma.Register(api, huma.Operation{OperationID: "vaultItemsDefault", Method: http.MethodPost, Path: "/api/vault/items/{id}/default", Summary: "Set Default Integration", Tags: []string{"Vault"}}, p.setDefaultItem)
	huma.Register(api, huma.Operation{OperationID: "vaultItemsTest", Method: http.MethodPost, Path: "/api/vault/items/{id}/test", Summary: "Test Integration or Credential", Tags: []string{"Vault"}}, p.testItem)
	huma.Register(api, huma.Operation{OperationID: "vaultItemsTOTP", Method: http.MethodGet, Path: "/api/vault/items/{id}/totp", Summary: "Compute Live TOTP Code", Tags: []string{"Vault"}}, p.getItemTOTP)

	huma.Register(api, huma.Operation{OperationID: "vaultImport", Method: http.MethodPost, Path: "/api/vault/import", Summary: "Bulk Import Secrets", Tags: []string{"Vault"}}, p.importSecrets)
	huma.Register(api, huma.Operation{OperationID: "vaultExport", Method: http.MethodGet, Path: "/api/vault/export", Summary: "Export Secrets", Tags: []string{"Vault"}}, p.exportSecrets)

	huma.Register(api, huma.Operation{OperationID: "vaultTokensList", Method: http.MethodGet, Path: "/api/vault/tokens", Summary: "List Service Tokens", Tags: []string{"Vault"}}, p.listServiceTokens)
	huma.Register(api, huma.Operation{OperationID: "vaultTokensCreate", Method: http.MethodPost, Path: "/api/vault/tokens", Summary: "Create Service Token", Tags: []string{"Vault"}, DefaultStatus: http.StatusCreated}, p.createServiceToken)
	huma.Register(api, huma.Operation{OperationID: "vaultTokensRevoke", Method: http.MethodDelete, Path: "/api/vault/tokens/{id}", Summary: "Revoke Service Token", Tags: []string{"Vault"}}, p.revokeServiceToken)

	huma.Register(api, huma.Operation{OperationID: "instanceVaultList", Method: http.MethodGet, Path: "/api/instance/vault/items", Summary: "List Instance Secrets", Tags: []string{"Instance Vault"}}, p.listInstanceItems)
	huma.Register(api, huma.Operation{OperationID: "instanceVaultPut", Method: http.MethodPost, Path: "/api/instance/vault/items", Summary: "Upsert Instance Secret", Tags: []string{"Instance Vault"}}, p.putInstanceItem)
	huma.Register(api, huma.Operation{OperationID: "instanceVaultReveal", Method: http.MethodPost, Path: "/api/instance/vault/items/{id}/reveal", Summary: "Reveal Instance Secret", Tags: []string{"Instance Vault"}}, p.revealInstanceItem)
	huma.Register(api, huma.Operation{OperationID: "instanceVaultDelete", Method: http.MethodDelete, Path: "/api/instance/vault/items/{id}", Summary: "Delete Instance Secret", Tags: []string{"Instance Vault"}}, p.deleteInstanceItem)
}

func (p *Plugin) callerOrg(ctx huma.Context, permKey, minRole string) (*http.Request, uint, uint, error) {
	r, _ := humago.Unwrap(ctx)
	if r == nil || p.host == nil || p.host.Session() == nil {
		return nil, 0, 0, huma.Error401Unauthorized("Authentication required. Sign in to continue.")
	}
	sess := p.host.Session()
	uid := sess.UserID(r)
	if uid == 0 {
		return nil, 0, 0, huma.Error401Unauthorized("Authentication required. Sign in to continue.")
	}
	orgID := sess.OrgID(r)
	if orgID == 0 {
		return nil, 0, 0, huma.Error400BadRequest("Workspace context required. Select an organization first.")
	}
	if !sess.RequirePerm(r, permKey, minRole) {
		return nil, 0, 0, huma.Error403Forbidden("Insufficient workspace permissions.")
	}
	if p.db == nil {
		return nil, 0, 0, huma.Error500InternalServerError("Vault database unavailable.")
	}
	return r, orgID, uid, nil
}

func (p *Plugin) callerInstanceAdmin(ctx huma.Context) (*http.Request, uint, error) {
	r, _ := humago.Unwrap(ctx)
	if r == nil || p.host == nil || p.host.Session() == nil {
		return nil, 0, huma.Error401Unauthorized("Authentication required. Sign in to continue.")
	}
	sess := p.host.Session()
	uid := sess.UserID(r)
	if uid == 0 {
		return nil, 0, huma.Error401Unauthorized("Authentication required. Sign in to continue.")
	}
	if !sess.IsInstanceAdmin(r) {
		return nil, 0, huma.Error403Forbidden("Instance administrator role required.")
	}
	if p.db == nil {
		return nil, 0, huma.Error500InternalServerError("Vault database unavailable.")
	}
	return r, uid, nil
}

func (p *Plugin) recordAudit(r *http.Request, action, targetType string, targetID uint, meta map[string]any) {
	if p.audit != nil && r != nil {
		p.audit(r, action, targetType, targetID, meta)
	}
}

type ctxInput struct {
	Ctx huma.Context `hidden:"true"`
}

func (i *ctxInput) Resolve(ctx huma.Context) []error { i.Ctx = ctx; return nil }

type SchemasOutput struct{ Body []CategorySchema }

func (p *Plugin) listSchemas(_ context.Context, _ *ctxInput) (*SchemasOutput, error) {
	return &SchemasOutput{Body: SchemasCatalog()}, nil
}

type CollectionsOutput struct{ Body []VaultCollection }

func (p *Plugin) listCollections(ctx context.Context, in *ctxInput) (*CollectionsOutput, error) {
	_, orgID, _, err := p.callerOrg(in.Ctx, "vault.item.read", "member")
	if err != nil {
		return nil, err
	}
	_, _ = p.ensureCollection(ctx, orgID, plugin.VaultScopeOrg, "integrations")
	_, _ = p.ensureCollection(ctx, orgID, plugin.VaultScopeOrg, "default")

	var cols []VaultCollection
	if err := p.db.WithContext(ctx).Where("org_id = ? AND scope = ?", orgID, plugin.VaultScopeOrg).Order("id ASC").Find(&cols).Error; err != nil {
		return nil, huma.Error500InternalServerError("Failed to list vault collections.")
	}
	return &CollectionsOutput{Body: cols}, nil
}

type CreateCollectionInput struct {
	Ctx  huma.Context `hidden:"true"`
	Body struct {
		Slug        string `json:"slug,omitempty"`
		Name        string `json:"name"`
		Kind        string `json:"kind,omitempty"`
		Description string `json:"description,omitempty"`
		Icon        string `json:"icon,omitempty"`
	}
}

func (i *CreateCollectionInput) Resolve(ctx huma.Context) []error { i.Ctx = ctx; return nil }

type CollectionOutput struct{ Body VaultCollection }

func (p *Plugin) createCollection(ctx context.Context, in *CreateCollectionInput) (*CollectionOutput, error) {
	r, orgID, uid, err := p.callerOrg(in.Ctx, "vault.item.create", "admin")
	if err != nil {
		return nil, err
	}
	name := strings.TrimSpace(in.Body.Name)
	if name == "" {
		return nil, huma.Error400BadRequest("Collection name is required.")
	}
	slug := normalizeSlug(in.Body.Slug, normalizeSlug(name, "vault"))
	kind := strings.TrimSpace(in.Body.Kind)
	if kind == "" {
		kind = "project"
	}
	col := VaultCollection{
		OrgID:       orgID,
		Scope:       plugin.VaultScopeOrg,
		Slug:        slug,
		Name:        name,
		Kind:        kind,
		Description: strings.TrimSpace(in.Body.Description),
		Icon:        strings.TrimSpace(in.Body.Icon),
		CreatedBy:   uid,
	}
	if err := p.db.WithContext(ctx).Create(&col).Error; err != nil {
		return nil, huma.Error409Conflict("A collection with this slug already exists.")
	}
	p.recordAudit(r, "vault.collection.created", "vault_collection", col.ID, map[string]any{"slug": col.Slug})
	return &CollectionOutput{Body: col}, nil
}

type IDPathInput struct {
	ID  uint         `path:"id"`
	Ctx huma.Context `hidden:"true"`
}

func (i *IDPathInput) Resolve(ctx huma.Context) []error { i.Ctx = ctx; return nil }

type OKOutput struct {
	Body struct {
		OK      bool   `json:"ok"`
		Message string `json:"message"`
	}
}

func (p *Plugin) deleteCollection(ctx context.Context, in *IDPathInput) (*OKOutput, error) {
	r, orgID, _, err := p.callerOrg(in.Ctx, "vault.item.delete", "owner")
	if err != nil {
		return nil, err
	}
	var col VaultCollection
	if err := p.db.WithContext(ctx).Where("org_id = ? AND scope = ? AND id = ?", orgID, plugin.VaultScopeOrg, in.ID).First(&col).Error; err != nil {
		return nil, huma.Error404NotFound("Collection not found.")
	}
	var itemCount int64
	p.db.WithContext(ctx).Model(&VaultItem{}).Where("org_id = ? AND collection_id = ?", orgID, col.ID).Count(&itemCount)
	if itemCount > 0 {
		return nil, huma.Error409Conflict("Collection still contains vault items. Delete or move items first.")
	}
	if err := p.db.WithContext(ctx).Delete(&col).Error; err != nil {
		return nil, huma.Error500InternalServerError("Failed to delete collection.")
	}
	p.recordAudit(r, "vault.collection.deleted", "vault_collection", col.ID, map[string]any{"slug": col.Slug})
	out := &OKOutput{}
	out.Body.OK = true
	out.Body.Message = "Collection deleted."
	return out, nil
}

type ListItemsInput struct {
	Collection   string       `query:"collection"`
	Environment  string       `query:"environment"`
	FolderPath   string       `query:"folder_path"`
	ItemType     string       `query:"item_type"`
	ProviderType string       `query:"provider_type"`
	Provider     string       `query:"provider"`
	Scope        string       `query:"scope"`
	Query        string       `query:"q"`
	Ctx          huma.Context `hidden:"true"`
}

func (i *ListItemsInput) Resolve(ctx huma.Context) []error { i.Ctx = ctx; return nil }

type ItemsOutput struct{ Body []plugin.VaultItemMeta }

func (p *Plugin) listItems(ctx context.Context, in *ListItemsInput) (*ItemsOutput, error) {
	_, orgID, _, err := p.callerOrg(in.Ctx, "vault.item.read", "member")
	if err != nil {
		return nil, err
	}
	if in.Scope != "" && !strings.EqualFold(in.Scope, string(plugin.VaultScopeOrg)) {
		return nil, huma.Error400BadRequest("Invalid vault scope.")
	}
	items, err := p.ListItems(ctx, orgID, plugin.VaultFilter{
		Scope:          plugin.VaultScopeOrg,
		CollectionSlug: in.Collection,
		Environment:    in.Environment,
		FolderPath:     in.FolderPath,
		ItemType:       in.ItemType,
		ProviderType:   in.ProviderType,
		Provider:       in.Provider,
		Query:          in.Query,
	})
	if err != nil {
		return nil, huma.Error500InternalServerError("Failed to list vault items.")
	}
	return &ItemsOutput{Body: items}, nil
}

type CreateItemInput struct {
	Ctx  huma.Context `hidden:"true"`
	Body struct {
		Scope                string         `json:"scope,omitempty"`
		CollectionSlug       string         `json:"collectionSlug,omitempty"`
		Environment          string         `json:"environment,omitempty"`
		FolderPath           string         `json:"folderPath,omitempty"`
		ItemType             string         `json:"itemType,omitempty"`
		ProviderType         string         `json:"providerType,omitempty"`
		Provider             string         `json:"provider,omitempty"`
		KeyName              string         `json:"keyName,omitempty"`
		Name                 string         `json:"name"`
		Tags                 []string       `json:"tags,omitempty"`
		IsDefault            bool           `json:"isDefault,omitempty"`
		Metadata             map[string]any `json:"metadata,omitempty"`
		Payload              map[string]any `json:"payload"`
		ChangeNote           string         `json:"changeNote,omitempty"`
		ExpiresAt            *time.Time     `json:"expiresAt,omitempty"`
		RotationIntervalDays int            `json:"rotationIntervalDays,omitempty"`
	}
}

func (i *CreateItemInput) Resolve(ctx huma.Context) []error { i.Ctx = ctx; return nil }

type ItemOutput struct{ Body plugin.VaultItemMeta }

func (p *Plugin) createItem(ctx context.Context, in *CreateItemInput) (*ItemOutput, error) {
	b := in.Body
	r, orgID, uid, err := p.callerOrg(in.Ctx, "vault.item.create", "admin")
	if err != nil {
		return nil, err
	}
	if b.Scope != "" && !strings.EqualFold(b.Scope, string(plugin.VaultScopeOrg)) {
		return nil, huma.Error400BadRequest("Invalid vault scope.")
	}
	meta, err := p.PutSecret(ctx, plugin.PutSecretRequest{
		OrgID:                orgID,
		Scope:                plugin.VaultScopeOrg,
		CollectionSlug:       b.CollectionSlug,
		Environment:          b.Environment,
		FolderPath:           b.FolderPath,
		ItemType:             b.ItemType,
		ProviderType:         b.ProviderType,
		Provider:             b.Provider,
		KeyName:              b.KeyName,
		Name:                 b.Name,
		Tags:                 b.Tags,
		IsDefault:            b.IsDefault,
		Metadata:             b.Metadata,
		Payload:              b.Payload,
		ChangeNote:           b.ChangeNote,
		ActorUserID:          uid,
		ExpiresAt:            b.ExpiresAt,
		RotationIntervalDays: b.RotationIntervalDays,
	})
	if err != nil {
		return nil, huma.Error400BadRequest(err.Error())
	}
	p.recordAudit(r, "vault.item.created", "vault_item", meta.ID, map[string]any{
		"scope": meta.Scope, "itemType": meta.ItemType, "providerType": meta.ProviderType, "provider": meta.Provider, "ref": meta.Ref,
	})
	return &ItemOutput{Body: *meta}, nil
}

func (p *Plugin) loadAndAuthorizeItemAccess(ctx context.Context, humaCtx huma.Context, permKey, orgMinRole string, itemID uint) (*http.Request, uint, uint, *plugin.VaultItemMeta, error) {
	r, orgID, uid, err := p.callerOrg(humaCtx, permKey, "member")
	if err != nil {
		return nil, 0, 0, nil, err
	}
	meta, err := p.GetMeta(ctx, orgID, itemID)
	if err != nil {
		if errors.Is(err, plugin.ErrSecretNotFound) {
			return nil, 0, 0, nil, huma.Error404NotFound("Vault item not found.")
		}
		return nil, 0, 0, nil, huma.Error500InternalServerError("Failed to read vault item.")
	}
	if meta.Scope != plugin.VaultScopeOrg {
		return nil, 0, 0, nil, huma.Error404NotFound("Vault item not found.")
	}
	if !p.host.Session().RequirePerm(r, permKey, orgMinRole) {
		return nil, 0, 0, nil, huma.Error403Forbidden("Insufficient workspace permissions.")
	}
	return r, orgID, uid, meta, nil
}

func (p *Plugin) getItem(ctx context.Context, in *IDPathInput) (*ItemOutput, error) {
	_, _, _, meta, err := p.loadAndAuthorizeItemAccess(ctx, in.Ctx, "vault.item.read", "member", in.ID)
	if err != nil {
		return nil, err
	}
	return &ItemOutput{Body: *meta}, nil
}

type UpdateItemInput struct {
	ID   uint         `path:"id"`
	Ctx  huma.Context `hidden:"true"`
	Body struct {
		Name       *string        `json:"name,omitempty"`
		IsDefault  *bool          `json:"isDefault,omitempty"`
		Metadata   map[string]any `json:"metadata,omitempty"`
		Payload    map[string]any `json:"payload,omitempty"`
		ChangeNote string         `json:"changeNote,omitempty"`
	}
}

func (i *UpdateItemInput) Resolve(ctx huma.Context) []error { i.Ctx = ctx; return nil }

func (p *Plugin) updateItem(ctx context.Context, in *UpdateItemInput) (*ItemOutput, error) {
	r, orgID, uid, _, err := p.loadAndAuthorizeItemAccess(ctx, in.Ctx, "vault.item.update", "admin", in.ID)
	if err != nil {
		return nil, err
	}
	meta, err := p.UpdateSecret(ctx, orgID, in.ID, in.Body.Name, in.Body.Metadata, in.Body.Payload, in.Body.IsDefault, in.Body.ChangeNote, uid)
	if err != nil {
		return nil, huma.Error400BadRequest(err.Error())
	}
	p.recordAudit(r, "vault.item.updated", "vault_item", meta.ID, map[string]any{"version": meta.CurrentVersion, "ref": meta.Ref})
	return &ItemOutput{Body: *meta}, nil
}

func (p *Plugin) deleteItem(ctx context.Context, in *IDPathInput) (*OKOutput, error) {
	r, orgID, _, existing, err := p.loadAndAuthorizeItemAccess(ctx, in.Ctx, "vault.item.delete", "owner", in.ID)
	if err != nil {
		return nil, err
	}
	if err := p.DeleteSecret(ctx, orgID, in.ID); err != nil {
		return nil, huma.Error500InternalServerError("Failed to delete vault item.")
	}
	p.recordAudit(r, "vault.item.deleted", "vault_item", in.ID, map[string]any{"ref": existing.Ref})
	out := &OKOutput{}
	out.Body.OK = true
	out.Body.Message = "Vault item permanently deleted and key shredded."
	return out, nil
}

type RevealOutput struct {
	Body struct {
		Meta    plugin.VaultItemMeta `json:"meta"`
		Version int                  `json:"version"`
		Payload map[string]any       `json:"payload"`
	}
}

func (p *Plugin) revealItem(ctx context.Context, in *IDPathInput) (*RevealOutput, error) {
	r, orgID, _, _, err := p.loadAndAuthorizeItemAccess(ctx, in.Ctx, "vault.secret.reveal", "admin", in.ID)
	if err != nil {
		return nil, err
	}
	sec, err := p.GetSecret(ctx, orgID, in.ID)
	if err != nil {
		if errors.Is(err, plugin.ErrSecretNotFound) {
			return nil, huma.Error404NotFound("Vault item not found.")
		}
		return nil, huma.Error500InternalServerError("Failed to decrypt secret payload.")
	}
	p.recordAudit(r, "vault.secret.revealed", "vault_item", sec.Meta.ID, map[string]any{"version": sec.Version, "ref": sec.Meta.Ref})
	out := &RevealOutput{}
	out.Body.Meta = sec.Meta
	out.Body.Version = sec.Version
	out.Body.Payload = sec.Payload
	return out, nil
}

type VersionDTO struct {
	Version    int       `json:"version"`
	CreatedBy  uint      `json:"createdBy,omitempty"`
	ChangeNote string    `json:"changeNote,omitempty"`
	CreatedAt  time.Time `json:"createdAt"`
}

type VersionsOutput struct{ Body []VersionDTO }

func (p *Plugin) listItemVersions(ctx context.Context, in *IDPathInput) (*VersionsOutput, error) {
	_, orgID, _, _, err := p.loadAndAuthorizeItemAccess(ctx, in.Ctx, "vault.item.read", "member", in.ID)
	if err != nil {
		return nil, err
	}
	var vers []VaultItemVersion
	if err := p.db.WithContext(ctx).Where("org_id = ? AND item_id = ?", orgID, in.ID).Order("version DESC").Find(&vers).Error; err != nil {
		return nil, huma.Error500InternalServerError("Failed to load version history.")
	}
	out := make([]VersionDTO, len(vers))
	for i, v := range vers {
		out[i] = VersionDTO{Version: v.Version, CreatedBy: v.CreatedBy, ChangeNote: v.ChangeNote, CreatedAt: v.CreatedAt}
	}
	return &VersionsOutput{Body: out}, nil
}

type RollbackInput struct {
	ID      uint         `path:"id"`
	Version int          `path:"version"`
	Ctx     huma.Context `hidden:"true"`
}

func (i *RollbackInput) Resolve(ctx huma.Context) []error { i.Ctx = ctx; return nil }

func (p *Plugin) rollbackItem(ctx context.Context, in *RollbackInput) (*ItemOutput, error) {
	r, orgID, uid, _, err := p.loadAndAuthorizeItemAccess(ctx, in.Ctx, "vault.item.update", "admin", in.ID)
	if err != nil {
		return nil, err
	}
	meta, err := p.RollbackVersion(ctx, orgID, in.ID, in.Version, uid)
	if err != nil {
		return nil, huma.Error400BadRequest(err.Error())
	}
	p.recordAudit(r, "vault.item.rolled_back", "vault_item", meta.ID, map[string]any{"targetVersion": in.Version, "newVersion": meta.CurrentVersion})
	return &ItemOutput{Body: *meta}, nil
}

func (p *Plugin) setDefaultItem(ctx context.Context, in *IDPathInput) (*ItemOutput, error) {
	r, orgID, uid, _, err := p.loadAndAuthorizeItemAccess(ctx, in.Ctx, "vault.item.update", "admin", in.ID)
	if err != nil {
		return nil, err
	}
	isDef := true
	meta, err := p.UpdateSecret(ctx, orgID, in.ID, nil, nil, nil, &isDef, "", uid)
	if err != nil {
		if errors.Is(err, plugin.ErrSecretNotFound) {
			return nil, huma.Error404NotFound("Vault item not found.")
		}
		return nil, huma.Error500InternalServerError("Failed to set default integration.")
	}
	p.recordAudit(r, "vault.item.default_set", "vault_item", meta.ID, map[string]any{"providerType": meta.ProviderType})
	return &ItemOutput{Body: *meta}, nil
}

func (p *Plugin) testItem(ctx context.Context, in *IDPathInput) (*OKOutput, error) {
	_, orgID, _, _, err := p.loadAndAuthorizeItemAccess(ctx, in.Ctx, "vault.item.read", "member", in.ID)
	if err != nil {
		return nil, err
	}
	var row VaultItem
	if err := p.db.WithContext(ctx).Where("org_id = ? AND id = ?", orgID, in.ID).First(&row).Error; err != nil {
		return nil, huma.Error404NotFound("Vault item not found.")
	}
	sec, err := p.decryptItem(ctx, row)
	if err != nil {
		return nil, huma.Error500InternalServerError("Failed to decrypt item credentials.")
	}
	ok, msg := p.testVaultItem(ctx, row, sec.Payload)
	if !ok {
		return nil, huma.Error400BadRequest(msg)
	}
	out := &OKOutput{}
	out.Body.OK = true
	out.Body.Message = msg
	return out, nil
}

type TOTPOutput struct {
	Body struct {
		Code             string `json:"code"`
		RemainingSeconds int    `json:"remainingSeconds"`
	}
}

func (p *Plugin) getItemTOTP(ctx context.Context, in *IDPathInput) (*TOTPOutput, error) {
	_, orgID, _, _, err := p.loadAndAuthorizeItemAccess(ctx, in.Ctx, "vault.item.read", "member", in.ID)
	if err != nil {
		return nil, err
	}
	code, rem, err := p.ComputeTOTP(ctx, orgID, in.ID)
	if err != nil {
		return nil, huma.Error400BadRequest(err.Error())
	}
	out := &TOTPOutput{}
	out.Body.Code = code
	out.Body.RemainingSeconds = rem
	return out, nil
}

type ImportSecretsInput struct {
	Ctx  huma.Context `hidden:"true"`
	Body struct {
		CollectionSlug string         `json:"collectionSlug,omitempty"`
		Environment    string         `json:"environment,omitempty"`
		FolderPath     string         `json:"folderPath,omitempty"`
		Format         string         `json:"format"`
		DotEnv         string         `json:"dotEnv,omitempty"`
		JSONItems      map[string]any `json:"jsonItems,omitempty"`
	}
}

func (i *ImportSecretsInput) Resolve(ctx huma.Context) []error { i.Ctx = ctx; return nil }

type ImportSecretsOutput struct {
	Body struct {
		Imported int `json:"imported"`
	}
}

func (p *Plugin) importSecrets(ctx context.Context, in *ImportSecretsInput) (*ImportSecretsOutput, error) {
	r, orgID, uid, err := p.callerOrg(in.Ctx, "vault.item.create", "admin")
	if err != nil {
		return nil, err
	}
	colSlug := normalizeSlug(in.Body.CollectionSlug, "default")
	env := normalizeEnvironment(in.Body.Environment)
	folder := normalizeFolderPath(in.Body.FolderPath)
	imported := 0

	switch strings.ToLower(strings.TrimSpace(in.Body.Format)) {
	case "env", "dotenv", "":
		entries, parseErr := parseDotEnv(in.Body.DotEnv)
		if parseErr != nil {
			return nil, huma.Error400BadRequest(parseErr.Error())
		}
		for _, e := range entries {
			if _, err := p.PutSecret(ctx, plugin.PutSecretRequest{
				OrgID:          orgID,
				Scope:          plugin.VaultScopeOrg,
				CollectionSlug: colSlug,
				Environment:    env,
				FolderPath:     folder,
				ItemType:       "kv_secret",
				KeyName:        e.Key,
				Name:           e.Key,
				Payload:        map[string]any{"value": e.Value},
				ChangeNote:     "Imported from .env",
				ActorUserID:    uid,
			}); err != nil {
				return nil, huma.Error400BadRequest(fmt.Sprintf("import %s: %v", e.Key, err))
			}
			imported++
		}
	case "json":
		for k, v := range in.Body.JSONItems {
			payload := map[string]any{"value": fmt.Sprint(v)}
			if m, ok := v.(map[string]any); ok {
				payload = m
			}
			if _, err := p.PutSecret(ctx, plugin.PutSecretRequest{
				OrgID:          orgID,
				Scope:          plugin.VaultScopeOrg,
				CollectionSlug: colSlug,
				Environment:    env,
				FolderPath:     folder,
				ItemType:       "kv_secret",
				KeyName:        k,
				Name:           k,
				Payload:        payload,
				ChangeNote:     "Imported from JSON",
				ActorUserID:    uid,
			}); err != nil {
				return nil, huma.Error400BadRequest(fmt.Sprintf("import %s: %v", k, err))
			}
			imported++
		}
	default:
		return nil, huma.Error400BadRequest("Unsupported format. Choose env or json.")
	}

	p.recordAudit(r, "vault.secrets.imported", "vault_collection", 0, map[string]any{"collection": colSlug, "env": env, "count": imported})
	out := &ImportSecretsOutput{}
	out.Body.Imported = imported
	return out, nil
}

type ExportSecretsInput struct {
	Collection  string       `query:"collection"`
	Environment string       `query:"environment"`
	Format      string       `query:"format"`
	Ctx         huma.Context `hidden:"true"`
}

func (i *ExportSecretsInput) Resolve(ctx huma.Context) []error { i.Ctx = ctx; return nil }

type ExportSecretsOutput struct {
	Body struct {
		Format  string            `json:"format"`
		DotEnv  string            `json:"dotEnv,omitempty"`
		Secrets map[string]any    `json:"secrets,omitempty"`
		Refs    map[string]string `json:"refs,omitempty"`
	}
}

func (p *Plugin) exportSecrets(ctx context.Context, in *ExportSecretsInput) (*ExportSecretsOutput, error) {
	r, orgID, _, err := p.callerOrg(in.Ctx, "vault.secret.export", "owner")
	if err != nil {
		return nil, err
	}
	metas, err := p.ListItems(ctx, orgID, plugin.VaultFilter{
		Scope:          plugin.VaultScopeOrg,
		CollectionSlug: in.Collection,
		Environment:    in.Environment,
	})
	if err != nil {
		return nil, huma.Error500InternalServerError("Failed to list items for export.")
	}
	decrypted := make([]plugin.VaultSecret, 0, len(metas))
	refs := make(map[string]string, len(metas))
	kvMap := make(map[string]any, len(metas))
	for _, m := range metas {
		sec, decErr := p.GetSecret(ctx, orgID, m.ID)
		if decErr != nil {
			return nil, huma.Error500InternalServerError("Failed to decrypt item during export.")
		}
		decrypted = append(decrypted, *sec)
		k := m.KeyName
		if k == "" {
			k = normalizeSlug(m.Name, fmt.Sprintf("item_%d", m.ID))
		}
		refs[k] = m.Ref
		if val, ok := sec.Payload["value"]; ok && len(sec.Payload) == 1 {
			kvMap[k] = val
		} else {
			kvMap[k] = sec.Payload
		}
	}

	p.recordAudit(r, "vault.secrets.exported", "vault_collection", 0, map[string]any{"collection": in.Collection, "env": in.Environment, "count": len(decrypted)})
	out := &ExportSecretsOutput{}
	fmtKind := strings.ToLower(strings.TrimSpace(in.Format))
	if fmtKind == "" {
		fmtKind = "env"
	}
	out.Body.Format = fmtKind
	out.Body.Refs = refs
	if fmtKind == "json" {
		out.Body.Secrets = kvMap
	} else {
		out.Body.DotEnv = formatDotEnv(decrypted)
	}
	return out, nil
}

type ServiceTokensOutput struct{ Body []VaultServiceToken }

func (p *Plugin) listServiceTokens(ctx context.Context, in *ctxInput) (*ServiceTokensOutput, error) {
	_, orgID, _, err := p.callerOrg(in.Ctx, "vault.token.manage", "admin")
	if err != nil {
		return nil, err
	}
	var toks []VaultServiceToken
	if err := p.db.WithContext(ctx).Where("org_id = ?", orgID).Order("id DESC").Find(&toks).Error; err != nil {
		return nil, huma.Error500InternalServerError("Failed to list service tokens.")
	}
	return &ServiceTokensOutput{Body: toks}, nil
}

type CreateServiceTokenInput struct {
	Ctx  huma.Context `hidden:"true"`
	Body struct {
		Name           string     `json:"name"`
		CollectionSlug string     `json:"collectionSlug,omitempty"`
		Environment    string     `json:"environment,omitempty"`
		FolderPrefix   string     `json:"folderPrefix,omitempty"`
		Permissions    string     `json:"permissions,omitempty"`
		ExpiresAt      *time.Time `json:"expiresAt,omitempty"`
	}
}

func (i *CreateServiceTokenInput) Resolve(ctx huma.Context) []error { i.Ctx = ctx; return nil }

type CreateServiceTokenOutput struct {
	Body struct {
		Token     VaultServiceToken `json:"token"`
		Plaintext string            `json:"plaintext"`
	}
}

func (p *Plugin) createServiceToken(ctx context.Context, in *CreateServiceTokenInput) (*CreateServiceTokenOutput, error) {
	r, orgID, uid, err := p.callerOrg(in.Ctx, "vault.token.manage", "admin")
	if err != nil {
		return nil, err
	}
	colSlug := normalizeSlug(in.Body.CollectionSlug, "default")
	_, _ = p.ensureCollection(ctx, orgID, plugin.VaultScopeOrg, colSlug)
	tok, raw, err := p.CreateServiceToken(ctx, orgID, in.Body.Name, colSlug, in.Body.Environment, in.Body.FolderPrefix, in.Body.Permissions, in.Body.ExpiresAt, uid)
	if err != nil {
		return nil, huma.Error400BadRequest(err.Error())
	}
	p.recordAudit(r, "vault.token.created", "vault_service_token", tok.ID, map[string]any{"name": tok.Name, "env": tok.Environment})
	out := &CreateServiceTokenOutput{}
	out.Body.Token = *tok
	out.Body.Plaintext = raw
	return out, nil
}

func (p *Plugin) revokeServiceToken(ctx context.Context, in *IDPathInput) (*OKOutput, error) {
	r, orgID, _, err := p.callerOrg(in.Ctx, "vault.token.manage", "admin")
	if err != nil {
		return nil, err
	}
	now := time.Now()
	res := p.db.WithContext(ctx).Model(&VaultServiceToken{}).
		Where("org_id = ? AND id = ? AND revoked_at IS NULL", orgID, in.ID).
		Update("revoked_at", &now)
	if res.Error != nil {
		return nil, huma.Error500InternalServerError("Failed to revoke token.")
	}
	if res.RowsAffected == 0 {
		return nil, huma.Error404NotFound("Service token not found or already revoked.")
	}
	p.recordAudit(r, "vault.token.revoked", "vault_service_token", in.ID, nil)
	out := &OKOutput{}
	out.Body.OK = true
	out.Body.Message = "Service token revoked."
	return out, nil
}

func (p *Plugin) handleServiceTokenPull(w http.ResponseWriter, r *http.Request) {
	tok, err := p.AuthenticateServiceToken(r.Context(), r.Header.Get("Authorization"))
	if err != nil {
		http.Error(w, err.Error(), http.StatusUnauthorized)
		return
	}
	var rows []VaultItem
	if err := p.db.WithContext(r.Context()).Where(
		"org_id = ? AND scope = ? AND collection_id = ? AND environment = ?",
		tok.OrgID, plugin.VaultScopeOrg, tok.CollectionID, tok.Environment,
	).Order("key_name ASC, id ASC").Find(&rows).Error; err != nil {
		http.Error(w, "failed to load secrets", http.StatusInternalServerError)
		return
	}
	var filtered []plugin.VaultSecret
	for _, row := range rows {
		if tok.FolderPrefix != "/" && row.FolderPath != tok.FolderPrefix && !strings.HasPrefix(row.FolderPath, tok.FolderPrefix+"/") {
			continue
		}
		sec, decErr := p.decryptItem(r.Context(), row)
		if decErr != nil {
			http.Error(w, "failed to decrypt secret", http.StatusInternalServerError)
			return
		}
		filtered = append(filtered, *sec)
	}
	auditMeta := map[string]any{
		"name":         tok.Name,
		"env":          tok.Environment,
		"folderPrefix": tok.FolderPrefix,
		"count":        len(filtered),
	}
	if p.audit != nil {
		rAudited := r.WithContext(plugin.WithOrgID(r.Context(), tok.OrgID))
		p.recordAudit(rAudited, "vault.service_token.pulled", "vault_service_token", tok.ID, auditMeta)
	} else if p.db != nil && p.db.Migrator().HasTable("audit_logs") {
		metaJSON, _ := json.Marshal(auditMeta)
		_ = p.db.WithContext(r.Context()).Table("audit_logs").Create(&struct {
			OrgID      uint      `gorm:"column:org_id"`
			ActorID    uint      `gorm:"column:actor_id"`
			Action     string    `gorm:"column:action"`
			TargetType string    `gorm:"column:target_type"`
			TargetID   uint      `gorm:"column:target_id"`
			Meta       string    `gorm:"column:meta"`
			IP         string    `gorm:"column:ip"`
			CreatedAt  time.Time `gorm:"column:created_at"`
		}{
			OrgID:      tok.OrgID,
			Action:     "vault.service_token.pulled",
			TargetType: "vault_service_token",
			TargetID:   tok.ID,
			Meta:       string(metaJSON),
			IP:         r.RemoteAddr,
			CreatedAt:  time.Now(),
		}).Error
	}
	format := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("format")))
	if format == "env" || format == "dotenv" {
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		_, _ = w.Write([]byte(formatDotEnv(filtered)))
		return
	}
	kv := make(map[string]any, len(filtered))
	for _, s := range filtered {
		k := s.Meta.KeyName
		if k == "" {
			k = normalizeSlug(s.Meta.Name, fmt.Sprintf("item_%d", s.Meta.ID))
		}
		if v, ok := s.Payload["value"]; ok && len(s.Payload) == 1 {
			kv[k] = v
		} else {
			kv[k] = s.Payload
		}
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"environment": tok.Environment,
		"secrets":     kv,
	})
}

func (p *Plugin) listInstanceItems(ctx context.Context, in *ctxInput) (*ItemsOutput, error) {
	if _, _, err := p.callerInstanceAdmin(in.Ctx); err != nil {
		return nil, err
	}
	items, err := p.ListItems(ctx, 0, plugin.VaultFilter{Scope: plugin.VaultScopeInstance})
	if err != nil {
		return nil, huma.Error500InternalServerError("Failed to list instance vault items.")
	}
	return &ItemsOutput{Body: items}, nil
}

func (p *Plugin) putInstanceItem(ctx context.Context, in *CreateItemInput) (*ItemOutput, error) {
	r, uid, err := p.callerInstanceAdmin(in.Ctx)
	if err != nil {
		return nil, err
	}
	b := in.Body
	meta, err := p.PutInstanceSecret(ctx, plugin.PutSecretRequest{
		CollectionSlug: b.CollectionSlug,
		Environment:    b.Environment,
		FolderPath:     b.FolderPath,
		ItemType:       b.ItemType,
		ProviderType:   b.ProviderType,
		Provider:       b.Provider,
		KeyName:        b.KeyName,
		Name:           b.Name,
		Tags:           b.Tags,
		IsDefault:      b.IsDefault,
		Metadata:       b.Metadata,
		Payload:        b.Payload,
		ChangeNote:     b.ChangeNote,
		ActorUserID:    uid,
	})
	if err != nil {
		return nil, huma.Error400BadRequest(err.Error())
	}
	p.recordAudit(r, "vault.instance.upserted", "vault_item", meta.ID, map[string]any{"keyName": meta.KeyName})
	return &ItemOutput{Body: *meta}, nil
}

func (p *Plugin) revealInstanceItem(ctx context.Context, in *IDPathInput) (*RevealOutput, error) {
	r, _, err := p.callerInstanceAdmin(in.Ctx)
	if err != nil {
		return nil, err
	}
	var row VaultItem
	if err := p.db.WithContext(ctx).Where("org_id = 0 AND scope = ? AND id = ?", plugin.VaultScopeInstance, in.ID).First(&row).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, huma.Error404NotFound("Instance vault item not found.")
		}
		return nil, huma.Error500InternalServerError("Failed to read instance vault item.")
	}
	sec, err := p.decryptItem(ctx, row)
	if err != nil {
		return nil, huma.Error500InternalServerError("Failed to decrypt instance secret.")
	}
	p.recordAudit(r, "vault.instance.revealed", "vault_item", row.ID, map[string]any{"keyName": row.KeyName})
	out := &RevealOutput{}
	out.Body.Meta = sec.Meta
	out.Body.Version = sec.Version
	out.Body.Payload = sec.Payload
	return out, nil
}

func (p *Plugin) deleteInstanceItem(ctx context.Context, in *IDPathInput) (*OKOutput, error) {
	r, _, err := p.callerInstanceAdmin(in.Ctx)
	if err != nil {
		return nil, err
	}
	var row VaultItem
	if err := p.db.WithContext(ctx).Where("org_id = 0 AND scope = ? AND id = ?", plugin.VaultScopeInstance, in.ID).First(&row).Error; err != nil {
		return nil, huma.Error404NotFound("Instance vault item not found.")
	}
	if err := p.deleteItemRow(ctx, row); err != nil {
		return nil, huma.Error500InternalServerError("Failed to delete instance secret.")
	}
	p.recordAudit(r, "vault.instance.deleted", "vault_item", row.ID, map[string]any{"keyName": row.KeyName})
	out := &OKOutput{}
	out.Body.OK = true
	out.Body.Message = "Instance vault item deleted and key shredded."
	return out, nil
}
