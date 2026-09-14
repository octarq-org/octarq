package models

import "time"

// File represents a stored file metadata record with MD5 deduplication.
type File struct {
	ID          uint      `gorm:"primaryKey" json:"id"`
	OrgID       uint      `gorm:"index;not null;column:owner_id" json:"orgId"`
	Name        string    `gorm:"size:255;not null" json:"name"`
	Size        int64     `gorm:"not null" json:"size"`
	ContentType string    `gorm:"size:128" json:"contentType"`
	MD5         string    `gorm:"index;size:32;not null" json:"md5"`
	Path        string    `gorm:"size:512;not null" json:"path"`
	CreatedAt   time.Time `json:"createdAt"`
	UpdatedAt   time.Time `json:"updatedAt"`
}

func (File) TableName() string {
	return "files"
}
