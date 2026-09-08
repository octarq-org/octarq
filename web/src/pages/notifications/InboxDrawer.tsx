import React from "react";
import { useNavigate } from "react-router-dom";
import { CheckCheck, ExternalLink, Settings2, Trash2, X } from "lucide-react";
import { useTranslation } from "../../i18n";
import { Button, cn } from "../../ui";
import { timeAgo } from "../../ui/time";
import { QueryClientContext } from "@tanstack/react-query";
import {
  useDeleteNotificationMutation,
  useMarkAllReadMutation,
  useMarkReadMutation,
  useNotificationsQuery,
} from "./api";
import { getNotificationIcon } from "./icons";
import { useNotificationUIStore } from "./store";

function InboxDrawerContent() {
  const { t } = useTranslation();
  const navigate = useNavigate();
  const { closeDrawer, filter, setFilter } = useNotificationUIStore();

  const { data: notifications = [], isLoading } = useNotificationsQuery({
    unreadOnly: filter === "unread",
  });

  const markRead = useMarkReadMutation();
  const markAllRead = useMarkAllReadMutation();
  const deleteNotif = useDeleteNotificationMutation();

  const unreadCount = notifications.filter((n) => !n.readAt).length;

  return (

    <div className="fixed inset-0 z-50 flex justify-end">
      {/* Backdrop */}
      <div
        className="fixed inset-0 bg-black/40 backdrop-blur-xs transition-opacity animate-in fade-in"
        onClick={closeDrawer}
        aria-hidden="true"
      />

      {/* Drawer Panel */}
      <div
        role="dialog"
        aria-modal="true"
        aria-label={t("notifications.inboxTitle", "站内信收件箱")}
        className="relative z-50 flex h-full w-full max-w-md flex-col border-l border-border bg-background shadow-2xl transition-all duration-200 animate-in slide-in-from-right"
      >
        {/* Header */}
        <div className="flex items-center justify-between border-b border-border px-4 py-3.5">
          <div className="flex items-center gap-2">
            <h2 className="text-base font-semibold text-foreground">
              {t("notifications.inboxTitle", "站内信收件箱")}
            </h2>
            {unreadCount > 0 && (
              <span className="rounded-full bg-primary/10 px-2 py-0.5 text-xs font-semibold text-primary">
                {unreadCount}
              </span>
            )}
          </div>

          <div className="flex items-center gap-1">
            <Button
              variant="subtle"
              size="sm"
              disabled={unreadCount === 0 || markAllRead.isPending}
              onClick={() => markAllRead.mutate()}
              className="flex items-center gap-1.5 text-xs"
              title={t("notifications.markAllRead", "全部已读")}
            >
              <CheckCheck className="h-3.5 w-3.5" />
              <span>{t("notifications.markAllRead", "全部已读")}</span>
            </Button>
            <button
              type="button"
              onClick={closeDrawer}
              aria-label={t("notifications.close", "关闭")}
              className="rounded-lg p-1.5 text-muted-foreground hover:bg-surface-hover hover:text-foreground"
            >
              <X className="h-4 w-4" />
            </button>
          </div>
        </div>

        {/* Filter Tabs */}
        <div className="flex border-b border-border bg-well/30 px-4 py-2">
          <div className="flex items-center gap-1 rounded-xl bg-surface-hover/50 p-0.5">
            <button
              type="button"
              onClick={() => setFilter("all")}
              className={cn(
                "rounded-lg px-3 py-1 text-xs font-medium transition-colors",
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
                "rounded-lg px-3 py-1 text-xs font-medium transition-colors",
                filter === "unread"
                  ? "bg-background text-foreground shadow-xs"
                  : "text-muted-foreground hover:text-foreground",
              )}
            >
              {t("notifications.filterUnread", "未读")}
            </button>
          </div>
        </div>

        {/* Notification List */}
        <div className="flex-1 overflow-y-auto divide-y divide-border/60">
          {isLoading ? (
            <div className="space-y-3 p-4">
              {[1, 2, 3].map((i) => (
                <div key={i} className="animate-pulse flex items-start gap-3 rounded-xl p-3 bg-well/40">
                  <div className="h-8 w-8 rounded-lg bg-foreground/10 shrink-0" />
                  <div className="flex-1 space-y-2">
                    <div className="h-3.5 w-3/4 rounded bg-foreground/10" />
                    <div className="h-3 w-full rounded bg-foreground/10" />
                  </div>
                </div>
              ))}
            </div>
          ) : notifications.length === 0 ? (
            <div className="flex flex-col items-center justify-center p-12 text-center">
              <div className="mb-3 flex h-12 w-12 items-center justify-center rounded-2xl border border-border bg-well/60 text-muted-foreground">
                <CheckCheck className="h-6 w-6 text-muted-foreground/60" />
              </div>
              <p className="text-sm font-medium text-foreground">
                {filter === "unread"
                  ? t("notifications.noUnread", "全部已读，暂无未读消息")
                  : t("notifications.empty", "暂无通知消息")}
              </p>
              <p className="mt-1 text-xs text-muted-foreground">
                {t("notifications.emptyDesc", "当有系统告警、安全提示或任务变更时将在此展示。")}
              </p>
            </div>
          ) : (
            notifications.map((notif) => {
              const isUnread = !notif.readAt;
              return (
                <div
                  key={notif.id}
                  className={cn(
                    "group relative flex items-start gap-3 p-4 transition-colors hover:bg-surface-hover/60",
                    isUnread && "bg-primary/[0.03]",
                  )}
                >
                  {/* Type Icon Container */}
                  <div className="relative mt-0.5 flex h-8 w-8 shrink-0 items-center justify-center rounded-lg border border-border bg-card">
                    {getNotificationIcon(notif.eventType, "h-4 w-4")}
                    {isUnread && (
                      <span
                        data-testid="unread-dot"
                        className="absolute -top-1 -right-1 h-2.5 w-2.5 rounded-full bg-primary ring-2 ring-background"
                      />
                    )}
                  </div>

                  {/* Content */}
                  <div className="min-w-0 flex-1">
                    <div className="flex items-center justify-between gap-2">
                      <span
                        className={cn(
                          "truncate text-sm text-foreground",
                          isUnread ? "font-semibold" : "font-medium text-foreground/80",
                        )}
                      >
                        {notif.title}
                      </span>
                      <span className="shrink-0 text-[11px] text-muted-foreground">
                        {timeAgo(notif.createdAt)}
                      </span>
                    </div>

                    <p className="mt-1 text-xs text-muted-foreground line-clamp-2">
                      {notif.body}
                    </p>

                    {/* Actions */}
                    <div className="mt-2.5 flex items-center justify-between gap-2">
                      <span className="rounded-md bg-muted px-1.5 py-0.5 text-[10px] font-mono text-muted-foreground">
                        {notif.eventType}
                      </span>
                      <div className="flex items-center gap-1.5 opacity-80 group-hover:opacity-100 transition-opacity">
                        {isUnread && (
                          <button
                            type="button"
                            onClick={() => markRead.mutate(notif.id)}
                            className="text-xs text-primary hover:underline"
                          >
                            {t("notifications.markRead", "标记已读")}
                          </button>
                        )}
                        <button
                          type="button"
                          onClick={() => deleteNotif.mutate(notif.id)}
                          aria-label={t("notifications.delete", "删除")}
                          className="p-1 text-muted-foreground hover:text-danger-fg transition-colors"
                        >
                          <Trash2 className="h-3 w-3" />
                        </button>
                      </div>
                    </div>
                  </div>
                </div>
              );
            })
          )}
        </div>

        {/* Drawer Footer Links */}
        <div className="flex items-center justify-between border-t border-border bg-well/40 px-4 py-3 text-xs">
          <button
            type="button"
            onClick={() => {
              closeDrawer();
              navigate("/notifications");
            }}
            className="flex items-center gap-1 font-medium text-foreground hover:text-primary transition-colors"
          >
            <span>{t("notifications.viewAllInbox", "查看全部站内信")}</span>
            <ExternalLink className="h-3 w-3" />
          </button>

          <button
            type="button"
            onClick={() => {
              closeDrawer();
              navigate("/notifications/preferences");
            }}
            className="flex items-center gap-1 text-muted-foreground hover:text-foreground transition-colors"
          >
            <Settings2 className="h-3.5 w-3.5" />
            <span>{t("notifications.preferencesTitle", "偏好设置")}</span>
          </button>
        </div>
      </div>
    </div>
  );
}

function InboxDrawerConnected() {
  const { isDrawerOpen } = useNotificationUIStore();
  if (!isDrawerOpen) return null;
  return <InboxDrawerContent />;
}

export function InboxDrawer() {
  const queryClient = React.useContext(QueryClientContext);
  if (!queryClient) {
    return null;
  }
  return <InboxDrawerConnected />;
}

