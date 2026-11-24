// @vitest-environment happy-dom
import React from "react";
import { render, screen, fireEvent, cleanup } from "@testing-library/react";
import { describe, expect, it, afterEach, vi } from "vitest";
import { I18nProvider } from "../../i18n";
import { CanvasApprovalGate } from "./CanvasApprovalGate";

function renderGate(props: Partial<React.ComponentProps<typeof CanvasApprovalGate>> = {}) {
  const onAction = props.onAction ?? vi.fn(async () => ({ status: 200 }));
  const utils = render(
    <I18nProvider>
      <CanvasApprovalGate
        approvalId="appr-1"
        actionLabel="Approve change"
        onAction={onAction}
        {...props}
      />
    </I18nProvider>,
  );
  return { ...utils, onAction };
}

describe("CanvasApprovalGate", () => {
  afterEach(() => {
    cleanup();
  });

  it("blocks submission when the approval ID is missing", () => {
    const { onAction } = renderGate({ approvalId: "" });

    expect(screen.getByTestId("approval-missing-id")).toBeDefined();
    fireEvent.click(screen.getByTestId("gate-approve-btn"));

    expect(onAction).not.toHaveBeenCalled();
  });

  it("submits an approve action with the approval token", async () => {
    const { onAction } = renderGate({ approvalToken: "tok-1" });
    fireEvent.click(screen.getByTestId("gate-approve-btn"));

    expect(onAction).toHaveBeenCalledWith(
      expect.objectContaining({ type: "approve", approvalId: "appr-1", approvalToken: "tok-1" }),
    );
    expect(await screen.findByTestId("gate-terminal")).toBeDefined();
    expect(screen.getByTestId("gate-terminal").getAttribute("data-verdict")).toBe("decided");
  });

  it("treats a 409 response as already decided", async () => {
    renderGate({ onAction: async () => ({ status: 409 }) });
    fireEvent.click(screen.getByTestId("gate-reject-btn"));

    const terminal = await screen.findByTestId("gate-terminal");
    expect(terminal.getAttribute("data-verdict")).toBe("decided");
  });

  it("treats a transport error as failed without claiming a change", async () => {
    renderGate({
      onAction: async () => {
        throw new Error("network offline");
      },
    });
    fireEvent.click(screen.getByTestId("gate-approve-btn"));

    const terminal = await screen.findByTestId("gate-terminal");
    expect(terminal.getAttribute("data-verdict")).toBe("failed");
    expect(screen.getByText(/network offline/)).toBeDefined();
  });
});
