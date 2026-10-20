import { lazy, Suspense } from "react";
import { Navigate, Route, Routes } from "react-router-dom";
import { RouteFallback } from "@octarq/plugin-sdk";
import { PluginGateContext, type PluginGateContextValue } from "../plugins/PluginGate";
import { pluginRouteElements, PluginUnavailable } from "../plugins/PluginRoutes";
import { InstanceExitRedirect } from "../pages/instance/redirect";

const OverviewPage = lazy(() => import("../pages/Overview"));
const SettingsPage = lazy(() => import("../pages/Settings"));
const NotificationsPage = lazy(() => import("../pages/notifications"));
const InviteAcceptPage = lazy(() => import("../pages/InviteAccept"));
const ResetPasswordPage = lazy(() => import("../pages/ResetPassword"));
const DevWorkbench = import.meta.env.DEV ? lazy(() => import("../dev/workbench")) : null;

export interface AppRoutesProps {
  pluginGateCtxValue: PluginGateContextValue;
}

export function AppRoutes({ pluginGateCtxValue }: AppRoutesProps) {
  return (
    <Suspense fallback={<RouteFallback />}>
      <PluginGateContext.Provider value={pluginGateCtxValue}>
        <Routes>
          <Route path="/" element={<Navigate to="/overview" replace />} />
          <Route path="/link-settings" element={<InstanceExitRedirect to="/instance/link-settings" />} />
          <Route path="/onboarding" element={<Navigate to="/overview" replace />} />
          <Route path="/overview" element={<OverviewPage />} />
          <Route path="/notifications/*" element={<NotificationsPage />} />
          <Route path="/settings/*" element={<SettingsPage />} />
          <Route path="/admin/invite/accept" element={<InviteAcceptPage />} />
          <Route path="/admin/reset" element={<ResetPasswordPage />} />
          {pluginRouteElements()}
          {DevWorkbench && <Route path="/_dev/workbench" element={<DevWorkbench />} />}
          <Route path="*" element={<PluginUnavailable />} />
        </Routes>
      </PluginGateContext.Provider>
    </Suspense>
  );
}
