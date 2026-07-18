import { useEffect, useMemo, useState } from "react";
import { api, Domain, HostEntry, ProviderAccount } from "../../../api";
import { dnsApi, DNSRecord, DNSVerifyResult, HostDNSStatus, LinkHostStatus, DNSRecordStatus } from "../api";
import { Code, Empty, Field, Guide, HostList, Modal, Toggle, timeAgo, ScreenWrap, PageHeader, GlassCard, Badge, Button, Select } from "../../../ui";
import { Globe, RefreshCw, Plus, Trash2, ArrowRight, ShieldCheck, Mail, Link as LinkIcon, Cloud } from "lucide-react";
import { ProviderAccounts } from "../../../pages/Settings";
import { useTranslation } from "../../../i18n";

function DnsStatusBadge({ status, label }: { status: DNSRecordStatus; label: string }) {
  const { t } = useTranslation();
  const tone = status.healthy ? "green" : status.set ? "amber" : "red";
  const text = status.healthy ? t("domains.configured") : status.set ? t("domains.misconfigured") : t("domains.missing");
  return (
    <div className="flex flex-col items-center p-3 rounded-xl bg-white/[0.02] border border-white/[0.04]">
      <span className="text-[10px] uppercase font-bold text-white/40 tracking-wider">{label}</span>
      <div className="mt-2"><Badge tone={tone as any}>{text}</Badge></div>
    </div>
  );
}

export function DnsHostRow({ host }: { host: HostDNSStatus }) {
  const dkimLabel = host.dkim.selector ? `DKIM (${host.dkim.selector})` : "DKIM";
  return (
    <div className="rounded-xl bg-white/[0.015] border border-white/[0.04] p-3">
      <div className="flex items-center gap-2 mb-2.5 px-1">
        <Mail className="h-3.5 w-3.5 text-indigo-400/70 shrink-0" />
        <span className="text-xs font-mono text-white/70 truncate">{host.host}</span>
      </div>
      <div className="grid grid-cols-3 gap-2.5">
        <DnsStatusBadge status={host.spf} label="SPF" />
        <DnsStatusBadge status={host.dkim} label={dkimLabel} />
        <DnsStatusBadge status={host.dmarc} label="DMARC" />
      </div>
    </div>
  );
}

// Short-link host: CNAME confirmed into zone (green), resolves but target
// unverified — e.g. proxied/A-record (amber), or not resolving (red).
export function LinkHostRow({ link }: { link: LinkHostStatus }) {
  const { t } = useTranslation();
  const tone = link.healthy ? "green" : link.set ? "amber" : "red";
  const text = link.healthy ? t("domains.pointsToZone") : link.set ? t("domains.unverified") : t("domains.notResolving");
  const detail = link.healthy
    ? `CNAME → ${link.cname}`
    : link.set
      ? (link.cname ? `CNAME → ${link.cname}` : t("domains.resolvesNoCname", { target: link.target }))
      : t("domains.addCname", { target: link.target });
  return (
    <div className="flex items-center gap-3 rounded-xl bg-white/[0.015] border border-white/[0.04] p-3">
      <LinkIcon className="h-3.5 w-3.5 text-indigo-400/70 shrink-0" />
      <div className="min-w-0 flex-1">
        <div className="text-xs font-mono text-white/70 truncate">{link.host}</div>
        <div className="text-[10px] text-white/40 truncate font-mono">{detail}</div>
      </div>
      <Badge tone={tone as any}>{text}</Badge>
    </div>
  );
}

// LinkHostGuide explains how to point a short-link subdomain at this app.
export function LinkHostGuide({ apex }: { apex: string }) {
  const { t } = useTranslation();
  return (
    <Guide title={t("domains.guideTitle")}>
      <p>{t("domains.guideEachHost")}<Code>{`go.${apex}`}</Code>) {t("domains.guideIntro")}</p>
      <ul className="list-disc pl-4 space-y-1">
        <li><b>CNAME</b> {t("domains.guideCnameTo")} <Code>{apex}</Code> {t("domains.guideCnameRec")}</li>
        <li>{t("domains.guideOrAdd")} <b>A / AAAA</b> {t("domains.guideAaaaRec")}</li>
        <li>{t("domains.guideProxiedIntro")} <b>{t("domains.guideProxiedWord")}</b> {t("domains.guideProxiedMid")} <b>{t("domains.guideUnverifiedWord")}</b> {t("domains.guideProxiedEnd")}</li>
      </ul>
      <p className="text-white/40">{t("domains.guideTipIntro")} <b>{t("domains.guideSubdomainPreset")}</b> {t("domains.guideTipEnd")}</p>
    </Guide>
  );
}

