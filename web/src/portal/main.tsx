import React from "react";
import ReactDOM from "react-dom/client";
import { BrowserRouter } from "react-router-dom";
import PortalApp from "./PortalApp";
import "../styles.css";

ReactDOM.createRoot(document.getElementById("root")!).render(
  <React.StrictMode>
    <BrowserRouter basename="/portal">
      <PortalApp />
    </BrowserRouter>
  </React.StrictMode>,
);
