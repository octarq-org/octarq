package dns

// HasEncryptedData reports whether a saved DNS provider holds credential
// ciphertext, which only the envelope DEK can read. The host asks this when a
// boot had to create a fresh DEK: a positive answer means the key that sealed
// those credentials is gone, so the boot stops instead of failing on every DNS
// call later.
func (p *Plugin) HasEncryptedData() (bool, error) {
	var count int64
	if err := p.db.Model(&ProviderAccount{}).Where("config <> ''").Count(&count).Error; err != nil {
		return false, err
	}
	return count > 0, nil
}
