import React from "react";
import ReactDOM from "react-dom/client";
import { BrowserRouter } from "react-router-dom";
import { MotionConfig } from "framer-motion";
import PortalApp from "./PortalApp";
import { I18nProvider } from "../i18n";
import { BrandBridge } from "../brand";
import "../styles.css";

ReactDOM.createRoot(document.getElementById("root")!).render(
  <React.StrictMode>
    <MotionConfig reducedMotion="user">
      <I18nProvider>
        <BrandBridge>
          <BrowserRouter basename="/portal">
            <PortalApp />
          </BrowserRouter>
        </BrandBridge>
      </I18nProvider>
    </MotionConfig>
  </React.StrictMode>,
);
