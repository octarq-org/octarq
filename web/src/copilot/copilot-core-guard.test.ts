import { readFileSync, readdirSync, statSync } from "node:fs";
import path from "node:path";
import { fileURLToPath } from "node:url";
import { describe, expect, it } from "vitest";

const copilotDir = path.dirname(fileURLToPath(import.meta.url));

const FORBIDDEN = [
  "LinkCard",
  "EmployeeCard",
  "OrderCard",
  "FinanceCard",
  "StatusCard",
  "PortalCard",
  "ResumeRankCard",
  "InvoiceAuditCard",
  "QuotationCard",
  "WeeklyReportCard",
  "ScriptStoryboardCard",
  "EmailManagementCard",
  "SupportEscalationCard",
  "FinancialAnalysisCard",
  "registerDomainCards",
  "DEFAULT_BRIEFING",
  "resume_rank",
  "invoice_audit",
  "financial_deck",
  "support_escalation",
  "weekly_report",
  "email_draft",
  "storyboard",
  "links_create",
  "crew_create",
  "orders_get",
  "finance_balance",
  "statuspage_public",
  "portal_licenses",
  "recruiting_screen",
  "quotation_generate",
  "support_escalate",
  "financial_analysis",
  "mail_draft_create",
  "email_manage",
  "links.create",
  "/api/links",
];

function sourceFiles(dir: string): string[] {
  const out: string[] = [];
  for (const entry of readdirSync(dir)) {
    const full = path.join(dir, entry);
    if (statSync(full).isDirectory()) {
      out.push(...sourceFiles(full));
      continue;
    }
    if (!/\.(ts|tsx)$/.test(entry)) continue;
    if (/\.(test|spec)\.tsx?$/.test(entry)) continue;
    out.push(full);
  }
  return out;
}

describe("copilot core guard", () => {
  it("contains no business card, canvas, or tool vocabulary", () => {
    const hits: string[] = [];

    for (const file of sourceFiles(copilotDir)) {
      const code = readFileSync(file, "utf8");
      for (const token of FORBIDDEN) {
        if (code.includes(token)) {
          hits.push(`${path.relative(copilotDir, file)}: ${token}`);
        }
      }
    }

    expect(hits).toEqual([]);
  });

  it("has no business card or canvas modules", () => {
    const files = sourceFiles(copilotDir).map((file) => path.relative(copilotDir, file));

    expect(files).not.toContain(path.join("cards", "LinkCard.tsx"));
    expect(files.some((file) => file.startsWith(`canvas${path.sep}editors`))).toBe(false);
  });
});
