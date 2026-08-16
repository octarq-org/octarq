// @vitest-environment happy-dom
import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen, waitFor } from "@testing-library/react";
import { MemoryRouter } from "react-router-dom";
import { I18nProvider } from "../../../i18n";

// See links/pages/emptyState.test.tsx — same contract: the page's empty state
// must render through the shared Empty primitive (tagged with a testid here),
// never a handwritten copy.
vi.mock("../../../ui", async (importOriginal) => {
  const ui = await importOriginal<typeof import("../../../ui")>();
  return {
    ...ui,
    Empty: (props: { children?: React.ReactNode; reason?: React.ReactNode; detail?: React.ReactNode; action?: React.ReactNode }) => (
      <div data-testid="shared-empty">
        {props.children}
        {props.reason}
        {props.detail}
        {props.action}
      </div>
    ),
  };
});

vi.mock("../../../api", async (importOriginal) => {
  const api = await importOriginal<typeof import("../../../api")>();
  return {
    ...api,
    api: {
      ...api.api,
      domains: vi.fn().mockResolvedValue([]),
      providerAccounts: vi.fn().mockResolvedValue([]),
    },
  };
});

vi.mock("../api", async (importOriginal) => {
  const dnsApi = await importOriginal<typeof import("../api")>();
  return {
    ...dnsApi,
    dnsApi: {
      ...dnsApi.dnsApi,
    },
  };
});

import DomainsPage from "./index";

beforeEach(() => {
  vi.clearAllMocks();
});

describe("DomainsPage empty state", () => {
  it("renders exactly one shared Empty (the right-column guide) with no data", async () => {
    render(
      <MemoryRouter initialEntries={["/domains"]}>
        <I18nProvider>
          <DomainsPage />
        </I18nProvider>
      </MemoryRouter>
    );

    await waitFor(() => {
      // One canonical empty state, not a left-column leftover + right-column
      // guide pair (the regression this guards against).
      expect(screen.getAllByTestId("shared-empty").length).toBe(1);
      // The stale left-column plain-text empty is gone with it.
      expect(screen.queryByText(/add your first domain on the right/i)).toBeNull();
    });
  });
});
