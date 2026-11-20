import React from "react";
import { describe, it, expect } from "vitest";
import { render, screen } from "@testing-library/react";
import { Badge, StatusBadge } from "./badge";

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
    expect(badge.childNodes[0]).toBe(glyph);
    expect(badge.textContent).toContain("Status");
  });

  it("accepts both the variant and tone prop names for the tone axis", () => {
    const { container } = render(<Badge variant="success">A</Badge>);
    expect((container.firstChild as HTMLElement).className).toContain("bg-success-bg");
    const { container: toneContainer } = render(<Badge tone="green">B</Badge>);
    expect((toneContainer.firstChild as HTMLElement).className).toContain("bg-success-bg");
  });

  it("maps semantic StatusBadge tones to consistent status tokens", () => {
    const { container: idle } = render(<StatusBadge tone="idle">Idle</StatusBadge>);
    expect((idle.firstChild as HTMLElement).className).toContain("bg-muted/70");

    const { container: running } = render(<StatusBadge tone="running">Running</StatusBadge>);
    expect((running.firstChild as HTMLElement).className).toContain("bg-info-bg");

    const { container: done } = render(<StatusBadge tone="done">Done</StatusBadge>);
    expect((done.firstChild as HTMLElement).className).toContain("bg-success-bg");

    const { container: paused } = render(<StatusBadge tone="paused">Paused</StatusBadge>);
    expect((paused.firstChild as HTMLElement).className).toContain("bg-warning-bg");

    const { container: blocked } = render(<StatusBadge tone="blocked">Blocked</StatusBadge>);
    expect((blocked.firstChild as HTMLElement).className).toContain("bg-danger-bg");

    const { container: review } = render(<StatusBadge tone="review">Review</StatusBadge>);
    expect((review.firstChild as HTMLElement).className).toContain("bg-review-bg");
  });
});
