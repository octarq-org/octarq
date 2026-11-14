// @vitest-environment happy-dom
import { afterEach, describe, expect, it, vi } from "vitest";
import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { api } from "../../api";
import { I18nProvider } from "../../i18n";
import { GatewaySettings } from "./gateway";

const provider = {
  id: 1,
  name: "Production",
  backend: "openai",
  baseUrl: "https://proxy.example/v1",
  model: "gpt-selected",
  cheapModel: "",
  isDefault: true,
  hasKey: true,
  createdAt: "",
  updatedAt: "",
};

function renderGateway() {
  return render(
    <I18nProvider>
      <GatewaySettings />
    </I18nProvider>,
  );
}

afterEach(() => {
  cleanup();
  vi.restoreAllMocks();
});

describe("GatewaySettings", () => {
  it("offers the agent endpoint and the workspace models", async () => {
    vi.spyOn(api, "llmProviders").mockResolvedValue([provider]);
    vi.spyOn(api, "modelPrices").mockResolvedValue([]);
    vi.spyOn(api, "llmBackends").mockResolvedValue(["openai", "ollama"]);

    renderGateway();

    expect(await screen.findByText("Production")).toBeTruthy();
    expect(screen.getByText("Coding agents")).toBeTruthy();
    expect(screen.getByText(`${window.location.origin}/v1`)).toBeTruthy();
  });

  it("reads the model list from the provider when the form opens", async () => {
    vi.spyOn(api, "llmProviders").mockResolvedValue([provider]);
    vi.spyOn(api, "modelPrices").mockResolvedValue([]);
    vi.spyOn(api, "llmBackends").mockResolvedValue(["openai", "ollama"]);
    const discover = vi
      .spyOn(api, "discoverModels")
      .mockResolvedValue([{ id: "gpt-selected" }, { id: "gpt-other" }]);

    renderGateway();
    fireEvent.click(await screen.findByRole("button", { name: "Edit" }));

    await waitFor(() => expect(discover).toHaveBeenCalled());
    expect(discover.mock.calls[0][0]).toEqual({
      providerId: 1,
      backend: "openai",
      baseUrl: "https://proxy.example/v1",
    });
    // The stored model stays selected, so an edit does not drop it.
    expect(await screen.findByText("gpt-selected")).toBeTruthy();
  });

  it("states the failure when the provider lists nothing", async () => {
    vi.spyOn(api, "llmProviders").mockResolvedValue([provider]);
    vi.spyOn(api, "modelPrices").mockResolvedValue([]);
    vi.spyOn(api, "llmBackends").mockResolvedValue(["openai"]);
    vi.spyOn(api, "discoverModels").mockRejectedValue(new Error("upstream"));

    renderGateway();
    fireEvent.click(await screen.findByRole("button", { name: "Edit" }));

    expect(
      await screen.findByText(
        "The provider did not list its models. Check the backend, the base URL and the API key.",
      ),
    ).toBeTruthy();
  });

  it("cannot load models before a backend is chosen", async () => {
    vi.spyOn(api, "llmProviders").mockResolvedValue([]);
    vi.spyOn(api, "modelPrices").mockResolvedValue([]);
    vi.spyOn(api, "llmBackends").mockResolvedValue(["openai"]);

    renderGateway();
    fireEvent.click(await screen.findByRole("button", { name: "Add provider" }));

    const load = screen.getByRole("button", { name: "Load models" }) as HTMLButtonElement;
    expect(load.disabled).toBe(true);
  });
});
