/**
 * abacad's mark, inline: two panes, 45° apart, 5 of air between them.
 *
 * The same geometry as brand/mark.svg — public/favicon.svg is the small-size
 * cut of it (air widened to 8 so the gap survives at 16px). Inline rather than
 * an <img> of the favicon so the chrome costs no extra request and the mark is
 * painted with the first frame. It keeps its own two colours in both themes,
 * the way the tab icon does: a mark tinted to the foreground would stop being
 * the mark.
 *
 * The coral L is one path with no mask: the outer 62-square (r 15) cut by the
 * front pane's silhouette grown by the air (25, r 20).
 */
export function BrandMark({ className }: { className?: string }) {
  return (
    <svg viewBox="0 0 100 100" className={className} aria-hidden="true">
      <path
        fill="#fd605e"
        d="M23 8H55A15 15 0 0 1 70 23V25H45A20 20 0 0 0 25 45V70H23A15 15 0 0 1 8 55V23A15 15 0 0 1 23 8Z"
      />
      <rect x="30" y="30" width="62" height="62" rx="15" fill="#13a7f5" />
    </svg>
  )
}
