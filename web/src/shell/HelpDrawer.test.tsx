// @vitest-environment jsdom
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { MemoryRouter } from "react-router-dom";
import { I18nProvider } from "../i18n";
import { api, type HelpDocMeta } from "../api";
import { HelpDrawer } from "./HelpDrawer";
import { TopBar } from "./TopBar";
import {
  buildHelpQuery,
  clearHelpDocsCache,
  parseHelpDeepLink,
  useHelpDrawerStore,
} from "./useHelpDrawer";

const DOCS: HelpDocMeta[] = [
  { slug: "domains", title: "Domains", category: "services", order: 1, routes: ["/domains"] },
  { slug: "mail", title: "Mail Setup", category: "services", order: 2, routes: ["/settings/mail"] },
];

function resetDrawer() {
  useHelpDrawerStore.setState({
    isOpen: false,
    slug: null,
    anchor: null,
    docs: [],
    docsKey: null,
  });
  window.history.replaceState(null, "", "/");
}

function renderDrawer(path = "/domains") {
  return render(
    <I18nProvider>
      <MemoryRouter initialEntries={[path]}>
        <HelpDrawer activeOrgId={1} />
      </MemoryRouter>
    </I18nProvider>,
  );
}

beforeEach(() => {
  resetDrawer();
  clearHelpDocsCache();
  vi.spyOn(api, "helpIndex").mockResolvedValue(DOCS);
  vi.spyOn(api, "helpPage").mockImplementation(async (slug: string) => ({
    title: slug === "domains" ? "Domains" : "Mail Setup",
    html: '<p>Manage your domains here.</p><h2 id="records">Records</h2><p>Add a record.</p>',
  }));
});

afterEach(() => {
  cleanup();
  vi.restoreAllMocks();
});

describe("help deep link helpers", () => {
  it("parses help and anchor from a query string", () => {
    expect(parseHelpDeepLink("?help=domains&anchor=records")).toEqual({
      slug: "domains",
      anchor: "records",
    });
    expect(parseHelpDeepLink("?help=domains")).toEqual({ slug: "domains", anchor: null });
    expect(parseHelpDeepLink("?tab=2")).toBeNull();
  });

  it("adds and removes help params without dropping other params", () => {
    expect(buildHelpQuery("?tab=2", "domains", "records")).toBe(
      "?tab=2&help=domains&anchor=records",
    );
    expect(buildHelpQuery("?tab=2&help=domains&anchor=records", null, null)).toBe("?tab=2");
    expect(buildHelpQuery("", "domains", null)).toBe("?help=domains");
  });
});

describe("HelpDrawer", () => {
  it("opens from a deep link and renders the shared article", async () => {
    window.history.replaceState(null, "", "/domains?help=domains&anchor=records");

    renderDrawer();

    const dialog = await screen.findByRole("dialog");
    expect(dialog.getAttribute("aria-modal")).toBe("true");
    expect(await screen.findByText("Manage your domains here.")).not.toBeNull();
    expect(api.helpPage).toHaveBeenCalledWith("domains", "en");

    const heading = screen.getByRole("heading", { level: 1 });
    expect(heading.textContent).toBe("Domains");
    await waitFor(() => expect(document.activeElement).toBe(heading));
  });

  it("renders no category tree inside the drawer", async () => {
    window.history.replaceState(null, "", "/domains?help=domains");

    renderDrawer();

    await screen.findByText("Manage your domains here.");
    expect(screen.queryByText("Mail Setup")).toBeNull();
    expect(screen.queryByRole("navigation")).toBeNull();
    expect(screen.queryByText("Domains", { selector: "nav *" })).toBeNull();
  });

  it("closes on Escape and removes deep-link params", async () => {
    window.history.replaceState(null, "", "/domains?tab=2&help=domains&anchor=records");

    renderDrawer();
    await screen.findByRole("dialog");

    fireEvent.keyDown(window, { key: "Escape" });

    await waitFor(() => expect(screen.queryByRole("dialog")).toBeNull());
    expect(window.location.search).toBe("?tab=2");
  });

  it("uses full-screen width on mobile and a capped width on desktop", async () => {
    window.history.replaceState(null, "", "/domains?help=domains");

    renderDrawer();
    const dialog = await screen.findByRole("dialog");
    expect(dialog.className).toContain("w-full");
    expect(dialog.className).toContain("md:max-w-[560px]");
  });

  it("opens with the F1 shortcut when the page has docs", async () => {
    renderDrawer();

    await waitFor(() => expect(useHelpDrawerStore.getState().docs).toHaveLength(2));

    fireEvent.keyDown(window, { key: "F1" });

    await waitFor(() => expect(useHelpDrawerStore.getState().isOpen).toBe(true));
    expect(useHelpDrawerStore.getState().slug).toBe("domains");
  });

  it("does not hijack the question mark key while typing in an input", async () => {
    renderDrawer();

    await waitFor(() => expect(useHelpDrawerStore.getState().docs).toHaveLength(2));

    const input = document.createElement("input");
    document.body.appendChild(input);
    input.focus();

    fireEvent.keyDown(input, { key: "?" });

    expect(useHelpDrawerStore.getState().isOpen).toBe(false);
    input.remove();
  });

  it("does not open the drawer on the help pages themselves", async () => {
    window.history.replaceState(null, "", "/help/services/domains?help=domains");

    render(
      <I18nProvider>
        <MemoryRouter initialEntries={["/help/services/domains"]}>
          <HelpDrawer activeOrgId={1} />
        </MemoryRouter>
      </I18nProvider>,
    );

    await waitFor(() => expect(useHelpDrawerStore.getState().docs).toHaveLength(2));
    expect(useHelpDrawerStore.getState().isOpen).toBe(false);
    expect(screen.queryByRole("dialog")).toBeNull();
  });
});

describe("TopBar help entry", () => {
  const baseProps = {
    areas: [],
    panelCollapsed: false,
    onTogglePanel: vi.fn(),
    onOpenCommand: vi.fn(),
    actions: [],
    activeOrgId: 1,
  };

  it("shows the help trigger only when the current route maps to a doc", async () => {
    const first = render(
      <I18nProvider>
        <MemoryRouter initialEntries={["/domains"]}>
          <TopBar {...baseProps} />
        </MemoryRouter>
      </I18nProvider>,
    );

    expect(await screen.findByTestId("help-drawer-trigger")).not.toBeNull();
    first.unmount();

    render(
      <I18nProvider>
        <MemoryRouter initialEntries={["/unrelated"]}>
          <TopBar {...baseProps} />
        </MemoryRouter>
      </I18nProvider>,
    );

    await waitFor(() => expect(screen.queryByTestId("help-drawer-trigger")).toBeNull());
  });

  it("never shows the help trigger on help pages", async () => {
    render(
      <I18nProvider>
        <MemoryRouter initialEntries={["/help/services/domains"]}>
          <TopBar {...baseProps} />
        </MemoryRouter>
      </I18nProvider>,
    );

    await waitFor(() => expect(useHelpDrawerStore.getState().docs).toHaveLength(2));
    expect(screen.queryByTestId("help-drawer-trigger")).toBeNull();
  });
});
