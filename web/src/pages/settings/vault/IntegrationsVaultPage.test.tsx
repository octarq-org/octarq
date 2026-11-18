// @vitest-environment happy-dom
import { afterEach, describe, expect, it, vi } from "vitest";
import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { MemoryRouter } from "react-router-dom";
import * as ui from "@octarq/plugin-sdk";
import { api } from "../../../api";
import type {
  VaultCategorySchema,
  VaultCollection,
  VaultItemMeta,
  VaultServiceToken,
} from "../../../api/types";
import { ConfirmBridge } from "../../../ConfirmBridge";
import { I18nProvider } from "../../../i18n";
import { RoleProvider } from "../../../shell/role";
import { IntegrationsVaultPage } from "./IntegrationsVaultPage";

const mockSchemas: VaultCategorySchema[] = [
  {
    key: "llm",
    label: "AI / LLM Providers",
    itemType: "integration",
    providers: [
      {
        id: "openai",
        label: "OpenAI",
        itemType: "integration",
        providerType: "llm",
        testable: true,
        fields: [
          { key: "apiKey", label: "API Key", kind: "secret", required: true, secret: true },
          { key: "model", label: "Default Model", kind: "text", nonSecret: true },
        ],
      },
    ],
  },
  {
    key: "communication",
    label: "Communication Channels",
    itemType: "integration",
    providers: [
      {
        id: "telegram",
        label: "Telegram Bot",
        itemType: "integration",
        providerType: "communication",
        testable: true,
        fields: [
          { key: "botToken", label: "Bot Token", kind: "secret", required: true, secret: true },
        ],
      },
    ],
  },
  {
    key: "secrets",
    label: "Secrets & Infrastructure",
    itemType: "kv_secret",
    providers: [
      {
        id: "kv_secret",
        label: "Key-Value Secret",
        itemType: "kv_secret",
        fields: [
          { key: "value", label: "Secret Value", kind: "secret", required: true, secret: true },
        ],
      },
    ],
  },
  {
    key: "vault_items",
    label: "Logins, 2FA & Notes",
    itemType: "login",
    providers: [
      {
        id: "twofa_totp",
        label: "2FA Authenticator (TOTP)",
        itemType: "twofa_totp",
        testable: true,
        fields: [
          { key: "issuer", label: "Issuer", kind: "text", nonSecret: true },
          { key: "secret", label: "TOTP Secret (Base32)", kind: "secret", secret: true },
        ],
      },
    ],
  },
];

const mockCollections: VaultCollection[] = [
  {
    id: 1,
    orgId: 1,
    scope: "org",
    slug: "integrations",
    name: "Service Integrations",
    kind: "integration",
    createdAt: "2026-01-01T00:00:00Z",
    updatedAt: "2026-01-01T00:00:00Z",
  },
  {
    id: 2,
    orgId: 1,
    scope: "org",
    slug: "default",
    name: "Default Vault",
    kind: "vault",
    createdAt: "2026-01-01T00:00:00Z",
    updatedAt: "2026-01-01T00:00:00Z",
  },
];

const mockItems: VaultItemMeta[] = [
  {
    id: 10,
    secretId: "sec-openai",
    orgId: 1,
    scope: "org",
    collectionId: 1,
    collectionSlug: "integrations",
    environment: "prod",
    folderPath: "/",
    itemType: "integration",
    providerType: "llm",
    provider: "openai",
    keyName: "OPENAI_PROD",
    name: "Prod OpenAI",
    isDefault: false,
    currentVersion: 2,
    configured: true,
    fieldKeys: ["apiKey", "model"],
    metadata: { model: "gpt-4o" },
    ref: "vault://integrations/prod/OPENAI_PROD",
    createdAt: "2026-01-01T00:00:00Z",
    updatedAt: "2026-01-02T00:00:00Z",
  },
  {
    id: 11,
    secretId: "sec-tg",
    orgId: 1,
    scope: "org",
    collectionId: 1,
    collectionSlug: "integrations",
    environment: "default",
    folderPath: "/",
    itemType: "integration",
    providerType: "communication",
    provider: "telegram",
    keyName: "OPS_TG",
    name: "Ops Telegram",
    isDefault: true,
    currentVersion: 1,
    configured: true,
    fieldKeys: ["botToken"],
    ref: "vault://integrations/default/OPS_TG",
    createdAt: "2026-01-01T00:00:00Z",
    updatedAt: "2026-01-01T00:00:00Z",
  },
  {
    id: 12,
    secretId: "sec-totp",
    orgId: 1,
    scope: "org",
    collectionId: 2,
    collectionSlug: "default",
    environment: "default",
    folderPath: "/",
    itemType: "twofa_totp",
    name: "GitHub 2FA",
    isDefault: false,
    currentVersion: 1,
    configured: true,
    fieldKeys: ["issuer", "secret"],
    metadata: { issuer: "GitHub" },
    ref: "vault://id/12",
    createdAt: "2026-01-01T00:00:00Z",
    updatedAt: "2026-01-01T00:00:00Z",
  },
];

const mockTokens: VaultServiceToken[] = [
  {
    id: 50,
    orgId: 1,
    name: "CI Pipeline",
    tokenPrefix: "octvq_abc12345",
    collectionId: 2,
    environment: "prod",
    folderPrefix: "/",
    permissions: "read",
    createdAt: "2026-01-01T00:00:00Z",
  },
];

function renderVaultPage(initialEntry = "/settings/integrations") {
  return render(
    <MemoryRouter initialEntries={[initialEntry]}>
      <I18nProvider>
        <RoleProvider value={{ role: "owner", isInstanceAdmin: true }}>
          <ConfirmBridge>
            <IntegrationsVaultPage />
          </ConfirmBridge>
        </RoleProvider>
      </I18nProvider>
    </MemoryRouter>,
  );
}

afterEach(() => {
  cleanup();
  vi.restoreAllMocks();
});

describe("IntegrationsVaultPage", () => {
  it("supports ?category=communication query param and filters items", async () => {
    vi.spyOn(api, "vaultSchemas").mockResolvedValue(mockSchemas);
    vi.spyOn(api, "vaultCollections").mockResolvedValue(mockCollections);
    vi.spyOn(api, "vaultItems").mockResolvedValue(mockItems);

    renderVaultPage("/settings/integrations?category=communication");

    expect(await screen.findByText("Ops Telegram")).toBeTruthy();
    expect(screen.queryByText("Prod OpenAI")).toBeNull();
    expect(screen.queryByText("GitHub 2FA")).toBeNull();
  });

  it("renders all items, reveals secret, shows TOTP code, tests, sets default, and rolls back version", async () => {
    vi.spyOn(api, "vaultSchemas").mockResolvedValue(mockSchemas);
    vi.spyOn(api, "vaultCollections").mockResolvedValue(mockCollections);
    vi.spyOn(api, "vaultItems").mockResolvedValue(mockItems);
    const revealSpy = vi.spyOn(api, "revealVaultItem").mockResolvedValue({
      meta: mockItems[0],
      version: 2,
      payload: { apiKey: "sk-revealed-999", model: "gpt-4o" },
    });
    const totpSpy = vi.spyOn(api, "vaultItemTOTP").mockResolvedValue({
      code: "482910",
      remainingSeconds: 22,
    });
    const testSpy = vi.spyOn(api, "testVaultItem").mockResolvedValue({
      ok: true,
      message: "LLM provider credentials verified.",
    });
    const defaultSpy = vi.spyOn(api, "setDefaultVaultItem").mockResolvedValue({
      ...mockItems[0],
      isDefault: true,
    });
    vi.spyOn(api, "vaultItemVersions").mockResolvedValue([
      { version: 2, changeNote: "Rotated key", createdAt: "2026-01-02T00:00:00Z" },
      { version: 1, changeNote: "Initial", createdAt: "2026-01-01T00:00:00Z" },
    ]);
    const rollbackSpy = vi.spyOn(api, "rollbackVaultItem").mockResolvedValue({
      ...mockItems[0],
      currentVersion: 3,
    });
    vi.spyOn(ui, "confirmDialog").mockResolvedValue(true);
    const toastSuccess = vi.spyOn(ui.toast, "success");

    renderVaultPage();

    expect(await screen.findByText("Prod OpenAI")).toBeTruthy();
    expect(screen.getByText("GitHub 2FA")).toBeTruthy();

    // Reveal first item
    const openaiRow = screen.getByTestId("vault-item-10");
    const revealBtn = Array.from(openaiRow.querySelectorAll("button")).find((b) =>
      b.textContent?.includes("Reveal"),
    )!;
    fireEvent.click(revealBtn);
    await waitFor(() => expect(revealSpy).toHaveBeenCalledWith(10));
    expect(await screen.findByText("apiKey")).toBeTruthy();
    fireEvent.click(screen.getAllByRole("button", { name: /show/i })[0]);
    expect(await screen.findByText("sk-revealed-999")).toBeTruthy();
    fireEvent.click(screen.getAllByRole("button", { name: "Close" })[0]);

    // Live TOTP viewer
    const totpRow = screen.getByTestId("vault-item-12");
    const totpBtn = Array.from(totpRow.querySelectorAll("button")).find((b) =>
      b.textContent?.includes("TOTP"),
    )!;
    fireEvent.click(totpBtn);
    await waitFor(() => expect(totpSpy).toHaveBeenCalledWith(12));
    expect(await screen.findByTestId("totp-code-value")).toBeTruthy();
    expect(screen.getByTestId("totp-code-value").textContent).toBe("482910");
    fireEvent.click(screen.getAllByRole("button", { name: "Close" })[0]);

    // Test connection
    const testBtn = Array.from(openaiRow.querySelectorAll("button")).find((b) =>
      b.textContent?.includes("Test"),
    )!;
    fireEvent.click(testBtn);
    await waitFor(() => expect(testSpy).toHaveBeenCalledWith(10));
    expect(toastSuccess).toHaveBeenCalledWith("LLM provider credentials verified.");

    // Set default
    const defaultBtn = Array.from(openaiRow.querySelectorAll("button")).find((b) =>
      b.textContent?.includes("Set default"),
    )!;
    fireEvent.click(defaultBtn);
    await waitFor(() => expect(defaultSpy).toHaveBeenCalledWith(10));

    // Versions & Rollback
    const versionsBtn = Array.from(openaiRow.querySelectorAll("button")).find((b) =>
      b.textContent?.includes("Versions"),
    )!;
    fireEvent.click(versionsBtn);
    expect(await screen.findByText("Initial")).toBeTruthy();
    fireEvent.click(screen.getByRole("button", { name: /rollback/i }));
    await waitFor(() => expect(rollbackSpy).toHaveBeenCalledWith(10, 1));
  });

  it("creates a vault item with dynamic schema fields and custom KV fields", async () => {
    vi.spyOn(api, "vaultSchemas").mockResolvedValue(mockSchemas);
    vi.spyOn(api, "vaultCollections").mockResolvedValue(mockCollections);
    vi.spyOn(api, "vaultItems").mockResolvedValue([]);
    const createSpy = vi.spyOn(api, "createVaultItem").mockResolvedValue(mockItems[0]);

    renderVaultPage();

    const newItemBtns = await screen.findAllByRole("button", { name: /new item/i });
    fireEvent.click(newItemBtns[0]);

    fireEvent.change(screen.getByPlaceholderText("e.g. Production OpenAI"), {
      target: { value: "Staging OpenAI" },
    });

    // Required apiKey field rendered from OpenAI schema
    const passwordInputs = document.querySelectorAll('input[type="password"]');
    fireEvent.change(passwordInputs[0], { target: { value: "sk-new-123" } });

    // Add custom KV field
    fireEvent.click(screen.getByRole("button", { name: /add custom field/i }));
    fireEvent.change(screen.getByPlaceholderText("Field key"), { target: { value: "ORG_ID" } });
    fireEvent.change(screen.getByPlaceholderText("Field value"), { target: { value: "org_abc" } });

    fireEvent.click(screen.getByRole("button", { name: /^create$/i }));

    await waitFor(() => {
      expect(createSpy).toHaveBeenCalledWith(
        expect.objectContaining({
          name: "Staging OpenAI",
          itemType: "integration",
          providerType: "llm",
          provider: "openai",
          payload: expect.objectContaining({
            apiKey: "sk-new-123",
            ORG_ID: "org_abc",
          }),
        }),
      );
    });
  });

  it("imports .env secrets, exports secrets, and manages service tokens", async () => {
    vi.spyOn(api, "vaultSchemas").mockResolvedValue(mockSchemas);
    vi.spyOn(api, "vaultCollections").mockResolvedValue(mockCollections);
    vi.spyOn(api, "vaultItems").mockResolvedValue(mockItems);
    const importSpy = vi.spyOn(api, "importVaultSecrets").mockResolvedValue({ imported: 2 });
    const exportSpy = vi.spyOn(api, "exportVaultSecrets").mockResolvedValue({
      format: "env",
      dotEnv: "FOO=bar\n",
    });
    vi.spyOn(api, "vaultTokens").mockResolvedValue(mockTokens);
    const createTokenSpy = vi.spyOn(api, "createVaultToken").mockResolvedValue({
      token: { ...mockTokens[0], id: 51, name: "Deploy Runner" },
      plaintext: "octvq_secret_token_once",
    });
    const revokeSpy = vi.spyOn(api, "revokeVaultToken").mockResolvedValue({
      ok: true,
      message: "Service token revoked.",
    });
    vi.spyOn(ui, "confirmDialog").mockResolvedValue(true);

    renderVaultPage();

    // Import .env
    fireEvent.click(await screen.findByRole("button", { name: /import/i }));
    fireEvent.change(screen.getByPlaceholderText(/API_KEY=sk-/i), {
      target: { value: "A=1\nB=2" },
    });
    fireEvent.click(screen.getByRole("button", { name: /import secrets/i }));
    await waitFor(() =>
      expect(importSpy).toHaveBeenCalledWith(
        expect.objectContaining({ format: "env", dotEnv: "A=1\nB=2" }),
      ),
    );

    // Export .env
    fireEvent.click(screen.getByRole("button", { name: /^export$/i }));
    fireEvent.click(screen.getByRole("button", { name: /generate export/i }));
    await waitFor(() => expect(exportSpy).toHaveBeenCalled());
    expect(await screen.findByDisplayValue(/FOO=bar/)).toBeTruthy();
    fireEvent.click(screen.getAllByRole("button", { name: "Close" })[0]);

    // Service Tokens tab
    fireEvent.click(screen.getByRole("tab", { name: /service tokens/i }));
    expect(await screen.findByText("CI Pipeline")).toBeTruthy();

    fireEvent.click(screen.getByRole("button", { name: /new service token/i }));
    fireEvent.change(screen.getByPlaceholderText(/github actions/i), {
      target: { value: "Deploy Runner" },
    });
    fireEvent.click(screen.getByRole("button", { name: /^create$/i }));

    await waitFor(() => expect(createTokenSpy).toHaveBeenCalled());
    expect(await screen.findByTestId("created-service-token")).toBeTruthy();
    expect(screen.getByTestId("created-service-token").textContent).toBe("octvq_secret_token_once");
    fireEvent.click(screen.getByRole("button", { name: "Done" }));

    fireEvent.click(screen.getAllByRole("button", { name: /revoke/i })[0]);
    await waitFor(() => expect(revokeSpy).toHaveBeenCalledWith(51));
  });
});
