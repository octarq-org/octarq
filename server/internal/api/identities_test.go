package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"testing"
	"time"

	"github.com/octarq-org/octarq/internal/models"
	"golang.org/x/crypto/bcrypt"
)

func TestListIdentities(t *testing.T) {
	srv, db := newTestHandler(t)
	cookies := loginCookies(t, srv)

	var user models.User
	if err := db.Where("email = ?", "admin").First(&user).Error; err != nil {
		t.Fatalf("find admin user: %v", err)
	}

	// 1. Unauthenticated -> 401
	rec := do(srv, "GET", "/api/account/identities", nil, "")
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("unauthenticated list identities: got %d, want 401", rec.Code)
	}

	// 2. Initially empty
	rec = do(srv, "GET", "/api/account/identities", cookies, "")
	if rec.Code != http.StatusOK {
		t.Fatalf("list identities: got %d, want 200", rec.Code)
	}
	var emptyList []IdentityRow
	if err := json.Unmarshal(rec.Body.Bytes(), &emptyList); err != nil {
		t.Fatalf("unmarshal empty list: %v", err)
	}
	if len(emptyList) != 0 {
		t.Fatalf("got %d identities, want 0", len(emptyList))
	}

	// 3. Seed identities for user and another user
	id1 := models.UserIdentity{
		UserID:    user.ID,
		Provider:  "github",
		Issuer:    "https://github.com",
		Subject:   "gh-12345",
		Email:     "admin@github.com",
		CreatedAt: time.Now(),
	}
	id2 := models.UserIdentity{
		UserID:    user.ID,
		Provider:  "google",
		Issuer:    "https://accounts.google.com",
		Subject:   "google-67890",
		Email:     "admin@gmail.com",
		CreatedAt: time.Now(),
	}
	otherUser := models.User{
		Email: "other@example.com",
	}
	if err := db.Create(&otherUser).Error; err != nil {
		t.Fatalf("create other user: %v", err)
	}
	idOther := models.UserIdentity{
		UserID:    otherUser.ID,
		Provider:  "github",
		Issuer:    "https://github.com",
		Subject:   "gh-99999",
		Email:     "other@github.com",
		CreatedAt: time.Now(),
	}
	if err := db.Create(&id1).Error; err != nil {
		t.Fatalf("create id1: %v", err)
	}
	if err := db.Create(&id2).Error; err != nil {
		t.Fatalf("create id2: %v", err)
	}
	if err := db.Create(&idOther).Error; err != nil {
		t.Fatalf("create idOther: %v", err)
	}

	rec = do(srv, "GET", "/api/account/identities", cookies, "")
	if rec.Code != http.StatusOK {
		t.Fatalf("list identities with rows: got %d, want 200", rec.Code)
	}
	var list []IdentityRow
	if err := json.Unmarshal(rec.Body.Bytes(), &list); err != nil {
		t.Fatalf("unmarshal list: %v", err)
	}
	if len(list) != 2 {
		t.Fatalf("got %d identities, want 2", len(list))
	}
	if list[0].Provider != "github" || list[0].Email != "admin@github.com" {
		t.Errorf("list[0] mismatch: %+v", list[0])
	}
	if list[1].Provider != "google" || list[1].Email != "admin@gmail.com" {
		t.Errorf("list[1] mismatch: %+v", list[1])
	}
}

func TestUnlinkIdentity(t *testing.T) {
	srv, db := newTestHandler(t)
	cookies := loginCookies(t, srv)

	var user models.User
	if err := db.Where("email = ?", "admin").First(&user).Error; err != nil {
		t.Fatalf("find admin user: %v", err)
	}

	id1 := models.UserIdentity{
		UserID:   user.ID,
		Provider: "github",
		Issuer:   "https://github.com",
		Subject:  "gh-12345",
		Email:    "admin@github.com",
	}
	if err := db.Create(&id1).Error; err != nil {
		t.Fatalf("create id1: %v", err)
	}

	// 1. Unauthenticated -> 401
	rec := do(srv, "DELETE", fmt.Sprintf("/api/account/identities/%d", id1.ID), nil, "")
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("unauthenticated unlink: got %d, want 401", rec.Code)
	}

	// 2. Non-existent identity -> 404
	rec = do(srv, "DELETE", "/api/account/identities/999999", cookies, "")
	if rec.Code != http.StatusNotFound {
		t.Fatalf("non-existent unlink: got %d, want 404", rec.Code)
	}

	// 3. Identity of another user -> 404
	otherUser := models.User{
		Email: "other2@example.com",
	}
	if err := db.Create(&otherUser).Error; err != nil {
		t.Fatalf("create other user: %v", err)
	}
	idOther := models.UserIdentity{
		UserID:   otherUser.ID,
		Provider: "github",
		Issuer:   "https://github.com",
		Subject:  "gh-88888",
		Email:    "other2@github.com",
	}
	if err := db.Create(&idOther).Error; err != nil {
		t.Fatalf("create idOther: %v", err)
	}
	rec = do(srv, "DELETE", fmt.Sprintf("/api/account/identities/%d", idOther.ID), cookies, "")
	if rec.Code != http.StatusNotFound {
		t.Fatalf("cross-user unlink: got %d, want 404", rec.Code)
	}

	// 4. Passwordless SSO user with only 1 identity -> 409 Conflict
	ssoUser := models.User{
		Email:        "sso@example.com",
		PasswordHash: "", // passwordless
	}
	if err := db.Create(&ssoUser).Error; err != nil {
		t.Fatalf("create ssoUser: %v", err)
	}
	ssoId1 := models.UserIdentity{
		UserID:   ssoUser.ID,
		Provider: "google",
		Issuer:   "https://accounts.google.com",
		Subject:  "google-sso-1",
		Email:    "sso@example.com",
	}
	if err := db.Create(&ssoId1).Error; err != nil {
		t.Fatalf("create ssoId1: %v", err)
	}

	// Create session for ssoUser
	ssoCookies := sessionCookies(t, ssoUser.ID, 1)
	rec = do(srv, "DELETE", fmt.Sprintf("/api/account/identities/%d", ssoId1.ID), ssoCookies, "")
	if rec.Code != http.StatusConflict {
		t.Fatalf("passwordless user unlinking last identity: got %d, want 409 (%s)", rec.Code, rec.Body.String())
	}

	// 5. Passwordless SSO user with 2 identities -> can unlink one
	ssoId2 := models.UserIdentity{
		UserID:   ssoUser.ID,
		Provider: "github",
		Issuer:   "https://github.com",
		Subject:  "github-sso-2",
		Email:    "sso@example.com",
	}
	if err := db.Create(&ssoId2).Error; err != nil {
		t.Fatalf("create ssoId2: %v", err)
	}
	rec = do(srv, "DELETE", fmt.Sprintf("/api/account/identities/%d", ssoId2.ID), ssoCookies, "")
	if rec.Code != http.StatusOK {
		t.Fatalf("passwordless user with 2 identities unlinking one: got %d, want 200 (%s)", rec.Code, rec.Body.String())
	}
	var count int64
	db.Model(&models.UserIdentity{}).Where("id = ?", ssoId2.ID).Count(&count)
	if count != 0 {
		t.Fatalf("ssoId2 still exists in db after unlink")
	}

	// 6. User with password can unlink their only identity
	hash, _ := bcrypt.GenerateFromPassword([]byte("somepass"), bcrypt.MinCost)
	db.Model(&user).Update("password_hash", string(hash))
	rec = do(srv, "DELETE", fmt.Sprintf("/api/account/identities/%d", id1.ID), cookies, "")
	if rec.Code != http.StatusOK {
		t.Fatalf("password user unlinking identity: got %d, want 200 (%s)", rec.Code, rec.Body.String())
	}
	db.Model(&models.UserIdentity{}).Where("id = ?", id1.ID).Count(&count)
	if count != 0 {
		t.Fatalf("id1 still exists in db after unlink")
	}

	// 7. Direct calls with nil Ctx
	h, _, _ := newTestHandlerRaw(t)
	if _, err := h.listIdentities(context.Background(), &ListIdentitiesInput{Ctx: nil}); err == nil {
		t.Error("expected error for nil Ctx in listIdentities")
	}
	if _, err := h.unlinkIdentity(context.Background(), &UnlinkIdentityInput{Ctx: nil}); err == nil {
		t.Error("expected error for nil Ctx in unlinkIdentity")
	}
}
