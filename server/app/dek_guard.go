package app

import (
	"fmt"

	"github.com/octarq-org/octarq/server/internal/crypto"
	"github.com/octarq-org/octarq/server/internal/models"
	"github.com/octarq-org/octarq/server/plugin"
)

func (a *App) verifyNoOrphanedPluginData() error {
	if !a.cipher.DEKCreated() {
		return nil
	}
	for _, p := range a.plugins {
		reporter, ok := p.(plugin.EncryptedDataReporter)
		if !ok {
			continue
		}
		hasData, err := reporter.HasEncryptedData()
		if err != nil {
			return fmt.Errorf("could not check the saved values of plugin %q: %w", p.Name(), err)
		}
		if !hasData {
			continue
		}
		if delErr := a.gdb.Delete(&models.Setting{}, "key = ?", crypto.DEKSettingKey).Error; delErr != nil {
			return fmt.Errorf("plugin %q holds saved values but the key that decrypts them is missing. Delete the settings row %q, then restore the backup taken before the upgrade: %w", p.Name(), crypto.DEKSettingKey, delErr)
		}
		return fmt.Errorf("plugin %q holds saved values but the key that decrypts them is missing. Restore the backup taken before the upgrade. The settings row %q was removed again, so the next boot repeats this check", p.Name(), crypto.DEKSettingKey)
	}
	return nil
}
