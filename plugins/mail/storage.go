package mail

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/octarq-org/octarq/plugin"
	"gorm.io/gorm"
)

// MailRawBlob stores raw email RFC822 blobs in the database when using DB storage.
type MailRawBlob struct {
	Key       string    `gorm:"primaryKey;size:255" json:"key"`
	Data      []byte    `gorm:"type:blob" json:"-"`
	UpdatedAt time.Time `json:"updatedAt"`
}

func (MailRawBlob) TableName() string { return "mail_raw_blobs" }

// DBStorageProvider implements plugin.StorageProvider backed by GORM database.
type DBStorageProvider struct {
	db *gorm.DB
}

func NewDBStorageProvider(db *gorm.DB) *DBStorageProvider {
	return &DBStorageProvider{db: db}
}

func (p *DBStorageProvider) Put(ctx context.Context, key string, data []byte) error {
	if p.db == nil {
		return errors.New("database connection is nil")
	}
	blob := MailRawBlob{
		Key:       key,
		Data:      data,
		UpdatedAt: time.Now(),
	}
	if err := p.db.WithContext(ctx).Save(&blob).Error; err != nil {
		return fmt.Errorf("failed to save raw blob: %w", err)
	}
	return nil
}

func (p *DBStorageProvider) Get(ctx context.Context, key string) ([]byte, error) {
	if p.db == nil {
		return nil, errors.New("database connection is nil")
	}
	var blob MailRawBlob
	err := p.db.WithContext(ctx).Where("key = ?", key).First(&blob).Error
	if err == nil && len(blob.Data) > 0 {
		return blob.Data, nil
	}
	return nil, plugin.ErrStorageNotFound
}

func (p *DBStorageProvider) Delete(ctx context.Context, key string) error {
	if p.db == nil {
		return errors.New("database connection is nil")
	}
	p.db.WithContext(ctx).Where("key = ?", key).Delete(&MailRawBlob{})
	return nil
}

// Stat asks the database for the byte count instead of loading the blob and
// measuring it: this is the call the storage meter makes, and a multi-megabyte
// original does not need to travel to the process to have its length taken.
func (p *DBStorageProvider) Stat(ctx context.Context, key string) (int64, error) {
	if p.db == nil {
		return 0, errors.New("database connection is nil")
	}
	var size int64
	err := p.db.WithContext(ctx).Model(&MailRawBlob{}).
		Where("key = ?", key).
		Select("length(data)").Scan(&size).Error
	if err == nil && size > 0 {
		return size, nil
	}
	return 0, plugin.ErrStorageNotFound
}
