package mail

// HasEncryptedData reports whether a saved SMTP sender holds a password, which
// only the envelope DEK can read. The host asks this when a boot had to create
// a fresh DEK: a positive answer means the key that sealed those passwords is
// gone, so the boot stops instead of failing on every send later.
func (p *Plugin) HasEncryptedData() (bool, error) {
	var count int64
	if err := p.db.Model(&SMTPSender{}).Where("pass <> ''").Count(&count).Error; err != nil {
		return false, err
	}
	return count > 0, nil
}
