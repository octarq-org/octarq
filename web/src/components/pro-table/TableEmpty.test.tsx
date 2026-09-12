// @vitest-environment happy-dom
import { describe, it, expect } from "vitest";
import { render, screen } from "@testing-library/react";
import { TableEmpty } from "./TableEmpty";
import { I18nProvider } from "../../i18n";

function renderWithProviders(ui: React.ReactElement) {
  return render(<I18nProvider>{ui}</I18nProvider>);
}

describe("TableEmpty", () => {
  it("renders with default props and translations", () => {
    renderWithProviders(<TableEmpty />);
    // Just ensuring it renders without crashing for now, will check actual output next
    expect(document.querySelector(".lucide-inbox")).toBeTruthy();
  });

  it("renders custom emptyText", () => {
    renderWithProviders(<TableEmpty emptyText="Custom Empty Text" />);
    expect(screen.getByText("Custom Empty Text")).toBeTruthy();
  });

  it("renders custom emptyReason", () => {
    renderWithProviders(<TableEmpty emptyReason="Custom Empty Reason" />);
    expect(screen.getByText("Custom Empty Reason")).toBeTruthy();
  });

  it("renders custom action", () => {
    renderWithProviders(<TableEmpty action={<button>Retry</button>} />);
    expect(screen.getByRole("button", { name: "Retry" })).toBeTruthy();
  });
});
