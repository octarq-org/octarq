// @vitest-environment jsdom
import { cleanup, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it } from "vitest";
// @ts-ignore
import fs from "node:fs";
// @ts-ignore
import path from "node:path";
import { ARCH_PATH, BrandGlyph } from "./index";

afterEach(() => {
  cleanup();
});

describe("BrandGlyph & ARCH_PATH", () => {
  it("renders custom logo image when logoUrl is provided", () => {
    const { container } = render(<BrandGlyph appName="octarq" logoUrl="https://example.com/logo.png" />);
    const img = container.querySelector("img");
    expect(img).not.toBeNull();
    expect(img?.getAttribute("src")).toBe("https://example.com/logo.png");
  });

  it("renders Keystone Arch OctarqMark when logoUrl is empty and appName is octarq", () => {
    const { container } = render(<BrandGlyph appName="octarq" logoUrl="" />);
    const svgPath = container.querySelector("path");
    expect(svgPath).not.toBeNull();
    expect(svgPath?.getAttribute("d")).toBe(ARCH_PATH);
  });

  it("renders initial character when logoUrl is empty and appName is custom", () => {
    render(<BrandGlyph appName="MyCompany" logoUrl="" />);
    expect(screen.getByText("M")).toBeDefined();
    expect(screen.queryByRole("img")).toBeNull();
  });

  it("ensures ARCH_PATH matches single source of truth across all svg/astro copies", () => {
    // @ts-ignore
    const currentDir = typeof __dirname !== "undefined" ? __dirname : path.dirname(import.meta.url.replace("file://", ""));
    const rootDir = path.resolve(currentDir, "../../../../");
    const logoMarkSvg = fs.readFileSync(path.join(rootDir, "web/public/logo-mark.svg"), "utf-8");
    const logoMarkMonoSvg = fs.readFileSync(path.join(rootDir, "web/public/logo-mark-mono.svg"), "utf-8");
    const headerAstro = fs.readFileSync(path.join(rootDir, "website/src/components/Header.astro"), "utf-8");

    expect(logoMarkSvg).toContain(ARCH_PATH);
    expect(logoMarkMonoSvg).toContain(ARCH_PATH);
    expect(headerAstro).toContain(ARCH_PATH);
  });
});
