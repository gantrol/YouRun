import { useMemo, useRef, useState } from "react";
import { Button } from "@/renderer/components/ui/button";
import { Checkbox } from "@/renderer/components/ui/checkbox";
import { Dialog, DialogContent, DialogTitle } from "@/renderer/components/ui/dialog";
import { Input } from "@/renderer/components/ui/input";
import {
  HighlightText,
  collectionPath,
  collectionSearchScore,
  commandSearchScore,
  rankByScore,
  searchTerms,
} from "./collections";
import { commandTitle, type Collection, type Command, type Project } from "./model";
import type { Messages } from "./messages";

export type CollectionDialogTarget = { kind: "collections" | "commands"; id: string };
export function CollectionDialog({ target, collections, commands, projects, m, onSave, onClose, onCreate }: {
  target: CollectionDialogTarget; collections: Collection[]; commands: Command[]; projects: Project[]; m: Messages;
  onSave: (commandIds: string[], collectionIds: string[]) => Promise<void>; onClose: () => void; onCreate: () => void;
}) {
  const [query, setQuery] = useState(""), [selected, setSelected] = useState<Set<string>>(() => new Set());
  const [busy, setBusy] = useState(false), [error, setError] = useState("");
  const submitting = useRef(false);
  const toCollections = target.kind === "collections";
  const collection = collections.find(c => c.id === target.id);
  const missing = toCollections ? !commands.some(c => c.id === target.id) : !collection;
  const hasQuery = searchTerms(query).length > 0;
  const { allOptions, visibleOptions } = useMemo(() => {
    const projectById = new Map(projects.map(p => [p.id, p]));
    const all = toCollections ? collections.map(c => {
      const title = collectionPath(c, collections);
      return {
        id: c.id, title, context: "", added: c.commandIds.includes(target.id),
        score: collectionSearchScore(c, collections, query, title),
      };
    }) : commands.map(c => {
      const projectName = c.project ? projectById.get(c.project)?.name : undefined;
      return {
        id: c.id, title: commandTitle(c, m), context: [projectName, ...(c.aliases ?? []), c.directory || c.source].filter(Boolean).join(" · "),
        added: !!collection?.commandIds.includes(c.id), score: commandSearchScore(c, query, m, projectName),
      };
    });
    return {
      allOptions: all,
      visibleOptions: rankByScore(all, query, item => item.score),
    };
  }, [toCollections, target.id, collections, commands, projects, collection, m, query]);
  const available = allOptions.filter(item => selected.has(item.id) && !item.added).map(item => item.id);
  return <Dialog open onOpenChange={open => { if (!open && !submitting.current) onClose(); }}>
    <DialogContent aria-describedby={undefined} className="flex max-h-[min(38rem,calc(100dvh-2rem))] flex-col" onEscapeKeyDown={event => { if (busy) event.preventDefault(); }}
      onPointerDownOutside={event => { if (busy) event.preventDefault(); }}>
      <DialogTitle className="font-semibold">{toCollections ? m.addToCollections : m.addExistingCommands}</DialogTitle>
      <Input autoFocus type="search" aria-label={m.searchCommands} placeholder={m.searchCommands} value={query}
        onChange={event => setQuery(event.target.value)}
        onKeyDown={event => {
          if (!event.nativeEvent.isComposing && event.key === "Escape" && query) {
            event.preventDefault(); event.stopPropagation(); setQuery("");
          }
        }}
        disabled={busy}/>
      <div className="min-h-0 flex-1 overflow-auto overscroll-contain">
        {visibleOptions.map(item => <label key={item.id} className="flex min-h-12 min-w-0 items-center gap-3 py-2" title={[item.title, item.context].filter(Boolean).join(" · ")}>
          <Checkbox checked={item.added || selected.has(item.id)} disabled={busy || item.added || missing} onCheckedChange={checked => setSelected(previous => {
            const next = new Set(previous); if (checked === true) next.add(item.id); else next.delete(item.id); return next;
          })}/>
          <span className="min-w-0 flex-1">
            <span className="block truncate text-sm"><HighlightText text={item.title} query={query} /></span>
            {item.context && <span className="block truncate text-xs text-muted-foreground"><HighlightText text={item.context} query={query} /></span>}
          </span>
          {item.added && <span className="shrink-0 text-xs text-muted-foreground">{m.alreadyInCollection}</span>}
        </label>)}
        {!visibleOptions.length && <div className="space-y-2 py-4">
          <p role="status" className="text-sm text-muted-foreground">{hasQuery ? m.emptyResults : toCollections ? m.noCollections : m.noCommands}</p>
          {hasQuery && <Button variant="outline" size="sm" onClick={() => setQuery("")}>{m.clear}</Button>}
        </div>}
      </div>
      {(error || missing) && <p role="alert" className="text-sm text-destructive">{missing ? toCollections ? m.commandMissing : m.collectionMissing : m[error as keyof Messages] ?? m.storageFailed}</p>}
      <div className="flex flex-wrap gap-2">
        {toCollections && <Button variant="ghost" size="sm" disabled={busy} onClick={onCreate}>{m.newCollection}</Button>}
        <Button className="ml-auto" size="sm" disabled={busy || missing || !available.length} onClick={async () => {
          if (submitting.current) return;
          submitting.current = true; setBusy(true); setError("");
          try { await onSave(toCollections ? [target.id] : available, toCollections ? available : [target.id]); onClose(); }
          catch (cause) { setError(cause instanceof Error ? cause.message : "storageFailed"); }
          finally { submitting.current = false; setBusy(false); }
        }}>{busy ? m.saving : m.add}</Button>
        <Button variant="outline" size="sm" disabled={busy} onClick={onClose}>{m.cancel}</Button>
      </div>
    </DialogContent>
  </Dialog>;
}

