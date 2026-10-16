import { useEffect, useState } from "react";
import { Button } from "@/renderer/components/ui/button";
import { RabbitActivity } from "../rabbit-activity";
import { RunStatus, type RunState } from "../run-state";
import { CommandAction } from "../command-action";
import { YouRunLogo } from "../yourun-logo";
import type { PreviewProps } from "./component-previews";

const states: RunState[] = ["starting", "running", "restarting", "stopping", "stopped", "success", "failed"];

export function RabbitActivityPreview({ m, c, artwork = true }: Pick<PreviewProps, "m" | "c"> & { artwork?: boolean }) {
  const [state, setState] = useState<RunState>("stopped");
  // Only presentation state changes here; this preview never launches a command.
  useEffect(() => {
    if (state !== "starting" && state !== "restarting" && state !== "stopping") return;
    const timer = window.setTimeout(() => setState(state === "stopping" ? "stopped" : "running"), 1600);
    return () => window.clearTimeout(timer);
  }, [state]);
  const active = state === "starting" || state === "running" || state === "restarting" || state === "stopping";
  const operation = state === "starting" || state === "restarting" || state === "stopping" ? state : undefined;
  return (
    <div className="space-y-6">
      <div className={`flex items-center gap-8 ${artwork ? "min-h-28" : ""}`}>
        {artwork && <div className={state === "running" || state === "success" ? "text-success" : state === "failed" ? "text-destructive" : state === "stopping" ? "text-warning" : "text-muted-foreground"}>
          {active ? <RabbitActivity state={state === "restarting" ? "starting" : state} size={80} /> : <YouRunLogo size={80} />}
        </div>}
        <div className="grid gap-4">
          <span role="status" className="inline-flex min-w-28"><RunStatus state={state} m={m} /></span>
          <CommandAction m={m} name="YouRun" operation={operation}
            running={state === "running" || state === "restarting" || state === "stopping"} stopping={state === "stopping"}
            onClick={() => setState("starting")} onReplay={() => setState("restarting")} onStop={() => setState("stopping")} />
        </div>
      </div>
      <div className="flex flex-wrap gap-2" role="group" aria-label={c.preview}>
        {states.map(value => <Button key={value} size="sm" variant={state === value ? "secondary" : "outline"}
          aria-pressed={state === value} onClick={() => setState(value)}>{m[value]}</Button>)}
      </div>
    </div>
  );
}
