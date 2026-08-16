import { useCallback, useEffect, useState } from "react";
import { api } from "../../../api";
import { Field, Toggle, Button, FormError } from "../../../ui";
import { useTranslation } from "../../../i18n";
import { useSettingsData, SavedBadge } from "../../../pages/settings/shared";

const MASK = "••••••••";

export function MailSettings() {
  const { t } = useTranslation();
  const { s } = useSettingsData();
  const [reservedMailboxes, setReservedMailboxes] = useState("");
  const [inboundToken, setInboundToken] = useState("");
  const [tokenLoaded, setTokenLoaded] = useState(false);
  const [revealed, setRevealed] = useState(false);
  const [copied, setCopied] = useState(false);
  const [catchAll, setCatchAll] = useState(false);
  const [autoWrap, setAutoWrap] = useState(false);
  const [busy, setBusy] = useState(false);
  const [saved, setSaved] = useState(false);
  const [err, setErr] = useState<{ message?: string; status?: number; requestId?: string } | null>(null);

  const adminView = s?.inboundTokenSet !== undefined;

  // The raw token is never part of the settings dump — it is fetched explicitly
  // from the admin-only /api/settings/inbound-token endpoint and masked until
  // the admin asks to see it.
  const loadToken = useCallback(async () => {
    if (tokenLoaded) return;
    try {
      const r = await api.inboundToken();
      setInboundToken(r.inboundToken);
      setTokenLoaded(true);
    } catch {
      // First-run org or a transient failure — keep the mask.
    }
  }, [tokenLoaded]);

  useEffect(() => { if (s) { setReservedMailboxes(s.reservedMailboxes); setCatchAll(s.catchAll || false); setAutoWrap(s.autoWrapLinks || false); } }, [s]);

  useEffect(() => { if (adminView) void loadToken(); }, [adminView, loadToken]);

  async function save() {
    setBusy(true);
    setErr(null);
    try {
      await api.updateSettings({
        reservedMailboxes,
        ...(tokenLoaded ? { inboundToken } : {}),
        catchAll,
        autoWrapLinks: autoWrap,
      });
      setSaved(true);
      setTimeout(() => setSaved(false), 2000);
      if (tokenLoaded) {
        // A save may have rotated the token (clearing it mints a fresh one);
        // refresh so the displayed value and URL stay accurate.
        const r = await api.inboundToken();
        setInboundToken(r.inboundToken);
      }
    } catch (e: any) {
      setErr({ message: e?.message, status: e?.status, requestId: e?.requestId });
    } finally { setBusy(false); }
  }

  async function reveal() {
    await loadToken();
    setRevealed((v) => !v);
  }

  async function copyToken() {
    if (!tokenLoaded) await loadToken();
    if (!inboundToken) return;
    await navigator.clipboard.writeText(inboundToken);
    setCopied(true);
    setTimeout(() => setCopied(false), 1500);
  }

  if (!s) return <div className="text-sm text-foreground/40">{t("settings.loadingLower")}</div>;

  const displayed = tokenLoaded && revealed ? inboundToken : MASK;

  return (
    <div className="space-y-6">
      <div className="flex items-center justify-between">
        <h2 className="text-sm font-semibold text-foreground/90">{t("settings.inboundMailboxesSettings")}</h2>
        <SavedBadge on={saved} />
      </div>
      <Field label={t("settings.reservedMailboxesLabel")} hint={t("settings.reservedMailboxesHint")}>
        <textarea className="input w-full font-mono text-xs" rows={2} value={reservedMailboxes} onChange={(e) => setReservedMailboxes(e.target.value)} placeholder="admin&#10;postmaster" />
      </Field>
      {adminView && (
        <>
          <Field label={t("settings.inboundWebhookUrlLabel")} hint={t("settings.inboundWebhookUrlHint")}>
            <input
              readOnly
              className="input w-full font-mono text-xs"
              value={`${location.origin}/api/v1/webhook/${s?.orgSlug || ""}/email/inbound/${displayed}`}
              onFocus={(e) => e.currentTarget.select()}
            />
          </Field>
          <Field label={t("settings.inboundTokenLabel")} hint={t("settings.inboundTokenHint")}>
            <div className="flex gap-2">
              <input
                className="input w-full font-mono text-xs"
                value={displayed}
                readOnly={!revealed || !tokenLoaded}
                onChange={(e) => setInboundToken(e.target.value)}
                placeholder={t("settings.inboundTokenPlaceholder")}
              />
              <Button variant="outline" className="shrink-0 text-xs" onClick={reveal}>
                {revealed ? t("settings.inboundTokenHide") : t("settings.inboundTokenReveal")}
              </Button>
              <Button variant="subtle" className="shrink-0 text-xs" onClick={copyToken}>
                {copied ? t("uiCommon.copied") : t("settings.inboundTokenCopy")}
              </Button>
            </div>
          </Field>
        </>
      )}
      <div className="flex items-center gap-3 border-t border-foreground/[0.04] pt-4">
        <Toggle on={catchAll} onChange={setCatchAll} />
        <div>
          <span className="block select-none text-xs font-semibold text-foreground/70">{t("settings.enableCatchAll")}</span>
          <span className="select-none text-[10px] text-foreground/40">{t("settings.enableCatchAllDesc")}</span>
        </div>
      </div>
      <div className="flex items-center gap-3 border-t border-foreground/[0.04] pt-4">
        <Toggle on={autoWrap} onChange={setAutoWrap} />
        <div>
          <span className="block select-none text-xs font-semibold text-foreground/70">{t("settings.autoWrapLinks")}</span>
          <span className="select-none text-[10px] text-foreground/40">{t("settings.autoWrapLinksDesc")}</span>
        </div>
      </div>
      <div className="border-t border-foreground/[0.06] pt-4 flex justify-end">
        <Button variant="primary" className="text-xs" onClick={save} disabled={busy}>{busy ? t("settings.saving") : t("settings.saveSettings")}</Button>
      </div>
      {err && (
        <div className="pt-2">
          <FormError err={err} />
        </div>
      )}
    </div>
  );
}
