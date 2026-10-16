import { useEffect, useRef, useState } from "react";
import { FileText, LoaderCircle, Square } from "lucide-react";
import { Button } from "@/renderer/components/ui/button";
import { ActionMenu, ActionMenuItem, ActionMenuLabel } from "@/renderer/components/ui/action-menu";
import { RabbitActivity } from "./rabbit-activity";
import { commandTitle, type Command, type Project, type Run } from "./model";
import { displayedRunState, type RunOperations } from "./models/run-operation";
import { RunStatus } from "./run-state";
import type { Messages } from "./messages";
import "./project-activity.css";

export function ProjectActivity({ project, commands, activeRuns, latestRuns, operations, m, onStop, onOutput, onOpen, separated = true }: {
  project: Project;
  separated?: boolean;
  commands: Command[];
  activeRuns: ReadonlyMap<string, Run>;
  latestRuns: ReadonlyMap<string, Run>;
  operations: RunOperations;
  m: Messages;
  onStop: (id: string) => void;
  onOutput: (run: Run) => void;
  onOpen: (command: Command) => void;
}) {
  const [open, setOpen] = useState(false);
  const trigger = useRef<HTMLButtonElement>(null);
  const heading = useRef<HTMLButtonElement | null>(null);
  const triggerFocused = useRef(false);
  // Include every command, including hidden commands and those without a shortcut.
  // Keep pending launches/restarts visible before their new run has arrived.
  const entries = commands.flatMap(command => {
    const operation = operations[command.id];
    const run = operation ? latestRuns.get(command.id) : activeRuns.get(command.id) ?? latestRuns.get(command.id);
    const state = displayedRunState(run, operation);
    return state === "starting" || state === "restarting" || state === "running" || state === "stopping"
      ? [{ command, run, state, operation }] : [];
  });
  useEffect(() => {
    if (!entries.length && !open && triggerFocused.current) {
      triggerFocused.current = false;
      heading.current?.focus();
    }
  }, [entries.length, open]);
  const visible = entries.length > 0 || open;
  const lastCount = useRef(entries.length);
  const count = new Intl.NumberFormat(document.documentElement.lang).format(visible ? entries.length : lastCount.current);
  const label = `${project.name} · ${m.activeRuns} · ${count}`;
  const state = entries.some(entry => entry.state === "running") ? "running" : entries.length ? entries[0].state : "stopped";
  const previousState = useRef(state);
  const [hop, setHop] = useState(false);
  useEffect(() => {
    setHop(state === "running" && ["starting", "restarting"].includes(previousState.current));
    previousState.current = state;
  }, [state]);
  useEffect(() => { if (entries.length) lastCount.current = entries.length; }, [entries.length]);
  const rememberHeading = (button: HTMLButtonElement | null) => {
    trigger.current = button;
    heading.current = button?.closest('[data-project-id]')?.querySelector<HTMLButtonElement>('[data-project-heading]') ?? null;
  };
  return <div className="project-activity" data-visible={visible || undefined} data-separated={separated || undefined}
    aria-hidden={!visible} inert={!visible}>
    <div className="project-activity-clip">
      <ActionMenu label={label} className="w-72" enabled={visible}
        onOpenChange={(value, button) => { rememberHeading(button); setOpen(value); }}
        getReturnFocus={() => entries.length ? trigger.current : heading.current}
        trigger={props => <Button {...props} variant="ghost" size="sm"
          className={`project-activity-trigger h-8 gap-1.5 px-1.5 ${state === "running" ? "text-success" : state === "stopping" || state === "restarting" ? "text-warning" : "text-muted-foreground"}`}
          data-project-activity={project.id} aria-label={`${label} · ${m[state]}`}
          onFocus={event => { triggerFocused.current = true; rememberHeading(event.currentTarget); }}
          onBlur={event => { if (event.relatedTarget && event.relatedTarget !== document.body) triggerFocused.current = false; }}>
          <RabbitActivity state={state === "running" ? "running" : state === "stopping" || state === "stopped" ? "stopping" : "starting"}
            size={16} animate={state !== "running" || hop} />
          <span key={count} aria-hidden="true" className="project-activity-count min-w-[1ch] text-xs tabular-nums">{count}</span>
        </Button>}>
        <ActionMenuLabel>{project.name} · {m.activeRuns}</ActionMenuLabel>
        {entries.map(({ command, run, state, operation }) => {
          const name = commandTitle(command, m);
          // A previous completed execution is not the output of a pending launch.
          const currentRun = run && (run.status === "running" || run.status === "stopping") ? run : undefined;
          const stopping = state === "stopping";
          return <div key={command.id} role="group" aria-label={name} className="flex min-w-0 items-center gap-1 py-0.5">
            <ActionMenuItem className="min-w-0 flex-1" title={`${name} · ${currentRun ? m.viewRun : m.viewCommand}`}
              aria-label={`${name} · ${currentRun ? m.viewRun : m.viewCommand} · ${m[state]}`}
              onSelect={() => currentRun ? onOutput(currentRun) : onOpen(command)}>
              <RunStatus state={state} m={m} iconOnly />
              <span className="flex-1 truncate">{name}</span>
              <FileText aria-hidden="true" className="text-muted-foreground" />
            </ActionMenuItem>
            <ActionMenuItem className="size-8 shrink-0 justify-center p-0 text-destructive"
              data-command-control="stop" disabled={!currentRun || currentRun.status !== "running" || !!operation}
              title={`${name} · ${stopping ? m.stopping : m.stop}`} aria-label={`${name} · ${stopping ? m.stopping : m.stop}`}
              onSelect={event => { event.preventDefault(); if (currentRun) onStop(currentRun.id); }}>
              {stopping ? <LoaderCircle aria-hidden="true" className="motion-safe:animate-spin" /> : <Square aria-hidden="true" />}
            </ActionMenuItem>
          </div>;
        })}
        {!entries.length && <ActionMenuItem disabled>{m.noActiveRuns}</ActionMenuItem>}
      </ActionMenu>
    </div>
  </div>;
}
