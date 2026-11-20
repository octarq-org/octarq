// @vitest-environment jsdom
import { cleanup, render as rtlRender, screen, waitFor } from "@testing-library/react";
import { createRef } from "react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { NsgoMount, type NsgoMountHandle } from "./NsgoMount";
import type { NsgoPlayer } from "./loader";
import { I18nProvider } from "../i18n";

const render = (ui: React.ReactElement) =>
  rtlRender(ui, { wrapper: ({ children }) => <I18nProvider>{children}</I18nProvider> });

const loadApp = vi.fn();
vi.mock("./loader", async (importOriginal) => {
  const actual = await importOriginal<typeof import("./loader")>();
  return { ...actual, loadApp: (...args: unknown[]) => loadApp(...args) };
});

interface Recorded {
  element: HTMLElement | null;
  url: string | null;
  options: Record<string, unknown>;
}

let mounted: Recorded[] = [];
let player: NsgoPlayer;

function fakeLoader(mountError?: Error) {
  return {
    mount: vi.fn(async (element: HTMLElement | null, url: string | null, options: Record<string, unknown>) => {
      mounted.push({ element, url, options });
      if (mountError) throw mountError;
      element?.appendChild(document.createElement("canvas"));
      return player;
    }),
  };
}

beforeEach(() => {
  mounted = [];
  player = {
    hostEvent: vi.fn(),
    setAppearance: vi.fn(),
    setLocale: vi.fn(),
    setFontScale: vi.fn(),
  };
  loadApp.mockResolvedValue({
    loader: fakeLoader(),
    manifest: { wasm: "a.wasm", exec: "e.js", loader: "l.js" },
    base: "http://localhost/console/nsgo.json",
    module: {} as WebAssembly.Module,
  });
});

afterEach(() => {
  cleanup();
  vi.clearAllMocks();
});

describe("NsgoMount", () => {
  it("starts the app with the host's parameters, appearance and locale", async () => {
    render(<NsgoMount src="/console/nsgo.json" params={{ orgId: 7, embedded: true }} appearance="dark" locale="zh-Hans" />);
    await waitFor(() => expect(mounted).toHaveLength(1));
    expect(loadApp).toHaveBeenCalledWith("/console/nsgo.json");
    expect(mounted[0].url).toBe("http://localhost/console/nsgo.json");
    expect(mounted[0].options.params).toEqual({ orgId: 7, embedded: true });
    expect(mounted[0].options.appearance).toBe("dark");
    expect(mounted[0].options.locale).toBe("zh-Hans");
    expect(mounted[0].options.module).toBeDefined();
    expect(mounted[0].element?.querySelector("canvas")).not.toBeNull();
  });

  it("answers the app's calls through the handlers prop", async () => {
    const call = vi.fn((name: string) => `answer:${name}`);
    const post = vi.fn();
    render(<NsgoMount src="/console/nsgo.json" handlers={{ call, post }} />);
    await waitFor(() => expect(mounted).toHaveLength(1));
    const host = mounted[0].options.host as {
      call: (name: string, args: unknown) => unknown;
      post: (name: string, args: unknown) => void;
    };
    expect(host.call("decide", { id: 1 })).toBe("answer:decide");
    expect(call).toHaveBeenCalledWith("decide", { id: 1 });
    host.post("log", {});
    expect(post).toHaveBeenCalledWith("log", {});
  });

  it("offers openURL only when the host supplied a handler", async () => {
    const onOpenURL = vi.fn();
    render(<NsgoMount src="/console/nsgo.json" onOpenURL={onOpenURL} />);
    await waitFor(() => expect(mounted).toHaveLength(1));
    const host = mounted[0].options.host as { openURL?: (url: string) => Promise<boolean> };
    expect(host.openURL).toBeDefined();
    await expect(host.openURL!("/decisions/3")).resolves.toBe(true);
    expect(onOpenURL).toHaveBeenCalledWith("/decisions/3");

    cleanup();
    mounted = [];
    render(<NsgoMount src="/console/nsgo.json" />);
    await waitFor(() => expect(mounted).toHaveLength(1));
    expect((mounted[0].options.host as { openURL?: unknown }).openURL).toBeUndefined();
  });

  it("pushes appearance and locale changes to the running app", async () => {
    const { rerender } = render(<NsgoMount src="/console/nsgo.json" appearance="light" locale="en" />);
    await waitFor(() => expect(player.setAppearance).toHaveBeenCalledWith("light"));
    rerender(<NsgoMount src="/console/nsgo.json" appearance="dark" locale="ja" />);
    await waitFor(() => expect(player.setAppearance).toHaveBeenLastCalledWith("dark"));
    expect(player.setLocale).toHaveBeenLastCalledWith("ja");
    expect(mounted).toHaveLength(1);
  });

  it("exposes the player through a ref", async () => {
    const ref = createRef<NsgoMountHandle>();
    render(<NsgoMount ref={ref} src="/console/nsgo.json" />);
    await waitFor(() => expect(ref.current).not.toBeNull());
    ref.current!.hostEvent("host:refresh", { org: 2 });
    expect(player.hostEvent).toHaveBeenCalledWith("host:refresh", { org: 2 });
    ref.current!.setFontScale(1.2);
    expect(player.setFontScale).toHaveBeenCalledWith(1.2);
  });

  it("renders the fallback with the error when the app cannot load", async () => {
    const failure = new Error("manifest returned HTTP 404");
    loadApp.mockRejectedValueOnce(failure);
    render(<NsgoMount src="/missing/nsgo.json" fallback={(error) => <p>failed: {error.message}</p>} />);
    expect(await screen.findByText("failed: manifest returned HTTP 404")).toBeTruthy();
  });

  it("renders an alert, not a blank frame, when no fallback is given", async () => {
    loadApp.mockRejectedValueOnce(new Error("no build"));
    render(<NsgoMount src="/missing/nsgo.json" />);
    const alert = await screen.findByRole("alert");
    expect(alert.textContent).toContain("no build");
  });

  it("reports the status for a caller that styles around it", async () => {
    const { container } = render(<NsgoMount src="/console/nsgo.json" />);
    await waitFor(() => {
      expect(container.querySelector('[data-nsgo-status="ready"]')).not.toBeNull();
    });
  });

  it("stops calling the player once unmounted", async () => {
    const ref = createRef<NsgoMountHandle>();
    const tree = render(<NsgoMount ref={ref} src="/console/nsgo.json" />);
    await waitFor(() => expect(ref.current).not.toBeNull());
    const handle = ref.current!;
    tree.unmount();
    handle.hostEvent("late", {});
    expect(player.hostEvent).not.toHaveBeenCalled();
  });
});
