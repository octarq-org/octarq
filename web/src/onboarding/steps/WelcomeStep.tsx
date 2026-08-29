import { Bot, Globe, Link2, Mail, Sparkles } from "lucide-react";
import { Button } from "../../ui";
import { useTranslation } from "../../i18n";

interface WelcomeStepProps {
  onNext: () => void;
}

export function WelcomeStep({ onNext }: WelcomeStepProps) {
  const { t } = useTranslation();

  return (
    <div className="flex flex-col items-center text-center max-w-xl mx-auto space-y-6">
      <div className="inline-flex items-center gap-2 px-3 py-1 rounded-full bg-accent-soft border border-accent-border text-xs font-medium text-accent-fg">
        <Sparkles className="w-3.5 h-3.5" />
        <span>{t("onboarding.welcomeBadge")}</span>
      </div>

      <div className="space-y-3">
        <h1 className="text-3xl sm:text-4xl font-bold tracking-tight text-foreground">
          {t("onboarding.welcomeTitle")}
        </h1>
        <p className="text-lg font-medium text-foreground/80">
          {t("onboarding.welcomeSubtitle")}
        </p>
        <p className="text-sm text-muted-foreground max-w-md mx-auto">
          {t("onboarding.welcomeDesc")}
        </p>
      </div>

      <div className="w-full glass rounded-2xl p-5 sm:p-6 border border-border space-y-4 text-left shadow-lg">
        <div className="flex items-center justify-between border-b border-border pb-3">
          <div className="flex items-center gap-2">
            <div className="w-3 h-3 rounded-full bg-destructive/60" />
            <div className="w-3 h-3 rounded-full bg-warning-fg/60" />
            <div className="w-3 h-3 rounded-full bg-success-fg/60" />
          </div>
          <span className="font-mono text-xs text-muted-foreground">{t("onboarding.coreVersion")}</span>
        </div>

        <div className="grid grid-cols-2 sm:grid-cols-4 gap-3 pt-1">
          <div className="rounded-xl p-3 bg-surface-hover/50 border border-border/50 flex flex-col items-center text-center gap-2">
            <Link2 className="w-5 h-5 text-accent-fg" />
            <span className="text-xs font-medium">{t("onboarding.welcomeServiceLinks")}</span>
          </div>
          <div className="rounded-xl p-3 bg-surface-hover/50 border border-border/50 flex flex-col items-center text-center gap-2">
            <Mail className="w-5 h-5 text-accent-fg" />
            <span className="text-xs font-medium">{t("onboarding.welcomeServiceMail")}</span>
          </div>
          <div className="rounded-xl p-3 bg-surface-hover/50 border border-border/50 flex flex-col items-center text-center gap-2">
            <Globe className="w-5 h-5 text-accent-fg" />
            <span className="text-xs font-medium">{t("onboarding.welcomeServiceDns")}</span>
          </div>
          <div className="rounded-xl p-3 bg-surface-hover/50 border border-border/50 flex flex-col items-center text-center gap-2">
            <Bot className="w-5 h-5 text-accent-fg" />
            <span className="text-xs font-medium">{t("onboarding.welcomeServiceMcp")}</span>
          </div>
        </div>
      </div>

      <div className="pt-2 w-full flex justify-center">
        <Button size="lg" onClick={onNext} className="w-full sm:w-auto px-8">
          {t("onboarding.startSetup")}
        </Button>
      </div>
    </div>
  );
}
