// @vitest-environment jsdom
import { cleanup, render, screen } from "@testing-library/react";
import { lazy } from "react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { ExtensionSlot } from "./ExtensionSlot";
import { registerUIPlugin, resetRegistry } from "./registry";
import type { LazyPage, UIWidget } from "./types";

// Real React.lazy, resolved synchronously — exercises the same Suspense path a
// plugin's async chunk goes through.
const lazyWidget = (text: string): LazyPage =>
  lazy(async () => ({ default: () => <div data-testid="widget">{text}</div> }));

const throwingWidget = (): LazyPage =>
  lazy(async () => ({
    default: () => {
      throw new Error("widget exploded");
    },
  }));

const compose = (name: string, widgets: UIWidget[]) =>
  registerUIPlugin({ name, routes: [], widgets });

beforeEach(() => {
  resetRegistry();
});

afterEach(() => {
  // RTL's automatic cleanup hooks into a global afterEach, which is off here
  // (no `globals: true`) — unmount explicitly so trees don't leak across tests.
  cleanup();
  vi.restoreAllMocks();
});

describe("ExtensionSlot", () => {
  it("renders null for an empty slot", () => {
    compose("other", [{ slot: "elsewhere", Component: lazyWidget("nope") }]);
    const { container } = render(<ExtensionSlot name="home" />);
    expect(container.innerHTML).toBe("");
  });

  it("renders registered widgets in ascending order across plugins", async () => {
    compose("a", [{ slot: "home", Component: lazyWidget("second"), order: 2 }]);
    compose("b", [
      { slot: "home", Component: lazyWidget("first"), order: 1 },
      { slot: "elsewhere", Component: lazyWidget("wrong slot") },
    ]);
    render(<ExtensionSlot name="home" />);
    const widgets = await screen.findAllByTestId("widget");
    expect(widgets.map((w) => w.textContent)).toEqual(["first", "second"]);
    expect(screen.queryByText("wrong slot")).toBeNull();
  });

  it("isolates a throwing widget: siblings still render, the crasher disappears", async () => {
    // React logs caught boundary errors to console.error — keep test output clean.
    vi.spyOn(console, "error").mockImplementation(() => {});
    compose("a", [
      { slot: "home", Component: lazyWidget("survivor-1"), order: 1 },
      { slot: "home", Component: throwingWidget(), order: 2 },
      { slot: "home", Component: lazyWidget("survivor-2"), order: 3 },
    ]);
    render(<ExtensionSlot name="home" />);
    const widgets = await screen.findAllByTestId("widget");
    expect(widgets.map((w) => w.textContent)).toEqual(["survivor-1", "survivor-2"]);
  });
});
