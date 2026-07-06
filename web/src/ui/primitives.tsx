import { ReactNode, useEffect, useState } from "react";
import { createPortal } from "react-dom";
import { twMerge } from "tailwind-merge";
import { motion } from "framer-motion";
import { HostEntry } from "../api";
import { useAppName } from "../brand";
import { useTranslation } from "../i18n";

export function GlassCard({
  className,
  children,
  strong,
}: {
  className?: string;
  children: ReactNode;
  strong?: boolean;
}) {
  return (
    <div className={twMerge(strong ? "glass-strong" : "glass", "rounded-2xl", className)}>
      {children}
    </div>
  );
}

// ─── Badge ───────────────────────────────────────────────────────────────────

type BadgeTone = "indigo" | "violet" | "green" | "amber" | "red" | "neutral" | "cyan";

const BADGE_TONE: Record<BadgeTone, string> = {
  indigo:  "bg-indigo-500/15 text-indigo-300 ring-indigo-400/20",
  violet:  "bg-violet-500/15 text-violet-300 ring-violet-400/20",
  green:   "bg-emerald-500/15 text-emerald-300 ring-emerald-400/20",
  amber:   "bg-amber-500/15  text-amber-300  ring-amber-400/20",
  red:     "bg-rose-500/15   text-rose-300   ring-rose-400/20",
  cyan:    "bg-cyan-500/15   text-cyan-300   ring-cyan-400/20",
  neutral: "bg-white/[0.08]  text-white/70   ring-white/10",
};

export function Badge({
  children,
  tone = "neutral",
  className,
}: {
  children: ReactNode;
  tone?: BadgeTone;
  className?: string;
}) {
  return (
    <span
      className={twMerge(
        "inline-flex items-center gap-1 rounded-full px-2 py-0.5 text-[11px] font-medium ring-1 ring-inset",
        BADGE_TONE[tone],
        className,
      )}
    >
      {children}
    </span>
  );
}

// ─── Button ──────────────────────────────────────────────────────────────────

type ButtonVariant = "primary" | "ghost" | "outline" | "subtle" | "danger";

const BTN_VARIANT: Record<ButtonVariant, string> = {
  primary: "bg-indigo-500 text-white hover:bg-indigo-400 shadow-[0_8px_30px_-8px_rgba(99,102,241,0.6)]",
  ghost:   "text-white/65 hover:text-white hover:bg-white/5",
  outline: "border border-white/10 text-white/80 hover:bg-white/5 hover:border-white/20",
  subtle:  "bg-white/5 text-white/80 hover:bg-white/10",
  danger:  "text-rose-300/90 hover:bg-rose-500/10 hover:text-rose-300",
};

export function Button({
  children,
  variant = "primary",
  className,
  ...props
}: React.ButtonHTMLAttributes<HTMLButtonElement> & { variant?: ButtonVariant }) {
  return (
    <button
      className={twMerge(
        "inline-flex items-center justify-center gap-2 rounded-xl px-3.5 py-2 text-sm font-medium transition-colors duration-150 focus:outline-none focus-visible:ring-2 focus-visible:ring-indigo-400/60 disabled:cursor-not-allowed disabled:opacity-50",
        BTN_VARIANT[variant],
        className,
      )}
      {...props}
    >
      {children}
    </button>
  );
}

// ─── ProPill ─────────────────────────────────────────────────────────────────

export function ProPill({ className, children }: { className?: string; children?: ReactNode }) {
  return (
    <span
      className={twMerge(
        "inline-flex items-center gap-1 rounded-full bg-gradient-to-r from-indigo-500/25 to-violet-500/25 px-2 py-0.5 text-[10px] font-semibold uppercase tracking-wide text-violet-200 ring-1 ring-inset ring-violet-400/30",
        className,
      )}
    >
      {children ?? "Pro"}
    </span>
  );
}

// ─── LockedFeature ────────────────────────────────────────────────────────────
// Unified upsell / degraded-state overlay for Pro-gated pages. The backend
// returns 402 when a feature exists but is unlicensed; any other failure means
// the plugin is absent or disabled in this build. One component renders both so
// every gated page (VPS, SSH, Inbox AI, …) speaks with one voice.

export const TIER_LABEL: Record<"pro" | "elite", string> = { pro: "Pro", elite: "Elite" };


export function StatCard({
  label,
  value,
  delta,
  positive = true,
  icon,
  index = 0,
  onClick,
}: {
  label: string;
  value: string | number;
  delta?: string;
  positive?: boolean;
  icon?: ReactNode;
  index?: number;
  onClick?: () => void;
}) {
  return (
    <motion.div
      initial={{ opacity: 0, y: 10 }}
      animate={{ opacity: 1, y: 0 }}
      transition={{ duration: 0.3, delay: index * 0.05 }}
      onClick={onClick}
      className={twMerge(
        "glass rounded-2xl p-4 text-left transition-all duration-150",
        onClick ? "cursor-pointer hover:bg-white/[0.06] active:scale-[0.98]" : ""
      )}
    >
      <div className="mb-2 flex items-center justify-between">
        <span className="text-[12px] font-medium text-white/45">{label}</span>
        {icon && <span className="text-white/40">{icon}</span>}
      </div>
      <div className="flex items-end gap-2">
        <span className="font-display text-2xl font-bold tracking-tight text-white">
          {value}
        </span>
        {delta && (
          <span
            className={`mb-1 text-[12px] font-medium ${positive ? "text-emerald-400" : "text-rose-400"}`}
          >
            {delta}
          </span>
        )}
      </div>
    </motion.div>
  );
}

// ─── PageHeader ───────────────────────────────────────────────────────────────

export function PageHeader({
  title,
  description,
  action,
}: {
  title: ReactNode;
  description?: string;
  action?: ReactNode;
}) {
  return (
    <div className="mb-6 flex flex-wrap items-start justify-between gap-4">
      <div>
        <h1 className="font-display text-2xl font-bold tracking-tight text-white">{title}</h1>
        {description && <p className="mt-1 text-sm text-white/50">{description}</p>}
      </div>
      {action}
    </div>
  );
}

// ─── ScreenWrap ───────────────────────────────────────────────────────────────

export function ScreenWrap({ children, className }: { children: ReactNode; className?: string }) {
  return (
    <motion.div
      className={className}
      initial={{ opacity: 0, y: 8 }}
      animate={{ opacity: 1, y: 0 }}
      transition={{ duration: 0.25, ease: "easeOut" }}
    >
      {children}
    </motion.div>
  );
}

// ─── Modal ───────────────────────────────────────────────────────────────────

export function Modal({
  title,
  onClose,
  children,
  wide,
}: {
  title: string;
  onClose: () => void;
  children: ReactNode;
  wide?: boolean;
}) {
  useEffect(() => {
    const h = (e: KeyboardEvent) => e.key === "Escape" && onClose();
    window.addEventListener("keydown", h);
    return () => window.removeEventListener("keydown", h);
  }, [onClose]);

  return createPortal(
    <div
      className="fixed inset-0 z-50 flex items-start justify-center overflow-y-auto p-4 pt-16 modal-overlay bg-black/60 backdrop-blur-sm"
      onClick={onClose}
    >
      <div
        className={`glass-strong w-full rounded-2xl p-5 modal-card relative z-50 ${wide ? "max-w-3xl" : "max-w-md"}`}
        onClick={(e) => e.stopPropagation()}
      >
        <div className="mb-4 flex items-center justify-between">
          <h2 className="font-display text-lg font-semibold text-white">{title}</h2>
          <button className="btn-ghost rounded-xl px-2 py-1 text-white/50 hover:text-white" onClick={onClose}>
            ✕
          </button>
        </div>
        {children}
      </div>
    </div>,
    document.body
  );
}

// ─── Field ───────────────────────────────────────────────────────────────────

export function Field({
  label,
  children,
  hint,
}: {
  label: string;
  children: ReactNode;
  hint?: string;
}) {
  return (
    <div className="mb-3">
      <label className="label">{label}</label>
      {children}
      {hint && <p className="mt-1 text-xs text-white/40">{hint}</p>}
    </div>
  );
}

// ─── Empty ───────────────────────────────────────────────────────────────────

export function Empty({ children }: { children: ReactNode }) {
  return (
    <div className="glass flex flex-col items-center justify-center gap-2 rounded-2xl py-16 text-white/45">
      {children}
    </div>
  );
}

// ─── Toggle ──────────────────────────────────────────────────────────────────

export function Toggle({ on, onChange }: { on: boolean; onChange: (v: boolean) => void }) {
  return (
    <button
      onClick={() => onChange(!on)}
      className={`relative h-5 w-9 rounded-full transition-all duration-300 ${on ? "bg-indigo-500" : "bg-white/15"}`}
    >
      <span
        className={`absolute top-0.5 h-4 w-4 rounded-full bg-white transition-all duration-300 shadow-sm ${
          on ? "left-4 scale-110" : "left-0.5 scale-90 opacity-70"
        }`}
      />
    </button>
  );
}

// ─── HostList ─────────────────────────────────────────────────────────────────


export function Guide({
  title,
  children,
  open = false,
}: {
  title: string;
  children: ReactNode;
  open?: boolean;
}) {
  const [show, setShow] = useState(open);
  return (
    <div className="glass mb-3 overflow-hidden rounded-2xl text-sm">
      <button
        className="flex w-full items-center justify-between px-3 py-2 text-left text-white/70 hover:bg-white/5"
        onClick={() => setShow((s) => !s)}
      >
        <span className="flex items-center gap-2">
          <span>💡</span>
          {title}
        </span>
        <span className="text-white/35">{show ? "▾" : "▸"}</span>
      </button>
      {show && (
        <div className="space-y-2 border-t border-white/[0.06] px-3 py-3 text-white/55 animate-expand">
          {children}
        </div>
      )}
    </div>
  );
}

// ─── Code ────────────────────────────────────────────────────────────────────

export function Code({ children }: { children: string }) {
  const [copied, setCopied] = useState(false);
  const { t } = useTranslation();
  return (
    <code
      className="cursor-pointer break-all rounded-lg bg-white/[0.06] px-1.5 py-0.5 font-mono text-[12px] text-indigo-200 hover:bg-white/10"
      title={t("uiCommon.clickToCopy")}
      onClick={() => {
        navigator.clipboard.writeText(children);
        setCopied(true);
        setTimeout(() => setCopied(false), 1000);
      }}
    >
      {copied ? t("uiCommon.copied") : children}
    </code>
  );
}

// ─── AreaChart ────────────────────────────────────────────────────────────────

