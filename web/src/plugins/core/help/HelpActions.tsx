import { useEffect, useMemo, useState } from "react";
import { ArrowUpRight } from "lucide-react";
import { confirmDialog } from "@octarq/plugin-sdk";
import { api, type Action } from "../../../api";
import { useTranslation } from "../../../i18n";
import { roleSatisfies, useCurrentRole } from "../../../shell/role";
import { menuIcon } from "../../../shell/areas";

export interface HelpActionsProps {
  ids?: string[];
  onNavigate: (path: string) => void;
}

export function HelpActions({ ids, onNavigate }: HelpActionsProps) {
  const { t } = useTranslation();
  const { role, isInstanceAdmin } = useCurrentRole();
  const [available, setAvailable] = useState<Action[]>([]);
  const idsKey = useMemo(() => (ids ?? []).join("\u0000"), [ids]);

  useEffect(() => {
    if (!idsKey) return;
    let active = true;
    api
      .actions()
      .then((list) => {
        if (active) setAvailable(list);
      })
      .catch(() => {
        if (active) setAvailable([]);
      });
    return () => {
      active = false;
    };
  }, [idsKey]);

  const cards = useMemo(() => {
    return (ids ?? []).flatMap((id) => {
      const action = available.find((candidate) => candidate.id === id);
      return action && roleSatisfies(action.requiredRole, role, isInstanceAdmin) ? [action] : [];
    });
  }, [ids, available, role, isInstanceAdmin]);

  if (cards.length === 0) return null;

  const confirmAndOpen = async (action: Action) => {
    const ok = await confirmDialog({
      title: t("help.action_confirm_title", "Leave this article?"),
      message: t("help.action_confirm_message", "This opens {{label}}.", { label: action.label }),
      confirmLabel: t("help.action_confirm", "Open"),
      danger: false,
    });
    if (ok) onNavigate(action.path);
  };

  return (
    <section data-testid="help-actions" className="mt-8 border-t border-border pt-6">
      <h2 className="mb-3 text-xs font-semibold uppercase tracking-wide text-muted-foreground">
        {t("help.actions_title", "Actions for this page")}
      </h2>
      <div className="grid gap-2 sm:grid-cols-2">
        {cards.map((action) => {
          const Icon = menuIcon(action.icon) ?? ArrowUpRight;
          const hint = action.description || action.category;
          return (
            <button
              key={action.id}
              type="button"
              onClick={() => void confirmAndOpen(action)}
              className="flex w-full items-start gap-3 rounded-lg border border-border bg-card p-3 text-left transition-colors hover:bg-surface-hover"
            >
              <span className="flex h-7 w-7 shrink-0 items-center justify-center rounded-md bg-accent-soft text-accent-fg">
                <Icon className="h-4 w-4" />
              </span>
              <span className="min-w-0 flex-1">
                <span className="block text-sm font-semibold text-foreground">{action.label}</span>
                {hint && (
                  <span className="mt-0.5 block text-xs text-muted-foreground">{hint}</span>
                )}
              </span>
              <ArrowUpRight className="mt-0.5 h-4 w-4 shrink-0 text-muted-foreground" />
            </button>
          );
        })}
      </div>
    </section>
  );
}
