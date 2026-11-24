// @vitest-environment jsdom
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { I18nProvider } from "../../../i18n";
import { HelpDocToc, HelpDocTocMobile, type TocItem } from "./HelpDocToc";

const ITEMS: TocItem[] = [
  { id: "install", text: "Install", level: 2 },
  { id: "configure", text: "Configure", level: 3 },
];

class IntersectionObserverStub {
  observe() {}
  unobserve() {}
  disconnect() {}
}

beforeEach(() => {
  vi.stubGlobal("IntersectionObserver", IntersectionObserverStub);
  Object.defineProperty(window, "matchMedia", {
    configurable: true,
    value: vi.fn().mockReturnValue({ matches: false }),
  });
  Element.prototype.scrollIntoView = vi.fn();
  localStorage.setItem("lang", "en");
  const heading = document.createElement("h2");
  heading.id = "install";
  heading.textContent = "Install";
  document.body.appendChild(heading);
});

afterEach(() => {
  cleanup();
  document.body.innerHTML = "";
  vi.unstubAllGlobals();
  vi.restoreAllMocks();
});

describe("HelpDocToc", () => {
  it("renders the desktop table of contents with section links", () => {
    render(
      <I18nProvider>
        <HelpDocToc items={ITEMS} />
      </I18nProvider>,
    );
    expect(screen.getByText("On this page")).toBeTruthy();
    expect(screen.getAllByRole("link").map((link) => link.textContent)).toEqual([
      "Install",
      "Configure",
    ]);
  });

  it("renders nothing without headings", () => {
    const { container } = render(
      <I18nProvider>
        <HelpDocToc items={[]} />
      </I18nProvider>,
    );
    expect(container.firstChild).toBeNull();
  });
});

describe("HelpDocTocMobile", () => {
  it("opens the table of contents dialog from the sticky pill", async () => {
    render(
      <I18nProvider>
        <HelpDocTocMobile items={ITEMS} />
      </I18nProvider>,
    );

    const pill = screen.getByRole("button", { name: "On this page" });
    expect(pill.getAttribute("aria-haspopup")).toBe("dialog");
    expect(screen.queryByRole("dialog")).toBeNull();

    fireEvent.click(pill);
    const dialog = await screen.findByRole("dialog");
    expect(dialog.textContent).toContain("Configure");
  });

  it("closes the dialog and scrolls to the selected section", async () => {
    render(
      <I18nProvider>
        <HelpDocTocMobile items={ITEMS} />
      </I18nProvider>,
    );

    fireEvent.click(screen.getByRole("button", { name: "On this page" }));
    const dialog = await screen.findByRole("dialog");
    fireEvent.click(screen.getByRole("link", { name: "Install" }));

    expect(Element.prototype.scrollIntoView).toHaveBeenCalled();
    await waitFor(() => {
      expect(dialog.isConnected).toBe(false);
    });
  });
});
