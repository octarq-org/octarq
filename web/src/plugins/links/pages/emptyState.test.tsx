// @vitest-environment happy-dom
import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen, waitFor } from "@testing-library/react";
import { MemoryRouter } from "react-router-dom";
import { I18nProvider } from "../../../i18n";

// The page's empty state must render THROUGH the shared Empty primitive — not
// a handwritten copy. Mock Empty at the barrel the page imports from, tagging
// it with a testid; if the page ever regresses to a hand-rolled div, this
// marker disappears and the test goes red.
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
  const linksApi = await importOriginal<typeof import("../api")>();
  return {
    ...linksApi,
    linksApi: {
      ...linksApi.linksApi,
      links: vi.fn().mockResolvedValue([]),
    },
  };
});

import LinksPage from "./index";

beforeEach(() => {
  vi.clearAllMocks();
});

describe("LinksPage empty state", () => {
  it("renders the shared Empty when there are no links (not a handwritten copy)", async () => {
    render(
      <MemoryRouter initialEntries={["/links"]}>
        <I18nProvider>
          <LinksPage />
        </I18nProvider>
      </MemoryRouter>
    );

    await waitFor(() => {
      expect(screen.getAllByTestId("shared-empty").length).toBeGreaterThan(0);
    });
  });
});
