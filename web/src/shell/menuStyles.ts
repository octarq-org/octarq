// Shared Base UI Menu popover styling. TopBar and AreaPanel each carried a
// verbatim copy of these two strings (B5). TopBar consumes this module now;
// AreaPanel still holds its copy and is migrated in the batch that owns that
// file — until then the duplication is halved, not gone.
//
// `rounded-2xl` / `rounded-xl` here are not stale: the radius ramp in
// styles.css maps every step to 4px, so the class names stay while the value
// is already flat.

export const MENU_POPUP =
  "glass-strong z-50 origin-[var(--transform-origin)] rounded-2xl p-1.5 outline-none " +
  "transition-[transform,opacity] duration-150 data-[starting-style]:scale-95 data-[starting-style]:opacity-0 data-[ending-style]:scale-95 data-[ending-style]:opacity-0";

export const MENU_ITEM =
  "flex w-full cursor-pointer items-center gap-2.5 rounded-xl px-2 py-2 text-left text-sm text-foreground/80 outline-none transition-colors data-[highlighted]:bg-surface-hover data-[highlighted]:text-foreground";
