// @vitest-environment happy-dom
import { describe, it, expect, vi, beforeEach } from "vitest";
import { cleanup, render, screen } from "@testing-library/react";
import { MemoryRouter } from "react-router-dom";
import { I18nProvider } from "../../../i18n";
import { registerUIPlugin, resetRegistry } from "@octarq/plugin-sdk";
import linksPlugin from "../index";
import TopLinksPanelWidget from "./TopLinksPanelWidget";
import * as apiModule from "../../../api";

describe("TopLinksPanelWidget title priority", () => {
  beforeEach(() => {
    cleanup();
    resetRegistry();
    registerUIPlugin(linksPlugin);
    vi.restoreAllMocks();
  });

  it("renders title as primary, slug as secondary, and tags as badges", () => {
    vi.spyOn(apiModule, "useOverviewData").mockReturnValue({
      topLinks: [
        {
          id: 1,
          slug: "promo-2026",
          host: "go.octarq.com",
          title: "Summer Promotion",
          tags: "marketing, summer, campaign",
          clicks: 120,
        },
        {
          id: 2,
          slug: "untitled-link",
          host: "",
          title: "",
          tags: "",
          clicks: 45,
        },
      ],
    } as any);

    render(
      <MemoryRouter>
        <I18nProvider>
          <TopLinksPanelWidget />
        </I18nProvider>
      </MemoryRouter>
    );

    // Link 1: title priority
    const titleEl = screen.getByText("Summer Promotion");
    expect(titleEl).toBeDefined();
    expect(titleEl.className).toContain("text-foreground");
    expect(titleEl.className).toContain("font-medium");

    // Link 1: secondary slug
    const secondarySlug = screen.getByText("go.octarq.com/promo-2026");
    expect(secondarySlug).toBeDefined();
    expect(secondarySlug.className).toContain("text-foreground/40");

    // Link 1: tags (first 2 badges visible + overflow badge "+1")
    expect(screen.getByTitle("marketing")).toBeDefined();
    expect(screen.getByTitle("summer")).toBeDefined();
    expect(screen.getByTitle("campaign")).toBeDefined(); // overflow title
    expect(screen.getByText("+1")).toBeDefined();

    // Link 2: fallback to slug when title is empty
    expect(screen.getByText("untitled-link")).toBeDefined();
    expect(screen.getByText("/untitled-link")).toBeDefined();
  });
});
