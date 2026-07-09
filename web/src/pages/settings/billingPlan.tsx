import { useEffect, useState } from "react";
import { NavLink, Navigate, Route, Routes } from "react-router-dom";
import { api, ApiError, Settings as SettingsData, OrgMember, LicenseStatus, Overview, PluginInfo } from "../../api";
import { Empty, Field, Modal, Toggle, timeAgo, ScreenWrap, PageHeader, GlassCard, Badge, Button } from "../../ui";
import { Settings as SettingsIcon, Cloud, Mail, Bell, Users, Trash2, Pencil, ShieldAlert, KeyRound, BellRing, Webhook, Plus, Send, AlertTriangle, CreditCard, Sparkles, Shield, DollarSign, Puzzle } from "lucide-react";
import { useTranslation } from "../../i18n";
import LLMProvidersSettings from "../LLMProviders";
import { useSettingsData, SavedBadge } from "./shared";

export function BillingPlanSettings() {
  const { t } = useTranslation();
  const [status, setStatus] = useState<LicenseStatus | null>(null);
  const [overview, setOverview] = useState<Overview | null>(null);
  const [unavailable, setUnavailable] = useState(false);

  useEffect(() => {
    api.license()
      .then(setStatus)
      .catch((e: ApiError) => {
        if (e.status === 404) setUnavailable(true);
      });
    api.overview(false)
      .then(setOverview)
      .catch(() => {});
  }, []);

  const plans = [
    {
      id: "starter",
      name: t("settings.planStarterName"),
      price: "$0",
      period: t("settings.periodForever"),
      description: t("settings.starterDesc"),
      features: [
        t("settings.featCoreDomainMapping"),
        t("settings.featUnlimitedLinks"),
        t("settings.featBasicAnalytics"),
        t("settings.featStandardEmail"),
        t("settings.featCommunitySupport"),
      ],
      current: unavailable || !status?.licensed,
    },
    {
      id: "pro",
      name: t("settings.planProName"),
      price: "$29",
      period: t("settings.periodMonth"),
      description: t("settings.proDesc"),
      features: [
        t("settings.featEverythingStarter"),
        t("settings.featVpsPanel"),
        t("settings.featSshVault"),
        t("settings.featSmtpRelay"),
        t("settings.featStorefront"),
        t("settings.featLicenseIssuance"),
      ],
      current: !unavailable && status?.licensed && status.tier?.toLowerCase() === "pro",
      popular: true,
    },
    {
      id: "elite",
      name: t("settings.planEliteName"),
      price: "$99",
      period: t("settings.periodMonth"),
      description: t("settings.eliteDesc"),
      features: [
        t("settings.featEverythingPro"),
        t("settings.featAiInbox"),
        t("settings.featOtpRouting"),
        t("settings.featMultipleLlm"),
        t("settings.featAuditLogging"),
        t("settings.featPrioritySupport"),
      ],
      current: !unavailable && status?.licensed && status.tier?.toLowerCase() === "elite",
    },
  ];

  return (
    <div className="space-y-6">
      <PageHeader
        title={t("settings.billingTitle")}
        description={t("settings.billingDescription")}
      />

      {/* Active plan card & metrics */}
      <div className="grid grid-cols-1 md:grid-cols-3 gap-6">
        <GlassCard className="p-5 flex flex-col justify-between">
          <div>
            <span className="text-[10px] text-white/40 uppercase tracking-widest block font-bold mb-1">{t("settings.activePlan")}</span>
            {unavailable ? (
              <div>
                <h3 className="text-xl font-bold text-white flex items-center gap-2">
                  {t("settings.openSource")}
                  <Badge tone="neutral">{t("settings.ossBuild")}</Badge>
                </h3>
                <p className="text-xs text-white/50 mt-1">{t("settings.noProEliteActive")}</p>
              </div>
            ) : status?.licensed ? (
              <div>
                <h3 className="text-xl font-bold text-white flex items-center gap-2 capitalize">
                  {status.tier} {t("settings.tierWord")}
                  <Badge tone="green">{t("settings.active")}</Badge>
                </h3>
                <p className="text-xs text-white/50 mt-1 truncate" title={status.email}>
                  {t("settings.licensedTo", { email: status.email || "" })}
                </p>
                <p className="text-[10px] text-white/40 mt-0.5">
                  {status.expiresAt ? t("settings.expiresDate", { date: status.expiresAt.slice(0, 10) }) : t("settings.lifetimeNeverExpires")}
                </p>
              </div>
            ) : (
              <div>
                <h3 className="text-xl font-bold text-white flex items-center gap-2">
                  {t("settings.unlicensed")}
                  <Badge tone="red">{t("settings.locked")}</Badge>
                </h3>
                <p className="text-xs text-white/50 mt-1">{t("settings.activateToUnlock")}</p>
              </div>
            )}
          </div>
          <div className="mt-6 border-t border-white/[0.06] pt-4 flex flex-col gap-2">
            {!unavailable && status?.licensed ? (
              <a
                href="https://app.octarq.com"
                target="_blank"
                rel="noreferrer"
                className="w-full text-center text-xs font-semibold py-2 px-3 rounded-xl bg-indigo-500 hover:bg-indigo-600 text-white transition-colors"
              >
                {t("settings.manageSubscription")}
              </a>
            ) : (
              <a
                href="https://octarq.com/pricing/"
                target="_blank"
                rel="noreferrer"
                className="w-full text-center text-xs font-semibold py-2 px-3 rounded-xl bg-white/10 hover:bg-white/15 text-white transition-colors"
              >
                {t("settings.viewPremiumPlans")}
              </a>
            )}
          </div>
        </GlassCard>

        <GlassCard className="p-5 flex flex-col justify-between">
          <div>
            <span className="text-[10px] text-white/40 uppercase tracking-widest block font-bold mb-1">{t("settings.redirectionLinks")}</span>
            <h3 className="text-xl font-bold text-white">
              {overview ? overview.links : "—"} {t("settings.linksUnit")}
            </h3>
            <p className="text-xs text-white/50 mt-1">
              {overview ? t("settings.activeRedirects", { count: String(overview.activeLinks) }) : t("settings.loadingMetrics")}
            </p>
          </div>
          <div className="mt-6 border-t border-white/[0.06] pt-4">
            <span className="text-xs text-emerald-400 font-semibold flex items-center gap-1">
              <Sparkles className="h-3.5 w-3.5" /> {t("settings.noLimitsApplied")}
            </span>
          </div>
        </GlassCard>

        <GlassCard className="p-5 flex flex-col justify-between">
          <div>
            <span className="text-[10px] text-white/40 uppercase tracking-widest block font-bold mb-1">{t("settings.managedDomains")}</span>
            <h3 className="text-xl font-bold text-white">
              {overview ? overview.domains : "—"} {t("settings.domainsUnit")}
            </h3>
            <p className="text-xs text-white/50 mt-1">
              {overview ? t("settings.domainsBreakdown", { link: String(overview.linkDomains), mail: String(overview.mailDomains) }) : t("settings.loadingMetrics")}
            </p>
          </div>
          <div className="mt-6 border-t border-white/[0.06] pt-4">
            <span className="text-xs text-emerald-400 font-semibold flex items-center gap-1">
              <Sparkles className="h-3.5 w-3.5" /> {t("settings.unlimitedDomains")}
            </span>
          </div>
        </GlassCard>
      </div>

      {/* Plans comparison */}
      <div className="space-y-4">
        <div>
          <h3 className="text-base font-bold text-white">{t("settings.planComparison")}</h3>
          <p className="text-xs text-white/50">{t("settings.planComparisonDesc")}</p>
        </div>

        <div className="grid grid-cols-1 md:grid-cols-3 gap-6">
          {plans.map((p) => (
            <GlassCard key={p.id} className={`p-5 flex flex-col justify-between border-t-2 relative ${p.current ? 'border-t-indigo-500 bg-indigo-500/[0.02]' : 'border-t-white/10'}`}>
              {p.popular && (
                <span className="absolute -top-3 right-4 bg-indigo-500 text-white text-[9px] font-bold uppercase px-2 py-0.5 rounded-full shadow-glow">
                  {t("settings.popular")}
                </span>
              )}
              <div className="space-y-4">
                <div>
                  <h4 className="font-bold text-white">{p.name}</h4>
                  <p className="text-xs text-white/40 mt-1 min-h-[32px]">{p.description}</p>
                </div>
                <div className="flex items-baseline gap-1">
                  <span className="text-3xl font-extrabold text-white">{p.price}</span>
                  <span className="text-xs text-white/40">/ {p.period}</span>
                </div>
                <ul className="space-y-2 border-t border-white/[0.06] pt-4">
                  {p.features.map((f, i) => (
                    <li key={i} className="text-xs text-white/70 flex items-center gap-2">
                      <span className="h-1 w-1 rounded-full bg-indigo-400" />
                      {f}
                    </li>
                  ))}
                </ul>
              </div>
              <div className="mt-6 pt-2">
                {p.current ? (
                  <Button variant="subtle" className="w-full text-xs cursor-default" disabled>
                    {t("settings.currentPlan")}
                  </Button>
                ) : (
                  <a
                    href={`https://octarq.com/pricing/${
                      p.id === "elite" ? "?plan=elite" : p.id === "pro" ? "?plan=pro" : ""
                    }`}
                    target="_blank"
                    rel="noreferrer"
                    className={`block w-full text-center text-xs font-semibold py-2 px-3 rounded-xl transition-colors ${
                      p.popular
                        ? "bg-indigo-500 hover:bg-indigo-600 text-white"
                        : "bg-transparent border border-white/20 hover:bg-white/5 text-white"
                    }`}
                  >
                    {p.price === "$0" ? t("settings.downgrade") : t("settings.upgradePlan")}
                  </a>
                )}
              </div>
            </GlassCard>
          ))}
        </div>
      </div>
    </div>
  );
}
