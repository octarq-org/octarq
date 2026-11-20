// @vitest-environment jsdom
import { cleanup, render, screen } from "@testing-library/react";
import { MemoryRouter } from "react-router-dom";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import type { ReactNode } from "react";
import WorkbenchPage from "./Workbench";
import { api } from "../api/client";
import { I18nProvider } from "../i18n";
import { NsgoLoadError } from "../../../packages/plugin-sdk/src";

interface MountProps {
  src: string;
  params?: Record<string, unknown>;
  appearance?: string;
  locale?: string;
  fallback?: ReactNode | ((error: Error) => ReactNode);
  onOpenURL?: (url: string) => void;
}
let lastProps: MountProps | null = null;

vi.mock("@octarq/plugin-sdk", async (importOriginal) => {
  const actual = await importOriginal<typeof import("@octarq/plugin-sdk")>();
  return {
    ...actual,
    NsgoMount: (props: MountProps) => {
      lastProps = props;
      return <div data-testid="mounted">{JSON.stringify({ params: props.params, locale: props.locale })}</div>;
    },
  };
});

function renderPage() {
  return render(
    <I18nProvider>
      <MemoryRouter>
        <WorkbenchPage />
      </MemoryRouter>
    </I18nProvider>,
  );
}

beforeEach(() => {
  lastProps = null;
  vi.spyOn(api, "orgs").mockResolvedValue([{ id: 4, name: "Acme", slug: "acme" }]);
});

afterEach(() => {
  cleanup();
  vi.restoreAllMocks();
});

describe("WorkbenchPage", () => {
  it("mounts the console build with the workspace and the embedded flag", async () => {
    renderPage();
    expect(await screen.findByTestId("mounted")).toBeTruthy();
    expect(lastProps?.src).toBe("/console/nsgo.json");
    expect(lastProps?.params).toEqual({ orgId: 4, embedded: true });
    expect(lastProps?.locale).toBe("en");
  });

  it("says the workspace failed instead of mounting without one", async () => {
    vi.spyOn(api, "orgs").mockRejectedValueOnce(new Error("boom"));
    renderPage();
    const alert = await screen.findByRole("alert");
    expect(alert.textContent).toContain("boom");
    expect(screen.queryByTestId("mounted")).toBeNull();
  });

  it("names a missing build as missing, not as a crash", async () => {
    renderPage();
    await screen.findByTestId("mounted");
    const fallback = lastProps?.fallback;
    expect(typeof fallback).toBe("function");
    const show = (error: Error) =>
      render(
        <I18nProvider>
          {typeof fallback === "function" ? fallback(error) : null}
        </I18nProvider>,
      ).container.textContent;
    expect(show(new NsgoLoadError("/console/nsgo.json", "HTTP 404", 404))).toContain(
      "does not serve the workbench build",
    );
    expect(show(new NsgoLoadError("/console/nsgo.json", "compile failed"))).toContain(
      "The workbench did not load.",
    );
  });
});
