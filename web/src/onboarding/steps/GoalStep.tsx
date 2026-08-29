import { useState } from "react";
import { Bot, Boxes, Check, Globe, Link2, Mail } from "lucide-react";
import { Button, cn } from "../../ui";
import { useTranslation } from "../../i18n";

interface GoalStepProps {
  initialValue?: string;
  onNext: (data: { goal: string }) => void;
  onBack: () => void;
}

const GOALS = [
  {
    id: "marketing",
    icon: Link2,
    titleKey: "onboarding.goalMarketing",
    descKey: "onboarding.goalMarketingDesc",
  },
  {
    id: "email",
    icon: Mail,
    titleKey: "onboarding.goalEmail",
    descKey: "onboarding.goalEmailDesc",
  },
  {
    id: "dns",
    icon: Globe,
    titleKey: "onboarding.goalDns",
    descKey: "onboarding.goalDnsDesc",
  },
  {
    id: "mcp",
    icon: Bot,
    titleKey: "onboarding.goalMcp",
    descKey: "onboarding.goalMcpDesc",
  },
  {
    id: "distribution",
    icon: Boxes,
    titleKey: "onboarding.goalDistribution",
    descKey: "onboarding.goalDistributionDesc",
  },
];

export function GoalStep({ initialValue = "", onNext, onBack }: GoalStepProps) {
  const { t } = useTranslation();
  const [selected, setSelected] = useState<string>(initialValue);

  return (
    <div className="flex flex-col max-w-xl mx-auto space-y-6">
      <div className="text-center space-y-2">
        <h2 className="text-2xl sm:text-3xl font-bold tracking-tight text-foreground">
          {t("onboarding.goalTitle")}
        </h2>
        <p className="text-sm text-muted-foreground">{t("onboarding.goalSubtitle")}</p>
      </div>

      <div className="space-y-3">
        {GOALS.map((g) => {
          const Icon = g.icon;
          const isSelected = selected === g.id;
          return (
            <button
              key={g.id}
              type="button"
              onClick={() => setSelected(g.id)}
              className={cn(
                "w-full flex items-start gap-4 p-4 rounded-2xl border text-left transition-all",
                isSelected
                  ? "bg-accent-soft/40 border-accent-border shadow-sm ring-1 ring-ring"
                  : "glass hover:bg-surface-hover border-border/80",
              )}
            >
              <div
                className={cn(
                  "p-2.5 rounded-xl shrink-0 mt-0.5 transition-colors",
                  isSelected
                    ? "bg-accent-soft text-accent-fg"
                    : "bg-surface-hover text-muted-foreground",
                )}
              >
                <Icon className="w-5 h-5" />
              </div>
              <div className="flex-1 min-w-0 space-y-1">
                <div className="flex items-center justify-between">
                  <span className="font-semibold text-sm sm:text-base text-foreground">
                    {t(g.titleKey)}
                  </span>
                  {isSelected && (
                    <div className="w-5 h-5 rounded-full bg-primary flex items-center justify-center text-primary-foreground shrink-0">
                      <Check className="w-3.5 h-3.5" />
                    </div>
                  )}
                </div>
                <p className="text-xs sm:text-sm text-muted-foreground line-clamp-2">
                  {t(g.descKey)}
                </p>
              </div>
            </button>
          );
        })}
      </div>

      <div className="flex items-center justify-between pt-2">
        <Button variant="ghost" onClick={onBack}>
          {t("onboarding.back")}
        </Button>
        <Button disabled={!selected} onClick={() => onNext({ goal: selected })}>
          {t("onboarding.continue")}
        </Button>
      </div>
    </div>
  );
}
