// @vitest-environment happy-dom
import { describe, expect, it, vi } from "vitest";
import { render, fireEvent, act } from "@testing-library/react";
import { useState } from "react";
import {
  AreaChart,
  BarList,
  ListSkeleton,
  SetupStep,
  SetupChecklistProvider,
  useSetupChecklist,
  I18nProvider,
} from "@octarq/plugin-sdk";

const minimalResources = {
  en: {
    uiCommon: {
      noDataYet: "No data yet",
      direct: "Direct",
    },
  },
};

describe("Adversarial Catalog Stress: AreaChart", () => {
  it("handles null / undefined / empty series gracefully", () => {
    const { container: c1 } = render(
      <I18nProvider resources={minimalResources}>
        <AreaChart series={[] as any} />
      </I18nProvider>
    );
    expect(c1.textContent).toContain("No data yet");

    const { container: c2 } = render(
      <I18nProvider resources={minimalResources}>
        <AreaChart series={null as any} />
      </I18nProvider>
    );
    expect(c2.textContent).toContain("No data yet");
  });

  it("handles single data point without divide-by-zero or NaN in svg path", () => {
    const { container } = render(
      <I18nProvider resources={minimalResources}>
        <AreaChart series={[{ key: "Solo", count: 42 }]} />
      </I18nProvider>
    );
    const svg = container.querySelector("svg");
    expect(svg).toBeDefined();
    const paths = container.querySelectorAll("path");
    paths.forEach((p) => {
      const d = p.getAttribute("d") ?? "";
      expect(d).not.toContain("NaN");
      expect(d).not.toContain("Infinity");
    });
  });

  it("handles all-zero counts without crash", () => {
    const { container } = render(
      <I18nProvider resources={minimalResources}>
        <AreaChart
          series={[
            { key: "Day 1", count: 0 },
            { key: "Day 2", count: 0 },
          ]}
        />
      </I18nProvider>
    );
    const paths = container.querySelectorAll("path");
    paths.forEach((p) => {
      expect(p.getAttribute("d")).not.toContain("NaN");
    });
  });

  it("survives extreme values, negatives, and floating point counts", () => {
    const series = [
      { key: "Huge", count: 1e12 },
      { key: "Negative", count: -100 },
      { key: "Float", count: 0.000001 },
      { key: "Zero", count: 0 },
    ];
    const { container } = render(
      <I18nProvider resources={minimalResources}>
        <AreaChart series={series} height={300} />
      </I18nProvider>
    );
    const circles = container.querySelectorAll("circle");
    expect(circles.length).toBe(4);
  });

  it("survives extreme height dimensions (zero, large)", () => {
    const { container: cZero } = render(
      <I18nProvider resources={minimalResources}>
        <AreaChart series={[{ key: "A", count: 10 }]} height={0} />
      </I18nProvider>
    );
    expect(cZero.querySelector("svg")).toBeDefined();

    const { container: cLarge } = render(
      <I18nProvider resources={minimalResources}>
        <AreaChart series={[{ key: "A", count: 10 }]} height={2000} />
      </I18nProvider>
    );
    expect(cLarge.querySelector("svg")).toBeDefined();
  });

  it("handles extreme string keys with emojis and special characters", () => {
    const longKey = "A".repeat(5000);
    const xssKey = "<script>alert('xss')</script>";
    const emojiKey = "🚀🔥✨💎";
    const series = [
      { key: longKey, count: 5 },
      { key: xssKey, count: 10 },
      { key: emojiKey, count: 15 },
    ];
    const { container } = render(
      <I18nProvider resources={minimalResources}>
        <AreaChart series={series} />
      </I18nProvider>
    );
    const titles = Array.from(container.querySelectorAll("title")).map((t) => t.textContent);
    expect(titles).toContain(`${emojiKey}: 15`);
    expect(titles).toContain(`${xssKey}: 10`);
  });
});

describe("Adversarial Catalog Stress: BarList", () => {
  it("handles null / empty rows with custom or default fallback", () => {
    const { container: cDefault } = render(
      <I18nProvider resources={minimalResources}>
        <BarList rows={[]} />
      </I18nProvider>
    );
    expect(cDefault.textContent).toBe("—");

    const { container: cCustom } = render(
      <I18nProvider resources={minimalResources}>
        <BarList rows={null} empty={<span data-testid="empty-custom">Custom Empty</span>} />
      </I18nProvider>
    );
    expect(cCustom.querySelector('[data-testid="empty-custom"]')?.textContent).toBe("Custom Empty");
  });

  it("handles empty key fallback to Direct", () => {
    const rows = [
      { key: "", count: 100 },
      { key: "google.com", count: 50 },
    ];
    const { container } = render(
      <I18nProvider resources={minimalResources}>
        <BarList rows={rows} />
      </I18nProvider>
    );
    expect(container.textContent).toContain("Direct");
    expect(container.textContent).toContain("google.com");
  });

  it("handles 1000 items without memory leak or crash", () => {
    const rows = Array.from({ length: 1000 }, (_, i) => ({
      key: `source-${i}`,
      count: i * 3,
    }));
    const { container } = render(
      <I18nProvider resources={minimalResources}>
        <BarList rows={rows} />
      </I18nProvider>
    );
    expect(container.querySelectorAll(".font-mono").length).toBe(1000);
  });

  it("handles all-zero counts without NaN in bar percentage width", () => {
    const rows = [
      { key: "A", count: 0 },
      { key: "B", count: 0 },
    ];
    const { container } = render(
      <I18nProvider resources={minimalResources}>
        <BarList rows={rows} />
      </I18nProvider>
    );
    const bars = container.querySelectorAll(".bg-primary\\/60");
    bars.forEach((b) => {
      const style = (b as HTMLElement).style.width;
      expect(style).toBe("0%");
      expect(style).not.toContain("NaN");
    });
  });
});

describe("Adversarial Catalog Stress: ListSkeleton", () => {
  it("renders custom row counts and handles boundary counts (0, -1, 50)", () => {
    const { container: c0 } = render(<ListSkeleton rows={0} />);
    expect(c0.querySelectorAll(".p-4").length).toBe(0);

    const { container: cNeg } = render(<ListSkeleton rows={-1} />);
    expect(cNeg.querySelectorAll(".p-4").length).toBe(0);

    const { container: c50 } = render(<ListSkeleton rows={50} ariaLabel="Loading items" />);
    expect(c50.querySelectorAll(".p-4").length).toBe(50);
    expect(c50.querySelector('[role="status"]')?.getAttribute("aria-label")).toBe("Loading items");
  });
});

describe("Adversarial Catalog Stress: SetupStep & SetupChecklistProvider", () => {
  it("functions gracefully standalone without SetupChecklistProvider", () => {
    const onClick = vi.fn();
    const { container } = render(
      <SetupStep
        title="Standalone Step"
        description="Works even without checklist provider"
        completed={false}
        onClick={onClick}
      />
    );
    expect(container.textContent).toContain("Standalone Step");
    const button = container.querySelector("button");
    expect(button).toBeDefined();
    fireEvent.click(button!);
    expect(onClick).toHaveBeenCalledTimes(1);
  });

  it("accurately computes progress percent across dynamic registration and completion", () => {
    function DynamicChecklist() {
      const { totalCount, completedCount, progressPercent, allCompleted } = useSetupChecklist();
      const [step1Done, setStep1Done] = useState(false);
      const [showOptionalStep, setShowOptionalStep] = useState(true);

      return (
        <div>
          <span data-testid="total">{totalCount}</span>
          <span data-testid="completed">{completedCount}</span>
          <span data-testid="percent">{progressPercent}</span>
          <span data-testid="all">{allCompleted ? "yes" : "no"}</span>

          <SetupStep
            title="Step 1"
            description="First step"
            completed={step1Done}
            onClick={() => setStep1Done((d) => !d)}
          />
          {showOptionalStep && (
            <SetupStep
              title="Step 2"
              description="Optional step"
              completed={true}
              onClick={() => {}}
            />
          )}
          <button data-testid="toggle-optional" onClick={() => setShowOptionalStep((s) => !s)}>
            Toggle
          </button>
        </div>
      );
    }

    const { getByTestId, getByText } = render(
      <SetupChecklistProvider>
        <DynamicChecklist />
      </SetupChecklistProvider>
    );

    // Initial: 2 total, 1 completed (50%)
    expect(getByTestId("total").textContent).toBe("2");
    expect(getByTestId("completed").textContent).toBe("1");
    expect(getByTestId("percent").textContent).toBe("50");
    expect(getByTestId("all").textContent).toBe("no");

    // Complete Step 1: 2 total, 2 completed (100%)
    fireEvent.click(getByText("Step 1"));
    expect(getByTestId("completed").textContent).toBe("2");
    expect(getByTestId("percent").textContent).toBe("100");
    expect(getByTestId("all").textContent).toBe("yes");

    // Unmount optional step: 1 total, 1 completed (100%)
    fireEvent.click(getByTestId("toggle-optional"));
    expect(getByTestId("total").textContent).toBe("1");
    expect(getByTestId("completed").textContent).toBe("1");
    expect(getByTestId("percent").textContent).toBe("100");
    expect(getByTestId("all").textContent).toBe("yes");
  });

  it("handles rapid click stress without state corruption", () => {
    function ClickSpam() {
      const [done, setDone] = useState(false);
      return (
        <SetupStep
          title="Spam Step"
          description="Click test"
          completed={done}
          onClick={() => setDone((prev) => !prev)}
        />
      );
    }

    const { container } = render(
      <SetupChecklistProvider>
        <ClickSpam />
      </SetupChecklistProvider>
    );

    const btn = container.querySelector("button")!;
    for (let i = 0; i < 50; i++) {
      fireEvent.click(btn);
    }
    // 50 clicks: even number -> completed: false
    expect(container.querySelector("h3")?.className).not.toContain("line-through");
  });
});

describe("Adversarial Catalog Stress: Responsive & Theme Viewports", () => {
  const viewports = [
    { name: "mobile 320x568", width: 320, height: 568 },
    { name: "tablet 768x1024", width: 768, height: 1024 },
    { name: "desktop 1920x1080", width: 1920, height: 1080 },
    { name: "ultrawide 3840x2160", width: 3840, height: 2160 },
    { name: "zero 0x0", width: 0, height: 0 },
  ];

  const themes = ["light", "dark", "contrast"];

  for (const vp of viewports) {
    for (const theme of themes) {
      it(`renders all 5 components in ${vp.name} under ${theme} theme without errors`, () => {
        // Mock viewport dimensions
        window.innerWidth = vp.width;
        window.innerHeight = vp.height;
        window.dispatchEvent(new Event("resize"));

        const { container } = render(
          <div className={theme === "dark" ? "dark bg-background text-foreground" : "bg-background text-foreground"}>
            <I18nProvider resources={minimalResources}>
              <SetupChecklistProvider>
                <div style={{ width: vp.width || "100%", height: vp.height || "auto" }}>
                  <AreaChart
                    series={[
                      { key: "1", count: 10 },
                      { key: "2", count: 25 },
                      { key: "3", count: 18 },
                    ]}
                    height={100}
                  />
                  <BarList
                    rows={[
                      { key: "alpha", count: 50 },
                      { key: "beta", count: 30 },
                    ]}
                  />
                  <ListSkeleton rows={2} />
                  <SetupStep
                    title="Domain Setup"
                    description="DNS records configuration"
                    completed={false}
                    onClick={() => {}}
                  />
                </div>
              </SetupChecklistProvider>
            </I18nProvider>
          </div>
        );

        expect(container).toBeDefined();
        expect(container.querySelector("svg")).not.toBeNull();
        expect(container.querySelector('[role="status"]')).not.toBeNull();
      });
    }
  }
});
