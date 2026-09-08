import React, { useState } from "react";
import { useNavigate } from "react-router-dom";
import { CheckCheck, RefreshCw, Settings2, Trash2, MailOpen } from "lucide-react";
import { useTranslation } from "../../i18n";
import { Button, GlassCard, PageHeader, ScreenWrap, cn } from "../../ui";
import { timeAgo } from "../../ui/time";
import {
  useDeleteNotificationMutation,
  useMarkAllReadMutation,
  useMarkReadMutation,
  useNotificationsQuery,
} from "./api";
import { getNotificationIcon } from "./icons";
import { NotificationFilter } from "./types";

export function InboxPage() {
  const { t } = useTranslation();
  const navigate = useNavigate();
  const [filter, setFilter] = useState<NotificationFilter>("all");

  const {
    data: notifications = [],
    isLoading,
    refetch,
    isFetching,
  } = useNotificationsQuery({
    unreadOnly: filter === "unread",
  });

  const markRead = useMarkReadMutation();
  const markAllRead = useMarkAllReadMutation();
  const deleteNotif = useDeleteNotificationMutation();

  const unreadCount = notifications.filter((n) => !n.readAt).length;

  return (
    <ScreenWrap>
      <div className="space-y-6">
        <PageHeader
          title={t("notifications.inboxPageTitle", "站内信收件箱")}
          description={t(
            "notifications.inboxPageDescription",
            "查看与管理工作区内的系统事件、任务状态及安全警告。",
          )}
          action={
            <div className="flex flex-wrap items-center gap-2.5">
              <Button
                variant="outline"
                size="sm"
                onClick={() => refetch()}
                disabled={isFetching}
                className="flex items-center gap-1.5 text-xs"
              >
                <RefreshCw className={cn("h-3.5 w-3.5", isFetching && "animate-spin")} />
                <span>{t("notifications.refresh", "刷新")}</span>
              </Button>

              <Button
                variant="subtle"
                size="sm"
                disabled={unreadCount === 0 || markAllRead.isPending}
                onClick={() => markAllRead.mutate()}
                className="flex items-center gap-1.5 text-xs"
              >
                <CheckCheck className="h-3.5 w-3.5" />
                <span>{t("notifications.markAllRead", "全部已读")}</span>
              </Button>

              <Button
                variant="outline"
                size="sm"
                onClick={() => navigate("/notifications/preferences")}
                className="flex items-center gap-1.5 text-xs"
              >
                <Settings2 className="h-3.5 w-3.5" />
                <span>{t("notifications.preferencesTitle", "路由偏好设置")}</span>
              </Button>
            </div>
          }
        />

        {/* Filter Tabs */}
        <div className="flex items-center justify-between">
          <div className="flex items-center gap-1 rounded-xl border border-border bg-well/60 p-1">
            <button
              type="button"
              onClick={() => setFilter("all")}
              className={cn(
                "rounded-lg px-3.5 py-1.5 text-xs font-medium transition-colors",
                filter === "all"
                  ? "bg-background text-foreground shadow-xs"
                  : "text-muted-foreground hover:text-foreground",
              )}
            >
              {t("notifications.filterAll", "全部")}
            </button>
            <button
              type="button"
              onClick={() => setFilter("unread")}
              className={cn(
                "rounded-lg px-3.5 py-1.5 text-xs font-medium transition-colors",
                filter === "unread"
                  ? "bg-background text-foreground shadow-xs"
                  : "text-muted-foreground hover:text-foreground",
              )}
            >
              {t("notifications.filterUnread", "未读")}
              {unreadCount > 0 && (
                <span className="ml-1.5 rounded-full bg-primary/10 px-1.5 py-0.2 text-[10px] font-semibold text-primary">
                  {unreadCount}
                </span>
              )}
            </button>
          </div>
        </div>

        {/* Notification Cards */}
        <GlassCard className="overflow-hidden !p-0">
          {isLoading ? (
            <div className="divide-y divide-border p-6 space-y-4">
              {[1, 2, 3, 4].map((i) => (
                <div key={i} className="animate-pulse flex items-start gap-4 pt-4 first:pt-0">
                  <div className="h-10 w-10 rounded-xl bg-foreground/10 shrink-0" />
                  <div className="flex-1 space-y-2">
                    <div className="h-4 w-1/3 rounded bg-foreground/10" />
                    <div className="h-3.5 w-2/3 rounded bg-foreground/10" />
                  </div>
                </div>
              ))}
            </div>
          ) : notifications.length === 0 ? (
            <div className="flex flex-col items-center justify-center py-16 text-center">
              <div className="mb-3 flex h-14 w-14 items-center justify-center rounded-2xl border border-border bg-well text-muted-foreground">
                <MailOpen className="h-7 w-7 text-muted-foreground/60" />
              </div>
              <h4 className="text-sm font-semibold text-foreground">
                {filter === "unread"
                  ? t("notifications.noUnread", "全部已读，暂无未读消息")
                  : t("notifications.empty", "收件箱为空")}
              </h4>
              <p className="mt-1 max-w-sm text-xs text-muted-foreground">
                {t(
                  "notifications.emptyDesc",
                  "系统生成的事件通知、健康告警及定时任务日志将在此呈现。",
                )}
              </p>
            </div>
          ) : (
            <div className="divide-y divide-border">
              {notifications.map((notif) => {
                const isUnread = !notif.readAt;
                return (
                  <div
                    key={notif.id}
                    className={cn(
                      "flex flex-col sm:flex-row sm:items-start justify-between gap-4 p-4 sm:p-5 transition-colors hover:bg-surface-hover/50",
                      isUnread && "bg-primary/[0.02]",
                    )}
                  >
                    <div className="flex items-start gap-3.5 min-w-0 flex-1">
                      <div className="relative mt-0.5 flex h-9 w-9 shrink-0 items-center justify-center rounded-xl border border-border bg-card shadow-xs">
                        {getNotificationIcon(notif.eventType, "h-5 w-5")}
                        {isUnread && (
                          <span className="absolute -top-1 -right-1 h-2.5 w-2.5 rounded-full bg-primary ring-2 ring-background" />
                        )}
                      </div>

                      <div className="min-w-0 flex-1">
                        <div className="flex flex-wrap items-center gap-2">
                          <h4
                            className={cn(
                              "text-sm",
                              isUnread
                                ? "font-semibold text-foreground"
                                : "font-medium text-foreground/85",
                            )}
                          >
                            {notif.title}
                          </h4>
                          <span className="rounded-md bg-muted px-2 py-0.5 text-[11px] font-mono text-muted-foreground">
                            {notif.eventType}
                          </span>
                          {notif.priority === "high" && (
                            <span className="rounded-md bg-danger-fg/10 px-2 py-0.5 text-[11px] font-semibold text-danger-fg">
                              {t("notifications.priorityHigh", "高优先级")}
                            </span>
                          )}
                        </div>

                        <p className="mt-1 text-xs text-muted-foreground sm:text-sm leading-relaxed whitespace-pre-wrap">
                          {notif.body}
                        </p>

                        <div className="mt-2 text-xs text-muted-foreground/75">
                          {timeAgo(notif.createdAt)}
                        </div>
                      </div>
                    </div>

                    {/* Action buttons */}
                    <div className="flex items-center gap-2 self-end sm:self-start shrink-0">
                      {isUnread && (
                        <Button
                          variant="subtle"
                          size="sm"
                          onClick={() => markRead.mutate(notif.id)}
                          className="text-xs"
                        >
                          {t("notifications.markRead", "标记已读")}
                        </Button>
                      )}
                      <Button
                        variant="ghost"
                        size="sm"
                        onClick={() => deleteNotif.mutate(notif.id)}
                        className="text-muted-foreground hover:text-danger-fg p-2"
                        title={t("notifications.delete", "删除")}
                      >
                        <Trash2 className="h-4 w-4" />
                      </Button>
                    </div>
                  </div>
                );
              })}
            </div>
          )}
        </GlassCard>
      </div>
    </ScreenWrap>
  );
}
