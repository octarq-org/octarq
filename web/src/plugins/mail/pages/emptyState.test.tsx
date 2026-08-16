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
    },
  };
});

vi.mock("../api", async (importOriginal) => {
  const mailApi = await importOriginal<typeof import("../api")>();
  return {
    ...mailApi,
    mailApi: {
      ...mailApi.mailApi,
      mailboxes: vi.fn().mockResolvedValue([]),
      emails: vi.fn().mockResolvedValue([]),
    },
  };
});

import MailPage from "./index";

beforeEach(() => {
  vi.clearAllMocks();
});

describe("MailPage empty state", () => {
  it("renders the shared Empty when there are no mailboxes (not a handwritten copy)", async () => {
    render(
      <MemoryRouter initialEntries={["/mail"]}>
        <I18nProvider>
          <MailPage />
        </I18nProvider>
      </MemoryRouter>
    );

    await waitFor(() => {
      expect(screen.getAllByTestId("shared-empty").length).toBeGreaterThan(0);
    });
  });
});
