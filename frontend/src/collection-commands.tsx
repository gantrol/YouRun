import { useEffect, useMemo, useState, type ComponentProps, type Ref } from "react";
import { Eye, EyeOff, Plus, Settings2 } from "lucide-react";
import { Button } from "@/renderer/components/ui/button";
import { DropdownMenu, DropdownMenuContent, DropdownMenuItem, DropdownMenuTrigger } from "@/renderer/components/ui/dropdown-menu";
import { CommandList } from "./command-list";
import { CollectionTree, type CollectionActions } from "./collection-tree";
import { collectionPath, commandContexts, commandSearchScore, rankByScore, searchTerms } from "./collections";
import { SearchBar } from "./search-bar";
import type { Collection, Project } from "./model";

export function CollectionCommands({ collection, collections, projects, actions, onAdd, onNew, searchRef, initialView, onViewChange, expanded, onExpanded, ...props }: ComponentProps<typeof CommandList> & {
  initialView?: { query: string; showHidden: boolean }; onViewChange?: (value: { query: string; showHidden: boolean }) => void;
  expanded?: Record<string, boolean>; onExpanded?: (value: Record<string, boolean>) => void;
  collection: Collection; collections: Collection[]; projects: Project[]; actions: CollectionActions;
  onAdd: () => void; onNew: () => void; searchRef?: Ref<HTMLInputElement>;
}) {
  const [query, setQuery] = useState(initialView?.query ?? ""), [showHidden, setShowHidden] = useState(initialView?.showHidden ?? false);
  const hasQuery = searchTerms(query).length > 0;
  useEffect(() => { onViewChange?.({ query, showHidden }); }, [query, showHidden, onViewChange]);
  useEffect(() => { if (props.revealId) { setQuery(""); setShowHidden(true); } }, [props.revealId]);
  const { visible, matchingCollections } = useMemo(() => {
    const byId = new Map(props.commands.map(c => [c.id, c]));
    const projectById = new Map(projects.map(p => [p.id, p]));
    const candidates = collection.commandIds.flatMap(id => {
      const c = byId.get(id), project = c?.project ? projectById.get(c.project) : undefined;
      return c && (c.id === props.revealId || showHidden || (!c.hidden && !project?.hidden)) ? [c] : [];
    });
    const ranked = rankByScore(candidates, query, c => {
      const score = commandSearchScore(c, query, props.m, c.project ? projectById.get(c.project)?.name : undefined);
      return c.id === props.revealId ? Math.max(1, score) : score;
    });
    const matchedCols = new Set<string>();
    if (hasQuery) {
      for (const col of collections) {
        if (col.commandIds.some(id => {
          const c = byId.get(id);
          const project = c?.project ? projectById.get(c.project) : undefined;
          return c && (showHidden || (!c.hidden && !project?.hidden)) && commandSearchScore(c, query, props.m, project?.name) > 0;
        })) {
          matchedCols.add(col.id);
        }
      }
    }
    return { visible: ranked, matchingCollections: matchedCols };
  }, [collection.commandIds, collections, props.commands, props.m, props.revealId, projects, query, hasQuery, showHidden]);
  const contexts = useMemo(() => commandContexts(visible, projects, props.m, query), [visible, projects, props.m, query]);
  const children = collections.some(c => c.parent === collection.id);
  return <section className="workspace-project flex min-h-0 min-w-0 flex-1 flex-col bg-surface" aria-label={collection.name}>
    <header className="flex h-14 shrink-0 items-center gap-1 border-b border-border px-3">
      <h2 className="min-w-0 flex-1 truncate font-semibold" title={collectionPath(collection, collections)}>{collection.name}</h2>
      <DropdownMenu><DropdownMenuTrigger asChild><Button variant="outline" size="icon-sm" aria-label={props.m.add}><Plus className="size-4"/></Button></DropdownMenuTrigger>
        <DropdownMenuContent align="end">
          <DropdownMenuItem onSelect={onAdd}>{props.m.addExistingCommands}</DropdownMenuItem>
          <DropdownMenuItem onSelect={onNew}>{props.m.newCommand}</DropdownMenuItem>
          <DropdownMenuItem onSelect={() => actions.create(collection.id)}>{props.m.newCollection}</DropdownMenuItem>
        </DropdownMenuContent>
      </DropdownMenu>
      <Button variant="ghost" size="icon-sm" title={props.m.hidden} aria-label={props.m.hidden} aria-pressed={showHidden} onClick={() => setShowHidden(value => !value)}>{showHidden ? <Eye className="size-4"/> : <EyeOff className="size-4"/>}</Button>
      <Button variant="ghost" size="icon-sm" aria-label={props.m.editCollection} title={props.m.editCollection} onClick={() => actions.edit(collection.id)}><Settings2 className="size-4"/></Button>
    </header>
    <CommandList {...props} commands={visible} contexts={contexts} query={query} onClearSearch={() => setQuery("")} header={children ? <>
      <CollectionTree collections={collections} parent={collection.id} m={props.m} actions={actions} query={query} matchingCollections={matchingCollections} heading={false} expanded={expanded} onExpanded={onExpanded}/>
      <h3 className="border-y border-border px-3 py-2 text-xs text-muted-foreground">{props.m.commandsList}</h3>
      {!visible.length && <div className="space-y-3 px-3 py-4">
        <p role="status" className="text-sm text-muted-foreground">{hasQuery ? props.m.emptyResults : props.m.noCommands}</p>
        {hasQuery && <Button variant="outline" size="sm" onClick={() => setQuery("")}>{props.m.clear}</Button>}
      </div>}
    </> : undefined}/>
    <SearchBar m={props.m} value={query} onChange={setQuery} inputRef={searchRef} count={hasQuery ? visible.length : undefined}/>
  </section>;
}

