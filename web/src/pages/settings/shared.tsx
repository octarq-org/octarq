import { useEffect, useState } from "react";
import { NavLink, Navigate, Route, Routes } from "react-router-dom";
import { api, ApiError, Settings as SettingsData, OrgMember, LicenseStatus, Overview, PluginInfo } from "../../api";
import { Empty, Field, Modal, Toggle, timeAgo, ScreenWrap, PageHeader, GlassCard, Badge, Button } from "../../ui";
import { Settings as SettingsIcon, Cloud, Mail, Bell, Users, Trash2, Pencil, ShieldAlert, KeyRound, BellRing, Webhook, Plus, Send, AlertTriangle, CreditCard, Sparkles, Shield, DollarSign, Puzzle } from "lucide-react";
import { useTranslation } from "../../i18n";
import LLMProvidersSettings from "../LLMProviders";

// ── Settings module pages (split out of the old monolithic General Settings) ──

// useSettingsData loads the shared workspace settings object once.
export function useSettingsData() {
  const [s, setS] = useState<SettingsData | null>(null);
  const reload = () => api.settings().then(setS);
  useEffect(() => { reload(); }, []);
  return { s, reload };
}

export function useInstanceSettingsData() {
  const { s: wS } = useSettingsData();
  const [s, setS] = useState<import("../../api").InstanceSettings | null>(null);
  const reload = () => api.instanceSettings().then(setS);
  useEffect(() => { if (wS?.isInstanceAdmin) reload(); }, [wS?.isInstanceAdmin]);
  return { s, reload };
}

export function SavedBadge({ on }: { on: boolean }) {
  const { t } = useTranslation();
  return on ? <Badge tone="green">{t("settings.saved")}</Badge> : null;
}

