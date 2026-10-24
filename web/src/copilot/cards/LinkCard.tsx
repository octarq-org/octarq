/**
 * LinkCard: Short-link management card.
 * Supports create, update, copy-to-clipboard, and enabled/disabled toggle.
 *
 * All user copy strictly conforms to ASD-STE100 (<= 20 words/sentence, active voice).
 */
import React, { useState } from "react";
import { Button } from "@octarq/plugin-sdk";
import { Copy, Check, ExternalLink, Globe } from "lucide-react";
import { useTranslation } from "../../i18n";
import type { ToolCardProps } from "./registry";
import { CardShell, type CardState } from "./CardShell";

export interface LinkArgs {
  host?: string;
  slug?: string;
  target?: string;
  enabled?: boolean;
  short_url?: string;
  shortUrl?: string;
}

export function LinkCard({
  args,
  onAction,
  state,
}: ToolCardProps & { state?: CardState }) {
  const { t } = useTranslation();
  const initial = (args ?? {}) as LinkArgs;
  const [slug, setSlug] = useState(initial.slug ?? "");
  const [target, setTarget] = useState(initial.target ?? "");
  const [enabled, setEnabled] = useState(initial.enabled ?? true);
  const [copied, setCopied] = useState(false);

  const shortURL = initial.short_url ?? initial.shortUrl ?? "";
  const canSubmit = slug.trim() !== "" && target.trim() !== "";

  const handleCopy = async () => {
    if (!shortURL) return;
    try {
      await navigator.clipboard.writeText(shortURL);
      setCopied(true);
      setTimeout(() => setCopied(false), 2000);
    } catch {
      setCopied(false);
    }
  };

  const handleToggleEnabled = (newVal: boolean) => {
    setEnabled(newVal);
    void onAction({
      type: "custom",
      payload: {
        action: "links.set_enabled",
        host: initial.host ?? "",
        slug: slug.trim(),
        enabled: newVal,
      },
    });
  };

  return (
    <CardShell title={t("copilot.cardShortLink", "Short Link")} state={state}>
      <div className="space-y-2">
        <div>
          <label htmlFor="link-slug-input" className="block text-[11px] font-semibold text-muted-foreground uppercase tracking-wider">
            {t("copilot.cardSlug", "Slug")}
          </label>
          <input
            id="link-slug-input"
            aria-label={t("copilot.cardSlug", "Slug")}
            className="mt-1 w-full rounded-lg border border-border bg-background px-2.5 py-1.5 font-mono text-xs text-foreground focus:border-primary focus:outline-none"
            value={slug}
            onChange={(e) => setSlug(e.target.value)}
          />
        </div>

        <div>
          <label htmlFor="link-target-input" className="block text-[11px] font-semibold text-muted-foreground uppercase tracking-wider">
            {t("copilot.cardTargetUrl", "Target Destination URL")}
          </label>
          <input
            id="link-target-input"
            aria-label={t("copilot.cardTargetUrl", "Target Destination URL")}
            className="mt-1 w-full rounded-lg border border-border bg-background px-2.5 py-1.5 font-mono text-xs text-foreground focus:border-primary focus:outline-none"
            value={target}
            onChange={(e) => setTarget(e.target.value)}
          />
        </div>

        <div className="flex flex-wrap items-center justify-between gap-2 pt-1 border-t border-border/40">
          <div className="flex items-center gap-2">
            <Button
              size="sm"
              variant="primary"
              disabled={!canSubmit}
              onClick={() =>
                void onAction({
                  type: "custom",
                  payload: {
                    action: initial.slug ? "links.update" : "links.create",
                    host: initial.host ?? "",
                    slug: slug.trim(),
                    target: target.trim(),
                    enabled,
                  },
                })
              }
              className="h-7 text-xs"
            >
              {initial.slug ? t("copilot.cardSaveChanges", "Save Changes") : t("copilot.cardCreateLink", "Create Link")}
            </Button>

            {shortURL && (
              <Button
                size="sm"
                variant="secondary"
                onClick={handleCopy}
                className="h-7 text-xs flex items-center gap-1"
              >
                {copied ? <Check className="h-3.5 w-3.5 text-success-fg" /> : <Copy className="h-3.5 w-3.5" />}
                <span>{copied ? t("copilot.cardCopied", "Copied") : t("copilot.cardCopy", "Copy")}</span>
              </Button>
            )}
          </div>

          <label className="flex items-center gap-1.5 text-xs text-muted-foreground cursor-pointer select-none">
            <input
              type="checkbox"
              checked={enabled}
              onChange={(e) => handleToggleEnabled(e.target.checked)}
              className="rounded border-border"
            />
            <span>{t("copilot.cardEnabled", "Enabled")}</span>
          </label>
        </div>

        {shortURL && (
          <div className="flex items-center gap-1.5 pt-1 text-[11px] font-mono text-muted-foreground">
            <Globe className="h-3 w-3 shrink-0" />
            <span data-testid="short-url" className="truncate">{shortURL}</span>
          </div>
        )}
      </div>
    </CardShell>
  );
}
