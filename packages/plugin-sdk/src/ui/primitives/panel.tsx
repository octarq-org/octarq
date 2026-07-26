import { ReactNode } from "react";
import { GlassCard } from "./glass-card";

export function Panel({ title, children }: { title: ReactNode; children: ReactNode }) {
  return (
    <GlassCard className="p-5">
      <h3 className="mb-3 text-[11px] font-semibold uppercase tracking-wider text-foreground/50">{title}</h3>
      {children}
    </GlassCard>
  );
}
