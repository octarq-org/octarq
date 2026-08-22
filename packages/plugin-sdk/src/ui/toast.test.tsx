import { describe, it, expect } from "vitest";
import { render, screen } from "@testing-library/react";
import { ToastProvider, useToast, toast, Toaster } from "./toast";

describe("Toast with sonner", () => {
  it("exports toast API with success, error, info, warning", () => {
    expect(typeof toast).toBe("function");
    expect(typeof toast.success).toBe("function");
    expect(typeof toast.error).toBe("function");
    expect(typeof toast.info).toBe("function");
    expect(typeof toast.warning).toBe("function");
  });

  it("useToast returns toast object", () => {
    function TestComponent() {
      const t = useToast();
      expect(t).toBe(toast);
      return <div>Toast Ready</div>;
    }
    render(
      <ToastProvider>
        <TestComponent />
      </ToastProvider>,
    );
    expect(screen.getByText("Toast Ready")).toBeDefined();
  });

  it("renders ToastProvider and Toaster without throwing", () => {
    const { container } = render(
      <ToastProvider position="top-right">
        <div>Child Component</div>
      </ToastProvider>,
    );
    expect(screen.getByText("Child Component")).toBeDefined();
    expect(container).toBeDefined();
  });
});
