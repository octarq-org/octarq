import React from "react";
import { Bell } from "lucide-react";
import { QueryClientContext } from "@tanstack/react-query";
import { useTranslation } from "../../i18n";
import { cn } from "../../ui";
import { useUnreadNotificationsCountQuery } from "./api";
import { useNotificationUIStore } from "./store";

const ACTION_BUTTON =
  "rounded-xl border border-foreground/10 dark:border-white/10 bg-surface-hover/50 " +
  "hover:bg-surface-hover hover:border-foreground/20 text-muted-foreground transition-all";

function NotificationBellConnected() {
  const { t } = useTranslation();
  const { toggleDrawer, isDrawerOpen } = useNotificationUIStore();
  const { data: unreadCount = 0 } = useUnreadNotificationsCountQuery();

  const badgeText = unreadCount > 99 ? "99+" : unreadCount > 0 ? String(unreadCount) : null;

  return (
    <button
      type="button"
      onClick={toggleDrawer}
      aria-label={t("notifications.bellTitle", "站内通知")}
      title={t("notifications.bellTitle", "站内通知")}
      aria-expanded={isDrawerOpen}
      className={cn(
        ACTION_BUTTON,
        "relative flex h-9 w-9 items-center justify-center transition-colors hover:text-foreground",
        isDrawerOpen && "bg-foreground/[0.06] text-foreground ring-1 ring-inset ring-border",
      )}
    >
      <Bell className="h-5 w-5" strokeWidth={1.75} />
      {unreadCount > 0 && (
        <span
          data-testid="notification-unread-badge"
          className="absolute -top-1 -right-1 flex h-4 min-w-[16px] items-center justify-center rounded-full bg-danger-fg px-1 text-[10px] font-bold text-white shadow-xs ring-2 ring-background"
        >
          {badgeText}
        </span>
      )}
    </button>
  );
}

export function NotificationBell() {
  const queryClient = React.useContext(QueryClientContext);
  if (!queryClient) {
    return null;
  }
  return <NotificationBellConnected />;
}

