import { Copy, QrCode, Share2, Sparkles } from "lucide-react";
import { Button, toast } from "../../ui";
import { useTranslation } from "../../i18n";
import { DEMO_LINKS, DemoLinkItem } from "../types";

interface ValueStepProps {
  demoPicks?: string[];
  onNext: () => void;
  onBack: () => void;
}

export function ValueStep({ demoPicks = [], onNext, onBack }: ValueStepProps) {
  const { t } = useTranslation();

  const chosenLinks = DEMO_LINKS.filter((l) =>
    demoPicks.length > 0 ? demoPicks.includes(l.id) : ["launch-2026", "discord", "deck"].includes(l.id),
  ).slice(0, 3);

  const formattedUrls = chosenLinks.map(
    (l) => `https://${l.domain}/${l.slug}?utm_source=${l.utmSource}&utm_medium=${l.utmMedium}`,
  );

  const handleCopyAll = async () => {
    try {
      await navigator.clipboard.writeText(formattedUrls.join("\n"));
      toast.success(t("onboarding.copiedAll"));
    } catch {
      toast.error("Failed to copy");
    }
  };

  const handleShare = async () => {
    const text = formattedUrls.join("\n");
    if (typeof navigator !== "undefined" && navigator.share) {
      try {
        await navigator.share({
          title: "My Octarq Links",
          text,
        });
        toast.success(t("onboarding.sharedSuccess"));
        return;
      } catch {
        // User cancelled or failed -> fallback to clipboard
      }
    }
    try {
      await navigator.clipboard.writeText(text);
      toast.success(t("onboarding.copiedAll"));
    } catch {
      toast.error("Failed to copy");
    }
  };

  return (
    <div className="flex flex-col max-w-xl mx-auto space-y-6">
      <div className="text-center space-y-2">
        <div className="inline-flex items-center gap-1.5 px-3 py-1 rounded-full text-xs font-medium bg-accent-soft text-accent-fg border border-accent-border">
          <Sparkles className="w-3.5 h-3.5" />
          <span>{t("onboarding.linkPackBadge")}</span>
        </div>
        <h2 className="text-2xl sm:text-3xl font-bold tracking-tight text-foreground">
          {t("onboarding.valueTitle")}
        </h2>
        <p className="text-sm text-muted-foreground">{t("onboarding.valueSubtitle")}</p>
      </div>

      <div className="space-y-3">
        {chosenLinks.map((link: DemoLinkItem) => (
          <div
            key={link.id}
            className="p-4 rounded-2xl glass border border-border space-y-3 shadow-sm"
          >
            <div className="flex items-center justify-between">
              <span className="font-semibold text-sm text-foreground">
                {t(link.titleKey)}
              </span>
              <div className="flex items-center gap-1.5 text-xs text-muted-foreground bg-surface-hover px-2 py-0.5 rounded-md">
                <QrCode className="w-3.5 h-3.5 text-accent-fg" />
                <span>{t("onboarding.qrReady")}</span>
              </div>
            </div>

            <div className="bg-background/60 rounded-xl p-2.5 font-mono text-xs text-accent-fg border border-border/60 flex items-center justify-between">
              <span className="truncate">https://{link.domain}/{link.slug}</span>
            </div>

            <div className="flex items-center gap-2 text-[11px] text-muted-foreground">
              <span className="bg-surface-hover px-2 py-0.5 rounded">{`utm_source=${link.utmSource}`}</span>
              <span className="bg-surface-hover px-2 py-0.5 rounded">{`utm_medium=${link.utmMedium}`}</span>
            </div>
          </div>
        ))}
      </div>

      <div className="flex items-center justify-center gap-3 pt-1">
        <Button variant="secondary" onClick={handleCopyAll} className="gap-2">
          <Copy className="w-4 h-4" />
          {t("onboarding.copyAll")}
        </Button>
        <Button variant="secondary" onClick={handleShare} className="gap-2">
          <Share2 className="w-4 h-4" />
          {t("onboarding.share")}
        </Button>
      </div>

      <div className="flex items-center justify-between pt-2">
        <Button variant="ghost" onClick={onBack}>
          {t("onboarding.back")}
        </Button>
        <Button onClick={onNext}>
          {t("onboarding.continue")}
        </Button>
      </div>
    </div>
  );
}
