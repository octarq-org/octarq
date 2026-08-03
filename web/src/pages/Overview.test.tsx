// @vitest-environment jsdom
import { act, lazy } from "react";
import { createRoot, Root } from "react-dom/client";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { SetupStep, SetupChecklistProvider, useSetupChecklist } from "../components/SetupStep";
import { ExtensionSlot, registerUIPlugin, resetRegistry, LazyPage } from "../plugin-sdk";

// @ts-ignore
globalThis.IS_REACT_ACT_ENVIRONMENT = true;

let container: HTMLDivElement | null = null;
let root: Root | null = null;

beforeEach(() => {
  container = document.createElement("div");
  document.body.appendChild(container);
  root = createRoot(container);
  resetRegistry();
});

afterEach(() => {
  if (root) {
    act(() => {
      root?.unmount();
    });
  }
  if (container) {
    container.remove();
  }
  container = null;
  root = null;
  resetRegistry();
  vi.restoreAllMocks();
});

const lazyWidget = (title: string, completed: boolean): LazyPage =>
  lazy(async () => ({
    default: () => (
      <SetupStep
        title={title}
        description={`${title} desc`}
        completed={completed}
        onClick={() => {}}
      />
    ),
  }));

function TestChecklistSummary() {
  const { totalCount, completedCount, progressPercent, allCompleted } = useSetupChecklist();
  if (totalCount === 0) {
    return <div data-testid="summary">empty</div>;
  }
  return (
    <div data-testid="summary">
      <span data-testid="progress">{progressPercent}%</span>
      <span data-testid="counts">{completedCount}/{totalCount}</span>
      <span data-testid="all-completed">{allCompleted ? "yes" : "no"}</span>
    </div>
  );
}

describe("Setup Checklist & Overview Progress", () => {
  it("calculates progress percentage correctly for N steps with M completed (M/N)", async () => {
    await act(async () => {
      root?.render(
        <SetupChecklistProvider>
          <TestChecklistSummary />
          <SetupStep title="Step 1" description="Desc 1" completed={true} onClick={() => {}} />
          <SetupStep title="Step 2" description="Desc 2" completed={false} onClick={() => {}} />
          <SetupStep title="Step 3" description="Desc 3" completed={false} onClick={() => {}} />
        </SetupChecklistProvider>
      );
    });

    const progress = container?.querySelector('[data-testid="progress"]');
    const counts = container?.querySelector('[data-testid="counts"]');
    const allCompleted = container?.querySelector('[data-testid="all-completed"]');

    expect(progress?.textContent).toBe("33%");
    expect(counts?.textContent).toBe("1/3");
    expect(allCompleted?.textContent).toBe("no");
  });

  it("counts steps rendered via ExtensionSlot towards total progress", async () => {
    registerUIPlugin({
      name: "test-plugin",
      routes: [],
      widgets: [
        {
          slot: "home-setup-steps",
          Component: lazyWidget("Plugin Step", false),
          order: 1,
        },
      ],
    });

    await act(async () => {
      root?.render(
        <SetupChecklistProvider>
          <TestChecklistSummary />
          <SetupStep title="2FA" description="2FA desc" completed={true} onClick={() => {}} />
          <ExtensionSlot name="home-setup-steps" />
        </SetupChecklistProvider>
      );
    });

    await act(async () => {
      await new Promise((resolve) => setTimeout(resolve, 50));
    });

    const progress = container?.querySelector('[data-testid="progress"]');
    const counts = container?.querySelector('[data-testid="counts"]');
    const allCompleted = container?.querySelector('[data-testid="all-completed"]');

    expect(progress?.textContent).toBe("50%");
    expect(counts?.textContent).toBe("1/2");
    expect(allCompleted?.textContent).toBe("no");
  });

  it("does not render NaN% or display checklist when zero steps are registered", async () => {
    await act(async () => {
      root?.render(
        <SetupChecklistProvider>
          <TestChecklistSummary />
        </SetupChecklistProvider>
      );
    });

    const summary = container?.querySelector('[data-testid="summary"]');
    const progress = container?.querySelector('[data-testid="progress"]');

    expect(summary?.textContent).toBe("empty");
    expect(progress).toBeNull();
  });
});
