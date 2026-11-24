// @vitest-environment happy-dom
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { act, cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { MemoryRouter, Route, Routes } from "react-router-dom";
import { I18nProvider } from "../../../i18n";
import { api } from "../../../api";
import type { HelpCategory, HelpSearchHit, HelpSearchResponse } from "../../../api";
import HelpSearch from "./HelpSearch";

vi.mock("../../../api", () => ({
  api: {
    helpIndex: vi.fn(),
    helpCategories: vi.fn(),
    helpSearch: vi.fn(),
  },
}));

const categories: HelpCategory[] = [
  { key: "start", order: 1, icon: "rocket", labels: { en: "Get started" } },
  { key: "infrastructure", order: 3, icon: "server", labels: { en: "Infrastructure" } },
];

function hit(overrides: Partial<HelpSearchHit> = {}): HelpSearchHit {
  return {
    slug: "backups",
    title: "Backup result",
    category: "infrastructure",
    anchor: "backups",
    anchorTitle: "Backups",
    snippet: { text: "Back up your data.", offsets: [[0, 4]] },
    score: 1,
    url: "/help/infrastructure/backups",
    ...overrides,
  };
}

function response(results: HelpSearchHit[], query = "back"): HelpSearchResponse {
  return { query, lang: "en", results };
}

function deferred<T>() {
  let resolve!: (value: T) => void;
  const promise = new Promise<T>((done) => {
    resolve = done;
  });
  return { promise, resolve };
}

function renderSearch() {
  return render(
    <MemoryRouter initialEntries={["/help"]}>
      <I18nProvider>
        <Routes>
          <Route path="/help" element={<HelpSearch categories={categories} />} />
          <Route path="/help/*" element={<div data-testid="doc-target" />} />
        </Routes>
      </I18nProvider>
    </MemoryRouter>,
  );
}

beforeEach(() => {
  localStorage.setItem("lang", "en");
  vi.mocked(api.helpSearch).mockReset();
});

afterEach(() => {
  cleanup();
  localStorage.clear();
});

describe("HelpSearch", () => {
  it("waits for the debounce interval before requesting", async () => {
    const search = vi.mocked(api.helpSearch);
    search.mockResolvedValue(response([]));
    renderSearch();

    fireEvent.change(screen.getByRole("combobox"), { target: { value: "back" } });
    expect(search).not.toHaveBeenCalled();

    await waitFor(() => expect(search).toHaveBeenCalledTimes(1));
    expect(search.mock.calls[0][0]).toBe("back");
  });

  it("highlights snippet offsets as escaped text, not markup", async () => {
    vi.mocked(api.helpSearch).mockResolvedValue(
      response([
        hit({
          snippet: { text: "Back <b>bold</b> data", offsets: [[0, 4]] },
        }),
      ]),
    );
    renderSearch();

    fireEvent.change(screen.getByRole("combobox"), { target: { value: "back" } });
    const option = await screen.findByRole("option");

    expect(option.textContent).toContain("<b>bold</b>");
    expect(option.querySelector("b")).toBeNull();
    const marks = option.querySelectorAll("mark");
    expect([...marks].map((mark) => mark.textContent)).toEqual(["Back"]);
  });

  it("moves the active option with arrow keys and opens the hit on Enter", async () => {
    vi.mocked(api.helpSearch).mockResolvedValue(
      response([
        hit({ title: "First result", url: "/help/infrastructure/first" }),
        hit({ title: "Second result", url: "/help/infrastructure/second", anchor: "second" }),
      ]),
    );
    renderSearch();

    const input = screen.getByRole("combobox");
    fireEvent.change(input, { target: { value: "back" } });
    await screen.findAllByRole("option");

    fireEvent.keyDown(input, { key: "ArrowDown" });
    expect(screen.getAllByRole("option")[0].getAttribute("aria-selected")).toBe("true");

    fireEvent.keyDown(input, { key: "ArrowDown" });
    expect(screen.getAllByRole("option")[1].getAttribute("aria-selected")).toBe("true");

    fireEvent.keyDown(input, { key: "ArrowUp" });
    expect(screen.getAllByRole("option")[0].getAttribute("aria-selected")).toBe("true");

    fireEvent.keyDown(input, { key: "Enter" });
    expect(await screen.findByTestId("doc-target")).not.toBeNull();
  });

  it("clears the query and results on Escape", async () => {
    vi.mocked(api.helpSearch).mockResolvedValue(response([hit()]));
    renderSearch();

    const input = screen.getByRole("combobox") as HTMLInputElement;
    fireEvent.change(input, { target: { value: "back" } });
    await screen.findByRole("option");

    fireEvent.keyDown(input, { key: "Escape" });

    expect(input.value).toBe("");
    expect(screen.queryByRole("option")).toBeNull();
    expect(screen.queryByRole("listbox")).toBeNull();
  });

  it("ignores a stale response that arrives after a newer query", async () => {
    const first = deferred<HelpSearchResponse>();
    const second = deferred<HelpSearchResponse>();
    vi.mocked(api.helpSearch).mockReturnValueOnce(first.promise).mockReturnValueOnce(second.promise);

    renderSearch();
    const input = screen.getByRole("combobox");

    fireEvent.change(input, { target: { value: "fir" } });
    await waitFor(() => expect(api.helpSearch).toHaveBeenCalledTimes(1));

    fireEvent.change(input, { target: { value: "fix" } });
    await waitFor(() => expect(api.helpSearch).toHaveBeenCalledTimes(2));

    await act(async () => {
      second.resolve(response([hit({ title: "Fix result" })], "fix"));
      await Promise.resolve();
    });
    expect(await screen.findByText("Fix result")).not.toBeNull();

    await act(async () => {
      first.resolve(response([hit({ title: "Fir result" })], "fir"));
      await Promise.resolve();
    });
    await new Promise((done) => setTimeout(done, 30));

    expect(screen.queryByText("Fir result")).toBeNull();
    expect(screen.queryByText("Fix result")).not.toBeNull();
  });

  it("shows a neutral note when the search endpoint is not deployed", async () => {
    vi.mocked(api.helpSearch).mockRejectedValue({ status: 404 });
    renderSearch();

    fireEvent.change(screen.getByRole("combobox"), { target: { value: "back" } });

    expect(await screen.findByText("Search is not available in this build.")).not.toBeNull();
  });

  it("shows an error with a retry action for other failures", async () => {
    const search = vi.mocked(api.helpSearch);
    search.mockRejectedValueOnce({ status: 500 }).mockResolvedValueOnce(response([hit()]));
    renderSearch();

    fireEvent.change(screen.getByRole("combobox"), { target: { value: "back" } });
    expect(await screen.findByText("Search failed. Try again.")).not.toBeNull();

    fireEvent.click(screen.getByRole("button", { name: "Retry" }));
    expect(await screen.findByText("Backup result")).not.toBeNull();
  });

  it("shows the empty query state without a results panel", () => {
    renderSearch();

    expect(screen.queryByRole("listbox")).toBeNull();
    expect(screen.queryByText(/No results for/)).toBeNull();
  });
});
