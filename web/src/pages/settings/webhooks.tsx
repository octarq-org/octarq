import { useEffect, useState } from "react";
import { api, WebhookEventGroup } from "../../api";
import { Field, Modal, Toggle, PageHeader, GlassCard, Button, toast, confirmDialog } from "../../ui";
import { Trash2, Plus, Send } from "lucide-react";
import { useTranslation } from "../../i18n";
import { roleSatisfies, useCurrentRole } from "../../shell/role";

export function WebhooksSettings() {
  const { t } = useTranslation();
  const { role, isInstanceAdmin } = useCurrentRole();
  const canManage = roleSatisfies("admin", role, isInstanceAdmin);
  const [webhooks, setWebhooks] = useState<any[]>([]);
  const [show, setShow] = useState(false);
  const [name, setName] = useState("");
  const [url, setUrl] = useState("");
  const [secret, setSecret] = useState("");
  const [all, setAll] = useState(true);
  const [selected, setSelected] = useState<Set<string>>(new Set());
  const [eventGroups, setEventGroups] = useState<WebhookEventGroup[] | null>(null);
  const [busy, setBusy] = useState(false);
  const [testingId, setTestingId] = useState<number | null>(null);

  function load() { api.webhooks().then(setWebhooks).catch(() => {}); }
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

  return (
    <div className="space-y-6">
      <PageHeader title={t("settings.webhooksTitle")} description={t("settings.webhooksDescription")} />
      <GlassCard className="p-6 space-y-4">
        <div className="flex items-center justify-between">
          <h2 className="text-base font-bold text-foreground">{t("settings.outboundEventWebhooks")}</h2>
          {canManage && (
            <Button variant="ghost" onClick={() => { setName(""); setUrl(""); setSecret(""); setAll(true); setSelected(new Set()); setShow(true); }} className="flex items-center gap-1.5 px-3 py-1 text-xs">
              <Plus className="h-3 w-3" /> {t("settings.addWebhook")}
            </Button>
          )}
        </div>
        {webhooks.length === 0 ? (
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
                    <Toggle on={w.enabled} onChange={() => toggle(w)} />
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

      {show && (
        <Modal title={t("settings.addWebhookEndpoint")} onClose={() => setShow(false)}>
          <form onSubmit={create} className="space-y-4">
            <Field label={t("settings.endpointName")}><input className="input w-full" value={name} onChange={(e) => setName(e.target.value)} placeholder="n8n automation" required autoFocus /></Field>
            <Field label={t("settings.endpointUrl")}><input className="input w-full font-mono text-xs" value={url} onChange={(e) => setUrl(e.target.value)} placeholder="https://your-server.com/webhooks/octarq" required /></Field>
            <Field label={t("settings.signingSecretOptional")} hint={t("settings.signingSecretHint")}>
              <input className="input w-full font-mono text-xs" value={secret} onChange={(e) => setSecret(e.target.value)} placeholder={t("settings.signingSecretPlaceholder")} />
            </Field>
            <Field label={t("settings.eventSubscriptions")}>
              <div className="mt-1 space-y-2">
                <label className="flex cursor-pointer items-center gap-2 text-xs text-zinc-300">
                  <input type="radio" name="webhook-events-mode" checked={all} onChange={() => setAll(true)} />
                  <span>{t("settings.allEventsStar")}</span>
                </label>
                <label className="flex cursor-pointer items-center gap-2 text-xs text-zinc-300">
                  <input type="radio" name="webhook-events-mode" checked={!all} onChange={() => setAll(false)} />
                  <span>{t("settings.customEvents")}</span>
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
