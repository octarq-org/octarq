import React from "react";
import { Navigate, Route, Routes, useLocation, useNavigate } from "react-router-dom";
import { Inbox, Sliders } from "lucide-react";
import { useTranslation } from "../../i18n";
import { cn } from "../../ui";
import { InboxPage } from "./InboxPage";
import { PreferencesPage } from "./PreferencesPage";

export { InboxPage } from "./InboxPage";
export { PreferencesPage } from "./PreferencesPage";
export { InboxDrawer } from "./InboxDrawer";
export { NotificationBell } from "./NotificationBell";
export { useNotificationUIStore } from "./store";
export * from "./types";
export * from "./schemas";
export * from "./api";

export default function NotificationsPage() {
  const { t } = useTranslation();
  const location = useLocation();
  const navigate = useNavigate();

  const isPreferences = location.pathname.includes("/preferences");

  return (
    <div className="flex flex-col h-full">
      {/* Subnav header */}
      <div className="border-b border-border bg-background px-4 sm:px-6 py-2.5">
        <div className="flex items-center gap-2">
          <button
            type="button"
            onClick={() => navigate("/notifications")}
            className={cn(
              "flex items-center gap-2 rounded-xl px-3 py-1.5 text-xs font-medium transition-colors",
              !isPreferences
                ? "bg-foreground/[0.08] text-foreground font-semibold"
                : "text-muted-foreground hover:bg-surface-hover hover:text-foreground",
            )}
          >
            <Inbox className="h-4 w-4" />
            <span>{t("notifications.tabInbox", "站内信收件箱")}</span>
          </button>

          <button
            type="button"
            onClick={() => navigate("/notifications/preferences")}
            className={cn(
              "flex items-center gap-2 rounded-xl px-3 py-1.5 text-xs font-medium transition-colors",
              isPreferences
                ? "bg-foreground/[0.08] text-foreground font-semibold"
                : "text-muted-foreground hover:bg-surface-hover hover:text-foreground",
            )}
          >
            <Sliders className="h-4 w-4" />
            <span>{t("notifications.tabPreferences", "通知路由设置")}</span>
          </button>
        </div>
      </div>

      <div className="flex-1 overflow-auto">
        <Routes>
          <Route path="/" element={<InboxPage />} />
          <Route path="/preferences" element={<PreferencesPage />} />
          <Route path="*" element={<Navigate to="/notifications" replace />} />
        </Routes>
      </div>
    </div>
  );
}
