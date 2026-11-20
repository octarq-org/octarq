import { useEffect, useState } from "react";
import {
  PageShell,
  PageCard,
  LockedFallback,
  useTranslation,
} from "@octarq/plugin-sdk";

interface Ping {
  message: string;
  time: string;
}

export default function HelloPage() {
  const { t } = useTranslation();
  const [ping, setPing] = useState<Ping | null>(null);
  const [status, setStatus] = useState<number | null>(null);

  useEffect(() => {
    fetch("/api/hello/ping", { credentials: "same-origin" })
      .then((res) => {
        if (!res.ok) {
          setStatus(res.status);
          return null;
        }
        return res.json() as Promise<Ping>;
      })
      .then((data) => data && setPing(data))
      .catch(() => setStatus(0));
  }, []);

  if (status === 402 || status === 404) {
    return (
      <PageShell title={t("hello.pageTitle", "Hello Plugin")}>
        <LockedFallback
          status={status}
          feature={t("hello.feature", "Hello Plugin")}
          description={t("hello.description", "A minimal example plugin.")}
        />
      </PageShell>
    );
  }

  return (
    <PageShell
      title={t("hello.pageTitle", "Hello Plugin")}
      description={t("hello.pageDesc", "A minimal full-stack example plugin.")}
    >
      <PageCard className="mt-6 p-5">
        {ping ? (
          <div className="space-y-3">
            <div className="flex items-center gap-2">
              <span className="h-1.5 w-1.5 shrink-0 rounded-full bg-success-fg" aria-hidden="true" />
              <span className="text-[13px] font-medium text-foreground">{ping.message}</span>
            </div>
            <div className="rounded-lg border border-border bg-well px-3 py-2 font-mono text-xs text-muted-foreground">
              {ping.time}
            </div>
          </div>
        ) : (
          <p className="text-[13px] text-muted-foreground">{t("hello.loading", "Loading…")}</p>
        )}
      </PageCard>
    </PageShell>
  );
}
