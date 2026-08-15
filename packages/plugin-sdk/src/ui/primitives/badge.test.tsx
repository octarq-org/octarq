import React from "react";
import { describe, it, expect } from "vitest";
import { render, screen } from "@testing-library/react";
import { Badge } from "./badge";

describe("Badge", () => {
  it("renders its text children (regression: children must not be dropped)", () => {
    render(<Badge>Operational</Badge>);
    expect(screen.getByText("Operational")).toBeDefined();
  });

  it("renders the shape glyph before the text when shape is set", () => {
    const { container } = render(<Badge shape="dot">Status</Badge>);
    const badge = container.firstChild as HTMLElement;
    const glyph = badge.firstElementChild as HTMLElement | null;
    expect(glyph).not.toBeNull();
    expect(glyph?.getAttribute("aria-hidden")).toBe("true");
    expect(glyph?.textContent).toBe("●");
    // glyph must come first, text still present
    expect(badge.childNodes[0]).toBe(glyph);
    expect(badge.textContent).toContain("Status");
  });

  it("accepts both the variant and tone prop names for the tone axis", () => {
    const { container } = render(<Badge variant="success">A</Badge>);
    expect((container.firstChild as HTMLElement).className).toContain("bg-success-bg");
    const { container: toneContainer } = render(<Badge tone="green">B</Badge>);
    expect((toneContainer.firstChild as HTMLElement).className).toContain("bg-success-bg");
  });
});
