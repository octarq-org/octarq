package db

import (
	"database/sql"
	"os"
	"path/filepath"
	"testing"
)

func TestVerifySQLiteIntegrity(t *testing.T) {
	tempDir := t.TempDir()

	validDBPath := filepath.Join(tempDir, "valid.db")
	db, err := sql.Open("sqlite", validDBPath)
	if err != nil {
		t.Fatalf("Failed to open valid db: %v", err)
	}
	_, err = db.Exec("CREATE TABLE test (id INTEGER PRIMARY KEY);")
	if err != nil {
		t.Fatalf("Failed to create table in valid db: %v", err)
	}
	db.Close()

	if err := VerifySQLiteIntegrity(validDBPath); err != nil {
		t.Errorf("Expected valid DB to pass, got error: %v", err)
	}

	corruptDBPath := filepath.Join(tempDir, "corrupt.db")
	corruptData := make([]byte, 100)
	for i := range corruptData {
		corruptData[i] = 0x42
	}
	if err := os.WriteFile(corruptDBPath, corruptData, 0644); err != nil {
		t.Fatalf("Failed to create corrupt db file: %v", err)
	}

	err = VerifySQLiteIntegrity(corruptDBPath)
	if err == nil {
		t.Errorf("Expected corrupt DB to fail integrity check, got nil")
	}
}
