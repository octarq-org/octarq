// @vitest-environment happy-dom
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { cleanup, fireEvent, render, screen, waitFor, within } from "@testing-library/react";
import { MemoryRouter } from "react-router-dom";
import { I18nProvider } from "../../../i18n";
import { api } from "../../../api";
import type { HelpCategory, HelpDocMeta } from "../../../api";
import HelpHub from "./HelpHub";

vi.mock("../../../api", () => ({
  api: {
    helpIndex: vi.fn(),
    helpCategories: vi.fn(),
    helpSearch: vi.fn(),
  },
}));

const categories: HelpCategory[] = [
  { key: "start", order: 1, icon: "rocket", labels: { en: "Get started" } },
  { key: "operations", order: 2, icon: "activity", labels: { en: "Operations" } },
  { key: "infrastructure", order: 3, icon: "server", labels: { en: "Infrastructure" } },
  { key: "commerce", order: 4, icon: "credit-card", labels: { en: "Commerce" } },
  { key: "settings", order: 5, icon: "settings", labels: { en: "Settings" } },
  { key: "licensing", order: 6, icon: "key", labels: { en: "Licensing" } },
];

const docs: HelpDocMeta[] = [
  { slug: "welcome", title: "Welcome", description: "What this workspace does.", category: "start", order: 2 },
  { slug: "intro", title: "Introduction", description: "Your first steps.", category: "start", order: 1 },
  { slug: "advanced", title: "Advanced setup", description: "Tune the instance.", category: "start", order: 4 },
  { slug: "runs", title: "Run history", description: "Read past runs.", category: "start", order: 3 },
  { slug: "backups", title: "Backups", description: "Protect your data.", category: "infrastructure", order: 1 },
];

function renderHub() {
  return render(
    <MemoryRouter>
      <I18nProvider>
        <HelpHub />
      </I18nProvider>
    </MemoryRouter>,
  );
}

function linkIds(container: HTMLElement) {
  return within(container)
    .getAllByRole("link")
    .map((link) => link.getAttribute("data-testid"));
}

beforeEach(() => {
  localStorage.setItem("lang", "en");
  vi.mocked(api.helpIndex).mockReset().mockResolvedValue(docs);
  vi.mocked(api.helpCategories).mockReset().mockResolvedValue(categories);
  vi.mocked(api.helpSearch).mockReset().mockResolvedValue({ query: "", lang: "en", results: [] });
});

afterEach(() => {
  cleanup();
  localStorage.clear();
});

describe("HelpHub", () => {
  it("groups docs under their category in order and hides categories with no docs", async () => {
    renderHub();

    const start = await screen.findByTestId("help-category-start");
    expect(linkIds(start)).toEqual([
      "help-doc-intro",
      "help-doc-welcome",
      "help-doc-runs",
      "help-doc-advanced",
    ]);

    const infrastructure = screen.getByTestId("help-category-infrastructure");
    expect(linkIds(infrastructure)).toEqual(["help-doc-backups"]);

    expect(screen.queryByTestId("help-category-commerce")).toBeNull();
    expect(screen.queryByTestId("help-category-settings")).toBeNull();
    expect(screen.queryByTestId("help-category-licensing")).toBeNull();
  });

  it("shows the first three start docs in the quickstart strip, ordered by doc order", async () => {
    renderHub();

    const quickstart = await screen.findByTestId("help-quickstart");
    expect(linkIds(quickstart)).toEqual(["help-doc-intro", "help-doc-welcome", "help-doc-runs"]);
  });

  it("shows an empty state when the edition ships no docs", async () => {
    vi.mocked(api.helpIndex).mockResolvedValue([]);
    renderHub();

    expect(await screen.findByText("No documentation is installed")).not.toBeNull();
    expect(screen.queryByTestId("help-quickstart")).toBeNull();
    expect(screen.queryByRole("combobox")).toBeNull();
  });

  it("recovers from a load failure when the user retries", async () => {
    vi.mocked(api.helpIndex).mockRejectedValueOnce(new Error("offline")).mockResolvedValue(docs);
    renderHub();

    expect(await screen.findByText("Could not load the documentation")).not.toBeNull();
    fireEvent.click(screen.getByRole("button", { name: "Retry" }));

    await waitFor(() => {
      expect(screen.queryByTestId("help-category-start")).not.toBeNull();
    });
  });
});

describe("HelpHub quickstart dedupe", () => {
  it("does not repeat the start category when Quickstart already shows every doc", async () => {
    vi.mocked(api.helpIndex).mockResolvedValue([
      { slug: "intro", title: "Introduction", description: "First steps.", category: "start", order: 1 },
      { slug: "advanced", title: "Advanced setup", description: "Tune it.", category: "start", order: 2 },
      { slug: "backups", title: "Backups", description: "Protect data.", category: "infrastructure", order: 1 },
    ]);
    renderHub();

    await screen.findByTestId("help-quickstart");
    expect(screen.getByTestId("help-quickstart").textContent).toContain("Introduction");
    expect(screen.getByTestId("help-quickstart").textContent).toContain("Advanced setup");
    expect(screen.queryByTestId("help-category-start")).toBeNull();
    expect(screen.getByTestId("help-category-infrastructure")).toBeTruthy();
  });
});
