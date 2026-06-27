import { useEffect, useState } from "react";
import { api, SSHKey } from "../api";
import { Empty, Field, Modal, timeAgo, Code } from "../ui";

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
      <div className="card flex flex-col items-center justify-center gap-4 py-20 px-6 text-center">
        <div className="text-5xl">{error.status === 402 ? "🔒" : "🔌"}</div>
        <div>
          <h2 className="text-xl font-bold mb-1">
            {error.status === 402 ? "Pro Feature Locked" : "Feature Unavailable"}
          </h2>
          <p className="text-sm text-white/55 max-w-md mx-auto">
            {error.status === 402
              ? "A valid led-pro license is required to manage SSH keys."
              : "The SSH keys management feature is not available or disabled in this installation."}
          </p>
        </div>
        {error.status === 402 && (
          <a
            href="/settings/license"
            className="btn-primary mt-2"
          >
            Manage License
          </a>
        )}
      </div>
    );
  }

  return (
    <div>
      <div className="mb-6 flex items-center justify-between">
        <div>
          <h1 className="font-display text-2xl font-bold tracking-tight text-white">SSH Keys</h1>
          <p className="text-sm text-white/55">Manage private keys for your VPS infrastructure.</p>
        </div>
        <button className="btn-primary" onClick={() => setShowAdd(true)}>
          + New Key
        </button>
      </div>

      {keys.length === 0 ? (
        <Empty>
          <div className="text-4xl mb-2">🔑</div>
          <p>No SSH keys yet.</p>
          <button className="btn-primary mt-4" onClick={() => setShowAdd(true)}>
            Add Key
          </button>
        </Empty>
      ) : (
        <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 gap-4">
          {keys.map((k) => (
            <div key={k.id} className="card p-4 relative group flex flex-col">
              <div className="flex justify-between items-start mb-2">
                <div className="font-semibold text-lg">{k.name}</div>
                <div className="text-xs px-2 py-0.5 rounded bg-white/[0.06] text-white/55 uppercase tracking-wide">
                  {k.type}
                </div>
              </div>
              
              <div className="text-xs text-white/40 mb-4">Added {timeAgo(k.createdAt)}</div>
              
              <div className="mb-4 flex-1">
                <div className="text-xs text-white/40 mb-1">Public Key</div>
                <Code>{k.pubKey.length > 50 ? k.pubKey.slice(0, 47) + "..." : k.pubKey}</Code>
              </div>

              <div className="border-t border-white/[0.06] pt-3 flex justify-between items-center">
                <button 
                  className="text-xs text-indigo-400 hover:text-indigo-300 transition-colors"
                  onClick={() => {
                    navigator.clipboard.writeText(k.pubKey);
                    alert("Public key copied to clipboard");
                  }}
                >
                  Copy PubKey
                </button>
                <button
                  className="text-xs text-red-500/0 group-hover:text-red-500 transition-colors"
                  onClick={async () => {
                    if (!confirm(`Delete key ${k.name}? Any VPS using this key will fail health checks.`)) return;
                    try {
                      await api.deleteSSHKey(k.id);
                      load();
                    } catch (e: any) {
                      alert(e.message);
                    }
                  }}
                >
                  Delete
                </button>
              </div>
            </div>
          ))}
        </div>
      )}

      {showAdd && <AddModal onClose={() => setShowAdd(false)} onAdded={load} />}
    </div>
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
        <div className="mb-4 text-green-400 flex items-center gap-2">
          <span className="text-xl">✓</span>
          The SSH key pair was successfully generated.
        </div>
        <div className="mb-4">
          <p className="text-sm text-white/75 mb-2">
            Please copy this private key and store it securely if you need it outside of led. 
            <span className="font-bold text-red-400"> It will not be shown again.</span>
          </p>
          <textarea
            readOnly
            className="input w-full h-48 font-mono text-xs whitespace-pre bg-black"
            value={generatedPrivKey}
            onClick={(e) => (e.target as HTMLTextAreaElement).select()}
          />
        </div>
        <button 
          className="btn-primary w-full"
          onClick={() => {
            onAdded();
            onClose();
          }}
        >
          I have saved it, finish
        </button>
      </Modal>
    );
  }

  return (
    <Modal title="Add SSH Key" onClose={onClose}>
      <form onSubmit={submit}>
        <Field label="Key Name" hint="A memorable name for this key, e.g. 'prod-key'">
          <input
            className="input w-full"
            value={name}
            onChange={(e) => setName(e.target.value)}
            required
            autoFocus
          />
        </Field>
        
        <Field label="Action">
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

        {err && <p className="mb-4 text-sm text-red-400">{err}</p>}
        
        <div className="flex justify-end gap-2 mt-6">
          <button type="button" className="btn-ghost" onClick={onClose}>
            Cancel
          </button>
          <button type="submit" className="btn-primary" disabled={busy || !name}>
            {busy ? "..." : (type === "imported" ? "Import" : "Generate")}
          </button>
        </div>
      </form>
    </Modal>
  );
}
