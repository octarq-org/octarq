import React from "react";
import { afterEach, describe, expect, it } from "vitest";
import { cleanup, render, screen } from "@testing-library/react";
import { AreaChart, BarList } from "./charts";
import { I18nProvider } from "../../i18n";

afterEach(() => {
  cleanup();
});

function renderWithI18n(ui: React.ReactElement, resources = {}) {
  return render(<I18nProvider resources={resources}>{ui}</I18nProvider>);
}

describe("AreaChart", () => {
  it("renders empty state placeholder when series is empty or undefined", () => {
    const { rerender } = renderWithI18n(<AreaChart series={[]} />);
    expect(screen.getByText("No data yet")).toBeDefined();

    rerender(
      <I18nProvider resources={{ en: { uiCommon: { noDataYet: "Keine Daten" } } }}>
        <AreaChart series={[]} />
      </I18nProvider>,
    );
    expect(screen.getByText("Keine Daten")).toBeDefined();
  });

  it("renders SVG with line, gradient fill, and data points when series is provided", () => {
    const series = [
      { key: "Mon", count: 10 },
      { key: "Tue", count: 30 },
      { key: "Wed", count: 20 },
    ];
    const { container } = renderWithI18n(<AreaChart series={series} height={150} />);

    const svg = container.querySelector("svg");
    expect(svg).toBeDefined();
    expect(svg?.getAttribute("viewBox")).toBe("0 0 600 150");

    const gradient = container.querySelector("linearGradient#octarq-area");
    expect(gradient).toBeDefined();

    const paths = container.querySelectorAll("path");
    expect(paths.length).toBe(2);

    const circles = container.querySelectorAll("circle");
    expect(circles.length).toBe(3);
    expect(circles[0].querySelector("title")?.textContent).toBe("Mon: 10");
    expect(circles[1].querySelector("title")?.textContent).toBe("Tue: 30");
    expect(circles[2].querySelector("title")?.textContent).toBe("Wed: 20");
  });

  it("handles single-item series without division by zero", () => {
    const { container } = renderWithI18n(<AreaChart series={[{ key: "Solo", count: 5 }]} />);
    const circle = container.querySelector("circle");
    expect(circle?.getAttribute("cx")).toBe("300");
  });

  it("handles all-zero counts without NaN coordinates", () => {
    const series = [
      { key: "A", count: 0 },
      { key: "B", count: 0 },
    ];
    const { container } = renderWithI18n(<AreaChart series={series} />);
    const paths = container.querySelectorAll("path");
    expect(paths[0].getAttribute("d")).not.toContain("NaN");
    expect(paths[1].getAttribute("d")).not.toContain("NaN");
  });
});

describe("BarList", () => {
  it("renders empty fallback when rows is empty or null", () => {
    renderWithI18n(<BarList rows={[]} />);
    expect(screen.getByText("—")).toBeDefined();

    renderWithI18n(<BarList rows={null} empty="No referrers yet" />);
    expect(screen.getByText("No referrers yet")).toBeDefined();
  });

  it("renders rows with key, bar percentage, and count", () => {
    const rows = [
      { key: "google.com", count: 100 },
      { key: "github.com", count: 50 },
    ];
    renderWithI18n(<BarList rows={rows} />);

    expect(screen.getByText("google.com")).toBeDefined();
    expect(screen.getByText("github.com")).toBeDefined();
    expect(screen.getByText("100")).toBeDefined();
    expect(screen.getByText("50")).toBeDefined();
  });

  it("falls back to 'Direct' when row key is empty string", () => {
    renderWithI18n(<BarList rows={[{ key: "", count: 25 }]} />);
    expect(screen.getByText("Direct")).toBeDefined();
    expect(screen.getByText("25")).toBeDefined();
  });

  it("handles all-zero counts without division by zero", () => {
    const { container } = renderWithI18n(<BarList rows={[{ key: "zero", count: 0 }]} />);
    const bar = container.querySelector(".bg-primary\\/60");
    expect((bar as HTMLElement)?.style.width).toBe("0%");
  });
});
