package models

import "time"

type OrgHost struct {
	ID        uint      `gorm:"primaryKey" json:"id"`
	OrgID     uint      `gorm:"column:owner_id;index" json:"orgId"`
	Host      string    `gorm:"uniqueIndex:idx_org_hosts_host_source;size:253" json:"host"`
	Traffic   bool      `json:"traffic"`
	Source    string    `gorm:"uniqueIndex:idx_org_hosts_host_source;size:64" json:"source"`
	CreatedAt time.Time `json:"createdAt"`
	UpdatedAt time.Time `json:"updatedAt"`
}

func (OrgHost) TableName() string {
	return "org_hosts"
}
