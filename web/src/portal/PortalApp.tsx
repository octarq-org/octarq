import { useEffect, useState } from "react";
import { Routes, Route, Link, useNavigate, useSearchParams } from "react-router-dom";
import { api, ApiError, IssuedLicense, LicenseDevice } from "../api";
import { ScreenWrap, PageHeader, GlassCard, Badge, Button, Empty } from "../ui";
import { KeyRound, LogOut, Laptop, ExternalLink, ShieldAlert, ArrowRight, CheckCircle } from "lucide-react";

export default function PortalApp() {
  const [customerEmail, setCustomerEmail] = useState<string | null>(null);
  const [loading, setLoading] = useState(true);

  // Check login status on boot
  useEffect(() => {
    api.customerMe()
      .then((res) => {
        setCustomerEmail(res.email);
        setLoading(false);
      })
      .catch(() => {
        setCustomerEmail(null);
        setLoading(false);
      });
  }, []);

  const handleLogout = async () => {
    try {
      await api.customerLogout();
      setCustomerEmail(null);
    } catch (e) {
      console.error(e);
    }
  };

  if (loading) {
    return (
      <div className="flex min-h-screen items-center justify-center bg-[#0b0b0f] text-white/50 text-sm">
        Loading Portal...
      </div>
    );
  }

  return (
    <div className="min-h-screen bg-[#0b0b0f] text-white">
      <Routes>
        <Route
          path="/"
          element={
            customerEmail ? (
              <DashboardView email={customerEmail} onLogout={handleLogout} />
            ) : (
              <LoginRedirect />
            )
          }
        />
        <Route path="/login" element={<LoginView onLogin={setCustomerEmail} />} />
        <Route path="/register" element={<RegisterView onRegister={setCustomerEmail} />} />
        <Route path="/claim" element={<ClaimView onClaim={setCustomerEmail} />} />
      </Routes>
    </div>
  );
}

function LoginRedirect() {
  const navigate = useNavigate();
  useEffect(() => {
    navigate("/login", { replace: true });
  }, [navigate]);
  return null;
}

// ─── LOGIN VIEW ──────────────────────────────────────────────────────────────
function LoginView({ onLogin }: { onLogin: (email: string) => void }) {
  const [email, setEmail] = useState("");
  const [password, setPassword] = useState("");
  const [error, setError] = useState("");
  const [submitting, setSubmitting] = useState(false);
  const navigate = useNavigate();

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault();
    setError("");
    setSubmitting(true);
    try {
      const res = await api.customerLogin(email, password);
      onLogin(res.email);
      navigate("/", { replace: true });
    } catch (err: any) {
      setError(err.error || "Invalid credentials");
    } finally {
      setSubmitting(false);
    }
  };

  return (
    <ScreenWrap className="flex items-center justify-center min-h-[85vh]">
      <GlassCard className="w-full max-w-md p-8">
        <div className="text-center mb-6">
          <KeyRound className="mx-auto h-12 w-12 text-indigo-400 mb-2" />
          <h2 className="text-2xl font-bold text-white">Customer Portal</h2>
          <p className="text-xs text-white/45 mt-1">Manage your licenses, devices, and billing</p>
        </div>

        {error && (
          <div className="mb-4 p-3 rounded-xl bg-rose-500/10 border border-rose-500/20 text-rose-300 text-xs flex gap-2 items-center">
            <ShieldAlert className="h-4 w-4 shrink-0" />
            <span>{error}</span>
          </div>
        )}

        <form onSubmit={handleSubmit} className="space-y-4">
          <div>
            <label className="block text-xs font-medium text-white/60 mb-1">Email Address</label>
            <input
              type="email"
              required
              value={email}
              onChange={(e) => setEmail(e.target.value)}
              className="w-full rounded-xl bg-white/5 border border-white/10 px-3.5 py-2.5 text-sm text-white focus:outline-none focus:border-indigo-500"
              placeholder="you@domain.com"
            />
          </div>
          <div>
            <label className="block text-xs font-medium text-white/60 mb-1">Password</label>
            <input
              type="password"
              required
              value={password}
              onChange={(e) => setPassword(e.target.value)}
              className="w-full rounded-xl bg-white/5 border border-white/10 px-3.5 py-2.5 text-sm text-white focus:outline-none focus:border-indigo-500"
              placeholder="••••••••"
            />
          </div>
          <Button type="submit" disabled={submitting} className="w-full py-2.5 mt-2">
            {submitting ? "Signing in..." : "Sign In"}
          </Button>
        </form>

        <div className="mt-6 text-center text-xs text-white/40">
          Need to link a purchase?{" "}
          <Link to="/claim" className="text-indigo-400 hover:text-indigo-300 font-medium">
            Claim License
          </Link>
          {" | "}
          <Link to="/register" className="text-indigo-400 hover:text-indigo-300 font-medium">
            Register Account
          </Link>
        </div>
      </GlassCard>
    </ScreenWrap>
  );
}

// ─── REGISTER VIEW ───────────────────────────────────────────────────────────
function RegisterView({ onRegister }: { onRegister: (email: string) => void }) {
  const [email, setEmail] = useState("");
  const [password, setPassword] = useState("");
  const [error, setError] = useState("");
  const [submitting, setSubmitting] = useState(false);
  const navigate = useNavigate();

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault();
    setError("");
    setSubmitting(true);
    try {
      const res = await api.customerRegister(email, password);
      onRegister(res.email);
      navigate("/", { replace: true });
    } catch (err: any) {
      setError(err.error || "Registration failed");
    } finally {
      setSubmitting(false);
    }
  };

  return (
    <ScreenWrap className="flex items-center justify-center min-h-[85vh]">
      <GlassCard className="w-full max-w-md p-8">
        <div className="text-center mb-6">
          <KeyRound className="mx-auto h-12 w-12 text-indigo-400 mb-2" />
          <h2 className="text-2xl font-bold text-white">Create Portal Account</h2>
          <p className="text-xs text-white/45 mt-1">Register to manage your purchases</p>
        </div>

        {error && (
          <div className="mb-4 p-3 rounded-xl bg-rose-500/10 border border-rose-500/20 text-rose-300 text-xs flex gap-2 items-center">
            <ShieldAlert className="h-4 w-4 shrink-0" />
            <span>{error}</span>
          </div>
        )}

        <form onSubmit={handleSubmit} className="space-y-4">
          <div>
            <label className="block text-xs font-medium text-white/60 mb-1">Email Address</label>
            <input
              type="email"
              required
              value={email}
              onChange={(e) => setEmail(e.target.value)}
              className="w-full rounded-xl bg-white/5 border border-white/10 px-3.5 py-2.5 text-sm text-white focus:outline-none focus:border-indigo-500"
              placeholder="you@domain.com"
            />
          </div>
          <div>
            <label className="block text-xs font-medium text-white/60 mb-1">Password (min 8 chars)</label>
            <input
              type="password"
              required
              value={password}
              onChange={(e) => setPassword(e.target.value)}
              className="w-full rounded-xl bg-white/5 border border-white/10 px-3.5 py-2.5 text-sm text-white focus:outline-none focus:border-indigo-500"
              placeholder="••••••••"
            />
          </div>
          <Button type="submit" disabled={submitting} className="w-full py-2.5 mt-2">
            {submitting ? "Registering..." : "Register"}
          </Button>
        </form>

        <div className="mt-6 text-center text-xs text-white/40">
          Already have an account?{" "}
          <Link to="/login" className="text-indigo-400 hover:text-indigo-300 font-medium">
            Sign In
          </Link>
        </div>
      </GlassCard>
    </ScreenWrap>
  );
}

// ─── CLAIM VIEW ──────────────────────────────────────────────────────────────
function ClaimView({ onClaim }: { onClaim: (email: string) => void }) {
  const [searchParams] = useSearchParams();
  const [sessionId, setSessionId] = useState(searchParams.get("sessionId") || "");
  const [password, setPassword] = useState("");
  const [error, setError] = useState("");
  const [success, setSuccess] = useState(false);
  const [submitting, setSubmitting] = useState(false);
  const navigate = useNavigate();

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault();
    setError("");
    setSubmitting(true);
    try {
      const res = await api.claimAccount(sessionId, password);
      onClaim(res.email);
      setSuccess(true);
      setTimeout(() => {
        navigate("/", { replace: true });
      }, 2000);
    } catch (err: any) {
      if (err.status === 409) {
        // Account exists
        setError("An account already exists for this purchase. Please sign in to view your licenses.");
      } else {
        setError(err.error || "Could not claim purchase. Ensure the Session ID is correct.");
      }
    } finally {
      setSubmitting(false);
    }
  };

  return (
    <ScreenWrap className="flex items-center justify-center min-h-[85vh]">
      <GlassCard className="w-full max-w-md p-8">
        <div className="text-center mb-6">
          <CheckCircle className="mx-auto h-12 w-12 text-emerald-400 mb-2" />
          <h2 className="text-2xl font-bold text-white">Claim Your Purchase</h2>
          <p className="text-xs text-white/45 mt-1">Set a password to activate your portal account</p>
        </div>

        {error && (
          <div className="mb-4 p-3 rounded-xl bg-rose-500/10 border border-rose-500/20 text-rose-300 text-xs flex gap-2 items-center">
            <ShieldAlert className="h-4 w-4 shrink-0" />
            <span>{error}</span>
          </div>
        )}

        {success ? (
          <div className="p-4 rounded-xl bg-emerald-500/10 border border-emerald-500/20 text-emerald-300 text-sm text-center">
            Account successfully claimed! Redirecting...
          </div>
        ) : (
          <form onSubmit={handleSubmit} className="space-y-4">
            <div>
              <label className="block text-xs font-medium text-white/60 mb-1">Stripe Checkout / Purchase Session ID</label>
              <input
                type="text"
                required
                value={sessionId}
                onChange={(e) => setSessionId(e.target.value)}
                className="w-full rounded-xl bg-white/5 border border-white/10 px-3.5 py-2.5 text-sm text-white focus:outline-none focus:border-indigo-500"
                placeholder="cs_live_..."
              />
            </div>
            <div>
              <label className="block text-xs font-medium text-white/60 mb-1">Set Portal Password</label>
              <input
                type="password"
                required
                value={password}
                onChange={(e) => setPassword(e.target.value)}
                className="w-full rounded-xl bg-white/5 border border-white/10 px-3.5 py-2.5 text-sm text-white focus:outline-none focus:border-indigo-500"
                placeholder="Choose a password (min 8 chars)"
              />
            </div>
            <Button type="submit" disabled={submitting} className="w-full py-2.5 mt-2">
              {submitting ? "Linking Purchase..." : "Link Purchase & Sign In"}
            </Button>
          </form>
        )}

        <div className="mt-6 text-center text-xs text-white/40">
          Already have a login?{" "}
          <Link to="/login" className="text-indigo-400 hover:text-indigo-300 font-medium">
            Sign In
          </Link>
        </div>
      </GlassCard>
    </ScreenWrap>
  );
}

// ─── DASHBOARD VIEW ──────────────────────────────────────────────────────────
function DashboardView({ email, onLogout }: { email: string; onLogout: () => void }) {
  const [licenses, setLicenses] = useState<IssuedLicense[]>([]);
  const [selectedLicense, setSelectedLicense] = useState<IssuedLicense | null>(null);
  const [devices, setDevices] = useState<LicenseDevice[]>([]);
  const [loadingLics, setLoadingLics] = useState(true);
  const [loadingDevs, setLoadingDevs] = useState(false);
  const [unverified, setUnverified] = useState(false);
  const [billingUrl, setBillingUrl] = useState("");
  const [claimingBilling, setClaimingBilling] = useState(false);

  useEffect(() => {
    setLoadingLics(true);
    api.portalLicenses()
      .then((res) => {
        setLicenses(res.licenses);
        setLoadingLics(false);
      })
      .catch((err: ApiError) => {
        if (err.status === 403) {
          setUnverified(true);
        }
        setLoadingLics(false);
      });
  }, []);

  const viewDevices = (lic: IssuedLicense) => {
    setSelectedLicense(lic);
    setLoadingDevs(true);
    api.portalDevices(lic.id)
      .then(setDevices)
      .catch(() => setDevices([]))
      .finally(() => setLoadingDevs(false));
  };

  const handleUnbind = async (deviceId: number) => {
    if (!selectedLicense) return;
    try {
      await api.portalUnbindDevice(selectedLicense.id, deviceId);
      setDevices(devices.filter((d) => d.id !== deviceId));
    } catch (e) {
      console.error(e);
    }
  };

  const handleBillingPortal = async () => {
    setClaimingBilling(true);
    try {
      const res = await api.portalBillingPortal();
      window.location.href = res.url;
    } catch (e: any) {
      alert(e.error || "Could not open billing portal");
    } finally {
      setClaimingBilling(false);
    }
  };

  return (
    <ScreenWrap>
      <div className="flex justify-between items-center border-b border-white/8 pb-4 mb-6">
        <div>
          <h1 className="text-xl font-bold text-white flex items-center gap-2">
            <KeyRound className="h-5 w-5 text-indigo-400" />
            <span>Customer Portal</span>
          </h1>
          <p className="text-xs text-white/50">{email}</p>
        </div>
        <div className="flex gap-2">
          <Button variant="outline" onClick={handleBillingPortal} disabled={claimingBilling} className="text-xs py-1.5 h-8">
            <ExternalLink className="h-3.5 w-3.5" />
            <span>{claimingBilling ? "Loading..." : "Billing & Invoices"}</span>
          </Button>
          <Button variant="ghost" onClick={onLogout} className="text-xs py-1.5 h-8">
            <LogOut className="h-3.5 w-3.5" />
            <span>Logout</span>
          </Button>
        </div>
      </div>

      {unverified ? (
        <GlassCard className="p-6 text-center max-w-lg mx-auto mt-8">
          <ShieldAlert className="h-10 w-10 text-amber-400 mx-auto mb-3" />
          <h3 className="font-semibold text-lg">Verify Your Account</h3>
          <p className="text-sm text-white/60 mt-2 mb-4">
            To view purchased licenses, you must link your purchase to this account.
          </p>
          <Link to="/claim">
            <Button>
              <span>Claim / Link Purchase</span>
              <ArrowRight className="h-4 w-4" />
            </Button>
          </Link>
        </GlassCard>
      ) : loadingLics ? (
        <div className="text-center py-10 text-white/40 text-sm">Loading your licenses...</div>
      ) : licenses.length === 0 ? (
        <Empty>
          <KeyRound className="h-10 w-10 text-white/20 mb-2" />
          <p className="text-sm text-white/50">No licenses linked to your account.</p>
          <p className="text-xs text-white/35 mt-1">If you just made a purchase, click "Claim License" below to link it.</p>
          <Link to="/claim" className="mt-4">
            <Button variant="outline">Link a Purchase</Button>
          </Link>
        </Empty>
      ) : (
        <div className="grid grid-cols-1 lg:grid-cols-3 gap-6">
          <div className="lg:col-span-2 space-y-4">
            <h2 className="text-sm font-semibold uppercase tracking-wider text-white/40">My Licenses</h2>
            {licenses.map((lic) => (
              <GlassCard key={lic.id} className={`p-5 transition-all ${selectedLicense?.id === lic.id ? "ring-1 ring-indigo-500/50" : ""}`}>
                <div className="flex justify-between items-start mb-3">
                  <div>
                    <div className="flex items-center gap-2">
                      <span className="text-sm font-bold text-white">License Key</span>
                      <Badge tone={lic.status === "active" ? "green" : "red"}>{lic.status}</Badge>
                      <Badge tone="violet">{lic.tier.toUpperCase()}</Badge>
                    </div>
                    <code className="text-xs select-all bg-black/40 px-2 py-1 rounded border border-white/5 text-indigo-300 font-mono mt-1.5 inline-block">
                      {lic.token}
                    </code>
                  </div>
                  <Button variant="subtle" onClick={() => viewDevices(lic)} className="text-xs">
                    <span>Manage Seats</span>
                  </Button>
                </div>
                <div className="flex gap-4 text-xs text-white/50 border-t border-white/5 pt-3">
                  <div>Product ID: <span className="text-white/80">#{lic.productId}</span></div>
                  <div>Provider: <span className="text-white/80">{lic.provider}</span></div>
                  <div>Expires: <span className="text-white/80">{lic.expiresAt ? lic.expiresAt.slice(0, 10) : "Never"}</span></div>
                </div>
              </GlassCard>
            ))}
          </div>

          <div>
            <h2 className="text-sm font-semibold uppercase tracking-wider text-white/40 mb-4">Device Seats</h2>
            {selectedLicense ? (
              <GlassCard className="p-4 space-y-4">
                <div>
                  <h3 className="text-sm font-medium text-white">Devices for #{selectedLicense.id}</h3>
                  <p className="text-[11px] text-indigo-300 select-all font-mono truncate">{selectedLicense.token}</p>
                </div>

                {loadingDevs ? (
                  <div className="text-center py-6 text-xs text-white/40">Loading devices...</div>
                ) : devices.length === 0 ? (
                  <p className="text-xs text-white/45 text-center py-6">No devices bound to this license seat yet.</p>
                ) : (
                  <div className="space-y-3 divide-y divide-white/5">
                    {devices.map((dev, idx) => (
                      <div key={dev.id} className={`flex justify-between items-center ${idx > 0 ? "pt-3" : ""}`}>
                        <div className="min-w-0 pr-2">
                          <p className="text-xs font-semibold text-white/90 truncate flex items-center gap-1.5">
                            <Laptop className="h-3.5 w-3.5 text-white/40" />
                            <span>{dev.name || "Unknown Machine"}</span>
                          </p>
                          <p className="text-[10px] text-white/40 mt-0.5 truncate">
                            IP: {dev.ip} · Active: {dev.lastSeenAt.slice(0, 10)}
                          </p>
                        </div>
                        <Button variant="danger" onClick={() => handleUnbind(dev.id)} className="text-[11px] px-2 py-1 h-6">
                          Unbind
                        </Button>
                      </div>
                    ))}
                  </div>
                )}
              </GlassCard>
            ) : (
              <GlassCard className="p-6 text-center text-xs text-white/40">
                Select a license key to view and manage bound active device seats.
              </GlassCard>
            )}
          </div>
        </div>
      )}
    </ScreenWrap>
  );
}
