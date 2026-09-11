import { useState } from "react";
import { DNSVerifyResult, HostDNSStatus, LinkHostStatus, DNSRecordStatus } from "../api";
import { Code, Guide, Badge, Button } from "../../../ui";
import { ShieldCheck, ShieldAlert, AlertTriangle, Zap, Copy, Check, ListChecks, Mail, Link as LinkIcon } from "lucide-react";
import { useTranslation } from "../../../i18n";

export interface DnsDiagnosticIssue {
  host: string;
  protocol: "SPF" | "DKIM" | "DMARC";
  issueType: "missing" | "misconfigured";
  recordType: "TXT";
  recordName: string;
  recommendedValue: string;
  observedValue?: string;
  selector?: string;
  rationaleKey: string;
  fixHintKey: string;
}

export interface DnsReputationScore {
  score: number;
  grade: "excellent" | "good" | "fair" | "poor";
  tone: "green" | "cyan" | "amber" | "red";
  passedCount: number;
  warningCount: number;
  missingCount: number;
  totalChecks: number;
  hasIssues: boolean;
  issues: DnsDiagnosticIssue[];
}

export function calculateReputationScore(
  status: DNSVerifyResult | null,
  apexDomain = ""
): DnsReputationScore | null {
  if (!status) return null;

  const mailHosts =
    status.hosts && status.hosts.length > 0
      ? status.hosts
      : [{ host: apexDomain || "apex", spf: status.spf, dkim: status.dkim, dmarc: status.dmarc }];

  let totalScoreSum = 0;
  let passedCount = 0;
  let warningCount = 0;
  let missingCount = 0;
  const issues: DnsDiagnosticIssue[] = [];

  for (const h of mailHosts) {
    let hostScore = 0;
    const isApex = !h.host || h.host === apexDomain;

    // SPF check (weight: 35)
    if (h.spf.healthy) {
      hostScore += 35;
      passedCount++;
    } else if (h.spf.set) {
      hostScore += 15;
      warningCount++;
      issues.push({
        host: h.host,
        protocol: "SPF",
        issueType: "misconfigured",
        recordType: "TXT",
        recordName: isApex ? "@" : h.host,
        recommendedValue: "v=spf1 include:_spf.mx.cloudflare.net ~all",
        observedValue: h.spf.value,
        rationaleKey: "domains.spfRationale",
        fixHintKey: "domains.spfFixHint",
      });
    } else {
      missingCount++;
      issues.push({
        host: h.host,
        protocol: "SPF",
        issueType: "missing",
        recordType: "TXT",
        recordName: isApex ? "@" : h.host,
        recommendedValue: "v=spf1 include:_spf.mx.cloudflare.net ~all",
        rationaleKey: "domains.spfRationale",
        fixHintKey: "domains.spfFixHint",
      });
    }

    // DKIM check (weight: 35)
    if (h.dkim.healthy) {
      hostScore += 35;
      passedCount++;
    } else if (h.dkim.set) {
      hostScore += 15;
      warningCount++;
      issues.push({
        host: h.host,
        protocol: "DKIM",
        issueType: "misconfigured",
        recordType: "TXT",
        recordName: `${h.dkim.selector || "default"}._domainkey`,
        recommendedValue: "v=DKIM1; k=rsa; p=...",
        observedValue: h.dkim.value,
        selector: h.dkim.selector,
        rationaleKey: "domains.dkimRationale",
        fixHintKey: "domains.dkimFixHint",
      });
    } else {
      missingCount++;
      issues.push({
        host: h.host,
        protocol: "DKIM",
        issueType: "missing",
        recordType: "TXT",
        recordName: `${h.dkim.selector || "default"}._domainkey`,
        recommendedValue: "v=DKIM1; k=rsa; p=...",
        selector: h.dkim.selector,
        rationaleKey: "domains.dkimRationale",
        fixHintKey: "domains.dkimFixHint",
      });
    }

    // DMARC check (weight: 30)
    if (h.dmarc.healthy) {
      hostScore += 30;
      passedCount++;
    } else if (h.dmarc.set) {
      hostScore += 10;
      warningCount++;
      issues.push({
        host: h.host,
        protocol: "DMARC",
        issueType: "misconfigured",
        recordType: "TXT",
        recordName: isApex ? "_dmarc" : `_dmarc.${h.host}`,
        recommendedValue: "v=DMARC1; p=none; sp=none;",
        observedValue: h.dmarc.value,
        rationaleKey: "domains.dmarcRationale",
        fixHintKey: "domains.dmarcFixHint",
      });
    } else {
      missingCount++;
      issues.push({
        host: h.host,
        protocol: "DMARC",
        issueType: "missing",
        recordType: "TXT",
        recordName: isApex ? "_dmarc" : `_dmarc.${h.host}`,
        recommendedValue: "v=DMARC1; p=none; sp=none;",
        rationaleKey: "domains.dmarcRationale",
        fixHintKey: "domains.dmarcFixHint",
      });
    }

    totalScoreSum += hostScore;
  }

  const score = Math.round(totalScoreSum / mailHosts.length);
  const grade: "excellent" | "good" | "fair" | "poor" =
    score === 100 ? "excellent" : score >= 70 ? "good" : score >= 40 ? "fair" : "poor";
  const tone: "green" | "cyan" | "amber" | "red" =
    grade === "excellent" ? "green" : grade === "good" ? "cyan" : grade === "fair" ? "amber" : "red";

  return {
    score,
    grade,
    tone,
    passedCount,
    warningCount,
    missingCount,
    totalChecks: mailHosts.length * 3,
    hasIssues: issues.length > 0,
    issues,
  };
}

function CopyButton({ text, label }: { text: string; label?: string }) {
  const { t } = useTranslation();
  const [copied, setCopied] = useState(false);
  function copy() {
    navigator.clipboard.writeText(text).then(() => {
      setCopied(true);
      setTimeout(() => setCopied(false), 1500);
    });
  }
  return (
    <button
      type="button"
      onClick={copy}
      className="inline-flex items-center gap-1 px-1.5 py-0.5 rounded text-xs hover:bg-foreground/10 text-foreground/60 hover:text-foreground transition-colors shrink-0 font-sans"
      title={t("domains.copy")}
    >
      {copied ? (
        <>
          <Check className="h-3 w-3 text-success-fg" />
          <span className="text-[10px] text-success-fg">{t("domains.copied")}</span>
        </>
      ) : (
        <>
          <Copy className="h-3 w-3" />
          {label && <span className="text-[10px]">{label}</span>}
        </>
      )}
    </button>
  );
}

export function DnsReputationScoreCard({ repScore }: { repScore: DnsReputationScore }) {
  const { t } = useTranslation();
  const gradeKey = `domains.scoreGrade_${repScore.grade}` as const;
  const descKey = `domains.scoreDesc_${repScore.grade}` as const;

  return (
    <div className="rounded-2xl bg-foreground/[0.02] border border-foreground/[0.06] p-5 space-y-4">
      <div className="flex flex-col sm:flex-row sm:items-center justify-between gap-4">
        <div className="flex items-start gap-4">
          <div className="flex flex-col items-center justify-center h-16 w-16 rounded-2xl bg-foreground/[0.03] border border-foreground/[0.06] shrink-0">
            <span className="text-2xl font-black font-mono tracking-tight text-foreground">
              {repScore.score}
            </span>
            <span className="text-[10px] font-mono text-foreground/40 font-medium">/ 100</span>
          </div>

          <div className="space-y-1">
            <div className="flex items-center gap-2 flex-wrap">
              <h4 className="text-sm font-bold text-foreground">{t("domains.dnsHealthScore")}</h4>
              <Badge tone={repScore.tone as any}>{t(gradeKey)}</Badge>
            </div>
            <p className="text-xs text-foreground/60 leading-relaxed max-w-xl">{t(descKey)}</p>
          </div>
        </div>

        <div className="flex items-center gap-2 shrink-0 self-start sm:self-center">
          <Badge tone="neutral" className="text-[11px] px-2.5 py-0.5">
            {t("domains.checksSummary", {
              passed: repScore.passedCount,
              warning: repScore.warningCount,
              missing: repScore.missingCount,
            })}
          </Badge>
        </div>
      </div>

      {/* Visual score progress bar */}
      <div className="w-full bg-foreground/[0.05] h-2 rounded-full overflow-hidden">
        <div
          className={`h-full rounded-full transition-all duration-500 ${
            repScore.tone === "green"
              ? "bg-success-fg"
              : repScore.tone === "cyan"
              ? "bg-info-fg"
              : repScore.tone === "amber"
              ? "bg-warning-fg"
              : "bg-danger-fg"
          }`}
          style={{ width: `${Math.max(repScore.score, 4)}%` }}
        />
      </div>
    </div>
  );
}

export function DnsFixAlertBanner({
  repScore,
  hasProvider,
  fixing,
  onOneClickFix,
  onOpenDetails,
}: {
  repScore: DnsReputationScore;
  hasProvider: boolean;
  fixing: boolean;
  onOneClickFix: () => void;
  onOpenDetails: () => void;
}) {
  const { t } = useTranslation();

  if (!repScore.hasIssues) {
    return (
      <div className="flex items-center gap-3 p-4 rounded-xl bg-success-bg/15 border border-success-border text-xs text-success-fg">
        <ShieldCheck className="h-5 w-5 shrink-0 text-success-fg" />
        <span className="font-medium">{t("domains.allRecordsHealthy")}</span>
      </div>
    );
  }

  return (
    <div className="p-4 rounded-2xl bg-warning-bg/10 border border-warning-border space-y-3">
      <div className="flex items-start gap-3">
        <AlertTriangle className="h-5 w-5 text-warning-fg shrink-0 mt-0.5" />
        <div className="flex-1 min-w-0">
          <h4 className="text-sm font-semibold text-foreground">{t("domains.autoFixBannerTitle")}</h4>
          <p className="text-xs text-foreground/60 mt-1 leading-relaxed">{t("domains.autoFixBannerDesc")}</p>
          <p className="text-[11px] text-foreground/45 mt-1">
            {hasProvider ? t("domains.providerAutoFixNote") : t("domains.manualFixNote")}
          </p>
        </div>
      </div>

      <div className="flex items-center gap-2.5 pt-1 pl-8 flex-wrap">
        {hasProvider && (
          <Button
            variant="primary"
            onClick={onOneClickFix}
            disabled={fixing}
            className="text-xs py-1.5 px-3 gap-1.5"
          >
            <Zap className={`h-3.5 w-3.5 ${fixing ? "animate-pulse" : ""}`} />
            {fixing ? t("domains.oneClickAutoFixing") : t("domains.oneClickAutoFix")}
          </Button>
        )}
        <Button variant="subtle" onClick={onOpenDetails} className="text-xs py-1.5 px-3 gap-1.5">
          <ListChecks className="h-3.5 w-3.5" />
          {t("domains.viewDetails")}
        </Button>
      </div>
    </div>
  );
}

export function DnsTroubleshootingGuide({ issues }: { issues: DnsDiagnosticIssue[] }) {
  const { t } = useTranslation();

  if (!issues || issues.length === 0) return null;

  return (
    <div className="space-y-4 pt-2">
      <div>
        <h4 className="text-sm font-semibold text-foreground flex items-center gap-2">
          <ShieldAlert className="h-4 w-4 text-warning-fg" />
          {t("domains.troubleshootingTitle")}
        </h4>
        <p className="text-xs text-foreground/50 mt-0.5">{t("domains.troubleshootingDesc")}</p>
      </div>

      <div className="space-y-3">
        {issues.map((issue, idx) => (
          <div
            key={`${issue.host}-${issue.protocol}-${idx}`}
            className="rounded-xl border border-foreground/[0.06] bg-foreground/[0.015] p-4 space-y-3"
          >
            <div className="flex items-center justify-between gap-2 flex-wrap">
              <div className="flex items-center gap-2">
                <Badge tone="neutral" className="font-mono text-xs font-bold">
                  {issue.protocol}
                </Badge>
                <span className="text-xs font-mono text-foreground/80 font-medium">{issue.host}</span>
              </div>
              <Badge tone={(issue.issueType === "misconfigured" ? "amber" : "red") as any}>
                {issue.issueType === "misconfigured" ? t("domains.misconfigured") : t("domains.missing")}
              </Badge>
            </div>

            <p className="text-xs text-foreground/65 leading-relaxed">{t(issue.rationaleKey as any)}</p>

            {issue.observedValue && (
              <div className="p-2.5 rounded-lg bg-foreground/[0.03] border border-foreground/[0.05] space-y-1">
                <span className="text-[10px] uppercase font-bold text-foreground/45 tracking-wider">
                  {t("domains.issueObserved")}
                </span>
                <div className="flex items-center justify-between gap-2">
                  <span className="font-mono text-xs text-foreground/60 break-all">{issue.observedValue}</span>
                  <CopyButton text={issue.observedValue} />
                </div>
              </div>
            )}

            <div className="p-3 rounded-xl bg-foreground/[0.03] border border-foreground/[0.06] space-y-2">
              <div className="flex items-center justify-between">
                <span className="text-[10px] uppercase font-bold text-foreground/45 tracking-wider">
                  {t("domains.issueRecommended")}
                </span>
                <span className="text-[11px] text-foreground/50">{t(issue.fixHintKey as any)}</span>
              </div>

              <div className="grid grid-cols-1 sm:grid-cols-3 gap-2 text-xs font-mono">
                <div className="p-2 rounded bg-foreground/[0.02] border border-foreground/[0.04]">
                  <span className="text-[10px] text-foreground/40 block font-sans">{t("domains.issueRecordType")}</span>
                  <span className="font-semibold text-foreground/80">{issue.recordType}</span>
                </div>
                <div className="p-2 rounded bg-foreground/[0.02] border border-foreground/[0.04]">
                  <span className="text-[10px] text-foreground/40 block font-sans">{t("domains.issueHost")}</span>
                  <div className="flex items-center justify-between gap-1">
                    <span className="truncate text-foreground/80">{issue.recordName}</span>
                    <CopyButton text={issue.recordName} />
                  </div>
                </div>
                <div className="p-2 rounded bg-foreground/[0.02] border border-foreground/[0.04] sm:col-span-1">
                  <span className="text-[10px] text-foreground/40 block font-sans">{t("domains.issueRecommended")}</span>
                  <div className="flex items-center justify-between gap-1">
                    <span className="truncate text-foreground/80">{issue.recommendedValue}</span>
                    <CopyButton text={issue.recommendedValue} />
                  </div>
                </div>
              </div>
            </div>
          </div>
        ))}
      </div>
    </div>
  );
}

function DnsStatusBadge({ status, label }: { status: DNSRecordStatus; label: string }) {
  const { t } = useTranslation();
  const tone = status.healthy ? "green" : status.set ? "amber" : "red";
  const text = status.healthy ? t("domains.configured") : status.set ? t("domains.misconfigured") : t("domains.missing");
  return (
    <div className="flex flex-col items-center p-3 rounded-xl bg-foreground/[0.02] border border-foreground/[0.04]">
      <span className="text-[10px] uppercase font-bold text-foreground/40 tracking-wider">{label}</span>
      <div className="mt-2"><Badge tone={tone as any}>{text}</Badge></div>
    </div>
  );
}

export function DnsHostRow({ host }: { host: HostDNSStatus }) {
  const dkimLabel = host.dkim.selector ? `DKIM (${host.dkim.selector})` : "DKIM";
  return (
    <div className="rounded-xl bg-foreground/[0.015] border border-foreground/[0.04] p-3">
      <div className="flex items-center gap-2 mb-2.5 px-1">
        <Mail className="h-3.5 w-3.5 text-accent-fg/70 shrink-0" />
        <span className="text-xs font-mono text-foreground/70 truncate">{host.host}</span>
      </div>
      <div className="grid grid-cols-1 sm:grid-cols-3 gap-2.5">
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
    <div className="flex items-center gap-3 rounded-xl bg-foreground/[0.015] border border-foreground/[0.04] p-3">
      <LinkIcon className="h-3.5 w-3.5 text-accent-fg/70 shrink-0" />
      <div className="min-w-0 flex-1">
        <div className="text-xs font-mono text-foreground/70 truncate">{link.host}</div>
        <div className="text-[10px] text-foreground/40 truncate font-mono">{detail}</div>
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
      <p className="text-foreground/40">{t("domains.guideTipIntro")} <b>{t("domains.guideSubdomainPreset")}</b> {t("domains.guideTipEnd")}</p>
    </Guide>
  );
}

