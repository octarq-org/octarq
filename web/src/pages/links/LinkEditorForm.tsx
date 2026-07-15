import { useEffect, useState } from "react";
import { api, Domain, effectiveLinkHosts, Link, LinkStats } from "../../api";
import { Empty, Field, Toggle, timeAgo, ScreenWrap, PageHeader, GlassCard, Badge, Button, StatCard, Select } from "../../ui";
import { Link2, Copy, Archive, Trash2, QrCode, Download, Eye, ExternalLink, Calendar, Search, Tag, Globe, Settings, Sparkles } from "lucide-react";
import { LinkSettings } from "../Settings";
import { useTranslation } from "../../i18n";

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
  const [host, setHost] = useState(link?.host ?? "");
  const [target, setTarget] = useState(link?.target ?? "");
  const [title, setTitle] = useState(link?.title ?? "");
  const [note, setNote] = useState(link?.note ?? "");
  const [tags, setTags] = useState(link?.tags ?? "");
  const [password, setPassword] = useState("");
  const [expiresAt, setExpiresAt] = useState(link?.expiresAt?.slice(0, 16) ?? "");
  const [expiredUrl, setExpiredUrl] = useState(link?.expiredUrl ?? "");
  const [clickLimit, setClickLimit] = useState(link?.clickLimit ?? 0);
  const [enabled, setEnabled] = useState(link?.enabled ?? true);
  const [err, setErr] = useState("");
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
      const m = await api.linkMetadata(target);
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
      clickLimit: Number(clickLimit) || 0,
      expiresAt: expiresAt ? new Date(expiresAt).toISOString() : null,
    };
    try {
      let res;
      if (link) res = await api.updateLink(link.id, payload);
      else res = await api.createLink(payload);
      onSaved(res);
    } catch (e: any) {
      setErr(e.message ?? t("links.saveFailed"));
    }
  }

  return (
    <div className="space-y-4">
      <Field label={t("links.destinationTargetUrl")}>
        <div className="flex gap-2 items-start">
          <textarea
            className="input w-full font-mono text-sm resize-y"
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
      
      <div className="grid grid-cols-1 sm:grid-cols-2 gap-4">
        <Field label={t("links.shortSlug")} hint={t("links.shortSlugHint")}>
          <div className="flex gap-2">
            <input className="input w-full font-mono" value={slug} onChange={(e) => setSlug(e.target.value)} placeholder="e.g. promo2026" />
            {aiEnabled && (
              <Button variant="subtle" className="shrink-0 text-xs py-1 gap-1" type="button" onClick={suggestSlugs} disabled={aiBusy || !target}>
                <Sparkles className="h-3.5 w-3.5" />
                {aiBusy ? t("links.aiSuggesting") : t("links.aiSuggest")}
              </Button>
            )}
          </div>
          {aiSlugs.length > 0 && (
            <div className="mt-2 flex flex-wrap gap-1.5">
              {aiSlugs.map((s) => (
                <button
                  key={s}
                  type="button"
                  onClick={() => setSlug(s)}
                  className="rounded-lg bg-indigo-500/10 border border-indigo-400/20 px-2.5 py-1 text-xs font-mono text-indigo-300 hover:bg-indigo-500/20"
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
              { value: "", label: t("links.defaultApexDomain") },
              ...hosts.map((h) => ({ value: h, label: h })),
              ...(host && !hosts.includes(host) ? [{ value: host, label: host }] : []),
            ]}
          />
        </Field>
      </div>

      <Field label={t("links.metadataPageTitle")}>
        <div className="flex gap-2">
          <input className="input w-full text-sm" value={title} onChange={(e) => setTitle(e.target.value)} placeholder={t("links.metadataPlaceholder")} />
          <Button variant="subtle" className="shrink-0 text-xs py-1" type="button" onClick={fetchTitle} disabled={fetching}>
            {fetching ? t("links.fetching") : t("links.fetch")}
          </Button>
        </div>
      </Field>

      <Field label={t("links.tags")} hint={t("links.tagsHint")}>
        <input className="input w-full text-sm" value={tags} onChange={(e) => setTags(e.target.value)} placeholder="e.g. q3-ads, product-hunt" />
      </Field>

      <Field label={t("links.internalAdminNote")}>
        <textarea className="input w-full text-sm" rows={2} value={note} onChange={(e) => setNote(e.target.value)} placeholder={t("links.notePlaceholder")} />
      </Field>

      <div className="grid grid-cols-1 sm:grid-cols-2 gap-4">
        <Field label={t("links.accessProtectionPassword")} hint={link?.hasPassword ? t("links.passwordSetHint") : t("links.passwordOptionalHint")}>
          <input className="input w-full font-mono text-sm" type="password" value={password} onChange={(e) => setPassword(e.target.value)} placeholder="••••••••" />
        </Field>
        <Field label={t("links.totalClickLimitation")} hint={t("links.clickLimitHint")}>
          <input
            type="number"
            min={0}
            className="input w-full font-mono"
            value={clickLimit}
            onChange={(e) => setClickLimit(Number(e.target.value))}
          />
        </Field>
      </div>

      <div className="grid grid-cols-1 sm:grid-cols-2 gap-4">
        <Field label={t("links.automaticExpiryDate")}>
          <input type="datetime-local" className="input w-full text-sm" value={expiresAt} onChange={(e) => setExpiresAt(e.target.value)} />
        </Field>
        <Field label={t("links.redirectUrlAfterExpiry")} hint={t("links.redirectUrlHint")}>
          <input className="input w-full text-sm font-mono" value={expiredUrl} onChange={(e) => setExpiredUrl(e.target.value)} placeholder="e.g. https://my-site.com/expired" />
        </Field>
      </div>

      <div className="flex items-center gap-3 pt-2">
        <Toggle on={enabled} onChange={setEnabled} />
        <span className="text-sm text-white/60 select-none">{t("links.linkRoutingActive")}</span>
      </div>

      {err && <p className="text-sm text-rose-400 font-medium">{err}</p>}

      <div className="flex justify-end gap-2.5 pt-4 border-t border-white/[0.06]">
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
    <div className="grid grid-cols-1 sm:grid-cols-2 md:grid-cols-3 gap-2.5 p-4 bg-black/30 border border-white/[0.05] rounded-xl">
      {fields.map(([k, label]) => (
        <input
          key={k}
          className="input w-full text-xs h-8"
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

