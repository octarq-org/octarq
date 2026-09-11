// @vitest-environment happy-dom
import { describe, it, expect, vi, afterEach } from "vitest";
import { render, screen, fireEvent, cleanup } from "@testing-library/react";
import { TableError } from "./TableError";
import { I18nProvider } from "../../i18n";

function renderWithI18n(ui: React.ReactElement) {
  return render(<I18nProvider>{ui}</I18nProvider>);
}

describe("TableError", () => {
  afterEach(() => {
    cleanup();
  });

  it("renders a 402 LockedFeature when status is 402", () => {
    renderWithI18n(<TableError error={{ status: 402 }} />);
    expect(screen.getByText(/This is a/i)).toBeDefined();
    expect(screen.getByText(/pro_table/i)).toBeDefined();
  });

  it("renders error text properly", () => {
    renderWithI18n(<TableError error={new Error("Something went wrong")} />);
    expect(screen.getByText("Something went wrong")).toBeDefined();
  });

  it("calls onRetry when retry button is clicked", () => {
    const onRetry = vi.fn();
    renderWithI18n(<TableError error={new Error("Failed")} onRetry={onRetry} />);

    const retryBtn = screen.getByRole("button");
    fireEvent.click(retryBtn);
    expect(onRetry).toHaveBeenCalledOnce();
  });

  it("renders provided errorText instead of error object message", () => {
    renderWithI18n(
      <TableError
        error={new Error("System error")}
        errorText="Custom user friendly error"
      />
    );
    expect(screen.getByText("Custom user friendly error")).toBeDefined();
    expect(screen.queryByText("System error")).toBeNull();
  });

  it("extracts status correctly from axios-like error object", () => {
    // Should extract 402 from an axios-like response and render the LockedFeature
    renderWithI18n(
      <TableError error={{ response: { status: 402 } }} />
    );
    expect(screen.getByText(/pro_table/i)).toBeDefined();
  });
});
