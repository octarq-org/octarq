import React from "react";
import ReactDOM from "react-dom/client";
import { BrowserRouter } from "react-router-dom";
import { MotionConfig } from "framer-motion";
import "./csrfFetch";
import "./plugins/core";
import "#octarq-plugins";
import App from "./App";
import { I18nProvider } from "./i18n";
import { BrandBridge } from "./brand";
import { ToastProvider } from "@octarq/plugin-sdk";
import { ConfirmBridge } from "./ConfirmBridge";
import { QueryClientProvider } from "@tanstack/react-query";
import { queryClient } from "./lib/queryClient";
import "./styles.css";

const routerBasename =
  window.location.pathname === "/status" || window.location.pathname === "/status/"
    ? "/"
    : window.location.pathname.startsWith("/instance")
      ? "/instance"
      : "/admin";

ReactDOM.createRoot(document.getElementById("root")!).render(
  <React.StrictMode>
    <MotionConfig reducedMotion="user">
      <QueryClientProvider client={queryClient}>
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
      </QueryClientProvider>
    </MotionConfig>
  </React.StrictMode>,
);
