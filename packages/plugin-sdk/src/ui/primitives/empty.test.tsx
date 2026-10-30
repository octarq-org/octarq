import React from "react";
import { describe, it, expect } from "vitest";
import { render, screen } from "@testing-library/react";
import { Empty } from "./empty";

describe("Empty", () => {
  it("renders reason, detail, and action as page text, not as the icon glyph", () => {
    const { container } = render(
      <Empty reason="No senders yet" detail="Add one to send mail." action={<button>Add sender</button>}>
        <svg data-testid="glyph" />
      </Empty>,
    );
    expect(screen.getByText("No senders yet")).toBeDefined();
    expect(screen.getByText("Add one to send mail.")).toBeDefined();
    expect(screen.getByRole("button", { name: "Add sender" })).toBeDefined();

    // The glyph sits in the small icon tile; the message does not.
    const glyph = screen.getByTestId("glyph");
    const tile = glyph.parentElement as HTMLElement;
    expect(tile.className).toContain("h-12");
    expect(tile.textContent).not.toContain("No senders yet");
    expect(container.textContent).toContain("No senders yet");
  });

  it("renders without a glyph", () => {
    const { container } = render(<Empty reason="Nothing here yet" />);
    expect(screen.getByText("Nothing here yet")).toBeDefined();
    expect(container.querySelector('[data-testid="glyph"]')).toBeNull();
  });
});
