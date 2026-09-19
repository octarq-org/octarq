import React from "react";
import { describe, it, expect } from "vitest";
import { render } from "@testing-library/react";
import { Switch } from "./switch";

describe("Switch", () => {
  it("forwards aria-label to the control (regression: it used to be dropped)", () => {
    const { container } = render(
      <Switch checked={false} onCheckedChange={() => {}} aria-label="Rename link" />,
    );
    const control = container.querySelector("[role='switch']");
    expect(control?.getAttribute("aria-label")).toBe("Rename link");
  });

  it("reflects checked state through data-checked", () => {
    const { container } = render(<Switch checked onCheckedChange={() => {}} />);
    expect(container.querySelector("[role='switch']")?.hasAttribute("data-checked")).toBe(true);
  });
});
