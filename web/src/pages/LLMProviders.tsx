// LLM Providers — the reusable registry of LLM backends (led-pro ai plugin).
// Inbox AI and other AI features select a provider from this list by id.
// Rendered as a Settings sub-page. 402 → upsell note; OSS build → 404 note.
import { useEffect, useState } from "react";
import { api, ApiError, LLMProvider, LLMProviderInput } from "../api";
import { PageHeader, GlassCard, Button, Badge, Modal, Field, Empty, LockedFeature, ScreenWrap } from "../ui";
import { Bot, Plus } from "lucide-react";
import { useTranslation } from "../i18n";

const PROVIDERS = ["claude", "openai", "gemini", "mistral", "cohere", "ollama"];

export default function LLMProvidersSettings({ embed, onChanged }: { embed?: boolean; onChanged?: () => void }) {
  const [rows, setRows] = useState<LLMProvider[]>([]);
  const [error, setError] = useState<{ status: number } | null>(null);
  const [editing, setEditing] = useState<LLMProvider | "new" | null>(null);
  const { t } = useTranslation();

  function load() {
    api.llmProviders()
      .then((r) => { setRows(r); setError(null); onChanged?.(); })
      .catch((e: ApiError) => setError({ status: e.status }));
  }
  useEffect(load, []);

  async function del(id: number) {
    if (!confirm(t("llmProviders.deleteConfirm"))) return;
    await api.deleteLlmProvider(id);
    load();
  }

  if (error) {
    return (
      <ScreenWrap>
        <LockedFeature
          status={error.status}
          tier="elite"
          feature={t("llmProviders.lockedFeature")}
          description={t("llmProviders.lockedDescription")}
          perks={[
            t("llmProviders.perkKeys"),
            t("llmProviders.perkPower"),
            t("llmProviders.perkShared"),
          ]}
          icon={<Bot className="h-7 w-7" />}
          pricingHref="https://octarq.com/pricing/"
        />
      </ScreenWrap>
    );
  }

  return (
    <div>
      {!embed && (
        <PageHeader
          title={t("llmProviders.pageTitle")}
          description={t("llmProviders.pageDesc")}
          action={<Button variant="primary" onClick={() => setEditing("new")}>{t("llmProviders.addProvider")}</Button>}
        />
      )}
      {embed && (
        <div className="flex justify-between items-center mb-4 pt-4 border-t border-white/[0.04]">
          <div className="text-xs font-semibold text-white/70">
            {t("llmProviders.embedTitle")}
            <div className="text-[10px] text-white/35 font-normal mt-0.5">{t("llmProviders.embedHint")}</div>
          </div>
          <Button variant="primary" className="text-xs py-1 px-2.5" onClick={() => setEditing("new")}>{t("llmProviders.addProvider")}</Button>
        </div>
      )}

      {rows.length === 0 ? (
        <Empty>
          <Bot className="mb-2 h-10 w-10 text-white/30" />
          <p className="text-sm text-white/50">{t("llmProviders.emptyTitle")}</p>
          <Button variant="primary" className="mt-4" onClick={() => setEditing("new")}>{t("llmProviders.emptyAction")}</Button>
        </Empty>
      ) : (
        <div className="grid gap-3 sm:grid-cols-2">
          {rows.map((p) => (
            <GlassCard key={p.id} className="p-4">
              <div className="flex items-start justify-between">
                <div>
                  <div className="flex items-center gap-2">
                    <span className="font-medium text-white">{p.name}</span>
                    <Badge tone="indigo">{p.provider}</Badge>
                  </div>
                  <div className="mt-1 text-xs text-white/45">
                    {p.apiKeySet ? t("llmProviders.keySet") : t("llmProviders.noKey")}
                    {p.model && ` · ${p.model}`}
                    {p.baseUrl && ` · ${p.baseUrl}`}
                  </div>
                </div>
                <div className="flex gap-1">
                  <button onClick={() => setEditing(p)} className="text-xs text-white/50 hover:text-white">{t("llmProviders.edit")}</button>
                  <span className="text-white/20">·</span>
                  <button onClick={() => del(p.id)} className="text-xs text-rose-300/80 hover:text-rose-300">{t("llmProviders.delete")}</button>
                </div>
              </div>
            </GlassCard>
          ))}
        </div>
      )}

      {editing && (
        <ProviderModal
          provider={editing === "new" ? null : editing}
          onClose={() => setEditing(null)}
          onSaved={() => { setEditing(null); load(); }}
        />
      )}
    </div>
  );
}

function ProviderModal({ provider, onClose, onSaved }: { provider: LLMProvider | null; onClose: () => void; onSaved: () => void }) {
  const [f, setF] = useState({
    name: provider?.name ?? "",
    provider: provider?.provider ?? "claude",
    apiKey: "",
    baseUrl: provider?.baseUrl ?? "",
    model: provider?.model ?? "",
    cheapModel: provider?.cheapModel ?? "",
  });
  const [busy, setBusy] = useState(false);
  const { t } = useTranslation();

  async function save() {
    setBusy(true);
    const payload: LLMProviderInput = {
      name: f.name.trim(), provider: f.provider,
      baseUrl: f.baseUrl.trim(), model: f.model.trim(), cheapModel: f.cheapModel.trim(),
      ...(f.apiKey ? { apiKey: f.apiKey } : {}),
    };
    try {
      if (provider) await api.updateLlmProvider(provider.id, payload);
      else await api.createLlmProvider(payload);
      onSaved();
    } catch (e) { alert((e as ApiError).message); } finally { setBusy(false); }
  }

  return (
    <Modal title={provider ? t("llmProviders.modalEditTitle") : t("llmProviders.modalNewTitle")} onClose={onClose}>
      <div className="grid gap-x-3 sm:grid-cols-2">
        <Field label={t("llmProviders.fieldName")}><input className="input w-full" value={f.name} onChange={(e) => setF({ ...f, name: e.target.value })} placeholder={t("llmProviders.fieldNamePlaceholder")} /></Field>
        <Field label={t("llmProviders.fieldProvider")}>
          <select className="input w-full" value={f.provider} onChange={(e) => setF({ ...f, provider: e.target.value })}>
            {PROVIDERS.map((v) => <option key={v} value={v}>{v}</option>)}
          </select>
        </Field>
      </div>
      <Field
        label={t("llmProviders.fieldApiKey")}
        hint={f.provider === "ollama" ? t("llmProviders.apiKeyHintOllama") : provider?.apiKeySet ? t("llmProviders.apiKeyHintSet") : t("llmProviders.apiKeyHintRequired")}
      >
        <input className="input w-full" type="password" value={f.apiKey} onChange={(e) => setF({ ...f, apiKey: e.target.value })}
          placeholder={provider?.apiKeySet ? "••••••••" : t("llmProviders.apiKeyPlaceholder")} disabled={f.provider === "ollama"} />
      </Field>
      <div className="grid gap-x-3 sm:grid-cols-2">
        <Field label={t("llmProviders.fieldReasoningModel")} hint={t("llmProviders.modelHint")}><input className="input w-full" value={f.model} onChange={(e) => setF({ ...f, model: e.target.value })} /></Field>
        <Field label={t("llmProviders.fieldCheapModel")} hint={t("llmProviders.modelHint")}><input className="input w-full" value={f.cheapModel} onChange={(e) => setF({ ...f, cheapModel: e.target.value })} /></Field>
      </div>
      <Field label={t("llmProviders.fieldBaseUrl")} hint={t("llmProviders.baseUrlHint")}>
        <input className="input w-full" value={f.baseUrl} onChange={(e) => setF({ ...f, baseUrl: e.target.value })} placeholder={t("llmProviders.baseUrlPlaceholder")} />
      </Field>
      <div className="mt-2 flex justify-end gap-2">
        <Button variant="ghost" onClick={onClose}>{t("llmProviders.cancel")}</Button>
        <Button variant="primary" onClick={save} disabled={busy || f.name.trim() === ""}>{busy ? t("llmProviders.saving") : t("llmProviders.save")}</Button>
      </div>
    </Modal>
  );
}
