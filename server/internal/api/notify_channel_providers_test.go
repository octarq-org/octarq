package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"testing"

	"github.com/octarq-org/octarq/internal/models"
	"github.com/octarq-org/octarq/internal/notify"
	"github.com/octarq-org/octarq/plugin"
)

// mockPlugin implements plugin.Plugin & plugin.Describer for testing plugin-contributed notification channel types.
type mockPlugin struct {
	name     string
	title    string
	desc     string
	icon     string
	category string
}

func (m *mockPlugin) Name() string { return m.name }
func (m *mockPlugin) Describe() plugin.Info {
	return plugin.Info{
		Title:       m.title,
		Description: m.desc,
		Icon:        m.icon,
		Category:    m.category,
	}
}
func (m *mockPlugin) Models() []any { return nil }
func (m *mockPlugin) Mount(mux plugin.Mux, ctx *plugin.Context) {
	if ctx.RegisterNotifier != nil {
		ctx.RegisterNotifier(m.name, func(c context.Context, cfgJSON, text string) error {
			if strings.Contains(cfgJSON, "fail_test") {
				return fmt.Errorf("mock delivery failure")
			}
			return nil
		})
	}
}

func TestNotificationChannelTypes_FilteringAndSecrets(t *testing.T) {
	p := &mockPlugin{
		name:     "mocknotify",
		title:    "Mock Notifier",
		desc:     "Delivers notifications to Mock Service",
		icon:     "bell",
		category: plugin.CategoryMessaging,
	}

	// Explicitly register descriptor in notify package for mock plugin
	notify.RegisterWithDescriptor(notify.Descriptor{
		Type:        p.name,
		Title:       p.title,
		Description: p.desc,
		Icon:        p.icon,
		PluginName:  p.name,
	}, func(c context.Context, cfgJSON, text string) error {
		return nil
	})

	h, srv, db := newTestHandlerWithInstance(t)
	h.SetPlugins([]plugin.Plugin{p})

	org1Cookies := sessionCookies(t, 1, 1)
	org2Cookies := sessionCookies(t, 2, 2)

	// Org 1: Enable mocknotify plugin explicitly
	db.Create(&models.PluginSetting{OrgID: 1, Plugin: "mocknotify", Enabled: true})
	// Org 2: Disable mocknotify plugin explicitly
	db.Create(&models.PluginSetting{OrgID: 2, Plugin: "mocknotify", Enabled: false})

	// 1 & 2: Test /api/notification-channel-types for Org 1 vs Org 2
	rec1 := do(srv, http.MethodGet, "/api/notification-channel-types", org1Cookies, "")
	if rec1.Code != http.StatusOK {
		t.Fatalf("org1 channel types failed: %d — %s", rec1.Code, rec1.Body.String())
	}
	var types1 []NotificationChannelType
	json.Unmarshal(rec1.Body.Bytes(), &types1)

	hasMock1 := false
	for _, tp := range types1 {
		if tp.Type == "mocknotify" {
			hasMock1 = true
			if tp.Title != "Mock Notifier" {
				t.Errorf("got title %q, want %q", tp.Title, "Mock Notifier")
			}
		}
	}
	if !hasMock1 {
		t.Errorf("org1 with enabled plugin should include mocknotify in type list")
	}

	rec2 := do(srv, http.MethodGet, "/api/notification-channel-types", org2Cookies, "")
	if rec2.Code != http.StatusOK {
		t.Fatalf("org2 channel types failed: %d — %s", rec2.Code, rec2.Body.String())
	}
	var types2 []NotificationChannelType
	json.Unmarshal(rec2.Body.Bytes(), &types2)

	hasMock2 := false
	for _, tp := range types2 {
		if tp.Type == "mocknotify" {
			hasMock2 = true
		}
	}
	if hasMock2 {
		t.Errorf("org2 with disabled plugin must omit mocknotify from type list")
	}

	// 3: Roundtrip create -> list -> test -> delete for plugin-provided channel type
	createBody := `{"name":"My Mock Channel","type":"mocknotify","config":"{\"secretKey\":\"super-secret-123\",\"targetUrl\":\"https://example.com/hook\"}"}`
	recCreate := do(srv, http.MethodPost, "/api/notification-channels", org1Cookies, createBody)
	if recCreate.Code != http.StatusCreated {
		t.Fatalf("create plugin channel failed: %d — %s", recCreate.Code, recCreate.Body.String())
	}
	var created models.NotificationChannel
	json.Unmarshal(recCreate.Body.Bytes(), &created)
	if created.ID == 0 || created.Type != "mocknotify" {
		t.Fatalf("unexpected created channel: %+v", created)
	}

	// 4: Verify secrets in list response are redacted ("super-secret-123" -> "[REDACTED]")
	recList := do(srv, http.MethodGet, "/api/notification-channels", org1Cookies, "")
	if recList.Code != http.StatusOK {
		t.Fatalf("list channels failed: %d", recList.Code)
	}
	if strings.Contains(recList.Body.String(), "super-secret-123") {
		t.Errorf("list notification channels leaked raw secret! Response: %s", recList.Body.String())
	}
	if !strings.Contains(recList.Body.String(), "[REDACTED]") {
		t.Errorf("list notification channels expected [REDACTED] in secret field. Response: %s", recList.Body.String())
	}

	recTest := do(srv, http.MethodPost, fmt.Sprintf("/api/notification-channels/%d/test", created.ID), org1Cookies, "")
	if recTest.Code != http.StatusOK {
		t.Errorf("test notification channel failed: %d — %s", recTest.Code, recTest.Body.String())
	}

	recDel := do(srv, http.MethodDelete, fmt.Sprintf("/api/notification-channels/%d", created.ID), org1Cookies, "")
	if recDel.Code != http.StatusOK {
		t.Errorf("delete notification channel failed: %d — %s", recDel.Code, recDel.Body.String())
	}
}
