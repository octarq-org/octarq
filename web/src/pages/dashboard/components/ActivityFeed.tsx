import { GlassCard, Skeleton, timeAgo } from "@octarq/plugin-sdk";
import { useQuery } from "@tanstack/react-query";
import { ArrowRight, Bell, Inbox, ScrollText } from "lucide-react";
import { useNavigate } from "react-router-dom";
import { api, type AuditLog } from "../../../api";
import { useTranslation } from "../../../i18n";
import type { NotificationItem } from "../../notifications/schemas";

export const ACTIVITY_QUERY_KEY = ["overview", "activity"] as const;

const FEED_LIMIT = 10;
const SOURCE_LIMIT = 15;

interface FeedCard {
  key: string;
  title: string;
  detail: string;
  at: string;
  unread: boolean;
  target: string;
  kind: "notification" | "audit";
}

function notificationCard(n: NotificationItem): FeedCard {
  return {
    key: `notification-${n.id}`,
    title: n.title || n.eventType,
    detail: n.body || "",
    at: n.createdAt,
    unread: !n.readAt,
    target: "/notifications",
    kind: "notification",
  };
}

function auditCard(a: AuditLog): FeedCard {
  const target = a.targetType && a.targetId ? `${a.targetType} #${a.targetId}` : a.targetType;
  return {
    key: `audit-${a.id}`,
    title: a.action,
    detail: target,
    at: a.createdAt,
    unread: false,
    target: "/audit",
    kind: "audit",
  };
}

export async function fetchActivityFeed(): Promise<FeedCard[]> {
  const [notifications, audits] = await Promise.all([
    api.notifications({ limit: SOURCE_LIMIT }).catch((): NotificationItem[] => []),
    api.auditLogs({ limit: SOURCE_LIMIT }).catch((): AuditLog[] => []),
  ]);
  return [...notifications.map(notificationCard), ...audits.map(auditCard)]
    .sort((a, b) => +new Date(b.at || 0) - +new Date(a.at || 0))
    .slice(0, FEED_LIMIT);
}

export function ActivityFeed() {
  const { t } = useTranslation();
  const nav = useNavigate();
  const { data: cards, isLoading } = useQuery({
    queryKey: ACTIVITY_QUERY_KEY,
    queryFn: fetchActivityFeed,
    refetchInterval: 60000,
    refetchIntervalInBackground: false,
  });

  return (
    <GlassCard className="mb-6 p-6">
      <div className="mb-4 flex items-center justify-between">
        <div>
          <h2 className="text-lg font-bold text-foreground">{t("overview.activityFeedTitle")}</h2>
          <p className="text-xs text-foreground/50 mt-0.5">{t("overview.activityFeedDesc")}</p>
        </div>
        <button
          onClick={() => nav("/notifications")}
          className="flex items-center gap-1 text-xs font-medium text-accent-fg hover:underline"
        >
          {t("overview.activityFeedViewAll")}
          <ArrowRight size={14} />
        </button>
      </div>

      {isLoading ? (
        <div className="space-y-3" aria-busy="true" aria-label={t("overview.loading")}>
          {Array.from({ length: 4 }).map((_, i) => (
            <div key={i} className="flex items-center gap-3">
              <Skeleton className="h-9 w-9 rounded-xl" />
              <div className="flex-1 space-y-1.5">
                <Skeleton className="h-3.5 w-2/3" />
                <Skeleton className="h-3 w-1/3" />
              </div>
            </div>
          ))}
        </div>
      ) : !cards || cards.length === 0 ? (
        <div className="flex flex-col items-center py-8 text-center">
          <Inbox size={28} className="text-foreground/25" />
          <p className="mt-2 text-sm font-medium text-foreground/60">{t("overview.activityFeedEmpty")}</p>
          <p className="mt-0.5 text-xs text-foreground/40">{t("overview.activityFeedEmptyDesc")}</p>
        </div>
      ) : (
        <ul className="divide-y divide-foreground/5">
          {cards.map((card) => (
            <li key={card.key}>
              <button
                onClick={() => nav(card.target)}
                className="flex w-full items-center gap-3 py-2.5 text-left rounded-lg px-2 -mx-2 hover:bg-foreground/5 transition-colors"
              >
                <span className="flex h-9 w-9 shrink-0 items-center justify-center rounded-xl bg-foreground/5">
                  {card.kind === "notification" ? (
                    <Bell size={16} className="text-accent-fg" />
                  ) : (
                    <ScrollText size={16} className="text-foreground/50" />
                  )}
                </span>
                <span className="min-w-0 flex-1">
                  <span className="flex items-center gap-2">
                    <span className="truncate text-sm font-medium text-foreground">{card.title}</span>
                    {card.unread && <span className="h-1.5 w-1.5 shrink-0 rounded-full bg-accent-fg" />}
                  </span>
                  {card.detail && (
                    <span className="block truncate text-xs text-foreground/50">{card.detail}</span>
                  )}
                </span>
                <span className="shrink-0 text-xs text-foreground/40">{timeAgo(card.at)}</span>
              </button>
            </li>
          ))}
        </ul>
      )}
    </GlassCard>
  );
}
