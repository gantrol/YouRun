import type { ReactNode } from "react";

export function FormActions({ error, children, contained = false }: { error?: string; children: ReactNode; contained?: boolean }) {
  return (
    <div className={`${contained ? "shrink-0" : "sticky bottom-0 -mx-5 -mb-5"} z-10 space-y-2 border-t border-border bg-surface px-5 py-3`}>
      {error && <p role="alert" className="text-sm text-destructive">{error}</p>}
      <div className="flex flex-wrap items-center gap-2">{children}</div>
    </div>
  );
}
