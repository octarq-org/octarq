import { ReactNode, useState } from "react";

// Collapsible setup guidance panel. Moved here from the host app so a plugin
// package can ship the same affordance.
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
        className="flex w-full items-center justify-between px-3 py-2 text-left text-foreground/70 hover:bg-surface-hover"
        onClick={() => setShow((s) => !s)}
        aria-expanded={show}
      >
        <span className="flex items-center gap-2">
          <span>💡</span>
          {title}
        </span>
        <span className="text-muted-foreground" aria-hidden="true">{show ? "▾" : "▸"}</span>
      </button>
      {show && (
        <div className="space-y-2 border-t border-border px-3 py-3 text-muted-foreground animate-expand">
          {children}
        </div>
      )}
    </div>
  );
}
