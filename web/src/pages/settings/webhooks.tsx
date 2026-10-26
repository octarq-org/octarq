import {
  Field,
  Input,
  Modal,
  Switch,
  PageHeader,
  GlassCard,
  Button,
  toast,
  confirmDialog,
  Alert,
  Tabs,
  type TabItem,
  cn,
} from "@octarq/plugin-sdk";
import { useEffect, useState } from "react";
import { api, WebhookEventGroup } from "../../api";
import {
  Trash2,
  Plus,
  Send,
  AlertCircle,
  RefreshCw,
  ArrowUpRight,
  ArrowDownLeft,
  ShieldCheck,
  RotateCcw,
  Clock,
  Radio,
} from "lucide-react";
import { useTranslation } from "../../i18n";
import { roleSatisfies, useCurrentRole } from "../../shell/role";
import { ListSkeleton } from "../../components/ListSkeleton";

export interface DeadLetterEntry {
  id: number;
  deliveryId: string;
  event: string;
  url: string;
  attempts: number;
  responseCode: number;
  lastError: string;
  createdAt: string;
  replaying?: boolean;
}

export function WebhooksSettings() {
  const { t } = useTranslation();
  const { role, isInstanceAdmin } = useCurrentRole();
  const canManage = roleSatisfies("admin", role, isInstanceAdmin);
  const [webhooks, setWebhooks] = useState<any[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<unknown>(null);
  const [show, setShow] = useState(false);
  const [name, setName] = useState("");
  const [url, setUrl] = useState("");
  const [secret, setSecret] = useState("");
  const [all, setAll] = useState(true);
  const [selected, setSelected] = useState<Set<string>>(new Set());
  const [eventGroups, setEventGroups] = useState<WebhookEventGroup[] | null>(null);
  const [busy, setBusy] = useState(false);
  const [testingId, setTestingId] = useState<number | null>(null);
  const [activeTab, setActiveTab] = useState<"outbound" | "inbound">("outbound");
  const [deadLetters, setDeadLetters] = useState<DeadLetterEntry[]>([]);
  const [inboundEndpoints, setInboundEndpoints] = useState<any[]>([]);
  const [dlqSearch, setDlqSearch] = useState("");

  const handleReplay = async (entry: DeadLetterEntry) => {
    setDeadLetters((prev) =>
      prev.map((item) => (item.id === entry.id ? { ...item, replaying: true } : item)),
    );
    try {
      if (api.replayWebhookDeadLetter) {
        await api.replayWebhookDeadLetter(entry.id);
      }
      toast.success(t("settings.replayQueued", "Delivery event queued for replay."));
      setDeadLetters((prev) => prev.filter((item) => item.id !== entry.id));
    } catch {
      toast.error(t("settings.replayFailed", "Replay request failed."));
      setDeadLetters((prev) =>
        prev.map((item) => (item.id === entry.id ? { ...item, replaying: false } : item)),
      );
    }
  };

  function load() {
    setLoading(true);
    setError(null);
    api.webhooks()
      .then((data) => {
        setWebhooks(data);
        setError(null);
      })
      .catch((err) => {
        setError(err);
      })
      .finally(() => {
        setLoading(false);
      });
    if (api.webhookRelayEndpoints) {
      api.webhookRelayEndpoints().then(setInboundEndpoints).catch(() => {});
    }
    if (api.webhookRelayDeadLetters) {
      api.webhookRelayDeadLetters().then((data) => {
        if (Array.isArray(data)) {
          setDeadLetters(data.map((d: any) => ({
            id: d.id,
            deliveryId: d.deliveryId || d.event_id || `del_${d.id}`,
            event: d.event || d.event_type || "webhook.relay",
            url: d.url || d.target_url || "/api/webhook/relay",
            attempts: d.attempts || d.retry_count || 1,
            responseCode: d.responseCode || d.status_code || 500,
            lastError: d.lastError || d.error_message || "Delivery failed",
            createdAt: d.createdAt || d.created_at || "recently",
          })));
        }
      }).catch(() => {});
    }
  }
  useEffect(load, []);
  useEffect(() => { api.webhookEvents().then(setEventGroups).catch(() => setEventGroups([])); }, []);

  // key → definition, for rendering a stored subscription string as titled badges.
  const defByKey = new Map((eventGroups ?? []).flatMap((g) => g.events.map((e) => [e.key, e] as const)));

  function toggleEvent(key: string, on: boolean) {
    setSelected((prev) => { const next = new Set(prev); if (on) next.add(key); else next.delete(key); return next; });
  }
  function toggleGroup(g: WebhookEventGroup, on: boolean) {
    setSelected((prev) => { const next = new Set(prev); for (const e of g.events) { if (on) next.add(e.key); else next.delete(e.key); } return next; });
  }

  async function del(id: number) {
    if (!(await confirmDialog(t("settings.confirmDeleteWebhook")))) return;
    try {
      await api.deleteWebhook(id);
      setWebhooks((w) => w.filter((h) => h.id !== id));
      toast.success(t("settings.saved"));
    } catch (err: any) {
      toast.error(err.message || t("settings.failed"));
    }
  }

  async function toggle(h: any) {
    try {
      const u = await api.updateWebhook(h.id, { enabled: !h.enabled });
      setWebhooks((w) => w.map((x) => x.id === h.id ? u : x));
      toast.success(t("settings.saved"));
    } catch (err: any) {
      toast.error(err.message || t("settings.failed"));
    }
  }

  async function test(h: any) {
    setTestingId(h.id);
    try {
      await api.testWebhook(h.id);
      toast.success(t("settings.webhookTestSent", { name: h.name }));
    } catch (err: any) {
      toast.error(t("settings.testFailed", { msg: err.message || "unknown error" }));
    } finally {
      setTestingId(null);
    }
  }

  async function create(e: React.FormEvent) {
    e.preventDefault();
    if (!name.trim() || !url.trim()) return;
    setBusy(true);
    try {
      const events = all || selected.size === 0 ? "*" : Array.from(selected).join(",");
      const created = await api.createWebhook({ name: name.trim(), url: url.trim(), secret: secret.trim() || undefined, events, enabled: true } as any);
      setWebhooks((w) => [created, ...w]);
      setError(null);
      setShow(false);
      setName("");
      setUrl("");
      setSecret("");
      toast.success(t("settings.saved"));
    } catch (err: any) {
      toast.error(err.message || t("settings.createFailed"));
    } finally {
      setBusy(false);
    }
  }

  const filteredDeadLetters = deadLetters.filter((entry) => {
    if (!dlqSearch.trim()) return true;
    const q = dlqSearch.toLowerCase();
    return (
      entry.deliveryId.toLowerCase().includes(q) ||
      entry.event.toLowerCase().includes(q) ||
      entry.url.toLowerCase().includes(q) ||
      entry.lastError.toLowerCase().includes(q)
    );
  });

  return (
    <div className="space-y-6">
      <PageHeader
        title={t("settings.webhooksRelaysTitle", "Webhooks & Relays")}
        description={t(
          "settings.webhooksRelaysDesc",
          "Manage outbound event subscriptions, inbound webhook relays, and dead-letter queues.",
        )}
      />

      {/* Tab Switcher */}
      <div className="max-w-md">
        <Tabs
          value={activeTab}
          onValueChange={(val) => setActiveTab(val as "outbound" | "inbound")}
          items={[
            {
              value: "outbound",
              label: (
                <span
                  data-testid="tab-outbound-webhooks"
                  className="flex items-center justify-center gap-1.5"
                >
                  <ArrowUpRight className="h-3.5 w-3.5" />
                  <span>{t("settings.tabOutboundSubscriptions", "Outbound Subscriptions")}</span>
                </span>
              ),
              content: null,
            },
            {
              value: "inbound",
              label: (
                <span
                  data-testid="tab-inbound-relays"
                  className="flex items-center justify-center gap-1.5"
                >
                  <ArrowDownLeft className="h-3.5 w-3.5" />
                  <span>{t("settings.tabInboundRelays", "Inbound Relays & DLQ")}</span>
                </span>
              ),
              content: null,
            },
          ]}
        />
      </div>

      {activeTab === "outbound" && (
        <GlassCard className="p-6 space-y-4">
          <div className="flex items-center justify-between">
            <div className="flex items-center gap-2">
              <h2 className="text-base font-bold text-foreground">{t("settings.outboundEventWebhooks")}</h2>
              <span className="rounded-full bg-primary/10 text-primary px-2 py-0.5 text-[10px] font-semibold">
                HMAC-SHA256 Signatures
              </span>
            </div>
            {canManage && (
              <Button
                variant="ghost"
                onClick={() => {
                  setName("");
                  setUrl("");
                  setSecret("");
                  setAll(true);
                  setSelected(new Set());
                  setShow(true);
                }}
                className="flex items-center gap-1.5 px-3 py-1 text-xs"
              >
                <Plus className="h-3 w-3" /> {t("settings.addWebhook")}
              </Button>
            )}
          </div>
          {loading ? (
            <ListSkeleton rows={4} ariaLabel={t("settings.loadingWebhooks")} />
          ) : error ? (
            <Alert
              variant="danger"
              icon={<AlertCircle className="h-5 w-5 shrink-0" />}
              actions={
                <Button
                  variant="outline"
                  size="sm"
                  onClick={load}
                  className="flex items-center gap-1.5 text-xs"
                >
                  <RefreshCw className="h-3.5 w-3.5" />
                  {t("settings.retry")}
                </Button>
              }
            >
              <div className="space-y-1">
                <p className="font-semibold text-danger-fg">{t("settings.loadFailed")}</p>
                {error instanceof Error ? (
                  <p className="text-xs text-danger-fg/80">{error.message}</p>
                ) : typeof error === "string" ? (
                  <p className="text-xs text-danger-fg/80">{error}</p>
                ) : null}
                {typeof error === "object" && error !== null && ("status" in error || "requestId" in error) && (
                  <p className="font-mono tabular-nums tnum text-[11px] text-danger-fg/70">
                    {typeof (error as any).status === "number" && t("uiCommon.formErrorStatus", { status: (error as any).status })}
                    {typeof (error as any).status === "number" && (error as any).requestId && " · "}
                    {(error as any).requestId && t("uiCommon.formErrorRequestId", { requestId: (error as any).requestId })}
                  </p>
                )}
              </div>
            </Alert>
          ) : webhooks.length === 0 ? (
            <div className="select-none rounded border border-dashed border-foreground/[0.06] py-4 text-center text-xs text-foreground/40">{t("settings.noWebhooks")}</div>
          ) : (
            <div className="space-y-3.5">
              {webhooks.map((w) => (
                <div key={w.id} className="flex flex-col justify-between gap-3 rounded-lg border border-foreground/[0.06] bg-foreground/[0.02] p-3 text-sm sm:flex-row sm:items-center">
                  <div className="min-w-0 flex-1 space-y-1">
                    <div className="flex flex-wrap items-center gap-2">
                      <span className="font-semibold text-foreground/80">{w.name}</span>
                      {w.events === "*" ? (
                        <span className="rounded border border-foreground/10 bg-foreground/5 px-1.5 py-0.5 font-mono text-[9px] uppercase text-foreground/45">{t("settings.allEvents")}</span>
                      ) : (
                        String(w.events).split(",").filter(Boolean).map((key: string) => {
                          const def = defByKey.get(key.trim());
                          return (
                            <span key={key} title={def ? `${def.group} — ${def.title}: ${def.description}` : undefined} className="rounded border border-foreground/10 bg-foreground/5 px-1.5 py-0.5 font-mono text-[9px] text-foreground/45">
                              {key.trim()}
                            </span>
                          );
                        })
                      )}
                    </div>
                    <div className="select-all truncate font-mono text-xs text-foreground/45">{w.url}</div>
                    <div className="select-all font-mono text-[10px] text-zinc-500">{t("settings.secretLabel")} {w.secret || (w.secretSet ? "••••••••" : "—")}</div>
                  </div>
                  {canManage && (
                    <div className="flex shrink-0 items-center justify-between sm:justify-end gap-3 w-full sm:w-auto pt-2 sm:pt-0 border-t sm:border-t-0 border-foreground/[0.04]">
                      <Switch checked={w.enabled} onCheckedChange={() => toggle(w)} />
                      <Button
                        variant="ghost"
                        onClick={() => test(w)}
                        disabled={testingId === w.id}
                        className="flex items-center gap-1 px-3 sm:px-2.5 py-2 sm:py-1 text-xs min-h-[44px] sm:min-h-0"
                      >
                        <Send className="h-3 w-3" /> {testingId === w.id ? t("settings.testing") : t("settings.test")}
                      </Button>
                      <Button variant="danger" onClick={() => del(w.id)} className="flex items-center gap-1 px-3 sm:px-2.5 py-2 sm:py-1 text-xs min-h-[44px] sm:min-h-0"><Trash2 className="h-3 w-3" /> {t("settings.delete")}</Button>
                    </div>
                  )}
                </div>
              ))}
            </div>
          )}
        </GlassCard>
      )}

      {activeTab === "inbound" && (
        <div className="space-y-6" data-testid="inbound-relays-panel">
          {/* Section 1: Inbound Webhook Relays */}
          <GlassCard className="p-6 space-y-4">
            <div className="flex items-center justify-between">
              <div>
                <h2 className="text-base font-bold text-foreground">
                  {t("settings.inboundRelaysHeading", "Inbound Webhook Relays")}
                </h2>
                <p className="text-xs text-muted-foreground mt-0.5">
                  {t("settings.inboundRelaysDesc", "Receive and verify webhooks from third-party services with cryptographic signatures.")}
                </p>
              </div>
            </div>

            <div className="space-y-3">
              {inboundEndpoints.length === 0 ? (
                <div className="rounded-xl border border-dashed border-border py-8 text-center text-xs text-muted-foreground space-y-1">
                  <div className="font-semibold text-foreground">
                    {t("settings.noInboundEndpoints", "No inbound webhook relays configured")}
                  </div>
                  <div>
                    {t("settings.noInboundEndpointsDesc", "Inbound relays allow external services (like Stripe or GitHub) to securely trigger automation rules.")}
                  </div>
                </div>
              ) : (
                inboundEndpoints.map((relay) => (
                  <div
                    key={relay.id}
                    className="flex flex-col sm:flex-row sm:items-center justify-between gap-3 rounded-xl border border-border bg-card p-3.5 shadow-xs"
                  >
                    <div className="space-y-1 min-w-0 flex-1">
                      <div className="flex items-center gap-2">
                        <span className="font-semibold text-sm text-foreground">{relay.name}</span>
                        <span className="rounded-full bg-success-fg/10 text-success-fg px-2 py-0.5 text-[10px] font-semibold flex items-center gap-1">
                          <ShieldCheck className="h-3 w-3" />
                          <span>{relay.algorithm || relay.provider || "HMAC-SHA256"}</span>
                        </span>
                      </div>
                      <div className="font-mono text-xs text-muted-foreground select-all">{relay.path || `/api/webhook/relay/${relay.id}`}</div>
                      {relay.description && <div className="text-xs text-muted-foreground">{relay.description}</div>}
                    </div>

                    <span className="rounded-full bg-surface-hover px-2.5 py-1 text-xs font-medium text-foreground self-start sm:self-center">
                      {relay.status || (relay.enabled !== false ? "Active" : "Disabled")}
                    </span>
                  </div>
                ))
              )}
            </div>
          </GlassCard>

          {/* Section 2: Dead-Letter Queue (DLQ) Audit */}
          <GlassCard className="p-6 space-y-4">
            <div className="flex flex-col sm:flex-row sm:items-center justify-between gap-2">
              <div>
                <h2 className="text-base font-bold text-foreground">
                  {t("settings.dlqHeading", "Dead-Letter Queue (DLQ) Audit")}
                </h2>
                <p className="text-xs text-muted-foreground mt-0.5">
                  {t("settings.dlqDesc", "Deliveries that exhausted all retry attempts. Inspect errors and replay when ready.")}
                </p>
              </div>

              <div className="w-full sm:w-64">
                <Input
                  className="text-xs"
                  placeholder={t("common.filter", "Filter failed deliveries…")}
                  value={dlqSearch}
                  onChange={(e) => setDlqSearch(e.target.value)}
                />
              </div>
            </div>

            {filteredDeadLetters.length === 0 ? (
              <div
                data-testid="dlq-empty-state"
                className="select-none rounded-xl border border-dashed border-border py-8 text-center text-xs text-muted-foreground space-y-1"
              >
                <div className="font-semibold text-foreground">
                  {t("settings.dlqEmptyTitle", "Dead-letter queue is clear")}
                </div>
                <div>{t("settings.dlqEmptyDesc", "All outbound deliveries succeeded or were manually resolved.")}</div>
              </div>
            ) : (
              <div className="space-y-3" data-testid="dlq-list">
                {filteredDeadLetters.map((entry) => (
                  <div
                    key={entry.id}
                    className="flex flex-col sm:flex-row sm:items-start justify-between gap-3 rounded-xl border border-border bg-card p-3.5 shadow-xs"
                  >
                    <div className="space-y-1.5 min-w-0 flex-1">
                      <div className="flex flex-wrap items-center gap-2">
                        <span className="rounded bg-danger-fg/10 text-danger-fg px-1.5 py-0.5 font-mono text-[10px] font-bold">
                          HTTP {entry.responseCode}
                        </span>
                        <span className="font-mono text-xs font-semibold text-foreground">{entry.event}</span>
                        <span className="text-[11px] text-muted-foreground font-mono">#{entry.deliveryId}</span>
                        <span className="text-[11px] text-muted-foreground flex items-center gap-1">
                          <Clock className="h-3 w-3" />
                          <span>{entry.createdAt}</span>
                        </span>
                      </div>

                      <div className="font-mono text-xs text-muted-foreground truncate select-all">
                        {entry.url}
                      </div>

                      <div className="rounded-lg bg-well p-2 text-xs text-danger-fg/90 font-mono">
                        {entry.lastError}
                      </div>

                      <div className="text-[11px] text-muted-foreground">
                        {entry.attempts} / 5
                      </div>
                    </div>

                    <div className="shrink-0 self-end sm:self-start">
                      <Button
                        variant="outline"
                        size="sm"
                        disabled={entry.replaying}
                        onClick={() => handleReplay(entry)}
                        className="flex items-center gap-1.5 text-xs"
                      >
                        <RotateCcw className={cn("h-3.5 w-3.5", entry.replaying && "animate-spin")} />
                        <span>{entry.replaying ? t("common.replaying", "Replaying…") : t("common.replay", "Replay")}</span>
                      </Button>
                    </div>
                  </div>
                ))}
              </div>
            )}
          </GlassCard>
        </div>
      )}

      {show && (
        <Modal title={t("settings.addWebhookEndpoint")} onClose={() => setShow(false)}>
          <form onSubmit={create} className="space-y-4">
            <Field label={t("settings.endpointName")}><Input value={name} onChange={(e) => setName(e.target.value)} placeholder="n8n automation" required autoFocus /></Field>
            <Field label={t("settings.endpointUrl")}><Input className="font-mono text-xs" value={url} onChange={(e) => setUrl(e.target.value)} placeholder="https://your-server.com/webhooks/octarq" required /></Field>
            <Field label={t("settings.signingSecretOptional")} hint={t("settings.signingSecretHint")}>
              <Input className="font-mono text-xs" value={secret} onChange={(e) => setSecret(e.target.value)} placeholder={t("settings.signingSecretPlaceholder")} />
            </Field>
            <Field label={t("settings.eventSubscriptions")}>
              <div className="mt-1 space-y-2">
                <label className="flex cursor-pointer items-center gap-2 text-xs text-zinc-300">
                  <Switch checked={all} onCheckedChange={setAll} />
                  <span>{all ? t("settings.allEventsStar") : t("settings.customEvents")}</span>
                </label>
                {!all && (
                  <div className="max-h-64 space-y-3 overflow-y-auto rounded-lg border border-foreground/[0.06] bg-well p-3">
                    {eventGroups === null ? (
                      <div className="py-2 text-center text-xs text-foreground/40">{t("settings.loadingEvents")}</div>
                    ) : (
                      eventGroups.map((g) => {
                        const allChecked = g.events.every((ev) => selected.has(ev.key));
                        return (
                          <div key={g.group} className="space-y-1.5">
                            <label className="flex cursor-pointer items-center gap-2 text-xs font-semibold text-foreground/70">
                              <input type="checkbox" checked={allChecked} onChange={(e) => toggleGroup(g, e.target.checked)} />
                              <span>{g.group}</span>
                            </label>
                            <div className="space-y-1.5 pl-6">
                              {g.events.map((ev) => (
                                <label key={ev.key} className="flex cursor-pointer items-start gap-2 text-xs text-zinc-300">
                                  <input type="checkbox" className="mt-0.5" checked={selected.has(ev.key)} onChange={(e) => toggleEvent(ev.key, e.target.checked)} />
                                  <span className="min-w-0">
                                    <span className="flex flex-wrap items-center gap-1.5">
                                      <span>{ev.title}</span>
                                      <span className="rounded border border-foreground/10 bg-foreground/5 px-1 py-px font-mono text-[9px] text-foreground/45">{ev.key}</span>
                                    </span>
                                    <span className="block text-[10px] text-foreground/40">{ev.description}</span>
                                  </span>
                                </label>
                              ))}
                            </div>
                          </div>
                        );
                      })
                    )}
                    {selected.size === 0 && <div className="text-[10px] text-warning-fg/80">{t("settings.noEventsSelectedHint")}</div>}
                  </div>
                )}
              </div>
            </Field>
            <div className="flex justify-end gap-2">
              <Button variant="ghost" onClick={() => setShow(false)}>{t("settings.cancel")}</Button>
              <Button type="submit" variant="primary" disabled={busy}>{busy ? t("settings.adding") : t("settings.add")}</Button>
            </div>
          </form>
        </Modal>
      )}
    </div>
  );
}
