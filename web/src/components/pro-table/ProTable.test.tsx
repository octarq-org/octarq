// @vitest-environment happy-dom
import { describe, it, expect, vi, beforeEach, afterEach } from "vitest";
import { render, screen, waitFor, fireEvent, cleanup } from "@testing-library/react";
import { z } from "zod";
import { ProTable } from "./ProTable";
import { ProColumn, ProTableActionRef } from "./types";
import { cleanParams, parseWithFallback, parseArrayWithFallback } from "./utils";
import { I18nProvider } from "../../i18n";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { useRef } from "react";

interface TestRow {
  id: number;
  name: string;
  role: string;
  count: number;
}

const testSchema = z.object({
  id: z.number(),
  name: z.string(),
  role: z.string(),
  count: z.number(),
});

function renderWithProviders(ui: React.ReactElement) {
  const testClient = new QueryClient({
    defaultOptions: {
      queries: {
        retry: false,
        staleTime: 0,
        gcTime: 0,
      },
    },
  });

  return render(
    <QueryClientProvider client={testClient}>
      <I18nProvider>{ui}</I18nProvider>
    </QueryClientProvider>
  );
}

afterEach(() => {
  cleanup();
});

describe("ProTable Utils", () => {
  it("cleanParams removes empty strings, null, and undefined", () => {
    const dirty = {
      name: "  Alice  ",
      role: "",
      email: "   ",
      count: 0,
      status: null,
      active: true,
      extra: undefined,
    };
    const cleaned = cleanParams(dirty);
    expect(cleaned).toEqual({
      name: "Alice",
      count: 0,
      active: true,
    });
  });

  it("parseWithFallback falls back on schema failure", () => {
    const warnSpy = vi.spyOn(console, "warn").mockImplementation(() => {});
    const fallback: TestRow = { id: 0, name: "Fallback", role: "none", count: 0 };
    const invalid = { id: "not-a-number", name: 123 };
    const parsed = parseWithFallback(testSchema, invalid, fallback);
    expect(parsed).toEqual(fallback);
    expect(warnSpy).toHaveBeenCalledWith("[ProTable Zod Schema Error]:", expect.any(Array));

    const valid = { id: 1, name: "Admin", role: "admin", count: 5 };
    const validParsed = parseWithFallback(testSchema, valid, fallback);
    expect(validParsed).toEqual(valid);
    warnSpy.mockRestore();
  });

  it("parseWithFallback calls onError when provided and skips warning", () => {
    const warnSpy = vi.spyOn(console, "warn").mockImplementation(() => {});
    const onError = vi.fn();
    const fallback: TestRow = { id: 0, name: "Fallback", role: "none", count: 0 };
    const invalid = { id: "not-a-number", name: 123 };

    const parsed = parseWithFallback(testSchema, invalid, fallback, onError);

    expect(parsed).toEqual(fallback);
    expect(onError).toHaveBeenCalled();
    expect(warnSpy).not.toHaveBeenCalled();
    warnSpy.mockRestore();
  });

  it("parseArrayWithFallback filters invalid array items", () => {
    const items = [
      { id: 1, name: "A", role: "admin", count: 10 },
      { id: "invalid", name: 123 },
      { id: 2, name: "B", role: "user", count: 20 },
    ];
    const parsed = parseArrayWithFallback(testSchema, items);
    expect(parsed).toHaveLength(2);
    expect(parsed[0].name).toBe("A");
    expect(parsed[1].name).toBe("B");
  });
});

describe("ProTable Component", () => {
  const mockData: TestRow[] = [
    { id: 1, name: "User 1", role: "admin", count: 10 },
    { id: 2, name: "User 2", role: "editor", count: 20 },
    { id: 3, name: "User 3", role: "viewer", count: 30 },
  ];

  const columns: ProColumn<TestRow>[] = [
    {
      title: "ID",
      dataIndex: "id",
      key: "id",
      hideInSearch: true,
      sorter: true,
    },
    {
      title: "Name",
      dataIndex: "name",
      key: "name",
      valueType: "text",
      sorter: true,
    },
    {
      title: "Role",
      dataIndex: "role",
      key: "role",
      valueType: "select",
      valueEnum: {
        admin: { text: "Administrator" },
        editor: { text: "Editor" },
        viewer: { text: "Viewer" },
      },
    },
    {
      title: "Count",
      dataIndex: "count",
      key: "count",
      valueType: "digit",
      render: (dom) => <span data-testid="count-cell">Qty: {dom}</span>,
    },
  ];

  it("renders table data from async request", async () => {
    const request = vi.fn().mockResolvedValue({
      data: mockData,
      total: 3,
      success: true,
    });

    renderWithProviders(
      <ProTable<TestRow>
        columns={columns}
        request={request}
        rowKey="id"
        headerTitle="Users Table"
      />
    );

    expect(screen.getByText("Users Table")).toBeDefined();

    await waitFor(() => {
      expect(screen.getByText("User 1")).toBeDefined();
      expect(screen.getByText("User 2")).toBeDefined();
      expect(screen.getByText("User 3")).toBeDefined();
    });

    expect(request).toHaveBeenCalledTimes(1);
    expect(screen.getAllByTestId("count-cell")).toHaveLength(3);
  });

  it("renders empty state when request returns empty data", async () => {
    const request = vi.fn().mockResolvedValue({
      data: [],
      total: 0,
      success: true,
    });

    renderWithProviders(
      <ProTable<TestRow>
        columns={columns}
        request={request}
        rowKey="id"
        emptyText="No users found"
      />
    );

    await waitFor(() => {
      expect(screen.getByText("No users found")).toBeDefined();
    });
  });

  it("renders error state and handles retry", async () => {
    let callCount = 0;
    const request = vi.fn().mockImplementation(() => {
      callCount++;
      if (callCount === 1) {
        return Promise.reject(new Error("Network Error"));
      }
      return Promise.resolve({
        data: mockData,
        total: 3,
        success: true,
      });
    });

    renderWithProviders(
      <ProTable<TestRow>
        columns={columns}
        request={request}
        rowKey="id"
      />
    );

    await waitFor(() => {
      expect(screen.getByText("Network Error")).toBeDefined();
    });

    // Click retry button
    const retryBtn = screen.getByRole("button", { name: /重试|Retry/i });
    fireEvent.click(retryBtn);

    await waitFor(() => {
      expect(screen.getByText("User 1")).toBeDefined();
    });

    expect(request).toHaveBeenCalledTimes(2);
  });

  it("supports server-side pagination", async () => {
    const request = vi.fn().mockImplementation((params) => {
      const page = params.page || 1;
      return Promise.resolve({
        data: [
          { id: page, name: `User Page ${page}`, role: "admin", count: 100 },
        ],
        total: 50,
        success: true,
      });
    });

    renderWithProviders(
      <ProTable<TestRow>
        columns={columns}
        request={request}
        rowKey="id"
        pagination={{
          defaultPageSize: 10,
          pageSizeOptions: [10, 20],
        }}
      />
    );

    await waitFor(() => {
      expect(screen.getByText("User Page 1")).toBeDefined();
    });

    // Click page 2
    const page2Btn = screen.getByRole("button", { name: "2" });
    fireEvent.click(page2Btn);

    await waitFor(() => {
      expect(screen.getByText("User Page 2")).toBeDefined();
    });

    expect(request).toHaveBeenLastCalledWith(
      expect.objectContaining({ page: 2, pageSize: 10 }),
      expect.any(Object),
      expect.any(Object)
    );
  });

  it("triggers search form filtering and cleans dirty params", async () => {
    const request = vi.fn().mockResolvedValue({
      data: mockData,
      total: 3,
      success: true,
    });

    renderWithProviders(
      <ProTable<TestRow>
        columns={columns}
        request={request}
        rowKey="id"
      />
    );

    await waitFor(() => {
      expect(screen.getByText("User 1")).toBeDefined();
    });

    // Type in name search input
    const nameInput = screen.getByPlaceholderText(/Name/i) as HTMLInputElement;
    fireEvent.change(nameInput, { target: { value: "   Alice   " } });

    // Click search button
    const searchBtn = screen.getByRole("button", { name: /查询|Search/i });
    fireEvent.click(searchBtn);

    await waitFor(() => {
      expect(request).toHaveBeenLastCalledWith(
        expect.objectContaining({ name: "Alice", page: 1 }),
        expect.any(Object),
        expect.any(Object)
      );
    });

    // Click reset button
    const resetBtn = screen.getByRole("button", { name: /重置|Reset/i });
    fireEvent.click(resetBtn);

    await waitFor(() => {
      expect(nameInput.value).toBe("");
    });
  });

  it("supports actionRef imperative reload and reset", async () => {
    let actionRefInstance: ProTableActionRef | undefined;

    const request = vi.fn().mockResolvedValue({
      data: mockData,
      total: 3,
      success: true,
    });

    function TestWrapper() {
      const ref = useRef<ProTableActionRef>();
      return (
        <ProTable<TestRow>
          actionRef={(r) => {
            ref.current = r;
            actionRefInstance = r;
          }}
          columns={columns}
          request={request}
          rowKey="id"
        />
      );
    }

    renderWithProviders(<TestWrapper />);

    await waitFor(() => {
      expect(screen.getByText("User 1")).toBeDefined();
      expect(actionRefInstance).toBeDefined();
    });

    // Call reload through actionRef
    actionRefInstance!.reload();
    await waitFor(() => {
      expect(request).toHaveBeenCalledTimes(2);
    });

    // Call setPage through actionRef
    actionRefInstance!.setPage(3);
    await waitFor(() => {
      expect(request).toHaveBeenLastCalledWith(
        expect.objectContaining({ page: 3 }),
        expect.any(Object),
        expect.any(Object)
      );
    });
  });

  it("validates data against Zod schema with fallback", async () => {
    const corruptData = [
      { id: 1, name: "Valid", role: "admin", count: 10 },
      { id: "invalid_id" as any, name: "Corrupt", role: 123 as any, count: "invalid" as any },
    ];

    const request = vi.fn().mockResolvedValue({
      data: corruptData,
      total: 2,
      success: true,
    });

    renderWithProviders(
      <ProTable<TestRow>
        columns={columns}
        request={request}
        schema={testSchema}
        rowKey="id"
      />
    );

    await waitFor(() => {
      expect(screen.getByText("Valid")).toBeDefined();
      expect(screen.queryByText("Corrupt")).toBeNull();
    });
  });
});
