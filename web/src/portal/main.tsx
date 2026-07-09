import React from "react";
import ReactDOM from "react-dom/client";
import { BrowserRouter } from "react-router-dom";
import PortalApp from "./PortalApp";
import { I18nProvider } from "../i18n";
import "../styles.css";

ReactDOM.createRoot(document.getElementById("root")!).render(
  <React.StrictMode>
    <I18nProvider>
      <BrowserRouter basename="/portal">
        <PortalApp />
      </BrowserRouter>
    </I18nProvider>
  </React.StrictMode>,
);
