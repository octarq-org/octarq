// Licenses — the issuer plugin's read-only record of every license minted
// (GET /api/issued). License-gated: 402 → upsell; OSS build → 404 note.
import { useEffect, useState } from "react";
import { api, ApiError, IssuedLicense } from "../api";
import { ScreenWrap, PageHeader, GlassCard, Badge, Empty, LockedFeature } from "../ui";
import { KeyRound } from "lucide-react";

export default function LicensesPage() {
  const [rows, setRows] = useState<IssuedLicense[]>([]);
  const [error, setError] = useState<{ status: number } | null>(null);
  const [unavailable, setUnavailable] = useState(false);

  useEffect(() => {
    api.issued()
      .then(setRows)
      .catch((e: ApiError) => {
        if (e.status === 404) setUnavailable(true);
        else setError({ status: e.status });
      });
  }, []);

  if (unavailable) {
    return (
      <ScreenWrap>
        <GlassCard className="mx-auto mt-12 max-w-md p-6 text-center text-sm text-white/55">
          License issuance is a <span className="text-white/80">Octarq Pro</span> feature and isn't part
          of the open-source build.
        </GlassCard>
      </ScreenWrap>
    );
  }
  if (error) {
    return (
      <ScreenWrap>
        <LockedFeature
          status={error.status}
          tier="pro"
          feature="License Issuance Control"
          description="Manage, verify, and audit cryptographic software licenses issued through your product storefronts."
          perks={[
            "Ed25519 cryptographic per-product signing keys",
            "Comprehensive registry tracking buyer, entitlement tier, and lifecycle state",
            "Automated access revocation synced with checkout billing events",
          ]}
          icon={<KeyRound className="h-7 w-7" />}
          pricingHref="https://octarq.com/pricing/"
        />
      </ScreenWrap>
    );
  }

  return (
    <ScreenWrap>
      <PageHeader title="License Registry" description="Unified registry tracking all active, expired, and revoked cryptographic client licenses" />
      {rows.length === 0 ? (
        <Empty>
          <KeyRound className="mb-2 h-10 w-10 text-white/30" />
          <p className="text-sm text-white/50">No licenses issued yet.</p>
        </Empty>
      ) : (
        <GlassCard className="overflow-hidden">
          <table className="w-full text-sm">
            <thead className="text-left text-white/45">
              <tr className="border-b border-white/8">
                <th className="px-4 py-3 font-medium">Email</th>
                <th className="px-4 py-3 font-medium">Tier</th>
                <th className="px-4 py-3 font-medium">Product</th>
                <th className="px-4 py-3 font-medium">Via</th>
                <th className="px-4 py-3 font-medium">Status</th>
                <th className="px-4 py-3 font-medium">Expires</th>
                <th className="px-4 py-3 font-medium">Issued</th>
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
                  <td className="px-4 py-3 text-white/45">{l.expiresAt ? l.expiresAt.slice(0, 10) : "never"}</td>
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
