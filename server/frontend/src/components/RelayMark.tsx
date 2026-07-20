// abacad relay mark — four connected dots (one hub → three devices) that read as
// an "A". The chrome (edges + outer dots) inherits `currentColor` so it adapts to
// the surrounding text color / theme; the hub is the `--success` green, the same
// token the UI uses for "connected / alive". Keep it in sync with assets/icon.svg.
export function RelayMark({
  className,
  title = "abacad",
}: {
  className?: string;
  title?: string;
}) {
  return (
    <svg
      viewBox="0 -14 512 512"
      className={className}
      role="img"
      aria-label={title}
      fill="none"
    >
      <g stroke="currentColor" strokeWidth={10} strokeLinecap="round" opacity={0.5}>
        <line x1="256" y1="262" x2="256" y2="116" />
        <line x1="256" y1="262" x2="108" y2="360" />
        <line x1="256" y1="262" x2="404" y2="360" />
      </g>
      <circle cx="256" cy="116" r="28" fill="currentColor" />
      <circle cx="108" cy="360" r="28" fill="currentColor" />
      <circle cx="404" cy="360" r="28" fill="currentColor" />
      <circle cx="256" cy="262" r="56" fill="var(--success)" opacity={0.16} />
      <circle cx="256" cy="262" r="34" fill="var(--success)" />
    </svg>
  );
}
