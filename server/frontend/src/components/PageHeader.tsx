import { type ReactNode } from "react";

// One page header for every top-level route so the content-top is pixel-identical
// as you navigate — eyebrow, title, and description share the same rhythm; the
// optional actions slot sits on the right (wraps below the copy on narrow screens).
export function PageHeader({
  eyebrow,
  title,
  description,
  actions,
}: {
  eyebrow: string;
  title: string;
  description: string;
  actions?: ReactNode;
}) {
  return (
    <header className="mb-7 flex flex-col gap-5 sm:flex-row sm:items-end sm:justify-between">
      <div className="min-w-0">
        <p className="font-mono text-[11px] font-medium uppercase tracking-[0.22em] text-brand">{eyebrow}</p>
        <h1 className="mt-3 font-display text-3xl font-bold leading-tight text-ink sm:text-4xl">{title}</h1>
        <p className="mt-2 max-w-2xl text-sm leading-6 text-ink-muted">{description}</p>
      </div>
      {actions && <div className="flex shrink-0 items-center gap-2">{actions}</div>}
    </header>
  );
}
