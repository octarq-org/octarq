// OctarqMark is the Octarq glyph itself: the Keystone Arch.
// An architectural arch structure (mining the "arq" / architecture root)
// enclosing a floating central keystone plugin module. Symbolizes self-hosted
// ownership, solid architectural foundations, and extensible modular plugins.
//
// It draws in `currentColor` with `fill-rule="evenodd"`, so it inherits whatever
// surface it sits on: white knocked out of the brand gradient in BrandMark,
// foreground text color in top bars or menu rows, or third-party plugin themes.
//
// The 16px favicon is a separate file (web/public/favicon.svg) with optically
// adjusted negative space for 16px rasterisation.

const ARCH_PATH =
  "M10,26 C10,13.8 19.8,4 32,4 C44.2,4 54,13.8 54,26 V56 H42 V28 C42,23.6 37.6,20 32,20 C26.4,20 22,23.6 22,28 V56 H10 V26 Z M26,33 H38 V46 H26 V33 Z";

export function OctarqMark({ className = "" }: { className?: string }) {
  return (
    <svg viewBox="0 0 64 64" fill="currentColor" aria-hidden="true" className={className}>
      <path fillRule="evenodd" clipRule="evenodd" d={ARCH_PATH} />
    </svg>
  );
}
