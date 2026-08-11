import React from "react";
import { describe, it, expect } from "vitest";
import { render } from "@testing-library/react";
import { Button } from "./button";

describe("Button", () => {
  it("uses the CSS variable for the primary brand gradient, not hardcoded colors", () => {
    const { container } = render(<Button variant="primary">Test</Button>);
    const button = container.firstChild as HTMLElement;
    
    expect(button).toBeDefined();
    expect(button.className).not.toContain("from-indigo");
    expect(button.className).not.toContain("to-violet");
    
    // It should use the css variable
    expect(button.className).toContain("var(--gradient-primary)");
  });
});
