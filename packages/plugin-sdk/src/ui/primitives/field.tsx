import { ReactNode } from "react";

// Field is the label/control/hint stack used by every form row. `label` uses
// the app's `.label` theme class (defined in styles.css).
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
