import { lazy, Suspense } from "react";
import { Navigate, Route, Routes } from "react-router-dom";
import { ScreenWrap } from "../ui";
import { RouteFallback } from "../App";
// Each settings panel is its own chunk, loaded when its sub-route is opened.
const PluginsSettings = lazy(() => import("./settings/plugins").then((m) => ({ default: m.PluginsSettings })));
const GeneralSettings = lazy(() => import("./settings/general").then((m) => ({ default: m.GeneralSettings })));
const WebhooksSettings = lazy(() => import("./settings/webhooks").then((m) => ({ default: m.WebhooksSettings })));
const NotificationChannels = lazy(() => import("./settings/notifications").then((m) => ({ default: m.NotificationChannels })));
const OrgMembersManager = lazy(() => import("./settings/members").then((m) => ({ default: m.OrgMembersManager })));
const AuthenticationSettings = lazy(() => import("./settings/auth").then((m) => ({ default: m.AuthenticationSettings })));
const InstancePluginsSettings = lazy(() => import("./settings/instance-plugins").then((m) => ({ default: m.InstancePluginsSettings })));
const InstanceSettings = lazy(() => import("./settings/instance").then((m) => ({ default: m.InstanceSettings })));
// Account panels — every settings page is served under /settings (one URL space).
const SecuritySettings = lazy(() => import("./settings/security").then((m) => ({ default: m.SecuritySettings })));
const ProfileSettings = lazy(() => import("./PersonalSettings").then((m) => ({ default: m.ProfileSettings })));
const ApiTokens = lazy(() => import("./PersonalSettings").then((m) => ({ default: m.ApiTokens })));

export default function SettingsPage() {
  return (
    <ScreenWrap>
      <Suspense fallback={<RouteFallback />}>
      <Routes>
        <Route path="/" element={<Navigate to="/settings/general" replace />} />
        <Route path="/general" element={<GeneralSettings />} />
        <Route path="/plugins" element={<PluginsSettings />} />
        <Route path="/webhooks" element={<WebhooksSettings />} />
        <Route path="/notifications" element={<NotificationChannels />} />
        <Route path="/members" element={<OrgMembersManager />} />
        <Route path="/auth" element={<Navigate to="/settings/instance/auth" replace />} />
        <Route path="/instance/auth" element={<AuthenticationSettings />} />
        <Route path="/instance" element={<InstanceSettings />} />
        <Route path="/instance/plugins" element={<InstancePluginsSettings />} />
        {/* Account panels (per-user) — same /settings space, no separate /personal tree. */}
        <Route path="/profile" element={<ProfileSettings />} />
        <Route path="/security" element={<SecuritySettings />} />
        <Route path="/tokens" element={<ApiTokens />} />
      </Routes>
      </Suspense>
    </ScreenWrap>
  );
}
