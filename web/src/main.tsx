import React from "react";
import ReactDOM from "react-dom/client";
import { BrowserRouter } from "react-router-dom";
import { MotionConfig } from "framer-motion";
// Compose build-time frontend plugins into the registry before anything reads
// it. Core-feature plugins first (always composed, in every edition — see
// plugins/core/index.ts), then `#octarq-plugins`, a virtual module generated
// from the active plugin manifest (see plugins-manifest.ts): it imports and
// registers exactly the plugins that edition ships. A build never imports —
// and never bundles — a plugin its manifest doesn't name.
import "./csrfFetch";
import "./plugins/core";
import "#octarq-plugins";
import App from "./App";
import { I18nProvider } from "./i18n";
import { BrandBridge } from "./brand";
import { ToastProvider } from "./ui";
import { ConfirmBridge } from "./ConfirmBridge";
import "./styles.css";

// The dashboard normally lives under /admin (Vite base + BrowserRouter
// basename). The public status page is served at the bare /status path (the
// backend returns the same index.html there), so the router basename must be
// "/" for that route — React Router renders nothing when the URL does not
// start with the basename. The instance console gets the same treatment under
// /instance. Everything else keeps /admin.
const routerBasename =
  window.location.pathname === "/status" || window.location.pathname === "/status/"
    ? "/"
    : window.location.pathname.startsWith("/instance")
      ? "/instance"
      : "/admin";

ReactDOM.createRoot(document.getElementById("root")!).render(
  <React.StrictMode>
    {/* Honor the OS "reduce motion" setting for every framer-motion animation
        (StatCard/ScreenWrap enter, dropdowns, …) — a11y baseline. */}
    <MotionConfig reducedMotion="user">
      {/* I18nProvider + BrandBridge feed the SDK's i18n/brand context, which the
          shared UI and plugin packages read. */}
      <I18nProvider>
        <BrandBridge>
          <ToastProvider>
            <ConfirmBridge>
              <BrowserRouter basename={routerBasename}>
                <App />
              </BrowserRouter>
            </ConfirmBridge>
          </ToastProvider>
        </BrandBridge>
      </I18nProvider>
    </MotionConfig>
  </React.StrictMode>,
);
