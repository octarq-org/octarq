// @vitest-environment happy-dom
import { describe, it, expect, vi, beforeEach, afterEach } from "vitest";
import { render, screen, fireEvent, waitFor, cleanup } from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { HealthOverview } from "./HealthOverview";
import { HEALTH_QUERY_KEY } from "../api";
import {
  formatBytes,
  formatDuration,
  normalizeHealthStatus,
  HealthReport,
} from "../types";
import { I18nProvider } from "../../../i18n";
import { parseWithFallback } from "../../../lib/parseWithFallback";
import { z } from "zod";

function createTestQueryClient() {
  return new QueryClient({
    defaultOptions: {
      queries: {
        retry: false,
        refetchOnWindowFocus: false,
      },
    },
  });
}

function renderWithClient(ui: React.ReactElement, client = createTestQueryClient()) {
  return render(
    <QueryClientProvider client={client}>
      <I18nProvider>{ui}</I18nProvider>
    </QueryClientProvider>
  );
}

const mockHealthyReport: HealthReport = {
  overall: "ok",
  checked_at: "2026-09-08T12:00:00Z",
  providers: [
    {
      name: "runtime",
      status: "ok",
      message: "ok",
      category: "runtime",
      unit: "bytes",
      metrics: {
        goroutines: 42,
        heap_alloc_bytes: 20971520, // 20 MB
        heap_sys_bytes: 67108864, // 64 MB
        heap_inuse_bytes: 25165824,
        uptime_seconds: 7325, // 2h 2m
        go_version: "go1.25.0",
        num_cpu: 8,
      },
    },
    {
      name: "database",
      status: "ok",
      message: "ok",
      category: "database",
      unit: "count",
      metrics: {
        open_connections: 5,
        max_open_connections: 50,
        in_use: 2,
        idle: 3,
        slow_queries: 0,
        ping_latency_ms: 2,
      },
    },
    {
      name: "disk",
      status: "ok",
      message: "ok",
      category: "storage",
      unit: "bytes",
      metrics: {
        total_bytes: 107374182400, // 100 GB
        used_bytes: 42949672960, // 40 GB
        available_bytes: 64424509440, // 60 GB
        usage_percent: 40.0,
        path: "/data/octarq",
      },
    },
  ],
};

describe("HealthOverview Component", () => {
  beforeEach(() => {
    vi.restoreAllMocks();
  });

  afterEach(() => {
    cleanup();
    vi.restoreAllMocks();
  });

  it("renders loading skeleton when request is pending", () => {
    const client = createTestQueryClient();
    vi.spyOn(globalThis, "fetch").mockReturnValue(new Promise(() => {}));

    renderWithClient(<HealthOverview />, client);
    expect(screen.getByTestId("health-overview-loading")).toBeTruthy();
  });

  it("renders healthy status and all 3 provider cards with metrics", async () => {
    const client = createTestQueryClient();
    client.setQueryData(HEALTH_QUERY_KEY, mockHealthyReport);

    renderWithClient(<HealthOverview />, client);

    // Global title and badge
    expect(screen.getByTestId("health-overview")).toBeTruthy();
    expect(screen.getByText("System Health")).toBeTruthy();
    expect(screen.getAllByText("Healthy").length).toBeGreaterThanOrEqual(1);

    // Runtime card
    expect(screen.getByTestId("health-card-runtime")).toBeTruthy();
    expect(screen.getByText("42")).toBeTruthy(); // goroutines
    expect(screen.getByText("go1.25.0 · 8 CPU Cores")).toBeTruthy();
    expect(screen.getByText("2h 2m")).toBeTruthy(); // uptime
    expect(screen.getByText("Heap: 20 MB")).toBeTruthy();
    expect(screen.getByText("Sys: 64 MB")).toBeTruthy();

    // Database card
    expect(screen.getByTestId("health-card-database")).toBeTruthy();
    expect(screen.getByText("5")).toBeTruthy(); // open conns
    expect(screen.getByText("/ 50")).toBeTruthy(); // max conns
    expect(screen.getByText("2 ms")).toBeTruthy(); // ping latency

    // Disk card
    expect(screen.getByTestId("health-card-disk")).toBeTruthy();
    expect(screen.getByText("40%")).toBeTruthy(); // usage percent
    expect(screen.getByText("/data/octarq")).toBeTruthy();
    expect(screen.getByText("40 GB")).toBeTruthy(); // used
    expect(screen.getByText("60 GB")).toBeTruthy(); // free
    expect(screen.getByText("100 GB")).toBeTruthy(); // total

    // No troubleshooting alert when healthy
    expect(screen.queryByTestId("health-troubleshooting-alert")).toBeNull();
  });

  it("renders troubleshooting alert when provider status is degraded", async () => {
    const degradedReport: HealthReport = {
      ...mockHealthyReport,
      overall: "warn",
      providers: [
        mockHealthyReport.providers[0],
        {
          ...mockHealthyReport.providers[1],
          status: "warn",
          message: "3 slow queries detected",
          metrics: {
            ...mockHealthyReport.providers[1].metrics,
            slow_queries: 3,
            ping_latency_ms: 45,
          },
        },
        mockHealthyReport.providers[2],
      ],
    };

    const client = createTestQueryClient();
    client.setQueryData(HEALTH_QUERY_KEY, degradedReport);

    renderWithClient(<HealthOverview />, client);

    // Overall status shows Degraded
    expect(screen.getAllByText("Degraded").length).toBeGreaterThanOrEqual(1);

    // Troubleshooting alert should appear with database recommendations
    const alert = screen.getByTestId("health-troubleshooting-alert");
    expect(alert).toBeTruthy();
    expect(screen.getByText("Subsystem Health Alert")).toBeTruthy();
    expect(screen.getByText(/3 slow queries detected/i)).toBeTruthy();
    expect(
      screen.getByText(
        /Check database service connectivity, pool configurations, and slow query logs/i
      )
    ).toBeTruthy();
  });

  it("renders danger alert when provider status is down", async () => {
    const downReport: HealthReport = {
      ...mockHealthyReport,
      overall: "error",
      providers: [
        mockHealthyReport.providers[0],
        mockHealthyReport.providers[1],
        {
          ...mockHealthyReport.providers[2],
          status: "error",
          message: "disk space critically low: 96.5% used",
          metrics: {
            ...mockHealthyReport.providers[2].metrics,
            usage_percent: 96.5,
          },
        },
      ],
    };

    const client = createTestQueryClient();
    client.setQueryData(HEALTH_QUERY_KEY, downReport);

    renderWithClient(<HealthOverview />, client);

    expect(screen.getAllByText("Down").length).toBeGreaterThanOrEqual(1);
    const alert = screen.getByTestId("health-troubleshooting-alert");
    expect(alert).toBeTruthy();
    expect(screen.getByText(/Disk usage is nearing capacity/i)).toBeTruthy();
  });

  it("triggers refetch on clicking manual refresh button", async () => {
    const client = createTestQueryClient();
    client.setQueryData(HEALTH_QUERY_KEY, mockHealthyReport);
    const fetchSpy = vi.spyOn(globalThis, "fetch").mockResolvedValue(
      new Response(JSON.stringify(mockHealthyReport), {
        status: 200,
        headers: { "Content-Type": "application/json" },
      })
    );

    renderWithClient(<HealthOverview />, client);

    const refreshBtn = screen.getByTestId("health-refresh-btn");
    fireEvent.click(refreshBtn);

    await waitFor(() => {
      expect(fetchSpy).toHaveBeenCalled();
    });
  });

  it("renders error view with retry button when query fails", async () => {
    const client = createTestQueryClient();
    const fetchSpy = vi.spyOn(globalThis, "fetch").mockRejectedValue(new Error("Failed"));

    renderWithClient(<HealthOverview />, client);

    await waitFor(() => {
      expect(screen.getByTestId("health-overview-error")).toBeTruthy();
    });
    expect(screen.getByText("Failed to load health status")).toBeTruthy();

    const retryBtn = screen.getByTestId("health-retry-btn");
    fireEvent.click(retryBtn);
    expect(fetchSpy).toHaveBeenCalledTimes(2);
  });
});

describe("Formatters and Enums", () => {
  it("formats bytes accurately across scales", () => {
    expect(formatBytes(0)).toBe("0 B");
    expect(formatBytes(1024)).toBe("1 KB");
    expect(formatBytes(1048576)).toBe("1 MB");
    expect(formatBytes(1073741824)).toBe("1 GB");
    expect(formatBytes(-1)).toBe("-");
    expect(formatBytes(NaN)).toBe("-");
    expect(formatBytes(undefined)).toBe("-");
  });

  it("formats duration accurately", () => {
    expect(formatDuration(45)).toBe("45s");
    expect(formatDuration(150)).toBe("2m 30s");
    expect(formatDuration(3665)).toBe("1h 1m");
    expect(formatDuration(90000)).toBe("1d 1h");
    expect(formatDuration(-10)).toBe("-");
    expect(formatDuration(null)).toBe("-");
  });

  it("normalizes health statuses with default fallback", () => {
    expect(normalizeHealthStatus("ok")).toBe("healthy");
    expect(normalizeHealthStatus("healthy")).toBe("healthy");
    expect(normalizeHealthStatus("warn")).toBe("degraded");
    expect(normalizeHealthStatus("degraded")).toBe("degraded");
    expect(normalizeHealthStatus("error")).toBe("down");
    expect(normalizeHealthStatus("down")).toBe("down");
    expect(normalizeHealthStatus("unknown_status")).toBe("healthy");
    expect(normalizeHealthStatus(undefined)).toBe("healthy");
  });

  it("safely parses with fallback without throwing on corrupt data", () => {
    const testSchema = z.object({ count: z.number() });
    const fallback = { count: 0 };

    expect(parseWithFallback(testSchema, { count: 5 }, fallback)).toEqual({ count: 5 });
    expect(parseWithFallback(testSchema, "invalid", fallback)).toEqual({ count: 0 });
    expect(parseWithFallback(testSchema, null, fallback)).toEqual({ count: 0 });
  });
});
