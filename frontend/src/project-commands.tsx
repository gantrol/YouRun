import { useEffect, useMemo, useState, type ComponentProps, type Ref } from "react";
import { Plus, Settings2, Eye, EyeOff } from "lucide-react";
import { Button } from "@/renderer/components/ui/button";
import { CommandList } from "./command-list";
import { commandSearchScore, rankByScore, searchTerms } from "./collections";
import { type Project } from "./model";
import { SearchBar } from "./search-bar";

export function ProjectCommands({ project, onSettings, onAdd, searchRef, ...props }: ComponentProps<typeof CommandList> & {
  project: Project; onSettings: () => void; onAdd: () => void;
  searchRef?: Ref<HTMLInputElement>;
}) {
  const [showHidden, setShowHidden] = useState(false);
  const [query, setQuery] = useState("");
  const hasQuery = searchTerms(query).length > 0;
  useEffect(() => { if (props.revealId && props.commands.some(c => c.id === props.revealId && c.hidden)) setShowHidden(true); }, [props.revealId, props.commands]);
  useEffect(() => { if (props.revealId) setQuery(""); }, [props.revealId]);
  const visible = useMemo(() => {
    const candidates = props.commands.filter(c => c.project === project.id && (c.id === props.revealId || showHidden || !c.hidden));
    return rankByScore(candidates, query, c => {
      const score = commandSearchScore(c, query, props.m, project.name);
      return c.id === props.revealId ? Math.max(1, score) : score;
    });
  }, [props.commands, project.id, project.name, showHidden, props.revealId, props.m, query]);
  return <section className="workspace-project flex min-h-0 min-w-0 flex-1 flex-col bg-surface" aria-label={`${project.name} · ${props.m.commandsList}`}>
    <header className="flex h-14 shrink-0 items-center gap-2 border-b border-border px-4">
      <h2 className="min-w-0 flex-1 truncate text-base font-semibold" title={project.name}>{project.name}</h2>
      <Button variant="outline" size="sm" onClick={onAdd} data-navigation-key={`project:${project.id}:add`}><Plus className="size-4"/>{props.m.newCommand}</Button>
      <Button variant="ghost" size="icon-sm" aria-label={props.m.hidden} title={props.m.hidden} aria-pressed={showHidden} onClick={() => setShowHidden(value => !value)}>{showHidden ? <Eye className="size-4"/> : <EyeOff className="size-4"/>}</Button>
      <Button variant="ghost" size="icon-sm" title={props.m.projectSettings} aria-label={props.m.projectSettings} onClick={onSettings} data-navigation-key={`project:${project.id}:settings`}><Settings2 className="size-4"/></Button>
    </header>
    <CommandList {...props} commands={visible} query={query} onClearSearch={() => setQuery("")}/>
    <SearchBar m={props.m} value={query} onChange={setQuery} inputRef={searchRef} count={hasQuery ? visible.length : undefined}/>
  </section>;
}

