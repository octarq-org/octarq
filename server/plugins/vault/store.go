package vault

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/octarq-org/octarq/server/plugin"
	"gorm.io/gorm"
)

var (
	_ plugin.VaultService       = (*Plugin)(nil)
	_ plugin.CredentialResolver = (*Plugin)(nil)
)

func (p *Plugin) ensureCollection(ctx context.Context, orgID uint, scope plugin.VaultScope, slug string) (*VaultCollection, error) {
	scope = normalizeScope(scope)
	if scope == plugin.VaultScopeInstance {
		orgID = 0
		slug = normalizeSlug(slug, "system")
	} else {
		if orgID == 0 {
			return nil, errors.New("vault: organization id is required")
		}
		slug = normalizeSlug(slug, "default")
	}
	var col VaultCollection
	err := p.db.WithContext(ctx).
		Where("org_id = ? AND scope = ? AND slug = ?", orgID, scope, slug).
		First(&col).Error
	if err == nil {
		return &col, nil
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, err
	}
	name := strings.ToUpper(slug[:1]) + slug[1:]
	kind := "vault"
	switch slug {
	case "integrations":
		name = "Service Integrations"
		kind = "integration"
	case "default":
		name = "Default Vault"
		kind = "vault"
	case "system":
		name = "Instance System Vault"
		kind = "system"
	}
	col = VaultCollection{
		OrgID: orgID,
		Scope: scope,
		Slug:  slug,
		Name:  name,
		Kind:  kind,
	}
	if err := p.db.WithContext(ctx).Create(&col).Error; err != nil {
		// Handle concurrent creation race cleanly.
		if err2 := p.db.WithContext(ctx).
			Where("org_id = ? AND scope = ? AND slug = ?", orgID, scope, slug).
			First(&col).Error; err2 == nil {
			return &col, nil
		}
		return nil, err
	}
	return &col, nil
}

func (p *Plugin) findCollection(ctx context.Context, orgID uint, scope plugin.VaultScope, slug string) (*VaultCollection, error) {
	var col VaultCollection
	err := p.db.WithContext(ctx).
		Where("org_id = ? AND scope = ? AND slug = ?", orgID, normalizeScope(scope), normalizeSlug(slug, "default")).
		First(&col).Error
	if err != nil {
		return nil, err
	}
	return &col, nil
}

func (p *Plugin) itemToMeta(row VaultItem, colSlug string, fieldKeys []string) plugin.VaultItemMeta {
	var tags []string
	if row.TagsJSON != "" {
		_ = json.Unmarshal([]byte(row.TagsJSON), &tags)
	}
	var meta map[string]any
	if row.MetadataJSON != "" {
		_ = json.Unmarshal([]byte(row.MetadataJSON), &meta)
	}
	return plugin.VaultItemMeta{
		ID:                   row.ID,
		SecretID:             row.SecretID,
		OrgID:                row.OrgID,
		Scope:                row.Scope,
		OwnerUserID:          row.OwnerUserID,
		CollectionID:         row.CollectionID,
		CollectionSlug:       colSlug,
		Environment:          row.Environment,
		FolderPath:           row.FolderPath,
		ItemType:             row.ItemType,
		ProviderType:         row.ProviderType,
		Provider:             row.Provider,
		KeyName:              row.KeyName,
		Name:                 row.Name,
		Tags:                 tags,
		IsDefault:            row.IsDefault,
		CurrentVersion:       row.CurrentVersion,
		Configured:           row.CurrentVersion > 0,
		FieldKeys:            fieldKeys,
		Metadata:             meta,
		ExpiresAt:            row.ExpiresAt,
		RotationIntervalDays: row.RotationIntervalDays,
		LastRotatedAt:        row.LastRotatedAt,
		Ref:                  buildVaultRef(colSlug, row.Environment, row.FolderPath, row.KeyName, row.ID),
		CreatedAt:            row.CreatedAt,
		UpdatedAt:            row.UpdatedAt,
	}
}

func (p *Plugin) collectionSlugMap(ctx context.Context, orgID uint, scope plugin.VaultScope) map[uint]string {
	var cols []VaultCollection
	_ = p.db.WithContext(ctx).Where("org_id = ? AND scope = ?", orgID, scope).Find(&cols).Error
	out := make(map[uint]string, len(cols))
	for _, c := range cols {
		out[c.ID] = c.Slug
	}
	return out
}

func (p *Plugin) fieldKeysForVersions(ctx context.Context, items []VaultItem) map[uint][]string {
	if len(items) == 0 {
		return nil
	}
	itemIDs := make([]uint, len(items))
	verMap := make(map[uint]int, len(items))
	for i, it := range items {
		itemIDs[i] = it.ID
		verMap[it.ID] = it.CurrentVersion
	}
	var versions []VaultItemVersion
	_ = p.db.WithContext(ctx).Where("item_id IN ?", itemIDs).Find(&versions).Error
	out := make(map[uint][]string, len(items))
	for _, v := range versions {
		if v.Version != verMap[v.ItemID] || v.FieldSchemaJSON == "" {
			continue
		}
		var schema map[string]string
		if json.Unmarshal([]byte(v.FieldSchemaJSON), &schema) == nil {
			keys := make([]string, 0, len(schema))
			for k := range schema {
				keys = append(keys, k)
			}
			out[v.ItemID] = keys
		}
	}
	return out
}

// PutSecret creates a new secret item (or creates a new version if KeyName collides in the same path).
func (p *Plugin) PutSecret(ctx context.Context, req plugin.PutSecretRequest) (*plugin.VaultItemMeta, error) {
	if p.db == nil {
		return nil, errors.New("vault: database unavailable")
	}
	scope := normalizeScope(req.Scope)
	if scope == plugin.VaultScopeInstance {
		return nil, errors.New("vault: use PutInstanceSecret for instance-scope secrets")
	}
	if req.OrgID == 0 {
		return nil, errors.New("vault: organization id is required")
	}
	return p.putSecretInternal(ctx, req, scope)
}

// PutInstanceSecret stores an instance-wide secret (org_id = 0, scope = instance).
func (p *Plugin) PutInstanceSecret(ctx context.Context, req plugin.PutSecretRequest) (*plugin.VaultItemMeta, error) {
	if p.db == nil {
		return nil, errors.New("vault: database unavailable")
	}
	req.OrgID = 0
	if req.CollectionSlug == "" {
		req.CollectionSlug = "system"
	}
	return p.putSecretInternal(ctx, req, plugin.VaultScopeInstance)
}

func (p *Plugin) putSecretInternal(ctx context.Context, req plugin.PutSecretRequest, scope plugin.VaultScope) (*plugin.VaultItemMeta, error) {
	name := strings.TrimSpace(req.Name)
	if name == "" {
		name = strings.TrimSpace(req.KeyName)
	}
	if name == "" {
		return nil, errors.New("vault: item name is required")
	}
	itemType := strings.ToLower(strings.TrimSpace(req.ItemType))
	if itemType == "" {
		if req.ProviderType != "" {
			itemType = "integration"
		} else {
			itemType = "kv_secret"
		}
	}
	provType := strings.ToLower(strings.TrimSpace(req.ProviderType))
	prov := strings.ToLower(strings.TrimSpace(req.Provider))
	if err := validatePayload(itemType, provType, prov, req.Payload); err != nil {
		return nil, err
	}

	colSlug := req.CollectionSlug
	if colSlug == "" {
		if itemType == "integration" {
			colSlug = "integrations"
		} else {
			colSlug = "default"
		}
	}
	col, err := p.ensureCollection(ctx, req.OrgID, scope, colSlug)
	if err != nil {
		return nil, err
	}
	env := normalizeEnvironment(req.Environment)
	folder := normalizeFolderPath(req.FolderPath)
	keyName := strings.TrimSpace(req.KeyName)

	// If keyName is specified and already exists in this exact (org, scope, collection, env, folder),
	// append a new immutable version instead of duplicating.
	if keyName != "" {
		var existing VaultItem
		err := p.db.WithContext(ctx).Where(
			"org_id = ? AND scope = ? AND collection_id = ? AND environment = ? AND folder_path = ? AND key_name = ?",
			req.OrgID, scope, col.ID, env, folder, keyName,
		).First(&existing).Error
		if err == nil {
			var isDef *bool
			if req.IsDefault {
				b := true
				isDef = &b
			}
			return p.updateSecretInternal(ctx, existing, &name, req.Metadata, req.Payload, isDef, req.ChangeNote, req.ActorUserID)
		}
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, err
		}
	}

	secretID := uuid.NewString()
	ct, err := p.sealPayload(req.OrgID, secretID, 1, req.Payload)
	if err != nil {
		return nil, err
	}

	tagsBytes, _ := json.Marshal(req.Tags)
	metaMap := extractNonSecretMetadata(itemType, provType, prov, req.Payload, req.Metadata)
	metaBytes, _ := json.Marshal(metaMap)
	schemaBytes, _ := json.Marshal(extractFieldSchema(req.Payload, itemType, provType, prov))
	now := time.Now()

	row := VaultItem{
		SecretID:             secretID,
		OrgID:                req.OrgID,
		Scope:                scope,
		OwnerUserID:          req.OwnerUserID,
		CollectionID:         col.ID,
		Environment:          env,
		FolderPath:           folder,
		ItemType:             itemType,
		ProviderType:         provType,
		Provider:             prov,
		KeyName:              keyName,
		Name:                 name,
		TagsJSON:             string(tagsBytes),
		IsDefault:            req.IsDefault,
		CurrentVersion:       1,
		ExpiresAt:            req.ExpiresAt,
		RotationIntervalDays: req.RotationIntervalDays,
		LastRotatedAt:        &now,
		MetadataJSON:         string(metaBytes),
	}

	err = p.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if req.IsDefault && provType != "" {
			if err := tx.Model(&VaultItem{}).
				Where("org_id = ? AND scope = ? AND provider_type = ?", req.OrgID, scope, provType).
				Update("is_default", false).Error; err != nil {
				return err
			}
		}
		if err := tx.Create(&row).Error; err != nil {
			return err
		}
		ver := VaultItemVersion{
			OrgID:           req.OrgID,
			ItemID:          row.ID,
			Version:         1,
			Ciphertext:      ct,
			FieldSchemaJSON: string(schemaBytes),
			CreatedBy:       req.ActorUserID,
			ChangeNote:      req.ChangeNote,
		}
		return tx.Create(&ver).Error
	})
	if err != nil {
		_ = p.shredItemKey(req.OrgID, secretID)
		return nil, err
	}

	keys := make([]string, 0, len(req.Payload))
	for k := range req.Payload {
		keys = append(keys, k)
	}
	out := p.itemToMeta(row, col.Slug, keys)
	return &out, nil
}

// UpdateSecret creates a new encrypted version for an org-scoped vault item.
func (p *Plugin) UpdateSecret(ctx context.Context, orgID, itemID uint, name *string, metadata map[string]any, payload map[string]any, isDefault *bool, changeNote string, actorUserID uint) (*plugin.VaultItemMeta, error) {
	if p.db == nil {
		return nil, errors.New("vault: database unavailable")
	}
	if orgID == 0 || itemID == 0 {
		return nil, errors.New("vault: organization id and item id are required")
	}
	var row VaultItem
	if err := p.db.WithContext(ctx).Where("org_id = ? AND id = ?", orgID, itemID).First(&row).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, plugin.ErrSecretNotFound
		}
		return nil, err
	}
	return p.updateSecretInternal(ctx, row, name, metadata, payload, isDefault, changeNote, actorUserID)
}

func (p *Plugin) updateSecretInternal(ctx context.Context, row VaultItem, name *string, metadata map[string]any, payload map[string]any, isDefault *bool, changeNote string, actorUserID uint) (*plugin.VaultItemMeta, error) {
	if name != nil && strings.TrimSpace(*name) != "" {
		row.Name = strings.TrimSpace(*name)
	}
	if isDefault != nil {
		row.IsDefault = *isDefault
	}

	var curPayload map[string]any
	if row.CurrentVersion > 0 {
		var curVer VaultItemVersion
		if err := p.db.WithContext(ctx).Where("item_id = ? AND version = ?", row.ID, row.CurrentVersion).First(&curVer).Error; err == nil {
			dec, decErr := p.openPayload(row.OrgID, row.SecretID, curVer.Version, curVer.Ciphertext)
			if decErr != nil {
				return nil, decErr
			}
			curPayload = dec
		}
	}
	if curPayload == nil {
		curPayload = map[string]any{}
	}

	newVersion := row.CurrentVersion
	var newCiphertext string
	var schemaBytes []byte
	finalPayload := curPayload

	if payload != nil {
		merged := make(map[string]any, len(curPayload)+len(payload))
		for k, v := range curPayload {
			merged[k] = v
		}
		for k, v := range payload {
			if s, ok := v.(string); ok {
				trimmed := strings.TrimSpace(s)
				if trimmed == "••••••••" || trimmed == "" {
					if trimmed == "" {
						delete(merged, k)
					}
					continue
				}
				merged[k] = trimmed
			} else if v != nil {
				merged[k] = v
			}
		}
		if err := validatePayload(row.ItemType, row.ProviderType, row.Provider, merged); err != nil {
			return nil, err
		}
		newVersion = row.CurrentVersion + 1
		ct, err := p.sealPayload(row.OrgID, row.SecretID, newVersion, merged)
		if err != nil {
			return nil, err
		}
		newCiphertext = ct
		schemaBytes, _ = json.Marshal(extractFieldSchema(merged, row.ItemType, row.ProviderType, row.Provider))
		finalPayload = merged
		now := time.Now()
		row.LastRotatedAt = &now
		row.CurrentVersion = newVersion
	}

	existingMeta := map[string]any{}
	if row.MetadataJSON != "" {
		_ = json.Unmarshal([]byte(row.MetadataJSON), &existingMeta)
	}
	for k, v := range metadata {
		if !looksSecretKey(k) {
			existingMeta[k] = v
		}
	}
	metaMap := extractNonSecretMetadata(row.ItemType, row.ProviderType, row.Provider, finalPayload, existingMeta)
	metaBytes, _ := json.Marshal(metaMap)
	row.MetadataJSON = string(metaBytes)

	err := p.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if row.IsDefault && row.ProviderType != "" {
			if err := tx.Model(&VaultItem{}).
				Where("org_id = ? AND scope = ? AND provider_type = ? AND id <> ?", row.OrgID, row.Scope, row.ProviderType, row.ID).
				Update("is_default", false).Error; err != nil {
				return err
			}
		}
		if err := tx.Save(&row).Error; err != nil {
			return err
		}
		if newCiphertext != "" {
			return tx.Create(&VaultItemVersion{
				OrgID:           row.OrgID,
				ItemID:          row.ID,
				Version:         newVersion,
				Ciphertext:      newCiphertext,
				FieldSchemaJSON: string(schemaBytes),
				CreatedBy:       actorUserID,
				ChangeNote:      changeNote,
			}).Error
		}
		return nil
	})
	if err != nil {
		return nil, err
	}

	colSlug := p.collectionSlugMap(ctx, row.OrgID, row.Scope)[row.CollectionID]
	keys := make([]string, 0, len(finalPayload))
	for k := range finalPayload {
		keys = append(keys, k)
	}
	out := p.itemToMeta(row, colSlug, keys)
	return &out, nil
}

// RollbackVersion creates a new head version copied from a previous historical version.
func (p *Plugin) RollbackVersion(ctx context.Context, orgID, itemID uint, targetVersion int, actorUserID uint) (*plugin.VaultItemMeta, error) {
	if p.db == nil || orgID == 0 || itemID == 0 {
		return nil, errors.New("vault: invalid rollback parameters")
	}
	var row VaultItem
	if err := p.db.WithContext(ctx).Where("org_id = ? AND id = ?", orgID, itemID).First(&row).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, plugin.ErrSecretNotFound
		}
		return nil, err
	}
	var ver VaultItemVersion
	if err := p.db.WithContext(ctx).Where("item_id = ? AND version = ?", row.ID, targetVersion).First(&ver).Error; err != nil {
		return nil, fmt.Errorf("vault: version %d not found", targetVersion)
	}
	payload, err := p.openPayload(row.OrgID, row.SecretID, ver.Version, ver.Ciphertext)
	if err != nil {
		return nil, err
	}
	return p.updateSecretInternal(ctx, row, nil, nil, payload, nil, fmt.Sprintf("Rollback to v%d", targetVersion), actorUserID)
}

func (p *Plugin) GetMeta(ctx context.Context, orgID, itemID uint) (*plugin.VaultItemMeta, error) {
	if p.db == nil || orgID == 0 || itemID == 0 {
		return nil, errors.New("vault: invalid organization or item ID")
	}
	var row VaultItem
	if err := p.db.WithContext(ctx).Where("org_id = ? AND id = ?", orgID, itemID).First(&row).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, plugin.ErrSecretNotFound
		}
		return nil, err
	}
	colSlug := p.collectionSlugMap(ctx, orgID, row.Scope)[row.CollectionID]
	fieldKeys := p.fieldKeysForVersions(ctx, []VaultItem{row})[row.ID]
	out := p.itemToMeta(row, colSlug, fieldKeys)
	return &out, nil
}

func (p *Plugin) GetSecret(ctx context.Context, orgID, itemID uint) (*plugin.VaultSecret, error) {
	if p.db == nil || orgID == 0 || itemID == 0 {
		return nil, errors.New("vault: invalid organization or item ID")
	}
	var row VaultItem
	if err := p.db.WithContext(ctx).Where("org_id = ? AND id = ?", orgID, itemID).First(&row).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, plugin.ErrSecretNotFound
		}
		return nil, err
	}
	return p.decryptItem(ctx, row)
}

func (p *Plugin) decryptItem(ctx context.Context, row VaultItem) (*plugin.VaultSecret, error) {
	var ver VaultItemVersion
	if err := p.db.WithContext(ctx).
		Where("item_id = ? AND version = ?", row.ID, row.CurrentVersion).
		First(&ver).Error; err != nil {
		return nil, fmt.Errorf("vault: active version %d missing: %w", row.CurrentVersion, err)
	}
	payload, err := p.openPayload(row.OrgID, row.SecretID, ver.Version, ver.Ciphertext)
	if err != nil {
		return nil, err
	}
	colSlug := p.collectionSlugMap(ctx, row.OrgID, row.Scope)[row.CollectionID]
	keys := make([]string, 0, len(payload))
	for k := range payload {
		keys = append(keys, k)
	}
	return &plugin.VaultSecret{
		Meta:    p.itemToMeta(row, colSlug, keys),
		Version: ver.Version,
		Payload: payload,
	}, nil
}

func (p *Plugin) ResolveByRef(ctx context.Context, orgID uint, ref string) (*plugin.VaultSecret, error) {
	if p.db == nil || orgID == 0 {
		return nil, errors.New("vault: organization id is required")
	}
	parsed, err := parseVaultRef(ref)
	if err != nil {
		return nil, err
	}
	if parsed.itemID > 0 {
		return p.GetSecret(ctx, orgID, parsed.itemID)
	}
	col, err := p.findCollection(ctx, orgID, plugin.VaultScopeOrg, parsed.collection)
	if err != nil {
		return nil, plugin.ErrSecretNotFound
	}
	var row VaultItem
	if err := p.db.WithContext(ctx).Where(
		"org_id = ? AND scope = ? AND collection_id = ? AND environment = ? AND folder_path = ? AND key_name = ?",
		orgID, plugin.VaultScopeOrg, col.ID, parsed.env, parsed.folder, parsed.keyName,
	).First(&row).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, plugin.ErrSecretNotFound
		}
		return nil, err
	}
	return p.decryptItem(ctx, row)
}

func (p *Plugin) ResolveInstanceSecret(ctx context.Context, keyName string) (*plugin.VaultSecret, error) {
	if p.db == nil {
		return nil, errors.New("vault: database unavailable")
	}
	keyName = strings.TrimSpace(keyName)
	if keyName == "" {
		return nil, errors.New("vault: keyName is required")
	}
	var row VaultItem
	if err := p.db.WithContext(ctx).Where(
		"org_id = 0 AND scope = ? AND key_name = ?",
		plugin.VaultScopeInstance, keyName,
	).First(&row).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, plugin.ErrSecretNotFound
		}
		return nil, err
	}
	return p.decryptItem(ctx, row)
}

func (p *Plugin) ListItems(ctx context.Context, orgID uint, filter plugin.VaultFilter) ([]plugin.VaultItemMeta, error) {
	if p.db == nil {
		return nil, errors.New("vault: database unavailable")
	}
	scope := normalizeScope(filter.Scope)
	if scope == plugin.VaultScopeInstance {
		orgID = 0
	} else if orgID == 0 {
		return nil, errors.New("vault: organization id is required")
	}
	q := p.db.WithContext(ctx).Where("org_id = ? AND scope = ?", orgID, scope)
	if scope == plugin.VaultScopeUser && filter.OwnerUserID > 0 {
		q = q.Where("owner_user_id = ?", filter.OwnerUserID)
	}
	if filter.CollectionSlug != "" {
		col, err := p.findCollection(ctx, orgID, scope, filter.CollectionSlug)
		if err != nil {
			return []plugin.VaultItemMeta{}, nil
		}
		q = q.Where("collection_id = ?", col.ID)
	}
	if filter.Environment != "" {
		q = q.Where("environment = ?", normalizeEnvironment(filter.Environment))
	}
	if filter.FolderPath != "" {
		q = q.Where("folder_path = ?", normalizeFolderPath(filter.FolderPath))
	}
	if filter.ItemType != "" {
		q = q.Where("item_type = ?", strings.ToLower(strings.TrimSpace(filter.ItemType)))
	}
	if filter.ProviderType != "" {
		q = q.Where("provider_type = ?", strings.ToLower(strings.TrimSpace(filter.ProviderType)))
	}
	if filter.Provider != "" {
		q = q.Where("provider = ?", strings.ToLower(strings.TrimSpace(filter.Provider)))
	}
	if filter.KeyName != "" {
		q = q.Where("key_name = ?", strings.TrimSpace(filter.KeyName))
	}
	if s := strings.TrimSpace(filter.Query); s != "" {
		like := "%" + strings.ToLower(s) + "%"
		q = q.Where("LOWER(name) LIKE ? OR LOWER(key_name) LIKE ? OR LOWER(provider) LIKE ? OR LOWER(tags_json) LIKE ?", like, like, like, like)
	}
	var rows []VaultItem
	if err := q.Order("is_default DESC, name ASC, id ASC").Find(&rows).Error; err != nil {
		return nil, err
	}
	colMap := p.collectionSlugMap(ctx, orgID, scope)
	fkMap := p.fieldKeysForVersions(ctx, rows)
	out := make([]plugin.VaultItemMeta, len(rows))
	for i, r := range rows {
		out[i] = p.itemToMeta(r, colMap[r.CollectionID], fkMap[r.ID])
	}
	return out, nil
}

// DeleteSecret hard-deletes the item and all version rows, then cryptographically shreds its SubjectVault DEK.
func (p *Plugin) DeleteSecret(ctx context.Context, orgID, itemID uint) error {
	if p.db == nil || orgID == 0 || itemID == 0 {
		return errors.New("vault: invalid organization or item ID")
	}
	var row VaultItem
	if err := p.db.WithContext(ctx).Where("org_id = ? AND id = ?", orgID, itemID).First(&row).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return plugin.ErrSecretNotFound
		}
		return err
	}
	return p.deleteItemRow(ctx, row)
}

func (p *Plugin) deleteItemRow(ctx context.Context, row VaultItem) error {
	if err := p.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("item_id = ?", row.ID).Delete(&VaultItemVersion{}).Error; err != nil {
			return err
		}
		return tx.Delete(&row).Error
	}); err != nil {
		return err
	}
	_ = p.shredItemKey(row.OrgID, row.SecretID)
	return nil
}

// ComputeTOTP generates the active RFC-6238 verification code for a twofa_totp or login item.
func (p *Plugin) ComputeTOTP(ctx context.Context, orgID, itemID uint) (string, int, error) {
	sec, err := p.GetSecret(ctx, orgID, itemID)
	if err != nil {
		return "", 0, err
	}
	return computeTOTPCode(sec.Payload, time.Now())
}

// LookupCredential satisfies plugin.CredentialResolver across all plugins and core services.
func (p *Plugin) LookupCredential(ctx context.Context, orgID uint, q plugin.CredentialQuery) (*plugin.Credential, error) {
	if p.db == nil {
		return nil, errors.New("vault: database unavailable")
	}
	if orgID == 0 {
		return nil, errors.New("vault: organization id is required")
	}
	query := p.db.WithContext(ctx).Where("org_id = ? AND scope = ?", orgID, plugin.VaultScopeOrg)
	if q.ID > 0 {
		query = query.Where("id = ?", q.ID)
	}
	if q.ProviderType != "" {
		query = query.Where("provider_type = ?", strings.ToLower(strings.TrimSpace(q.ProviderType)))
	}
	if q.Provider != "" {
		query = query.Where("provider = ?", strings.ToLower(strings.TrimSpace(q.Provider)))
	}
	if q.ID == 0 {
		query = query.Order("is_default DESC, id ASC")
	}
	var row VaultItem
	if err := query.First(&row).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, plugin.ErrCredentialNotFound
		}
		return nil, err
	}
	sec, err := p.decryptItem(ctx, row)
	if err != nil {
		return nil, err
	}
	cfgJSON, err := json.Marshal(sec.Payload)
	if err != nil {
		return nil, err
	}
	return &plugin.Credential{
		ID:           row.ID,
		OrgID:        row.OrgID,
		ProviderType: row.ProviderType,
		Provider:     row.Provider,
		Name:         row.Name,
		ConfigJSON:   json.RawMessage(cfgJSON),
		IsDefault:    row.IsDefault,
	}, nil
}

func (p *Plugin) purge(orgID uint) error {
	if p.db == nil || orgID == 0 {
		return nil
	}
	var items []VaultItem
	if err := p.db.Where("org_id = ?", orgID).Find(&items).Error; err != nil {
		return err
	}
	err := p.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("org_id = ?", orgID).Delete(&VaultServiceToken{}).Error; err != nil {
			return err
		}
		if err := tx.Where("org_id = ?", orgID).Delete(&VaultItemVersion{}).Error; err != nil {
			return err
		}
		if err := tx.Where("org_id = ?", orgID).Delete(&VaultItem{}).Error; err != nil {
			return err
		}
		return tx.Where("org_id = ?", orgID).Delete(&VaultCollection{}).Error
	})
	if err != nil {
		return err
	}
	for _, it := range items {
		_ = p.shredItemKey(orgID, it.SecretID)
	}
	return nil
}

func (p *Plugin) exportData(orgID uint) map[string]any {
	if p.db == nil || orgID == 0 {
		return map[string]any{"items": []plugin.VaultItemMeta{}}
	}
	items, err := p.ListItems(context.Background(), orgID, plugin.VaultFilter{Scope: plugin.VaultScopeOrg})
	if err != nil {
		return map[string]any{"items": []plugin.VaultItemMeta{}}
	}
	// Never export plaintext secrets or ciphertexts in workspace metadata export.
	return map[string]any{"items": items}
}
