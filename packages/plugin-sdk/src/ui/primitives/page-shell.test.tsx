import React from "react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import {
  PageCard,
  PageEmpty,
  PageRow,
  PageRows,
  PageSection,
  PageShell,
} from "./page-shell";
import { Separator } from "./separator";

afterEach(cleanup);

describe("PageShell layout system", () => {
  it("renders prose width (max-w-3xl) by default with header, section, rows and inset separator", () => {
    const { container } = render(
      <PageShell title="Appearance" description="Theme and display preferences.">
        <PageSection title="Interface">
          <PageRows>
            <PageRow title="Theme" description="Light or dark" control={<button>Light</button>} />
            <Separator />
            <PageRow title="Language" control={<button>English</button>} />
          </PageRows>
        </PageSection>
      </PageShell>,
    );

    expect(screen.getByRole("heading", { level: 1, name: "Appearance" })).toBeDefined();
    expect(screen.getByText("Theme and display preferences.")).toBeDefined();
    expect(screen.getByRole("heading", { level: 2, name: "Interface" })).toBeDefined();
    expect(screen.getByText("Theme")).toBeDefined();
    expect(screen.getByText("Language")).toBeDefined();

    const column = container.querySelector(".max-w-3xl");
    expect(column).not.toBeNull();

    const rowsCard = container.querySelector('[data-slot="page-rows"]');
    expect(rowsCard?.className).toContain("bg-card");
    expect(rowsCard?.className).toContain("border-border");
    expect(rowsCard?.className).toContain("[&_[data-slot=page-row]]:min-h-[var(--row-height,3.5rem)]");
    expect(rowsCard?.className).toContain("[&_[data-slot=separator][data-orientation=horizontal]]:mx-4");

    const sep = container.querySelector('[data-slot="separator"]');
    expect(sep?.getAttribute("data-orientation")).toBe("horizontal");
  });

  it("supports full width without max-w-3xl constraint", () => {
    const { container } = render(
      <PageShell title="Audit Log" width="full">
        <div>Table content</div>
      </PageShell>,
    );
    expect(container.querySelector(".max-w-3xl")).toBeNull();
    expect(screen.getByText("Table content")).toBeDefined();
  });

  it("renders interactive PageRow as a button when onClick is provided", () => {
    const onClick = vi.fn();
    render(
      <PageRows>
        <PageRow title="Edit Webhook" onClick={onClick} />
      </PageRows>,
    );
    const btn = screen.getByRole("button", { name: /Edit Webhook/i });
    fireEvent.click(btn);
    expect(onClick).toHaveBeenCalledTimes(1);
  });

  it("renders PageCard with footer and PageEmpty", () => {
    render(
      <>
        <PageCard footer={<button>Save</button>}>
          <input aria-label="Name" />
        </PageCard>
        <PageEmpty title="Nothing here" detail="Add your first item." action={<button>Add</button>} />
      </>,
    );
    expect(screen.getByLabelText("Name")).toBeDefined();
    expect(screen.getByRole("button", { name: "Save" })).toBeDefined();
    expect(screen.getByText("Nothing here")).toBeDefined();
    expect(screen.getByText("Add your first item.")).toBeDefined();
    expect(screen.getByRole("button", { name: "Add" })).toBeDefined();
  });
});
