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
    vi.spyOn(api, "modelPrices").mockResolvedValue([
      { model: "qwen-flash", displayName: "Qwen Flash", providerId: "alibaba-qwen", input: 0.05, output: 0.4, reasoning: 0.4, cacheRead: 0.01, cacheWrite: 0.02 },
    ]);
    vi.spyOn(api, "llmBackends").mockResolvedValue(["openai", "ollama"]);

    renderGateway();

    expect(await screen.findByText("Production")).toBeTruthy();
    expect(screen.getByText("Coding agents")).toBeTruthy();
    expect(screen.getByText(`${window.location.origin}/v1`)).toBeTruthy();
    expect(screen.getByText("GET /v1/models")).toBeTruthy();
    expect(screen.getByText("POST /v1/messages")).toBeTruthy();
    expect(screen.getByText("GET /v1beta/models")).toBeTruthy();
    expect(screen.getByText("POST /v1beta/models/{model}:generateContent")).toBeTruthy();
  });

  it("lists the bundled prices and offers no editor", async () => {
    vi.spyOn(api, "llmProviders").mockResolvedValue([]);
    vi.spyOn(api, "modelPrices").mockResolvedValue([
      { model: "qwen-flash", displayName: "Qwen Flash", providerId: "alibaba-qwen", input: 0.05, output: 0.4, reasoning: 0.4, cacheRead: 0.01, cacheWrite: 0.02 },
    ]);
    vi.spyOn(api, "llmBackends").mockResolvedValue(["openai"]);

    renderGateway();

    expect(await screen.findByText("alibaba-qwen/qwen-flash")).toBeTruthy();
    expect(screen.getByText("Qwen Flash")).toBeTruthy();
    expect(screen.getByRole("columnheader", { name: "Model" })).toBeTruthy();
    expect(screen.getByRole("columnheader", { name: "Cache read" })).toBeTruthy();
    expect(screen.getByRole("columnheader", { name: "Cache write" })).toBeTruthy();
    expect(screen.getByText("0.05")).toBeTruthy();
    expect(screen.getAllByText("0.4")).toHaveLength(2);
    expect(screen.queryByRole("button", { name: "Add price" })).toBeNull();
    expect(screen.queryByRole("button", { name: /price/i })).toBeNull();
  });

  it("shows the cache read and cache write prices when the catalog carries them", async () => {
    vi.spyOn(api, "llmProviders").mockResolvedValue([]);
    vi.spyOn(api, "modelPrices").mockResolvedValue([
      {
        model: "qwen-flash",
        displayName: "Qwen Flash",
        providerId: "alibaba-qwen",
        input: 0.05,
        output: 0.4,
        reasoning: 0.4,
        cacheRead: 0.5,
        cacheWrite: 6.25,
      },
    ]);
    vi.spyOn(api, "llmBackends").mockResolvedValue(["openai"]);

    renderGateway();

    expect(await screen.findByText("alibaba-qwen/qwen-flash")).toBeTruthy();
    expect(screen.getByText("0.5")).toBeTruthy();
    expect(screen.getByText("6.25")).toBeTruthy();
  });

  it("filters the price list by free text", async () => {
    vi.spyOn(api, "llmProviders").mockResolvedValue([]);
    vi.spyOn(api, "modelPrices").mockResolvedValue([
      { model: "qwen-flash", displayName: "Qwen Flash", providerId: "alibaba-qwen", input: 0.05, output: 0.4, reasoning: 0.4, cacheRead: 0.01, cacheWrite: 0.02 },
      { model: "gpt-4o", displayName: "GPT-4o", providerId: "openai", input: 2.5, output: 10, reasoning: 10, cacheRead: 1.25, cacheWrite: 5 },
    ]);
    vi.spyOn(api, "llmBackends").mockResolvedValue(["openai"]);

    renderGateway();

    expect(await screen.findByText("alibaba-qwen/qwen-flash")).toBeTruthy();
    expect(screen.getByText("openai/gpt-4o")).toBeTruthy();

    fireEvent.change(screen.getByLabelText("Search models"), { target: { value: "gpt" } });
    await waitFor(() => expect(screen.queryByText("alibaba-qwen/qwen-flash")).toBeNull());
    expect(screen.getByText("openai/gpt-4o")).toBeTruthy();
    expect(screen.getByText("1 of 2 models")).toBeTruthy();
    expect(screen.getByLabelText("Provider")).toBeTruthy();

    fireEvent.change(screen.getByLabelText("Search models"), { target: { value: "nothing-matches" } });
    expect(await screen.findByText("No model matches the filter.")).toBeTruthy();
  });

  it("paginates the price list", async () => {
    const rows = Array.from({ length: 25 }, (_, i) => ({
      model: `model-${String(i + 1).padStart(2, "0")}`,
      displayName: "",
      providerId: "openai",
      input: 1,
      output: 2,
      reasoning: 2,
      cacheRead: 0.5,
      cacheWrite: 1,
    }));
    vi.spyOn(api, "llmProviders").mockResolvedValue([]);
    vi.spyOn(api, "modelPrices").mockResolvedValue(rows);
    vi.spyOn(api, "llmBackends").mockResolvedValue(["openai"]);

    renderGateway();

    expect(await screen.findByText("openai/model-01")).toBeTruthy();
    expect(screen.queryByText("openai/model-21")).toBeNull();

    fireEvent.click(screen.getByRole("button", { name: "2" }));
    expect(await screen.findByText("openai/model-21")).toBeTruthy();
    expect(screen.queryByText("openai/model-01")).toBeNull();
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
