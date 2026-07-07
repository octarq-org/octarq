import { ReactNode } from "react";

// Empty is the centered "nothing here yet" placeholder card.
export function Empty({ children }: { children: ReactNode }) {
  return (
    <div className="glass flex flex-col items-center justify-center gap-2 rounded-2xl py-16 text-white/45">
      {children}
    </div>
  );
}
