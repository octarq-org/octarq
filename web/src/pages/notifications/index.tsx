import React from "react";
import { Navigate, Route, Routes } from "react-router-dom";
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
  return (
    <Routes>
      <Route path="/" element={<InboxPage />} />
      <Route path="/preferences" element={<Navigate to="/settings/personal/notifications" replace />} />
      <Route path="*" element={<Navigate to="/notifications" replace />} />
    </Routes>
  );
}
