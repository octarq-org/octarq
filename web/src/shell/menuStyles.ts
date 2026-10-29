// Shared Base UI Menu popover styling. TopBar and AreaPanel each carried a
// verbatim copy of these two strings (B5); both consume this module now.
//
// The radius classes are semantic steps on the host's ramp (styles.css): the
// popup is the 18px floating radius, items sit at 12px. Keep them in step with
// Dialog, which is the other floating layer.

export const MENU_POPUP =
  "glass-strong z-50 origin-[var(--transform-origin)] rounded-2xl p-1.5 outline-none " +
  "transition-[transform,opacity] duration-[var(--dur-fast)] ease-[var(--ease-out)] data-[starting-style]:scale-95 data-[starting-style]:opacity-0 data-[ending-style]:scale-95 data-[ending-style]:opacity-0";

export const MENU_ITEM =
  "flex w-full cursor-pointer items-center gap-2.5 rounded-lg px-2.5 py-2 text-left text-[13px] text-foreground/80 outline-none transition-colors duration-[var(--dur-fast)] data-[highlighted]:bg-surface-hover data-[highlighted]:text-foreground";
