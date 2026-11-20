import { PageEmpty, PageRows, PageSection, Skeleton, timeAgo } from "@octarq/plugin-sdk";
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
    target: "/governance/audit",
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
    <PageSection
      className="mb-8"
      title={t("overview.activityFeedTitle")}
      description={t("overview.activityFeedDesc")}
      action={
        <button
          onClick={() => nav("/notifications")}
          className="flex items-center gap-1 rounded-md px-1.5 py-1 text-[12px] font-medium text-accent-fg transition-colors hover:bg-accent-soft"
        >
          {t("overview.activityFeedViewAll")}
          <ArrowRight size={14} />
        </button>
      }
    >
      {isLoading ? (
        <PageRows>
          <div className="space-y-3 p-4" aria-busy="true" aria-label={t("overview.loading")}>
            {Array.from({ length: 4 }).map((_, i) => (
              <div key={i} className="flex items-center gap-3">
                <Skeleton className="h-9 w-9 rounded-lg" />
                <div className="flex-1 space-y-1.5">
                  <Skeleton className="h-3.5 w-2/3" />
                  <Skeleton className="h-3 w-1/3" />
                </div>
              </div>
            ))}
          </div>
        </PageRows>
      ) : !cards || cards.length === 0 ? (
        <PageEmpty
          icon={<Inbox size={28} />}
          title={t("overview.activityFeedEmpty")}
          detail={t("overview.activityFeedEmptyDesc")}
        />
      ) : (
        <PageRows>
          <ul className="divide-y divide-border">
            {cards.map((card) => (
              <li key={card.key}>
                <button
                  onClick={() => nav(card.target)}
                  className="flex min-h-[var(--row-height,3.5rem)] w-full items-center gap-3 px-4 py-2.5 text-left transition-colors duration-100 hover:bg-surface-hover"
                >
                  <span className="flex h-8 w-8 shrink-0 items-center justify-center rounded-md border border-border bg-well">
                    {card.kind === "notification" ? (
                      <Bell size={15} className="text-accent-fg" />
                    ) : (
                      <ScrollText size={15} className="text-muted-foreground" />
                    )}
                  </span>
                  <span className="min-w-0 flex-1">
                    <span className="flex items-center gap-2">
                      <span className="truncate text-sm font-medium text-foreground">{card.title}</span>
                      {card.unread && <span className="h-1.5 w-1.5 shrink-0 rounded-full bg-accent-fg" />}
                    </span>
                    {card.detail && (
                      <span className="block truncate text-xs text-muted-foreground">{card.detail}</span>
                    )}
                  </span>
                  <span className="shrink-0 text-xs text-muted-foreground">{timeAgo(card.at)}</span>
                </button>
              </li>
            ))}
          </ul>
        </PageRows>
      )}
    </PageSection>
  );
}
