import React, { useState } from "react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { cleanup, render, screen, fireEvent } from "@testing-library/react";
import { SetupStep, SetupChecklistProvider, useSetupChecklist } from "./setup-step";

afterEach(() => {
  cleanup();
});

describe("SetupStep", () => {
  it("renders title, description, and pending circle when completed is false", () => {
    const onClick = vi.fn();
    render(
      <SetupStep
        title="Verify domain"
        description="Add TXT record to domain"
        completed={false}
        onClick={onClick}
      />,
    );

    expect(screen.getByText("Verify domain")).toBeDefined();
    expect(screen.getByText("Add TXT record to domain")).toBeDefined();
    const btn = screen.getByRole("button");
    expect(btn.className).toContain("border-border");

    fireEvent.click(btn);
    expect(onClick).toHaveBeenCalledTimes(1);
  });

  it("renders completed styles and checkmark icon when completed is true", () => {
    render(
      <SetupStep
        title="Create link"
        description="First link created"
        completed={true}
        onClick={() => {}}
      />,
    );

    const title = screen.getByText("Create link");
    expect(title.className).toContain("line-through");
    expect(title.className).toContain("text-muted-foreground");

    const btn = screen.getByRole("button");
    expect(btn.className).toContain("border-success-border/50");
  });
});

describe("SetupChecklistProvider & useSetupChecklist", () => {
  function TestConsumer() {
    const { totalCount, completedCount, progressPercent, allCompleted } = useSetupChecklist();
    return (
      <div>
        <span data-testid="total">{totalCount}</span>
        <span data-testid="completed">{completedCount}</span>
        <span data-testid="percent">{progressPercent}</span>
        <span data-testid="all">{allCompleted ? "yes" : "no"}</span>
      </div>
    );
  }

  it("returns zeros and allCompleted=false when outside provider", () => {
    render(<TestConsumer />);
    expect(screen.getByTestId("total").textContent).toBe("0");
    expect(screen.getByTestId("completed").textContent).toBe("0");
    expect(screen.getByTestId("percent").textContent).toBe("0");
    expect(screen.getByTestId("all").textContent).toBe("no");
  });

  it("registers child steps and dynamically tracks progress", () => {
    function Checklist() {
      const [step1Done, setStep1Done] = useState(true);
      const [step2Done, setStep2Done] = useState(false);
      return (
        <SetupChecklistProvider>
          <TestConsumer />
          <SetupStep
            title="Step 1"
            description="Desc 1"
            completed={step1Done}
            onClick={() => setStep1Done(!step1Done)}
          />
          <SetupStep
            title="Step 2"
            description="Desc 2"
            completed={step2Done}
            onClick={() => setStep2Done(!step2Done)}
          />
        </SetupChecklistProvider>
      );
    }

    render(<Checklist />);
    expect(screen.getByTestId("total").textContent).toBe("2");
    expect(screen.getByTestId("completed").textContent).toBe("1");
    expect(screen.getByTestId("percent").textContent).toBe("50");
    expect(screen.getByTestId("all").textContent).toBe("no");

    fireEvent.click(screen.getByText("Step 2"));
    expect(screen.getByTestId("completed").textContent).toBe("2");
    expect(screen.getByTestId("percent").textContent).toBe("100");
    expect(screen.getByTestId("all").textContent).toBe("yes");
  });

  it("unregisters steps on unmount", () => {
    function ToggleChecklist() {
      const [showSecond, setShowSecond] = useState(true);
      return (
        <SetupChecklistProvider>
          <TestConsumer />
          <button onClick={() => setShowSecond(false)}>Remove 2</button>
          <SetupStep title="Step 1" description="D1" completed={true} onClick={() => {}} />
          {showSecond && (
            <SetupStep title="Step 2" description="D2" completed={false} onClick={() => {}} />
          )}
        </SetupChecklistProvider>
      );
    }

    render(<ToggleChecklist />);
    expect(screen.getByTestId("total").textContent).toBe("2");
    expect(screen.getByTestId("percent").textContent).toBe("50");

    fireEvent.click(screen.getByText("Remove 2"));
    expect(screen.getByTestId("total").textContent).toBe("1");
    expect(screen.getByTestId("completed").textContent).toBe("1");
    expect(screen.getByTestId("percent").textContent).toBe("100");
    expect(screen.getByTestId("all").textContent).toBe("yes");
  });
});
