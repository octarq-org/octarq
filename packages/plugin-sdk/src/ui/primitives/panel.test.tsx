import React from "react";
import { describe, it, expect } from "vitest";
import { render, screen } from "@testing-library/react";
import { Panel } from "./panel";

describe("Panel", () => {
  it("renders the title, description, action, and content", () => {
    const { container } = render(
      <Panel title="Rate limits" description="Per-minute ceilings." action={<button>Save</button>}>
        <p>Body</p>
      </Panel>,
    );
    expect(screen.getByRole("heading", { name: "Rate limits" })).toBeDefined();
    expect(screen.getByText("Per-minute ceilings.")).toBeDefined();
    expect(screen.getByRole("button", { name: "Save" })).toBeDefined();
    expect(screen.getByText("Body")).toBeDefined();
    expect(container.firstChild).not.toBeNull();
  });

  it("adds its own vertical spacing when the caller passes none", () => {
    const { container } = render(<Panel title="A">x</Panel>);
    expect((container.firstChild as HTMLElement).className).toContain("space-y-4");
  });

  it("keeps the caller's spacing instead of stacking a second one", () => {
    // The caller's className must stay last in cn(): two space-y-* utilities
    // resolve by stylesheet order, not by the order in the class list.
    const { container } = render(
      <Panel title="A" className="space-y-6">
        x
      </Panel>,
    );
    const className = (container.firstChild as HTMLElement).className;
    expect(className).toContain("space-y-6");
    expect(className).not.toContain("space-y-4");
  });

  it("renders a heading-only panel, for a section that is a single control", () => {
    render(
      <Panel
        title="Runaway protection"
        action={<label htmlFor="guard">Enabled</label>}
      />,
    );
    expect(screen.getByRole("heading", { name: "Runaway protection" })).toBeDefined();
    expect(screen.getByText("Enabled")).toBeDefined();
  });

  it("colors the icon with the accent token unless the caller sets its own", () => {
    const { container: plain } = render(
      <Panel title="A" icon={<span data-testid="plain-icon" />}>
        x
      </Panel>,
    );
    expect(screen.getByTestId("plain-icon").parentElement?.className).toContain("text-accent-fg");

    // A caller-supplied color on the icon itself wins over the wrapper.
    render(
      <Panel title="B" icon={<span data-testid="danger-icon" className="text-danger-fg" />}>
        x
      </Panel>,
    );
    expect(screen.getByTestId("danger-icon").className).toContain("text-danger-fg");
    expect(plain.firstChild).not.toBeNull();
  });
});
