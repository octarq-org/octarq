import { lazy, Suspense } from "react";
import { Navigate, Route, Routes } from "react-router-dom";
import { ScreenWrap } from "../ui";
import { RouteFallback } from "../App";
import { InstanceExitRedirect } from "./instance/redirect";
// Each settings panel is its own chunk, loaded when its sub-route is opened.
const PluginsSettings = lazy(() => import("./settings/plugins").then((m) => ({ default: m.PluginsSettings })));
const GeneralSettings = lazy(() => import("./settings/general").then((m) => ({ default: m.GeneralSettings })));
const WebhooksSettings = lazy(() => import("./settings/webhooks").then((m) => ({ default: m.WebhooksSettings })));
const NotificationChannels = lazy(() => import("./settings/notifications").then((m) => ({ default: m.NotificationChannels })));
const OrgMembersManager = lazy(() => import("./settings/members").then((m) => ({ default: m.OrgMembersManager })));
// Account panels — every settings page is served under /settings (one URL space).
const SecuritySettings = lazy(() => import("./settings/security").then((m) => ({ default: m.SecuritySettings })));
const ProfileSettings = lazy(() => import("./PersonalSettings").then((m) => ({ default: m.ProfileSettings })));
const ApiTokens = lazy(() => import("./PersonalSettings").then((m) => ({ default: m.ApiTokens })));
const AppearanceSettings = lazy(() => import("./settings/appearance").then((m) => ({ default: m.AppearanceSettings })));

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
        {/* The instance console moved out of /settings into its own /instance
            basename (R4) — old paths redirect with a full page navigation. */}
        <Route path="/instance/*" element={<InstanceExitRedirect />} />
        <Route path="/auth" element={<InstanceExitRedirect to="/instance/auth" />} />
        <Route path="/link-settings" element={<InstanceExitRedirect to="/instance/link-settings" />} />
        {/* Account panels (per-user) — same /settings space, no separate /personal tree. */}
        <Route path="/profile" element={<ProfileSettings />} />
        <Route path="/security" element={<SecuritySettings />} />
        <Route path="/tokens" element={<ApiTokens />} />
        <Route path="/appearance" element={<AppearanceSettings />} />
      </Routes>
      </Suspense>
    </ScreenWrap>
  );
}
