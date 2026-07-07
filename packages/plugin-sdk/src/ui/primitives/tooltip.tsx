import { ReactNode } from "react";
import { Tooltip as BaseTooltip } from "@base-ui/react/tooltip";
import { cn } from "../cn";

// Tooltip wraps Base UI's accessible Tooltip primitive (hover + focus triggered,
// portalled and positioned, dismissible) with led's glass theme. `children` is
// the trigger; `content` is the floating label. A per-instance Provider carries
// the open delay.
export function Tooltip({
  content,
  children,
  side = "top",
  delay = 200,
  className,
}: {
  content: ReactNode;
  children: ReactNode;
  side?: "top" | "bottom" | "left" | "right";
  delay?: number;
  className?: string;
}) {
  return (
    <BaseTooltip.Provider delay={delay}>
      <BaseTooltip.Root>
        <BaseTooltip.Trigger render={<span className="inline-flex" />}>
          {children}
        </BaseTooltip.Trigger>
        <BaseTooltip.Portal>
          <BaseTooltip.Positioner side={side} sideOffset={6} className="z-50 outline-none">
            <BaseTooltip.Popup
              className={cn(
                "glass-strong max-w-xs rounded-lg px-2.5 py-1.5 text-xs text-white/85 shadow-[0_12px_32px_-8px_rgba(0,0,0,0.6)]",
                "origin-[var(--transform-origin)]",
                className,
              )}
            >
              {content}
            </BaseTooltip.Popup>
          </BaseTooltip.Positioner>
        </BaseTooltip.Portal>
      </BaseTooltip.Root>
    </BaseTooltip.Provider>
  );
}
