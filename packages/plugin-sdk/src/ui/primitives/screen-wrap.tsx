import { forwardRef, type ReactNode } from "react";
import { motion } from "framer-motion";

export interface ScreenWrapProps {
  children: ReactNode;
  className?: string;
}

export const ScreenWrap = forwardRef<HTMLDivElement, ScreenWrapProps>(({ children, className }, ref) => (
  <motion.div
    ref={ref}
    className={className}
    initial={{ opacity: 0 }}
    animate={{ opacity: 1 }}
    transition={{ duration: 0.15, ease: "easeOut" }}
  >
    {children}
  </motion.div>
));
ScreenWrap.displayName = "ScreenWrap";
