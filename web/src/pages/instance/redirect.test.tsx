// @vitest-environment happy-dom
import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, waitFor } from "@testing-library/react";
import { MemoryRouter, Route, Routes } from "react-router-dom";
import { InstanceExitRedirect } from "./redirect";

function renderRedirect(entry: string, props: { base?: string; to?: string } = {}) {
  render(
    <MemoryRouter initialEntries={[entry]}>
      <Routes>
        <Route
          path="/settings/*"
          element={
            <Routes>
              <Route path="/instance/*" element={<InstanceExitRedirect {...props} />} />
              <Route path="/auth" element={<InstanceExitRedirect {...props} />} />
            </Routes>
          }
        />
      </Routes>
    </MemoryRouter>,
  );
}

describe("InstanceExitRedirect", () => {
  beforeEach(() => {
    vi.restoreAllMocks();
  });

  it("sends /settings/instance to the console root", async () => {
    const replace = vi.fn();
    Object.defineProperty(window, "location", { value: { replace }, writable: true });

    renderRedirect("/settings/instance");

    await waitFor(() => expect(replace).toHaveBeenCalledWith("/instance"));
  });

  it("sends /settings/instance/auth to /instance/auth", async () => {
    const replace = vi.fn();
    Object.defineProperty(window, "location", { value: { replace }, writable: true });

    renderRedirect("/settings/instance/auth");

    await waitFor(() => expect(replace).toHaveBeenCalledWith("/instance/auth"));
  });

  it("sends /settings/auth to /instance/auth via the explicit target", async () => {
    const replace = vi.fn();
    Object.defineProperty(window, "location", { value: { replace }, writable: true });

    renderRedirect("/settings/auth", { to: "/instance/auth" });

    await waitFor(() => expect(replace).toHaveBeenCalledWith("/instance/auth"));
  });
});
