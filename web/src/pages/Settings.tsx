import { lazy, Suspense, useEffect, useState } from "react";
import { NavLink, Navigate, Route, Routes } from "react-router-dom";
import { api, ApiError, Settings as SettingsData, OrgMember, LicenseStatus, Overview } from "../api";
import { Empty, Field, Modal, Toggle, timeAgo, ScreenWrap, PageHeader, GlassCard, Badge, Button } from "../ui";
import { Settings as SettingsIcon, Cloud, Mail, Bell, Users, Trash2, Pencil, ShieldAlert, KeyRound, BellRing, Webhook, Plus, Send, AlertTriangle, CreditCard, Sparkles, Shield, DollarSign, Puzzle } from "lucide-react";
import { PluginInfo } from "../api";
import { useTranslation } from "../i18n";
import { RouteFallback } from "../App";
// Each settings panel is its own chunk, loaded when its sub-route is opened.
const PluginsSettings = lazy(() => import("./settings/plugins").then((m) => ({ default: m.PluginsSettings })));
const LicenseSettings = lazy(() => import("./settings/license").then((m) => ({ default: m.LicenseSettings })));
const GeneralSettings = lazy(() => import("./settings/general").then((m) => ({ default: m.GeneralSettings })));
const SecuritySettings = lazy(() => import("./settings/security").then((m) => ({ default: m.SecuritySettings })));
const WebhooksSettings = lazy(() => import("./settings/webhooks").then((m) => ({ default: m.WebhooksSettings })));
const NotificationChannels = lazy(() => import("./settings/webhooks").then((m) => ({ default: m.NotificationChannels })));
const OrgMembersManager = lazy(() => import("./settings/members").then((m) => ({ default: m.OrgMembersManager })));
const BillingPlanSettings = lazy(() => import("./settings/billingPlan").then((m) => ({ default: m.BillingPlanSettings })));
const InstanceSettings = lazy(() => import("./settings/instance").then((m) => ({ default: m.InstanceSettings })));

// Re-exported for other pages that embed a settings section.
export { ProviderAccounts } from "./settings/providers";
export { LinkSettings, MailSettings } from "./settings/linkMail";
export { SMTPSenders } from "./settings/smtp";

export default function SettingsPage() {
  return (
    <ScreenWrap>
      <Suspense fallback={<RouteFallback />}>
      <Routes>
        <Route path="/" element={<Navigate to="/settings/general" replace />} />
        <Route path="/general" element={<GeneralSettings />} />
        <Route path="/plugins" element={<PluginsSettings />} />
        <Route path="/security" element={<SecuritySettings />} />
        <Route path="/webhooks" element={<WebhooksSettings />} />
        <Route path="/billing" element={<BillingPlanSettings />} />
        <Route path="/license" element={<LicenseSettings />} />
        <Route path="/notifications" element={<NotificationChannels />} />
        <Route path="/members" element={<OrgMembersManager />} />
        <Route path="/instance" element={<InstanceSettings />} />
      </Routes>
      </Suspense>
    </ScreenWrap>
  );
}
