package dns

import "testing"

func TestHasEncryptedDataSeesSavedCredentials(t *testing.T) {
	p, _ := setupFullDNSTestDB(t)

	has, err := p.HasEncryptedData()
	if err != nil {
		t.Fatalf("HasEncryptedData: %v", err)
	}
	if has {
		t.Fatal("an empty table must report no saved ciphertext")
	}

	if err := p.db.Create(&ProviderAccount{OrgID: 1, Name: "cf", Type: "cloudflare", Config: "ciphertext"}).Error; err != nil {
		t.Fatalf("create provider: %v", err)
	}
	has, err = p.HasEncryptedData()
	if err != nil {
		t.Fatalf("HasEncryptedData: %v", err)
	}
	if !has {
		t.Fatal("a saved provider config must report saved ciphertext")
	}
}
