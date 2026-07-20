import { type ReactNode } from "react";

// One page header for every top-level route so the content-top is pixel-identical
// as you navigate — just the title, with an optional actions slot on the right
// (wraps below the title on narrow screens).
export function PageHeader({
  title,
  actions,
}: {
  title: string;
  actions?: ReactNode;
}) {
  return (
    <header className="mb-7 flex flex-col gap-5 sm:flex-row sm:items-end sm:justify-between">
      <div className="min-w-0">
        <h1 className="font-display text-3xl font-bold leading-tight text-ink sm:text-4xl">{title}</h1>
      </div>
      {actions && <div className="flex shrink-0 items-center gap-2">{actions}</div>}
    </header>
  );
}
