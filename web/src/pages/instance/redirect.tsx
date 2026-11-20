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
