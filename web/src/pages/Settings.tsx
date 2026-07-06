import { useEffect, useState } from "react";
import { NavLink, Navigate, Route, Routes } from "react-router-dom";
import { api, ApiError, Settings as SettingsData, OrgMember, LicenseStatus, Overview } from "../api";
import { Empty, Field, Modal, Toggle, timeAgo, ScreenWrap, PageHeader, GlassCard, Badge, Button } from "../ui";
import { Settings as SettingsIcon, Cloud, Mail, Bell, Users, Trash2, Pencil, ShieldAlert, KeyRound, BellRing, Webhook, Plus, Send, AlertTriangle, CreditCard, Sparkles, Shield, DollarSign, Puzzle } from "lucide-react";
import { PluginInfo } from "../api";
import { useTranslation } from "../i18n";
import LLMProvidersSettings from "./LLMProviders";
import { PluginsSettings } from "./settings/plugins";
import { LicenseSettings } from "./settings/license";
import { GeneralSettings } from "./settings/general";
import { SecuritySettings } from "./settings/security";
import { WebhooksSettings, NotificationChannels } from "./settings/webhooks";
import { OrgMembersManager } from "./settings/members";
import { BillingPlanSettings } from "./settings/billingPlan";

// Re-exported for other pages that embed a settings section.
export { ProviderAccounts } from "./settings/providers";
export { LinkSettings, MailSettings } from "./settings/linkMail";
export { SMTPSenders } from "./settings/smtp";

export default function SettingsPage() {
  return (
    <ScreenWrap>
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
      </Routes>
    </ScreenWrap>
  );
}
