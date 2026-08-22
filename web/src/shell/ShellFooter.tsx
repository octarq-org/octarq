import { useEffect, useState } from "react";
import { api } from "../api";
import { useTranslation } from "../i18n";

// Authenticated build stamp for the dashboard shell footer. The endpoint
// requires a session — a version string on an unauthenticated page would be a
// CVE checklist for scanners. dev/unknown are legit values for a non-git
// build and render as-is. All values are machine-produced → mono.
export function ShellFooter() {
  const { t } = useTranslation();
  const [build, setBuild] = useState<{ version: string; commit: string; builtAt: string } | null>(null);
  useEffect(() => {
    api.instanceBuild().then(setBuild).catch(() => {});
  }, []);
  if (!build) return null;
  return (
    <footer className="mt-10 border-t border-foreground/[0.06] pb-1 pt-3">
      <p className="font-mono tnum text-[11px] text-foreground/35">
        {t("app.shellBuildFooter", {
          version: build.version,
          commit: build.commit ? (build.commit.length > 8 ? build.commit.slice(0, 8) : build.commit) : "unknown",
          builtAt: build.builtAt,
        })}
      </p>
    </footer>
  );
}
