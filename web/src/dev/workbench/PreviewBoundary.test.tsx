// @vitest-environment happy-dom
import { describe, expect, it, vi } from "vitest";
import { render, screen } from "@testing-library/react";
import { PreviewBoundary } from "./PreviewBoundary";

function Boom(): never {
  throw new Error("kaboom");
}

describe("PreviewBoundary", () => {
  it("surfaces a throwing preview instead of taking down the workbench", () => {
    const consoleError = vi.spyOn(console, "error").mockImplementation(() => {});
    render(
      <PreviewBoundary name="Boom">
        <Boom />
      </PreviewBoundary>,
    );
    expect(screen.getByText(/Boom preview threw/)).toBeTruthy();
    expect(screen.getByText(/kaboom/)).toBeTruthy();
    consoleError.mockRestore();
  });

  it("renders its children untouched when nothing throws", () => {
    render(
      <PreviewBoundary name="Fine">
        <p>all good</p>
      </PreviewBoundary>,
    );
    expect(screen.getByText("all good")).toBeTruthy();
  });
});
