import { describe, expect, it } from "vitest";
import { render, screen } from "@testing-library/react";
import { I18nProvider } from "../../i18n";
import { TableEmpty } from "./table-empty";

const resources = {
  en: {
    proTable: {
      empty: "No data yet",
      emptyReason: "No matching data found for the current filters",
    },
  },
};

function renderWithI18n(ui: React.ReactElement) {
  return render(<I18nProvider resources={resources}>{ui}</I18nProvider>);
}

describe("TableEmpty", () => {
  it("renders the host-supplied default copy", () => {
    renderWithI18n(<TableEmpty />);
    expect(screen.getByText("No data yet")).toBeTruthy();
    expect(screen.getByText("No matching data found for the current filters")).toBeTruthy();
  });

  it("draws its own glyph rather than depending on an icon library", () => {
    const { container } = renderWithI18n(<TableEmpty />);
    expect(container.querySelector("svg")).toBeTruthy();
  });

  it("lets the caller override the glyph", () => {
    const { container } = renderWithI18n(<TableEmpty icon={<span data-testid="custom-glyph" />} />);
    expect(screen.getByTestId("custom-glyph")).toBeTruthy();
    expect(container.querySelector("svg")).toBeNull();
  });

  it("renders custom emptyText and emptyReason", () => {
    renderWithI18n(<TableEmpty emptyText="Custom Empty Text" emptyReason="Custom Empty Reason" />);
    expect(screen.getByText("Custom Empty Text")).toBeTruthy();
    expect(screen.getByText("Custom Empty Reason")).toBeTruthy();
  });

  it("renders custom action", () => {
    renderWithI18n(<TableEmpty action={<button>Retry</button>} />);
    expect(screen.getByRole("button", { name: "Retry" })).toBeTruthy();
  });
});
