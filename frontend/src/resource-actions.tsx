import { createContext, Fragment, useContext, useEffect, useMemo, useRef, useState, type ComponentProps, type ReactElement, type ReactNode } from "react";
import { Check, FileText, LoaderCircle, MoreHorizontal, RotateCw, Shield, Square, X } from "lucide-react";
import { Button } from "@/renderer/components/ui/button";
import { Checkbox } from "@/renderer/components/ui/checkbox";
import { ContextMenu, ContextMenuContent, ContextMenuItem, ContextMenuSeparator, ContextMenuTrigger } from "@/renderer/components/ui/context-menu";
import { DropdownMenu, DropdownMenuContent, DropdownMenuItem, DropdownMenuSeparator, DropdownMenuTrigger } from "@/renderer/components/ui/dropdown-menu";
import { Dialog, DialogContent, DialogTitle } from "@/renderer/components/ui/dialog";
import { api } from "./api";
import type { Messages } from "./messages";
import type { Command, Run } from "./model";
import type { RunOperations } from "./models/run-operation";

export type ResourceProcess = {
  pid: number;
  processIdentity?: string;
  controlRunId?: string;
  name?: string;
  displayName?: string;
  processName?: string;
  processDisplayName?: string;
  ownership?: string;
  runId?: string;
  commandTitle?: string;
  projectId?: string;
  projectName?: string;
  workspaceId?: string;
  workspaceName?: string;
};
export type ResourceRunActions = {
  runs: Run[];
  commands: Command[];
  operations: RunOperations;
  errors: Record<string, string>;
  onOpenRun: (item: ResourceProcess, run: Run) => void;
  onOpenCommand: (item: ResourceProcess, run: Run, command: Command) => void;
  onReplay: (id: string) => Promise<void>;
  onStop: (id: string) => Promise<void>;
};
type Target = { pid: number; startedAt: string; name: string; runId?: string };
type Result = Target & { status: string };
type Mode = "graceful" | "force";
type Scope = "task" | "process" | "tree";
type Plan = { id: string; mode: Mode; scope: Scope; targets: Target[]; results?: Result[]; canElevate: boolean };
const key = (item: ResourceProcess) => `${item.pid}:${item.processIdentity ?? ""}`;
const title = (item: ResourceProcess) => item.displayName || item.processDisplayName || item.name || item.processName || String(item.pid);
const unique = (items: ResourceProcess[]) => Array.from(new Map(items.map(item => [key(item), item])).values());

type Controls = {
  m: Messages;
  busy: boolean;
  selected: ResourceProcess[];
  selectedKeys: Set<string>;
  toggle: (items: ResourceProcess[], checked: boolean) => void;
  prepare: (items: ResourceProcess[], mode: Mode, scope: Scope) => void;
  runActions: ResourceRunActions;
  controlRun: (id: string, action: (id: string) => Promise<void>) => void;
};
const ControlContext = createContext<Controls | null>(null);
function useControls() { const value = useContext(ControlContext); if (!value) throw new Error("ResourceActionsProvider missing"); return value; }

export function ResourceActionsProvider({ items, m, onRefresh, runActions, children }: {
  items: ResourceProcess[]; m: Messages; onRefresh: () => void | Promise<void>; runActions: ResourceRunActions; children: ReactNode;
}) {
  const [selected, setSelected] = useState<ResourceProcess[]>([]);
  const [plan, setPlan] = useState<Plan>();
  const [busy, setBusy] = useState(false);
  const [issue, setIssue] = useState("");
  const inFlight = useRef(false);
  const trigger = useRef<HTMLElement | null>(null);
  const cancel = useRef<HTMLButtonElement>(null);
  const mounted = useRef(true);
  useEffect(() => { mounted.current = true; return () => { mounted.current = false; }; }, []);
  useEffect(() => {
    const current = new Map(items.map(item => [key(item), item]));
    setSelected(previous => previous.flatMap(item => current.has(key(item)) ? [current.get(key(item))!] : []));
  }, [items]);
  const selectedKeys = useMemo(() => new Set(selected.map(key)), [selected]);
  const message = (code: string) => m[code as keyof Messages] ?? m.processControlFailed;
  async function prepare(items: ResourceProcess[], mode: Mode, scope: Scope) {
    if (inFlight.current) return;
    inFlight.current = true; setBusy(true); setIssue("");
    trigger.current = document.activeElement instanceof HTMLElement ? document.activeElement : null;
    try {
      const next = await api<Plan>("resources/control", "POST", {
        action: "prepare", mode, scope,
        targets: unique(items).map(item => ({ pid: item.pid, startedAt: item.processIdentity ?? "", name: title(item), runId: item.controlRunId })),
      });
      if (mounted.current) setPlan(next);
    } catch (error) { if (mounted.current) setIssue(error instanceof Error ? error.message : "processControlFailed"); }
    finally { inFlight.current = false; if (mounted.current) setBusy(false); }
  }
  async function execute(elevate = false) {
    if (inFlight.current || !plan) return;
    inFlight.current = true; setBusy(true); setIssue("");
    try {
      const result = await api<Plan>("resources/control", "POST", { action: "execute", id: plan.id, elevate });
      if (mounted.current) setPlan(result);
      await onRefresh();
    } catch (error) { if (mounted.current) setIssue(error instanceof Error ? error.message : "processControlFailed"); }
    finally { inFlight.current = false; if (mounted.current) setBusy(false); }
  }
  async function reviewForce() {
    if (inFlight.current || !plan) return;
    inFlight.current = true; setBusy(true); setIssue("");
    try {
      const next = await api<Plan>("resources/control", "POST", { action: "force-preview", id: plan.id });
      if (mounted.current) setPlan(next);
    } catch (error) { if (mounted.current) setIssue(error instanceof Error ? error.message : "processControlFailed"); }
    finally { inFlight.current = false; if (mounted.current) setBusy(false); }
  }
  async function controlRun(id: string, action: (id: string) => Promise<void>) {
    if (inFlight.current) return;
    inFlight.current = true; setBusy(true); setIssue("");
    try {
      await action(id);
      await onRefresh();
    } catch (error) { if (mounted.current) setIssue(error instanceof Error ? error.message : "requestFailed"); }
    finally { inFlight.current = false; if (mounted.current) setBusy(false); }
  }
  const controls: Controls = { m, busy: busy || Object.values(runActions.operations).some(Boolean), selected, selectedKeys, prepare, runActions, controlRun,
    toggle: (items, checked) => {
      if (inFlight.current) return;
      const next = checked ? unique([...selected, ...items]) : selected.filter(item => !items.some(target => key(target) === key(item)));
      if (next.length > 128) { setIssue("processTooManyTargets"); return; }
      setSelected(next);
    },
  };
  const denied = plan?.results?.some(item => item.status === "processAccessDenied");
  const forceable = plan?.mode === "graceful" && plan.results?.filter(item => ["processExitTimeout", "processGracefulUnsupported"].includes(item.status));
  return <ControlContext.Provider value={controls}>
    {Object.entries(runActions.errors).map(([id, code]) => <div key={id} role="alert" className="border-b border-border px-4 py-2 text-xs text-destructive">
      {runActions.runs.find(run => run.commandId === id)?.title} · {m[code as keyof Messages] ?? m.requestFailed}
    </div>)}
    {issue && !plan && <div role="alert" className="flex items-center justify-between gap-2 border-b border-border px-4 py-2 text-xs text-destructive">
      {message(issue)}<Button variant="ghost" size="icon-sm" aria-label={m.close} onClick={() => setIssue("")}><X /></Button>
    </div>}
    {children}
    <Dialog open={!!plan} onOpenChange={open => { if (!open && !inFlight.current) { setPlan(undefined); setIssue(""); } }}>
      <DialogContent aria-describedby={undefined} onOpenAutoFocus={event => { event.preventDefault(); cancel.current?.focus(); }}
        onCloseAutoFocus={event => { event.preventDefault(); if (trigger.current?.isConnected) trigger.current.focus(); }}
        onEscapeKeyDown={event => { if (busy) event.preventDefault(); }} onPointerDownOutside={event => { if (busy) event.preventDefault(); }}>
        <DialogTitle>{plan?.mode === "graceful" ? m.processGraceful : plan?.scope === "tree" ? m.processEndTree : m.processEndTask} · {plan?.targets.length ?? 0}</DialogTitle>
        <div className="max-h-72 overflow-auto divide-y divide-border" aria-live="polite">
          {(plan?.results ?? plan?.targets ?? []).map(item => <div key={`${item.pid}:${item.startedAt}`} className="flex flex-wrap items-center justify-between gap-2 py-2 text-sm">
            <div className="min-w-0"><span className="break-all">{item.name}</span> <span className="font-mono text-xs text-muted-foreground">PID {item.pid}</span>
              {item.runId && <div className="break-all text-xs text-muted-foreground">{m.processWholeRun} · {item.runId}</div>}</div>
            {"status" in item && <span className={`inline-flex items-center gap-1 text-xs ${item.status === "processExited" || item.status === "processGone" ? "text-success" : "text-destructive"}`}>
              {(item.status === "processExited" || item.status === "processGone") && <Check className="size-3" />}{message(String(item.status))}</span>}
          </div>)}
        </div>
        {plan?.scope === "tree" && <div className="text-xs text-muted-foreground">{m.processTreeSnapshot}</div>}
        {plan?.mode === "force" && !plan.results && <div className="text-xs text-destructive">{m.processUnsavedWarning}</div>}
        {issue && <div role="alert" className="text-sm text-destructive">{message(issue)}</div>}
        <div className="flex flex-wrap justify-end gap-2">
          <Button ref={cancel} variant="outline" disabled={busy} onClick={() => { setPlan(undefined); setIssue(""); }}>{plan?.results ? m.close : m.cancel}</Button>
          {plan?.results && denied && plan.canElevate && <Button variant="outline" disabled={busy} onClick={() => void execute(true)}><Shield className="size-4" />{m.processElevateRetry}</Button>}
          {forceable && forceable.length > 0 && <Button variant="outline" disabled={busy} onClick={() => void reviewForce()}>{m.processForceReview}</Button>}
          {!plan?.results && <Button variant={plan?.mode === "force" ? "destructive" : "default"} disabled={busy} onClick={() => void execute()}>
            {busy && <LoaderCircle className="size-4 animate-spin" />}{busy ? m.stopping : plan?.mode === "graceful" ? m.processGraceful : m.processEndTask}
          </Button>}
          {busy && plan?.results && <span role="status" className="inline-flex items-center gap-1 text-xs"><LoaderCircle className="size-4 animate-spin" />{m.stopping}</span>}
        </div>
      </DialogContent>
    </Dialog>
  </ControlContext.Provider>;
}

function OperationItems({ items, dropdown = false }: { items: ResourceProcess[]; dropdown?: boolean }) {
  const { m, busy, prepare } = useControls();
  const Item = dropdown ? DropdownMenuItem : ContextMenuItem;
  const Separator = dropdown ? DropdownMenuSeparator : ContextMenuSeparator;
  const disabled = busy || !items.length || items.some(item => !item.processIdentity || item.pid <= 1);
  return <>
    <ManagedOperationItems items={items} dropdown={dropdown} />
    <Item disabled={disabled} onSelect={() => prepare(items, "graceful", "task")}>{m.processGraceful}</Item>
    <Item className="text-destructive" disabled={disabled} onSelect={() => prepare(items, "force", "task")}><Square />{m.processEndTask}</Item>
    <Separator />
    {items.some(item => item.controlRunId) && <Item className="text-destructive" disabled={disabled} onSelect={() => prepare(items, "force", "process")}>{m.processEndProcess}</Item>}
    <Item disabled={disabled} onSelect={() => prepare(items, "graceful", "tree")}>{m.processGracefulTree}</Item>
    <Item className="text-destructive" disabled={disabled} onSelect={() => prepare(items, "force", "tree")}>{m.processEndTree}</Item>
  </>;
}

function ManagedOperationItems({ items, dropdown }: { items: ResourceProcess[]; dropdown: boolean }) {
  const { m, busy, runActions, controlRun } = useControls();
  const Item = dropdown ? DropdownMenuItem : ContextMenuItem;
  const Separator = dropdown ? DropdownMenuSeparator : ContextMenuSeparator;
  // A process group or several endpoints can refer to the same execution.
  const managed = new Map<string, { item: ResourceProcess; run: Run; canControl: boolean }>();
  for (const item of items) {
    if (item.ownership !== "managed" || !item.runId) continue;
    const run = runActions.runs.find(run => run.id === item.runId);
    if (!run) continue;
    const previous = managed.get(run.id);
    managed.set(run.id, { item: previous?.item ?? item, run, canControl: previous?.canControl || item.controlRunId === run.id });
  }
  return Array.from(managed.values()).map(({ item, run, canControl }) => {
    const command = runActions.commands.find(command => command.id === run.commandId);
    const operation = runActions.operations[run.commandId];
    const disabled = busy || !canControl || run.status !== "running";
    const name = [item.projectName, item.commandTitle || run.title, item.workspaceName].filter(Boolean).join(" · ");
    return <Fragment key={run.id}>
      <div className="px-2 py-1.5 text-xs font-medium text-muted-foreground [overflow-wrap:anywhere]">{name}</div>
      <Item onSelect={() => runActions.onOpenRun(item, run)}><FileText aria-hidden="true" />{m.viewRun}</Item>
      <Item disabled={disabled} onSelect={() => controlRun(run.id, runActions.onReplay)}>
        <RotateCw aria-hidden="true" className={operation?.state === "restarting" ? "motion-safe:animate-spin" : undefined} />{operation?.state === "restarting" ? m.restarting : m.restart}
      </Item>
      <Item disabled={!command} onSelect={() => { if (command) runActions.onOpenCommand(item, run, command); }}><FileText aria-hidden="true" />{m.viewCommand}</Item>
      <Item className="text-destructive" disabled={disabled} onSelect={() => controlRun(run.id, runActions.onStop)}>
        {operation?.state === "stopping" ? <LoaderCircle aria-hidden="true" className="motion-safe:animate-spin" /> : <Square aria-hidden="true" />}{operation?.state === "stopping" ? m.stopping : m.stop}
      </Item>
      <Separator />
    </Fragment>;
  });
}

export function ResourceActions({ items }: { items: ResourceProcess[] }) {
  const { m, busy } = useControls();
  return <DropdownMenu><DropdownMenuTrigger asChild><Button variant="outline" size="icon-sm" disabled={busy || !items.length} aria-label={m.processActions} title={m.processActions}><MoreHorizontal className="size-4" aria-hidden="true" /></Button></DropdownMenuTrigger>
    <DropdownMenuContent><OperationItems items={items} dropdown /></DropdownMenuContent>
  </DropdownMenu>;
}

export function ResourceMenu({ items, children }: { items: ResourceProcess[]; children: ReactElement }) {
  const controls = useControls();
  const targets = items.every(item => controls.selectedKeys.has(key(item))) ? controls.selected : items;
  return <ContextMenu><ContextMenuTrigger asChild>{children}</ContextMenuTrigger><ContextMenuContent>
    <OperationItems items={targets} />
  </ContextMenuContent></ContextMenu>;
}

function Selection({ items }: { items: ResourceProcess[] }) {
  const { selectedKeys, toggle, m, busy } = useControls();
  const count = items.filter(item => selectedKeys.has(key(item))).length;
  return <Checkbox aria-label={items.length === 1 ? `${m.processSelect} ${title(items[0])} · PID ${items[0].pid}` : m.selectAll}
    checked={count === 0 ? false : count === items.length ? true : "indeterminate"}
    disabled={busy || !items.length} onClick={event => event.stopPropagation()} onDoubleClick={event => event.stopPropagation()}
    onCheckedChange={checked => toggle(items, checked === true)} />;
}

export function ResourceRow({ items, children, ...props }: ComponentProps<"tr"> & { items: ResourceProcess[] }) {
  return <ResourceMenu items={items}><tr tabIndex={0} {...props}><td className="w-8 px-2" onClick={event => event.stopPropagation()}><Selection items={items} /></td>{children}</tr></ResourceMenu>;
}
export function ResourceSelectAll({ items }: { items: ResourceProcess[] }) { return <th className="w-8 px-2"><Selection items={unique(items)} /></th>; }
export function ResourceBatchActions() {
  const { selected, toggle, m } = useControls();
  if (!selected.length) return null;
  return <div className="flex shrink-0 items-center gap-2 border-b border-border px-4 py-1.5 text-xs">
    <span>{m.processSelected} · {selected.length}</span><ResourceActions items={selected} />
    <Button variant="ghost" size="sm" onClick={() => toggle(selected, false)}>{m.clear}</Button>
  </div>;
}
