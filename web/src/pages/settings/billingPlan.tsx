import { useEffect, useState } from "react";
import { NavLink, Navigate, Route, Routes } from "react-router-dom";
import { api, ApiError, Settings as SettingsData, OrgMember, LicenseStatus, Overview, PluginInfo } from "../../api";
import { Empty, Field, Modal, Toggle, timeAgo, ScreenWrap, PageHeader, GlassCard, Badge, Button } from "../../ui";
import { Settings as SettingsIcon, Cloud, Mail, Bell, Users, Trash2, Pencil, ShieldAlert, KeyRound, BellRing, Webhook, Plus, Send, AlertTriangle, CreditCard, Sparkles, Shield, DollarSign, Puzzle } from "lucide-react";
import { useTranslation } from "../../i18n";
import LLMProvidersSettings from "../LLMProviders";
import { useSettingsData, SavedBadge } from "./shared";

export function BillingPlanSettings() {
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
      name: "Starter (OSS)",
      price: "$0",
      period: "forever",
      description: "Ideal for personal sites, hobby projects, and open-source hosting.",
      features: [
        "Core Domain Mapping",
        "Unlimited Redirection Links",
        "Basic Click Analytics",
        "Standard Email Routing",
        "Community Support",
      ],
      current: unavailable || !status?.licensed,
    },
    {
      name: "Octarq Pro",
      price: "$29",
      period: "month",
      description: "Perfect for growing projects, creators, and commercial workloads.",
      features: [
        "Everything in Starter",
        "Direct VPS Control Panel",
        "Secure SSH Credentials Vault",
        "Outbound SMTP Relay",
        "Storefront Product Catalog",
        "Cryptographic License Issuance",
      ],
      current: !unavailable && status?.licensed && status.tier?.toLowerCase() === "pro",
      popular: true,
    },
    {
      name: "Octarq Elite",
      price: "$99",
      period: "month",
      description: "Full AI automation, dedicated resources, and advanced compliance.",
      features: [
        "Everything in Pro",
        "AI Inbox Automation & Summaries",
        "Semantic OTP Code Routing",
        "Multiple LLM Provider Keys",
        "Comprehensive Audit Logging",
        "Priority Support Escalation",
      ],
      current: !unavailable && status?.licensed && status.tier?.toLowerCase() === "elite",
    },
  ];

  return (
    <div className="space-y-6">
      <PageHeader
        title="Billing & Plan"
        description="Monitor your subscription package, license details, and usage metrics."
      />

      {/* Active plan card & metrics */}
      <div className="grid grid-cols-1 md:grid-cols-3 gap-6">
        <GlassCard className="p-5 flex flex-col justify-between">
          <div>
            <span className="text-[10px] text-white/40 uppercase tracking-widest block font-bold mb-1">Active Plan</span>
            {unavailable ? (
              <div>
                <h3 className="text-xl font-bold text-white flex items-center gap-2">
                  Open Source
                  <Badge tone="neutral">OSS Build</Badge>
                </h3>
                <p className="text-xs text-white/50 mt-1">No Pro/Elite license active</p>
              </div>
            ) : status?.licensed ? (
              <div>
                <h3 className="text-xl font-bold text-white flex items-center gap-2 capitalize">
                  {status.tier} Tier
                  <Badge tone="green">Active</Badge>
                </h3>
                <p className="text-xs text-white/50 mt-1 truncate" title={status.email}>
                  Licensed to {status.email}
                </p>
                <p className="text-[10px] text-white/40 mt-0.5">
                  {status.expiresAt ? `Expires ${status.expiresAt.slice(0, 10)}` : "Lifetime / Never expires"}
                </p>
              </div>
            ) : (
              <div>
                <h3 className="text-xl font-bold text-white flex items-center gap-2">
                  Unlicensed
                  <Badge tone="red">Locked</Badge>
                </h3>
                <p className="text-xs text-white/50 mt-1">Activate a license key to unlock features</p>
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
                Manage Subscription on Octarq
              </a>
            ) : (
              <a
                href="https://octarq.com/pricing/"
                target="_blank"
                rel="noreferrer"
                className="w-full text-center text-xs font-semibold py-2 px-3 rounded-xl bg-white/10 hover:bg-white/15 text-white transition-colors"
              >
                View Premium Plans
              </a>
            )}
          </div>
        </GlassCard>

        <GlassCard className="p-5 flex flex-col justify-between">
          <div>
            <span className="text-[10px] text-white/40 uppercase tracking-widest block font-bold mb-1">Redirection Links</span>
            <h3 className="text-xl font-bold text-white">
              {overview ? overview.links : "—"} Links
            </h3>
            <p className="text-xs text-white/50 mt-1">
              {overview ? `${overview.activeLinks} active redirects` : "Loading metrics…"}
            </p>
          </div>
          <div className="mt-6 border-t border-white/[0.06] pt-4">
            <span className="text-xs text-emerald-400 font-semibold flex items-center gap-1">
              <Sparkles className="h-3.5 w-3.5" /> No limits applied
            </span>
          </div>
        </GlassCard>

        <GlassCard className="p-5 flex flex-col justify-between">
          <div>
            <span className="text-[10px] text-white/40 uppercase tracking-widest block font-bold mb-1">Managed Domains</span>
            <h3 className="text-xl font-bold text-white">
              {overview ? overview.domains : "—"} Domains
            </h3>
            <p className="text-xs text-white/50 mt-1">
              {overview ? `${overview.linkDomains} for links · ${overview.mailDomains} for mail` : "Loading metrics…"}
            </p>
          </div>
          <div className="mt-6 border-t border-white/[0.06] pt-4">
            <span className="text-xs text-emerald-400 font-semibold flex items-center gap-1">
              <Sparkles className="h-3.5 w-3.5" /> Unlimited domains
            </span>
          </div>
        </GlassCard>
      </div>

      {/* Plans comparison */}
      <div className="space-y-4">
        <div>
          <h3 className="text-base font-bold text-white">Octarq Plan Comparison</h3>
          <p className="text-xs text-white/50">Compare feature availability across tiers in the self-hosted environment.</p>
        </div>

        <div className="grid grid-cols-1 md:grid-cols-3 gap-6">
          {plans.map((p) => (
            <GlassCard key={p.name} className={`p-5 flex flex-col justify-between border-t-2 relative ${p.current ? 'border-t-indigo-500 bg-indigo-500/[0.02]' : 'border-t-white/10'}`}>
              {p.popular && (
                <span className="absolute -top-3 right-4 bg-indigo-500 text-white text-[9px] font-bold uppercase px-2 py-0.5 rounded-full shadow-glow">
                  Popular
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
                    Current Plan
                  </Button>
                ) : (
                  <a
                    href={`https://octarq.com/pricing/${
                      p.name.toLowerCase().includes("elite")
                        ? "?plan=elite"
                        : p.name.toLowerCase().includes("pro")
                        ? "?plan=pro"
                        : ""
                    }`}
                    target="_blank"
                    rel="noreferrer"
                    className={`block w-full text-center text-xs font-semibold py-2 px-3 rounded-xl transition-colors ${
                      p.popular
                        ? "bg-indigo-500 hover:bg-indigo-600 text-white"
                        : "bg-transparent border border-white/20 hover:bg-white/5 text-white"
                    }`}
                  >
                    {p.price === "$0" ? "Downgrade" : "Upgrade Plan"}
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
