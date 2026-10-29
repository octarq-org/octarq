// The example plugin's page — the JS half of the feature. It calls the Go half
// (GET /api/hello/ping) and renders with the shared UI from @octarq/plugin-sdk.
//
// A third-party plugin can't import octarq's internal `api` client, so it uses a
// plain fetch and handles the two gated states octarq standardises on:
//   402 → the feature is unlicensed (show an upsell),
//   404 → the plugin isn't built into this installation (neutral note).
// Both are covered by the SDK's <LockedFallback status={…} />.
import { useEffect, useState } from "react";
import {
  ScreenWrap,
  PageHeader,
  GlassCard,
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

  // 402/404 → the standard gated fallback. Anything else falls through to a
  // neutral loading/empty state below (never a raw error).
  if (status === 402 || status === 404) {
    return (
      <ScreenWrap>
        <LockedFallback
          status={status}
          feature={t("hello.feature", "Hello Plugin")}
          description={t("hello.description", "A minimal example plugin.")}
        />
      </ScreenWrap>
    );
  }

  return (
    <ScreenWrap>
      <PageHeader
        title={t("hello.pageTitle", "Hello Plugin")}
        description={t("hello.pageDesc", "A minimal full-stack example plugin.")}
      />
      <GlassCard className="p-5">
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
      </GlassCard>
    </ScreenWrap>
  );
}
