import { useEffect, useState } from "react";
import { api, SSHKey } from "../api";
import { Empty, Field, Modal, timeAgo, Code, ScreenWrap, PageHeader, GlassCard, Badge, Button } from "../ui";
import { ShieldAlert, Key, ClipboardCopy, Trash2 } from "lucide-react";

export default function SSHKeysPage() {
  const [keys, setKeys] = useState<SSHKey[]>([]);
  const [showAdd, setShowAdd] = useState(false);
  const [error, setError] = useState<{ status: number; message: string } | null>(null);

  function load() {
    api.sshKeys()
      .then(setKeys)
      .catch((err) => setError({ status: err.status, message: err.message }));
  }

  useEffect(() => {
    load();
  }, []);

  if (error) {
    return (
      <ScreenWrap>
        <GlassCard className="flex flex-col items-center justify-center gap-5 py-16 px-6 text-center max-w-md mx-auto mt-12">
          <div className="h-14 w-14 rounded-2xl bg-rose-500/10 flex items-center justify-center text-rose-400">
            <ShieldAlert className="h-8 w-8" />
          </div>
          <div>
            <h2 className="text-xl font-bold mb-2">
              {error.status === 402 ? "Pro Feature Locked" : "Feature Unavailable"}
            </h2>
            <p className="text-sm text-white/50 leading-relaxed">
              {error.status === 402
                ? "A valid led-pro license is required to manage SSH keys."
                : "The SSH keys management feature is not available or disabled in this installation."}
            </p>
          </div>
          {error.status === 402 && (
            <Button
              variant="primary"
              onClick={() => window.location.href = "/settings/license"}
              className="mt-2"
            >
              Manage License
            </Button>
          )}
        </GlassCard>
      </ScreenWrap>
    );
  }

  const getKeyTypeTone = (type: string) => {
    if (type === "ed25519") return "indigo";
    if (type === "rsa") return "violet";
    return "neutral";
  };

  return (
    <ScreenWrap>
      <PageHeader
        title="SSH Keys"
        description="Manage private keys for your VPS remote servers"
        action={
          <Button variant="primary" onClick={() => setShowAdd(true)}>
            + New Key
          </Button>
        }
      />

      {keys.length === 0 ? (
        <Empty>
          <Key className="h-10 w-10 text-white/30 mb-2" />
          <p className="text-sm text-white/50">No SSH keys yet.</p>
          <Button variant="primary" className="mt-4" onClick={() => setShowAdd(true)}>
            Add Key
          </Button>
        </Empty>
      ) : (
        <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 gap-4">
          {keys.map((k) => (
            <GlassCard key={k.id} className="p-5 flex flex-col group relative">
              <div className="flex justify-between items-start mb-2 gap-2">
                <div className="font-semibold text-base text-white truncate">{k.name}</div>
                <Badge tone={getKeyTypeTone(k.type)} className="uppercase tracking-wider text-[9px] shrink-0">
                  {k.type}
                </Badge>
              </div>
              
              <div className="text-[11px] text-white/35 mb-4">Added {timeAgo(k.createdAt)}</div>
              
              <div className="mb-4 flex-1">
                <div className="text-[11px] font-medium text-white/40 mb-1">Public Key</div>
                <div className="text-xs break-all leading-normal bg-black/30 border border-white/[0.04] p-2.5 rounded-lg select-all font-mono">
                  {k.pubKey.length > 55 ? k.pubKey.slice(0, 52) + "..." : k.pubKey}
                </div>
              </div>

              <div className="border-t border-white/[0.06] pt-4 mt-auto flex justify-between items-center">
                <Button 
                  variant="ghost"
                  onClick={() => {
                    navigator.clipboard.writeText(k.pubKey);
                    alert("Public key copied to clipboard");
                  }}
                  className="text-xs py-1 px-2.5 text-indigo-300 hover:text-indigo-200"
                >
                  <ClipboardCopy className="h-3.5 w-3.5 mr-1" />
                  Copy PubKey
                </Button>
                <Button
                  variant="danger"
                  onClick={async () => {
                    if (!confirm(`Delete key ${k.name}? Any VPS using this key will fail health checks.`)) return;
                    try {
                      await api.deleteSSHKey(k.id);
                      load();
                    } catch (e: any) {
                      alert(e.message);
                    }
                  }}
                  className="text-xs py-1 px-2.5 text-rose-300 hover:text-rose-200 bg-rose-500/0 hover:bg-rose-500/10 border-0"
                >
                  <Trash2 className="h-3.5 w-3.5 mr-1" />
                  Delete
                </Button>
              </div>
            </GlassCard>
          ))}
        </div>
      )}

      {showAdd && <AddModal onClose={() => setShowAdd(false)} onAdded={load} />}
    </ScreenWrap>
  );
}

function AddModal({ onClose, onAdded }: { onClose: () => void; onAdded: () => void }) {
  const [name, setName] = useState("");
  const [type, setType] = useState("ed25519");
  const [privKey, setPrivKey] = useState("");
  const [busy, setBusy] = useState(false);
  const [err, setErr] = useState("");

  // After generating a new key, we show the private key once.
  const [generatedPrivKey, setGeneratedPrivKey] = useState("");

  async function submit(e: React.FormEvent) {
    e.preventDefault();
    setBusy(true);
    setErr("");
    try {
      const res = await api.createSSHKey({
        name,
        type,
        key: type === "imported" ? privKey : undefined,
      });
      if (res.rawPrivateKey) {
        setGeneratedPrivKey(res.rawPrivateKey);
      } else {
        onAdded();
        onClose();
      }
    } catch (e: any) {
      setErr(e.message);
    } finally {
      setBusy(false);
    }
  }

  if (generatedPrivKey) {
    return (
      <Modal title="Key Generated Successfully" onClose={onClose} wide>
        <div className="mb-4 text-emerald-400 flex items-center gap-2 text-sm font-medium">
          <span className="text-xl">✓</span>
          The SSH key pair was successfully generated.
        </div>
        <div className="mb-4">
          <p className="text-sm text-white/75 mb-2 leading-relaxed">
            Please copy this private key and store it securely if you need it outside of led. 
            <span className="font-bold text-rose-400"> It will not be shown again.</span>
          </p>
          <textarea
            readOnly
            className="input w-full h-48 font-mono text-xs whitespace-pre bg-black/45 border-white/[0.06] focus:border-white/10"
            value={generatedPrivKey}
            onClick={(e) => (e.target as HTMLTextAreaElement).select()}
          />
        </div>
        <Button 
          variant="primary"
          onClick={() => {
            onAdded();
            onClose();
          }}
          className="w-full"
        >
          I have saved it, finish
        </Button>
      </Modal>
    );
  }

  return (
    <Modal title="Create SSH Key" onClose={onClose}>
      <form onSubmit={submit} className="space-y-4">
        <Field label="Key Friendly Name" hint="A memorable name for this key, e.g. 'prod-key'">
          <input
            className="input w-full"
            value={name}
            onChange={(e) => setName(e.target.value)}
            required
            autoFocus
            placeholder="e.g. prod-core-key"
          />
        </Field>
        
        <Field label="Key Type / Action">
          <select 
            className="input w-full" 
            value={type} 
            onChange={(e) => setType(e.target.value)}
          >
            <option value="ed25519">Generate new Ed25519 pair (Recommended)</option>
            <option value="rsa">Generate new RSA pair</option>
            <option value="imported">Import existing private key</option>
          </select>
        </Field>

        {type === "imported" && (
          <Field label="Private Key" hint="Paste your PEM encoded private key">
            <textarea
              className="input w-full h-32 font-mono text-xs"
              placeholder="-----BEGIN OPENSSH PRIVATE KEY-----..."
              value={privKey}
              onChange={(e) => setPrivKey(e.target.value)}
              required
            />
          </Field>
        )}

        {err && <p className="text-sm text-rose-400 font-medium">{err}</p>}
        
        <div className="flex justify-end gap-2.5 pt-4 border-t border-white/[0.06]">
          <Button type="button" variant="ghost" onClick={onClose}>
            Cancel
          </Button>
          <Button type="submit" variant="primary" disabled={busy || !name}>
            {busy ? "..." : (type === "imported" ? "Import" : "Generate")}
          </Button>
        </div>
      </form>
    </Modal>
  );
}
