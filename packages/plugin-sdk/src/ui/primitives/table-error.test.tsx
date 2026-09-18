import { afterEach, describe, expect, it, vi } from "vitest";
import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { I18nProvider } from "../../i18n";
import { TableError } from "./table-error";

const resources = {
  en: {
    proTable: {
      errorTitle: "Failed to load data",
      retry: "Retry",
    },
    uiCommon: {
      lockedIntroPre: "This is a ",
      lockedIntroPost: " feature.",
      upgradeTo: "Upgrade to {{tier}}",
      comparePlans: "Compare plans",
      notAvailable: "{{feature}} is not available.",
    },
  },
};

function renderWithI18n(ui: React.ReactElement) {
  return render(<I18nProvider resources={resources}>{ui}</I18nProvider>);
}

describe("TableError", () => {
  afterEach(() => {
    cleanup();
  });

  it("renders the 402 upsell mask for a licensed-feature error", () => {
    renderWithI18n(<TableError error={{ status: 402 }} />);
    expect(screen.getByText(/This is a/i)).toBeTruthy();
    expect(screen.getByText(/pro_table/i)).toBeTruthy();
  });

  it("renders the error message", () => {
    renderWithI18n(<TableError error={new Error("Something went wrong")} />);
    expect(screen.getByText("Something went wrong")).toBeTruthy();
  });

  it("calls onRetry when the retry button is clicked", () => {
    const onRetry = vi.fn();
    renderWithI18n(<TableError error={new Error("Failed")} onRetry={onRetry} />);

    fireEvent.click(screen.getByRole("button"));
    expect(onRetry).toHaveBeenCalledOnce();
  });

  it("prefers an explicit errorText over the error's own message", () => {
    renderWithI18n(<TableError error={new Error("System error")} errorText="Custom user friendly error" />);
    expect(screen.getByText("Custom user friendly error")).toBeTruthy();
    expect(screen.queryByText("System error")).toBeNull();
  });

  it("reads status from an axios-style nested response", () => {
    renderWithI18n(<TableError error={{ response: { status: 402 } }} />);
    expect(screen.getByText(/pro_table/i)).toBeTruthy();
  });

  it("falls back to the localized title when a non-Error is rejected", () => {
    // The title and the message fallback are the same string here, so assert on
    // the rendered set rather than a single node.
    renderWithI18n(<TableError error={{ unexpected: true }} />);
    expect(screen.getAllByText("Failed to load data").length).toBeGreaterThan(0);
  });
});
