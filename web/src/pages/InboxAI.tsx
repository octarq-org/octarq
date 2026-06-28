// Inbox AI page — surfaces the led-pro `ai` plugin's per-email analysis:
// one-line summaries, category labels, importance scores, and extracted
// verification codes. It is an elite-tier Pro feature; when the backend reports
// the feature unlicensed or unconfigured, the page explains how to enable it
// instead of showing an empty table.
import { useEffect, useState } from "react";
import { api, AIStatus, AISettings, EmailAIAnnotation, ApiError } from "../api";
import { ScreenWrap, PageHeader, GlassCard, Badge, Button, Empty, ProPill, Field, timeAgo } from "../ui";

// Category → badge tone + label, so the list reads at a glance.
const CATEGORY_META: Record<string, { tone: any; label: string }> = {
  bill: { tone: "amber", label: "Bill" },
  otp: { tone: "cyan", label: "OTP" },
  marketing: { tone: "neutral", label: "Marketing" },
  important: { tone: "red", label: "Important" },
  personal: { tone: "violet", label: "Personal" },
  other: { tone: "neutral", label: "Other" },
};

const CATEGORIES = ["", "important", "bill", "otp", "marketing", "personal", "other"];

export default function InboxAIPage() {
  const [status, setStatus] = useState<AIStatus | null>(null);
  const [rows, setRows] = useState<EmailAIAnnotation[]>([]);
  const [category, setCategory] = useState("");
  const [loading, setLoading] = useState(true);
  const [locked, setLocked] = useState(false);
  const [showConfig, setShowConfig] = useState(false);
  const [cfg, setCfg] = useState<AISettings | null>(null);
  const [apiKey, setApiKey] = useState("");
  const [saving, setSaving] = useState(false);

  const refreshStatus = () => api.aiStatus().then(setStatus).catch(() => setStatus(null));

  const openConfig = () => {
    api
      .aiSettings()
      .then((s) => {
        setCfg(s);
        setApiKey("");
        setShowConfig(true);
      })
      .catch(() => {});
  };

  const saveConfig = async () => {
    if (!cfg) return;
    setSaving(true);
    try {
      await api.updateAiSettings({
        provider: cfg.provider,
        model: cfg.model,
        cheapModel: cfg.cheapModel,
        baseUrl: cfg.baseUrl,
        briefingHour: cfg.briefingHour,
        ...(apiKey ? { apiKey } : {}),
      });
      setShowConfig(false);
      refreshStatus();
      load();
    } catch (e: any) {
      alert("Save failed: " + e.message);
    } finally {
      setSaving(false);
    }
  };

  const load = () => {
    setLoading(true);
    api
      .aiEmails(category)
      .then((r) => {
        setRows(r);
        setLocked(false);
      })
      .catch((e: ApiError) => {
        // 402 = needs an elite license; show the upsell rather than an error.
        if (e.status === 402) setLocked(true);
      })
      .finally(() => setLoading(false));
  };

  useEffect(() => {
    refreshStatus();
  }, []);

  useEffect(() => {
    load();
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [category]);

  const importanceDots = (n: number) =>
    "●".repeat(Math.max(0, Math.min(5, n))) + "○".repeat(5 - Math.max(0, Math.min(5, n)));

  return (
    <ScreenWrap>
      <PageHeader
        title={
          <span className="inline-flex items-center gap-2">
            Inbox AI <ProPill />
          </span>
        }
        description="AI summaries, classification and verification-code extraction for incoming mail."
        action={
          <div className="flex items-center gap-2">
            {status && (
              <Badge tone={status.enabled ? "green" : "amber"}>
                {status.enabled ? `Active · ${status.model ?? status.provider}` : "Not configured"}
              </Badge>
            )}
            <Button variant="outline" className="text-xs" onClick={openConfig}>
              Configure
            </Button>
          </div>
        }
      />

      {showConfig && cfg && (
        <GlassCard className="mb-4 p-5">
          <div className="mb-3 flex items-center justify-between">
            <h3 className="text-sm font-semibold text-white/90">AI configuration</h3>
            <button className="text-xs text-white/40 hover:text-white/70" onClick={() => setShowConfig(false)}>
              Close
            </button>
          </div>
          <div className="grid gap-x-4 sm:grid-cols-2">
            <Field label="Provider" hint="claude · openai · gemini · mistral · cohere · ollama">
              <select
                className="input w-full"
                value={cfg.provider}
                onChange={(e) => setCfg({ ...cfg, provider: e.target.value })}
              >
                {["claude", "openai", "gemini", "mistral", "cohere", "ollama"].map((v) => (
                  <option key={v} value={v}>
                    {v}
                  </option>
                ))}
              </select>
            </Field>
            <Field
              label="API key"
              hint={
                cfg.provider === "ollama"
                  ? "Not needed for local Ollama."
                  : cfg.apiKeySet
                    ? "A key is set — leave blank to keep it."
                    : "Required for cloud providers."
              }
            >
              <input
                className="input w-full"
                type="password"
                value={apiKey}
                onChange={(e) => setApiKey(e.target.value)}
                placeholder={cfg.apiKeySet ? "••••••••" : "sk-…"}
                disabled={cfg.provider === "ollama"}
              />
            </Field>
            <Field label="Reasoning model" hint="Briefings (per-provider default if blank)">
              <input
                className="input w-full"
                value={cfg.model}
                onChange={(e) => setCfg({ ...cfg, model: e.target.value })}
                placeholder="(provider default)"
              />
            </Field>
            <Field label="Cheap model" hint="Per-email classification (per-provider default if blank)">
              <input
                className="input w-full"
                value={cfg.cheapModel}
                onChange={(e) => setCfg({ ...cfg, cheapModel: e.target.value })}
                placeholder="(provider default)"
              />
            </Field>
            <Field label="Base URL" hint="Optional: OpenAI-compatible gateway / local Ollama address">
              <input
                className="input w-full"
                value={cfg.baseUrl}
                onChange={(e) => setCfg({ ...cfg, baseUrl: e.target.value })}
                placeholder="(default)"
              />
            </Field>
            <Field label="Daily briefing hour" hint="Local hour (0–23) to push the morning briefing">
              <input
                className="input w-full"
                type="number"
                min={0}
                max={23}
                value={cfg.briefingHour}
                onChange={(e) => setCfg({ ...cfg, briefingHour: Number(e.target.value) })}
              />
            </Field>
          </div>
          <div className="mt-2 flex justify-end">
            <Button variant="primary" onClick={saveConfig} disabled={saving}>
              {saving ? "Saving…" : "Save"}
            </Button>
          </div>
        </GlassCard>
      )}

      {locked ? (
        <GlassCard className="p-8">
          <Empty>
            Inbox AI requires an <strong>elite</strong> led-pro license. It analyzes each incoming
            email — a one-line summary, a category (bill / OTP / marketing / important), an importance
            score, and instant verification-code extraction pushed to your alert channels.
          </Empty>
        </GlassCard>
      ) : (
        <>
          <div className="mb-4 flex flex-wrap gap-2">
            {CATEGORIES.map((c) => (
              <button
                key={c || "all"}
                onClick={() => setCategory(c)}
                className={
                  "rounded-full px-3 py-1 text-xs font-medium ring-1 ring-inset transition " +
                  (category === c
                    ? "bg-indigo-500/20 text-indigo-200 ring-indigo-400/40"
                    : "text-white/60 ring-white/10 hover:bg-white/5")
                }
              >
                {c === "" ? "All" : CATEGORY_META[c]?.label ?? c}
              </button>
            ))}
          </div>

          <GlassCard className="overflow-hidden">
            {loading ? (
              <div className="p-8 text-center text-white/40">Loading…</div>
            ) : rows.length === 0 ? (
              <div className="p-8">
                <Empty>
                  No analyzed email yet. As mail arrives it is summarized automatically; you can also
                  re-run analysis from any row.
                </Empty>
              </div>
            ) : (
              <div className="divide-y divide-white/5">
                {rows.map((r) => {
                  const meta = CATEGORY_META[r.category] ?? CATEGORY_META.other;
                  return (
                    <div key={r.emailId} className="flex items-start gap-3 px-4 py-3">
                      <div className="min-w-0 flex-1">
                        <div className="flex items-center gap-2">
                          <Badge tone={meta.tone}>{meta.label}</Badge>
                          {r.otp && <Badge tone="cyan">🔐 {r.otp}</Badge>}
                          <span className="truncate text-sm font-medium text-white/90">{r.subject}</span>
                        </div>
                        <p className="mt-1 truncate text-sm text-white/60">{r.summary}</p>
                        <p className="mt-0.5 text-xs text-white/35">
                          {r.from} · {timeAgo(r.createdAt)}
                        </p>
                      </div>
                      <div className="flex flex-col items-end gap-1">
                        <span
                          className="text-xs tracking-widest text-amber-300/80"
                          title={`importance ${r.importance}/5`}
                        >
                          {importanceDots(r.importance)}
                        </span>
                        <Button
                          variant="subtle"
                          className="text-xs"
                          onClick={async () => {
                            try {
                              await api.aiReprocess(r.emailId);
                              load();
                            } catch (e: any) {
                              alert("Reprocess failed: " + e.message);
                            }
                          }}
                        >
                          Re-analyze
                        </Button>
                      </div>
                    </div>
                  );
                })}
              </div>
            )}
          </GlassCard>
        </>
      )}
    </ScreenWrap>
  );
}
