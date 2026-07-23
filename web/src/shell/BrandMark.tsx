// BrandMark is the single brand glyph used across the shell (app loader, login
// card, top bar). It renders the operator's white-label logo when one is set
// (Pro white-label plugin), otherwise the gradient initial derived from the
// product name. The gradient reads the --gradient-primary design token, so the
// white-label accent colors recolor it automatically.
import { useAppName, brandInitial, useBrandLogo } from "../brand";

type Size = "sm" | "md" | "lg";

const BOX: Record<Size, string> = {
  sm: "h-9 w-9",
  md: "h-10 w-10",
  lg: "h-12 w-12",
};

const TEXT: Record<Size, string> = {
  sm: "text-sm",
  md: "text-base",
  lg: "text-xl",
};

export function BrandMark({ size = "sm", className = "" }: { size?: Size; className?: string }) {
  const appName = useAppName();
  const logoUrl = useBrandLogo();
  const box = BOX[size];

  if (logoUrl) {
    return (
      <img
        src={logoUrl}
        alt={appName}
        className={`${box} rounded-xl object-contain shadow-glow ${className}`}
      />
    );
  }

  return (
    <div
      className={`${box} brand-gradient flex items-center justify-center rounded-xl shadow-glow ${className}`}
    >
      <span className={`font-display ${TEXT[size]} font-extrabold text-white`}>{brandInitial(appName)}</span>
    </div>
  );
}
