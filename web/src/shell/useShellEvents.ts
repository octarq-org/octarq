import { useEffect } from "react";
import { api, type Org } from "../api";

export interface ShellEventsOptions {
  setOrgs: (fn: (prev: Org[]) => Org[]) => void;
  refreshNav: () => void;
  setRole: (role: string | undefined) => void;
  setEmailVerified: (verified: boolean | undefined) => void;
}

export function useShellEvents({
  setOrgs,
  refreshNav,
  setRole,
  setEmailVerified,
}: ShellEventsOptions) {
  useEffect(() => {
    const refreshOrgs = () => {
      api
        .orgs()
        .then((os) => setOrgs(() => os))
        .catch(() => setOrgs(() => []));
    };
    const refreshPlugins = () => refreshNav();
    const refreshAuth = () => {
      api
        .me()
        .then((m) => {
          setRole(m.role);
          setEmailVerified(m.emailVerified);
        })
        .catch(() => {});
    };
    window.addEventListener("octarq:orgs-changed", refreshOrgs);
    window.addEventListener("octarq:plugins-changed", refreshPlugins);
    window.addEventListener("octarq:auth-changed", refreshAuth);
    return () => {
      window.removeEventListener("octarq:orgs-changed", refreshOrgs);
      window.removeEventListener("octarq:plugins-changed", refreshPlugins);
      window.removeEventListener("octarq:auth-changed", refreshAuth);
    };
  }, [refreshNav, setRole, setEmailVerified, setOrgs]);
}
