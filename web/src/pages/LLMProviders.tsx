// LLM Providers — the reusable registry of LLM backends (led-pro ai plugin).
// Inbox AI and other AI features select a provider from this list by id.
// Rendered as a Settings sub-page. 402 → upsell note; OSS build → 404 note.
import { useEffect, useState } from "react";
import { api, ApiError, LLMProvider, LLMProviderInput } from "../api";
import { PageHeader, GlassCard, Button, Badge, Modal, Field, Empty } from "../ui";
import { Bot, Plus } from "lucide-react";

const PROVIDERS = ["claude", "openai", "gemini", "mistral", "cohere", "ollama"];

export default function LLMProvidersSettings() {
  const [rows, setRows] = useState<LLMProvider[]>([]);
  const [note, setNote] = useState<string | null>(null);
  const [editing, setEditing] = useState<LLMProvider | "new" | null>(null);

  function load() {
    api.llmProviders()
      .then((r) => { setRows(r); setNote(null); })
      .catch((e: ApiError) => {
        if (e.status === 404) setNote("LLM features are part of Octarq Elite and aren't in the open-source build.");
        else if (e.status === 402) setNote("LLM providers require an Elite license.");
        else setNote(e.message);
      });
  }
  useEffect(load, []);

  async function del(id: number) {
    if (!confirm("Delete this provider? Any AI feature using it will fall back to none.")) return;
    await api.deleteLlmProvider(id);
    load();
  }

  return (
    <div>
      <PageHeader
        title="LLM Providers"
        description="Configure LLM backends once; AI features select one by name."
        action={!note && <Button variant="primary" onClick={() => setEditing("new")}>+ Add provider</Button>}
      />

      {note ? (
        <GlassCard className="p-6 text-sm text-white/55">{note}</GlassCard>
      ) : rows.length === 0 ? (
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
