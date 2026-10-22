package models

import "gorm.io/gorm"

// Settings rows whose value is AES-GCM ciphertext under the envelope DEK. The
// envelope bootstrap reads this list before it creates a key: a fresh DEK on a
// database that still holds these rows would leave every one of them
// unreadable.
const (
	SettingGoogleClientSecret = "oauth.google.client_secret"
	SettingGitHubClientSecret = "oauth.github.client_secret"
	SettingMetricsToken       = "metrics_token"
)

// EncryptedSettingKeys lists every settings row that holds ciphertext.
var EncryptedSettingKeys = []string{
	SettingGoogleClientSecret,
	SettingGitHubClientSecret,
	SettingMetricsToken,
}

// HasEncryptedValues reports whether the database holds a value that only the
// envelope DEK can read: settings secrets, notification channel configs,
// webhook signing secrets, and TOTP shared secrets. A fresh DEK on such a
// database would make every one of them unreadable.
func HasEncryptedValues(db *gorm.DB) (bool, error) {
	var count int64
	if err := db.Model(&Setting{}).
		Where("key IN ? AND value <> ''", EncryptedSettingKeys).
		Count(&count).Error; err != nil {
		return false, err
	}
	if count > 0 {
		return true, nil
	}

	for _, column := range []struct {
		model  any
		column string
	}{
		{model: &NotificationChannel{}, column: "config"},
		{model: &Webhook{}, column: "secret"},
		{model: &User{}, column: "totp_secret"},
	} {
		count = 0
		if err := db.Model(column.model).Where(column.column + " <> ''").Count(&count).Error; err != nil {
			return false, err
		}
		if count > 0 {
			return true, nil
		}
	}
	return false, nil
}
