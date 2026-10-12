package api

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/octarq-org/octarq/server/internal/models"
)

func TestNotificationPreferencesAPI(t *testing.T) {
	_, srv, db := newTestHandlerWithInstance(t)

	user1Cookies := sessionCookies(t, 1, 1)
	user2Cookies := sessionCookies(t, 2, 1)

	// 1. Unauthorized GET
	recGetUnauth := do(srv, http.MethodGet, "/api/notification-preferences", nil, "")
	if recGetUnauth.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 unauthorized, got %d", recGetUnauth.Code)
	}

	// 2. GET empty preferences for user 1
	recGetEmpty := do(srv, http.MethodGet, "/api/notification-preferences", user1Cookies, "")
	if recGetEmpty.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", recGetEmpty.Code)
	}
	var emptyList []models.NotificationPreference
	json.Unmarshal(recGetEmpty.Body.Bytes(), &emptyList)
	if len(emptyList) != 0 {
		t.Fatalf("expected empty preferences, got %d", len(emptyList))
	}

	// 3. Unauthorized PUT
	recPutUnauth := do(srv, http.MethodPut, "/api/notification-preferences", nil, `{"preferences":[]}`)
	if recPutUnauth.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 unauthorized, got %d", recPutUnauth.Code)
	}

	// 4. PUT batch preferences for user 1
	putBody := `{
		"preferences": [
			{"eventPattern": "security.*", "channels": ["email", "in_app"]},
			{"eventPattern": "cron.*", "channels": ["in_app"]}
		]
	}`
	recPut := do(srv, http.MethodPut, "/api/notification-preferences", user1Cookies, putBody)
	if recPut.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", recPut.Code, recPut.Body.String())
	}
	var putResp []models.NotificationPreference
	if err := json.Unmarshal(recPut.Body.Bytes(), &putResp); err != nil {
		t.Fatalf("unmarshal put response: %v", err)
	}
	if len(putResp) != 2 {
		t.Fatalf("expected 2 preferences, got %d", len(putResp))
	}

	// 5. GET preferences for user 1
	recGet := do(srv, http.MethodGet, "/api/notification-preferences", user1Cookies, "")
	if recGet.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", recGet.Code)
	}
	var getResp []models.NotificationPreference
	json.Unmarshal(recGet.Body.Bytes(), &getResp)
	if len(getResp) != 2 {
		t.Fatalf("expected 2 preferences, got %d", len(getResp))
	}

	// 6. User 2 has empty preferences (isolation)
	recUser2 := do(srv, http.MethodGet, "/api/notification-preferences", user2Cookies, "")
	if recUser2.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", recUser2.Code)
	}
	var user2Resp []models.NotificationPreference
	json.Unmarshal(recUser2.Body.Bytes(), &user2Resp)
	if len(user2Resp) != 0 {
		t.Fatalf("expected user 2 to have 0 preferences, got %d", len(user2Resp))
	}

	// 7. PUT single legacy format
	putSingle := `{
		"eventPattern": "backup.*",
		"channels": ["email"]
	}`
	recSingle := do(srv, http.MethodPut, "/api/notification-preferences", user1Cookies, putSingle)
	if recSingle.Code != http.StatusOK {
		t.Fatalf("expected 200 for single pref update, got %d", recSingle.Code)
	}
	var singleResp []models.NotificationPreference
	json.Unmarshal(recSingle.Body.Bytes(), &singleResp)
	foundBackup := false
	for _, p := range singleResp {
		if p.EventPattern == "backup.*" {
			foundBackup = true
		}
	}
	if !foundBackup {
		t.Errorf("expected backup.* preference to be added")
	}

	_ = db
}
