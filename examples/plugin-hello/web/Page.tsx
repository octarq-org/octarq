// The example plugin's page — the JS half of the feature. It calls the Go half
// (GET /api/hello/ping) and renders with the shared UI from @led/plugin-sdk.
//
// A third-party plugin can't import led's internal `api` client, so it uses a
// plain fetch and handles the two gated states led standardises on:
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
} from "@led/plugin-sdk";

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
      <GlassCard className="p-6">
        {ping ? (
          <div className="space-y-1 text-sm">
            <p className="text-white/80">{ping.message}</p>
            <p className="text-white/40">{ping.time}</p>
          </div>
        ) : (
          <p className="text-sm text-white/40">{t("hello.loading", "Loading…")}</p>
        )}
      </GlassCard>
    </ScreenWrap>
  );
}
