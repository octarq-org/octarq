// @vitest-environment happy-dom
import { describe, it, expect, afterEach } from "vitest";
import { render, screen, cleanup } from "@testing-library/react";
import { I18nProvider } from "../../i18n";
import { A2UIRenderer } from "./A2UIRenderer";
import { ChartCard } from "./ChartCard";
import { DiffCard } from "./DiffCard";
import { ChartWidget, DiffWidget, ApprovalWidget, UnknownWidget } from "./types";

describe("A2UI Components", () => {
  afterEach(() => {
    cleanup();
  });

  const renderWithI18n = (ui: React.ReactElement) => {
    return render(<I18nProvider>{ui}</I18nProvider>);
  };

  describe("ChartCard", () => {
    it("renders chart with title, labels, and series data", () => {
      const widget: ChartWidget = {
        kind: "chart",
        title: "Monthly Invocations",
        data: {
          labels: ["Jan", "Feb", "Mar"],
          series: [
            { name: "Success", values: [100, 200, 300] },
            { name: "Error", values: [5, 10, 15] },
          ],
        },
      };

      renderWithI18n(<ChartCard widget={widget} />);

      expect(screen.getByText("Monthly Invocations")).not.toBeNull();
      expect(screen.getByText("Jan")).not.toBeNull();
      expect(screen.getByText("Feb")).not.toBeNull();
      expect(screen.getByText("Mar")).not.toBeNull();
      expect(screen.getByText("100 / 5")).not.toBeNull();
      expect(screen.getByText("200 / 10")).not.toBeNull();
      expect(screen.getByText("300 / 15")).not.toBeNull();
      expect(screen.getAllByText("Success").length).toBeGreaterThan(0);
      expect(screen.getAllByText("Error").length).toBeGreaterThan(0);
    });

    it("renders empty state when labels array is empty", () => {
      const widget: ChartWidget = {
        kind: "chart",
        title: "Empty Chart",
        data: { labels: [], series: [] },
      };

      renderWithI18n(<ChartCard widget={widget} />);
      expect(screen.getByText("Empty Chart")).not.toBeNull();
      expect(screen.getByText("No chart data")).not.toBeNull();
    });
  });

  describe("DiffCard", () => {
    it("renders diff with title, before, and after content", () => {
      const widget: DiffWidget = {
        kind: "diff",
        title: "Config Change Diff",
        before: "timeout: 30s\nretries: 3",
        after: "timeout: 60s\nretries: 5",
      };

      renderWithI18n(<DiffCard widget={widget} />);

      expect(screen.getByText("Config Change Diff")).not.toBeNull();
      expect(screen.getByText("Before")).not.toBeNull();
      expect(screen.getByText("After")).not.toBeNull();
      expect(screen.getByText((content) => content.includes("timeout: 30s"))).not.toBeNull();
      expect(screen.getByText((content) => content.includes("timeout: 60s"))).not.toBeNull();
    });
  });

  describe("A2UIRenderer dispatcher", () => {
    it("dispatches chart widget to ChartCard", () => {
      const widget: ChartWidget = {
        kind: "chart",
        title: "Traffic Overview",
        data: {
          labels: ["Day 1", "Day 2"],
          series: [{ name: "Hits", values: [42, 84] }],
        },
      };

      renderWithI18n(<A2UIRenderer widget={widget} />);
      expect(screen.getByText("Traffic Overview")).not.toBeNull();
      expect(screen.getByText("Day 1")).not.toBeNull();
      expect(screen.getByText("Day 2")).not.toBeNull();
    });

    it("dispatches diff widget to DiffCard", () => {
      const widget: DiffWidget = {
        kind: "diff",
        title: "Schema Diff",
        before: "v1 code",
        after: "v2 code",
      };

      renderWithI18n(<A2UIRenderer widget={widget} />);
      expect(screen.getByText("Schema Diff")).not.toBeNull();
      expect(screen.getByText("v1 code")).not.toBeNull();
      expect(screen.getByText("v2 code")).not.toBeNull();
    });

    it("renders approval widget with tool name and args", () => {
      const widget: ApprovalWidget = {
        kind: "approval",
        title: "Approve Deployment",
        tool: "deploy_service",
        args: { service: "octarq-api", replicaCount: 3 },
      };

      renderWithI18n(<A2UIRenderer widget={widget} />);
      expect(screen.getByText("Approve Deployment")).not.toBeNull();
      expect(screen.getByText("deploy_service")).not.toBeNull();
      expect(screen.getByText((content) => content.includes("octarq-api"))).not.toBeNull();
    });

    it("renders fallback JSON for unknown widget kind without crashing", () => {
      const widget: UnknownWidget = {
        kind: "custom_pro_topology",
        title: "Custom Network Topology",
        nodes: ["node-a", "node-b"],
      };

      renderWithI18n(<A2UIRenderer widget={widget} />);
      expect(screen.getByText("Unsupported widget:")).not.toBeNull();
      expect(screen.getByText("custom_pro_topology")).not.toBeNull();
      expect(screen.getByText((content) => content.includes("node-a"))).not.toBeNull();
    });
  });
});
