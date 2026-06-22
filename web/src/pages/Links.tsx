import { useEffect, useState } from "react";
import { api, Link, LinkStats } from "../api";
import { Empty, Field, Modal, Toggle, timeAgo } from "../ui";

export default function LinksPage() {
  const [links, setLinks] = useState<Link[]>([]);
  const [q, setQ] = useState("");
  const [editing, setEditing] = useState<Link | "new" | null>(null);
  const [statsFor, setStatsFor] = useState<Link | null>(null);
  const [qrFor, setQrFor] = useState<Link | null>(null);

  async function load() {
    setLinks(await api.links(q));
  }
  useEffect(() => {
    load();
  }, [q]);

  return (
    <div>
      <Header title="Links" subtitle="Short links with click analytics & notes">
        <button className="btn-primary" onClick={() => setEditing("new")}>
          + New link
        </button>
      </Header>

      <input
        className="input mb-4"
        placeholder="Search slug, target, note…"
        value={q}
        onChange={(e) => setQ(e.target.value)}
      />

      {links.length === 0 ? (
        <Empty>
          <span className="text-3xl">🔗</span>
          <p>No links yet</p>
        </Empty>
      ) : (
        <div className="card divide-y divide-zinc-800">
          {links.map((l) => (
            <div key={l.id} className="flex items-center gap-3 p-3">
              <div className="min-w-0 flex-1">
                <div className="flex items-center gap-2">
                  <span className="font-medium text-indigo-300">
                    /{l.slug}
                    {l.host && <span className="text-zinc-500"> @{l.host}</span>}
                  </span>
                  {!l.enabled && <span className="badge">disabled</span>}
                  {l.hasPassword && <span className="badge">🔒</span>}
                  {l.expiresAt && <span className="badge">⏳</span>}
                </div>
                <div className="truncate text-sm text-zinc-400">{l.target}</div>
                {l.note && <div className="mt-0.5 truncate text-xs text-amber-300/80">📝 {l.note}</div>}
              </div>
              <div className="hidden text-right text-sm sm:block">
                <div className="font-semibold">{l.clicks}</div>
                <div className="text-xs text-zinc-500">clicks</div>
              </div>
              <div className="flex items-center gap-1">
                <button className="btn-ghost px-2" title="Stats" onClick={() => setStatsFor(l)}>
                  📊
                </button>
                <button className="btn-ghost px-2" title="QR" onClick={() => setQrFor(l)}>
                  ▦
                </button>
                <button className="btn-ghost px-2" onClick={() => setEditing(l)}>
                  Edit
                </button>
                <button
                  className="btn-danger px-2"
                  onClick={async () => {
                    if (confirm(`Delete /${l.slug}?`)) {
                      await api.deleteLink(l.id);
                      load();
                    }
                  }}
                >
                  Del
                </button>
              </div>
            </div>
          ))}
        </div>
      )}

      {editing && (
        <LinkEditor
          link={editing === "new" ? null : editing}
          onClose={() => setEditing(null)}
          onSaved={() => {
            setEditing(null);
            load();
          }}
        />
      )}
      {statsFor && <StatsModal link={statsFor} onClose={() => setStatsFor(null)} />}
      {qrFor && <QRModal link={qrFor} onClose={() => setQrFor(null)} />}
    </div>
  );
}

export function Header({
  title,
  subtitle,
  children,
}: {
  title: string;
  subtitle?: string;
  children?: React.ReactNode;
}) {
  return (
    <div className="mb-5 flex items-end justify-between">
      <div>
        <h1 className="text-2xl font-bold">{title}</h1>
        {subtitle && <p className="text-sm text-zinc-500">{subtitle}</p>}
      </div>
      {children}
    </div>
  );
}

function LinkEditor({
  link,
  onClose,
  onSaved,
}: {
  link: Link | null;
  onClose: () => void;
  onSaved: () => void;
}) {
  const [slug, setSlug] = useState(link?.slug ?? "");
  const [host, setHost] = useState(link?.host ?? "");
  const [target, setTarget] = useState(link?.target ?? "");
  const [title, setTitle] = useState(link?.title ?? "");
  const [note, setNote] = useState(link?.note ?? "");
  const [password, setPassword] = useState("");
  const [expiresAt, setExpiresAt] = useState(link?.expiresAt?.slice(0, 16) ?? "");
  const [enabled, setEnabled] = useState(link?.enabled ?? true);
  const [err, setErr] = useState("");

  async function save() {
    setErr("");
    const payload: any = {
      slug,
      host,
      target,
      title,
      note,
      password,
      enabled,
      expiresAt: expiresAt ? new Date(expiresAt).toISOString() : null,
    };
    try {
      if (link) await api.updateLink(link.id, payload);
      else await api.createLink(payload);
      onSaved();
    } catch (e: any) {
      setErr(e.message ?? "save failed");
    }
  }

  return (
    <Modal title={link ? "Edit link" : "New link"} onClose={onClose}>
      <Field label="Destination URL">
        <input className="input" value={target} onChange={(e) => setTarget(e.target.value)} placeholder="example.com/page" />
      </Field>
      <div className="grid grid-cols-2 gap-3">
        <Field label="Slug" hint="blank = random">
          <input className="input" value={slug} onChange={(e) => setSlug(e.target.value)} />
        </Field>
        <Field label="Host" hint="blank = any domain">
          <input className="input" value={host} onChange={(e) => setHost(e.target.value)} placeholder="go.example.com" />
        </Field>
      </div>
      <Field label="Title">
        <input className="input" value={title} onChange={(e) => setTitle(e.target.value)} />
      </Field>
      <Field label="Note" hint="Private remark (not shown to visitors)">
        <textarea className="input" rows={2} value={note} onChange={(e) => setNote(e.target.value)} />
      </Field>
      <div className="grid grid-cols-2 gap-3">
        <Field label="Password" hint={link?.hasPassword ? "set, leave blank to keep" : "optional"}>
          <input className="input" value={password} onChange={(e) => setPassword(e.target.value)} />
        </Field>
        <Field label="Expires at">
          <input type="datetime-local" className="input" value={expiresAt} onChange={(e) => setExpiresAt(e.target.value)} />
        </Field>
      </div>
      <div className="mb-4 flex items-center gap-2">
        <Toggle on={enabled} onChange={setEnabled} />
        <span className="text-sm text-zinc-400">Enabled</span>
      </div>
      {err && <p className="mb-3 text-sm text-red-400">{err}</p>}
      <div className="flex justify-end gap-2">
        <button className="btn-ghost" onClick={onClose}>
          Cancel
        </button>
        <button className="btn-primary" onClick={save}>
          Save
        </button>
      </div>
    </Modal>
  );
}

function StatsModal({ link, onClose }: { link: Link; onClose: () => void }) {
  const [stats, setStats] = useState<LinkStats | null>(null);
  useEffect(() => {
    api.linkStats(link.id).then(setStats);
  }, [link.id]);
  return (
    <Modal title={`Stats · /${link.slug}`} onClose={onClose} wide>
      {!stats ? (
        <p className="text-zinc-500">loading…</p>
      ) : (
        <div className="space-y-4">
          <div className="grid grid-cols-3 gap-3">
            <Stat label="Total clicks" value={stats.total} />
            <Stat label={`Last ${stats.days}d`} value={stats.windowed} />
            <Stat label="Days tracked" value={stats.series.length} />
          </div>
          <Spark series={stats.series} />
          <div className="grid grid-cols-2 gap-4">
            <TopList title="Countries" rows={stats.countries} />
            <TopList title="Devices" rows={stats.devices} />
            <TopList title="Browsers" rows={stats.browsers} />
            <TopList title="Referers" rows={stats.referers} />
          </div>
        </div>
      )}
    </Modal>
  );
}

function Stat({ label, value }: { label: string; value: number }) {
  return (
    <div className="card p-3">
      <div className="text-2xl font-bold">{value}</div>
      <div className="text-xs text-zinc-500">{label}</div>
    </div>
  );
}

function Spark({ series }: { series: { key: string; count: number }[] }) {
  if (!series.length) return <p className="text-sm text-zinc-500">No clicks in window.</p>;
  const max = Math.max(...series.map((s) => s.count), 1);
  return (
    <div className="card flex h-28 items-end gap-1 p-3">
      {series.map((s) => (
        <div
          key={s.key}
          title={`${s.key}: ${s.count}`}
          className="flex-1 rounded-t bg-indigo-500/70"
          style={{ height: `${(s.count / max) * 100}%`, minHeight: 2 }}
        />
      ))}
    </div>
  );
}

function TopList({ title, rows }: { title: string; rows: { key: string; count: number }[] | null }) {
  return (
    <div>
      <div className="mb-1 text-xs font-semibold uppercase tracking-wide text-zinc-500">{title}</div>
      {!rows || rows.length === 0 ? (
        <p className="text-sm text-zinc-600">—</p>
      ) : (
        <div className="space-y-1">
          {rows.map((r) => (
            <div key={r.key} className="flex justify-between text-sm">
              <span className="truncate text-zinc-300">{r.key || "(direct)"}</span>
              <span className="text-zinc-500">{r.count}</span>
            </div>
          ))}
        </div>
      )}
    </div>
  );
}

function QRModal({ link, onClose }: { link: Link; onClose: () => void }) {
  return (
    <Modal title={`QR · /${link.slug}`} onClose={onClose}>
      <div className="flex flex-col items-center gap-3">
        <img
          src={`/api/links/${link.id}/qr`}
          alt="QR"
          className="rounded-lg bg-white p-2"
          width={240}
          height={240}
        />
        <a className="btn-ghost" href={`/api/links/${link.id}/qr`} download={`${link.slug}.png`}>
          Download PNG
        </a>
      </div>
    </Modal>
  );
}
