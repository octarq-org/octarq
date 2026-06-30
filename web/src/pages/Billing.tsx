// Billing — the billing plugin's config. The webhook secret (encrypted) plus the
// price map: Stripe payment-link/price id → your product + tier. Resolving sales
// through the map means operators never set checkout metadata. License-gated:
// 402 → upsell; OSS build → 404 note. Issuance itself lives under Licenses.
import { useEffect, useState } from "react";
import { api, ApiError, BillingConfig, PriceMap, PriceMapInput } from "../api";
import { ScreenWrap, PageHeader, GlassCard, Button, Badge, Field, Modal, Empty, LockedFeature } from "../ui";
import { Receipt, Plus } from "lucide-react";

export default function BillingPage() {
  const [cfg, setCfg] = useState<BillingConfig | null>(null);
  const [prices, setPrices] = useState<PriceMap[]>([]);
  const [error, setError] = useState<{ status: number } | null>(null);
  const [unavailable, setUnavailable] = useState(false);
  const [secret, setSecret] = useState("");
  const [msg, setMsg] = useState<string | null>(null);
  const [busy, setBusy] = useState(false);
  const [editing, setEditing] = useState<PriceMap | "new" | null>(null);

  function load() {
    api.billingConfig()
      .then((c) => { setCfg(c); setError(null); api.billingPrices().then(setPrices).catch(() => {}); })
      .catch((e: ApiError) => {
        if (e.status === 404) setUnavailable(true);
        else setError({ status: e.status });
      });
  }
  useEffect(load, []);

  async function saveSecret(webhookSecret: string) {
    setBusy(true);
    setMsg(null);
    try {
      await api.updateBillingConfig({ webhookSecret });
      setSecret("");
      setMsg("Saved.");
      load();
    } catch (e) { setMsg((e as ApiError).message); } finally { setBusy(false); }
  }

  async function delPrice(id: number) {
    if (!confirm("Delete this price mapping? Checkouts using it will be rejected until re-mapped.")) return;
    await api.deleteBillingPrice(id);
    load();
  }

  if (unavailable) {
    return (
      <ScreenWrap>
        <GlassCard className="mx-auto mt-12 max-w-md p-6 text-center text-sm text-white/55">
          Billing is a <span className="text-white/80">Octarq Pro</span> feature and isn't part of the
          open-source build.
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
          feature="Billing"
          description="Take payments and auto-issue licenses for your products."
          perks={[
            "Map each Stripe payment link to a product + tier — no checkout metadata",
            "Webhook secret stored encrypted, editable here",
            "Issuance recorded under Licenses",
          ]}
          icon={<Receipt className="h-7 w-7" />}
          pricingHref="https://octarq.com/pricing/"
        />
      </ScreenWrap>
    );
  }
  if (!cfg) return <ScreenWrap><div className="p-8 text-center text-white/40">Loading…</div></ScreenWrap>;

  return (
    <ScreenWrap>
      <PageHeader title="Billing" description="Checkout → license configuration" />

      <GlassCard className="mb-4 p-5">
        <div className="mb-3 flex items-center gap-2 text-sm text-white/70">
          Webhook routes live:
          {cfg.providers.map((p) => <Badge key={p} tone="indigo">{p}</Badge>)}
        </div>
        <p className="text-xs text-white/40">
          Point your payment platform's webhook at <code>/api/billing/webhook/&lt;provider&gt;</code>.
        </p>
      </GlassCard>

      {/* Price map */}
      <GlassCard className="mb-4 p-5">
        <div className="mb-3 flex items-center justify-between">
          <div>
            <h3 className="text-sm font-semibold text-white/90">Price map</h3>
            <p className="text-xs text-white/40">
              Map each Stripe Payment Link (<code>plink_…</code>) to a product + tier. The webhook
              resolves sales through this table — no <code>metadata</code> to set on checkout.
            </p>
          </div>
          <Button variant="subtle" onClick={() => setEditing("new")}><Plus className="h-3.5 w-3.5" /> Mapping</Button>
        </div>
        {prices.length === 0 ? (
          <Empty>
            <Receipt className="mb-2 h-8 w-8 text-white/30" />
            <p className="text-sm text-white/45">No mappings yet — add one per Payment Link.</p>
          </Empty>
        ) : (
          <div className="overflow-hidden rounded-xl border border-white/8">
            <table className="w-full text-sm">
              <thead className="text-left text-white/40">
                <tr className="border-b border-white/8">
                  <th className="px-3 py-2 font-medium">Stripe ref</th>
                  <th className="px-3 py-2 font-medium">Product</th>
                  <th className="px-3 py-2 font-medium">Tier</th>
                  <th className="px-3 py-2 font-medium">Term</th>
                  <th className="px-3 py-2"></th>
                </tr>
              </thead>
              <tbody className="divide-y divide-white/5">
                {prices.map((m) => (
                  <tr key={m.id} className="text-white/75">
                    <td className="px-3 py-2 font-mono text-xs">{m.stripeRef}</td>
                    <td className="px-3 py-2">{m.productSlug}</td>
                    <td className="px-3 py-2"><Badge tone="violet">{m.tier}</Badge></td>
                    <td className="px-3 py-2 text-white/45">{m.term || "—"}</td>
                    <td className="px-3 py-2 text-right">
                      <button onClick={() => setEditing(m)} className="text-xs text-white/50 hover:text-white">Edit</button>
                      <span className="text-white/20"> · </span>
                      <button onClick={() => delPrice(m.id)} className="text-xs text-rose-300/80 hover:text-rose-300">Delete</button>
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        )}
      </GlassCard>

      {/* Webhook secret */}
      <GlassCard className="p-5">
        <Field
          label="Stripe webhook secret"
          hint={cfg.webhookSecretSet ? "A secret is set — enter a new one to replace it." : "Not set. Paste your whsec_… secret."}
        >
          <input className="input w-full font-mono text-xs" type="password" value={secret}
            onChange={(e) => setSecret(e.target.value)} placeholder={cfg.webhookSecretSet ? "••••••••" : "whsec_…"} />
        </Field>
        <div className="flex items-center gap-2">
          <Button variant="primary" onClick={() => saveSecret(secret.trim())} disabled={busy || secret.trim() === ""}>
            Save secret
          </Button>
          {cfg.webhookSecretSet && (
            <Button variant="danger" onClick={() => saveSecret("")} disabled={busy}>Clear</Button>
          )}
        </div>
        {msg && <p className="mt-3 text-sm text-emerald-300">{msg}</p>}
      </GlassCard>

      {editing && (
        <PriceModal
          price={editing === "new" ? null : editing}
          onClose={() => setEditing(null)}
          onSaved={() => { setEditing(null); load(); }}
        />
      )}
    </ScreenWrap>
  );
}

function PriceModal({ price, onClose, onSaved }: { price: PriceMap | null; onClose: () => void; onSaved: () => void }) {
  const [f, setF] = useState<PriceMapInput>({
    stripeRef: price?.stripeRef ?? "",
    productSlug: price?.productSlug ?? "",
    tier: price?.tier ?? "pro",
    term: price?.term ?? "",
  });
  const [busy, setBusy] = useState(false);

  async function save() {
    setBusy(true);
    try {
      if (price) await api.updateBillingPrice(price.id, f);
      else await api.createBillingPrice(f);
      onSaved();
    } catch (e) { alert((e as ApiError).message); } finally { setBusy(false); }
  }

  return (
    <Modal title={price ? "Edit price mapping" : "New price mapping"} onClose={onClose}>
      <Field label="Stripe ref" hint="The Payment Link id (plink_…) — find it on the link in Stripe.">
        <input className="input w-full font-mono text-xs" value={f.stripeRef}
          onChange={(e) => setF({ ...f, stripeRef: e.target.value })} placeholder="plink_…" />
      </Field>
      <div className="grid gap-x-3 sm:grid-cols-3">
        <Field label="Product slug"><input className="input w-full" value={f.productSlug} onChange={(e) => setF({ ...f, productSlug: e.target.value })} placeholder="octarq" /></Field>
        <Field label="Tier">
          <select className="input w-full" value={f.tier} onChange={(e) => setF({ ...f, tier: e.target.value })}>
            <option value="pro">pro</option>
            <option value="elite">elite</option>
          </select>
        </Field>
        <Field label="Term" hint="for expiry">
          <select className="input w-full" value={f.term} onChange={(e) => setF({ ...f, term: e.target.value })}>
            <option value="">(subscription)</option>
            <option value="monthly">monthly</option>
            <option value="yearly">yearly</option>
            <option value="lifetime">lifetime</option>
          </select>
        </Field>
      </div>
      <div className="mt-2 flex justify-end gap-2">
        <Button variant="ghost" onClick={onClose}>Cancel</Button>
        <Button variant="primary" onClick={save} disabled={busy || f.stripeRef.trim() === "" || f.productSlug.trim() === ""}>
          {busy ? "Saving…" : "Save"}
        </Button>
      </div>
    </Modal>
  );
}
