// The SSH Vault UIPlugin — the `infra` backend plugin's SSH key page, composed
// through the frontend SDK. Absent from the OSS build (no route, no sidebar
// entry) since ../index.pro.ts is the only module that registers it.
import { lazy } from "react";
import type { UIPlugin } from "@octarq-org/plugin-sdk";
import { sshKeys } from "./i18n";

export const sshKeysPlugin: UIPlugin = {
  name: "sshKeys",
  routes: [{ path: "/sshkeys", Component: lazy(() => import("./page")) }],
  // Assets area, matching where the static sidebar used to place SSH Vault.
  menu: [{ id: "sshkeys", label: "SSH Vault", path: "/sshkeys", icon: "🔐", category: "Hosting" }],
  i18n: sshKeys,
};
