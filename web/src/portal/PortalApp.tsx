import { useEffect, useState } from "react";
import { Routes, Route, Link, useNavigate, useSearchParams } from "react-router-dom";
import { api, ApiError, IssuedLicense, LicenseDevice } from "../api";
import { ScreenWrap, PageHeader, GlassCard, Badge, Button, Empty } from "../ui";
import { KeyRound, LogOut, Laptop, ExternalLink, ShieldAlert, ArrowRight, CheckCircle, ArrowLeft, Mail, Lock } from "lucide-react";

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
        <Route path="/forgot-password" element={<ForgotPasswordView />} />
        <Route path="/reset" element={<ResetPasswordView />} />
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
    <ScreenWrap className="flex items-center justify-center min-h-[85vh] p-4">
      <GlassCard className="w-full max-w-md p-8 relative overflow-hidden shadow-glow">
        <div className="absolute top-0 right-0 h-32 w-32 bg-indigo-500/5 blur-3xl rounded-full pointer-events-none" />
        <div className="absolute -bottom-10 -left-10 h-32 w-32 bg-violet-500/5 blur-3xl rounded-full pointer-events-none" />

        <div className="text-center mb-6">
          <div className="mx-auto mb-4 flex h-12 w-12 items-center justify-center rounded-xl bg-gradient-to-br from-indigo-500 to-violet-500 shadow-glow">
            <span className="font-display text-xl font-extrabold text-white">L</span>
          </div>
          <h2 className="text-2xl font-bold text-white">Customer Portal</h2>
          <p className="text-xs text-white/45 mt-1.5 leading-relaxed">Manage your licenses, devices, and billing info</p>
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
            <div className="flex justify-between items-center mb-1">
              <label className="block text-xs font-medium text-white/60">Password</label>
              <Link to="/forgot-password" className="text-xs text-indigo-400 hover:text-indigo-300 font-medium">
                Forgot password?
              </Link>
            </div>
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

        <div className="mt-6 text-center text-xs text-white/40 flex flex-col gap-2">
          <div>
            Don't have a portal account yet?{" "}
            <Link to="/register" className="text-indigo-400 hover:text-indigo-300 font-medium">
              Register here
            </Link>
          </div>
          <div className="text-[11px] text-white/30 border-t border-white/5 pt-3 mt-1">
            Need to link a Stripe checkout?{" "}
            <Link to="/claim" className="text-indigo-400 hover:text-indigo-300 font-medium">
              Claim Purchase
            </Link>
          </div>
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
    <ScreenWrap className="flex items-center justify-center min-h-[85vh] p-4">
      <GlassCard className="w-full max-w-md p-8 relative overflow-hidden shadow-glow">
        <div className="absolute top-0 right-0 h-32 w-32 bg-indigo-500/5 blur-3xl rounded-full pointer-events-none" />
        <div className="absolute -bottom-10 -left-10 h-32 w-32 bg-violet-500/5 blur-3xl rounded-full pointer-events-none" />

        <div className="text-center mb-6">
          <div className="mx-auto mb-4 flex h-12 w-12 items-center justify-center rounded-xl bg-gradient-to-br from-indigo-500 to-violet-500 shadow-glow">
            <span className="font-display text-xl font-extrabold text-white">L</span>
          </div>
          <h2 className="text-2xl font-bold text-white">Create Portal Account</h2>
          <p className="text-xs text-white/45 mt-1.5 leading-relaxed">Register to manage your purchases and licenses</p>
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
        setError("An account already exists for this purchase. Please sign in to view your licenses.");
      } else {
        setError(err.error || "Could not claim purchase. Ensure the Session ID is correct.");
      }
    } finally {
      setSubmitting(false);
    }
  };

  return (
    <ScreenWrap className="flex items-center justify-center min-h-[85vh] p-4">
      <GlassCard className="w-full max-w-md p-8 relative overflow-hidden shadow-glow">
        <div className="absolute top-0 right-0 h-32 w-32 bg-indigo-500/5 blur-3xl rounded-full pointer-events-none" />
        <div className="absolute -bottom-10 -left-10 h-32 w-32 bg-violet-500/5 blur-3xl rounded-full pointer-events-none" />

        <div className="text-center mb-6">
          <div className="mx-auto mb-4 flex h-12 w-12 items-center justify-center rounded-xl bg-gradient-to-br from-emerald-500/10 to-teal-500/10 shadow-glow border border-emerald-500/20 text-emerald-400">
            <CheckCircle className="h-6 w-6" />
          </div>
          <h2 className="text-2xl font-bold text-white">Claim Your Purchase</h2>
          <p className="text-xs text-white/45 mt-1.5 leading-relaxed">Set a password to link and activate your portal account</p>
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
              <label className="block text-xs font-medium text-white/60 mb-1">Set Portal Password (min 8 chars)</label>
              <input
                type="password"
                required
                value={password}
                onChange={(e) => setPassword(e.target.value)}
                className="w-full rounded-xl bg-white/5 border border-white/10 px-3.5 py-2.5 text-sm text-white focus:outline-none focus:border-indigo-500"
                placeholder="Choose a password"
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

// ─── FORGOT PASSWORD VIEW ───────────────────────────────────────────────────
function ForgotPasswordView() {
  const [email, setEmail] = useState("");
  const [error, setError] = useState("");
  const [successMsg, setSuccessMsg] = useState("");
  const [submitting, setSubmitting] = useState(false);

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault();
    setError("");
    setSuccessMsg("");
    setSubmitting(true);
    try {
      const res = await api.customerForgotPassword(email);
      setSuccessMsg(res.message || "If an account exists for that email, a reset link has been sent.");
    } catch (err: any) {
      setError(err.error || "Could not request password reset. Please try again later.");
    } finally {
      setSubmitting(false);
    }
  };

  return (
    <ScreenWrap className="flex items-center justify-center min-h-[85vh] p-4">
      <GlassCard className="w-full max-w-md p-8 relative overflow-hidden shadow-glow">
        <div className="absolute top-0 right-0 h-32 w-32 bg-indigo-500/5 blur-3xl rounded-full pointer-events-none" />
        <div className="absolute -bottom-10 -left-10 h-32 w-32 bg-violet-500/5 blur-3xl rounded-full pointer-events-none" />

        <div className="mb-6 text-center">
          <KeyRound className="mx-auto h-12 w-12 text-indigo-400 mb-2" />
          <h2 className="text-2xl font-bold text-white">Reset Password</h2>
          <p className="text-xs text-white/45 mt-1">We'll send you an email with instructions to reset your password</p>
        </div>

        {error && (
          <div className="mb-4 p-3 rounded-xl bg-rose-500/10 border border-rose-500/20 text-rose-300 text-xs flex gap-2 items-center">
            <ShieldAlert className="h-4 w-4 shrink-0" />
            <span>{error}</span>
          </div>
        )}

        {successMsg ? (
          <div className="space-y-4 text-center">
            <div className="p-4 rounded-xl bg-emerald-500/10 border border-emerald-500/20 text-emerald-300 text-sm">
              {successMsg}
            </div>
            <Link to="/login" className="inline-flex items-center gap-1.5 text-xs text-indigo-400 hover:text-indigo-300 font-medium">
              <ArrowLeft className="h-3.5 w-3.5" />
              <span>Back to Sign In</span>
            </Link>
          </div>
        ) : (
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
            <Button type="submit" disabled={submitting} className="w-full py-2.5 mt-2">
              {submitting ? "Sending Reset Link..." : "Send Reset Link"}
            </Button>
          </form>
        )}

        {!successMsg && (
          <div className="mt-6 text-center text-xs text-white/40">
            Remembered your password?{" "}
            <Link to="/login" className="text-indigo-400 hover:text-indigo-300 font-medium inline-flex items-center gap-1">
              <span>Sign In</span>
            </Link>
          </div>
        )}
      </GlassCard>
    </ScreenWrap>
  );
}

// ─── RESET PASSWORD VIEW ─────────────────────────────────────────────────────
function ResetPasswordView() {
  const [searchParams] = useSearchParams();
  const token = searchParams.get("token") || "";
  const [password, setPassword] = useState("");
  const [confirmPassword, setConfirmPassword] = useState("");
  const [error, setError] = useState("");
  const [success, setSuccess] = useState(false);
  const [submitting, setSubmitting] = useState(false);
  const navigate = useNavigate();

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault();
    setError("");

    if (!token) {
      setError("Reset token is missing from the URL.");
      return;
    }

    if (password.length < 8) {
      setError("Password must be at least 8 characters long.");
      return;
    }

    if (password !== confirmPassword) {
      setError("Passwords do not match.");
      return;
    }

    setSubmitting(true);
    try {
      await api.customerResetPassword(token, password);
      setSuccess(true);
      setTimeout(() => {
        navigate("/login", { replace: true });
      }, 2000);
    } catch (err: any) {
      setError(err.error || "Failed to reset password. The link may have expired or is invalid.");
    } finally {
      setSubmitting(false);
    }
  };

  return (
    <ScreenWrap className="flex items-center justify-center min-h-[85vh] p-4">
      <GlassCard className="w-full max-w-md p-8 relative overflow-hidden shadow-glow">
        <div className="absolute top-0 right-0 h-32 w-32 bg-indigo-500/5 blur-3xl rounded-full pointer-events-none" />
        <div className="absolute -bottom-10 -left-10 h-32 w-32 bg-violet-500/5 blur-3xl rounded-full pointer-events-none" />

        <div className="mb-6 text-center">
          <KeyRound className="mx-auto h-12 w-12 text-indigo-400 mb-2" />
          <h2 className="text-2xl font-bold text-white">Create New Password</h2>
          <p className="text-xs text-white/45 mt-1">Please enter your new password below</p>
        </div>

        {error && (
          <div className="mb-4 p-3 rounded-xl bg-rose-500/10 border border-rose-500/20 text-rose-300 text-xs flex gap-2 items-center">
            <ShieldAlert className="h-4 w-4 shrink-0" />
            <span>{error}</span>
          </div>
        )}

        {success ? (
          <div className="p-4 rounded-xl bg-emerald-500/10 border border-emerald-500/20 text-emerald-300 text-sm text-center">
            Password successfully reset! Redirecting to login...
          </div>
        ) : (
          <form onSubmit={handleSubmit} className="space-y-4">
            {!token && (
              <div className="p-3 bg-rose-500/10 border border-rose-500/20 text-rose-300 text-xs rounded-xl">
                Warning: No token found in URL. You will not be able to reset your password.
              </div>
            )}
            <div>
              <label className="block text-xs font-medium text-white/60 mb-1">New Password (min 8 chars)</label>
              <input
                type="password"
                required
                value={password}
                onChange={(e) => setPassword(e.target.value)}
                className="w-full rounded-xl bg-white/5 border border-white/10 px-3.5 py-2.5 text-sm text-white focus:outline-none focus:border-indigo-500"
                placeholder="••••••••"
                disabled={!token}
              />
            </div>
            <div>
              <label className="block text-xs font-medium text-white/60 mb-1">Confirm New Password</label>
              <input
                type="password"
                required
                value={confirmPassword}
                onChange={(e) => setConfirmPassword(e.target.value)}
                className="w-full rounded-xl bg-white/5 border border-white/10 px-3.5 py-2.5 text-sm text-white focus:outline-none focus:border-indigo-500"
                placeholder="••••••••"
                disabled={!token}
              />
            </div>
            <Button type="submit" disabled={submitting || !token} className="w-full py-2.5 mt-2">
              {submitting ? "Resetting Password..." : "Reset Password"}
            </Button>
          </form>
        )}

        <div className="mt-6 text-center text-xs text-white/40">
          <Link to="/login" className="text-indigo-400 hover:text-indigo-300 font-medium inline-flex items-center gap-1.5">
            <ArrowLeft className="h-3.5 w-3.5" />
            <span>Back to Sign In</span>
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
                            Last active: {dev.lastSeenAt.slice(0, 10)}
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
