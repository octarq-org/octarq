package models

import (
	"testing"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

func TestFileModel(t *testing.T) {
	t.Parallel()

	f := File{}
	if f.TableName() != "files" {
		t.Errorf("File.TableName() = %q, want files", f.TableName())
	}

	db, err := gorm.Open(sqlite.Open("file:"+t.Name()+"?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite db: %v", err)
	}

	if err := db.AutoMigrate(&File{}); err != nil {
		t.Fatalf("AutoMigrate File failed: %v", err)
	}

	file1 := File{
		OrgID:       1,
		Name:        "avatar.png",
		Size:        1024,
		ContentType: "image/png",
		MD5:         "098f6bcd4621d373cade4e832627b4f6",
		Path:        "dedup/09/098f6bcd4621d373cade4e832627b4f6.png",
	}
	file2 := File{
		OrgID:       2,
		Name:        "document.pdf",
		Size:        2048,
		ContentType: "application/pdf",
		MD5:         "5eb63bbbe01eeed093cb22bb8f5acdc3",
		Path:        "dedup/5e/5eb63bbbe01eeed093cb22bb8f5acdc3.pdf",
	}
	if err := db.Create(&file1).Error; err != nil {
		t.Fatalf("create file1: %v", err)
	}
	if err := db.Create(&file2).Error; err != nil {
		t.Fatalf("create file2: %v", err)
	}

	// Multi-tenant check
	var org1Files []File
	if err := db.Where("owner_id = ?", 1).Find(&org1Files).Error; err != nil {
		t.Fatalf("query org1 files: %v", err)
	}
	if len(org1Files) != 1 || org1Files[0].Name != "avatar.png" {
		t.Fatalf("expected org1 file avatar.png, got %+v", org1Files)
	}

	// MD5 lookup
	var found File
	if err := db.Where("md5 = ?", "098f6bcd4621d373cade4e832627b4f6").First(&found).Error; err != nil {
		t.Fatalf("find by md5: %v", err)
	}
	if found.ID != file1.ID {
		t.Errorf("expected file ID %d, got %d", file1.ID, found.ID)
	}
}
