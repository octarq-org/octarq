package plugin

import (
	"context"
	"errors"
	"time"
)

// ServiceVault is the well-known service name under which the core Vault
// plugin provides the unified credential and secret store (contract type
// VaultService).
const ServiceVault = "vault.service"

var (
	// ErrVaultUnavailable reports that no vault service is mounted on this host.
	ErrVaultUnavailable = errors.New("vault service unavailable")
	// ErrSecretNotFound reports that the requested vault item does not exist.
	ErrSecretNotFound = errors.New("vault secret not found")
)

// VaultScope identifies the ownership boundary of a vault collection or item.
type VaultScope string

const (
	VaultScopeInstance VaultScope = "instance" // org_id = 0, instance-admin only
	VaultScopeOrg      VaultScope = "org"      // org_id > 0, workspace-scoped
	VaultScopeUser     VaultScope = "user"     // org_id > 0, owner_user_id > 0
)

// VaultItemMeta is the non-secret metadata projection of a vault item.
// Plaintext secrets and ciphertexts are never included here.
type VaultItemMeta struct {
	ID                   uint           `json:"id"`
	SecretID             string         `json:"secretId"`
	OrgID                uint           `json:"orgId"`
	Scope                VaultScope     `json:"scope"`
	OwnerUserID          uint           `json:"ownerUserId,omitempty"`
	CollectionID         uint           `json:"collectionId"`
	CollectionSlug       string         `json:"collectionSlug"`
	Environment          string         `json:"environment"`
	FolderPath           string         `json:"folderPath"`
	ItemType             string         `json:"itemType"`
	ProviderType         string         `json:"providerType,omitempty"`
	Provider             string         `json:"provider,omitempty"`
	KeyName              string         `json:"keyName,omitempty"`
	Name                 string         `json:"name"`
	Tags                 []string       `json:"tags,omitempty"`
	IsDefault            bool           `json:"isDefault"`
	CurrentVersion       int            `json:"currentVersion"`
	Configured           bool           `json:"configured"`
	FieldKeys            []string       `json:"fieldKeys,omitempty"`
	Metadata             map[string]any `json:"metadata,omitempty"`
	ExpiresAt            *time.Time     `json:"expiresAt,omitempty"`
	RotationIntervalDays int            `json:"rotationIntervalDays,omitempty"`
	LastRotatedAt        *time.Time     `json:"lastRotatedAt,omitempty"`
	Ref                  string         `json:"ref"`
	CreatedAt            time.Time      `json:"createdAt"`
	UpdatedAt            time.Time      `json:"updatedAt"`
}

// VaultSecret carries the decrypted payload of one vault item version.
type VaultSecret struct {
	Meta    VaultItemMeta  `json:"meta"`
	Version int            `json:"version"`
	Payload map[string]any `json:"payload"`
}

// PutSecretRequest creates or replaces a secret item inside the vault.
type PutSecretRequest struct {
	OrgID                uint
	Scope                VaultScope
	OwnerUserID          uint
	CollectionSlug       string
	Environment          string
	FolderPath           string
	ItemType             string
	ProviderType         string
	Provider             string
	KeyName              string
	Name                 string
	Tags                 []string
	IsDefault            bool
	Metadata             map[string]any
	Payload              map[string]any
	ChangeNote           string
	ActorUserID          uint
	ExpiresAt            *time.Time
	RotationIntervalDays int
}

// VaultFilter filters listed vault item metadata.
type VaultFilter struct {
	Scope          VaultScope
	OwnerUserID    uint
	CollectionSlug string
	Environment    string
	FolderPath     string
	ItemType       string
	ProviderType   string
	Provider       string
	KeyName        string
	Query          string
}

// VaultService is the cross-plugin contract for storing, resolving, rotating,
// and shredding unified credentials and secrets across Core, Community Plugins,
// and downstream modules.
type VaultService interface {
	PutSecret(ctx context.Context, req PutSecretRequest) (*VaultItemMeta, error)
	UpdateSecret(ctx context.Context, orgID, itemID uint, name *string, metadata map[string]any, payload map[string]any, isDefault *bool, changeNote string, actorUserID uint) (*VaultItemMeta, error)
	GetMeta(ctx context.Context, orgID, itemID uint) (*VaultItemMeta, error)
	GetSecret(ctx context.Context, orgID, itemID uint) (*VaultSecret, error)
	ResolveByRef(ctx context.Context, orgID uint, ref string) (*VaultSecret, error)
	ResolveInstanceSecret(ctx context.Context, keyName string) (*VaultSecret, error)
	PutInstanceSecret(ctx context.Context, req PutSecretRequest) (*VaultItemMeta, error)
	DeleteInstanceSecret(ctx context.Context, keyName string) error
	ListItems(ctx context.Context, orgID uint, filter VaultFilter) ([]VaultItemMeta, error)
	DeleteSecret(ctx context.Context, orgID, itemID uint) error
	ComputeTOTP(ctx context.Context, orgID, itemID uint) (code string, remainingSec int, err error)
}
