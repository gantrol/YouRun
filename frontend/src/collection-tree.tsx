import { useMemo, useState, type ReactNode } from "react";
import { ChevronRight, Folder, MoreHorizontal, Plus } from "lucide-react";
import { Button } from "@/renderer/components/ui/button";
import { DropdownMenu, DropdownMenuContent, DropdownMenuItem, DropdownMenuTrigger } from "@/renderer/components/ui/dropdown-menu";
import { HighlightText, collectionPath, collectionSearchScore, searchTerms } from "./collections";
import type { Collection } from "./model";
import type { Messages } from "./messages";

export type CollectionActions = {
  open: (id: string) => void;
  create: (parent?: string) => void;
  edit: (id: string) => void;
  remove: (id: string) => void;
  reorder: (id: string, direction: number) => void;
};

export function CollectionTree({ collections, m, actions, selectedId, query = "", matchingCollections, parent = null, heading = true, expanded, onExpanded }: {
  collections: Collection[]; m: Messages; actions: CollectionActions; selectedId?: string;
  expanded?: Record<string, boolean>; onExpanded?: (value: Record<string, boolean>) => void;
  query?: string; matchingCollections?: ReadonlySet<string>; parent?: string | null; heading?: boolean;
}) {
  const [collapsed, setCollapsed] = useState<Set<string>>(() => new Set());
  const hasQuery = searchTerms(query).length > 0;
  const matchingIds = useMemo(() => {
    if (!hasQuery) return new Set(collections.map(c => c.id));
    const result = new Set<string>(), byId = new Map(collections.map(c => [c.id, c]));
    for (const collection of collections) {
      const path = collectionPath(collection, collections);
      const matches = collectionSearchScore(collection, collections, query, path) > 0 || !!matchingCollections?.has(collection.id);
      if (!matches) continue;
      let item: Collection | undefined = collection;
      while (item && !result.has(item.id)) { result.add(item.id); item = item.parent ? byId.get(item.parent) : undefined; }
    }
    return result;
  }, [collections, hasQuery, matchingCollections, query]);
  function rows(parentId: string | null, depth: number): ReactNode {
    const siblings = collections.filter(c => c.parent === parentId);
    return siblings.map((c, index) => {
      const children = collections.some(item => item.parent === c.id);
      const path = collectionPath(c, collections);
      if (hasQuery && !matchingIds.has(c.id)) return null;
      const open = hasQuery || (expanded?.[`collection:${c.id}`] ?? !collapsed.has(c.id));
      return <li key={c.id}>
        <div className={`flex h-10 min-w-0 items-center gap-1 pr-2 ${selectedId === c.id ? "bg-selected" : "hover:bg-hover"}`} style={{ paddingLeft: 8 + Math.min(depth, 8) * 14 }}>
          {children ? <Button variant="ghost" size="icon-sm" aria-label={c.name} aria-expanded={open} onClick={() => onExpanded ? onExpanded({ ...expanded, [`collection:${c.id}`]: !open }) : setCollapsed(previous => {
            const next = new Set(previous); if (next.has(c.id)) next.delete(c.id); else next.add(c.id); return next;
          })}><ChevronRight className={`size-4 text-muted-foreground motion-safe:transition-transform motion-safe:duration-fast ${open ? "rotate-90" : ""}`} aria-hidden="true"/></Button> : <span className="w-8 shrink-0"/>}
          <Button variant="ghost" className="h-9 min-w-0 flex-1 justify-start gap-2 px-0 font-normal" title={path}
            data-collection-row={c.id} data-navigation-key={`collection:${c.id}`} aria-current={selectedId === c.id ? "page" : undefined} onClick={() => actions.open(c.id)}>
            <Folder className="size-4 shrink-0"/><span className="truncate"><HighlightText text={c.name} query={query} /></span>
          </Button>
          <span className="text-xs tabular-nums text-muted-foreground">{c.commandIds.length}</span>
          <DropdownMenu><DropdownMenuTrigger asChild><Button size="icon-sm" variant="ghost" aria-label={`${c.name} · ${m.settings}`}><MoreHorizontal className="size-4"/></Button></DropdownMenuTrigger>
            <DropdownMenuContent align="end">
              <DropdownMenuItem onSelect={() => actions.create(c.id)}>{m.newCollection}</DropdownMenuItem>
              <DropdownMenuItem onSelect={() => actions.edit(c.id)}>{m.editCollection}</DropdownMenuItem>
              <DropdownMenuItem disabled={index === 0} onSelect={() => actions.reorder(c.id, -1)}>{m.moveUp}</DropdownMenuItem>
              <DropdownMenuItem disabled={index === siblings.length - 1} onSelect={() => actions.reorder(c.id, 1)}>{m.moveDown}</DropdownMenuItem>
              <DropdownMenuItem className="text-destructive" onSelect={() => actions.remove(c.id)}>{m.deleteCollection}</DropdownMenuItem>
            </DropdownMenuContent>
          </DropdownMenu>
        </div>
        {children && open && <ul>{rows(c.id, depth + 1)}</ul>}
      </li>;
    });
  }
  if (hasQuery && !collections.some(c => c.parent === parent && matchingIds.has(c.id))) return null;
  return <section aria-label={m.collections} onKeyDown={event => {
    const target = event.target as HTMLElement;
    if (!target.hasAttribute("data-collection-row") || !["ArrowUp", "ArrowDown", "Home", "End"].includes(event.key)) return;
    const items = Array.from(event.currentTarget.querySelectorAll<HTMLElement>("[data-collection-row]"));
    const current = items.indexOf(target);
    const next = event.key === "Home" ? 0 : event.key === "End" ? items.length - 1 : Math.max(0, Math.min(items.length - 1, current + (event.key === "ArrowDown" ? 1 : -1)));
    event.preventDefault(); items[next]?.focus();
  }}>
    {heading && <header className="flex h-10 items-center gap-2 border-t border-border px-3">
      <h2 className="flex-1 text-xs text-muted-foreground">{m.collections}</h2>
      <Button variant="ghost" size="icon-sm" aria-label={m.newCollection} title={m.newCollection} onClick={() => actions.create(parent ?? undefined)}><Plus className="size-4"/></Button>
    </header>}
    <ul>{rows(parent, 0)}</ul>
  </section>;
}

