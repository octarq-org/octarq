import React from "react";
import ReactDOM from "react-dom/client";
import { BrowserRouter } from "react-router-dom";
import { MotionConfig } from "framer-motion";
// Compose build-time frontend plugins into the registry before anything reads
// it. `#octarq-plugins` resolves to the empty OSS injection module by default, and
// to ./plugins/index.pro.ts in a commercial build (VITE_OCTARQ_PLUGINS=pro) — so
// the OSS bundle never imports, and never ships, any Pro page (see vite.config).
import "#octarq-plugins";
import App from "./App";
import { I18nProvider } from "./i18n";
import "./styles.css";

ReactDOM.createRoot(document.getElementById("root")!).render(
  <React.StrictMode>
    {/* Honor the OS "reduce motion" setting for every framer-motion animation
        (StatCard/ScreenWrap enter, dropdowns, …) — a11y baseline. */}
    <MotionConfig reducedMotion="user">
      <I18nProvider>
        <BrowserRouter basename="/admin">
          <App />
        </BrowserRouter>
      </I18nProvider>
    </MotionConfig>
  </React.StrictMode>,
);
