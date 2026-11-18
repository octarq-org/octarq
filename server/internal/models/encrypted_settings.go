package models

import "gorm.io/gorm"

// EncryptedSettingKeys lists every settings row that holds ciphertext under the
// instance envelope DEK. OAuth secrets and the metrics token have migrated to
// VaultService (per-item SubjectVault DEKs).
var EncryptedSettingKeys = []string{}

// HasEncryptedValues reports whether the database holds a value that only the
// instance envelope DEK can read (currently TOTP shared secrets). A fresh DEK
// on such a database would make every one of them unreadable.
func HasEncryptedValues(db *gorm.DB) (bool, error) {
	if len(EncryptedSettingKeys) > 0 {
		var count int64
		if err := db.Model(&Setting{}).
			Where("key IN ? AND value <> ''", EncryptedSettingKeys).
			Count(&count).Error; err != nil {
			return false, err
		}
		if count > 0 {
			return true, nil
		}
	}

	var count int64
	if err := db.Model(&User{}).Where("totp_secret <> ''").Count(&count).Error; err != nil {
		return false, err
	}
	return count > 0, nil
}
