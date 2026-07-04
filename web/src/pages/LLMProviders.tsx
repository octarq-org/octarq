// LLM Providers — the reusable registry of LLM backends (led-pro ai plugin).
// Inbox AI and other AI features select a provider from this list by id.
// Rendered as a Settings sub-page. 402 → upsell note; OSS build → 404 note.
import { useEffect, useState } from "react";
import { api, ApiError, LLMProvider, LLMProviderInput } from "../api";
import { PageHeader, GlassCard, Button, Badge, Modal, Field, Empty, LockedFeature, ScreenWrap } from "../ui";
import { Bot, Plus } from "lucide-react";

const PROVIDERS = ["claude", "openai", "gemini", "mistral", "cohere", "ollama"];

export default function LLMProvidersSettings({ embed, onChanged }: { embed?: boolean; onChanged?: () => void }) {
  const [rows, setRows] = useState<LLMProvider[]>([]);
  const [error, setError] = useState<{ status: number } | null>(null);
  const [unavailable, setUnavailable] = useState(false);
  const [editing, setEditing] = useState<LLMProvider | "new" | null>(null);

  function load() {
    api.llmProviders()
      .then((r) => { setRows(r); setError(null); setUnavailable(false); onChanged?.(); })
      .catch((e: ApiError) => {
        if (e.status === 404) setUnavailable(true);
        else setError({ status: e.status });
      });
  }
  useEffect(load, []);

  async function del(id: number) {
    if (!confirm("Delete this provider? Any AI feature using it will fall back to none.")) return;
    await api.deleteLlmProvider(id);
    load();
  }

  if (unavailable) {
    return (
      <ScreenWrap>
        <GlassCard className="mx-auto mt-12 max-w-md p-6 text-center text-sm text-white/55">
          LLM providers is an <span className="text-white/80">Octarq Elite</span> feature and isn't part of the open-source build.
        </GlassCard>
      </ScreenWrap>
    );
  }

  if (error) {
    return (
      <ScreenWrap>
        <LockedFeature
          status={error.status}
          tier="elite"
          feature="LLM Providers"
          description="Configure LLM backends once; AI features select one by name."
          perks={[
            "Bring your own keys for OpenAI, Claude, Gemini, Mistral, and more",
            "Power semantic email summary, sorting, and OTP extraction",
            "Shared backend configurations reusable across all workspace agents",
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
          title="LLM Providers"
          description="Configure LLM backends once; AI features select one by name."
          action={<Button variant="primary" onClick={() => setEditing("new")}>+ Add provider</Button>}
        />
      )}
      {embed && (
        <div className="flex justify-between items-center mb-4 pt-4 border-t border-white/[0.04]">
          <div className="text-xs font-semibold text-white/70">
            LLM API Keys & Providers
            <div className="text-[10px] text-white/35 font-normal mt-0.5">Define your OpenAI/Gemini keys here.</div>
          </div>
          <Button variant="primary" className="text-xs py-1 px-2.5" onClick={() => setEditing("new")}>+ Add provider</Button>
        </div>
      )}

      {rows.length === 0 ? (
        <Empty>
          <Bot className="mb-2 h-10 w-10 text-white/30" />
          <p className="text-sm text-white/50">No LLM providers yet.</p>
          <Button variant="primary" className="mt-4" onClick={() => setEditing("new")}>Add provider</Button>
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
                    {p.apiKeySet ? "key set" : "no key"}
                    {p.model && ` · ${p.model}`}
                    {p.baseUrl && ` · ${p.baseUrl}`}
                  </div>
                </div>
                <div className="flex gap-1">
                  <button onClick={() => setEditing(p)} className="text-xs text-white/50 hover:text-white">Edit</button>
                  <span className="text-white/20">·</span>
                  <button onClick={() => del(p.id)} className="text-xs text-rose-300/80 hover:text-rose-300">Delete</button>
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
    <Modal title={provider ? "Edit provider" : "New LLM provider"} onClose={onClose}>
      <div className="grid gap-x-3 sm:grid-cols-2">
        <Field label="Name"><input className="input w-full" value={f.name} onChange={(e) => setF({ ...f, name: e.target.value })} placeholder="Claude (work)" /></Field>
        <Field label="Provider">
          <select className="input w-full" value={f.provider} onChange={(e) => setF({ ...f, provider: e.target.value })}>
            {PROVIDERS.map((v) => <option key={v} value={v}>{v}</option>)}
          </select>
        </Field>
      </div>
      <Field
        label="API key"
        hint={f.provider === "ollama" ? "Not needed for local Ollama." : provider?.apiKeySet ? "A key is set — leave blank to keep it." : "Required for cloud providers."}
      >
        <input className="input w-full" type="password" value={f.apiKey} onChange={(e) => setF({ ...f, apiKey: e.target.value })}
          placeholder={provider?.apiKeySet ? "••••••••" : "sk-…"} disabled={f.provider === "ollama"} />
      </Field>
      <div className="grid gap-x-3 sm:grid-cols-2">
        <Field label="Reasoning model" hint="(provider default if blank)"><input className="input w-full" value={f.model} onChange={(e) => setF({ ...f, model: e.target.value })} /></Field>
        <Field label="Cheap model" hint="(provider default if blank)"><input className="input w-full" value={f.cheapModel} onChange={(e) => setF({ ...f, cheapModel: e.target.value })} /></Field>
      </div>
      <Field label="Base URL" hint="Optional: OpenAI-compatible gateway / local Ollama address">
        <input className="input w-full" value={f.baseUrl} onChange={(e) => setF({ ...f, baseUrl: e.target.value })} placeholder="(default)" />
      </Field>
      <div className="mt-2 flex justify-end gap-2">
        <Button variant="ghost" onClick={onClose}>Cancel</Button>
        <Button variant="primary" onClick={save} disabled={busy || f.name.trim() === ""}>{busy ? "Saving…" : "Save"}</Button>
      </div>
    </Modal>
  );
}
