import { memo, useLayoutEffect, useMemo, useRef, useState, type ReactNode } from "react";
import { defaultRangeExtractor } from "@tanstack/react-virtual";
import { usePageVirtualizer } from "./page-virtualizer";
import { FileCode2 } from "lucide-react";
import { Button } from "@/renderer/components/ui/button";
import { CommandAction } from "./command-action";
import { CommandFailure } from "./command-failure";
import { CommandMenu, type LauncherMenuActions } from "./launcher-menu";
import { commandTitle, isActiveRun, type Command, type Run } from "./model";
import { HighlightText, commandContexts, compactContext } from "./collections";
import { RunStatus } from "./run-state";
import { displayedRunState, type RunOperation, type RunOperations } from "./models/run-operation";
import type { Messages } from "./messages";
import "./command-list.css";

type Actions = {
  onOpen: (command: Command) => void;
  onRun: (command: Command) => void;
  onStop: (id: string) => void;
  onReplay: (id: string) => void;
  onOutput: (run: Run) => void;
  menus: LauncherMenuActions;
};
type Props = Actions & {
  header?: ReactNode;
  contexts?: Record<string, string>;
  directories?: Record<string, string>;
  query?: string;
  onClearSearch?: () => void;
  commands: Command[]; runs: Run[]; m: Messages; selectedId?: string;
  runOperations: RunOperations; launchErrors: Record<string, string>;
  revealId?: string; onRevealed?: () => void; initialOffset?: number;
  onScroll?: (offset: number) => void; className?: string;
};

const CommandRow = memo(function CommandRow({ command, latest, active, selected, operation, error, context, unbound, query, m, actions }: {
  command: Command; latest?: Run; active?: Run; selected: boolean; operation?: RunOperation;
  error?: string; context?: string; unbound: boolean; query?: string; m: Messages; actions: Actions;
}) {
  const name = commandTitle(command, m);
  const output = active ?? latest;
  const state = displayedRunState(operation ? latest : active ?? latest, operation);
  return <CommandMenu command={command} run={active} m={m} actions={actions.menus}
    onRun={actions.onRun} onOpen={actions.onOpen} onStop={actions.onStop} onReplay={actions.onReplay}>
    <div className={selected ? "bg-selected" : "hover:bg-hover focus-within:bg-hover"}>
      <div className="flex h-12 min-w-0 items-center gap-2 px-3">
        <Button variant="ghost" className="h-10 min-w-0 flex-1 justify-start gap-2 px-0 text-left font-normal hover:bg-transparent"
          data-command-row={command.id} data-navigation-key={`command:${command.id}`} title={[name, context].filter(Boolean).join(" · ")}
          aria-current={selected ? "true" : undefined} onClick={() => actions.onOpen(command)}>
          <FileCode2 className="size-4 shrink-0 text-muted-foreground" aria-hidden="true" />
          <span className="command-row-name min-w-0 flex-1 truncate"><HighlightText text={name} query={query} /></span>
          {command.hidden && <span className="shrink-0 text-xs text-muted-foreground">{m.hidden}</span>}
          {context && <span className="command-row-context min-w-0 truncate text-xs text-muted-foreground"><HighlightText text={compactContext(context)} query={query} /></span>}
          <span className="flex min-w-5 shrink-0 justify-end">{state ? <RunStatus state={state} m={m} iconOnly /> : (command.unavailable || unbound) && <span className="text-xs text-muted-foreground">{command.unavailable ? m.unavailable : m.unbound}</span>}</span>
        </Button>
        <CommandAction m={m} name={name} icon={command.icon} running={!!active} stopping={active?.status === "stopping"}
          onStop={active ? () => actions.onStop(active.id) : undefined} onReplay={active ? () => actions.onReplay(active.id) : undefined}
          onOpen={() => actions.onOpen(command)} onOutput={output ? () => actions.onOutput(output) : undefined}
          operation={operation?.state} onClick={() => actions.onRun(command)} />
      </div>
      {!operation && (error || latest?.error || latest?.status === "failed") && <div className="px-3 pb-2"><CommandFailure m={m} error={error} run={latest}
        onEdit={() => actions.menus.editCommand(command.id)} onOutput={latest ? () => actions.onOutput(latest) : undefined} /></div>}
    </div>
  </CommandMenu>;
});

export function CommandList(props: Props) {
  const { commands, runs, m } = props;
  const container = useRef<HTMLDivElement>(null);
  const header = useRef<HTMLDivElement>(null);
  const [headerHeight, setHeaderHeight] = useState(0);
  useLayoutEffect(() => {
    const element = header.current; if (!element) return;
    const update = () => { if (container.current?.clientHeight) setHeaderHeight(element.offsetHeight); };
    const observer = new ResizeObserver(update);
    observer.observe(element); update();
    return () => observer.disconnect();
  }, []);
  const current = useRef(props);
  useLayoutEffect(() => { current.current = props; });
  const actions = useMemo<Actions>(() => ({
    onOpen: c => current.current.onOpen(c), onRun: c => current.current.onRun(c),
    onStop: id => current.current.onStop(id), onReplay: id => current.current.onReplay(id),
    onOutput: r => current.current.onOutput(r),
    menus: new Proxy({} as LauncherMenuActions, { get: (_target, key) => current.current.menus[key as keyof LauncherMenuActions] }),
  }), []);
  const index = useMemo(() => {
    const latest = new Map<string, Run>(), active = new Map<string, Run>();
    for (const run of runs) {
      if (!latest.has(run.commandId)) latest.set(run.commandId, run);
      if (isActiveRun(run) && !active.has(run.commandId)) active.set(run.commandId, run);
    }
    return { latest, active };
  }, [runs]);
  const contexts = useMemo(() => commandContexts(commands, [], m, props.query), [commands, m, props.query]);
  const [focusedId, setFocusedId] = useState<string>();
  const focusedIndex = commands.findIndex(c => c.id === focusedId);
  const revealIndex = commands.findIndex(c => c.id === props.revealId);
  const virtualizer = usePageVirtualizer({
    scrollMargin: headerHeight,
    count: commands.length, getScrollElement: () => container.current,
    getItemKey: index => commands[index].id, estimateSize: () => 48, overscan: 6,
    initialOffset: props.initialOffset ?? 0, useFlushSync: false,
    rangeExtractor: range => [...new Set([...defaultRangeExtractor(range), ...[focusedIndex, revealIndex].filter(index => index >= 0)])].sort((a, b) => a - b),
  });
  useLayoutEffect(() => {
    if (revealIndex < 0 || !props.revealId || !container.current?.clientHeight) return;
    virtualizer.scrollToIndex(revealIndex, { align: "auto" });
    const frame = requestAnimationFrame(() => {
      const element = Array.from(container.current?.querySelectorAll<HTMLElement>("[data-command-row]") ?? [])
        .find(element => element.dataset.commandRow === props.revealId);
      if (element) { element.focus({ preventScroll: true }); props.onRevealed?.(); }
    });
    return () => cancelAnimationFrame(frame);
  }, [props.revealId, revealIndex, headerHeight, virtualizer, virtualizer.scrollRect?.height]);
  return <div ref={container} style={{ overflowAnchor: "none" }} className={`min-h-0 flex-1 overflow-auto overscroll-contain ${props.className ?? ""}`}
    onScroll={event => { if (event.currentTarget.clientHeight) props.onScroll?.(event.currentTarget.scrollTop); }}
    onBlur={event => { if (!event.currentTarget.contains(event.relatedTarget)) setFocusedId(undefined); }}
    onFocus={event => setFocusedId((event.target as HTMLElement).closest<HTMLElement>("[data-command-id]")?.dataset.commandId)}
    onKeyDown={event => {
      const target = event.target as HTMLElement;
      if (!target.hasAttribute("data-command-row") || !["ArrowDown", "ArrowUp", "Home", "End"].includes(event.key)) return;
      const i = commands.findIndex(c => c.id === target.dataset.commandRow);
      const next = event.key === "Home" ? 0 : event.key === "End" ? commands.length - 1 : Math.max(0, Math.min(commands.length - 1, i + (event.key === "ArrowDown" ? 1 : -1)));
      event.preventDefault(); setFocusedId(commands[next]?.id); virtualizer.scrollToIndex(next, { align: "auto" });
      requestAnimationFrame(() => container.current?.querySelectorAll<HTMLElement>("[data-command-row]").forEach(el => {
        if (el.dataset.commandRow === commands[next]?.id) el.focus({ preventScroll: true });
      }));
    }}>
    <div ref={header}>{props.header}</div>
    {!commands.length ? props.header ? null : props.query?.trim() ? <div className="space-y-3 px-5 py-8">
      <p role="status" className="text-sm text-muted-foreground">{m.emptyResults}</p>
      {props.onClearSearch && <Button variant="outline" size="sm" onClick={props.onClearSearch}>{m.clear}</Button>}
    </div> : <p role="status" className="px-5 py-8 text-sm text-muted-foreground">{m.noCommands}</p> :
      <div role="list" aria-label={m.commandsList} style={{ height: virtualizer.getTotalSize(), position: "relative" }}>
        {virtualizer.getVirtualItems().map(row => {
          const command = commands[row.index];
          return <div key={row.key} ref={virtualizer.measureElement} data-index={row.index} data-command-id={command.id}
            role="listitem" aria-posinset={row.index + 1} aria-setsize={commands.length}
            style={{ position: "absolute", top: 0, left: 0, width: "100%", transform: `translateY(${row.start - headerHeight}px)` }}>
            <CommandRow command={command} latest={index.latest.get(command.id)} active={index.active.get(command.id)}
              unbound={!(props.directories?.[command.id] ?? command.directory).trim()} selected={command.id === props.selectedId} operation={props.runOperations[command.id]} error={props.launchErrors[command.id]}
              context={props.contexts?.[command.id] ?? contexts[command.id]} query={props.query} m={m} actions={actions}/>
          </div>;
        })}
      </div>}
  </div>;
}

