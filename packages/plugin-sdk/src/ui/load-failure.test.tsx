import { afterEach, describe, expect, it, vi } from "vitest";
import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { I18nProvider } from "../i18n";
import { AccessDenied, LoadFailure } from "./load-failure";

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
      accessDeniedTitle: "Access denied",
      accessDeniedBody: "You don’t have permission to view this page.",
    },
  },
};

function renderWithI18n(ui: React.ReactElement) {
  return render(<I18nProvider resources={resources}>{ui}</I18nProvider>);
}

const header = <div data-testid="page-header">Storefront</div>;

describe("LoadFailure", () => {
  afterEach(() => {
    cleanup();
  });

  it("renders the upsell mask for 402 and hides the page header", () => {
    renderWithI18n(<LoadFailure error={{ status: 402 }} feature="Storefront products" header={header} />);
    expect(screen.getByText(/Storefront products/i)).toBeTruthy();
    expect(screen.getByText(/This is a/i)).toBeTruthy();
    expect(screen.queryByTestId("page-header")).toBeNull();
  });

  it("treats 404 as a gated feature, not a load failure", () => {
    renderWithI18n(<LoadFailure error={{ response: { status: 404 } }} feature="Storefront products" />);
    expect(screen.getByText(/This is a/i)).toBeTruthy();
    expect(screen.queryByText("Failed to load data")).toBeNull();
  });

  it("renders access denied for 403, above the page header", () => {
    renderWithI18n(<LoadFailure error={{ status: 403 }} feature="Storefront products" header={header} />);
    expect(screen.getByTestId("page-header")).toBeTruthy();
    expect(screen.getByText("Access denied")).toBeTruthy();
    expect(screen.getByText("You don’t have permission to view this page.")).toBeTruthy();
    expect(screen.queryByText(/This is a/i)).toBeNull();
  });

  it("renders a retryable failure for any other status and uses the detail", () => {
    const onRetry = vi.fn();
    renderWithI18n(
      <LoadFailure error={{ status: 500, detail: "upstream exploded" }} feature="Storefront products" header={header} onRetry={onRetry} />,
    );

    expect(screen.getByTestId("page-header")).toBeTruthy();
    expect(screen.getByText("upstream exploded")).toBeTruthy();

    fireEvent.click(screen.getByRole("button", { name: /retry/i }));
    expect(onRetry).toHaveBeenCalledOnce();
  });

  it("derives HTTP <status> when the error carries no detail", () => {
    renderWithI18n(<LoadFailure error={{ status: 503 }} feature="Storefront products" />);
    expect(screen.getByText("HTTP 503")).toBeTruthy();
  });

  it("shows the title once when there is nothing to add", () => {
    renderWithI18n(<LoadFailure error={{ unexpected: true }} feature="Storefront products" />);
    expect(screen.getAllByText("Failed to load data")).toHaveLength(1);
  });

  it("accepts an explicit status for callers that only kept the number", () => {
    renderWithI18n(<LoadFailure status={403} feature="Storefront products" />);
    expect(screen.getByText("Access denied")).toBeTruthy();
  });

  it("prefers an explicit errorText over the error detail", () => {
    renderWithI18n(
      <LoadFailure error={{ status: 500, detail: "raw" }} errorText="Readable" feature="Storefront products" />,
    );
    expect(screen.getByText("Readable")).toBeTruthy();
    expect(screen.queryByText("raw")).toBeNull();
  });

  it("honours denied copy overrides", () => {
    renderWithI18n(
      <LoadFailure
        status={403}
        feature="Storefront products"
        deniedTitle="Workspace admin only"
        deniedBody="Ask the workspace owner."
      />,
    );
    expect(screen.getByText("Workspace admin only")).toBeTruthy();
    expect(screen.getByText("Ask the workspace owner.")).toBeTruthy();
  });
});

describe("AccessDenied", () => {
  afterEach(() => {
    cleanup();
  });

  it("renders the shared copy by default", () => {
    renderWithI18n(<AccessDenied />);
    expect(screen.getByText("Access denied")).toBeTruthy();
    expect(screen.getByText("You don’t have permission to view this page.")).toBeTruthy();
  });
});
