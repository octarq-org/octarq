// Licenses page — the issuer plugin's read-only record of every license minted
// (GET /api/issued). License-gated: 402 → upsell; plugin absent → 404 note.
//
// This is the same page that used to live at web/src/pages/Licenses.tsx, now
// composed through the frontend plugin SDK. Behavior is unchanged: it imports
// its UI from `@octarq-org/plugin-sdk` (the stable plugin surface) instead of reaching
// into `../ui` directly, and its translation namespace is injected by the
// plugin (see ./index.ts) rather than baked into the central i18n bundle.
import { useEffect, useState } from "react";
import { api, ApiError, IssuedLicense } from "../../api";
import {
  ScreenWrap,
  PageHeader,
  GlassCard,
  Badge,
  Empty,
  LockedFeature,
  useTranslation,
} from "@octarq-org/plugin-sdk";
import { KeyRound } from "lucide-react";

export default function LicensesPage() {
  const { t } = useTranslation();
  const [rows, setRows] = useState<IssuedLicense[]>([]);
  const [error, setError] = useState<{ status: number } | null>(null);

  useEffect(() => {
    api.issued()
      .then(setRows)
      .catch((e: ApiError) => setError({ status: e.status }));
  }, []);

  if (error) {
    return (
      <ScreenWrap>
        <LockedFeature
          status={error.status}
          tier="pro"
          feature={t("licenses.lockedFeature")}
          description={t("licenses.lockedDescription")}
          perks={[
            t("licenses.lockedPerk1"),
            t("licenses.lockedPerk2"),
            t("licenses.lockedPerk3"),
          ]}
          icon={<KeyRound className="h-7 w-7" />}
          pricingHref="https://octarq.com/pricing/"
        />
      </ScreenWrap>
    );
  }

  return (
    <ScreenWrap>
      <PageHeader title={t("licenses.pageTitle")} description={t("licenses.pageDesc")} />
      {rows.length === 0 ? (
        <Empty>
          <KeyRound className="mb-2 h-10 w-10 text-white/50" />
          <p className="text-sm text-white/50">{t("licenses.emptyState")}</p>
        </Empty>
      ) : (
        <GlassCard className="overflow-hidden">
          <table className="w-full text-sm">
            <thead className="text-left text-white/45">
              <tr className="border-b border-white/8">
                <th className="px-4 py-3 font-medium">{t("licenses.colEmail")}</th>
                <th className="px-4 py-3 font-medium">{t("licenses.colTier")}</th>
                <th className="px-4 py-3 font-medium">{t("licenses.colProduct")}</th>
                <th className="px-4 py-3 font-medium">{t("licenses.colVia")}</th>
                <th className="px-4 py-3 font-medium">{t("licenses.colStatus")}</th>
                <th className="px-4 py-3 font-medium">{t("licenses.colExpires")}</th>
                <th className="px-4 py-3 font-medium">{t("licenses.colIssued")}</th>
              </tr>
            </thead>
            <tbody className="divide-y divide-white/5">
              {rows.map((l) => (
                <tr key={l.id} className="text-white/75">
                  <td className="px-4 py-3">{l.email}</td>
                  <td className="px-4 py-3"><Badge tone="violet">{(l.tier || "").toUpperCase()}</Badge></td>
                  <td className="px-4 py-3 text-white/45">#{l.productId}</td>
                  <td className="px-4 py-3 text-white/45">{l.provider}</td>
                  <td className="px-4 py-3">
                    <Badge tone={l.status === "active" ? "green" : "red"}>{l.status}</Badge>
                  </td>
                  <td className="px-4 py-3 text-white/45">{l.expiresAt ? l.expiresAt.slice(0, 10) : t("licenses.never")}</td>
                  <td className="px-4 py-3 text-white/45">{l.createdAt.slice(0, 10)}</td>
                </tr>
              ))}
            </tbody>
          </table>
        </GlassCard>
      )}
    </ScreenWrap>
  );
}
