import { useMemo, useRef, useState } from "react";
import { defaultRangeExtractor } from "@tanstack/react-virtual";
import { Minus, Plus, Search, X } from "lucide-react";
import { usePageVirtualizer } from "./page-virtualizer";
import { useViewPosition, useViewToggle } from "@/renderer/components/ui/view-state";
import { Button } from "@/renderer/components/ui/button";
import { Input } from "@/renderer/components/ui/input";
import { Picker } from "./controls";
import { HighlightText, scoreMultiTerm, searchTerms } from "./collections";
import { RunStatus } from "./run-state";
import type { Messages } from "./messages";
import type { Command, Project, Run } from "./model";

export type HistoryFilters = { query: string; project: string };
type Section = "activeRuns" | "completedRuns" | "unconfirmedRuns" | "externalRuns";

const sections: {
  label: Section;
  states: Run["status"][];
}[] = [
  { label: "activeRuns", states: ["running", "stopping"] },
  { label: "unconfirmedRuns", states: ["unknown"] },
  { label: "externalRuns", states: ["handedOff"] },
  { label: "completedRuns", states: ["success", "failed", "stopped"] },
];

function startedAt(run: Run) {
  const time = Date.parse(run.time);
  return Number.isFinite(time) ? time : 0;
}

export function formatRunTime(time: string, locale: string) {
  const date = new Date(time);
  if (!Number.isFinite(date.getTime())) return time;
  return new Intl.DateTimeFormat(locale, {
    dateStyle: "short",
    timeStyle: "short",
  }).format(date);
}

export function RunHistory({
  runs,
  commands = [],
  projects = [],
  filters: controlledFilters,
  onFiltersChange,
  m,
  locale,
  onOpen,
}: {
  runs: readonly Run[];
  commands?: readonly Command[];
  projects?: readonly Project[];
  filters?: HistoryFilters;
  onFiltersChange?: (filters: HistoryFilters) => void;
  m: Messages;
  locale: string;
  onOpen: (id: string) => void;
}) {
  const container = useRef<HTMLDivElement>(null);
  const search = useRef<HTMLInputElement>(null);
  const position = useViewPosition("history-scroll");
  const [focused, setFocused] = useState<string>();
  const [localFilters, setLocalFilters] = useState<HistoryFilters>({ query: "", project: "all" });
  const filters = controlledFilters ?? localFilters;
  const [activeOpen, setActiveOpen] = useViewToggle("history:active", false);
  const [unknownOpen, setUnknownOpen] = useViewToggle("history:unknown", false);
  const [externalOpen, setExternalOpen] = useViewToggle("history:external", false);
  const [completedOpen, setCompletedOpen] = useViewToggle("history:completed", false);
  const expanded = { activeRuns: activeOpen, unconfirmedRuns: unknownOpen, externalRuns: externalOpen, completedRuns: completedOpen };
  const setExpanded = { activeRuns: setActiveOpen, unconfirmedRuns: setUnknownOpen, externalRuns: setExternalOpen, completedRuns: setCompletedOpen };
  const associations = useMemo(() => {
    const byCommand = new Map(commands.map(command => [command.id, command]));
    const byProject = new Map(projects.map(project => [project.id, project]));
    return new Map(runs.map(run => {
      const command = byCommand.get(run.commandId);
      const project = command?.project ? byProject.get(command.project) : undefined;
      return [run.id, project ? { key: `project:${project.id}`, name: project.name, command }
        : command && !command.project ? { key: "independent", name: m.independentCommands, command }
          : { key: "unknown", name: m.unknownProject, command }];
    }));
  }, [runs, commands, projects, m]);
  const projectOptions: [string, string][] = [
    ["all", m.allProjects], ["independent", m.independentCommands],
    ...projects.map(project => [`project:${project.id}`, project.name] as [string, string]),
    ["unknown", m.unknownProject],
  ];
  // Removing a project must not silently turn its selected filter into "all".
  if (!projectOptions.some(([key]) => key === filters.project)) projectOptions.push([filters.project, m.projectMissing]);
  const filtered = !!filters.query.trim() || filters.project !== "all";
  const matching = useMemo(() => {
    const hasQuery = searchTerms(filters.query, locale).length > 0;
    return runs.filter(run => {
      const assoc = associations.get(run.id)!;
      if (filters.project !== "all" && assoc.key !== filters.project) return false;
      if (!hasQuery) return true;
      return scoreMultiTerm(filters.query, [
        { value: run.title, weight: 4.0, subsequence: true },
        { value: assoc.name, weight: 3.0, subsequence: true },
        ...(assoc.command?.aliases ?? []).map(alias => ({ value: alias, weight: 3.2, subsequence: true })),
        { value: run.text, weight: 2.0, subsequence: true },
        { value: assoc.command?.source ?? "", weight: 1.4, subsequence: false },
        { value: run.directory, weight: 1.2, subsequence: false },
        { value: run.shell, weight: 1.0, subsequence: false },
        { value: m[run.status], weight: 1.0, subsequence: false },
        { value: run.id, weight: 1.0, subsequence: false },
        { value: run.exitCode !== undefined ? String(run.exitCode) : "", weight: 0.8, subsequence: false },
      ], locale) > 0;
    }).sort((a, b) => startedAt(b) - startedAt(a) || a.id.localeCompare(b.id));
  }, [runs, associations, filters.query, filters.project, locale, m]);
  const rows = useMemo(() => {
    const open = { activeRuns: activeOpen, unconfirmedRuns: unknownOpen, externalRuns: externalOpen, completedRuns: completedOpen };
    return sections.flatMap(({ label, states }) => {
      const items = matching.filter(run => states.includes(run.status));
      return items.length ? [{ id: `section:${label}`, label, count: items.length, run: undefined as Run | undefined },
        ...(open[label] ? items.map(run => ({ id: `run:${run.id}`, label, count: 0, run })) : [])] : [];
    });
  }, [matching, activeOpen, unknownOpen, externalOpen, completedOpen]);
  const focusedIndex = rows.findIndex(row => row.id === focused);
  const virtualizer = usePageVirtualizer({ count: rows.length, getScrollElement: () => container.current, getItemKey: i => rows[i].id,
    estimateSize: i => rows[i].run ? 48 : 40, overscan: 5, useFlushSync: false, initialOffset: position.read() ?? 0,
    rangeExtractor: range => [...new Set([...defaultRangeExtractor(range), ...(focusedIndex >= 0 ? [focusedIndex] : [])])].sort((a, b) => a - b) });
  function changeFilters(next: HistoryFilters) {
    setLocalFilters(next); onFiltersChange?.(next); setFocused(undefined);
    if (next.query.trim() || next.project !== "all") Object.values(setExpanded).forEach(change => change(true));
    position.write(0); virtualizer.scrollToOffset(0);
  }
  return <>
    <div className="flex shrink-0 flex-wrap items-center gap-2 px-5 py-3" role="search" aria-label={m.searchRuns}>
      <div className="relative min-w-40 flex-1">
        <Search className="pointer-events-none absolute left-3 top-1/2 size-4 -translate-y-1/2 text-muted-foreground" aria-hidden="true"/>
        <Input ref={search} type="search" value={filters.query} onChange={event => changeFilters({ ...filters, query: event.target.value })}
          onKeyDown={event => {
            if (event.nativeEvent.isComposing) return;
            if (event.key === "Escape" && filters.query) {
              event.preventDefault(); event.stopPropagation();
              changeFilters({ ...filters, query: "" });
              return;
            }
            if (event.key === "ArrowDown" && rows.length > 0) {
              event.preventDefault();
              setFocused(rows[0].id);
              virtualizer.scrollToIndex(0, { align: "auto" });
              requestAnimationFrame(() => container.current?.querySelector<HTMLElement>("[data-history-id]")?.focus({ preventScroll: true }));
            }
          }}
          aria-label={m.searchRuns} placeholder={m.searchRuns} autoComplete="off" spellCheck={false} className="pl-9"/>
      </div>
      <div className="w-44 max-w-full"><Picker label={m.project} value={filters.project} options={projectOptions}
        onChange={project => changeFilters({ ...filters, project })}/></div>
      {filtered && <Button variant="ghost" size="icon-sm" aria-label={m.clear} title={m.clear}
        onClick={() => { changeFilters({ query: "", project: "all" }); search.current?.focus(); }}><X className="size-4" aria-hidden="true"/></Button>}
    </div>
    <div ref={container} style={{ overflowAnchor: "none" }} className="min-h-0 flex-1 overflow-auto overscroll-contain" onScroll={event => { if (event.currentTarget.clientHeight) position.write(event.currentTarget.scrollTop); }}
    onFocus={event => setFocused((event.target as HTMLElement).dataset.historyId)}
    onBlur={event => { if (!event.currentTarget.contains(event.relatedTarget)) setFocused(undefined); }}
    onKeyDown={event => {
      const target = event.target as HTMLElement;
      const current = rows.findIndex(row => row.id === target.dataset.historyId);
      if (current < 0) return;
      if (!rows[current].run && ["ArrowLeft", "ArrowRight"].includes(event.key)) {
        event.preventDefault(); setExpanded[rows[current].label](event.key === "ArrowRight"); return;
      }
      if (!["ArrowUp", "ArrowDown", "Home", "End"].includes(event.key)) return;
      const next = event.key === "Home" ? 0 : event.key === "End" ? rows.length - 1 : Math.max(0, Math.min(rows.length - 1, current + (event.key === "ArrowDown" ? 1 : -1)));
      event.preventDefault(); setFocused(rows[next].id); virtualizer.scrollToIndex(next, { align: "auto" });
      requestAnimationFrame(() => container.current?.querySelectorAll<HTMLElement>("[data-history-id]").forEach(element => {
        if (element.dataset.historyId === rows[next].id) element.focus({ preventScroll: true });
      }));
    }}>
    {!rows.length && <div className="space-y-3 p-5">
      <p className="text-sm text-muted-foreground" role="status">{filtered ? m.noMatchingRuns : m.noRuns}</p>
      {filtered && <Button variant="outline" size="sm" onClick={() => { changeFilters({ query: "", project: "all" }); search.current?.focus(); }}>{m.clear}</Button>}
    </div>}
    <div role="list" aria-label={m.history} style={{ height: virtualizer.getTotalSize(), position: "relative" }}>
      {virtualizer.getVirtualItems().map(item => {
        const row = rows[item.index], run = row.run;
        return <div key={row.id} ref={virtualizer.measureElement} data-index={item.index} role="listitem" aria-posinset={item.index + 1} aria-setsize={rows.length}
          style={{ position: "absolute", top: 0, left: 0, width: "100%", transform: `translateY(${item.start}px)` }}>
          {run ? <Button variant="ghost" className="flex h-12 w-full min-w-0 items-center gap-3 px-5 text-left font-normal"
            title={`${run.title} · ${associations.get(run.id)!.name} · ${run.directory}`} onClick={() => onOpen(run.id)} data-history-id={row.id} data-navigation-key={`history:${run.id}`}>
            <span className="min-w-0 flex-1 truncate"><HighlightText text={run.title} query={filters.query} locale={locale} /></span>
            <span className="max-w-28 truncate text-xs text-muted-foreground"><HighlightText text={associations.get(run.id)!.name} query={filters.query} locale={locale} /></span>
            <time dateTime={startedAt(run) ? run.time : undefined} className="shrink-0 text-xs tabular-nums text-muted-foreground" aria-label={`${m.startedAt} · ${formatRunTime(run.time, locale)}`}>{formatRunTime(run.time, locale)}</time>
            <RunStatus state={run.status} m={m} iconOnly/>
          </Button> : <h2 className="border-t border-border"><Button variant="ghost" className="h-10 w-full justify-start gap-2 px-5 text-xs font-semibold text-muted-foreground"
            aria-expanded={expanded[row.label]} data-history-id={row.id} data-navigation-key={`history:${row.id}`}
            onClick={() => { setFocused(row.id); setExpanded[row.label](!expanded[row.label]); }}>
            {expanded[row.label] ? <Minus className="size-4" aria-hidden="true"/> : <Plus className="size-4" aria-hidden="true"/>}
            {m[row.label]} · {new Intl.NumberFormat(locale).format(row.count)}
          </Button></h2>}
        </div>;
      })}
    </div>
  </div></>;
}

