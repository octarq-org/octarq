package api

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"github.com/octarq-org/octarq/internal/models"
	"github.com/octarq-org/octarq/internal/notification"
)

func TestNotificationsAPI(t *testing.T) {
	_, srv, db := newTestHandlerWithInstance(t)

	// Ensure router is initialized
	router := notification.NewRouter(db)
	notification.SetDefaultRouter(router)

	user1Cookies := sessionCookies(t, 1, 1)
	user2Cookies := sessionCookies(t, 2, 1)

	// 1. Unauthorized request
	recUnauth := do(srv, http.MethodGet, "/api/notifications", nil, "")
	if recUnauth.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 unauthorized, got %d", recUnauth.Code)
	}

	// 2. Seed notifications for user 1 and user 2
	t1 := time.Now().Add(-2 * time.Hour)
	t2 := time.Now().Add(-1 * time.Hour)
	t3 := time.Now()

	n1 := models.Notification{
		UserID:    "1",
		OrgID:     "1",
		EventType: "security.login_failed",
		Title:     "Login Failed 1",
		Body:      "Details 1",
		CreatedAt: t1,
	}
	n2 := models.Notification{
		UserID:    "1",
		OrgID:     "1",
		EventType: "security.password_changed",
		Title:     "Password Changed",
		Body:      "Details 2",
		ReadAt:    &t2,
		CreatedAt: t2,
	}
	n3 := models.Notification{
		UserID:    "1",
		OrgID:     "1",
		EventType: "cron.job_failed",
		Title:     "Job Failed",
		Body:      "Details 3",
		CreatedAt: t3,
	}
	nOther := models.Notification{
		UserID:    "2",
		OrgID:     "1",
		EventType: "security.login_failed",
		Title:     "User2 Notification",
		Body:      "Details Other",
		CreatedAt: t3,
	}

	if err := db.Create(&n1).Error; err != nil {
		t.Fatalf("create n1: %v", err)
	}
	if err := db.Create(&n2).Error; err != nil {
		t.Fatalf("create n2: %v", err)
	}
	if err := db.Create(&n3).Error; err != nil {
		t.Fatalf("create n3: %v", err)
	}
	if err := db.Create(&nOther).Error; err != nil {
		t.Fatalf("create nOther: %v", err)
	}

	// 3. GET /api/notifications - List all for user 1
	recList := do(srv, http.MethodGet, "/api/notifications", user1Cookies, "")
	if recList.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", recList.Code, recList.Body.String())
	}
	var listResp []models.Notification
	if err := json.Unmarshal(recList.Body.Bytes(), &listResp); err != nil {
		t.Fatalf("unmarshal list: %v", err)
	}
	if len(listResp) != 3 {
		t.Fatalf("expected 3 notifications for user 1, got %d", len(listResp))
	}
	// Verify sort order created_at DESC (n3, then n2, then n1)
	if listResp[0].ID != n3.ID || listResp[1].ID != n2.ID || listResp[2].ID != n1.ID {
		t.Errorf("unexpected order: ids = [%d, %d, %d]", listResp[0].ID, listResp[1].ID, listResp[2].ID)
	}

	// 4. GET /api/notifications?unread_only=true
	recUnread := do(srv, http.MethodGet, "/api/notifications?unread_only=true", user1Cookies, "")
	if recUnread.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", recUnread.Code)
	}
	var unreadResp []models.Notification
	if err := json.Unmarshal(recUnread.Body.Bytes(), &unreadResp); err != nil {
		t.Fatalf("unmarshal unread: %v", err)
	}
	if len(unreadResp) != 2 {
		t.Fatalf("expected 2 unread notifications, got %d", len(unreadResp))
	}
	for _, n := range unreadResp {
		if n.ReadAt != nil {
			t.Errorf("expected nil ReadAt for unread notification %d", n.ID)
		}
	}

	// 5. GET /api/notifications with limit and offset
	recPage := do(srv, http.MethodGet, "/api/notifications?limit=1&offset=1", user1Cookies, "")
	if recPage.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", recPage.Code)
	}
	var pageResp []models.Notification
	if err := json.Unmarshal(recPage.Body.Bytes(), &pageResp); err != nil {
		t.Fatalf("unmarshal page: %v", err)
	}
	if len(pageResp) != 1 || pageResp[0].ID != n2.ID {
		t.Errorf("expected pageResp to contain n2, got %+v", pageResp)
	}

	// 6. PATCH /api/notifications/{id}/read
	recMarkUnauth := do(srv, http.MethodPatch, "/api/notifications/1/read", nil, "")
	if recMarkUnauth.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 unauthorized, got %d", recMarkUnauth.Code)
	}

	// Mark user 1's n1 as read
	recMark := do(srv, http.MethodPatch, "/api/notifications/1/read", user1Cookies, "")
	if recMark.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", recMark.Code, recMark.Body.String())
	}
	var checkN1 models.Notification
	if err := db.First(&checkN1, n1.ID).Error; err != nil {
		t.Fatalf("find n1: %v", err)
	}
	if checkN1.ReadAt == nil {
		t.Errorf("expected n1.ReadAt to be set")
	}

	// Mark again (already read) -> should succeed idempotently
	recMarkAgain := do(srv, http.MethodPatch, "/api/notifications/1/read", user1Cookies, "")
	if recMarkAgain.Code != http.StatusOK {
		t.Fatalf("expected 200 on repeat mark read, got %d", recMarkAgain.Code)
	}

	// User 1 trying to mark User 2's notification as read -> 404
	recMarkOther := do(srv, http.MethodPatch, "/api/notifications/4/read", user1Cookies, "")
	if recMarkOther.Code != http.StatusNotFound {
		t.Fatalf("expected 404 for marking other user's notification, got %d", recMarkOther.Code)
	}

	// Mark non-existent notification as read -> 404
	recMarkMissing := do(srv, http.MethodPatch, "/api/notifications/999999/read", user1Cookies, "")
	if recMarkMissing.Code != http.StatusNotFound {
		t.Fatalf("expected 404 for missing notification, got %d", recMarkMissing.Code)
	}

	// 7. DELETE /api/notifications/{id}
	recDelUnauth := do(srv, http.MethodDelete, "/api/notifications/1", nil, "")
	if recDelUnauth.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 unauthorized, got %d", recDelUnauth.Code)
	}

	// User 1 deletes User 2's notification -> 404
	recDelOther := do(srv, http.MethodDelete, "/api/notifications/4", user1Cookies, "")
	if recDelOther.Code != http.StatusNotFound {
		t.Fatalf("expected 404 for deleting other user's notification, got %d", recDelOther.Code)
	}

	// User 1 deletes non-existent notification -> 404
	recDelMissing := do(srv, http.MethodDelete, "/api/notifications/999999", user1Cookies, "")
	if recDelMissing.Code != http.StatusNotFound {
		t.Fatalf("expected 404 for deleting missing notification, got %d", recDelMissing.Code)
	}

	// User 1 deletes n1 -> 200
	recDel := do(srv, http.MethodDelete, "/api/notifications/1", user1Cookies, "")
	if recDel.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", recDel.Code, recDel.Body.String())
	}
	var count int64
	db.Model(&models.Notification{}).Where("id = ?", n1.ID).Count(&count)
	if count != 0 {
		t.Errorf("expected n1 to be deleted from DB, but count = %d", count)
	}

	// User 2's notification is still intact
	recListUser2 := do(srv, http.MethodGet, "/api/notifications", user2Cookies, "")
	if recListUser2.Code != http.StatusOK {
		t.Fatalf("expected 200 for user 2 list, got %d", recListUser2.Code)
	}
	var user2List []models.Notification
	json.Unmarshal(recListUser2.Body.Bytes(), &user2List)
	if len(user2List) != 1 || user2List[0].ID != nOther.ID {
		t.Errorf("user 2 notifications mismatch: %+v", user2List)
	}
}

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

func TestRegisteredChannelsAPI(t *testing.T) {
	_, srv, db := newTestHandlerWithInstance(t)

	router := notification.NewRouter(db)
	notification.SetDefaultRouter(router)

	userCookies := sessionCookies(t, 1, 1)

	// 1. GET /api/notification-preferences/channels
	recUnauth := do(srv, http.MethodGet, "/api/notification-preferences/channels", nil, "")
	if recUnauth.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 unauthorized, got %d", recUnauth.Code)
	}

	rec := do(srv, http.MethodGet, "/api/notification-preferences/channels", userCookies, "")
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var channels []RegisteredChannelInfo
	if err := json.Unmarshal(rec.Body.Bytes(), &channels); err != nil {
		t.Fatalf("unmarshal channels: %v", err)
	}
	if len(channels) < 2 {
		t.Fatalf("expected at least 2 channels (in_app and email), got %d", len(channels))
	}
	hasInApp, hasEmail := false, false
	for _, ch := range channels {
		if ch.Name == "in_app" {
			hasInApp = true
		}
		if ch.Name == "email" {
			hasEmail = true
			if len(ch.ConfigSchema) == 0 {
				t.Errorf("expected email channel to have ConfigSchema")
			}
		}
	}
	if !hasInApp || !hasEmail {
		t.Errorf("expected in_app and email channels, got %+v", channels)
	}

	// 2. GET /api/notification-channels?registered=true
	recReg := do(srv, http.MethodGet, "/api/notification-channels?registered=true", userCookies, "")
	if recReg.Code != http.StatusOK {
		t.Fatalf("expected 200 for registered=true, got %d", recReg.Code)
	}
	var regChannels []RegisteredChannelInfo
	if err := json.Unmarshal(recReg.Body.Bytes(), &regChannels); err != nil {
		t.Fatalf("unmarshal registered channels: %v", err)
	}
	if len(regChannels) < 2 {
		t.Fatalf("expected at least 2 registered channels, got %d", len(regChannels))
	}
}

func TestNotificationsDirectHandlerErrors(t *testing.T) {
	h, _, _ := newTestHandlerWithInstance(t)
	ctx := context.Background()

	// Missing huma context on listNotifications
	if _, err := h.listNotifications(ctx, &ListNotificationsInput{}); err == nil {
		t.Errorf("expected error for missing context")
	}

	// Missing huma context on markNotificationRead
	if _, err := h.markNotificationRead(ctx, &MarkNotificationReadInput{}); err == nil {
		t.Errorf("expected error for missing context")
	}

	// Missing huma context on deleteNotification
	if _, err := h.deleteNotification(ctx, &DeleteNotificationInput{}); err == nil {
		t.Errorf("expected error for missing context")
	}

	// Missing huma context on getNotificationPreferences
	if _, err := h.getNotificationPreferences(ctx, &GetNotificationPreferencesInput{}); err == nil {
		t.Errorf("expected error for missing context")
	}

	// Missing huma context on updateNotificationPreferences
	if _, err := h.updateNotificationPreferences(ctx, &UpdateNotificationPreferencesInput{}); err == nil {
		t.Errorf("expected error for missing context")
	}

	// Missing huma context on listRegisteredChannels
	if _, err := h.listRegisteredChannels(ctx, &ListRegisteredChannelsInput{}); err == nil {
		t.Errorf("expected error for missing context")
	}
}
