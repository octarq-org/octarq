import { ReactNode } from "react";
import { motion } from "framer-motion";

// ScreenWrap is the page-level entrance-animation wrapper; every route mounts
// its content inside one for a consistent fade/slide-in.
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
