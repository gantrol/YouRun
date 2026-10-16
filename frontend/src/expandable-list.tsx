import { useId, type ReactNode } from "react";
import { Button } from "@/renderer/components/ui/button";
import type { Messages } from "./messages";
import { useViewToggle } from "@/renderer/components/ui/view-state";

export function ExpandableList<T>({ items, renderItem, m, className, limit = 8, stateKey }: {
  items: readonly T[];
  renderItem: (item: T) => ReactNode;
  m: Pick<Messages, "showAll" | "showLess">;
  className?: string;
  limit?: number;
  stateKey?: string;
}) {
  const [expanded, setExpanded] = useViewToggle(stateKey, false);
  const id = useId();
  return (
    <div>
      <ul id={id} className={className}>{(expanded ? items : items.slice(0, limit)).map(renderItem)}</ul>
      {items.length > limit && <Button type="button" variant="ghost" size="sm"
        aria-expanded={expanded} aria-controls={id}
        onClick={(event) => {
          // Keep the control in view when rows above it are removed.
          const button = event.currentTarget;
          setExpanded(!expanded);
          if (expanded) requestAnimationFrame(() => button.scrollIntoView({ block: "nearest" }));
        }}>
        {expanded ? m.showLess : `${m.showAll} · ${new Intl.NumberFormat(document.documentElement.lang).format(items.length)}`}
      </Button>}
    </div>
  );
}
