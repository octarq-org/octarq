// Storefront — the octarq-pro `product` plugin's admin UI. Manage the products a
// one-person company sells, their pricing plans (read by the public pricing
// page), and their downloadable releases (served to buyers by /api/delivery once
// they hold a valid license). License-gated: 402 → upsell; OSS build → 404 note.
import { useEffect, useState } from "react";
import {
  api, ApiError, Product, ProductInput, Plan, PlanInput, Release, ReleaseInput, ReleaseAsset, ProductKeyInfo,
} from "../../api";
import {
  ScreenWrap, PageHeader, GlassCard, Button, Badge, Modal, Field, Empty, Toggle, LockedFeature, timeAgo,
} from "@octarq-org/plugin-sdk";
import { Store, Package, Tag, Download, KeyRound, Pencil, Trash2, Plus, ExternalLink } from "lucide-react";
import { useTranslation } from "@octarq-org/plugin-sdk";

export default function StorefrontPage() {
  const [products, setProducts] = useState<Product[]>([]);
  const [error, setError] = useState<{ status: number } | null>(null);
  const [editing, setEditing] = useState<Product | "new" | null>(null);
  const { t } = useTranslation();

  function load() {
    api.products()
      .then((p) => { setProducts(p); setError(null); })
      .catch((e: ApiError) => setError({ status: e.status }));
  }
  useEffect(load, []);

  if (error) {
    return (
      <ScreenWrap>
        <LockedFeature
          status={error.status}
          tier="pro"
          feature={t("storefront.lockedFeature")}
          description={t("storefront.lockedDescription")}
          perks={[
            t("storefront.lockedPerk1"),
            t("storefront.lockedPerk2"),
            t("storefront.lockedPerk3"),
          ]}
          icon={<Store className="h-7 w-7" />}
          pricingHref="https://octarq.com/pricing/"
        />
      </ScreenWrap>
    );
  }

  return (
    <ScreenWrap>
      <PageHeader
        title={t("storefront.pageTitle")}
        description={t("storefront.pageDesc")}
        action={<Button variant="primary" onClick={() => setEditing("new")}>{t("storefront.addProduct")}</Button>}
      />

      {products.length === 0 ? (
        <Empty>
          <Package className="mb-2 h-10 w-10 text-white/50" />
          <p className="text-sm text-white/50">{t("storefront.noProducts")}</p>
          <Button variant="primary" className="mt-4" onClick={() => setEditing("new")}>{t("storefront.addProductShort")}</Button>
        </Empty>
      ) : (
        <div className="space-y-4">
          {products.map((p) => (
            <ProductCard key={p.id} product={p} onEdit={() => setEditing(p)} onChanged={load} />
          ))}
        </div>
      )}

      {editing && (
        <ProductModal
          product={editing === "new" ? null : editing}
          onClose={() => setEditing(null)}
          onSaved={() => { setEditing(null); load(); }}
        />
      )}
    </ScreenWrap>
  );
}

const TABS = [
  { id: "plans", labelKey: "storefront.tabPlans", Icon: Tag },
  { id: "releases", labelKey: "storefront.tabReleases", Icon: Download },
  { id: "key", labelKey: "storefront.tabKey", Icon: KeyRound },
] as const;
type TabId = (typeof TABS)[number]["id"];

function ProductCard({ product, onEdit, onChanged }: { product: Product; onEdit: () => void; onChanged: () => void }) {
  const [tab, setTab] = useState<TabId>("plans");
  const { t } = useTranslation();

  async function del() {
    if (!confirm(t("storefront.confirmDeleteProduct", { name: product.name }))) return;
    await api.deleteProduct(product.id);
    onChanged();
  }

  return (
    <GlassCard className="p-5">
      <div className="flex flex-wrap items-start justify-between gap-3">
        <div>
          <div className="flex items-center gap-2">
            <h3 className="text-lg font-semibold text-white">{product.name}</h3>
            <Badge tone={product.status === "active" ? "green" : "neutral"}>{product.status}</Badge>
            <code className="text-xs text-white/40">/{product.slug}</code>
          </div>
          {product.tagline && <p className="mt-1 text-sm text-white/55">{product.tagline}</p>}
        </div>
        <div className="flex gap-1">
          {product.homepageUrl && (
            <a href={product.homepageUrl} target="_blank" rel="noreferrer"
               className="rounded-lg p-2 text-white/50 hover:bg-white/5 hover:text-white">
              <ExternalLink className="h-4 w-4" />
            </a>
          )}
          <button onClick={onEdit} className="rounded-lg p-2 text-white/50 hover:bg-white/5 hover:text-white">
            <Pencil className="h-4 w-4" />
          </button>
          <button onClick={del} className="rounded-lg p-2 text-rose-300/80 hover:bg-rose-500/10 hover:text-rose-300">
            <Trash2 className="h-4 w-4" />
          </button>
        </div>
      </div>

      <div className="mt-4 flex gap-1 border-b border-white/5">
        {TABS.map(({ id, labelKey, Icon }) => (
          <button
            key={id}
            onClick={() => setTab(id)}
            className={"flex items-center gap-1.5 px-3 py-2 text-sm font-medium transition " +
              (tab === id ? "border-b-2 border-indigo-400 text-white" : "text-white/45 hover:text-white/70")}
          >
            <Icon className="h-4 w-4" />
            {t(labelKey)}
          </button>
        ))}
      </div>

      <div className="pt-4">
        {tab === "plans" && <PlansSection productId={product.id} />}
        {tab === "releases" && <ReleasesSection productId={product.id} />}
        {tab === "key" && <KeySection productId={product.id} />}
      </div>
    </GlassCard>
  );
}

// ── Plans ─────────────────────────────────────────────────────────────────────

function PlansSection({ productId }: { productId: number }) {
  const [plans, setPlans] = useState<Plan[]>([]);
  const [editing, setEditing] = useState<Plan | "new" | null>(null);
  const { t } = useTranslation();

  function load() { api.plans(productId).then(setPlans).catch(() => {}); }
  useEffect(load, [productId]);

  async function del(id: number) {
    if (!confirm(t("storefront.confirmDeletePlan"))) return;
    await api.deletePlan(id);
    load();
  }

  return (
    <div>
      <div className="mb-2 flex justify-end">
        <Button variant="subtle" onClick={() => setEditing("new")}><Plus className="h-3.5 w-3.5" /> {t("storefront.planButton")}</Button>
      </div>
      {plans.length === 0 ? (
        <p className="py-3 text-center text-sm text-white/50">{t("storefront.noPlans")}</p>
      ) : (
        <div className="grid gap-2 sm:grid-cols-2 lg:grid-cols-3">
          {plans.map((pl) => (
            <div key={pl.id} className="rounded-xl border border-white/8 bg-white/[0.02] p-3">
              <div className="flex items-center justify-between">
                <span className="font-medium text-white">{pl.name}</span>
                {pl.highlighted && <Badge tone="violet">{t("storefront.featured")}</Badge>}
              </div>
              <div className="mt-1 text-sm text-white/70">
                {pl.priceCents === 0 ? t("storefront.priceFree") : `${pl.currency} ${(pl.priceCents / 100).toFixed(2)}`}
                <span className="text-white/40"> / {pl.interval}</span>
                {pl.tier && <span className="ml-1 text-xs text-violet-300">· {pl.tier}</span>}
              </div>
              <div className="mt-2 flex gap-1">
                <button onClick={() => setEditing(pl)} className="text-xs text-white/50 hover:text-white">{t("storefront.edit")}</button>
                <span className="text-white/20">·</span>
                <button onClick={() => del(pl.id)} className="text-xs text-rose-300/80 hover:text-rose-300">{t("storefront.delete")}</button>
              </div>
            </div>
          ))}
        </div>
      )}
      {editing && (
        <PlanModal
          productId={productId}
          plan={editing === "new" ? null : editing}
          onClose={() => setEditing(null)}
          onSaved={() => { setEditing(null); load(); }}
        />
      )}
    </div>
  );
}

// ── Releases ──────────────────────────────────────────────────────────────────

function ReleasesSection({ productId }: { productId: number }) {
  const [releases, setReleases] = useState<Release[]>([]);
  const [adding, setAdding] = useState(false);
  const { t } = useTranslation();

  function load() { api.releases(productId).then(setReleases).catch(() => {}); }
  useEffect(load, [productId]);

  async function del(id: number) {
    if (!confirm(t("storefront.confirmDeleteRelease"))) return;
    await api.deleteRelease(id);
    load();
  }

  return (
    <div>
      <div className="mb-2 flex justify-end">
        <Button variant="subtle" onClick={() => setAdding(true)}><Plus className="h-3.5 w-3.5" /> {t("storefront.releaseButton")}</Button>
      </div>
      {releases.length === 0 ? (
        <p className="py-3 text-center text-sm text-white/50">
          {t("storefront.noReleasesPre")}<em>{t("storefront.noReleasesStable")}</em>{t("storefront.noReleasesPost")}
        </p>
      ) : (
        <div className="space-y-2">
          {releases.map((rel) => (
            <div key={rel.id} className="rounded-xl border border-white/8 bg-white/[0.02] p-3">
              <div className="flex items-center justify-between">
                <div className="flex items-center gap-2">
                  <span className="font-mono text-sm text-white">{rel.version}</span>
                  <Badge tone={rel.channel === "stable" ? "green" : "amber"}>{rel.channel}</Badge>
                  <span className="text-xs text-white/50">{timeAgo(rel.createdAt)}</span>
                </div>
                <button onClick={() => del(rel.id)} className="text-xs text-rose-300/80 hover:text-rose-300">{t("storefront.delete")}</button>
              </div>
              {rel.assets && rel.assets.length > 0 && (
                <ul className="mt-2 space-y-1">
                  {rel.assets.map((a, i) => (
                    <li key={i} className="flex items-center gap-2 text-xs text-white/55">
                      <Download className="h-3 w-3 text-white/50" />
                      <span className="text-white/70">{a.label || a.url}</span>
                      {(a.os || a.arch) && <span className="text-white/50">{[a.os, a.arch].filter(Boolean).join("/")}</span>}
                      <span className="text-white/50">{a.kind}</span>
                    </li>
                  ))}
                </ul>
              )}
            </div>
          ))}
        </div>
      )}
      {adding && (
        <ReleaseModal
          productId={productId}
          onClose={() => setAdding(false)}
          onSaved={() => { setAdding(false); load(); }}
        />
      )}
    </div>
  );
}

// ── Signing key (issuer plugin) ───────────────────────────────────────────────

function KeySection({ productId }: { productId: number }) {
  const [info, setInfo] = useState<ProductKeyInfo | null>(null);
  const [mode, setMode] = useState<"none" | "import">("none");
  const [privKey, setPrivKey] = useState("");
  const [busy, setBusy] = useState(false);
  const [note, setNote] = useState<string | null>(null);
  const { t } = useTranslation();

  function load() { api.productKey(productId).then(setInfo).catch(() => {}); }
  useEffect(load, [productId]);

  async function create(privateKey?: string) {
    setBusy(true);
    setNote(null);
    try {
      const r = await api.createProductKey(productId, privateKey);
      setNote(r.note);
      setMode("none");
      setPrivKey("");
      load();
    } catch (e) { alert((e as ApiError).message); } finally { setBusy(false); }
  }

  async function del() {
    if (!confirm(t("storefront.confirmDeleteKey"))) return;
    setBusy(true);
    try { await api.deleteProductKey(productId); setNote(null); load(); }
    catch (e) { alert((e as ApiError).message); } finally { setBusy(false); }
  }

  if (!info) return <p className="py-3 text-center text-sm text-white/50">{t("storefront.loading")}</p>;

  return (
    <div className="space-y-3">
      <p className="text-xs text-white/45">
        {t("storefront.keyIntroPre")}<strong>{t("storefront.keyIntroPublic")}</strong>{t("storefront.keyIntroMid")}<strong>{t("storefront.keyIntroPrivate")}</strong>{t("storefront.keyIntroPost")}
      </p>

      {info.hasKey ? (
        <div className="rounded-xl border border-white/8 bg-white/[0.02] p-3">
          <div className="mb-1 flex items-center justify-between">
            <span className="text-xs text-white/40">{t("storefront.publicKey")} {info.createdAt && `· ${t("storefront.addedOn", { date: info.createdAt.slice(0, 10) })}`}</span>
            <button onClick={del} disabled={busy} className="text-xs text-rose-300/80 hover:text-rose-300">{t("storefront.delete")}</button>
          </div>
          <code className="block break-all font-mono text-xs text-emerald-300/90">{info.publicKey}</code>
          <p className="mt-2 text-xs text-white/50">{t("storefront.embedNotePre")}<code>license.publicKeyB64</code>{t("storefront.embedNotePost")}</p>
        </div>
      ) : (
        <div className="rounded-xl border border-amber-400/20 bg-amber-500/[0.04] p-3">
          <p className="text-sm text-amber-200/90">{t("storefront.noKeyWarning")}</p>
          <div className="mt-3 flex flex-wrap gap-2">
            <Button variant="primary" onClick={() => create()} disabled={busy}>{t("storefront.generateKey")}</Button>
            <Button variant="subtle" onClick={() => setMode(mode === "import" ? "none" : "import")} disabled={busy}>
              {t("storefront.importExisting")}
            </Button>
          </div>
          {mode === "import" && (
            <div className="mt-3">
              <Field label={t("storefront.privateKeyLabel")} hint={t("storefront.privateKeyHint")}>
                <textarea className="input w-full font-mono text-xs" rows={3} value={privKey}
                  onChange={(e) => setPrivKey(e.target.value)} placeholder={t("storefront.privateKeyPlaceholder")} />
              </Field>
              <Button variant="primary" onClick={() => create(privKey.trim())} disabled={busy || privKey.trim() === ""}>
                {t("storefront.importKey")}
              </Button>
            </div>
          )}
        </div>
      )}

      {note && <p className="text-xs text-emerald-300">{note}</p>}
    </div>
  );
}

// ── Modals ────────────────────────────────────────────────────────────────────

function ProductModal({ product, onClose, onSaved }: { product: Product | null; onClose: () => void; onSaved: () => void }) {
  const [f, setF] = useState<ProductInput>({
    slug: product?.slug ?? "", name: product?.name ?? "", tagline: product?.tagline ?? "",
    description: product?.description ?? "", homepageUrl: product?.homepageUrl ?? "",
    status: product?.status ?? "draft",
  });
  const [busy, setBusy] = useState(false);
  const { t } = useTranslation();

  async function save() {
    setBusy(true);
    try {
      if (product) await api.updateProduct(product.id, f);
      else await api.createProduct(f);
      onSaved();
    } catch (e) { alert((e as ApiError).message); } finally { setBusy(false); }
  }

  return (
    <Modal title={product ? t("storefront.editProduct") : t("storefront.newProduct")} onClose={onClose}>
      <Field label={t("storefront.fieldName")}><input className="input w-full" value={f.name} onChange={(e) => setF({ ...f, name: e.target.value })} /></Field>
      <Field label={t("storefront.fieldSlug")} hint={t("storefront.slugHint")}>
        <input className="input w-full" value={f.slug} onChange={(e) => setF({ ...f, slug: e.target.value })} placeholder="octarq" />
      </Field>
      <Field label={t("storefront.fieldTagline")}><input className="input w-full" value={f.tagline} onChange={(e) => setF({ ...f, tagline: e.target.value })} /></Field>
      <Field label={t("storefront.fieldDescription")}><textarea className="input w-full" rows={3} value={f.description} onChange={(e) => setF({ ...f, description: e.target.value })} /></Field>
      <Field label={t("storefront.fieldHomepageUrl")}><input className="input w-full" value={f.homepageUrl} onChange={(e) => setF({ ...f, homepageUrl: e.target.value })} placeholder="https://octarq.com" /></Field>
      <div className="mb-3 flex items-center gap-3">
        <Toggle on={f.status === "active"} onChange={(v) => setF({ ...f, status: v ? "active" : "draft" })} />
        <span className="text-sm text-white/70">{t("storefront.activeLabel")}</span>
      </div>
      <div className="flex justify-end gap-2">
        <Button variant="ghost" onClick={onClose}>{t("storefront.cancel")}</Button>
        <Button variant="primary" onClick={save} disabled={busy || f.name.trim() === ""}>{busy ? t("storefront.saving") : t("storefront.save")}</Button>
      </div>
    </Modal>
  );
}

function PlanModal({ productId, plan, onClose, onSaved }: { productId: number; plan: Plan | null; onClose: () => void; onSaved: () => void }) {
  const [f, setF] = useState({
    name: plan?.name ?? "", tier: plan?.tier ?? "", interval: plan?.interval ?? "month",
    price: plan ? (plan.priceCents / 100).toString() : "", currency: plan?.currency ?? "USD",
    features: (plan?.features ?? []).join("\n"), checkoutUrl: plan?.checkoutUrl ?? "",
    highlighted: plan?.highlighted ?? false, sort: plan?.sort ?? 0,
  });
  const [busy, setBusy] = useState(false);
  const { t } = useTranslation();

  async function save() {
    setBusy(true);
    const payload: PlanInput = {
      name: f.name.trim(), tier: f.tier.trim(), interval: f.interval as PlanInput["interval"],
      priceCents: Math.round(parseFloat(f.price || "0") * 100), currency: f.currency,
      features: f.features.split("\n").map((s) => s.trim()).filter(Boolean),
      checkoutUrl: f.checkoutUrl.trim(), highlighted: f.highlighted, sort: f.sort,
    };
    try {
      if (plan) await api.updatePlan(plan.id, payload);
      else await api.createPlan(productId, payload);
      onSaved();
    } catch (e) { alert((e as ApiError).message); } finally { setBusy(false); }
  }

  return (
    <Modal title={plan ? t("storefront.editPlan") : t("storefront.newPlan")} onClose={onClose}>
      <div className="grid gap-x-3 sm:grid-cols-2">
        <Field label={t("storefront.fieldName")}><input className="input w-full" value={f.name} onChange={(e) => setF({ ...f, name: e.target.value })} placeholder="Pro" /></Field>
        <Field label={t("storefront.fieldLicenseTier")} hint={t("storefront.licenseTierHint")}><input className="input w-full" value={f.tier} onChange={(e) => setF({ ...f, tier: e.target.value })} placeholder="pro" /></Field>
        <Field label={t("storefront.fieldPrice")}><input className="input w-full" type="number" step="0.01" value={f.price} onChange={(e) => setF({ ...f, price: e.target.value })} placeholder="5.00" /></Field>
        <Field label={t("storefront.fieldCurrency")}><input className="input w-full" value={f.currency} onChange={(e) => setF({ ...f, currency: e.target.value })} /></Field>
        <Field label={t("storefront.fieldInterval")}>
          <select className="input w-full" value={f.interval} onChange={(e) => setF({ ...f, interval: e.target.value as Plan["interval"] })}>
            <option value="month">{t("storefront.intervalMonth")}</option><option value="year">{t("storefront.intervalYear")}</option><option value="once">{t("storefront.intervalOnce")}</option>
          </select>
        </Field>
        <Field label={t("storefront.fieldSort")}><input className="input w-full" type="number" value={f.sort} onChange={(e) => setF({ ...f, sort: parseInt(e.target.value || "0") })} /></Field>
      </div>
      <Field label={t("storefront.fieldFeatures")}><textarea className="input w-full" rows={4} value={f.features} onChange={(e) => setF({ ...f, features: e.target.value })} /></Field>
      <Field label={t("storefront.fieldCheckoutUrl")} hint={t("storefront.checkoutUrlHint")}><input className="input w-full" value={f.checkoutUrl} onChange={(e) => setF({ ...f, checkoutUrl: e.target.value })} /></Field>
      <div className="mb-3 flex items-center gap-3">
        <Toggle on={f.highlighted} onChange={(v) => setF({ ...f, highlighted: v })} />
        <span className="text-sm text-white/70">{t("storefront.featuredPlan")}</span>
      </div>
      <div className="flex justify-end gap-2">
        <Button variant="ghost" onClick={onClose}>{t("storefront.cancel")}</Button>
        <Button variant="primary" onClick={save} disabled={busy || f.name.trim() === ""}>{busy ? t("storefront.saving") : t("storefront.save")}</Button>
      </div>
    </Modal>
  );
}

function ReleaseModal({ productId, onClose, onSaved }: { productId: number; onClose: () => void; onSaved: () => void }) {
  const [version, setVersion] = useState("");
  const [channel, setChannel] = useState<"stable" | "beta">("stable");
  const [notes, setNotes] = useState("");
  const [assets, setAssets] = useState<ReleaseAsset[]>([{ label: "", url: "", os: "", arch: "", kind: "binary" }]);
  const [busy, setBusy] = useState(false);
  const { t } = useTranslation();

  function setAsset(i: number, patch: Partial<ReleaseAsset>) {
    setAssets(assets.map((a, j) => (j === i ? { ...a, ...patch } : a)));
  }

  async function save() {
    setBusy(true);
    const payload: ReleaseInput = {
      version: version.trim(), channel, notes,
      assets: assets.filter((a) => a.url.trim() !== ""),
    };
    try { await api.createRelease(productId, payload); onSaved(); }
    catch (e) { alert((e as ApiError).message); } finally { setBusy(false); }
  }

  return (
    <Modal title={t("storefront.newRelease")} onClose={onClose} wide>
      <div className="grid gap-x-3 sm:grid-cols-2">
        <Field label={t("storefront.fieldVersion")}><input className="input w-full" value={version} onChange={(e) => setVersion(e.target.value)} placeholder="v1.2.0" /></Field>
        <Field label={t("storefront.fieldChannel")}>
          <select className="input w-full" value={channel} onChange={(e) => setChannel(e.target.value as "stable" | "beta")}>
            <option value="stable">{t("storefront.channelStable")}</option><option value="beta">{t("storefront.channelBeta")}</option>
          </select>
        </Field>
      </div>
      <Field label={t("storefront.fieldReleaseNotes")}><textarea className="input w-full" rows={2} value={notes} onChange={(e) => setNotes(e.target.value)} /></Field>

      <label className="label">{t("storefront.assets")}</label>
      <div className="space-y-2">
        {assets.map((a, i) => (
          <div key={i} className="grid grid-cols-12 gap-2">
            <input className="input col-span-3" placeholder={t("storefront.assetLabelPlaceholder")} value={a.label} onChange={(e) => setAsset(i, { label: e.target.value })} />
            <input className="input col-span-4" placeholder={t("storefront.assetUrlPlaceholder")} value={a.url} onChange={(e) => setAsset(i, { url: e.target.value })} />
            <input className="input col-span-2" placeholder={t("storefront.assetOsPlaceholder")} value={a.os} onChange={(e) => setAsset(i, { os: e.target.value })} />
            <input className="input col-span-1" placeholder={t("storefront.assetArchPlaceholder")} value={a.arch} onChange={(e) => setAsset(i, { arch: e.target.value })} />
            <select className="input col-span-2" value={a.kind} onChange={(e) => setAsset(i, { kind: e.target.value })}>
              <option value="binary">{t("storefront.assetKindBinary")}</option><option value="image">{t("storefront.assetKindImage")}</option><option value="checksum">{t("storefront.assetKindChecksum")}</option>
            </select>
          </div>
        ))}
      </div>
      <button className="mt-2 text-xs text-indigo-300 hover:underline"
              onClick={() => setAssets([...assets, { label: "", url: "", os: "", arch: "", kind: "binary" }])}>
        {t("storefront.addAsset")}
      </button>

      <div className="mt-4 flex justify-end gap-2">
        <Button variant="ghost" onClick={onClose}>{t("storefront.cancel")}</Button>
        <Button variant="primary" onClick={save} disabled={busy || version.trim() === ""}>{busy ? t("storefront.saving") : t("storefront.createRelease")}</Button>
      </div>
    </Modal>
  );
}
