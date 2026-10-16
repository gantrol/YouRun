import { LoaderCircle, Pencil, Play, RotateCw, Square } from "lucide-react";
import { displayedRunState, type RunOperation } from "./models/run-operation";
import { Button } from "@/renderer/components/ui/button";
import { RunStatus } from "./run-state";
import type { Run } from "./model";
import type { Messages } from "./messages";
import { useEffect, useLayoutEffect, useRef, useState } from "react";
import { streamAPI } from "./api";
import { CommandFailure } from "./command-failure";
import { useViewPosition } from "@/renderer/components/ui/view-state";
import { Disclosure } from "@/renderer/components/ui/disclosure";

export function RunOutput({
  run,
  m,
  onStop,
  onReplay,
  onRecheck,
  onEdit,
  operation,
  launchError,
  showEditAction = true,
}: {
  run: Run;
  m: Messages;
  onStop?: (id: string) => void | Promise<void>;
  onReplay?: (id: string) => void | Promise<void>;
  onRecheck?: () => Promise<void>;
  onEdit?: () => void;
  operation?: RunOperation;
  launchError?: string;
  showEditAction?: boolean;
}) {
  const [localOperation, setLocalOperation] = useState<RunOperation>();
  const ownOperation = operation?.runId === run.id ? operation : localOperation?.runId === run.id ? localOperation : undefined;
  const pending = !!operation || !!localOperation;
  const state = displayedRunState(run, ownOperation) ?? run.status;
  const replaying = !!ownOperation && ownOperation.state !== "stopping";
  const stopping = state === "stopping";
  const [checking, setChecking] = useState(false);
  const [checkError, setCheckError] = useState("");
  const [output, setOutput] = useState("");
  const [issue, setIssue] = useState("");
  const [truncated, setTruncated] = useState(false);
  const logRef = useRef<HTMLPreElement>(null);
  const following = useRef(true);
  const position = useViewPosition(`run:${run.id}:output`);
  const restoring = useRef<number | undefined>(position.read());
  useLayoutEffect(() => {
    const element = logRef.current;
    if (!element) return;
    if (restoring.current !== undefined) {
      element.scrollTop = restoring.current;
      if (element.scrollHeight - element.clientHeight >= restoring.current) restoring.current = undefined;
    } else if (following.current) element.scrollTop = element.scrollHeight;
  }, [output]);
  useEffect(() => {
    const controller = new AbortController();
    let cursor: number | undefined;
    let decoder = new TextDecoder();
    setOutput(""); setIssue(""); setTruncated(false); restoring.current = position.read(); following.current = restoring.current === undefined;
    if (run.omitOutput) return;
    type Event = { error?: string; reset?: boolean; chunk?: { data: string; cursor: number; nextCursor: number } };
    void streamAPI<Event>(() => `log-events?id=${encodeURIComponent(run.id)}${cursor === undefined ? "" : `&cursor=${cursor}`}`, event => {
      if (event.error) { setIssue(m.logMissing); return; }
      if (!event.chunk) return;
      const { chunk } = event;
      if (event.reset) decoder = new TextDecoder();
      const text = decoder.decode(Uint8Array.from(atob(chunk.data), c => c.charCodeAt(0)), { stream: true });
      cursor = chunk.nextCursor; setIssue("");
      if (event.reset) { setOutput(text.slice(-131072)); setTruncated(chunk.cursor > 0 || text.length > 131072); }
      else if (text) setOutput(previous => (previous + text).slice(-131072));
      if (chunk.nextCursor > 131072) setTruncated(true);
    }, () => setIssue(m.logMissing), controller.signal);
    return () => controller.abort();
  }, [run.id, run.omitOutput, m.logMissing]);
  return (
    <section className="flex min-h-0 flex-1 flex-col" aria-label={m.outputTab}>
      <div className="mb-3 flex flex-wrap items-center justify-between gap-2">
        <h2 className="text-sm font-medium">{m.outputTab}</h2>
        <span
          className="flex items-center gap-2 text-xs text-muted-foreground"
          role="status"
        >
          <span className="tabular-nums">
            {new Intl.DateTimeFormat(document.documentElement.lang, {
              dateStyle: "short",
              timeStyle: "medium",
            }).format(new Date(run.time))}
          </span>
          <RunStatus state={state} m={m} />
        </span>
        {onEdit && showEditAction && <Button variant="ghost" size="sm" onClick={onEdit} data-navigation-key={`run:${run.id}:edit`}>
          <Pencil className="size-4" aria-hidden="true" />{m.editCommand}
        </Button>}
        {onReplay && run.status !== "unknown" && (
          <Button
            variant="outline"
            size="sm"
            className="min-w-28"
            disabled={pending || stopping}
            aria-busy={replaying}
            onClick={async () => {
              setLocalOperation({ state: run.status === "running" ? "restarting" : "starting", runId: run.id, previousRunId: run.id });
              try {
                await onReplay(run.id);
              } finally {
                setLocalOperation(undefined);
              }
            }}
          >
            {replaying ? ownOperation?.state === "restarting" ? <RotateCw className="size-4 motion-safe:animate-spin" aria-hidden="true" /> : <LoaderCircle className="size-4 motion-safe:animate-spin" aria-hidden="true" /> : run.status === "running" || stopping ? (
              <RotateCw className="size-4" aria-hidden="true" />
            ) : (
              <Play className="size-4" aria-hidden="true" />
            )}
            {replaying ? m[ownOperation!.state] : run.status === "running" || stopping ? m.restart : m.runAgain}
          </Button>
        )}
        {onStop && <span className="inline-flex min-w-24">{run.status === "running" || stopping ? (
          <Button variant="outline" size="sm" className="min-w-24" disabled={pending || stopping} aria-busy={stopping} onClick={async () => {
            setLocalOperation({ state: "stopping", runId: run.id, previousRunId: run.id });
            try { await onStop(run.id); } finally { setLocalOperation(undefined); }
          }}>
            {stopping ? <LoaderCircle className="size-4 motion-safe:animate-spin" aria-hidden="true" /> : <Square className="size-4" aria-hidden="true" />}
            {stopping ? m.stopping : m.stop}
          </Button>
        ) : null}</span>}
      </div>
      {run.status === "unknown" && <div className="mb-3 flex flex-wrap items-center gap-2 text-xs text-muted-foreground">
        <span role="status">{m.runOutcomeUnknown}</span>
        {onRecheck && <Button variant="outline" size="sm" disabled={checking || pending} aria-busy={checking} onClick={async () => {
          setChecking(true); setCheckError("");
          try { await onRecheck(); }
          catch { setCheckError(m.connectionLost); }
          finally { setChecking(false); }
        }}><RotateCw className={`size-4 ${checking ? "motion-safe:animate-spin" : ""}`} aria-hidden="true"/>{m.recheckRun}</Button>}
        {checkError && <span role="alert" className="text-destructive">{checkError}</span>}
      </div>}
      {!pending && <CommandFailure error={launchError} run={run} m={m} onEdit={onEdit} />}
        {run.exitCode !== undefined && (
          <p className="mb-2 text-xs text-muted-foreground">{m.exitCode} · <span className="tabular-nums">{run.exitCode}</span></p>
        )}
      {run.omitOutput ? <p role="status" className="flex-1 text-sm text-muted-foreground">{m.outputNotRecorded}</p> : <pre ref={logRef} onWheel={() => { restoring.current = undefined; }} onPointerDown={() => { restoring.current = undefined; }} onScroll={event => {
        if (restoring.current !== undefined) return;
        const el = event.currentTarget; following.current = el.scrollHeight - el.scrollTop - el.clientHeight < 24; position.write(el.scrollTop);
      }}
        className="min-h-32 flex-1 overflow-auto whitespace-pre-wrap break-all bg-surface-sunken p-3 text-xs leading-6"
        aria-label={m.recentOutput}
      >{`$ ${run.text}\n${output}`}</pre>}
      {!run.omitOutput && (issue || truncated) && (
        <p role="status" className="mt-2 text-xs text-muted-foreground">
          {issue || m.logsTruncated}
        </p>
      )}
      <Disclosure key={run.id} stateKey={`run:${run.id}:context`} label={m.snapshot} className="mt-3 max-h-[35%] shrink-0 overflow-auto">
      <dl className="space-y-1 text-xs text-muted-foreground">
        <div className="flex gap-2">
          <dt>{m.directory}</dt>
          <dd className="break-all">{run.directory}</dd>
        </div>
        <div className="flex gap-2">
          <dt>{m.shell}</dt>
          <dd>{run.shell}</dd>
        </div>
        {Object.entries(run.values).map(([key, value]) => (
          <div key={key} className="flex gap-2">
            <dt>{m[key as keyof Messages] ?? key}</dt>
            <dd className="break-all">{value}</dd>
          </div>
        ))}
      </dl>
      </Disclosure>
    </section>
  );
}
