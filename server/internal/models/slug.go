package models

import (
	"crypto/rand"
	"errors"
	"strings"

	"gorm.io/gorm"
)

const slugAlphabet = "abcdefghijkmnpqrstuvwxyz23456789"

func RandomSlug(n int) string {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		panic("models: crypto/rand entropy failure: " + err.Error())
	}
	for i := range b {
		b[i] = slugAlphabet[int(b[i])%len(slugAlphabet)]
	}
	return string(b)
}

var orgSlugLen = 8

var orgSlugAttempts = 20

var reservedOrgSlugs = map[string]bool{
	"admin": true, "api": true, "auth": true, "login": true, "logout": true,
	"sso": true, "static": true, "assets": true, "public": true, "_": true,
	".well-known": true,
	"webhook":     true, "billing": true, "cloud": true, "customer": true,
	"delivery": true, "license": true, "portal": true, "storefront": true,
	"update": true, "updates": true,
	"new": true, "settings": true, "help": true, "docs": true,
}

func IsReservedOrgSlug(slug string) bool {
	return reservedOrgSlugs[strings.ToLower(strings.TrimSpace(slug))]
}

type SlugStatus int

const (
	SlugAvailable SlugStatus = iota
	SlugReserved
	SlugTaken
)

func CheckOrgSlugAvailable(db *gorm.DB, slug string, targetOrgID uint) (SlugStatus, error) {
	if IsReservedOrgSlug(slug) {
		return SlugReserved, nil
	}

	var n int64
	query := db.Model(&Org{}).Where("slug = ?", slug)
	if targetOrgID > 0 {
		query = query.Where("id <> ?", targetOrgID)
	}
	if err := query.Count(&n).Error; err != nil {
		return SlugTaken, err
	}
	if n > 0 {
		return SlugTaken, nil
	}

	var history OrgSlugHistory
	err := db.Where("slug = ?", slug).First(&history).Error
	if err == nil {
		if targetOrgID > 0 && history.OrgID == targetOrgID {
			return SlugAvailable, nil
		}
		return SlugTaken, nil
	} else if !errors.Is(err, gorm.ErrRecordNotFound) {
		return SlugTaken, err
	}

	return SlugAvailable, nil
}

func AllocateOrgSlug(db *gorm.DB) (string, error) {
	if db == nil {
		return "", errors.New("models: no database configured")
	}
	for range orgSlugAttempts {
		slug := RandomSlug(orgSlugLen)
		status, err := CheckOrgSlugAvailable(db, slug, 0)
		if err != nil {
			return "", err
		}
		if status == SlugAvailable {
			return slug, nil
		}
	}
	return "", errors.New("models: could not allocate a free org slug")
}
