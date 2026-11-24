// @vitest-environment jsdom
import { act } from "react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { MemoryRouter } from "react-router-dom";
import { I18nProvider } from "../../../i18n";
import { HelpArticle } from "./HelpArticle";

// @ts-ignore
globalThis.IS_REACT_ACT_ENVIRONMENT = true;

const HTML = `
<h2 id="install">Install</h2>
<p>Run this:</p>
<pre><code class="language-bash">pnpm install foo</code></pre>
<blockquote>
<p>[!TIP]
Save the token.</p>
</blockquote>
<blockquote><p>Plain quote.</p></blockquote>
<table><thead><tr><th>Key</th></tr></thead><tbody><tr><td>Value</td></tr></tbody></table>
<p>See <a href="/help/other-slug#setup">the other page</a>, <a href="https://example.com/docs">external docs</a>, or <a href="#install">jump</a>.</p>
<script>window.__x = 1</script>
`;

function renderArticle(onNavigate = vi.fn()) {
  const result = render(
    <I18nProvider>
      <MemoryRouter initialEntries={["/help/services/current"]}>
        <HelpArticle
          title="Getting started"
          html={HTML}
          slug="current"
          lang="en"
          onNavigate={onNavigate}
          resolveDocPath={(slug) => (slug === "other-slug" ? "/help/guides/other-slug" : null)}
        />
      </MemoryRouter>
    </I18nProvider>,
  );
  return { ...result, onNavigate };
}

beforeEach(() => {
  Object.defineProperty(navigator, "clipboard", {
    configurable: true,
    value: { writeText: vi.fn().mockResolvedValue(undefined) },
  });
});

afterEach(() => {
  cleanup();
  vi.useRealTimers();
  vi.restoreAllMocks();
});

describe("HelpArticle component rendering", () => {
  it("renders the title as the only h1 of the article", () => {
    const { container } = renderArticle();
    const heading = screen.getByRole("heading", { level: 1, name: "Getting started" });
    expect(heading).toBeTruthy();
    expect(container.querySelectorAll("h1")).toHaveLength(1);
  });

  it("renders a code block with language label and localized copy control", () => {
    vi.useFakeTimers();
    renderArticle();
    expect(screen.getByText("BASH")).toBeTruthy();
    const copy = screen.getByRole("button", { name: "Copy" });
    fireEvent.click(copy);
    expect(navigator.clipboard.writeText).toHaveBeenCalledWith("pnpm install foo");
    expect(screen.getByRole("button", { name: "Copied!" })).toBeTruthy();
    act(() => {
      vi.advanceTimersByTime(2000);
    });
    expect(screen.getByRole("button", { name: "Copy" })).toBeTruthy();
  });

  it("turns GFM alerts into callouts and leaves plain quotes alone", () => {
    const { container } = renderArticle();
    expect(screen.getByText("TIP")).toBeTruthy();
    expect(screen.getByText("Save the token.").closest(".callout")).toBeTruthy();
    expect(screen.getByText("Plain quote.").closest(".callout")).toBeNull();
    expect(container.querySelectorAll(".callout")).toHaveLength(1);
  });

  it("wraps tables for horizontal scrolling", () => {
    const { container } = renderArticle();
    const table = container.querySelector("table");
    expect(table).toBeTruthy();
    expect(table?.closest(".table-wrapper")).toBeTruthy();
  });

  it("routes /help links through the app and leaves external links untouched", () => {
    const { onNavigate } = renderArticle();
    const internal = screen.getByRole("link", { name: "the other page" });
    expect(internal.getAttribute("href")).toBe("/help/guides/other-slug#setup");
    fireEvent.click(internal);
    expect(onNavigate).toHaveBeenCalledWith("/help/guides/other-slug#setup");

    const external = screen.getByRole("link", { name: "external docs" });
    expect(external.getAttribute("href")).toBe("https://example.com/docs");
    expect(onNavigate).toHaveBeenCalledTimes(1);
  });

  it("drops scripts from untrusted html", () => {
    const { container } = renderArticle();
    expect(container.querySelector("script")).toBeNull();
  });
});
