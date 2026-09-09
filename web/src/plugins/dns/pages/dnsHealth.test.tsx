// @vitest-environment happy-dom
import { describe, it, expect, vi, beforeEach, afterEach } from "vitest";
import { cleanup, render, screen, waitFor, fireEvent } from "@testing-library/react";
import { MemoryRouter } from "react-router-dom";
import { I18nProvider } from "../../../i18n";
import { RoleProvider } from "../../../shell/role";
import {
  calculateReputationScore,
  DnsReputationScoreCard,
  DnsFixAlertBanner,
  DnsTroubleshootingGuide,
} from "./dnsStatus";
import DomainsPage from "./index";
import * as apiModule from "../api";

function jsonResponse(body: unknown, status = 200) {
  return {
    ok: status >= 200 && status < 300,
    status,
    json: async () => body,
    headers: { get: (name: string) => (name === "content-type" ? "application/json" : null) },
  };
}

// --- mocks -----------------------------------------------------------------
vi.mock("../../../shell/role", () => ({
  useCurrentRole: () => ({ role: "admin", isInstanceAdmin: false }),
  roleSatisfies: () => true,
  RoleProvider: ({ children }: any) => children,
}));

vi.mock("../api", async (importOriginal) => {
  const actual = await importOriginal<typeof apiModule>();
  return {
    ...actual,
    dnsApi: {
      ...actual.dnsApi,
      verifyDNS: vi.fn(),
      applyEmailBlueprint: vi.fn(),
      emailBlueprint: vi.fn(),
      records: vi.fn().mockResolvedValue([]),
    },
  };
});

const mockDomain = {
  id: 10,
  name: "example.com",
  providerAccountId: 1,
  zoneId: "zone_123",
  forMail: true,
  forLink: false,
  linkHosts: [],
  mailHosts: [{ host: "example.com", enabled: true }],
  createdAt: "2026-01-01T00:00:00Z",
};

const mockProviderAccounts = [
  { id: 1, name: "Cloudflare", type: "cloudflare", config: {}, hasCredentials: true, createdAt: "", updatedAt: "" },
];

let originalFetch: typeof globalThis.fetch;

beforeEach(() => {
  originalFetch = globalThis.fetch;
  globalThis.fetch = vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
    const rawUrl = typeof input === "string" ? input : input instanceof URL ? input.toString() : (input as Request).url;
    const urlObj = new URL(rawUrl, "http://localhost");
    const path = urlObj.pathname;

    if (path === "/api/domains") {
      return jsonResponse([mockDomain]);
    }
    if (path === "/api/provider-accounts") {
      return jsonResponse(mockProviderAccounts);
    }
    return jsonResponse({});
  }) as any;
});

afterEach(() => {
  cleanup();
  vi.clearAllMocks();
  globalThis.fetch = originalFetch;
});

function wrapper({ children }: { children: React.ReactNode }) {
  return (
    <MemoryRouter>
      <RoleProvider value={{ role: "admin", isInstanceAdmin: false }}>
        <I18nProvider>{children}</I18nProvider>
      </RoleProvider>
    </MemoryRouter>
  );
}

// Mock DNS status payloads
const allHealthyStatus: apiModule.DNSVerifyResult = {
  spf: { set: true, healthy: true, value: "v=spf1 include:_spf.mx.cloudflare.net ~all" },
  dkim: { set: true, healthy: true, selector: "default", value: "v=DKIM1; k=rsa; p=MIGfMA0GCSqGSIb3DQEBAQUAA4GNADCBiQKBgQC..." },
  dmarc: { set: true, healthy: true, value: "v=DMARC1; p=reject; sp=reject;" },
  hosts: [
    {
      host: "example.com",
      spf: { set: true, healthy: true, value: "v=spf1 include:_spf.mx.cloudflare.net ~all" },
      dkim: { set: true, healthy: true, selector: "default", value: "v=DKIM1; k=rsa; p=..." },
      dmarc: { set: true, healthy: true, value: "v=DMARC1; p=reject; sp=reject;" },
    },
  ],
  links: [
    { host: "go.example.com", set: true, healthy: true, cname: "example.com", target: "example.com" },
  ],
};

const issuesStatus: apiModule.DNSVerifyResult = {
  spf: { set: true, healthy: false, value: "v=spf1 -all" },
  dkim: { set: false, healthy: false },
  dmarc: { set: true, healthy: true, value: "v=DMARC1; p=none;" },
  hosts: [
    {
      host: "example.com",
      spf: { set: true, healthy: false, value: "v=spf1 -all" },
      dkim: { set: false, healthy: false },
      dmarc: { set: true, healthy: true, value: "v=DMARC1; p=none;" },
    },
  ],
  links: [],
};

const allMissingStatus: apiModule.DNSVerifyResult = {
  spf: { set: false, healthy: false },
  dkim: { set: false, healthy: false },
  dmarc: { set: false, healthy: false },
  hosts: [
    {
      host: "example.com",
      spf: { set: false, healthy: false },
      dkim: { set: false, healthy: false },
      dmarc: { set: false, healthy: false },
    },
  ],
  links: [],
};

describe("DNS Health & Deliverability Reputation", () => {
  describe("calculateReputationScore", () => {
    it("returns null if status is null", () => {
      expect(calculateReputationScore(null)).toBeNull();
    });

    it("calculates 100 score and excellent grade when all records are healthy", () => {
      const result = calculateReputationScore(allHealthyStatus, "example.com");
      expect(result).not.toBeNull();
      expect(result!.score).toBe(100);
      expect(result!.grade).toBe("excellent");
      expect(result!.tone).toBe("green");
      expect(result!.passedCount).toBe(3);
      expect(result!.warningCount).toBe(0);
      expect(result!.missingCount).toBe(0);
      expect(result!.hasIssues).toBe(false);
      expect(result!.issues.length).toBe(0);
    });

    it("calculates partial score when SPF is misconfigured and DKIM is missing", () => {
      const result = calculateReputationScore(issuesStatus, "example.com");
      expect(result).not.toBeNull();
      // SPF (15) + DKIM (0) + DMARC (30) = 45
      expect(result!.score).toBe(45);
      expect(result!.grade).toBe("fair");
      expect(result!.tone).toBe("amber");
      expect(result!.passedCount).toBe(1);
      expect(result!.warningCount).toBe(1);
      expect(result!.missingCount).toBe(1);
      expect(result!.hasIssues).toBe(true);
      expect(result!.issues.length).toBe(2);

      const spfIssue = result!.issues.find((i) => i.protocol === "SPF");
      expect(spfIssue).toBeDefined();
      expect(spfIssue!.issueType).toBe("misconfigured");
      expect(spfIssue!.observedValue).toBe("v=spf1 -all");

      const dkimIssue = result!.issues.find((i) => i.protocol === "DKIM");
      expect(dkimIssue).toBeDefined();
      expect(dkimIssue!.issueType).toBe("missing");
    });

    it("calculates 0 score and poor grade when all records are missing", () => {
      const result = calculateReputationScore(allMissingStatus, "example.com");
      expect(result).not.toBeNull();
      expect(result!.score).toBe(0);
      expect(result!.grade).toBe("poor");
      expect(result!.tone).toBe("red");
      expect(result!.missingCount).toBe(3);
      expect(result!.hasIssues).toBe(true);
      expect(result!.issues.length).toBe(3);
    });

    it("calculates average score across multiple mail hosts", () => {
      const multiHostStatus: apiModule.DNSVerifyResult = {
        ...allHealthyStatus,
        hosts: [
          allHealthyStatus.hosts[0], // 100 pts
          issuesStatus.hosts[0],     // 45 pts
        ],
      };
      const result = calculateReputationScore(multiHostStatus, "example.com");
      expect(result).not.toBeNull();
      // (100 + 45) / 2 = 72.5 -> 73 -> good
      expect(result!.score).toBe(73);
      expect(result!.grade).toBe("good");
      expect(result!.tone).toBe("cyan");
      expect(result!.totalChecks).toBe(6);
    });
  });

  describe("DnsReputationScoreCard", () => {
    it("renders score and grade badge", () => {
      const score = calculateReputationScore(allHealthyStatus, "example.com")!;
      render(<DnsReputationScoreCard repScore={score} />, { wrapper });

      expect(screen.getByText("100")).toBeTruthy();
      expect(screen.getByText("/ 100")).toBeTruthy();
      expect(screen.getByText(/domains.dnsHealthScore/)).toBeTruthy();
    });
  });

  describe("DnsFixAlertBanner", () => {
    it("renders success alert when all records are healthy", () => {
      const score = calculateReputationScore(allHealthyStatus, "example.com")!;
      render(
        <DnsFixAlertBanner
          repScore={score}
          hasProvider={true}
          fixing={false}
          onOneClickFix={vi.fn()}
          onOpenDetails={vi.fn()}
        />,
        { wrapper }
      );

      expect(screen.getByText(/domains.allRecordsHealthy/)).toBeTruthy();
      expect(screen.queryByText(/domains.oneClickAutoFix/)).toBeNull();
    });

    it("renders one-click fix action button when provider is connected and issues exist", () => {
      const score = calculateReputationScore(issuesStatus, "example.com")!;
      const onFix = vi.fn();
      const onOpen = vi.fn();
      render(
        <DnsFixAlertBanner
          repScore={score}
          hasProvider={true}
          fixing={false}
          onOneClickFix={onFix}
          onOpenDetails={onOpen}
        />,
        { wrapper }
      );

      expect(screen.getByText(/domains.autoFixBannerTitle/)).toBeTruthy();
      const fixBtn = screen.getByText(/domains.oneClickAutoFix/);
      expect(fixBtn).toBeTruthy();
      fireEvent.click(fixBtn);
      expect(onFix).toHaveBeenCalledTimes(1);

      const detailsBtn = screen.getByText(/domains.viewDetails/);
      fireEvent.click(detailsBtn);
      expect(onOpen).toHaveBeenCalledTimes(1);
    });

    it("hides auto-fix button and displays manual hint when no provider is connected", () => {
      const score = calculateReputationScore(issuesStatus, "example.com")!;
      render(
        <DnsFixAlertBanner
          repScore={score}
          hasProvider={false}
          fixing={false}
          onOneClickFix={vi.fn()}
          onOpenDetails={vi.fn()}
        />,
        { wrapper }
      );

      expect(screen.queryByText(/domains.oneClickAutoFix/)).toBeNull();
      expect(screen.getByText(/domains.manualFixNote/)).toBeTruthy();
      expect(screen.getByText(/domains.viewDetails/)).toBeTruthy();
    });
  });

  describe("DnsTroubleshootingGuide", () => {
    it("renders diagnostics for each missing or misconfigured record", () => {
      const score = calculateReputationScore(issuesStatus, "example.com")!;
      render(<DnsTroubleshootingGuide issues={score.issues} />, { wrapper });

      expect(screen.getByText(/domains.troubleshootingTitle/)).toBeTruthy();
      expect(screen.getByText("v=spf1 -all")).toBeTruthy();
      expect(screen.getByText(/_domainkey/)).toBeTruthy();
    });
  });

  describe("DomainsPage verification integration", () => {
    it("automatically fetches DNS verification and allows one-click auto-fix", async () => {
      vi.mocked(apiModule.dnsApi.verifyDNS)
        .mockResolvedValueOnce(issuesStatus)
        .mockResolvedValueOnce(allHealthyStatus);
      vi.mocked(apiModule.dnsApi.applyEmailBlueprint).mockResolvedValue({ ok: true, applied: 2, skipped: 0 });

      render(<DomainsPage />, { wrapper });

      // Wait for domain list to load
      await waitFor(() => expect(screen.getByText("example.com")).toBeTruthy());

      // Click the domain health button on the domain row
      const healthButtons = screen.getAllByTitle(/domains.domainHealth/);
      fireEvent.click(healthButtons[0]);

      // Should have triggered verifyDNS for domain 10
      await waitFor(() => expect(apiModule.dnsApi.verifyDNS).toHaveBeenCalledWith(10));

      // Should show the health score for issuesStatus (score: 45)
      await waitFor(() => expect(screen.getByText("45")).toBeTruthy());
      expect(screen.getByText(/domains.autoFixBannerTitle/)).toBeTruthy();

      // Click one-click auto-fix button
      const autoFixBtn = screen.getByText(/domains.oneClickAutoFix/);
      fireEvent.click(autoFixBtn);

      // Verify applyEmailBlueprint was called
      await waitFor(() => expect(apiModule.dnsApi.applyEmailBlueprint).toHaveBeenCalledWith(10));

      // Verify second call to verifyDNS was made
      await waitFor(() => expect(apiModule.dnsApi.verifyDNS).toHaveBeenCalledTimes(2));

      // Now should show updated score 100
      await waitFor(() => expect(screen.getByText("100")).toBeTruthy());
      expect(screen.getByText(/domains.allRecordsHealthy/)).toBeTruthy();
    });
  });
});
