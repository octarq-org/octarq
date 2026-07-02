// Inbox AI page — surfaces the led-pro `ai` plugin's per-email analysis:
// one-line summaries, category labels, importance scores, and extracted
// verification codes. It is an elite-tier Pro feature; when the backend reports
// the feature unlicensed or unconfigured, the page explains how to enable it
// instead of showing an empty table.
import { useEffect, useState } from "react";
import { api, AIStatus, AISettings, EmailAIAnnotation, ApiError, LLMProvider } from "../api";
import { ScreenWrap, PageHeader, GlassCard, Badge, Button, Empty, ProPill, Field, timeAgo, LockedFeature } from "../ui";
import { Sparkles } from "lucide-react";
import LLMProvidersSettings from "./LLMProviders";

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

// AIConfigPanel — inline config for briefing hour + provider selection.
function AIConfigPanel() {
  const [cfg, setCfg] = useState<AISettings | null>(null);
  const [providers, setProviders] = useState<LLMProvider[]>([]);
  const [saving, setSaving] = useState(false);

  useEffect(() => {
    Promise.all([api.aiSettings(), api.llmProviders().catch(() => [])])
      .then(([s, list]) => {
        setCfg(s);
        setProviders(list);
      })
      .catch(() => {});
  }, []);

  async function save() {
    if (!cfg) return;
    setSaving(true);
    try {
      await api.updateAiSettings({ providerId: cfg.providerId, briefingHour: cfg.briefingHour });
    } catch (e: any) {
      alert("Save failed: " + e.message);
    } finally {
      setSaving(false);
    }
  }

  if (!cfg) return <div className="text-sm text-white/40 p-4">Loading…</div>;

  return (
    <GlassCard className="p-6 space-y-4">
      <h3 className="text-sm font-semibold text-white/90">AI Configuration</h3>
      <div className="grid gap-x-4 sm:grid-cols-2">
        <Field
          label="LLM provider"
          hint={providers.length === 0 ? "No providers yet — add one below." : "Pick a configured provider."}
        >
          <select
            className="input w-full"
            value={cfg.providerId}
            onChange={(e) => setCfg({ ...cfg, providerId: e.target.value })}
          >
            <option value="">(none — use env)</option>
            {providers.map((pr) => (
              <option key={pr.id} value={String(pr.id)}>
                {pr.name} · {pr.provider}
              </option>
            ))}
          </select>
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
      <div className="flex justify-end border-t border-white/[0.04] pt-3">
        <Button variant="primary" onClick={save} disabled={saving} className="text-xs">
          {saving ? "Saving…" : "Save Configuration"}
        </Button>
      </div>
    </GlassCard>
  );
}

export default function InboxAIPage() {
  const [status, setStatus] = useState<AIStatus | null>(null);
  const [rows, setRows] = useState<EmailAIAnnotation[]>([]);
  const [category, setCategory] = useState("");
  const [loading, setLoading] = useState(true);
  const [locked, setLocked] = useState(false);
  const [tab, setTab] = useState<'ai' | 'settings'>('ai');

  const refreshStatus = () => api.aiStatus().then(setStatus).catch(() => setStatus(null));

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
            AI Inbox <ProPill />
          </span>
        }
        description="AI email summaries, classification & OTP code extraction"
        action={
          status ? (
            <Badge tone={status.enabled ? "green" : "amber"}>
              {status.enabled ? `Active · ${status.model ?? status.provider}` : "Not configured"}
            </Badge>
          ) : null
        }
      />

      <div className="flex gap-0 border-b border-white/[0.06] mb-6">
        <button
          onClick={() => setTab('ai')}
          className={`px-4 py-2 text-sm font-medium border-b-2 -mb-px transition-colors ${
            tab === 'ai'
              ? 'border-indigo-500 text-white'
              : 'border-transparent text-white/45 hover:text-white/70'
          }`}
        >
          AI Inbox
        </button>
        <button
          onClick={() => setTab('settings')}
          className={`px-4 py-2 text-sm font-medium border-b-2 -mb-px transition-colors flex items-center gap-1.5 ${
            tab === 'settings'
              ? 'border-indigo-500 text-white'
              : 'border-transparent text-white/45 hover:text-white/70'
          }`}
        >
          Settings
        </button>
      </div>

      {tab === 'ai' && (
        <>
          {locked ? (
            <LockedFeature
              status={402}
              tier="elite"
              feature="AI Inbox Automation"
              description="Leverage large language models to automate email sorting, priority indexing, and real-time multi-factor authentication routing."
              perks={[
                "Semantic classification and automated tagging (Invoices, OTPs, Support, Marketing)",
                "Executive summaries and importance priority scoring",
                "Instant verification-code (OTP) routing to your alert channels",
                "Bring-Your-Own-LLM (BYO-LLM) model architecture",
              ]}
              icon={<Sparkles className="h-7 w-7" />}
              pricingHref="https://octarq.com/pricing/"
            />
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
        </>
      )}

      {tab === 'settings' && (
        <div className="space-y-8">
          <AIConfigPanel />
          <div>
            <h3 className="text-sm font-semibold text-white/70 mb-3">LLM Providers</h3>
            <LLMProvidersSettings />
          </div>
        </div>
      )}
    </ScreenWrap>
  );
}
