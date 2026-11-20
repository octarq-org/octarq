import { useEffect, useMemo, useState } from "react";
import { useNavigate } from "react-router-dom";
import { NsgoMount, NsgoLoadError, type NsgoHostHandlers } from "@octarq/plugin-sdk";
import { useTranslation } from "../i18n";
import { api } from "../api/client";
import { useTheme } from "../theme";

const MANIFEST = "/console/nsgo.json";

const LOCALES: Record<string, string> = { en: "en", zh: "zh-Hans", es: "es", pt: "pt", ja: "ja" };

export default function WorkbenchPage() {
  const { t, lang } = useTranslation();
  const theme = useTheme();
  const navigate = useNavigate();
  const [orgId, setOrgId] = useState<number | null>(null);
  const [orgError, setOrgError] = useState<string | null>(null);

  useEffect(() => {
    let alive = true;
    api
      .orgs()
      .then((orgs) => {
        if (alive) setOrgId(orgs[0]?.id ?? 0);
      })
      .catch((e: unknown) => {
        if (alive) setOrgError(e instanceof Error ? e.message : String(e));
      });
    return () => {
      alive = false;
    };
  }, []);

  const handlers = useMemo<NsgoHostHandlers>(
    () => ({
      call: (name: string) => {
        throw new Error(`The dashboard host does not answer ${name}.`);
      },
    }),
    [],
  );

  if (orgError) {
    return (
      <PageShell title={t("workbench.title", "Workbench")}>
        <p role="alert">
          {t("workbench.workspaceFailed", "The workspace did not load.")} {orgError}
        </p>
      </PageShell>
    );
  }

  if (orgId === null) {
    return (
      <PageShell title={t("workbench.title", "Workbench")}>
        <p>{t("workbench.loading", "Starting the workbench…")}</p>
      </PageShell>
    );
  }

  return (
    <PageShell title={t("workbench.title", "Workbench")} full>
      <NsgoMount
        className="h-full w-full"
        title={t("workbench.title", "Workbench")}
        src={MANIFEST}
        params={{ orgId, embedded: true }}
        appearance={theme}
        locale={LOCALES[lang] ?? lang}
        handlers={handlers}
        onOpenURL={(url) => {
          const target = new URL(url, window.location.origin);
          if (target.origin === window.location.origin) {
            navigate(`${target.pathname}${target.search}${target.hash}`);
            return;
          }
          window.open(target.href, "_blank", "noopener");
        }}
        fallback={(error) => (
          <div role="alert" className="space-y-1 p-4 text-sm">
            <p className="font-medium">
              {error instanceof NsgoLoadError && error.status === 404
                ? t("workbench.notInstalled", "This instance does not serve the workbench build.")
                : t("workbench.loadFailed", "The workbench did not load.")}
            </p>
            <p className="text-muted-foreground">{error.message}</p>
            <p className="text-muted-foreground">{t("workbench.retry", "Reload the page to try again.")}</p>
          </div>
        )}
      />
    </PageShell>
  );
}

function PageShell({
  title,
  full,
  children,
}: {
  title: string;
  full?: boolean;
  children: React.ReactNode;
}) {
  return (
    <div className={full ? "absolute inset-0 flex min-h-0 flex-col" : "p-6"}>
      <h1 className="sr-only">{title}</h1>
      <div className="min-h-0 flex-1">{children}</div>
    </div>
  );
}
