import React from "react";
import { afterEach, describe, expect, it } from "vitest";
import { cleanup, render } from "@testing-library/react";
import { ListSkeleton } from "./list-skeleton";

afterEach(() => {
  cleanup();
});

describe("ListSkeleton", () => {
  it("renders default 6 skeleton rows with status accessibility attributes", () => {
    const { container } = render(<ListSkeleton />);
    const root = container.firstChild as HTMLElement;
    expect(root.getAttribute("role")).toBe("status");
    expect(root.getAttribute("aria-busy")).toBe("true");

    const rows = container.querySelectorAll(".divide-y > div");
    expect(rows.length).toBe(6);

    rows.forEach((row) => {
      expect(row.querySelectorAll(".skeleton").length).toBe(2);
    });
  });

  it("renders custom row count", () => {
    const { container } = render(<ListSkeleton rows={3} />);
    const rows = container.querySelectorAll(".divide-y > div");
    expect(rows.length).toBe(3);
  });

  it("attaches aria-label when provided", () => {
    const { container } = render(<ListSkeleton ariaLabel="Loading email threads" />);
    const root = container.firstChild as HTMLElement;
    expect(root.getAttribute("aria-label")).toBe("Loading email threads");
  });

  it("handles rows=0 gracefully", () => {
    const { container } = render(<ListSkeleton rows={0} />);
    const rows = container.querySelectorAll(".divide-y > div");
    expect(rows.length).toBe(0);
  });
});
