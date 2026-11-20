package vault

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"

	"gorm.io/gorm"
)

const serviceTokenPrefix = "octvq_"

func hashServiceToken(raw string) string {
	sum := sha256.Sum256([]byte(raw))
	return hex.EncodeToString(sum[:])
}

func generateServiceToken() (raw string, prefix string, hash string, err error) {
	buf := make([]byte, 24)
	if _, err = rand.Read(buf); err != nil {
		return "", "", "", err
	}
	raw = serviceTokenPrefix + hex.EncodeToString(buf)
	prefix = raw[:14]
	hash = hashServiceToken(raw)
	return raw, prefix, hash, nil
}

func (p *Plugin) CreateServiceToken(ctx context.Context, orgID uint, name string, collectionSlug, env, folderPrefix, perms string, expiresAt *time.Time, actorUserID uint) (*VaultServiceToken, string, error) {
	if orgID == 0 {
		return nil, "", errors.New("vault: organization id required")
	}
	if p.db == nil {
		return nil, "", errors.New("vault: database unavailable")
	}
	name = strings.TrimSpace(name)
	if name == "" {
		return nil, "", errors.New("vault: token name is required")
	}
	col, err := p.findCollection(ctx, orgID, "org", normalizeSlug(collectionSlug, "default"))
	if err != nil {
		return nil, "", fmt.Errorf("vault: collection %q not found", collectionSlug)
	}
	perms = strings.ToLower(strings.TrimSpace(perms))
	if perms == "" {
		perms = "read"
	} else if perms != "read" {
		return nil, "", errors.New("vault: only \"read\" permission is supported for service tokens")
	}
	raw, prefix, hash, err := generateServiceToken()
	if err != nil {
		return nil, "", err
	}
	tok := VaultServiceToken{
		OrgID:        orgID,
		Name:         name,
		TokenPrefix:  prefix,
		TokenHash:    hash,
		CollectionID: col.ID,
		Environment:  normalizeEnvironment(env),
		FolderPrefix: normalizeFolderPath(folderPrefix),
		Permissions:  perms,
		ExpiresAt:    expiresAt,
		CreatedBy:    actorUserID,
	}
	if err := p.db.WithContext(ctx).Create(&tok).Error; err != nil {
		return nil, "", err
	}
	return &tok, raw, nil
}

func (p *Plugin) AuthenticateServiceToken(ctx context.Context, bearer string) (*VaultServiceToken, error) {
	if p.db == nil {
		return nil, errors.New("vault: database unavailable")
	}
	raw := strings.TrimSpace(strings.TrimPrefix(bearer, "Bearer "))
	if !strings.HasPrefix(raw, serviceTokenPrefix) {
		return nil, errors.New("invalid vault service token")
	}
	hash := hashServiceToken(raw)
	var tok VaultServiceToken
	if err := p.db.WithContext(ctx).Where("token_hash = ?", hash).First(&tok).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, errors.New("vault service token not found")
		}
		return nil, err
	}
	if tok.RevokedAt != nil {
		return nil, errors.New("vault service token revoked")
	}
	if tok.ExpiresAt != nil && time.Now().After(*tok.ExpiresAt) {
		return nil, errors.New("vault service token expired")
	}
	now := time.Now()
	_ = p.db.WithContext(ctx).Model(&tok).Update("last_used_at", &now).Error
	return &tok, nil
}
