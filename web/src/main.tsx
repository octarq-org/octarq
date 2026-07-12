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
import "./plugins/core";
import "#octarq-plugins";
import App from "./App";
import { I18nProvider } from "./i18n";
import { BrandBridge } from "./brand";
import "./styles.css";

ReactDOM.createRoot(document.getElementById("root")!).render(
  <React.StrictMode>
    {/* Honor the OS "reduce motion" setting for every framer-motion animation
        (StatCard/ScreenWrap enter, dropdowns, …) — a11y baseline. */}
    <MotionConfig reducedMotion="user">
      {/* I18nProvider + BrandBridge feed the SDK's i18n/brand context, which the
          shared UI and plugin packages read. */}
      <I18nProvider>
        <BrandBridge>
          <BrowserRouter basename="/admin">
            <App />
          </BrowserRouter>
        </BrandBridge>
      </I18nProvider>
    </MotionConfig>
  </React.StrictMode>,
);
