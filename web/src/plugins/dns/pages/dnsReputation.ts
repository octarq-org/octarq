import { DNSVerifyResult } from "../api";

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
