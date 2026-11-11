package api

import (
	"testing"

	"github.com/octarq-org/octarq/server/internal/models"
)

func TestComprehensiveAPI(t *testing.T) {
	srv, db := newTestHandler(t)

	cookies := sessionCookies(t, 1, 1)

	// Seed the caller as an org owner so role-gated endpoints (e.g. settings)
	// behave as they do after a real admin login. Instance-level admin is now
	// bound to the stable User.IsInstanceAdmin flag (not org-1 ownership), which
	// a real admin login sets via bootstrapUserID — mirror that here.
	db.Create(&models.User{Email: "admin", IsInstanceAdmin: true}) // user id 1
	db.Create(&models.OrgMember{OrgID: 1, UserID: 1, Role: "owner"})

	// 1. Overview API
	comprehensiveTestOverview(t, srv, cookies)

	// 2. Settings API
	comprehensiveTestSettings(t, srv, cookies)

	// 3. Notification Channels CRUD
	comprehensiveTestNotificationChannels(t, srv, cookies)
}
