// Cross-basename redirect from the tenant shell (/settings/instance*) to the
// instance console (/instance…). The console lives under its own basename, so
// leaving the tenant shell for it MUST be a full page navigation — a router
// <Navigate> would produce /admin/instance and render the tenant 404.
import { useEffect } from "react";
import { useLocation } from "react-router-dom";

export function InstanceExitRedirect({ base = "/settings/instance", to }: { base?: string; to?: string }) {
  const location = useLocation();
  const target = to ?? `/instance${location.pathname.slice(base.length)}`;
  useEffect(() => {
    window.location.replace(target);
  }, [target]);
  return null;
}
