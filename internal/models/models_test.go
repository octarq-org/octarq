package models

import (
	"reflect"
	"testing"
	"time"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

func TestStringListRoundTrip(t *testing.T) {
	t.Parallel()

	cases := []StringList{nil, {}, {"go.example.com"}, {"go.example.com", "s.example.com"}}
	for _, in := range cases {
		v, err := in.Value()
		if err != nil {
			t.Fatalf("Value(%v): %v", in, err)
		}
		var out StringList
		if err := out.Scan(v); err != nil {
			t.Fatalf("Scan(%v): %v", v, err)
		}
		if len(out) != len(in) {
			t.Errorf("len mismatch: in=%v out=%v", in, out)
			continue
		}
		if len(in) > 0 && !reflect.DeepEqual([]string(in), []string(out)) {
			t.Errorf("round-trip mismatch: in=%v out=%v", in, out)
		}
	}
}

func TestStringListScanEdges(t *testing.T) {
	t.Parallel()

	var s StringList
	if err := s.Scan(nil); err != nil || s != nil {
		t.Errorf("Scan(nil) failed: %v", err)
	}

	if err := s.Scan([]byte("")); err != nil || s != nil {
		t.Errorf("Scan(\"\") failed: %v", err)
	}

	if err := s.Scan(123); err == nil {
		t.Errorf("Scan(int) expected error")
	}

	if err := s.Scan([]byte(`["a","b"]`)); err != nil || len(s) != 2 {
		t.Errorf("Scan([]byte) failed: %v", err)
	}
}

func TestHostListOperations(t *testing.T) {
	t.Parallel()

	var l HostList
	if err := l.Scan(nil); err != nil || l != nil {
		t.Errorf("Scan(nil) failed: %v", err)
	}
	if err := l.Scan([]byte("")); err != nil || l != nil {
		t.Errorf("Scan(\"\") failed: %v", err)
	}
	if err := l.Scan(12345); err == nil {
		t.Errorf("Scan(int) expected error")
	}

	val, err := l.Value()
	if err != nil || val != "[]" {
		t.Errorf("Value() empty expected \"[]\", got %v", val)
	}

	populated := HostList{
		{Host: "a.com", Enabled: true},
		{Host: "b.com", Enabled: false},
	}
	val, err = populated.Value()
	if err != nil {
		t.Fatalf("Value() populated error: %v", err)
	}

	var scanned HostList
	if err := scanned.Scan(val); err != nil || len(scanned) != 2 {
		t.Fatalf("Scan() populated error: %v", err)
	}

	enabled := populated.Enabled()
	if len(enabled) != 1 || enabled[0] != "a.com" {
		t.Errorf("Enabled() expected [a.com], got %v", enabled)
	}

	if populated.Blocks("a.com") {
		t.Errorf("Blocks(\"a.com\") expected false (since enabled)")
	}
	if !populated.Blocks("b.com") {
		t.Errorf("Blocks(\"b.com\") expected true (since disabled)")
	}
	if populated.Blocks("c.com") {
		t.Errorf("Blocks(\"c.com\") expected false (unlisted)")
	}
}

func TestHostListLegacyScan(t *testing.T) {
	t.Parallel()

	var l HostList
	if err := l.Scan(`["go.example.com","s.example.com"]`); err != nil {
		t.Fatalf("legacy scan: %v", err)
	}
	if len(l) != 2 || !l[0].Enabled || l[0].Host != "go.example.com" {
		t.Errorf("legacy []string not upgraded: %+v", l)
	}

	if err := l.Scan(`invalid-json`); err == nil {
		t.Errorf("Scan invalid json expected error")
	}
}

func TestTokenExpired(t *testing.T) {
	t.Parallel()

	tok := Token{}
	if tok.Expired() {
		t.Error("Token with nil ExpiresAt should not be expired")
	}

	future := time.Now().Add(time.Hour)
	tok.ExpiresAt = &future
	if tok.Expired() {
		t.Error("Token with future ExpiresAt should not be expired")
	}

	past := time.Now().Add(-time.Hour)
	tok.ExpiresAt = &past
	if !tok.Expired() {
		t.Error("Token with past ExpiresAt should be expired")
	}
}

func TestHashTokenDeterministicAndDistinct(t *testing.T) {
	t.Parallel()

	a := HashToken("oct_abc")
	if a != HashToken("oct_abc") {
		t.Error("HashToken not deterministic")
	}
	if a == HashToken("oct_xyz") {
		t.Error("HashToken collided on different inputs")
	}
	if len(a) != 64 {
		t.Errorf("hash length = %d, want 64", len(a))
	}
}

func TestSessionTableName(t *testing.T) {
	t.Parallel()

	s := Session{}
	if s.TableName() != "user_sessions" {
		t.Errorf("Session.TableName() = %q, want user_sessions", s.TableName())
	}
}

func TestRandomSlugAndStatKV(t *testing.T) {
	t.Parallel()

	slug := RandomSlug(8)
	if len(slug) != 8 {
		t.Errorf("RandomSlug(8) length = %d, want 8", len(slug))
	}

	stats := []StatKV{
		{Key: "k1", Count: 10},
		{Key: "k2", Count: 25},
	}
	if SumStatKV(stats) != 35 {
		t.Errorf("SumStatKV expected 35, got %d", SumStatKV(stats))
	}
}

func TestAllModelsAndOrgDataIsolation(t *testing.T) {
	t.Parallel()

	db, err := gorm.Open(sqlite.Open("file::memory:?cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatalf("failed to open sqlite DB: %v", err)
	}

	modelsList := AllModels()
	if len(modelsList) == 0 {
		t.Fatal("expected AllModels() to be non-empty")
	}

	if err := db.AutoMigrate(modelsList...); err != nil {
		t.Fatalf("AutoMigrate failed: %v", err)
	}

	org1 := Org{Name: "Org Alpha", Slug: "org-alpha"}
	org2 := Org{Name: "Org Beta", Slug: "org-beta"}
	if err := db.Create(&org1).Error; err != nil {
		t.Fatalf("create org1 error: %v", err)
	}
	if err := db.Create(&org2).Error; err != nil {
		t.Fatalf("create org2 error: %v", err)
	}

	// Create scoped data for Org 1 and Org 2
	t1 := Token{OrgID: org1.ID, Name: "Org1 Token", Hash: HashToken("tok1"), Prefix: "oct_1", UserID: 1}
	t2 := Token{OrgID: org2.ID, Name: "Org2 Token", Hash: HashToken("tok2"), Prefix: "oct_2", UserID: 2}
	db.Create(&t1)
	db.Create(&t2)

	w1 := Webhook{OrgID: org1.ID, Name: "Org1 Webhook", URL: "http://org1.example.com", Secret: "s1"}
	w2 := Webhook{OrgID: org2.ID, Name: "Org2 Webhook", URL: "http://org2.example.com", Secret: "s2"}
	db.Create(&w1)
	db.Create(&w2)

	a1 := AuditLog{OrgID: org1.ID, ActorID: 1, Action: "link.create"}
	a2 := AuditLog{OrgID: org2.ID, ActorID: 2, Action: "link.delete"}
	db.Create(&a1)
	db.Create(&a2)

	// Cross-tenant data isolation assertions: Org 1 query MUST NOT return Org 2 data
	var org1Tokens []Token
	db.Where("owner_id = ?", org1.ID).Find(&org1Tokens)
	if len(org1Tokens) != 1 || org1Tokens[0].Name != "Org1 Token" {
		t.Errorf("Org 1 token isolation failed: got %+v", org1Tokens)
	}

	var org1Webhooks []Webhook
	db.Where("owner_id = ?", org1.ID).Find(&org1Webhooks)
	if len(org1Webhooks) != 1 || org1Webhooks[0].Name != "Org1 Webhook" {
		t.Errorf("Org 1 webhook isolation failed: got %+v", org1Webhooks)
	}

	var org1Audit []AuditLog
	db.Where("org_id = ?", org1.ID).Find(&org1Audit)
	if len(org1Audit) != 1 || org1Audit[0].Action != "link.create" {
		t.Errorf("Org 1 audit log isolation failed: got %+v", org1Audit)
	}
}
