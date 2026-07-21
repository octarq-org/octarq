// Infrastructure asset placeholders (Certificates / Databases / Object Storage)
// as one small core UIPlugin. They're business surface — roadmap pages for the
// Infrastructure area — not shell, so they live behind the same plugin pipeline
// as every other feature; when a real implementation lands it replaces the
// ComingSoon route here without touching App.tsx.
import { lazy } from "react";
import type { UIPlugin } from "@octarq/plugin-sdk";

const page = (name: "CertificatesComingSoon" | "DatabasesComingSoon" | "StorageComingSoon") =>
  lazy(() => import("../../pages/ComingSoon").then((m) => ({ default: m[name] })));

const assets: UIPlugin = {
  name: "assets",
  routes: [
    { path: "/assets/certificates", Component: page("CertificatesComingSoon") },
    { path: "/assets/databases", Component: page("DatabasesComingSoon") },
    { path: "/assets/storage", Component: page("StorageComingSoon") },
  ],
  menu: [
    { id: "certs", label: "Certificates", path: "/assets/certificates", icon: "shield", category: "Network", order: 20 },
    { id: "databases", label: "Databases", path: "/assets/databases", icon: "database", category: "Storage & Databases" },
    { id: "storage", label: "Object Storage", path: "/assets/storage", icon: "hard-drive", category: "Storage & Databases" },
  ],
};

export default assets;
