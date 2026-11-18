import type { VaultCategorySchema, VaultItemMeta, VaultProviderSchema } from "../../../api/types";

export type VaultTab = "all" | "integrations" | "secrets" | "logins" | "tokens";

export const VAULT_ENVIRONMENTS = ["default", "prod", "staging", "dev"] as const;
export type VaultEnvironment = (typeof VAULT_ENVIRONMENTS)[number];

export const INTEGRATION_CATEGORIES = ["llm", "payment", "storage", "communication", "dns"] as const;
export type IntegrationCategory = (typeof INTEGRATION_CATEGORIES)[number];

export const SECRET_ITEM_TYPES = new Set([
  "kv_secret",
  "api_token",
  "ssh_key",
  "database",
  "certificate",
  "oauth_app",
  "webhook",
  "signing_key",
]);

export const LOGIN_ITEM_TYPES = new Set([
  "login",
  "twofa_totp",
  "tracker_credential",
  "secure_note",
]);

export function resolveTabFromCategory(category: string | null): {
  tab: VaultTab;
  providerType: string;
} {
  const norm = (category ?? "").trim().toLowerCase();
  if (!norm) return { tab: "all", providerType: "" };
  if (norm === "integrations") return { tab: "integrations", providerType: "" };
  if ((INTEGRATION_CATEGORIES as readonly string[]).includes(norm)) {
    return { tab: "integrations", providerType: norm };
  }
  if (norm === "secrets" || norm === "infra") {
    return { tab: "secrets", providerType: "" };
  }
  if (norm === "vault_items" || norm === "logins" || norm === "2fa") {
    return { tab: "logins", providerType: "" };
  }
  if (norm === "tokens") {
    return { tab: "tokens", providerType: "" };
  }
  return { tab: "all", providerType: "" };
}

export function findSchemaForItem(
  schemas: VaultCategorySchema[],
  item: Pick<VaultItemMeta, "itemType" | "providerType" | "provider">,
): VaultProviderSchema | undefined {
  const p = (item.provider ?? "").toLowerCase().trim();
  const pt = (item.providerType ?? "").toLowerCase().trim();
  const it = (item.itemType ?? "").toLowerCase().trim();
  for (const cat of schemas) {
    for (const ps of cat.providers) {
      if (p && ps.id === p && (!pt || ps.providerType === pt)) return ps;
      if (!p && it && ps.id === it) return ps;
    }
  }
  return undefined;
}

export function itemMatchesTab(item: VaultItemMeta, tab: VaultTab, providerTypeFilter: string): boolean {
  if (tab === "all") return true;
  if (tab === "integrations") {
    if (item.itemType !== "integration") return false;
    if (providerTypeFilter && (item.providerType ?? "").toLowerCase() !== providerTypeFilter) {
      return false;
    }
    return true;
  }
  if (tab === "secrets") {
    return SECRET_ITEM_TYPES.has(item.itemType);
  }
  if (tab === "logins") {
    return LOGIN_ITEM_TYPES.has(item.itemType);
  }
  return false;
}

export function looksSecretFieldKey(key: string): boolean {
  const lk = key.toLowerCase();
  return [
    "secret",
    "password",
    "token",
    "private",
    "apikey",
    "api_key",
    "seed",
    "cookie",
    "passphrase",
    "credential",
    "value",
  ].some((marker) => lk.includes(marker));
}
