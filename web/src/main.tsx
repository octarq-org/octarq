import React from "react";
import ReactDOM from "react-dom/client";
import { BrowserRouter } from "react-router-dom";
// Compose build-time frontend plugins into the registry before anything reads
// it. `#led-plugins` resolves to the empty OSS injection module by default, and
// to ./plugins/index.pro.ts in a commercial build (VITE_LED_PLUGINS=pro) — so
// the OSS bundle never imports, and never ships, any Pro page (see vite.config).
import "#led-plugins";
import App from "./App";
import { I18nProvider } from "./i18n";
import "./styles.css";

ReactDOM.createRoot(document.getElementById("root")!).render(
  <React.StrictMode>
    <I18nProvider>
      <BrowserRouter basename="/admin">
        <App />
      </BrowserRouter>
    </I18nProvider>
  </React.StrictMode>,
);
