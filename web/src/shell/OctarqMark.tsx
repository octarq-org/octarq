// OctarqMark is the Octarq glyph itself: an octagonal ring — octa, eight sides,
// eight plugins composing one system — broken at the lower right by a tail that
// turns the ring into a q. Both halves of the name in one geometric mark.
//
// It draws in `currentColor` with no fills, so it inherits whatever surface it
// sits on: white knocked out of the brand gradient in BrandMark, foreground
// color in a menu row, whatever a plugin gives it. That is what makes it safe
// to reuse at 20px without a second asset.
//
// The 16px favicon is a separate file (web/public/favicon.svg) with a thicker
// stroke — this one scaled down loses the ring to rasterisation.

// Geometry: regular octagon, circumradius 20, centered at (32,32); vertices at
// 22.5° + 45°k. The tail leaves the midpoint of the lower-right edge along 45°.
const RING =
  "M50.48 39.65 39.65 50.48H24.35L13.52 39.65V24.35L24.35 13.52h15.3L50.48 24.35Z";
const TAIL = "M45.06 45.06 54 54";

export function OctarqMark({
  className = "",
  strokeWidth = 7.5,
}: {
  className?: string;
  strokeWidth?: number;
}) {
  return (
    <svg viewBox="0 0 64 64" fill="none" aria-hidden="true" className={className}>
      <path
        d={RING}
        stroke="currentColor"
        strokeWidth={strokeWidth}
        strokeLinejoin="round"
      />
      <path
        d={TAIL}
        stroke="currentColor"
        strokeWidth={strokeWidth}
        strokeLinecap="round"
      />
    </svg>
  );
}
