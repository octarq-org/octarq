import { forwardRef, type ReactNode } from "react";
import { motion } from "framer-motion";

// ScreenWrap is the page-level entrance-animation wrapper; every route mounts
// its content inside one for a consistent fade/slide-in.
export interface ScreenWrapProps {
  children: ReactNode;
  className?: string;
}

// The ref lands on the motion div itself — the element callers would measure or
// scroll — and framer-motion forwards refs through.
export const ScreenWrap = forwardRef<HTMLDivElement, ScreenWrapProps>(({ children, className }, ref) => (
  <motion.div
    ref={ref}
    className={className}
    initial={{ opacity: 0, y: 8 }}
    animate={{ opacity: 1, y: 0 }}
    transition={{ duration: 0.25, ease: "easeOut" }}
  >
    {children}
  </motion.div>
));
ScreenWrap.displayName = "ScreenWrap";
