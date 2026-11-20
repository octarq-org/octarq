import React from "react";
import { describe, it, expect } from "vitest";
import { render } from "@testing-library/react";
import { Button } from "./button";

describe("Button", () => {
  it("paints primary from the flat accent tokens, never a hardcoded brand color", () => {
    const { container } = render(<Button variant="primary">Test</Button>);
    const button = container.firstChild as HTMLElement;

    expect(button.className).toContain("bg-primary-solid");
    expect(button.className).toContain("text-primary-foreground");
    expect(button.className).toContain("hover:bg-primary-solid-hover");

    expect(button.className).not.toContain("from-indigo");
    expect(button.className).not.toContain("to-violet");
    expect(button.className).not.toContain("gradient-primary");
    expect(button.className).not.toMatch(/\b(bg|text|border)-(indigo|violet|blue|sky|purple)-\d/);
  });

  it("applies the size axis, defaulting to md", () => {
    const { container: sm } = render(<Button size="sm">S</Button>);
    expect((sm.firstChild as HTMLElement).className).toContain("h-8");

    const { container: lg } = render(<Button size="lg">L</Button>);
    expect((lg.firstChild as HTMLElement).className).toContain("h-11");

    const { container: def } = render(<Button>D</Button>);
    expect((def.firstChild as HTMLElement).className).toContain("h-10");
  });

  it("supports the secondary variant that only the app-local copy used to have", () => {
    const { container } = render(<Button variant="secondary">Test</Button>);
    const button = container.firstChild as HTMLElement;

    expect(button.className).toContain("bg-card");
    expect(button.className).toContain("border-border");
  });

  it("forwards a ref to the underlying button element", () => {
    const ref = React.createRef<HTMLButtonElement>();
    render(<Button ref={ref}>Test</Button>);

    expect(ref.current).toBeInstanceOf(HTMLButtonElement);
  });
});
