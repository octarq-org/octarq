// @vitest-environment happy-dom
import { describe, it, expect, vi, beforeEach, afterEach } from "vitest";
import { cleanup, render, screen, fireEvent, waitFor } from "@testing-library/react";
import { MemoryRouter } from "react-router-dom";
import { I18nProvider } from "../../../i18n";
import { EmailBlueprintPanel } from "./EmailBlueprintPanel";
import * as apiModule from "../api";
import * as roleModule from "../../../shell/role";

// --- mocks -----------------------------------------------------------------
vi.mock("../api", async (importOriginal) => {
  const actual = await importOriginal<typeof apiModule>();
  return {
    ...actual,
    dnsApi: {
      ...actual.dnsApi,
      emailBlueprint: vi.fn(),
      applyEmailBlueprint: vi.fn(),
    },
  };
});

vi.mock("../../../shell/role", async (importOriginal) => {
  const actual = await importOriginal<typeof roleModule>();
  return {
    ...actual,
    useCurrentRole: vi.fn(() => ({ role: "admin", isInstanceAdmin: false })),
    roleSatisfies: vi.fn((min: string, role: string) => role === "admin" || min === role),
  };
});

beforeEach(() => {
  vi.mocked(roleModule.useCurrentRole).mockReturnValue({ role: "admin", isInstanceAdmin: false } as any);
  vi.mocked(roleModule.roleSatisfies).mockReturnValue(true);
});

afterEach(() => {
  cleanup();
  vi.clearAllMocks();
});

const noop = () => {};

const missingRecords = [
  { type: "MX", name: "@", content: "route1.mx.cloudflare.net", ttl: 1, priority: 10, status: "missing" as const },
  { type: "MX", name: "@", content: "route2.mx.cloudflare.net", ttl: 1, priority: 53, status: "missing" as const },
  { type: "TXT", name: "@", content: "v=spf1 include:_spf.mx.cloudflare.net ~all", ttl: 1, status: "missing" as const },
  { type: "TXT", name: "_dmarc", content: "v=DMARC1; p=none; sp=none;", ttl: 1, status: "missing" as const },
];

const okRecords = missingRecords.map((r) => ({ ...r, status: "ok" as const }));

function wrapper({ children }: { children: React.ReactNode }) {
  return (
    <MemoryRouter>
      <I18nProvider>{children}</I18nProvider>
    </MemoryRouter>
  );
}

// --- tests -----------------------------------------------------------------
describe("EmailBlueprintPanel", () => {
  it("renders blueprint records after loading", async () => {
    vi.mocked(apiModule.dnsApi.emailBlueprint).mockResolvedValue(missingRecords);
    render(<EmailBlueprintPanel domainId={1} hasProvider onClose={noop} onApplied={noop} />, { wrapper });
    await waitFor(() => expect(screen.queryByText(/emailBlueprintLoading/)).toBeNull());
    // Each missing record should show a status badge
    const missing = screen.getAllByText(/blueprintStatus_missing/);
    expect(missing.length).toBeGreaterThanOrEqual(4);
  });

  it("shows apply button when admin + hasProvider + records missing", async () => {
    vi.mocked(apiModule.dnsApi.emailBlueprint).mockResolvedValue(missingRecords);
    render(<EmailBlueprintPanel domainId={1} hasProvider onClose={noop} onApplied={noop} />, { wrapper });
    await waitFor(() => expect(apiModule.dnsApi.emailBlueprint).toHaveBeenCalledWith(1));
    expect(screen.getByText(/emailBlueprintApply/)).toBeTruthy();
  });

  it("hides apply button when hasProvider=false", async () => {
    vi.mocked(apiModule.dnsApi.emailBlueprint).mockResolvedValue(missingRecords);
    render(<EmailBlueprintPanel domainId={1} hasProvider={false} onClose={noop} onApplied={noop} />, { wrapper });
    await waitFor(() => expect(apiModule.dnsApi.emailBlueprint).toHaveBeenCalledWith(1));
    expect(screen.queryByText(/emailBlueprintApply/)).toBeNull();
  });

  it("hides apply button when user is not admin", async () => {
    vi.mocked(roleModule.useCurrentRole).mockReturnValue({ role: "member", isInstanceAdmin: false } as any);
    vi.mocked(roleModule.roleSatisfies).mockReturnValue(false);
    vi.mocked(apiModule.dnsApi.emailBlueprint).mockResolvedValue(missingRecords);
    render(<EmailBlueprintPanel domainId={1} hasProvider onClose={noop} onApplied={noop} />, { wrapper });
    await waitFor(() => expect(apiModule.dnsApi.emailBlueprint).toHaveBeenCalledWith(1));
    expect(screen.queryByText(/emailBlueprintApply/)).toBeNull();
  });

  it("calls applyEmailBlueprint on apply click and shows result", async () => {
    vi.mocked(apiModule.dnsApi.emailBlueprint)
      .mockResolvedValueOnce(missingRecords)
      .mockResolvedValueOnce(okRecords);
    vi.mocked(apiModule.dnsApi.applyEmailBlueprint).mockResolvedValue({ ok: true, applied: 4, skipped: 0 });
    const onApplied = vi.fn();
    render(<EmailBlueprintPanel domainId={1} hasProvider onClose={noop} onApplied={onApplied} />, { wrapper });
    await waitFor(() => screen.getByText(/emailBlueprintApply/));
    fireEvent.click(screen.getByText(/emailBlueprintApply/));
    await waitFor(() => expect(apiModule.dnsApi.applyEmailBlueprint).toHaveBeenCalledWith(1));
    await waitFor(() => expect(onApplied).toHaveBeenCalled());
    await waitFor(() => expect(screen.getByText(/emailBlueprintApplyResult/)).toBeTruthy());
  });

  it("shows allOk alert when all records are ok", async () => {
    vi.mocked(apiModule.dnsApi.emailBlueprint).mockResolvedValue(okRecords);
    render(<EmailBlueprintPanel domainId={1} hasProvider onClose={noop} onApplied={noop} />, { wrapper });
    await waitFor(() => expect(screen.getByText(/emailBlueprintAllOk/)).toBeTruthy());
  });

  it("calls onClose when cancel button clicked", async () => {
    vi.mocked(apiModule.dnsApi.emailBlueprint).mockResolvedValue(okRecords);
    const onClose = vi.fn();
    render(<EmailBlueprintPanel domainId={1} hasProvider onClose={onClose} onApplied={noop} />, { wrapper });
    await waitFor(() => expect(apiModule.dnsApi.emailBlueprint).toHaveBeenCalled());
    fireEvent.click(screen.getByText(/domains\.cancel/));
    expect(onClose).toHaveBeenCalled();
  });
});
