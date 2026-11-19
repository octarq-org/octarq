package vault

import (
	"fmt"
	"net/url"
	"strings"
	"time"

	"github.com/octarq-org/octarq/server/plugin"
)

// VaultCollection groups secrets into Infisical-style projects or 1Password-style vaults.
type VaultCollection struct {
	ID          uint              `gorm:"primaryKey" json:"id"`
	OrgID       uint              `gorm:"column:org_id;not null;uniqueIndex:idx_vc_org_scope_slug" json:"orgId"`
	Scope       plugin.VaultScope `gorm:"column:scope;size:16;not null;default:'org';uniqueIndex:idx_vc_org_scope_slug" json:"scope"`
	Slug        string            `gorm:"column:slug;size:64;not null;uniqueIndex:idx_vc_org_scope_slug" json:"slug"`
	Name        string            `gorm:"column:name;size:128;not null" json:"name"`
	Kind        string            `gorm:"column:kind;size:32;not null;default:'vault'" json:"kind"` // integration, project, vault, system
	Description string            `gorm:"column:description;type:text" json:"description,omitempty"`
	Icon        string            `gorm:"column:icon;size:32" json:"icon,omitempty"`
	CreatedBy   uint              `gorm:"column:created_by;default:0" json:"createdBy,omitempty"`
	CreatedAt   time.Time         `json:"createdAt"`
	UpdatedAt   time.Time         `json:"updatedAt"`
}

func (VaultCollection) TableName() string { return "vault_collections" }

// VaultItem stores searchable, non-secret metadata for a credential or secret.
// Encrypted payloads live exclusively in VaultItemVersion under a per-item SubjectVault DEK.
type VaultItem struct {
	ID                   uint              `gorm:"primaryKey" json:"id"`
	SecretID             string            `gorm:"column:secret_id;size:64;not null;uniqueIndex" json:"secretId"`
	OrgID                uint              `gorm:"column:org_id;not null;index:idx_vi_org_scope;index:idx_vi_org_prov" json:"orgId"`
	Scope                plugin.VaultScope `gorm:"column:scope;size:16;not null;default:'org';index:idx_vi_org_scope" json:"scope"`
	OwnerUserID          uint              `gorm:"column:owner_user_id;default:0;index:idx_vi_org_scope" json:"ownerUserId,omitempty"`
	CollectionID         uint              `gorm:"column:collection_id;not null;index:idx_vi_col_env_path" json:"collectionId"`
	Environment          string            `gorm:"column:environment;size:32;not null;default:'default';index:idx_vi_col_env_path" json:"environment"`
	FolderPath           string            `gorm:"column:folder_path;size:255;not null;default:'/';index:idx_vi_col_env_path" json:"folderPath"`
	ItemType             string            `gorm:"column:item_type;size:32;not null;index" json:"itemType"`
	ProviderType         string            `gorm:"column:provider_type;size:32;index:idx_vi_org_prov" json:"providerType,omitempty"`
	Provider             string            `gorm:"column:provider;size:64;index:idx_vi_org_prov" json:"provider,omitempty"`
	KeyName              string            `gorm:"column:key_name;size:128;index" json:"keyName,omitempty"`
	Name                 string            `gorm:"column:name;size:128;not null" json:"name"`
	TagsJSON             string            `gorm:"column:tags_json;type:text" json:"-"`
	IsDefault            bool              `gorm:"column:is_default;default:false;index:idx_vi_org_prov" json:"isDefault"`
	CurrentVersion       int               `gorm:"column:current_version;not null;default:0" json:"currentVersion"`
	ExpiresAt            *time.Time        `gorm:"column:expires_at" json:"expiresAt,omitempty"`
	RotationIntervalDays int               `gorm:"column:rotation_interval_days;default:0" json:"rotationIntervalDays,omitempty"`
	LastRotatedAt        *time.Time        `gorm:"column:last_rotated_at" json:"lastRotatedAt,omitempty"`
	MetadataJSON         string            `gorm:"column:metadata_json;type:text" json:"-"`
	CreatedAt            time.Time         `json:"createdAt"`
	UpdatedAt            time.Time         `json:"updatedAt"`
}

func (VaultItem) TableName() string { return "vault_items" }

// VaultItemVersion stores an immutable AES-256-GCM encrypted payload snapshot for a VaultItem.
type VaultItemVersion struct {
	ID              uint      `gorm:"primaryKey" json:"id"`
	OrgID           uint      `gorm:"column:org_id;not null;index" json:"orgId"`
	ItemID          uint      `gorm:"column:item_id;not null;uniqueIndex:idx_viv_item_ver" json:"itemId"`
	Version         int       `gorm:"column:version;not null;uniqueIndex:idx_viv_item_ver" json:"version"`
	Ciphertext      string    `gorm:"column:ciphertext;type:text;not null" json:"-"`
	FieldSchemaJSON string    `gorm:"column:field_schema_json;type:text" json:"-"`
	CreatedBy       uint      `gorm:"column:created_by;default:0" json:"createdBy,omitempty"`
	ChangeNote      string    `gorm:"column:change_note;size:255" json:"changeNote,omitempty"`
	CreatedAt       time.Time `json:"createdAt"`
}

func (VaultItemVersion) TableName() string { return "vault_item_versions" }

// VaultServiceToken authorizes scoped machine/CI reads against a specific collection, environment, and folder prefix.
type VaultServiceToken struct {
	ID           uint       `gorm:"primaryKey" json:"id"`
	OrgID        uint       `gorm:"column:org_id;not null;index" json:"orgId"`
	Name         string     `gorm:"column:name;size:128;not null" json:"name"`
	TokenPrefix  string     `gorm:"column:token_prefix;size:24;not null" json:"tokenPrefix"`
	TokenHash    string     `gorm:"column:token_hash;size:64;not null;uniqueIndex" json:"-"`
	CollectionID uint       `gorm:"column:collection_id;not null;index" json:"collectionId"`
	Environment  string     `gorm:"column:environment;size:32;not null;default:'default'" json:"environment"`
	FolderPrefix string     `gorm:"column:folder_prefix;size:255;not null;default:'/'" json:"folderPrefix"`
	Permissions  string     `gorm:"column:permissions;size:16;not null;default:'read'" json:"permissions"`
	ExpiresAt    *time.Time `gorm:"column:expires_at" json:"expiresAt,omitempty"`
	LastUsedAt   *time.Time `gorm:"column:last_used_at" json:"lastUsedAt,omitempty"`
	RevokedAt    *time.Time `gorm:"column:revoked_at" json:"revokedAt,omitempty"`
	CreatedBy    uint       `gorm:"column:created_by;default:0" json:"createdBy,omitempty"`
	CreatedAt    time.Time  `json:"createdAt"`
}

func (VaultServiceToken) TableName() string { return "vault_service_tokens" }

func normalizeScope(s plugin.VaultScope) plugin.VaultScope {
	switch plugin.VaultScope(strings.ToLower(strings.TrimSpace(string(s)))) {
	case plugin.VaultScopeInstance:
		return plugin.VaultScopeInstance
	case plugin.VaultScopeUser:
		return plugin.VaultScopeUser
	default:
		return plugin.VaultScopeOrg
	}
}

func normalizeSlug(s, fallback string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	if s == "" {
		return fallback
	}
	var b strings.Builder
	for _, r := range s {
		switch {
		case (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '-' || r == '_':
			b.WriteRune(r)
		case r == ' ' || r == '.':
			b.WriteByte('-')
		}
	}
	out := strings.Trim(b.String(), "-")
	if out == "" {
		return fallback
	}
	return out
}

func normalizeEnvironment(env string) string {
	env = strings.ToLower(strings.TrimSpace(env))
	if env == "" {
		return "default"
	}
	return normalizeSlug(env, "default")
}

func normalizeFolderPath(p string) string {
	p = strings.TrimSpace(p)
	if p == "" || p == "/" {
		return "/"
	}
	if !strings.HasPrefix(p, "/") {
		p = "/" + p
	}
	p = strings.TrimRight(p, "/")
	if p == "" {
		return "/"
	}
	return p
}

func buildVaultRef(colSlug, env, folderPath, keyName string, itemID uint) string {
	if keyName != "" {
		fp := strings.TrimPrefix(normalizeFolderPath(folderPath), "/")
		if fp != "" {
			return fmt.Sprintf("vault://%s/%s/%s/%s", colSlug, env, fp, keyName)
		}
		return fmt.Sprintf("vault://%s/%s/%s", colSlug, env, keyName)
	}
	return fmt.Sprintf("vault://id/%d", itemID)
}

type parsedVaultRef struct {
	itemID     uint
	collection string
	env        string
	folder     string
	keyName    string
}

func parseVaultRef(ref string) (*parsedVaultRef, error) {
	ref = strings.TrimSpace(ref)
	if !strings.HasPrefix(ref, "vault://") {
		return nil, fmt.Errorf("invalid vault ref %q: must start with vault://", ref)
	}
	u, err := url.Parse(ref)
	if err != nil {
		return nil, fmt.Errorf("invalid vault ref %q: %w", ref, err)
	}
	host := strings.ToLower(strings.TrimSpace(u.Host))
	pathParts := strings.Split(strings.Trim(u.Path, "/"), "/")
	if host == "id" {
		if len(pathParts) != 1 || pathParts[0] == "" {
			return nil, fmt.Errorf("invalid vault id ref %q", ref)
		}
		var id uint
		if _, err := fmt.Sscanf(pathParts[0], "%d", &id); err != nil || id == 0 {
			return nil, fmt.Errorf("invalid vault item id in ref %q", ref)
		}
		return &parsedVaultRef{itemID: id}, nil
	}
	if host == "" || len(pathParts) < 2 {
		return nil, fmt.Errorf("invalid vault path ref %q: expected vault://<collection>/<env>/[folder/]<key>", ref)
	}
	env := normalizeEnvironment(pathParts[0])
	keyName := pathParts[len(pathParts)-1]
	if strings.TrimSpace(keyName) == "" {
		return nil, fmt.Errorf("missing key name in vault ref %q", ref)
	}
	folder := "/"
	if len(pathParts) > 2 {
		folder = normalizeFolderPath(strings.Join(pathParts[1:len(pathParts)-1], "/"))
	}
	return &parsedVaultRef{
		collection: normalizeSlug(host, "default"),
		env:        env,
		folder:     folder,
		keyName:    keyName,
	}, nil
}
