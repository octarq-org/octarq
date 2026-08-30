import { useEffect, useState } from "react";
import { api } from "../../../api";
import { linksApi, Link } from "../api";
import { Field, Toggle, Button, Select, FormError, Input, Textarea } from "../../../ui";
import { Sparkles, Trash2 } from "lucide-react";
import { useTranslation } from "../../../i18n";

export function LinkEditorForm({
  link,
  hosts,
  onCancel,
  onSaved,
}: {
  link: Link | null;
  hosts: string[];
  onCancel: () => void;
  onSaved: (l?: any) => void;
}) {
  const { t } = useTranslation();
  const [slug, setSlug] = useState(link?.slug ?? "");
  // New links default to the first real link host when one exists; editing an
  // existing link always keeps its own host. A bare "" default would drop every
  // new Cloud link into the cross-tenant host='' shared pool.
  const [host, setHost] = useState(() => (link ? (link.host ?? "") : hosts.length > 0 ? hosts[0] : ""));
  const [target, setTarget] = useState(link?.target ?? "");
  const [title, setTitle] = useState(link?.title ?? "");
  const [note, setNote] = useState(link?.note ?? "");
  const [tags, setTags] = useState(link?.tags ?? "");
  const [password, setPassword] = useState(link?.password ?? "");
  const [expiresAt, setExpiresAt] = useState(link?.expiresAt?.slice(0, 16) ?? "");
  const [expiredUrl, setExpiredUrl] = useState(link?.expiredUrl ?? "");
  const [clickLimit, setClickLimit] = useState(link?.clickLimit ?? 0);
  const [enabled, setEnabled] = useState(link?.enabled ?? true);
  const [routingRules, setRoutingRules] = useState<any[]>(link?.routingRules ?? []);
  const [err, setErr] = useState<string | { message?: string; status?: number; requestId?: string }>("");
  const [fetching, setFetching] = useState(false);
  const [showUtm, setShowUtm] = useState(false);
  const [aiEnabled, setAiEnabled] = useState(false);
  const [aiBusy, setAiBusy] = useState(false);
  const [aiSlugs, setAiSlugs] = useState<string[]>([]);

  useEffect(() => {
    api.aiAssistStatus().then((s) => setAiEnabled(s.configured)).catch(() => {});
  }, []);

  async function suggestSlugs() {
    if (!target) return;
    setAiBusy(true);
    setAiSlugs([]);
    try {
      const r = await api.aiSuggestSlug(target, title || undefined);
      setAiSlugs(r.slugs);
    } catch {
      setErr(t("links.aiSuggestFailed"));
    } finally {
      setAiBusy(false);
    }
  }

  async function fetchTitle() {
    if (!target) return;
    setFetching(true);
    try {
      const m = await linksApi.linkMetadata(target);
      if (m.title) setTitle(m.title);
    } catch {
      /* ignore */
    } finally {
      setFetching(false);
    }
  }

  async function save() {
    setErr("");
    const payload: any = {
      slug,
      host,
      target,
      title,
      note,
      tags,
      password,
      enabled,
      expiredUrl,
      routingRules,
      clickLimit: Number(clickLimit) || 0,
      expiresAt: expiresAt ? new Date(expiresAt).toISOString() : null,
    };
    try {
      let res;
      if (link) res = await linksApi.updateLink(link.id, payload);
      else res = await linksApi.createLink(payload);
      onSaved(res);
    } catch (e: any) {
      // The backend refuses hostless links on multi-tenant instances. That
      // specific 400 gets a localized line; everything else keeps the server's
      // own message (FormError's normal contract).
      if (e?.status === 400 && typeof e?.message === "string" && e.message.includes("host is required")) {
        setErr({ message: t("links.hostRequired"), status: 400, requestId: e.requestId });
      } else {
        setErr(e);
      }
    }
  }

  return (
    <div className="space-y-4 min-w-0">
      <Field label={t("links.destinationTargetUrl")}>
        <div className="flex gap-2 items-start min-w-0">
          <Textarea
            className="w-full font-mono text-sm min-w-0"
            rows={3}
            value={target}
            onChange={(e) => setTarget(e.target.value)}
            placeholder="https://example.com/blog-post-xyz"
            required
          />
          <Button variant="subtle" className="shrink-0 text-xs py-2 mt-0.5" type="button" onClick={() => setShowUtm((v) => !v)}>
            UTM
          </Button>
        </div>
      </Field>
      {showUtm && <UtmBuilder target={target} onApply={setTarget} />}
      
      <div className="grid grid-cols-1 sm:grid-cols-2 gap-4 min-w-0">
        <Field label={t("links.shortSlug")} hint={t("links.shortSlugHint")}>
          <div className="flex gap-2 min-w-0">
            <Input className="w-full font-mono min-w-0" value={slug} onChange={(e) => setSlug(e.target.value)} placeholder="e.g. promo2026" />
            {aiEnabled && (
              <Button variant="subtle" className="shrink-0 text-xs py-1 gap-1" type="button" onClick={suggestSlugs} disabled={aiBusy || !target}>
                <Sparkles className="h-3.5 w-3.5" />
                {aiBusy ? t("links.aiSuggesting") : t("links.aiSuggest")}
              </Button>
            )}
          </div>
          {aiSlugs.length > 0 && (
            <div className="mt-2 flex flex-wrap gap-1.5 min-w-0">
              {aiSlugs.map((s) => (
                <button
                  key={s}
                  type="button"
                  onClick={() => setSlug(s)}
                  className="rounded-lg bg-info-bg border border-info-border px-2.5 py-1 text-xs font-mono text-info-fg hover:bg-info-fg/10 truncate max-w-full"
                >
                  {s}
                </button>
              ))}
            </div>
          )}
        </Field>
        <Field label={t("links.routingHostDomain")} hint={hosts.length ? t("links.configuredDomains") : t("links.configureDomainsFirst")}>
          <Select
            value={host}
            onValueChange={setHost}
            options={[
              // The hostless "Default (Apex Domain)" option is only offered when
              // no link host exists; once one does, exposing it would let users
              // drop their links back into the shared host='' pool.
              ...(hosts.length === 0 ? [{ value: "", label: t("links.defaultApexDomain") }] : []),
              ...hosts.map((h) => ({ value: h, label: h })),
              ...(host && !hosts.includes(host) ? [{ value: host, label: host }] : []),
            ]}
          />
        </Field>
      </div>

      <Field label={t("links.metadataPageTitle")}>
        <div className="flex gap-2 min-w-0">
          <Input className="w-full text-sm min-w-0" value={title} onChange={(e) => setTitle(e.target.value)} placeholder={t("links.metadataPlaceholder")} />
          <Button variant="subtle" className="shrink-0 text-xs py-1" type="button" onClick={fetchTitle} disabled={fetching}>
            {fetching ? t("links.fetching") : t("links.fetch")}
          </Button>
        </div>
      </Field>

      <Field label={t("links.tags")} hint={t("links.tagsHint")}>
        <Input className="w-full text-sm min-w-0" value={tags} onChange={(e) => setTags(e.target.value)} placeholder="e.g. q3-ads, product-hunt" />
      </Field>

      <Field label={t("links.internalAdminNote")}>
        <Textarea className="w-full text-sm min-w-0" rows={2} value={note} onChange={(e) => setNote(e.target.value)} placeholder={t("links.notePlaceholder")} />
      </Field>

      <div className="grid grid-cols-1 sm:grid-cols-2 gap-4 min-w-0">
        <Field label={t("links.accessProtectionPassword")} hint={t("links.passwordOptionalHint")}>
          <Input className="w-full font-mono text-sm min-w-0" type="text" value={password} onChange={(e) => setPassword(e.target.value)} placeholder="e.g. 123456" />
        </Field>
        <Field label={t("links.totalClickLimitation")} hint={t("links.clickLimitHint")}>
          <Input
            type="number"
            min={0}
            className="w-full font-mono min-w-0"
            value={clickLimit}
            onChange={(e) => setClickLimit(Number(e.target.value))}
          />
        </Field>
      </div>

      <div className="grid grid-cols-1 sm:grid-cols-2 gap-4 min-w-0">
        <Field label={t("links.automaticExpiryDate")}>
          <Input type="datetime-local" className="w-full text-sm min-w-0" value={expiresAt} onChange={(e) => setExpiresAt(e.target.value)} />
        </Field>
        <Field label={t("links.redirectUrlAfterExpiry")} hint={t("links.redirectUrlHint")}>
          <Input className="w-full text-sm font-mono min-w-0" value={expiredUrl} onChange={(e) => setExpiredUrl(e.target.value)} placeholder="e.g. https://my-site.com/expired" />
        </Field>
      </div>

      <div className="flex items-center gap-3 pt-2 min-w-0">
        <Toggle on={enabled} onChange={setEnabled} />
        <span className="text-sm text-foreground/60 select-none">{t("links.linkRoutingActive")}</span>
      </div>

      <Field label={t("links.routingRules")}>
        <RoutingRulesEditor rules={routingRules} onChange={setRoutingRules} />
      </Field>

      {err && <FormError err={err} />}

      <div className="flex justify-end gap-2.5 pt-4 border-t border-foreground/[0.06] min-w-0">
        <Button variant="ghost" onClick={onCancel}>
          {t("links.cancel")}
        </Button>
        <Button variant="primary" onClick={save} disabled={!target}>
          {t("links.saveLink")}
        </Button>
      </div>
    </div>
  );
}


function UtmBuilder({ target, onApply }: { target: string; onApply: (url: string) => void }) {
  const { t } = useTranslation();
  const [utm, setUtm] = useState({ source: "", medium: "", campaign: "", term: "", content: "" });
  function apply() {
    if (!target) return;
    let base = target;
    if (!base.includes("://")) base = "https://" + base;
    try {
      const u = new URL(base);
      const map: Record<string, string> = {
        utm_source: utm.source,
        utm_medium: utm.medium,
        utm_campaign: utm.campaign,
        utm_term: utm.term,
        utm_content: utm.content,
      };
      for (const [k, v] of Object.entries(map)) {
        if (v) u.searchParams.set(k, v);
        else u.searchParams.delete(k);
      }
      onApply(u.toString());
    } catch {
      /* ignore */
    }
  }
  const fields: [keyof typeof utm, string][] = [
    ["source", "source"],
    ["medium", "medium"],
    ["campaign", "campaign"],
    ["term", "term"],
    ["content", "content"],
  ];
  return (
    <div className="grid grid-cols-1 sm:grid-cols-2 md:grid-cols-3 gap-2.5 p-4 bg-well border border-foreground/[0.05] rounded-xl min-w-0">
      {fields.map(([k, label]) => (
        <Input
          key={k}
          className="w-full text-xs h-8 min-w-0"
          placeholder={`utm_${label}`}
          value={utm[k]}
          onChange={(e) => setUtm({ ...utm, [k]: e.target.value })}
        />
      ))}
      <Button variant="subtle" className="sm:col-span-2 md:col-span-3 h-8 text-xs py-1.5" onClick={apply}>
        {t("links.applyUtmParameters")}
      </Button>
    </div>
  );
}

function RoutingRulesEditor({ rules, onChange }: { rules: any[]; onChange: (r: any[]) => void }) {
  const { t } = useTranslation();
  const types = [
    { value: "geo", label: t("links.ruleTypeGeo") },
    { value: "device", label: t("links.ruleTypeDevice") },
    { value: "os", label: t("links.ruleTypeOS") },
    { value: "language", label: t("links.ruleTypeLanguage") },
    { value: "split", label: t("links.ruleTypeSplit") },
  ];

  const splitTotal = rules.filter((r) => r.type === "split").reduce((acc, r) => acc + (Number(r.weight) || 0), 0);
  const splitRem = Math.max(0, 100 - splitTotal);

  return (
    <div className="space-y-3 min-w-0">
      {rules.map((rule, i) => (
        <div key={i} className="flex gap-2 items-start bg-well p-3 rounded-lg border border-foreground/[0.05] min-w-0">
          <div className="flex-1 space-y-2 min-w-0">
            <div className="flex gap-2 min-w-0">
              <div className="w-1/3 min-w-[100px] shrink-0">
                <Select
                  value={rule.type}
                  onValueChange={(v) => {
                    const next = [...rules];
                    next[i] = { ...rule, type: v };
                    if (v === "split") {
                      delete next[i].match;
                      next[i].weight = 50;
                    } else {
                      delete next[i].weight;
                      next[i].match = "";
                    }
                    onChange(next);
                  }}
                  options={types}
                />
              </div>
              <div className="flex-1 min-w-0">
                {rule.type === "split" ? (
                  <Input
                    type="number"
                    min={0}
                    max={100}
                    className="w-full text-sm font-mono min-w-0"
                    placeholder={t("links.ruleWeight")}
                    value={rule.weight ?? ""}
                    onChange={(e) => {
                      const next = [...rules];
                      next[i] = { ...rule, weight: Number(e.target.value) };
                      onChange(next);
                    }}
                  />
                ) : (
                  <Input
                    className="w-full text-sm font-mono min-w-0"
                    placeholder={t("links.ruleMatch")}
                    value={rule.match ?? ""}
                    onChange={(e) => {
                      const next = [...rules];
                      next[i] = { ...rule, match: e.target.value };
                      onChange(next);
                    }}
                  />
                )}
              </div>
            </div>
            <Input
              className="w-full text-sm font-mono min-w-0"
              placeholder={t("links.ruleTarget")}
              value={rule.target ?? ""}
              onChange={(e) => {
                const next = [...rules];
                next[i] = { ...rule, target: e.target.value };
                onChange(next);
              }}
            />
          </div>
          <Button variant="ghost" className="shrink-0 p-2 text-danger-fg" onClick={() => {
            const next = [...rules];
            next.splice(i, 1);
            onChange(next);
          }}>
            <Trash2 className="h-4 w-4" />
          </Button>
        </div>
      ))}
      
      <div className="flex items-center justify-between mt-2 flex-wrap gap-2 min-w-0">
        <Button variant="subtle" className="text-xs py-1.5" onClick={() => onChange([...rules, { type: "split", weight: 50, target: "" }])}>
          + {t("links.addRule")}
        </Button>
        
        {rules.some((r) => r.type === "split") && (
          <div className="text-xs text-foreground/60 font-mono">
            {t("links.splitTotalLabel", { total: splitTotal })} · {t("links.ruleSplitRemainder")}: {splitRem}%
          </div>
        )}
      </div>
    </div>
  );
}

