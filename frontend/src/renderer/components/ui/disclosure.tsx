import type { ReactNode } from "react";
import { Plus, Minus } from "lucide-react";
import { Button } from "./button";
import { Collapsible, CollapsibleContent, CollapsibleTrigger } from "./collapsible";
import { cn } from "@/renderer/lib/utils";
import { useViewToggle } from "./view-state";

export function Disclosure({ label, summary, children, defaultOpen = false, open, onOpenChange, stateKey, className }: {
  label: ReactNode;
  summary?: ReactNode;
  children: ReactNode;
  defaultOpen?: boolean;
  open?: boolean;
  onOpenChange?: (open: boolean) => void;
  stateKey?: string;
  className?: string;
}) {
  const [expanded, setExpanded] = useViewToggle(stateKey, defaultOpen);
  return (
    <Collapsible open={open ?? expanded} onOpenChange={(next) => { setExpanded(next); onOpenChange?.(next); }} className={cn("group/disclosure min-w-0", className)}>
      <CollapsibleTrigger asChild>
        <Button type="button" variant="ghost" size="sm" className="h-auto min-h-9 max-w-full justify-start gap-2 px-1 text-left whitespace-normal">
          {(open ?? expanded) ? <Minus className="size-4" aria-hidden="true"/> : <Plus className="size-4" aria-hidden="true"/>}
          {label}
        </Button>
      </CollapsibleTrigger>
      {summary && <div className="mt-2 group-data-[state=open]/disclosure:hidden">{summary}</div>}
      <CollapsibleContent forceMount className="data-[state=closed]:hidden"><div className="pt-3">{children}</div></CollapsibleContent>
    </Collapsible>
  );
}
