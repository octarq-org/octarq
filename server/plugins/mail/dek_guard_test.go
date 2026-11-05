package mail

import "testing"

func TestHasEncryptedDataSeesSavedSenderPasswords(t *testing.T) {
	p, _ := setupFullMailTestDB(t)
	wipeMailTables(t, p)

	has, err := p.HasEncryptedData()
	if err != nil {
		t.Fatalf("HasEncryptedData: %v", err)
	}
	if has {
		t.Fatal("an empty table must report no saved ciphertext")
	}

	if err := p.db.Create(&SMTPSender{OrgID: 1, Name: "relay", Host: "h", Port: 25, Pass: "ciphertext"}).Error; err != nil {
		t.Fatalf("create sender: %v", err)
	}
	has, err = p.HasEncryptedData()
	if err != nil {
		t.Fatalf("HasEncryptedData: %v", err)
	}
	if !has {
		t.Fatal("a saved sender password must report saved ciphertext")
	}

	// The column, not the row, decides: a sender without a password holds no
	// ciphertext and must not stop a boot.
	p.db.Model(&SMTPSender{}).Where("1 = 1").Update("pass", "")
	has, err = p.HasEncryptedData()
	if err != nil {
		t.Fatalf("HasEncryptedData: %v", err)
	}
	if has {
		t.Fatal("a sender without a password must report no saved ciphertext")
	}
}
