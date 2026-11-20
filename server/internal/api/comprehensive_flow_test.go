package api

import (
	"testing"

	"github.com/octarq-org/octarq/server/internal/models"
)

func TestComprehensiveAPI(t *testing.T) {
	srv, db := newTestHandler(t)

	cookies := sessionCookies(t, 1, 1)

	db.Create(&models.User{Email: "admin", IsInstanceAdmin: true})
	db.Create(&models.OrgMember{OrgID: 1, UserID: 1, Role: "owner"})

	comprehensiveTestOverview(t, srv, cookies)

	comprehensiveTestSettings(t, srv, cookies)

	comprehensiveTestNotificationChannels(t, srv, cookies)
}
